package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"dsc-sdk"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

const (
	decayLambda  = 0.05
	archiveDays  = 30
	timeNodeDays = 30
	// maxAutoMemoryLen 钩子自动记录单条记忆的最大长度（防长输出撑爆记忆库）
	maxAutoMemoryLen = 2000
)

// Memory 对应 memories 表
type Memory struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Content     string `gorm:"type:text;not null" json:"content"`
	CreatedAt   int64  `gorm:"not null" json:"created_at"`
	LastAccess  int64  `gorm:"not null;index" json:"last_access"`
	AccessCount int    `gorm:"not null;default:0" json:"access_count"`
	Archived    bool   `gorm:"not null;default:false;index" json:"archived"`
	Source      string `gorm:"type:text;default:'user'" json:"source"`
}

// CallLog 对应 call_logs 表
type CallLog struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Query       string `gorm:"type:text;not null" json:"query"`
	Keywords    string `gorm:"type:text;not null" json:"keywords"`
	ResultCount int    `gorm:"not null" json:"result_count"`
	CreatedAt   int64  `gorm:"not null" json:"created_at"`
}

// resultItem 是搜索返回的结果项，包含分数
type resultItem struct {
	Memory
	Score float64 `json:"score"`
}

// dbPath 返回记忆库文件路径：位于 DSC 程序可执行目录下的 memory 目录中
// （宿主把插件子进程工作目录设为可执行目录，故用 os.Getwd 解析；目录由本插件创建）。
// 记忆是跨会话共享的，并非项目级，因此不复用 DSC_WORKSPACE_ROOT。
func dbPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, "memory", "memory.db")
}

// initDB 初始化数据库：常规表自动迁移 + FTS5 虚拟表与同步触发器 + 预置高频问题。
func initDB(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var err error
	DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return err
	}

	// 自动迁移常规表（不含虚拟表）
	if err := DB.AutoMigrate(&Memory{}, &CallLog{}); err != nil {
		return err
	}

	// 创建 FTS5 虚拟表（需原生 SQL）
	ftsSQL := `
        CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
            content,
            content='memories',
            content_rowid='id',
            tokenize='unicode61'
        );`
	if err := DB.Exec(ftsSQL).Error; err != nil {
		return err
	}

	// 创建触发器保持同步
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
            INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
        END;`,
		`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
            INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.id, old.content);
        END;`,
		`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
            INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.id, old.content);
            INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
        END;`,
	}
	for _, trig := range triggers {
		if err := DB.Exec(trig).Error; err != nil {
			return err
		}
	}

	// 预置高频问题（如果表为空）
	var count int64
	DB.Model(&Memory{}).Count(&count)
	if count == 0 {
		now := time.Now().Unix()
		presets := []Memory{
			{Content: "今天天气怎么样？", CreatedAt: now, LastAccess: now, AccessCount: 0, Archived: false, Source: "preset"},
			{Content: "帮我写一份周报。", CreatedAt: now, LastAccess: now, AccessCount: 0, Archived: false, Source: "preset"},
			{Content: "最近有什么新闻？", CreatedAt: now, LastAccess: now, AccessCount: 0, Archived: false, Source: "preset"},
		}
		DB.Create(&presets)
	}
	return nil
}

// timeFactor 计算时间衰减因子
func timeFactor(lastAccess int64, now time.Time) float64 {
	days := now.Sub(time.Unix(lastAccess, 0)).Hours() / 24
	if days <= timeNodeDays {
		return 1.0
	}
	return math.Exp(-decayLambda * (days - timeNodeDays))
}

// archiveIdleMemories 自动归档超过30天未访问的记忆
func archiveIdleMemories(now time.Time) {
	threshold := now.Add(-archiveDays * 24 * time.Hour).Unix()
	DB.Model(&Memory{}).
		Where("archived = ? AND last_access < ?", false, threshold).
		Update("archived", true)
}

// joinMatchTerms 把搜索关键词拼成 FTS5 MATCH 查询串：每个词按 FTS5 语法加双引号包裹
// （词内双引号写成两个），以 OR 连接。加引号后含 `.`/`-`/`/` 等标点的词按短语解析，
// 不再触发 "fts5: syntax error near '.'" 之类错误。
func joinMatchTerms(keywords []string) string {
	quoted := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		quoted = append(quoted, `"`+strings.ReplaceAll(kw, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}

// searchMemories 执行记忆检索：FTS5 优先，LIKE 兜底；结果按相关度×时间衰减稳定排序。
func searchMemories(query string, now time.Time) ([]resultItem, error) {
	keywords := strings.Fields(query)
	if len(keywords) == 0 {
		return []resultItem{}, nil
	}
	archiveIdleMemories(now)

	var results []resultItem

	// ---- 第一层：FTS5 检索 ----
	// 关键词必须按 FTS5 语法转义后加引号包裹：未加引号的裸词中，`.`/`-`/`/` 等标点
	// 会被 FTS5 当作语法字符（如 `config.yaml` 解析成列引用 `config.yaml`）而报
	// "fts5: syntax error"。加引号后按短语（phrase）解析，标点仅作分词边界、不再报错。
	matchQuery := joinMatchTerms(keywords)
	type ftsRow struct {
		ID          int64
		Content     string
		CreatedAt   int64
		LastAccess  int64
		AccessCount int
		RelScore    float64
	}
	var ftsRows []ftsRow
	err := DB.Raw(`
        SELECT m.id, m.content, m.created_at, m.last_access, m.access_count,
               -bm25(memories_fts) AS rel_score
        FROM memories_fts
        JOIN memories m ON m.id = memories_fts.rowid
        WHERE memories_fts MATCH ? AND m.archived = 0
        ORDER BY rel_score DESC
        LIMIT 50`, matchQuery).Scan(&ftsRows).Error
	if err != nil {
		// FTS 查询失败（语法/分词问题等）：不使工具整体失败，降级到 LIKE 兜底。
		// 真正需要保留的错误（如 memories 表缺失）会由下方 LIKE 查询再次报出。
		ftsRows = nil
	}

	if len(ftsRows) > 0 {
		// FTS 有结果，计算时间衰减并组合
		for _, row := range ftsRows {
			mem := Memory{
				ID:          row.ID,
				Content:     row.Content,
				CreatedAt:   row.CreatedAt,
				LastAccess:  row.LastAccess,
				AccessCount: row.AccessCount,
				Archived:    false,
			}
			score := row.RelScore * timeFactor(row.LastAccess, now)
			results = append(results, resultItem{Memory: mem, Score: score})
		}
	} else {
		// ---- 第二层：LIKE 模糊匹配兜底 ----
		likeConditions := make([]string, len(keywords))
		likeArgs := make([]interface{}, len(keywords))
		for i, kw := range keywords {
			likeConditions[i] = "content LIKE ?"
			likeArgs[i] = "%" + kw + "%"
		}
		likeWhere := strings.Join(likeConditions, " OR ")

		var memories []Memory
		err := DB.Model(&Memory{}).
			Where("archived = ?", false).
			Where(likeWhere, likeArgs...).
			Limit(50).
			Find(&memories).Error
		if err != nil {
			return nil, err
		}

		for _, mem := range memories {
			score := timeFactor(mem.LastAccess, now) // 相关性固定为1
			results = append(results, resultItem{Memory: mem, Score: score})
		}
	}

	// 稳定排序：分数降序，相同分数按 ID 升序
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})
	return results, nil
}

// handleSearch 工具处理器：按 query/keywords 检索记忆，返回 JSON 结果。
func handleSearch(ctx context.Context, args json.RawMessage) (string, error) {
	var req struct {
		Query    string `json:"query"`
		Keywords string `json:"keywords"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	kwStr := req.Keywords
	if kwStr == "" {
		kwStr = req.Query
	}
	now := time.Now()
	results, err := searchMemories(kwStr, now)
	if err != nil {
		return "", err
	}

	// 更新访问信息
	for _, item := range results {
		DB.Model(&Memory{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
			"last_access":  now.Unix(),
			"access_count": gorm.Expr("access_count + 1"),
		})
	}

	// 记录调用日志
	DB.Create(&CallLog{
		Query:       req.Query,
		Keywords:    kwStr,
		ResultCount: len(results),
		CreatedAt:   now.Unix(),
	})

	out, err := json.Marshal(map[string]interface{}{
		"results": results,
		"total":   len(results),
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// addMemory 写入一条记忆；content 相同则视为重复，跳过插入并返回重复标记。
func addMemory(content, source string) (id int64, dedup bool, err error) {
	if source == "" {
		source = "user"
	}
	var existing Memory
	if err := DB.Where("content = ?", content).First(&existing).Error; err == nil {
		return existing.ID, true, nil // 已存在相同记忆，去重
	}
	now := time.Now().Unix()
	mem := Memory{
		Content:     content,
		CreatedAt:   now,
		LastAccess:  now,
		AccessCount: 0,
		Archived:    false,
		Source:      source,
	}
	if err := DB.Create(&mem).Error; err != nil {
		return 0, false, err
	}
	return mem.ID, false, nil
}

// handleAdd 工具处理器：把一条记忆写入记忆库，返回 JSON 结果。
func handleAdd(ctx context.Context, args json.RawMessage) (string, error) {
	var req struct {
		Content string `json:"content"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if req.Content == "" {
		return "", fmt.Errorf("content is required")
	}
	id, dedup, err := addMemory(req.Content, req.Source)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(map[string]interface{}{
		"id":    id,
		"dedup": dedup,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// memorySearchView 构造 memory_search 结果的结构化视图（表格）。
func memorySearchView(result string) (json.RawMessage, error) {
	var out struct {
		Results []struct {
			ID      int64   `json:"id"`
			Content string  `json:"content"`
			Source  string  `json:"source"`
			Score   float64 `json:"score"`
		} `json:"results"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		return nil, nil
	}
	rows := make([]dsc.ViewRow, 0, len(out.Results))
	for _, r := range out.Results {
		rows = append(rows, dsc.ViewRow{
			"id":      strconv.FormatInt(r.ID, 10),
			"content": r.Content,
			"score":   strconv.FormatFloat(r.Score, 'f', 2, 64),
			"source":  r.Source,
		})
	}
	return dsc.TableView("Memory", &dsc.ViewBadge{Text: fmt.Sprintf("%d hit(s)", out.Total), Tone: "teal"}, []dsc.ViewColumn{
		{Key: "id", Title: "id"},
		{Key: "content", Title: "content"},
		{Key: "score", Title: "score", Tone: "green"},
		{Key: "source", Title: "source"},
	}, rows), nil
}

// memoryAddView 构造 memory_add 结果的结构化视图（卡片）。
func memoryAddView(result string) (json.RawMessage, error) {
	var out struct {
		ID    int64 `json:"id"`
		Dedup bool  `json:"dedup"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		return nil, nil
	}
	badge := &dsc.ViewBadge{Text: "saved", Tone: "green"}
	status := "saved"
	if out.Dedup {
		badge = &dsc.ViewBadge{Text: "duplicate", Tone: "yellow"}
		status = "skipped (duplicate content)"
	}
	return dsc.CardView("Memory", badge, []dsc.ViewField{
		{Key: "id", Value: strconv.FormatInt(out.ID, 10)},
		{Key: "status", Value: status, Tone: badge.Tone},
	}), nil
}

// recordToolResult 是 AfterTool 钩子：把其他工具的成功执行结果自动写入记忆库
// （源标记为 tool），使记忆库随工具活动自动积累；跳过记忆服务自身的工具避免回环。
func recordToolResult(toolName, result, toolErr string) {
	if toolErr != "" || result == "" {
		return
	}
	if strings.HasPrefix(toolName, "memory_") {
		return
	}
	content := toolName + ": " + result
	if len(content) > maxAutoMemoryLen {
		content = content[:maxAutoMemoryLen]
	}
	if _, _, err := addMemory(content, "tool"); err != nil {
		fmt.Fprintf(os.Stderr, "memory-service: record tool result failed: %v\n", err)
	}
}

func main() {
	if err := initDB(dbPath()); err != nil {
		fmt.Fprintf(os.Stderr, "memory-service: init db failed: %v\n", err)
		os.Exit(2)
	}

	searchSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "自然语言查询语句"},
			"keywords": {"type": "string", "description": "空格分隔的搜索关键词，优先于 query；缺省时取 query"}
		},
		"description": "query 与 keywords 至少提供一个"
	}`)
	addSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"content": {"type": "string", "description": "要保存的记忆内容"},
			"source": {"type": "string", "description": "记忆来源标记，缺省 user"}
		},
		"required": ["content"]
	}`)

	sdk := dsc.New(dsc.Config{Name: "memory-service", Version: "1.0.0", Type: dsc.TypeTool})
	sdk.Tool(dsc.Tool{
		Name:        "memory_search",
		Description: "搜索记忆库：按关键词检索历史记忆（用户偏好、项目约定、工具执行结果等），返回按相关度与时间衰减排序的结果。参数 query 或 keywords 至少提供一个。",
		Schema:      searchSchema,
		Handler:     handleSearch,
		Context:     "记忆服务：可用 memory_search 检索历史记忆、memory_add 保存值得长期保留的信息（用户偏好、项目约定、重要结论）。其他工具的执行结果会自动写入记忆库，无需手动重复添加。",
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return memorySearchView(result)
		},
	})
	sdk.Tool(dsc.Tool{
		Name:        "memory_add",
		Description: "添加一条记忆到记忆库：把值得长期保留的信息（如用户偏好、项目约定、重要结论）写入记忆库，供后续 memory_search 检索。参数 content 为记忆内容，source 可选（默认 user）。",
		Schema:      addSchema,
		Handler:     handleAdd,
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return memoryAddView(result)
		},
	})
	// 钩子：工具执行成功后自动沉淀为记忆（原 HTTP 实现没有钩子，SDK 化时补齐，
	// 使记忆库能随工具活动自动积累，而非仅依赖模型显式调用 memory_add）。
	sdk.Hook(dsc.Hook{
		AfterTool: func(ctx context.Context, toolName, argumentsJSON, result, toolErr string) (string, string) {
			recordToolResult(toolName, result, toolErr)
			return result, toolErr
		},
	})
	sdk.Serve()
}

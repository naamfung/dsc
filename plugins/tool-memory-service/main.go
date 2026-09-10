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

	dsc "dsc-sdk"
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
	// defaultPageSize 列表查询的默认每页条数
	defaultPageSize = 20
	// maxPageSize 列表查询的最大每页条数
	maxPageSize = 100
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
func dbPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, "memory", "memory.db")
}

// initDB 初始化数据库：常规表自动迁移 + FTS5 虚拟表与同步触发器。
func initDB(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var err error
	DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return err
	}

	if err := DB.AutoMigrate(&Memory{}, &CallLog{}); err != nil {
		return err
	}

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

// joinMatchTerms 把搜索关键词拼成 FTS5 MATCH 查询串
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
		ftsRows = nil
	}

	if len(ftsRows) > 0 {
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
			score := timeFactor(mem.LastAccess, now)
			results = append(results, resultItem{Memory: mem, Score: score})
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})
	return results, nil
}

// ---- 查 (memory_search) ----

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

	for _, item := range results {
		DB.Model(&Memory{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
			"last_access":  now.Unix(),
			"access_count": gorm.Expr("access_count + 1"),
		})
	}

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

// ---- 增 (memory_add) ----

func addMemory(content, source string) (id int64, dedup bool, err error) {
	if source == "" {
		source = "user"
	}
	var existing Memory
	if err := DB.Where("content = ?", content).First(&existing).Error; err == nil {
		return existing.ID, true, nil
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

// ---- 删 (memory_delete) ----

func handleDelete(ctx context.Context, args json.RawMessage) (string, error) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if req.ID == 0 {
		return "", fmt.Errorf("id is required")
	}
	result := DB.Delete(&Memory{}, req.ID)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", fmt.Errorf("memory id %d not found", req.ID)
	}
	out, _ := json.Marshal(map[string]interface{}{
		"ok":      true,
		"id":      req.ID,
		"deleted": result.RowsAffected,
	})
	return string(out), nil
}

// ---- 改 (memory_update) ----

func handleUpdate(ctx context.Context, args json.RawMessage) (string, error) {
	var req struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if req.ID == 0 {
		return "", fmt.Errorf("id is required")
	}

	updates := map[string]interface{}{}
	if req.Content != "" {
		updates["content"] = req.Content
	}
	if req.Source != "" {
		updates["source"] = req.Source
	}
	if len(updates) == 0 {
		return "", fmt.Errorf("at least one of content or source must be provided")
	}
	updates["last_access"] = time.Now().Unix()

	result := DB.Model(&Memory{}).Where("id = ?", req.ID).Updates(updates)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", fmt.Errorf("memory id %d not found", req.ID)
	}

	var updated Memory
	DB.First(&updated, req.ID)
	out, _ := json.Marshal(map[string]interface{}{
		"ok":      true,
		"id":      req.ID,
		"updated": result.RowsAffected,
		"memory":  updated,
	})
	return string(out), nil
}

// ---- 查列表 (memory_list) ----

func handleList(ctx context.Context, args json.RawMessage) (string, error) {
	var req struct {
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
		Source   string `json:"source"`
		Archived *bool  `json:"archived"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = defaultPageSize
	}
	if req.PageSize > maxPageSize {
		req.PageSize = maxPageSize
	}

	query := DB.Model(&Memory{})
	if req.Source != "" {
		query = query.Where("source = ?", req.Source)
	}
	if req.Archived != nil {
		query = query.Where("archived = ?", *req.Archived)
	} else {
		query = query.Where("archived = ?", false)
	}

	var total int64
	query.Count(&total)

	var memories []Memory
	query.Order("created_at DESC").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Find(&memories)

	out, _ := json.Marshal(map[string]interface{}{
		"memories":  memories,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
	return string(out), nil
}

// ---- 视图 ----

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

func memoryDeleteView(result string) (json.RawMessage, error) {
	var out struct {
		OK      bool  `json:"ok"`
		ID      int64 `json:"id"`
		Deleted int64 `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		return nil, nil
	}
	return dsc.CardView("MemoryDelete", &dsc.ViewBadge{Text: "deleted", Tone: "yellow"}, []dsc.ViewField{
		{Key: "id", Value: strconv.FormatInt(out.ID, 10)},
		{Key: "rows", Value: strconv.FormatInt(out.Deleted, 10)},
	}), nil
}

func memoryUpdateView(result string) (json.RawMessage, error) {
	var out struct {
		OK     bool  `json:"ok"`
		ID     int64 `json:"id"`
		Memory struct {
			Content string `json:"content"`
			Source  string `json:"source"`
		} `json:"memory"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		return nil, nil
	}
	return dsc.CardView("MemoryUpdate", &dsc.ViewBadge{Text: "updated", Tone: "green"}, []dsc.ViewField{
		{Key: "id", Value: strconv.FormatInt(out.ID, 10)},
		{Key: "content", Value: out.Memory.Content},
		{Key: "source", Value: out.Memory.Source},
	}), nil
}

func memoryListView(result string) (json.RawMessage, error) {
	var out struct {
		Memories []struct {
			ID      int64  `json:"id"`
			Content string `json:"content"`
			Source  string `json:"source"`
		} `json:"memories"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		return nil, nil
	}
	rows := make([]dsc.ViewRow, 0, len(out.Memories))
	for _, m := range out.Memories {
		rows = append(rows, dsc.ViewRow{
			"id":      strconv.FormatInt(m.ID, 10),
			"content": m.Content,
			"source":  m.Source,
		})
	}
	return dsc.TableView("Memory", &dsc.ViewBadge{Text: fmt.Sprintf("%d items", out.Total), Tone: "teal"}, []dsc.ViewColumn{
		{Key: "id", Title: "id"},
		{Key: "content", Title: "content"},
		{Key: "source", Title: "source"},
	}, rows), nil
}

// recordToolResult 是 AfterTool 钩子：把其他工具的成功执行结果自动写入记忆库
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
	deleteSchema := json.RawMessage(`{
                "type": "object",
                "properties": {
                        "id": {"type": "integer", "description": "要删除的记忆 ID"}
                },
                "required": ["id"]
        }`)
	updateSchema := json.RawMessage(`{
                "type": "object",
                "properties": {
                        "id": {"type": "integer", "description": "要修改的记忆 ID"},
                        "content": {"type": "string", "description": "新的记忆内容（可选，至少提供 content 或 source 之一）"},
                        "source": {"type": "string", "description": "新的来源标记（可选）"}
                },
                "required": ["id"]
        }`)
	listSchema := json.RawMessage(`{
                "type": "object",
                "properties": {
                        "page": {"type": "integer", "description": "页码，从 1 开始，默认 1"},
                        "page_size": {"type": "integer", "description": "每页条数，默认 20，最大 100"},
                        "source": {"type": "string", "description": "按来源筛选（可选）"},
                        "archived": {"type": "boolean", "description": "是否只看已归档记忆，默认 false"}
                }
        }`)

	sdk := dsc.New(dsc.Config{
		Name:    "memory-service",
		Version: "1.1.0",
		Type:    dsc.TypeTool,
		Provides: map[string]string{
			"memory": "true",
		},
	})

	// 查（搜索）
	sdk.Tool(dsc.Tool{
		Name:        "memory_search",
		Description: "搜索记忆库：按关键词检索历史记忆（用户偏好、项目约定、工具执行结果等），返回按相关度与时间衰减排序的结果。参数 query 或 keywords 至少提供一个。",
		Schema:      searchSchema,
		Handler:     handleSearch,
		// ContextFn 动态贡献 system prompt（每次 ListContext 求值，对齐 DSH
		// ctx.systemPrompt.section）：记忆库规模/容量等运行态变化时即时反映。
		ContextFn: func() string {
			return "记忆服务：可用 memory_search 检索、memory_add 新增、memory_update 修改、memory_delete 删除、memory_list 列表。其他工具的执行结果会自动写入记忆库。"
		},
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return memorySearchView(result)
		},
	})
	// 增
	sdk.Tool(dsc.Tool{
		Name:        "memory_add",
		Description: "添加一条记忆到记忆库：把值得长期保留的信息（如用户偏好、项目约定、重要结论）写入记忆库，供后续 memory_search 检索。",
		Schema:      addSchema,
		Handler:     handleAdd,
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return memoryAddView(result)
		},
	})
	// 删
	sdk.Tool(dsc.Tool{
		Name:        "memory_delete",
		Description: "按 ID 删除一条记忆。删除后不可恢复，FTS 索引同步清除。",
		Schema:      deleteSchema,
		Handler:     handleDelete,
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return memoryDeleteView(result)
		},
	})
	// 改
	sdk.Tool(dsc.Tool{
		Name:        "memory_update",
		Description: "按 ID 修改记忆内容或来源标记。至少提供 content 或 source 之一。修改后 last_access 自动刷新。",
		Schema:      updateSchema,
		Handler:     handleUpdate,
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return memoryUpdateView(result)
		},
	})
	// 查列表（分页）
	sdk.Tool(dsc.Tool{
		Name:        "memory_list",
		Description: "分页列出记忆库中的记忆，按创建时间降序排列。可按来源筛选、查看已归档记忆。",
		Schema:      listSchema,
		Handler:     handleList,
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return memoryListView(result)
		},
	})

	sdk.Hook(dsc.Hook{
		AfterTool: func(ctx context.Context, toolName, argumentsJSON, result, toolErr string) (string, string) {
			recordToolResult(toolName, result, toolErr)
			return result, toolErr
		},
	})
	sdk.Serve()
}

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// spillDefaultThreshold 外置阈值的默认字符数（对齐 DSH spill）。
const spillDefaultThreshold = 4000

// spillThreshold 返回工具结果外置阈值（字符数）：可用 DSC_SPILL_THRESHOLD 覆盖。
func spillThreshold() int {
	if s := os.Getenv("DSC_SPILL_THRESHOLD"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return spillDefaultThreshold
}

// Spill 超大工具输出外置（对齐 DSH spill 子系统）：工具流水线 post-execute
// 阶段把超过阈值的纯文本结果保存到本地，模型侧只看到「头尾预览 + 定位符 +
// 取回说明」，需要完整内容时用 read_spill 工具按定位符取回。节省上下文 token，
// 同时保留可读性。

// SpillStore 外置内容的本地存储（按递增 id 存为 <dir>/spill-<n>.txt）。
type SpillStore struct {
	dir  string
	mu   sync.Mutex
	next int
}

// NewSpillStore 创建/打开外置存储目录。
func NewSpillStore(dir string) (*SpillStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("spill store: mkdir %s: %w", dir, err)
	}
	s := &SpillStore{dir: dir, next: 1}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("spill store: read dir: %w", err)
	}
	for _, e := range entries {
		var n int
		if _, err := fmt.Sscanf(e.Name(), "spill-%d.txt", &n); err == nil && n >= s.next {
			s.next = n + 1
		}
	}
	return s, nil
}

// SaveText 保存文本并返回定位符（形如 spill:<id>）。
func (s *SpillStore) SaveText(content string) (string, error) {
	s.mu.Lock()
	id := s.next
	s.next++
	s.mu.Unlock()
	path := filepath.Join(s.dir, fmt.Sprintf("spill-%d.txt", id))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("spill store: write %s: %w", path, err)
	}
	return fmt.Sprintf("spill:%d", id), nil
}

// Read 按定位符读取外置内容；仅接受本存储生成的 locator（spill:<id>），
// 拒绝路径穿越。
func (s *SpillStore) Read(locator string) (string, error) {
	var id int
	if _, err := fmt.Sscanf(locator, "spill:%d", &id); err != nil {
		return "", fmt.Errorf("spill store: invalid locator %q", locator)
	}
	path := filepath.Join(s.dir, fmt.Sprintf("spill-%d.txt", id))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("spill store: read %s: %w", path, err)
	}
	return string(data), nil
}

// spillLargeResult 工具流水线 post-execute 策略：结果超过阈值（字符数）时
// 外置为头尾预览 + 定位符。保存失败时保留内联结果（尽力而为，不阻断执行）。
func spillLargeResult(store *SpillStore, threshold int) WaterfallListener {
	return func(ctx EventContext, next func(EventContext) error) error {
		inv, _ := ctx.Data.(*ToolInvocation)
		if err := next(ctx); err != nil {
			return err
		}
		if inv == nil || inv.Err != nil || len([]rune(inv.Result)) <= threshold {
			return nil
		}
		locator, err := store.SaveText(inv.Result)
		if err != nil {
			return nil // 保存失败尽力保留内联结果
		}
		inv.Result = spillPreview(inv.Result, locator, threshold/2)
		return nil
	}
}

// spillPreview 把完整结果替换为「头尾预览 + 定位符 + 取回说明」。
func spillPreview(content, locator string, head int) string {
	runes := []rune(content)
	if len(runes) <= head*2 {
		return content
	}
	headStr := string(runes[:head])
	tailStr := string(runes[len(runes)-head:])
	return fmt.Sprintf(
		"[内容已外置: %s]\n%s\n...(完整内容共 %d 字符，可用 read_spill 工具按定位符读取)\n%s",
		locator, headStr, len(runes), tailStr)
}

// readSpillTool 按定位符取回外置内容的工具（模型端读取完整结果）。
type readSpillTool struct{ store *SpillStore }

func (t *readSpillTool) Name() string { return "read_spill" }

func (t *readSpillTool) Description() string {
	return "Read the full content of a previously externalized (spilled) tool result by its locator. " +
		"This is NOT a generic file reader — it only reads content that this host previously " +
		"spilled to disk when a tool result exceeded the inline threshold (usually >4000 chars). " +
		"\n\n" +
		"When to use:\n" +
		"  - A tool result was replaced with `[内容已外置: spill:N]\\n<head>\\n...\\n<tail>` " +
		"and you need the full content (not just the head/tail preview).\n" +
		"  - Pass the locator string `spill:N` (e.g. `spill:3`) — NOT a file path.\n" +
		"\n" +
		"Parameters:\n" +
		"  - locator: the spill locator string of the form `spill:<id>` (e.g. `spill:3`). " +
		"Do NOT pass filesystem paths like `D:/foo/spill-3.txt` — only the `spill:N` locator " +
		"that was returned to you in a previous tool result preview.\n" +
		"\n" +
		"If the locator is invalid or the spill file has been cleaned up (host restart / " +
		"session expired), an error is returned. Re-run the original tool to regenerate the " +
		"result if needed."
}

func (t *readSpillTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
                "type": "object",
                "properties": {
                        "locator": {
                                "type": "string",
                                "description": "The spill locator string of the form 'spill:<id>' (e.g. 'spill:3'). Must be a locator previously returned by the host in a tool result preview — NOT a filesystem path.",
                                "pattern": "^spill:[0-9]+$"
                        }
                },
                "required": ["locator"]
        }`)
}

func (t *readSpillTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Locator string `json:"locator"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("read_spill: invalid args: %w", err)
	}
	if p.Locator == "" {
		return "", fmt.Errorf("read_spill: locator is required (e.g. \"spill:3\")")
	}
	// 友好提示：模型若误把文件路径作为 locator，引导其改用 spill:N 格式。
	// 常见误用：把 spill 文件的真实磁盘路径（如 D:/.../spill-3.txt）当 locator 传入。
	if len(p.Locator) > 0 && !isSpillLocator(p.Locator) {
		return "", fmt.Errorf("read_spill: locator must be of form \"spill:<id>\" (e.g. \"spill:3\"); "+
			"got %q which looks like a filesystem path. read_spill only reads content previously "+
			"spilled by the host (returned in a tool result preview as \"[内容已外置: spill:N]\"); "+
			"it is NOT a generic file reader — use the `shell` tool with `cat` if you need to read "+
			"an arbitrary file from disk.", p.Locator)
	}
	content, err := t.store.Read(p.Locator)
	if err != nil {
		// 区分「locator 不存在」与「locator 合法但文件已清理」两种错误，便于模型自我纠正。
		return "", fmt.Errorf("read_spill: %w (locator %q may have been cleaned up after host restart "+
			"or session expiry; re-run the original tool to regenerate the spilled result)", err, p.Locator)
	}
	return content, nil
}

// isSpillLocator 报告 s 是否形如 "spill:<digits>"。
func isSpillLocator(s string) bool {
	var id int
	_, err := fmt.Sscanf(s, "spill:%d", &id)
	return err == nil && id > 0
}

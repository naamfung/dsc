package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// longTool 返回超长结果的测试工具。
type longTool struct{ content string }

func (t *longTool) Name() string                      { return "long-tool" }
func (t *longTool) Description() string               { return "returns long output" }
func (t *longTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *longTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return t.content, nil
}

func longText(n int) string {
	block := strings.Repeat("0123456789", 100) // 1000 chars
	return strings.Repeat(block, n/1000+1)[:n]
}

func newSpillManager(t *testing.T, store *SpillStore) *Manager {
	t.Helper()
	m := newRouterManager() // 无默认 retry 的事件总线
	m.events.OnWaterfall(EventToolPostExecute, spillLargeResult(store, 1000))
	return m
}

func TestSpillStoreSaveRead(t *testing.T) {
	store, err := NewSpillStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	content := longText(5000)
	loc, err := store.SaveText(content)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasPrefix(loc, "spill:") {
		t.Fatalf("locator = %q, want spill: prefix", loc)
	}
	got, err := store.Read(loc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != content {
		t.Fatalf("round-trip mismatch: len %d vs %d", len(got), len(content))
	}
}

func TestSpillReplacesLargeResult(t *testing.T) {
	store, _ := NewSpillStore(t.TempDir())
	m := newSpillManager(t, store)
	long := longText(3000)
	_ = m.toolRegistry.Register(&longTool{content: long})

	result, err := m.ExecuteTool(context.Background(), "long-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result, "[内容已外置: spill:") {
		t.Fatalf("large result should be spilled, got: %.80s", result)
	}
	if !strings.Contains(result, "read_spill") {
		t.Fatal("preview should mention read_spill")
	}
	if strings.Contains(result, long) {
		t.Fatal("full content should not remain inline")
	}
}

func TestSpillKeepsShortResult(t *testing.T) {
	store, _ := NewSpillStore(t.TempDir())
	m := newSpillManager(t, store)
	_ = m.toolRegistry.Register(&longTool{content: "short"})

	result, err := m.ExecuteTool(context.Background(), "long-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result != "short" {
		t.Fatalf("short result should be kept inline, got %q", result)
	}
}

func TestReadSpillTool(t *testing.T) {
	store, _ := NewSpillStore(t.TempDir())
	content := longText(2500)
	loc, _ := store.SaveText(content)
	tool := &readSpillTool{store: store}

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"locator":"`+loc+`"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got != content {
		t.Fatalf("read_spill returned wrong content (len %d vs %d)", len(got), len(content))
	}
}

func TestSpillRejectsInvalidLocator(t *testing.T) {
	store, _ := NewSpillStore(t.TempDir())
	tool := &readSpillTool{store: store}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"locator":"../../etc/passwd"}`)); err == nil {
		t.Fatal("path traversal locator should be rejected")
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"locator":"spill:99999"}`)); err == nil {
		t.Fatal("missing spill file should error")
	}
}

// TestSpillExemptsReadSpillTool 验证 read_spill 工具的结果不被二次 spill。
// 修复前的 bug：read_spill 返回大内容 → post-execute spill 流水线又把结果外置 →
// 模型 read_spill(spill:2) → 又返回大内容 → 又被外置 → 死循环，最终连接中断。
func TestSpillExemptsReadSpillTool(t *testing.T) {
	store, _ := NewSpillStore(t.TempDir())
	m := newSpillManager(t, store)

	// 用一个返回超长内容的 mock 工具模拟 read_spill 行为
	// （真正的 read_spill 在 NewManager 时已注册到 m.toolRegistry，使用的是
	// 生产 spill store；这里用独立 mock 工具 "mock_read_spill" 避免冲突）
	longContent := longText(5000)
	_ = m.toolRegistry.Register(&longTool{content: longContent})

	// 改名工具为 "read_spill" 来触发豁免逻辑——但 toolRegistry 不支持改名，
	// 故直接验证 spillLargeResult 监听器对 ToolName=="read_spill" 的豁免：
	// 用 ToolInvocation 直接调 events.Waterfall 模拟流水线
	inv := &ToolInvocation{
		ToolName: "read_spill",
		Result:   longContent,
	}
	err := m.events.Waterfall(EventToolPostExecute, EventContext{Data: inv}, func(EventContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("waterfall: %v", err)
	}
	// read_spill 豁免：结果应保持完整，不被替换为 spill 预览
	if inv.Result != longContent {
		t.Fatalf("read_spill result was spilled again (len %d, want %d); first 80 chars: %.80s",
			len(inv.Result), len(longContent), inv.Result)
	}
	if strings.Contains(inv.Result, "[内容已外置:") {
		t.Fatalf("read_spill result should not be spill-previewed, got: %.80s", inv.Result)
	}

	// 对照：非 read_spill 工具（如 "long-tool"）的超长结果应被 spill
	inv2 := &ToolInvocation{
		ToolName: "long-tool",
		Result:   longContent,
	}
	_ = m.events.Waterfall(EventToolPostExecute, EventContext{Data: inv2}, func(EventContext) error {
		return nil
	})
	if inv2.Result == longContent {
		t.Fatalf("non-read_spill tool should have its result spilled (still same length %d)", len(inv2.Result))
	}
	if !strings.Contains(inv2.Result, "[内容已外置:") {
		t.Fatalf("non-read_spill tool result should be spill-previewed, got: %.80s", inv2.Result)
	}
}

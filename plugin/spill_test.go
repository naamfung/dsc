package plugin

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

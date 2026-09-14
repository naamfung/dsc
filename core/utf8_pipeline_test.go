package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// dirtyOutputTool 返回含非法 UTF-8 字节的结果，用于验证宿主侧 executeToolBody
// 的结果净化防线（覆盖宿主内置工具与一切绕过 SDK 的路径）。
type dirtyOutputTool struct{ name string }

func (t *dirtyOutputTool) Name() string                      { return t.name }
func (t *dirtyOutputTool) Description() string               { return "returns invalid utf8" }
func (t *dirtyOutputTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *dirtyOutputTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "listing \xc4\xe3\xba\xc3 done", nil
}

// TestExecuteToolSanitizesInvalidUTF8Result 验证宿主侧结果净化：非法 UTF-8
// 退化为 U+FFFD 而非进入会话日志/LLM 请求（proto string 字段会拒绝 marshal，
// 曾致整个工具结果以 "grpc: error while marshaling" 失败告终）。
func TestExecuteToolSanitizesInvalidUTF8Result(t *testing.T) {
	m := newPipelineManager(t)
	if err := m.toolRegistry.Register(&dirtyOutputTool{name: "dirty-out"}); err != nil {
		t.Fatalf("register dirty tool: %v", err)
	}
	result, err := m.ExecuteTool(context.Background(), "dirty-out", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ExecuteTool 错误: %v", err)
	}
	if !utf8.ValidString(result) {
		t.Fatalf("结果必须为合法 UTF-8: %q", result)
	}
	if !strings.Contains(result, "\uFFFD") {
		t.Fatalf("非法字节应替换为 U+FFFD: %q", result)
	}
}

// TestSanitizeUTF8 合法串零开销原样返回；非法串净化且结果合法。
func TestSanitizeUTF8(t *testing.T) {
	const good = "普通 UTF-8 ✓"
	if got := sanitizeUTF8(good); got != good {
		t.Fatalf("合法串应原样返回: %q", got)
	}
	got := sanitizeUTF8("a\xffb")
	if !utf8.ValidString(got) || !strings.Contains(got, "\uFFFD") {
		t.Fatalf("非法串应净化: %q", got)
	}
}

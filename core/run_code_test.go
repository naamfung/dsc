package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dsc/proto"
)

// mulTool 测试用简单工具：读取 {a,b} 返回 {"product":a*b}。
type mulTool struct{}

func (mulTool) Name() string                      { return "mul" }
func (mulTool) Description() string               { return "multiply two ints" }
func (mulTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (mulTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct{ A, B int }
	_ = json.Unmarshal(args, &p)
	b, _ := json.Marshal(map[string]any{"product": p.A * p.B})
	return string(b), nil
}

func TestRunCodeToolCompletes(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})
	if err := m.toolRegistry.Register(mulTool{}); err != nil {
		t.Fatal(err)
	}
	tool, ok := m.toolRegistry.Get("run_code")
	if !ok {
		t.Fatal("run_code tool not registered")
	}

	// 通过生成 SDK 的同名函数调用 mul，组合成一步。
	args, _ := json.Marshal(map[string]any{
		"source": "return mul{a = 2, b = 3}.product",
	})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `"stop_reason": "completed"`) {
		t.Fatalf("output missing completed: %s", out)
	}
	var r struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if numericCV(r.Value) != 6 {
		t.Fatalf("value = %v, want 6; out=%s", r.Value, out)
	}
}

func TestRunCodeToolFailureAsData(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})
	if err := m.toolRegistry.Register(mulTool{}); err != nil {
		t.Fatal(err)
	}
	tool, _ := m.toolRegistry.Get("run_code")

	// 程序调用不存在的工具名 → 工具失败（ok=false 走渲染路径），不回硬错误。
	args, _ := json.Marshal(map[string]any{
		"source": "local ok, r = tool(\"nope\", {})\nreturn \"handled:\" .. tostring(r)",
	})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !strings.Contains(out, `"stop_reason": "completed"`) || !strings.Contains(out, "handled:") {
		t.Fatalf("expected failure handled as data, got: %s", out)
	}
}

// numericCV 归一数值（int64/float64）便于断言。
func numericCV(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return -1
	}
}

// viewTool 测试用带视图的宿主工具（实现 ViewExecutor），用于验证聚合层透传 ViewJson。
type viewTool struct{}

func (viewTool) Name() string                      { return "viewed" }
func (viewTool) Description() string               { return "tool with view" }
func (viewTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (viewTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return `{"ok":true}`, nil
}
func (viewTool) ExecuteWithView(_ context.Context, _ json.RawMessage) (string, string, error) {
	v, _ := json.Marshal(ToolView{
		Kind: "card", Title: "Viewed", Badge: &ViewBadge{Text: "ok", Tone: "green"},
		Fields: []ViewField{{Key: "state", Value: "ready"}},
	})
	return `{"ok":true}`, string(v), nil
}

// TestToolGRPCServerPropagatesViewJson 验证宿主聚合 Tool 服务把 ViewExecutor 工具
// 的视图透传到 ExecuteToolResponse.ViewJson（agent → ToolGRPCServer → TUI 链路关键点，
// 防止视图在聚合层被丢弃）。
func TestToolGRPCServerPropagatesViewJson(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})
	if err := m.toolRegistry.Register(viewTool{}); err != nil {
		t.Fatal(err)
	}
	srv := NewToolGRPCServer(m)
	resp, err := srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{
		ToolName: "viewed", ArgumentsJson: "{}",
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if resp.Error != "" || resp.Content != `{"ok":true}` {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.ViewJson == "" {
		t.Fatal("ViewJson 未在聚合层透传")
	}
	var v ToolView
	if err := json.Unmarshal([]byte(resp.ViewJson), &v); err != nil {
		t.Fatalf("ViewJson 非法: %v", err)
	}
	if v.Kind != "card" || v.Title != "Viewed" || v.Badge == nil || v.Badge.Text != "ok" || len(v.Fields) != 1 {
		t.Fatalf("view = %+v", v)
	}
}

// TestRunCodeView 校验 run_code 结果的结构化视图：stop_reason 语义着色徽标与正文。
func TestRunCodeView(t *testing.T) {
	cases := []struct {
		name     string
		result   string
		badge    string
		tone     string
		contains string
	}{
		{"completed", `{"value":{"n":6},"stop_reason":"completed","tool_calls":[{"name":"mul"}]}`, "completed", "green", `"n": 6`},
		{"error", `{"stop_reason":"error","error":"boom"}`, "error", "red", "boom"},
		{"cancelled", `{"stop_reason":"cancelled","error":"timed out"}`, "cancelled", "yellow", "timed out"},
		{"empty", `{"stop_reason":"completed"}`, "completed", "green", "no return value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := runCodeView(context.Background(), nil, c.result)
			if err != nil {
				t.Fatalf("runCodeView: %v", err)
			}
			var v ToolView
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				t.Fatalf("view 非法: %v", err)
			}
			if v.Kind != "plain" || v.Title != "RunCode" || v.Badge == nil || v.Badge.Text != c.badge || v.Badge.Tone != c.tone {
				t.Fatalf("view = %+v", v)
			}
			if !strings.Contains(v.Body, c.contains) {
				t.Fatalf("body = %q, want contains %q", v.Body, c.contains)
			}
		})
	}
}

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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

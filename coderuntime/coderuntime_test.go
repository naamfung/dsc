package coderuntime

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeTool 测试用 ToolCaller：mul 返回 JSON 数值，err 返回错误。
func fakeTool() ToolCaller {
	return func(ctx context.Context, name, argsJSON string) (string, error) {
		switch name {
		case "mul":
			var a struct{ A, B int }
			_ = json.Unmarshal([]byte(argsJSON), &a)
			b, _ := json.Marshal(map[string]any{"product": a.A * a.B})
			return string(b), nil
		case "err":
			return "", errorString("boom")
		}
		return "", nil // 不会到达
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestRunBasic(t *testing.T) {
	r := Run(context.Background(), Options{
		Script:   `return args.a + args.b`,
		Bindings: map[string]any{"args": map[string]any{"a": 2, "b": 3}},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, want completed (error=%q)", r.StopReason, r.Error)
	}
	if numeric(r.Value) != 5 {
		t.Fatalf("value = %v (%T), want 5", r.Value, r.Value)
	}
}

func TestRunToolAwaitChain(t *testing.T) {
	r := Run(context.Background(), Options{
		Script: `
log("start")
local ok1, r1 = tool("mul", {a = 2, b = 3})
local ok2, r2 = tool("mul", {a = 4, b = 5})
log("done")
return { first = r1.product, second = r2.product }`,
		Tool: fakeTool(),
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	v := r.Value.(map[string]any)
	if numeric(v["first"]) != 6 || numeric(v["second"]) != 20 {
		t.Fatalf("value = %v", r.Value)
	}
	if len(r.Logs) != 2 || r.Logs[0] != "start" || r.Logs[1] != "done" {
		t.Fatalf("logs = %+v", r.Logs)
	}
}

func TestRunToolResultIsJSON(t *testing.T) {
	// mul 返回 {"product":<n>}，应被解析为 Lua 表而非字符串。
	r := Run(context.Background(), Options{
		Script: `local ok, r = tool("mul", {a = 3, b = 4}); return { ok = ok, product = r.product }`,
		Tool:   fakeTool(),
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	v := r.Value.(map[string]any)
	if v["ok"] != true {
		t.Fatalf("ok = %v", v["ok"])
	}
	if numeric(v["product"]) != 12 {
		t.Fatalf("product = %v, want 12", v["product"])
	}
	// 工具调用快照：参数为 JSON、无错误。
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "mul" || r.ToolCalls[0].Error != "" {
		t.Fatalf("tool_calls = %+v", r.ToolCalls)
	}
}

func TestRunToolFailureBranches(t *testing.T) {
	// 工具失败以 (false, errMsg) 返回，程序可据此分支而不抛异常。
	r := Run(context.Background(), Options{
		Script: `
local ok, r = tool("err", {})
if ok then return "unexpected" end
return "handled:" .. r`,
		Tool: fakeTool(),
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	if r.Value != "handled:boom" {
		t.Fatalf("value = %v", r.Value)
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Error != "boom" {
		t.Fatalf("tool_calls = %+v", r.ToolCalls)
	}
}

func TestRunLogs(t *testing.T) {
	r := Run(context.Background(), Options{Script: `log("a"); log("b"); return 1`})
	if len(r.Logs) != 2 || r.Logs[0] != "a" || r.Logs[1] != "b" {
		t.Fatalf("logs = %+v", r.Logs)
	}
}

func TestRunTimeoutCancelsCPU(t *testing.T) {
	// CPU 死循环：mainLoopWithContext 依据 ctx 截断 → cancelled。
	r := Run(context.Background(), Options{
		Script:  `while true do end`,
		Timeout: 50 * time.Millisecond,
	})
	if r.StopReason != StopCancelled {
		t.Fatalf("stop_reason = %q, want cancelled (error=%q)", r.StopReason, r.Error)
	}
}

func TestRunUnavailableTool(t *testing.T) {
	r := Run(context.Background(), Options{Script: `local ok, r = tool("x", {}); return ok`})
	if r.StopReason != StopError {
		t.Fatalf("stop_reason = %q, want error", r.StopReason)
	}
}

func TestRunScriptRuntimeError(t *testing.T) {
	r := Run(context.Background(), Options{Script: `error("kaboom")`})
	if r.StopReason != StopError {
		t.Fatalf("stop_reason = %q, want error", r.StopReason)
	}
	if r.Error == "" {
		t.Fatalf("error is empty")
	}
}

func TestRunEmptyScript(t *testing.T) {
	r := Run(context.Background(), Options{Script: "  "})
	if r.StopReason != StopError {
		t.Fatalf("stop_reason = %q, want error", r.StopReason)
	}
}

// TestRunToolTailPosition 验证 tail 位置直接 `return tool(...)` 也能正常 yield →
// resume（go-lua 尾调用 yield 修复后）。tool 返回两值，因此结果按多值物化。
func TestRunToolTailPosition(t *testing.T) {
	r := Run(context.Background(), Options{
		Script: `return tool("mul", {a = 3, b = 4})`,
		Tool:   fakeTool(),
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	// 两值返回 (ok, value)：被物化为 []any。
	arr, ok := r.Value.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("value = %v (%T), want 2-value result", r.Value, r.Value)
	}
	if arr[0] != true {
		t.Fatalf("ok = %v, want true", arr[0])
	}
	if numeric(arr[1].(map[string]any)["product"]) != 12 {
		t.Fatalf("product value = %v, want 12", arr[1])
	}
}

// numeric 把 int64/float64 归一到 float64，便于跨表示比较。
func numeric(v any) float64 {
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

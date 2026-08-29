package coderuntime

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateSDK(t *testing.T) {
	out := GenerateSDK([]ToolSpec{
		{Name: "read_file", Description: "Read a file.\nMulti-line."},
		{Name: "write_file", Description: "Write a file"},
		{Name: "tool-with-dash", Description: "Not a valid Lua ident"},
	})
	if !strings.Contains(out, "function read_file(args)") {
		t.Fatalf("missing read_file wrapper:\n%s", out)
	}
	if !strings.Contains(out, `tool("read_file", args or {})`) {
		t.Fatalf("missing tool() forward:\n%s", out)
	}
	if !strings.Contains(out, "function write_file(args)") {
		t.Fatalf("missing write_file wrapper:\n%s", out)
	}
	// 含连字符的工具名不生成同名函数（模型仍可用 tool("tool-with-dash", ...)）。
	if strings.Contains(out, "function tool-with-dash(") {
		t.Fatalf("should not emit a bare function for non-identifier name:\n%s", out)
	}
	// 多行描述压成单行，且不嵌 `--` 破坏注释。
	if strings.Contains(out, "Read a file.\nMulti-line.") {
		t.Fatalf("description should be single-line:\n%s", out)
	}
}

func TestRunWithGeneratedSDK(t *testing.T) {
	// 生成的 SDK 拼接用户脚本，验证「同名函数 → tool() → 宿主工具」整条通路。
	sdk := GenerateSDK([]ToolSpec{{Name: "mul", Description: "multiply"}})
	r := Run(context.Background(), Options{
		Script: sdk + "\nreturn mul{a = 2, b = 3}.product",
		Tool:   fakeTool(),
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	if numeric(r.Value) != 6 {
		t.Fatalf("value = %v, want 6", r.Value)
	}
}

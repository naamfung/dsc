package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dsc/core"
)

// TestShellView 校验 shell 工具的结构化视图：退出码徽标（0 绿 / 非 0 红）
// + 输出正文（剥离追加的 [exit_code: N] 标记）。
func TestShellView(t *testing.T) {
	cases := []struct {
		name     string
		result   string
		badge    string
		tone     string
		contains string
	}{
		{"zero-with-output", "hello\nworld", "exit 0", "green", "hello"},
		{"zero-no-output", "\n[exit_code: 0]\n", "exit 0", "green", "(no output)"},
		{"nonzero", "boom\n[exit_code: 2]\n", "exit 2", "red", "boom"},
		{"negative", "killed\n[exit_code: -1]\n", "exit -1", "red", "killed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := shellView(context.Background(), nil, c.result)
			if err != nil {
				t.Fatalf("shellView: %v", err)
			}
			var v core.ToolView
			if err := json.Unmarshal(out, &v); err != nil {
				t.Fatalf("view 非法: %v", err)
			}
			if v.Kind != "plain" || v.Title != "Shell" || v.Badge == nil || v.Badge.Text != c.badge || v.Badge.Tone != c.tone {
				t.Fatalf("view = %+v", v)
			}
			if !strings.Contains(v.Body, c.contains) {
				t.Fatalf("body = %q, want contains %q", v.Body, c.contains)
			}
			// 视图正文不得残留退出码标记（已移到徽标）
			if c.badge != "exit 0" && strings.Contains(v.Body, "[exit_code:") {
				t.Fatalf("body 残留退出码标记: %q", v.Body)
			}
		})
	}
}

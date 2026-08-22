package main

import (
	"strings"
	"testing"
)

func TestCanonicalArgs(t *testing.T) {
	// 仅属性顺序不同 → 相同
	a := canonicalArgs(`{"b":1,"a":{"d":2,"c":3}}`)
	b := canonicalArgs(`{"a":{"c":3,"d":2},"b":1}`)
	if a != b {
		t.Fatalf("key order should not matter: %q vs %q", a, b)
	}
	if !strings.Contains(a, `"a":{"c":3,"d":2}`) {
		t.Fatalf("canonical = %q", a)
	}
	// 数组顺序保留（有意义）
	if canonicalArgs(`[1,2]`) == canonicalArgs(`[2,1]`) {
		t.Fatal("array order must be preserved")
	}
	// 非法 JSON 原样返回
	if canonicalArgs(`{bad`) != `{bad` {
		t.Fatal("invalid JSON should pass through")
	}
}

func TestRepeatGuardTrack(t *testing.T) {
	a := newTestAgent(t)
	a.repeatThresholds = []int{3, 5, 8}
	a.repeatExclude = []string{"todo_write"}

	// 前两次无提醒
	if r := a.repeatGuardTrack("shell", `{"cmd":"ls"}`); r != "" {
		t.Fatalf("first call should not remind, got %q", r)
	}
	if r := a.repeatGuardTrack("shell", `{"cmd":"ls"}`); r != "" {
		t.Fatalf("second call should not remind, got %q", r)
	}
	// 第三次 → 简短提醒
	r := a.repeatGuardTrack("shell", `{"cmd":"ls"}`)
	if r == "" || !strings.Contains(r, "You are repeating the exact same tool call") {
		t.Fatalf("third call should remind briefly, got %q", r)
	}
	// 第四次无；第五次 → 详细提醒（含工具/次数/参数）
	if r := a.repeatGuardTrack("shell", `{"cmd":"ls"}`); r != "" {
		t.Fatalf("fourth call should not remind, got %q", r)
	}
	r = a.repeatGuardTrack("shell", `{"cmd":"ls"}`)
	if r == "" || !strings.Contains(r, "Repeated tool call detected:") ||
		!strings.Contains(r, "- tool: shell") || !strings.Contains(r, "consecutive_calls: 5") ||
		!strings.Contains(r, `- arguments: {"cmd":"ls"}`) {
		t.Fatalf("fifth call should remind in detail, got %q", r)
	}

	// 排除工具对链透明：穿插 todo_write 不重置链
	if r := a.repeatGuardTrack("todo_write", `{"todos":[]}`); r != "" {
		t.Fatalf("excluded tool should not remind, got %q", r)
	}
	// 继续 shell 相同调用 → 链仍连续（6）
	if r := a.repeatGuardTrack("shell", `{"cmd":"ls"}`); r != "" {
		t.Fatalf("chain should continue through excluded tool, got %q", r)
	}

	// 换工具/换参数 → 重置链为 1
	if r := a.repeatGuardTrack("shell", `{"cmd":"pwd"}`); r != "" {
		t.Fatalf("different args should reset chain, got %q", r)
	}
	if r := a.repeatGuardTrack("read_file", `{"path":"x"}`); r != "" {
		t.Fatalf("different tool should reset chain, got %q", r)
	}
	// 新链达第 3 次 → 再次简短提醒
	a.repeatGuardTrack("read_file", `{"path":"x"}`)
	r = a.repeatGuardTrack("read_file", `{"path":"x"}`)
	if r == "" || !strings.Contains(r, "You are repeating") {
		t.Fatalf("new chain third call should remind, got %q", r)
	}
}

func TestRepeatGuardExcludeWildcard(t *testing.T) {
	a := newTestAgent(t)
	a.repeatThresholds = []int{3}
	a.repeatExclude = []string{"tool_*"}
	for _, name := range []string{"tool_foo", "tool_bar"} {
		if !a.repeatToolExcluded(name) {
			t.Fatalf("%s should be excluded by tool_*", name)
		}
	}
	if a.repeatToolExcluded("shell") {
		t.Fatal("shell should not be excluded")
	}
}

func TestRepeatReminderPreview(t *testing.T) {
	long := strings.Repeat("x", 600)
	r := repeatReminderText("write", 5, `{"content":"`+long+`"}`, true)
	if !strings.Contains(r, "… (+") || !strings.Contains(r, "more chars)") {
		t.Fatalf("long args should be truncated with a marker, got %q", r)
	}
	// 截断后整体远小于未截断（未截断 ≈ 916 字符）
	if len(r) > 850 {
		t.Fatalf("reminder should stay bounded, got %d", len(r))
	}
}

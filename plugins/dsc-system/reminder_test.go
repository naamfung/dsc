package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"dsc/proto"
)

// 行为套件（对齐 DSH repeat-tool-reminder spec）：链语义（相同/异调用重置/
// 排除透明/跨会话隔离/用户插话重置）、阈值升级（含 thresholds[0] 简短档键定）、
// 规范化、参数预览上限、被拒调用计数、fail-loud 配置校验。

func newTestServer(t *testing.T, env map[string]string) *reminderServer {
	t.Helper()
	for _, k := range []string{envRepeatThresholds, envRepeatInclude, envRepeatExclude, envRepeatPreview} {
		t.Setenv(k, "") // 隔离宿主环境泄漏
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	s, err := newReminderServer()
	if err != nil {
		t.Fatalf("newReminderServer: %v", err)
	}
	return s
}

// call 以 post-execute 事件驱动一次观察，返回 notice（空 = 未达阈值）。
func call(t *testing.T, s *reminderServer, session, tool, argsJSON string) string {
	t.Helper()
	dec, err := s.OnEvent(context.Background(), &proto.PolicyEvent{
		Kind: "tool/post-execute", Tool: tool, ArgumentsJson: argsJSON, Session: session,
	})
	if err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	return dec.GetNotice()
}

func TestThresholdEscalationGentleThenDetailed(t *testing.T) {
	s := newTestServer(t, nil)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	third := call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	if !strings.Contains(third, "repeating the exact same tool call") {
		t.Fatalf("3rd call should be gentle, got %q", third)
	}
	if n := call(t, s, "s1", "shell", `{"cmd":"ls"}`); n != "" {
		t.Fatalf("4th call must be silent (between thresholds), got %q", n)
	}
	fifth := call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	if !strings.Contains(fifth, "consecutive_calls: 5") || !strings.Contains(fifth, "- tool: shell") {
		t.Fatalf("5th call should be detailed with count 5, got %q", fifth)
	}
}

func TestGentleTextKeyedToFirstThreshold(t *testing.T) {
	s := newTestServer(t, map[string]string{envRepeatThresholds: "4,2"}) // 乱序输入：归一为 [2,4]
	call(t, s, "s1", "shell", `{}`)
	second := call(t, s, "s1", "shell", `{}`)
	if !strings.Contains(second, "repeating the exact same tool call") {
		t.Fatalf("thresholds[0]=2 should be gentle, got %q", second)
	}
	call(t, s, "s1", "shell", `{}`)
	fourth := call(t, s, "s1", "shell", `{}`)
	if !strings.Contains(fourth, "consecutive_calls: 4") {
		t.Fatalf("thresholds[1]=4 should be detailed at 4, got %q", fourth)
	}
}

func TestDifferentTrackedCallResetsChain(t *testing.T) {
	s := newTestServer(t, nil)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	call(t, s, "s1", "other", `{}`) // 已跟踪异调用 → 重置
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	if n := call(t, s, "s1", "shell", `{"cmd":"ls"}`); !strings.Contains(n, "repeating") {
		t.Fatalf("3rd consecutive shell after reset should be gentle, got %q", n)
	}
}

func TestExcludedToolsTransparent(t *testing.T) {
	s := newTestServer(t, nil) // 缺省排除 todo_write
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	call(t, s, "s1", "todo_write", `{"todos":[]}`) // 排除：不计数也不重置
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	if n := call(t, s, "s1", "shell", `{"cmd":"ls"}`); !strings.Contains(n, "repeating") {
		t.Fatalf("excluded interleave must not mask the chain, got %q", n)
	}
	if n := call(t, s, "s1", "todo_write", `{"todos":[]}`); n != "" {
		t.Fatalf("excluded tool must never be counted, got %q", n)
	}
}

func TestIncludePatternsTrackOnlyMatching(t *testing.T) {
	s := newTestServer(t, map[string]string{envRepeatInclude: "pro*"})
	for i := 0; i < 3; i++ {
		call(t, s, "s1", "other", `{}`) // 未命中 include：透明
	}
	call(t, s, "s1", "probe", `{}`)
	call(t, s, "s1", "probe", `{}`)
	if n := call(t, s, "s1", "probe", `{}`); !strings.Contains(n, "repeating") {
		t.Fatalf("tracked tool should escalate, got %q", n)
	}
}

func TestWildcardEscapesRegexMetachars(t *testing.T) {
	s := newTestServer(t, map[string]string{envRepeatExclude: "pr.be"}) // 应按字面点匹配，不得命中 probe
	for i := 0; i < 3; i++ {
		if n := call(t, s, "s1", "probe", `{}`); i == 2 && n == "" {
			t.Fatalf("probe must not be excluded by regex-ish pattern")
		}
	}
}

func TestCanonicalizationIgnoresPropertyOrderDeeply(t *testing.T) {
	s := newTestServer(t, nil)
	call(t, s, "s1", "probe", `{"a":1,"nested":{"x":[1,2],"y":null}}`)
	call(t, s, "s1", "probe", `{"nested":{"y":null,"x":[1,2]},"a":1}`)
	if n := call(t, s, "s1", "probe", `{"a":1,"nested":{"x":[1,2],"y":null}}`); !strings.Contains(n, "repeating") {
		t.Fatalf("deep-equal args must canonicalize identically, got %q", n)
	}
}

func TestPreviewCapBoundsOnlyVisibleText(t *testing.T) {
	// 双阈值：首个（2）为简短档无参数引用，详细档（3）才携带参数预览
	s := newTestServer(t, map[string]string{envRepeatThresholds: "2,3", envRepeatPreview: "24"})
	big := strings.Repeat("x", 400)
	payload := fmt.Sprintf(`{"body":%q}`, big)
	call(t, s, "s1", "probe", payload)
	call(t, s, "s1", "probe", payload)
	n := call(t, s, "s1", "probe", payload) // 链键全量匹配仍命中（检测不受预览上限影响）
	if !strings.Contains(n, `{"body":"xxxxxxxxxxxxxxx… (+387 more chars)`) {
		t.Fatalf("preview should be capped at 24 chars with omission marker, got %q", n)
	}
	if strings.Contains(n, big) {
		t.Fatalf("full payload must not ride into the reminder")
	}
}

func TestChainsArePerSession(t *testing.T) {
	s := newTestServer(t, nil)
	call(t, s, "a", "shell", `{"cmd":"ls"}`)
	call(t, s, "b", "shell", `{"cmd":"ls"}`)
	call(t, s, "b", "shell", `{"cmd":"ls"}`)
	if n := call(t, s, "a", "shell", `{"cmd":"ls"}`); n != "" {
		t.Fatalf("session a count must be independent (2 < 3), got %q", n)
	}
	if n := call(t, s, "b", "shell", `{"cmd":"ls"}`); !strings.Contains(n, "repeating") {
		t.Fatalf("session b should escalate at 3 (gentle), got %q", n)
	}
}

func TestUserInputResetsChain(t *testing.T) {
	s := newTestServer(t, nil)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	if _, err := s.handleHostEvent(context.Background(), "agent/pre-step",
		`{"agent":"agent-react-loop","session":"s1","user_input":true}`); err != nil {
		t.Fatalf("handleHostEvent: %v", err)
	}
	if n := call(t, s, "s1", "shell", `{"cmd":"ls"}`); n != "" {
		t.Fatalf("chain must reset on user interjection, got %q", n)
	}
}

func TestUserInputOtherSessionDoesNotReset(t *testing.T) {
	s := newTestServer(t, nil)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	call(t, s, "s1", "shell", `{"cmd":"ls"}`)
	_, _ = s.handleHostEvent(context.Background(), "agent/pre-step",
		`{"session":"other","user_input":true}`)
	// 其他会话的插话不得重置 s1：第 3 连击照常命中阈值（若被重置则此处应为空）
	if n := call(t, s, "s1", "shell", `{"cmd":"ls"}`); !strings.Contains(n, "repeating") {
		t.Fatalf("other session's interjection must not reset s1, got %q", n)
	}
}

func TestDeniedCallsStillCounted(t *testing.T) {
	s := newTestServer(t, map[string]string{envRepeatThresholds: "2"})
	callDenied := func() string {
		dec, err := s.OnEvent(context.Background(), &proto.PolicyEvent{
			Kind: "tool/post-execute", Tool: "shell", ArgumentsJson: `{"cmd":"sealed"}`,
			Session: "s1", Error: "tool call denied by policy policy-fs-observation",
		})
		if err != nil {
			t.Fatalf("OnEvent: %v", err)
		}
		return dec.GetNotice()
	}
	callDenied()
	if n := callDenied(); !strings.Contains(n, "repeating") {
		t.Fatalf("hammering a denied tool must draw the reminder, got %q", n)
	}
}

func TestNoSessionOwnerIgnored(t *testing.T) {
	s := newTestServer(t, nil)
	for i := 0; i < 5; i++ {
		if n := call(t, s, "", "shell", `{"cmd":"ls"}`); n != "" {
			t.Fatalf("calls without session owner must not be tracked, got %q", n)
		}
	}
}

func TestNonPostExecuteIgnored(t *testing.T) {
	s := newTestServer(t, nil)
	for i := 0; i < 5; i++ {
		dec, err := s.OnEvent(context.Background(), &proto.PolicyEvent{
			Kind: "tool/pre-execute", Tool: "shell", ArgumentsJson: `{"cmd":"ls"}`, Session: "s1",
		})
		if err != nil {
			t.Fatalf("OnEvent: %v", err)
		}
		if dec.GetNotice() != "" || dec.GetAction() != "" {
			t.Fatalf("pre-execute must be pass-through allow, got %+v", dec)
		}
	}
}

func TestConfigValidationFailsLoud(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"empty thresholds", map[string]string{envRepeatThresholds: ","}, "must not be empty"},
		{"below 2", map[string]string{envRepeatThresholds: "1,3"}, "integer >= 2"},
		{"non-integer", map[string]string{envRepeatThresholds: "2.5"}, "must be integers"},
		{"duplicates", map[string]string{envRepeatThresholds: "3,3"}, "must not contain duplicates"},
		{"zero preview", map[string]string{envRepeatPreview: "0"}, "integer >= 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{envRepeatThresholds, envRepeatInclude, envRepeatExclude, envRepeatPreview} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := newReminderServer(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("newReminderServer err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

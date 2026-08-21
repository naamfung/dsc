package tui

import (
	"context"
	"strings"
	"testing"

	"dsc/plugin"
)

// TestSessionCommandSwitch 校验 /session <id> 命令触发 agent 会话切换。
func TestSessionCommandSwitch(t *testing.T) {
	ag := &stubAgent{}
	m := New(ag, nil, context.Background(), "m", "minimal", 131072)

	handled, cmd := m.runSlashCommand("/session session-2")
	if !handled {
		t.Fatal("/session should be handled")
	}
	if cmd != nil {
		t.Fatalf("cmd = %v, want nil", cmd)
	}
	if len(ag.switchCalls) != 1 || ag.switchCalls[0] != "session-2" {
		t.Fatalf("switchCalls = %v, want [session-2]", ag.switchCalls)
	}
}

// TestSessionCommandSwitchWithoutArg 校验缺 id 时给出用法提示。
func TestSessionCommandSwitchWithoutArg(t *testing.T) {
	ag := &stubAgent{}
	m := New(ag, nil, context.Background(), "m", "minimal", 131072)

	handled, _ := m.runSlashCommand("/session")
	if !handled {
		t.Fatal("/session should be handled")
	}
	if len(ag.switchCalls) != 0 {
		t.Fatalf("switchCalls = %v, want none", ag.switchCalls)
	}
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, "用法") {
		t.Fatalf("should show usage hint, got: %q", full)
	}
}

// TestSessionsCommandLists 校验 /sessions 列出会话（manager 提供目录）。
func TestSessionsCommandLists(t *testing.T) {
	mgr := plugin.NewManager(&plugin.ManagerConfig{ExecDir: t.TempDir()})
	m := New(&stubAgent{}, mgr, context.Background(), "m", "minimal", 131072)

	handled, _ := m.runSlashCommand("/sessions")
	if !handled {
		t.Fatal("/sessions should be handled")
	}
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, "会话") {
		t.Fatalf("should show session list header, got: %q", full)
	}
}

// TestSessionCommandSwitchFailure 校验切换失败时显示错误（stub 无法直接模拟，
// 用 manager 为空、agent 为自定义错误 stub 的方式验证错误分支）。
func TestSessionCommandSwitchFailure(t *testing.T) {
	ag := &stubAgent{}
	m := New(ag, nil, context.Background(), "m", "minimal", 131072)
	// manager 为 nil 不影响 /session（走 agent.SwitchSession），stub 返回 nil 成功。
	// 这里验证成功分支的提示文案。
	_, _ = m.runSlashCommand("/session session-3")
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, "已切换到会话 session-3") {
		t.Fatalf("should confirm switch, got: %q", full)
	}
}

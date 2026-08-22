package tui

import (
	"context"
	"strings"
	"testing"

	"dsc/jobs"
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

// TestPlanCommandToggle 校验 /plan 与 /plan off 触发 agent 的 SetPlanMode。
func TestPlanCommandToggle(t *testing.T) {
	ag := &stubAgent{}
	m := New(ag, nil, context.Background(), "m", "minimal", 131072)

	handled, _ := m.runSlashCommand("/plan")
	if !handled {
		t.Fatal("/plan should be handled")
	}
	handled, _ = m.runSlashCommand("/plan off")
	if !handled {
		t.Fatal("/plan off should be handled")
	}
	if len(ag.planCalls) != 2 || ag.planCalls[0] != true || ag.planCalls[1] != false {
		t.Fatalf("planCalls = %v, want [true false]", ag.planCalls)
	}
}

// TestJobCommandAddAndList 校验 /job add 添加任务、/jobs 列出任务。
func TestJobCommandAddAndList(t *testing.T) {
	mgr := plugin.NewManager(&plugin.ManagerConfig{ExecDir: t.TempDir()})
	if err := mgr.StartJobs(); err != nil {
		t.Fatalf("start jobs: %v", err)
	}
	defer mgr.StopJobs()
	m := New(&stubAgent{}, mgr, context.Background(), "m", "minimal", 131072)

	handled, _ := m.runSlashCommand("/job add 0 8 * * * 写今日日报")
	if !handled {
		t.Fatal("/job add should be handled")
	}
	list := mgr.ListJobs()
	if len(list) != 1 || list[0].Cron != "0 8 * * *" || list[0].Prompt != "写今日日报" || !list[0].Enabled {
		t.Fatalf("jobs = %+v", list)
	}

	handled, _ = m.runSlashCommand("/jobs")
	if !handled {
		t.Fatal("/jobs should be handled")
	}
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, list[0].ID) {
		t.Fatalf("should list job, got: %q", full)
	}

	// 无效 cron 应报错且不落盘
	if err := mgr.AddJob(&jobs.Job{Name: "bad", Cron: "nope", Prompt: "x"}); err == nil {
		t.Fatal("invalid cron should fail")
	}
}

// TestSessionCommandNew 校验 /session new 新建会话并切换。
func TestSessionCommandNew(t *testing.T) {
	mgr := plugin.NewManager(&plugin.ManagerConfig{ExecDir: t.TempDir()})
	ag := &stubAgent{}
	m := New(ag, mgr, context.Background(), "m", "minimal", 131072)

	if handled, _ := m.runSlashCommand("/session new"); !handled {
		t.Fatal("/session new should be handled")
	}
	if len(ag.switchCalls) != 1 {
		t.Fatalf("switchCalls = %v, want 1", ag.switchCalls)
	}
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, "已新建并切换到会话") {
		t.Fatalf("should confirm new session, got: %q", full)
	}
}

// TestSessionCommandDelete 校验 /session delete <id> 删除会话。
func TestSessionCommandDelete(t *testing.T) {
	mgr := plugin.NewManager(&plugin.ManagerConfig{ExecDir: t.TempDir()})
	// 先建一个会话供删除
	id, err := mgr.CreateSession()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	m := New(&stubAgent{}, mgr, context.Background(), "m", "minimal", 131072)

	if handled, _ := m.runSlashCommand("/session delete " + id); !handled {
		t.Fatal("/session delete should be handled")
	}
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, "已删除会话") {
		t.Fatalf("should confirm delete, got: %q", full)
	}
	// 验证文件已被删除
	summaries, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, s := range summaries {
		if s.ID == id {
			t.Fatalf("session %s should be deleted", id)
		}
	}
}

// TestSandboxCommandToggle 校验 /sandbox on/off 切换运行时沙箱策略。
func TestSandboxCommandToggle(t *testing.T) {
	mgr := plugin.NewManager(&plugin.ManagerConfig{ExecDir: t.TempDir()})
	m := New(&stubAgent{}, mgr, context.Background(), "m", "minimal", 131072)

	// 初始为缺省 workspace
	if got := mgr.GetSandboxPolicy(); got != plugin.SandboxWorkspaceWrite {
		t.Fatalf("initial policy = %v, want workspace", got)
	}
	if handled, _ := m.runSlashCommand("/sandbox on"); !handled {
		t.Fatal("/sandbox on should be handled")
	}
	if got := mgr.GetSandboxPolicy(); got != plugin.SandboxReadOnly {
		t.Fatalf("after on, policy = %v, want read-only", got)
	}
	if handled, _ := m.runSlashCommand("/sandbox off"); !handled {
		t.Fatal("/sandbox off should be handled")
	}
	if got := mgr.GetSandboxPolicy(); got != plugin.SandboxFullAccess {
		t.Fatalf("after off, policy = %v, want full", got)
	}
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, "沙箱") {
		t.Fatalf("should show sandbox messages, got: %q", full)
	}
}

// TestSandboxCommandWithoutManager 校验 manager 缺失时的错误分支。
func TestSandboxCommandWithoutManager(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	if handled, _ := m.runSlashCommand("/sandbox on"); !handled {
		t.Fatal("/sandbox on should be handled")
	}
	full := strings.Join(m.lines, "\n")
	if !strings.Contains(full, "插件管理器不可用") {
		t.Fatalf("should show manager-unavailable error, got: %q", full)
	}
}

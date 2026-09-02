package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dsc/userquestions"
)

// approvalTestManager 组装审批门 + 沙箱判定的测试 manager（因 newRouterManager
// 重置了事件总线，listeners 需显式注册：先审批门后沙箱，与生产一致）。
func approvalTestManager(t testing.TB, sandbox SandboxPolicy, policy ApprovalPolicy) *Manager {
	t.Helper()
	m := newRouterManager()
	m.SetSandboxPolicy(sandbox)
	m.SetApprovalPolicy(policy)
	m.events.OnWaterfall(EventToolPreExecute, m.approvalEscalation())
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(m.GetSandboxPolicy))
	return m
}

// escArgs 为一次写调用附加升级参数（sandbox_permissions + justification）。
func escArgs(perm, justification string) json.RawMessage {
	args := map[string]any{
		"command": "str_replace", "path": WorkspaceRoot + "/a.txt",
		"old_str": "a", "new_str": "b",
	}
	if perm != "" {
		args["sandbox_permissions"] = perm
	}
	if justification != "" {
		args["justification"] = justification
	}
	b, _ := json.Marshal(args)
	return b
}

func TestParseApprovalPolicy(t *testing.T) {
	cases := map[string]ApprovalPolicy{
		"ask": ApprovalAsk, "on": ApprovalAsk, "": ApprovalAsk, "bogus": ApprovalAsk,
		"never": ApprovalNever, "off": ApprovalNever,
	}
	for in, want := range cases {
		if got := ParseApprovalPolicy(in); got != want {
			t.Errorf("ParseApprovalPolicy(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWidenableLadder(t *testing.T) {
	if !widenable(SandboxReadOnly, SandboxWorkspaceWrite) {
		t.Error("read-only→workspace-write 应严格加宽")
	}
	if !widenable(SandboxReadOnly, SandboxFullAccess) {
		t.Error("read-only→full 应严格加宽")
	}
	if widenable(SandboxWorkspaceWrite, SandboxWorkspaceWrite) {
		t.Error("workspace→workspace 非严格加宽")
	}
	if !widenable(SandboxWorkspaceWrite, SandboxFullAccess) {
		t.Error("workspace→full 应严格加宽")
	}
	if widenable(SandboxFullAccess, SandboxFullAccess) || widenable(SandboxFullAccess, SandboxWorkspaceWrite) {
		t.Error("full 是顶档，任何升级都非严格加宽")
	}
}

func TestEscalationArgsValidation(t *testing.T) {
	if err := validateEscalationArgs("", ""); err != nil {
		t.Fatalf("双空应放行（非升级）: %v", err)
	}
	if err := validateEscalationArgs("workspace-write", ""); err == nil || !strings.Contains(err.Error(), "requires a justification") {
		t.Fatalf("perm 无理由应报错: %v", err)
	}
	if err := validateEscalationArgs("", "reason"); err == nil || !strings.Contains(err.Error(), "only valid together") {
		t.Fatalf("仅理由无权限应报错: %v", err)
	}
}

func TestEscalationBlockedNoProvider(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()
	m := approvalTestManager(t, SandboxReadOnly, ApprovalAsk)
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	_, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		escArgs("workspace-write", "need to edit a file"))
	if err == nil || !strings.Contains(err.Error(), "no approval channel is available") {
		t.Fatalf("read-only 升级 ws 无评审通道应 fail-closed，got %v", err)
	}
}

func TestEscalationAutoRejectUnderNever(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()
	m := approvalTestManager(t, SandboxReadOnly, ApprovalNever)
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	_, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		escArgs("workspace-write", "need to edit a file"))
	if err == nil || !strings.Contains(err.Error(), "approval prompts are disabled") {
		t.Fatalf("never 应自动拒绝、不问人，got %v", err)
	}
}

func TestEscalationRejectsNonWidening(t *testing.T) {
	m := approvalTestManager(t, SandboxFullAccess, ApprovalAsk)
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	// full 已是顶档：升级任何更宽档都非严格加宽，执行前拒绝且不问人。
	_, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		escArgs("danger-full-access", "need it"))
	if err == nil || !strings.Contains(err.Error(), "not strictly wider") {
		t.Fatalf("full 下升级应拒绝，got %v", err)
	}
}

func TestEscalationAllowedOnceExecutesUnderWiderMode(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()
	m := approvalTestManager(t, SandboxReadOnly, ApprovalAsk)
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{"Allow once"}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}

	// read-only 基档下升级到 workspace-write 并获批：本次应按更宽档放行、真正执行。
	result, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		escArgs("workspace-write", "need to edit a file"))
	if err != nil {
		t.Fatalf("allowed-once 应按更宽档放行: %v", err)
	}
	if strings.TrimSpace(result) != "mock-result" {
		t.Fatalf("result = %q, want mock-result", result)
	}
}

func TestNonWriteToolIgnoresEscalationArgs(t *testing.T) {
	m := approvalTestManager(t, SandboxReadOnly, ApprovalNever)
	_ = m.toolRegistry.Register(&mockTool{name: "grep"})
	// 非写/非执行器工具（grep）不受升级参数影响，走沙箱正常判定（read-only 下读放行）。
	args, _ := json.Marshal(map[string]any{"sandbox_permissions": "danger-full-access", "justification": "why"})
	if _, err := m.ExecuteTool(context.Background(), "grep", args); err != nil {
		t.Fatalf("非写工具应忽略 sandbox_permissions，got %v", err)
	}
}

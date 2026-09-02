package core

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"dsc/userquestions"
)

// approvalTestManager 组装审批门 + 工具声明审批门 + 沙箱判定的测试 manager（因
// newRouterManager 重置了事件总线，listeners 需显式注册：顺序与生产一致）。
func approvalTestManager(t testing.TB, sandbox SandboxPolicy, policy ApprovalPolicy) *Manager {
	t.Helper()
	m := newRouterManager()
	m.SetSandboxPolicy(sandbox)
	m.SetApprovalPolicy(policy)
	m.events.OnWaterfall(EventToolPreExecute, m.approvalEscalation())
	m.events.OnWaterfall(EventToolPreExecute, m.toolApprovalGate())
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

func TestApprovalPolicyChangeEmitted(t *testing.T) {
	m := approvalTestManager(t, SandboxReadOnly, ApprovalAsk)
	var gotPolicy string
	var gotSession string
	m.events.OnAny(func(ctx EventContext) (any, error) {
		if ctx.Name != EventApprovalPolicy {
			return nil, nil
		}
		d, _ := ctx.Data.(map[string]string)
		gotSession, gotPolicy = d["session"], d["policy"]
		return nil, nil
	})

	m.SetSessionApprovalPolicy("sess-5", ApprovalNever)
	if gotSession != "sess-5" || gotPolicy != "never" {
		t.Fatalf("policy-change event = (%q,%q), want (sess-5,never)", gotSession, gotPolicy)
	}
}

func TestPerSessionApprovalPolicy(t *testing.T) {
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

	// 会话 A 覆盖为 never：升级自动拒（不问人）。
	m.SetSessionApprovalPolicy("sessionA", ApprovalNever)
	if _, err := m.ExecuteTool(WithCaller(context.Background(), "sessionA"), "str_replace_editor",
		escArgs("workspace-write", "need it")); err == nil || !strings.Contains(err.Error(), "approval prompts are disabled") {
		t.Fatalf("sessionA never 应自动拒，got %v", err)
	}
	// 会话 B 无覆盖（回退默认 ask）：升级获批放行。
	_, err := m.ExecuteTool(WithCaller(context.Background(), "sessionB"), "str_replace_editor",
		escArgs("workspace-write", "need it"))
	if err != nil {
		t.Fatalf("sessionB 默认 ask 应获批放行，got %v", err)
	}
}

func TestApprovalAuditEmittedWithSession(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()
	m := approvalTestManager(t, SandboxReadOnly, ApprovalAsk)
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{"Reject"}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}

	type audit struct{ name, session, outcome string }
	var got []audit
	m.events.OnAny(func(ctx EventContext) (any, error) {
		if ctx.Name != EventApprovalAsked && ctx.Name != EventApprovalDecided {
			return nil, nil
		}
		d, _ := ctx.Data.(map[string]string)
		got = append(got, audit{name: string(ctx.Name), session: d["session"], outcome: d["outcome"]})
		return nil, nil
	})

	ctx := WithCaller(context.Background(), "sess-1")
	_, err := m.ExecuteTool(ctx, "str_replace_editor", escArgs("workspace-write", "need it"))
	if err == nil || !strings.Contains(err.Error(), "user rejected") {
		t.Fatalf("reject 应返回被拒，got %v", err)
	}
	if len(got) != 2 || got[0].name != "approval/asked" || got[1].name != "approval/decided" {
		t.Fatalf("audit = %+v, want asked+decided", got)
	}
	if got[0].session != "sess-1" || got[1].session != "sess-1" {
		t.Fatalf("audit 应带会话 id，got %+v", got)
	}
	if got[1].outcome != "rejected" {
		t.Fatalf("decided outcome = %q, want rejected", got[1].outcome)
	}
}

// askApprovalTool 一个实现 ApprovalRequester 的 mock 工具（入口②工具声明需审批）。
type askApprovalTool struct {
	name, reason string
}

func (t *askApprovalTool) Name() string                      { return t.name }
func (t *askApprovalTool) Description() string               { return "declares approval need" }
func (t *askApprovalTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *askApprovalTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "mock-result", nil
}
func (t *askApprovalTool) ApprovalReason(_ string) string { return t.reason }

func TestSandboxApprovalDefault(t *testing.T) {
	if SandboxApprovalDefault(SandboxFullAccess) != ApprovalNever {
		t.Error("full → never")
	}
	if SandboxApprovalDefault(SandboxWorkspaceWrite) != ApprovalAsk {
		t.Error("workspace → ask")
	}
	if SandboxApprovalDefault(SandboxReadOnly) != ApprovalAsk {
		t.Error("read-only → ask")
	}
}

func TestPermissionPresetBindingAndPrecedence(t *testing.T) {
	// 隔离环境：确保审批「未显式设置」（否则绑定默认不生效）。
	old := os.Getenv("DSC_APPROVAL")
	_ = os.Setenv("DSC_APPROVAL", "")
	defer func() { _ = os.Setenv("DSC_APPROVAL", old) }()
	m := newRouterManager()

	// 绑定默认：full sandbox → never；workspace → ask。
	m.SetSandboxPolicy(SandboxFullAccess)
	if got := m.approvalPolicyFor("s1"); got != ApprovalNever {
		t.Fatalf("full sandbox 绑定默认 = %v, want never", got)
	}
	m.SetSandboxPolicy(SandboxWorkspaceWrite)
	if got := m.approvalPolicyFor("s1"); got != ApprovalAsk {
		t.Fatalf("workspace sandbox 绑定默认 = %v, want ask", got)
	}

	// 会话覆盖优先于绑定。
	m.SetSandboxPolicy(SandboxFullAccess)
	m.SetSessionApprovalPolicy("s2", ApprovalAsk)
	if got := m.approvalPolicyFor("s2"); got != ApprovalAsk {
		t.Fatalf("会话覆盖应优先于绑定，got %v", got)
	}

	// 显式全局优先于绑定。
	m.SetApprovalPolicy(ApprovalAsk)
	m.SetSandboxPolicy(SandboxFullAccess)
	if got := m.approvalPolicyFor("s3"); got != ApprovalAsk {
		t.Fatalf("显式全局应优先于绑定，got %v", got)
	}
}

func TestApprovalPolicyForwardedByCaller(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()
	// 宿主本地解析会落到 never（显式全局 never 或 full 沙箱绑定）；但调用方会话随调用
	// 转发 ask → 审批门应以调用方策略为准（重启后 per-session 恢复）。
	m := approvalTestManager(t, SandboxWorkspaceWrite, ApprovalNever)
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{"Allow once"}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}

	ctx := WithApprovalPolicy(WithCaller(context.Background(), "s"), "ask")
	result, err := m.ExecuteTool(ctx, "str_replace_editor", escArgs("danger-full-access", "need it"))
	if err != nil {
		t.Fatalf("调用方转发 ask 应问人获批并放行，got %v", err)
	}
	if strings.TrimSpace(result) != "mock-result" {
		t.Fatalf("result = %q, want mock-result", result)
	}
}

func TestEscalationSubject(t *testing.T) {
	if escalationSubject("shell") != "command" {
		t.Error("shell 应为 command")
	}
	if escalationSubject("str_replace_editor") != "operation" {
		t.Error("str_replace_editor 应为 operation")
	}
	if escalationSubject("grep") != "operation" {
		t.Error("其它工具默认 operation")
	}
}

func TestApprovalCancellationPropagatesToAsk(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()
	m := approvalTestManager(t, SandboxReadOnly, ApprovalAsk)
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})
	// provider 感知 ctx 取消：返回 CANCELLED 错误，令审批映射为 cancelled。
	if err := m.RegisterUserQuestionProvider(func(ctx context.Context, _ *userquestions.Request) (*userquestions.Answer, error) {
		if ctx.Err() != nil {
			return nil, &userquestions.Error{Code: userquestions.ErrCanceled, Err: ctx.Err()}
		}
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{"Allow once"}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}

	cctx, cancel := context.WithCancel(WithCaller(context.Background(), "s"))
	cancel() // 立即取消：审批提问应感知到 → cancelled
	_, err := m.ExecuteTool(cctx, "str_replace_editor", escArgs("workspace-write", "need it"))
	if err == nil || !strings.Contains(err.Error(), "was cancelled") {
		t.Fatalf("取消 ctx 应令审批为 cancelled，got %v", err)
	}
}

func TestToolDeclaredApprovalAllowedOnce(t *testing.T) {
	m := approvalTestManager(t, SandboxReadOnly, ApprovalAsk)
	_ = m.toolRegistry.Register(&askApprovalTool{name: "danger_tool", reason: "needs approval"})
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{"Allow once"}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := m.ExecuteTool(context.Background(), "danger_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("允许应放行执行: %v", err)
	}
	if strings.TrimSpace(result) != "mock-result" {
		t.Fatalf("result = %q, want mock-result", result)
	}
}

func TestToolDeclaredApprovalRejected(t *testing.T) {
	m := approvalTestManager(t, SandboxReadOnly, ApprovalAsk)
	_ = m.toolRegistry.Register(&askApprovalTool{name: "danger_tool", reason: "needs approval"})
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{"Reject"}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := m.ExecuteTool(context.Background(), "danger_tool", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), `user rejected tool "danger_tool"`) {
		t.Fatalf("拒绝应返回被拒，got %v", err)
	}
}

func TestToolDeclaredApprovalNeverAutoReject(t *testing.T) {
	m := approvalTestManager(t, SandboxReadOnly, ApprovalNever)
	_ = m.toolRegistry.Register(&askApprovalTool{name: "danger_tool", reason: "needs approval"})
	_, err := m.ExecuteTool(context.Background(), "danger_tool", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "approval prompts are disabled") {
		t.Fatalf("never 应自动拒，got %v", err)
	}
}

func TestToolDeclaredApprovalEmptyReasonPasses(t *testing.T) {
	m := approvalTestManager(t, SandboxReadOnly, ApprovalNever)
	_ = m.toolRegistry.Register(&askApprovalTool{name: "benign_tool", reason: ""})
	if _, err := m.ExecuteTool(context.Background(), "benign_tool", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("空 reason 应正常执行，got %v", err)
	}
}

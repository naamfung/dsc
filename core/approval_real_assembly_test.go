package core

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"

	"dsc/userquestions"
)

// TestApprovalChainRealManagerAssembly 守卫「生产装配」下的审批链关键顺序：
// 宿主用 NewManager（而非测试 helper 手动排门）构造时，升级审批门与工具声明审批门
// 必须注册在沙箱策略门**之前**。若未来有人把构造器里的 OnWaterfall 注册顺序改乱
// （如把 sandboxPolicy 排到审批门前面），那么：
//   - 升级重试会被沙箱先拒（deny marker），升级审批永远走不到 —— 升级链静默失效；
//   - read-only 下即便工具声明了审批，也会先被沙箱拒绝而非走审批提示。
//
// 此测试不手动注册任何门，只用 NewManager 真实装配，从行为上锁定顺序约束。
func TestApprovalChainRealManagerAssembly(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()

	m := NewManager(&ManagerConfig{Logger: hclog.NewNullLogger(), PluginLogger: hclog.NewNullLogger()})
	m.SetSandboxPolicy(SandboxReadOnly)
	m.SetApprovalPolicy(ApprovalAsk)
	// 写类工具（承载升级语义；isWriteTool/isWriteCapableExecutor 命中 str_replace_editor）
	if err := m.toolRegistry.Register(&mockTool{name: "str_replace_editor"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	// 工具声明审批（入口②）
	if err := m.toolRegistry.Register(&askApprovalTool{name: "ask_approval_tool", reason: "needs approval"}); err != nil {
		t.Fatalf("register approval-declared tool: %v", err)
	}

	// 固定批准 provider（用户放行）。
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{"Allow once"}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}

	// 1) 升级重试获审批放行 → 必须以更宽档真正执行（确保升级门在沙箱门之前）。
	res, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		escArgs("workspace-write", "need to finalize the deliverable"))
	if err != nil {
		t.Fatalf("read-only 下升级重试经审批应放行（说明升级门先于沙箱门），got err: %v", err)
	}
	if !strings.Contains(res, "mock-result") {
		t.Fatalf("放行后应执行工具本体, got: %q", res)
	}

	// 2) 工具声明审批（无升级参数）在真实装配下同样走审批提示并放行
	//    （说明 toolApprovalGate 在沙箱门之前）。
	if _, err := m.ExecuteTool(context.Background(), "ask_approval_tool", []byte(`{}`)); err != nil {
		t.Fatalf("工具声明审批在 read-only 下应经审批放行（审批门先于沙箱门），got err: %v", err)
	}
}

// TestApprovalChainRealManagerSandboxStillDenies 守卫真实装配下「无审批参数时由沙箱
// 正常判定」，避免审批门误吞沙箱原有语义（read-only 下非升级写调用仍应按沙箱拒绝，
// 而不是静默放行或报「无审批通道」）。
func TestApprovalChainRealManagerSandboxStillDenies(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir()
	defer func() { WorkspaceRoot = orig }()

	m := NewManager(&ManagerConfig{Logger: hclog.NewNullLogger(), PluginLogger: hclog.NewNullLogger()})
	m.SetSandboxPolicy(SandboxReadOnly)
	// 无 provider：有审批参数的调用才可能触发审批；无审批参数则纯走沙箱判定。
	if err := m.toolRegistry.Register(&mockTool{name: "str_replace_editor"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	// 不带升级参数的写调用 → 应被沙箱拒（deny marker），而不是因无审批通道而报错。
	_, err := m.ExecuteTool(context.Background(), "str_replace_editor", escArgs("", ""))
	if err == nil {
		t.Fatal("read-only 下非升级写调用应被沙箱拒绝")
	}
	if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "deny") {
		t.Fatalf("拒绝信息应含沙箱 deny 标记, got: %v", err)
	}
}

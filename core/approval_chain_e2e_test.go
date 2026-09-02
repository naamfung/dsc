package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
	"dsc/userquestions"
	plugin "github.com/hashicorp/go-plugin"
)

// 本文件覆盖「审批链」在宿主聚合层的真实链路集成（对齐 AGENTS.md 第3条的真实插件集成测试）：
// 真实工具插件进程 → RemoteTool → 宿主 pre-execute 审批门（approveEscalation）→ 审批 Ask
// （假 provider 回答）→ 更宽档放行 → 真实插件执行。此前审批测试全用 mockTool，未接真实
// 插件进程，补上这跳以防「宿主审批门 ↔ 真实工具插件」衔接点被忽略。

// TestApprovalChainEscalationRealPlugin 用真实 tool-filesystem(shell) 插件进程验证：
//  1. 无升级：read-only 下 shell 被拒 → 返回 DSH 拒绝标记 + 升级提示；
//  2. 升级重试（sandbox_permissions 更宽档 + justification）：审批 ask 批准 → 以更宽档放行
//     → 真实插件真正执行；同时审批审计事件（approval/asked + decided）被广播。
func TestApprovalChainEscalationRealPlugin(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-filesystem.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "tool-filesystem"), exe)

	// spawn 真实 shell 插件（与生产 loadPluginWithBroker 同路径）。
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_WORKSPACE_ROOT="+filepath.ToSlash(dir))
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          map[string]plugin.Plugin{"dsc_core": &DSCPluginGRPC{}},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Cmd:              cmd,
	})
	defer client.Kill()
	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	grpcClient, ok := rpcClient.(*plugin.GRPCClient)
	if !ok {
		t.Fatalf("client %T not *GRPCClient", rpcClient)
	}
	toolClient := proto.NewToolServiceClient(grpcClient.Conn)
	defs, _, err := listStagedTools(toolClient)
	if err != nil {
		t.Fatalf("listStagedTools: %v", err)
	}

	// 宿主：保留 NewManager 注册的审批门(+沙箱)；把沙箱切到 read-only。
	m := NewManager(&ManagerConfig{ExecDir: dir})
	m.SetSandboxPolicy(SandboxReadOnly)
	for _, d := range defs {
		if err := m.toolRegistry.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	// 假审批 answerer：始终 Allow once。
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{approvalAllowLabel}}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	// 捕获审批审计事件。
	type audit struct{ name, mode, outcome string }
	var got []audit
	m.events.OnAny(func(ctx EventContext) (any, error) {
		if ctx.Name != EventApprovalAsked && ctx.Name != EventApprovalDecided {
			return nil, nil
		}
		d, _ := ctx.Data.(map[string]string)
		got = append(got, audit{name: string(ctx.Name), mode: d["mode"], outcome: d["outcome"]})
		return nil, nil
	})

	srv := NewToolGRPCServer(m)

	// 1) 无升级：read-only 下 shell（执行器）被拒 → DSH 拒绝标记 + 升级提示。
	resp, err := srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{
		ToolName: "shell", ArgumentsJson: `{"command":"echo hi"}`,
	})
	if err != nil {
		t.Fatalf("first ExecuteTool: %v", err)
	}
	if resp.Error == "" || !strings.Contains(resp.Error, "file access denied under read-only mode") ||
		!strings.Contains(resp.Error, "escalation available") {
		t.Fatalf("read-only 下 shell 应按 DSH 标记拒绝，got %q", resp.Error)
	}

	// 2) 升级重试（sandbox_permissions=workspace-write + justification）→ approval ask 批准
	//    → 以更宽档放行 → 真实插件执行。
	resp, err = srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{
		ToolName:      "shell",
		ArgumentsJson: `{"command":"echo hi","sandbox_permissions":"workspace-write","justification":"need to run a command"}`,
	})
	if err != nil {
		t.Fatalf("escalation ExecuteTool: %v", err)
	}
	if resp.Error != "" || !strings.Contains(resp.Content, "hi") {
		t.Fatalf("升级获批后应真实执行 shell，got error=%q content=%q", resp.Error, resp.Content)
	}
	// 审批审计事件确已发出（asked + decided，mode=workspace-write，outcome=allowed-once）。
	if len(got) != 2 || got[0].name != string(EventApprovalAsked) || got[1].name != string(EventApprovalDecided) {
		t.Fatalf("audit = %+v, want asked+decided", got)
	}
	if got[1].outcome != "allowed-once" || got[0].mode != "workspace-write" {
		t.Fatalf("audit = %+v, want allowed-once / workspace-write", got)
	}
}

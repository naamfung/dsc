package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"dsc/plugin"
	goplugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证 SDK 重写后的 agent 插件：
// 以宿主侧 go-plugin 客户端（loadAgentAndGetBroker 同款）spawn exe，经
// rpcClient.Dispense("agent") 获取 plugin.Agent 实例（SDK 复用
// plugin.AgentGRPCPlugin 的 GRPCClient），并验证 Name/Version/SwitchSession/
// SetPlanMode。完整 Run/RunStream 需宿主挂载 LLM+Tool 聚合服务并走
// RegisterServices，属宿主集成范畴，不在 SDK 层验证。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "agent-react-loop.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程（loadAgentAndGetBroker 同款 Plugins 配置）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"DSC_SESSION_DIR="+filepath.Join(dir, "sessions"),
		"DSC_SINGLE_TURN=0",
	)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  plugin.Handshake,
		Plugins:          map[string]goplugin.Plugin{"agent": &plugin.AgentGRPCPlugin{}},
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Cmd:              cmd,
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	// 3. Dispense("agent") 获取 Agent 实例（宿主 loadAgentAndGetBroker 同款路径）
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		t.Fatalf("Dispense(agent): %v", err)
	}
	agent, ok := raw.(plugin.Agent)
	if !ok {
		t.Fatalf("dispensed value %T 未实现 plugin.Agent", raw)
	}

	// 4. 基础方法
	ctx := context.Background()
	if name := agent.Name(ctx); name != "agent-react-loop" {
		t.Fatalf("Name = %q, want agent-react-loop", name)
	}
	if ver := agent.Version(ctx); ver == "" {
		t.Fatal("Version 不应为空")
	}

	// 5. 会话操作：SwitchSession 后 SetPlanMode（store 已初始化，default 会话可切换）
	if err := agent.SwitchSession(ctx, "default"); err != nil {
		t.Fatalf("SwitchSession: %v", err)
	}
	if err := agent.SetPlanMode(ctx, true); err != nil {
		t.Fatalf("SetPlanMode: %v", err)
	}
}

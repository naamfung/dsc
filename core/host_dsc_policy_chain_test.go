package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	plugin "github.com/hashicorp/go-plugin"
)

// 本文件覆盖「策略 advisory 上下文（notices）」的宿主聚合链（AGENTS 字段保真
// 规则）：真实 dsc-system 插件进程（通用类型 + services 声明 "policy"）经生产
// 同款 registerDscCoreLocked 桥接工具流水线后，宿主聚合工具调用须把策略裁决的
// notice 机械收集并透传到调用方（ExecuteToolWithView 第 4 返回值 → 聚合 Tool
// 服务的 ExecuteToolResponse.notices），成功与失败调用同样携带。任何一层只返回
// 子集都会让提醒静默丢失——此处用真实插件进程端到端锁死。

// TestHostDscPolicyChainPropagatesNotices 真实 dsc-system 进程 → 宿主 policy 桥
// → 工具流水线 → notices 透传的端到端断言。
func TestHostDscPolicyChainPropagatesNotices(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "dsc-system.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "dsc-system"), exe)

	// 1. spawn 真实插件进程（阈值 2 便于断言；排除表清空避免宿主 env 泄漏干扰）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_REPEAT_THRESHOLDS=2", "DSC_REPEAT_EXCLUDE=")
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
	info, err := GetPluginInfo(grpcClient.Conn)
	if err != nil || info.Type != "dsc" {
		t.Fatalf("GetPluginInfo = %+v, err %v（期望 dsc 通用类型）", info, err)
	}

	// 2. 生产同款登记：case "dsc" 按 PluginInfo.services 的 "policy" 声明桥接流水线
	m := NewManager(&ManagerConfig{ExecDir: dir})
	m.mu.Lock()
	m.registerDscCoreLocked("dsc-system", info, client, grpcClient, nil, interconnectRefs{})
	m.mu.Unlock()
	t.Cleanup(func() { m.Shutdown() })
	if _, bridged := m.policyOff["dsc-system"]; !bridged {
		t.Fatalf("services 声明 policy 的 dsc 插件应被桥接到工具流水线")
	}

	// 3. 注册宿主侧被观察工具并连续调用同一参数
	if err := m.toolRegistry.Register(&mockTool{name: "plain-tool"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	exec := func() (string, []string, error) {
		ctx := WithCaller(context.Background(), "sess-1")
		result, _, _, notices, err := m.ExecuteToolWithView(ctx, "plain-tool", json.RawMessage(`{"k":1}`))
		return result, notices, err
	}
	if _, notices, err := exec(); err != nil || len(notices) != 0 {
		t.Fatalf("call 1: err=%v notices=%v（首调未达阈值应无提醒）", err, notices)
	}
	_, notices, err := exec()
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "repeating the exact same tool call") {
		t.Fatalf("call 2 应携带重复提醒 notice，got %v", notices)
	}

	// 4. 用户插话重置：pre-step 事件（waterfall）清链后重新计数
	m.dispatchEventToPlugins(EventAgentPreStep, AgentPreStepEvent{
		Agent: "agent-react-loop", Session: "sess-1", UserInput: true,
	})
	if _, notices, err := exec(); err != nil || len(notices) != 0 {
		t.Fatalf("after user input reset: err=%v notices=%v（链应已重置）", err, notices)
	}
	_, notices, err = exec()
	if err != nil || len(notices) != 1 {
		t.Fatalf("reset 后第 2 调应再次提醒: err=%v notices=%v", err, notices)
	}
}

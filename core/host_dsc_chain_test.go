package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	plugin "github.com/hashicorp/go-plugin"
)

// 本文件覆盖「通用 dsc 类型插件」在宿主聚合层的事件接线：真实插件进程能登记为
// hook client，从而接收宿主 EventBus 广播（OnEvent）。此前 dsc 类型仅为遗留 label、
// 不登记 hook client，事件订阅只对 tool 型接线——此处用真实 dsc 插件进程补上这条
// 链的宿主侧断言。登记走生产同款 registerDscCoreLocked（loadPluginWithBroker 的
// case "dsc" 共用同一实现），spawn 用与本包 tool-filesystem 宿主链测试一致的直接
// 路径（backslash 绝对路径，规避 normalizeBinaryPath 转前斜杠在测试环境 exec 的坑）。

// TestHostDscChainRegistersHook 验证真实 dsc 通用插件经 registerDscCoreLocked
// 登记后，被接入 hookClientsSnapshot（事件订阅接线成立），类型映射/元数据正确。
func TestHostDscChainRegistersHook(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "dsc-notify.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "dsc-notify"), exe)

	// 1. 以宿主侧客户端 spawn 真实 dsc 插件（与 tool-filesystem 宿主链测试同款路径）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_NOTIFY_NO_AUDIO=1")
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

	// 元数据须为 dsc 通用类型
	info, err := GetPluginInfo(grpcClient.Conn)
	if err != nil || info.Type != "dsc" {
		t.Fatalf("GetPluginInfo = %+v, err %v（期望 dsc 通用类型）", info, err)
	}

	// 2. 经生产同款登记（loadPluginWithBroker case "dsc" 的单一真源）把插件接入事件订阅
	m := NewManager(&ManagerConfig{ExecDir: dir})
	m.mu.Lock()
	m.registerDscCoreLocked("dsc-notify", info, client, grpcClient)
	m.mu.Unlock()
	t.Cleanup(func() { m.Shutdown() })

	// 3. 事件接线断言：dsc 插件已登记为 hook client（本次修复的核心）
	if clients := m.hookClientsSnapshot(); len(clients) != 1 {
		t.Fatalf("hookClientsSnapshot = %d, want 1（dsc 插件应接线到事件广播）", len(clients))
	}
	if m.typeMap["dsc-notify"] != "dsc" || m.coreMetadata["dsc-notify"].Type != "dsc" {
		t.Fatalf("dsc-notify 类型映射/元数据错误: typeMap=%q meta=%+v", m.typeMap["dsc-notify"], m.coreMetadata["dsc-notify"])
	}

	// 4. 经宿主 EventBus 广播 agent/status idle，触发到 dsc 插件的广播路径不应报错
	//（异步；插件端 OnEvent 接收已验证于插件 e2e 与真机听声）。此处冒烟确保不禁用发射路径。
	m.events.Emit(EventAgentStatus, EventContext{Data: AgentStatusEvent{
		Agent:  "agent-react-loop",
		Status: AgentStatusIdle,
	}})
}

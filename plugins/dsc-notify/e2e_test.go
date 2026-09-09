package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证通用（dsc）类型插件 notify 的宿主侧契约：
// 以 go-core 客户端（宿主 Manager 同款协议路径）spawn 本插件 exe，验证——元数据
// 类型为 dsc、提供 PluginHookService 可经 OnEvent 订阅宿主事件（agent/status idle），
// 且不再暴露模型可调用的工具面（ListTools 返回空 or 未实现）。DSC_NOTIFY_NO_AUDIO=1
// 跳过音频初始化，保证在无音频设备的环境（如 CI）也能确定性运行。
func TestE2EWithHostClient(t *testing.T) {
	dir := t.TempDir()

	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	exe := filepath.Join(dir, "dsc-notify.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_NOTIFY_NO_AUDIO=1")
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  core.Handshake,
		Plugins:          map[string]plugin.Plugin{},
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
		t.Fatalf("unexpected client type %T", rpcClient)
	}
	conn := grpcClient.Conn
	ctx := context.Background()

	// 3. 元数据：类型应为 dsc（通用类型），而非 tool
	info, err := metadata.NewPluginMetadataClient(conn).GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "dsc" || info.Name != "notify" || info.Version == "" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v（期望 dsc 通用类型）", info, err)
	}

	// 4. Hook 服务可用：经 OnEvent 订阅宿主事件（agent/status idle）无错误。
	// 这是 notify 程序性完成音效的核心链路：宿主广播 → 插件 Hook.OnEvent。
	hc := proto.NewPluginHookServiceClient(conn)
	idle := core.AgentStatusEvent{Agent: "agent-react-loop", Status: core.AgentStatusIdle}
	data, _ := json.Marshal(idle)
	if _, err := hc.OnEvent(ctx, &proto.OnEventRequest{Name: string(core.EventAgentStatus), DataJson: string(data)}); err != nil {
		t.Fatalf("OnEvent(agent/status idle) 失败: %v", err)
	}
	if _, err := hc.OnEvent(ctx, &proto.OnEventRequest{Name: string(core.EventAgentError), DataJson: `{"agent":"a","error":"boom"}`}); err != nil {
		t.Fatalf("OnEvent(agent/error) 失败: %v", err)
	}

	// 5. 无模型可调用的工具面：通用插件不注册 ToolService，ListTools 应报未实现
	//（而非返回 notify 工具），证明已收敛为纯后台程序性插件。
	tc := proto.NewToolServiceClient(conn)
	if _, err := tc.ListTools(ctx, &proto.ListToolsRequest{}); err == nil {
		t.Fatalf("dsc 通用插件不应暴露 ListTools（工具面已收敛）")
	}
}

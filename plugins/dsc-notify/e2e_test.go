package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
)

// assertView 校验工具结果经完整 gRPC 链路透传的 ViewJson：非空且可解析为合法视图。
func assertView(t *testing.T, resp *proto.ExecuteToolResponse) core.ToolView {
	t.Helper()
	if resp.ViewJson == "" {
		t.Fatalf("ViewJson 为空（ViewFn 未生效或 gRPC 透传缺失）: %+v", resp)
	}
	var v core.ToolView
	if err := json.Unmarshal([]byte(resp.ViewJson), &v); err != nil {
		t.Fatalf("ViewJson 非法: %v", err)
	}
	if v.Kind == "" {
		t.Fatalf("ViewJson 缺 kind: %q", resp.ViewJson)
	}
	return v
}

// TestE2EWithHostClient 端到端验证 SDK 重写后的 tool-notify 能被宿主侧正常拉起并调用：
// 以 go-core 客户端（宿主 Manager 同款协议路径）spawn 本插件 exe，经 gRPC 验证元数据、
// 工具目录与 notify 工具执行。以 DSC_NOTIFY_NO_AUDIO=1 跳过音频初始化，保证在无音频
// 设备的环境（如 CI）也能确定性运行；工具执行只入队即返回，不依赖音频设备播放。
func TestE2EWithHostClient(t *testing.T) {
	dir := t.TempDir()

	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	exe := filepath.Join(dir, "tool-notify.exe")
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

	// 3. 元数据（SDK 自动提供）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "tool" || info.Name != "notify" || info.Version == "" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 4. 工具目录（SDK 自动聚合唯一 notify 工具）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "notify" {
		t.Fatalf("expected 1 notify tool, got %+v", list.Tools)
	}
	if list.Tools[0].Description == "" || list.Tools[0].ParametersJson == "" {
		t.Fatalf("notify 工具缺 description/schema: %+v", list.Tools[0])
	}

	// 5. 工具执行：默认 success 音效（入队即返回，不依赖音频设备）
	execTool := func(args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: "notify", ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool(notify, %s): %v", args, err)
		}
		return resp
	}
	if resp := execTool(`{}`); resp.Error != "" || !strings.Contains(resp.Content, "success") {
		t.Fatalf("默认 success 执行 = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "card" || v.Title != "Notify" || v.Badge == nil || v.Badge.Tone != "green" {
		t.Fatalf("success view = %+v", v)
	}
	if resp := execTool(`{"type":"warning"}`); resp.Error != "" || !strings.Contains(resp.Content, "warning") {
		t.Fatalf("warning 执行 = %+v", resp)
	} else if v := assertView(t, resp); v.Badge.Tone != "yellow" {
		t.Fatalf("warning view = %+v", v)
	}
	// 未知音效类型应报错（错误经响应透传，而非 RPC 错误）
	if resp := execTool(`{"type":"boom"}`); resp.Error == "" {
		t.Fatalf("未知音效类型应报错: %+v", resp)
	}
	// 自定义文件不存在应报错
	if resp := execTool(`{"file":"/nonexistent/notify.mp3"}`); resp.Error == "" {
		t.Fatalf("不存在的文件应报错: %+v", resp)
	}
}

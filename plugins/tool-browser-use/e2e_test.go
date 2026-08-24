package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证 SDK 重写后的插件能被宿主侧正常拉起并调用：
// 以 go-plugin 客户端 spawn 本插件 exe，经 gRPC 验证元数据、工具目录（5 个浏览器
// 工具）与空钩子（SDK 默认注册 PluginHookService）。浏览器实际操作依赖本机
// chromium，不在 SDK 范畴内验证。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-browser-use.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程
	cmd := exec.Command(exe)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  plugin.Handshake,
		Plugins:          map[string]goplugin.Plugin{},
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Cmd:              cmd,
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		t.Fatalf("unexpected client type %T", rpcClient)
	}
	conn := grpcClient.Conn
	ctx := context.Background()

	// 3. 元数据（SDK 自动提供）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "tool" || info.Name != "browser-use" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 4. 工具目录（SDK 自动聚合 5 个浏览器工具）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) != 5 {
		t.Fatalf("expected 5 tools, got %d: %+v", len(list.Tools), list.Tools)
	}
	names := map[string]bool{}
	for _, tl := range list.Tools {
		names[tl.Name] = true
		if tl.Description == "" || tl.ParametersJson == "" {
			t.Fatalf("tool %s 缺 description/schema", tl.Name)
		}
	}
	for _, want := range []string{"fetch_url", "web_search", "browser_click", "browser_type", "browser_screenshot"} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}

	// 5. 空钩子：宿主调用无副作用（SDK 默认注册 PluginHookService）
	hook := proto.NewPluginHookServiceClient(conn)
	bt, err := hook.BeforeTool(ctx, &proto.BeforeToolRequest{ToolName: "fetch_url", ArgumentsJson: `{"url":"https://example.com"}`})
	if err != nil || bt.Veto || bt.ArgumentsJson != `{"url":"https://example.com"}` {
		t.Fatalf("BeforeTool(空钩子) = %+v, err %v", bt, err)
	}
}

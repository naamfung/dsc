package main

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证 SDK 重写后的插件能被宿主侧正常拉起并调用：
// 以 go-core 客户端 spawn 本插件 exe，经 gRPC 验证元数据、工具目录，并真实
// 执行 create/view 工具（workspace 根经 DSC_WORKSPACE_ROOT 注入）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-str-replace-editor.exe")
	if out, err := osexec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 准备 workspace 根
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	// 3. 以宿主侧客户端拉起插件进程（注入 DSC_WORKSPACE_ROOT）
	cmd := osexec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_WORKSPACE_ROOT="+ws)
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

	// 4. 元数据（SDK 自动提供）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "tool" || info.Name != "str_replace_editor" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. 工具目录（SDK 自动聚合 1 个工具）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "str_replace_editor" {
		t.Fatalf("ListTools = %+v", list.Tools)
	}

	// 6. 真实执行：create → view（写文件到 workspace 根，验证路径与内容）
	run := func(args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: "str_replace_editor", ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool: %v", err)
		}
		return resp
	}
	if resp := run(`{"command":"create","path":"/workspace/e2e.txt","file_text":"hello e2e"}`); resp.Error != "" || !strings.Contains(resp.Content, "File created successfully.") {
		t.Fatalf("create = %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(ws, "e2e.txt")); err != nil {
		t.Fatalf("e2e.txt 未创建于 workspace 根: %v", err)
	}
	if resp := run(`{"command":"view","path":"/workspace/e2e.txt"}`); resp.Error != "" || resp.Content != "hello e2e" {
		t.Fatalf("view = %+v", resp)
	}
}

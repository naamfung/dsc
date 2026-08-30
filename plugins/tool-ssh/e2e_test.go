package main

import (
	"context"
	"encoding/json"
	osexec "os/exec"
	"path/filepath"
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

// TestE2EWithHostClient 端到端验证 tool-ssh 能被宿主侧正常拉起并调用，且各工具结果
// 的 ViewJson 穿透完整 gRPC 链路。ssh_list/ssh_close 不依赖真实 SSH 服务器；
// ssh_connect 以本地关闭端口触发确定性失败，验证失败态卡片。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-ssh.exe")
	if out, err := osexec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程
	cmd := osexec.Command(exe)
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
	if err != nil || info.Type != "tool" || info.Name != "ssh" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 4. 工具目录（SDK 自动聚合 4 个工具）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"ssh_connect", "ssh_exec", "ssh_list", "ssh_close"} {
		if !names[want] {
			t.Fatalf("missing tool %s, got %+v", want, list.Tools)
		}
	}

	// 5. 真实执行（均无需真实 SSH 服务器）
	run := func(tool, args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: tool, ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool(%s): %v", tool, err)
		}
		return resp
	}

	// ssh_list：空会话 → 表格视图（列定义 + "0" 徽标）
	if resp := run("ssh_list", `{}`); resp.Error != "" {
		t.Fatalf("ssh_list = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "table" || v.Title != "SSH Sessions" || v.Badge == nil || v.Badge.Text != "0" || len(v.Columns) != 1 {
		t.Fatalf("ssh_list view = %+v", v)
	}

	// ssh_connect：本地关闭端口 → 失败态卡片（badge=failed red，error 字段）
	if resp := run("ssh_connect", `{"host":"127.0.0.1","port":1,"username":"nobody"}`); resp.Error != "" {
		t.Fatalf("ssh_connect = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "card" || v.Title != "SSH" || v.Badge == nil || v.Badge.Text != "failed" || v.Badge.Tone != "red" || len(v.Fields) < 1 {
		t.Fatalf("ssh_connect view = %+v", v)
	}

	// ssh_close：不存在的会话 → 卡片徽标 not found
	if resp := run("ssh_close", `{"session_id":"ghost"}`); resp.Error != "" {
		t.Fatalf("ssh_close = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "card" || v.Title != "SSH" || v.Badge == nil || v.Badge.Text != "not found" || v.Fields[0].Value != "ghost" {
		t.Fatalf("ssh_close view = %+v", v)
	}
}

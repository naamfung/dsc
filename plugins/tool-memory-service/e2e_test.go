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

// TestE2EWithHostClient 端到端验证 SDK 化后的记忆服务插件能被宿主侧正常拉起并调用：
// 以 go-plugin 客户端 spawn 本插件 exe，经 gRPC 验证元数据、工具目录，并真实执行
// memory_add → memory_search（记忆库经 DSC_WORKSPACE_ROOT 落到工作区根）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-memory-service.exe")
	if out, err := osexec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 准备 workspace 根（记忆库落点）
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
	if err != nil || info.Type != "tool" || info.Name != "memory-service" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. 工具目录（SDK 自动聚合 memory_search + memory_add）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	if len(list.Tools) != 2 || !names["memory_search"] || !names["memory_add"] {
		t.Fatalf("ListTools = %+v", list.Tools)
	}

	// 6. 真实执行：memory_add → memory_search
	run := func(tool, args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: tool, ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool(%s): %v", tool, err)
		}
		return resp
	}
	if resp := run("memory_add", `{"content":"用户偏好：用粤语交流","source":"user"}`); resp.Error != "" {
		t.Fatalf("memory_add = %+v", resp)
	}
	if resp := run("memory_search", `{"query":"粤语"}`); resp.Error != "" || !strings.Contains(resp.Content, "粤语") {
		t.Fatalf("memory_search = %+v", resp)
	}

	// 7. 记忆库落点校验：DSC_WORKSPACE_ROOT/memory.db
	if _, err := os.Stat(filepath.Join(ws, "memory.db")); err != nil {
		t.Fatalf("memory.db 未创建于 workspace 根: %v", err)
	}
}

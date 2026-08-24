package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证 SDK 重写后的插件能被宿主侧正常拉起并调用：
// 以 go-plugin 客户端 spawn 本插件 exe，经 gRPC 验证元数据、工具目录、shell 执行
// 与 workspace 根（DSC_WORKSPACE_ROOT 注入后 pwd 返回该目录）。
func TestE2EWithHostClient(t *testing.T) {
	dir := t.TempDir()

	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	exe := filepath.Join(dir, "tool-filesystem.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 准备 workspace 根目录，并以其作为 shell 工作目录
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	// 3. 以宿主侧客户端拉起插件进程（注入 DSC_WORKSPACE_ROOT）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_WORKSPACE_ROOT="+ws)
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

	// 4. 元数据（SDK 自动提供）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "tool" || info.Name != "filesystem" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. 工具目录（SDK 自动聚合 1 个 shell 工具）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "shell" {
		t.Fatalf("ListTools = %+v", list.Tools)
	}
	if list.Tools[0].Description == "" || list.Tools[0].ParametersJson == "" {
		t.Fatalf("shell 缺 description/schema: %+v", list.Tools[0])
	}

	// 6. shell 执行：pwd 应返回注入的 workspace 根（对齐「shell 默认以 workspace 根为工作目录」）
	run := func(args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: "shell", ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool(shell): %v", err)
		}
		return resp
	}
	if resp := run(`{"command":"pwd"}`); resp.Error != "" {
		t.Fatalf("pwd err = %+v", resp)
	} else if got := strings.TrimSpace(resp.Content); filepath.Clean(got) != filepath.Clean(ws) {
		t.Fatalf("pwd = %q（应返回 workspace 根 %s）", got, ws)
	}
	// echo 输出与退出码
	if resp := run(`{"command":"echo hello"}`); resp.Error != "" || !strings.Contains(resp.Content, "hello") {
		t.Fatalf("echo = %+v", resp)
	}
	// 非零退出码：结果应带 [exit_code: N]
	if resp := run(`{"command":"exit 3"}`); resp.Error != "" || !strings.Contains(resp.Content, "[exit_code: 3]") {
		t.Fatalf("exit 3 = %+v", resp)
	}

	// 7. 空钩子：宿主调用无副作用（SDK 默认注册 PluginHookService）
	hook := proto.NewPluginHookServiceClient(conn)
	bt, err := hook.BeforeTool(ctx, &proto.BeforeToolRequest{ToolName: "shell", ArgumentsJson: `{"command":"pwd"}`})
	if err != nil || bt.Veto || bt.ArgumentsJson != `{"command":"pwd"}` {
		t.Fatalf("BeforeTool(空钩子) = %+v, err %v", bt, err)
	}
}

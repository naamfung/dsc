package main

import (
	"context"
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
// 以 go-plugin 客户端（宿主 Manager 同款协议路径）spawn 本插件 exe，经 gRPC
// 验证元数据、工具目录与工具执行（lisp 表达式求值）。
func TestE2EWithHostClient(t *testing.T) {
	dir := t.TempDir()

	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	exe := filepath.Join(dir, "tool-lisp-eval.exe")
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
	if err != nil || info.Type != "tool" || info.Name != "lisp-eval" || info.Version == "" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 4. 工具目录（SDK 自动聚合 1 个工具）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "lisp_eval" {
		t.Fatalf("ListTools = %+v", list.Tools)
	}
	if list.Tools[0].Description == "" || list.Tools[0].ParametersJson == "" {
		t.Fatalf("lisp_eval 缺 description/schema: %+v", list.Tools[0])
	}

	// 5. 工具执行：精确整数求值
	run := func(expr string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{
			ToolName:      "lisp_eval",
			ArgumentsJson: `{"expression":"` + expr + `"}`,
		})
		if err != nil {
			t.Fatalf("ExecuteTool(%s): %v", expr, err)
		}
		return resp
	}
	if resp := run("(+ 1 2)"); resp.Error != "" || !strings.Contains(resp.Content, "3") {
		t.Fatalf("(+ 1 2) = %+v", resp)
	}
	if resp := run("(* 3 4)"); resp.Error != "" || !strings.Contains(resp.Content, "12") {
		t.Fatalf("(* 3 4) = %+v", resp)
	}
	if resp := run("(sum (range 10))"); resp.Error != "" || !strings.Contains(resp.Content, "45") {
		t.Fatalf("(sum (range 10)) = %+v", resp)
	}
	if resp := run("(sqrt 16)"); resp.Error != "" || !strings.Contains(resp.Content, "4") {
		t.Fatalf("(sqrt 16) = %+v", resp)
	}
	// 非法表达式应报错
	if resp := run("(unknown-fn 1)"); resp.Error == "" {
		t.Fatalf("未知函数应报错: %+v", resp)
	}

	// 6. 空钩子：宿主调用无副作用（SDK 默认注册 PluginHookService）
	hook := proto.NewPluginHookServiceClient(conn)
	bt, err := hook.BeforeTool(ctx, &proto.BeforeToolRequest{ToolName: "lisp_eval", ArgumentsJson: `{"expression":"(+ 1 2)"}`})
	if err != nil || bt.Veto || bt.ArgumentsJson != `{"expression":"(+ 1 2)"}` {
		t.Fatalf("BeforeTool(空钩子) = %+v, err %v", bt, err)
	}
}

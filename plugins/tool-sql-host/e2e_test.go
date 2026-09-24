package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	dsc "dsc-sdk"
	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
	"tool-sql-host/internal/dsp"
)

// TestE2EWithHostClient 端到端验证 SDK 装配后的插件能被宿主侧正常拉起：
// 以 go-core 客户端（宿主 Manager 同款协议路径）spawn 本插件 exe，经 gRPC 验证
// 元数据、.dsp 加载（SetInterconnect 握手）、工具目录与工具执行。
//
// 覆盖的核心链路：.dsp 文件 → 宿主加载（dsp.Open + Lua VM）→ dsc.register_tool →
// ToolProvider → SDK ToolService → 宿主侧 gRPC 客户端。中间任一处字段/契约漂移，
// 模型用 .dsp 造插件的能力就会静默失效，故用真实进程钉死。
func TestE2EWithHostClient(t *testing.T) {
	dir := t.TempDir()

	// 插件目录约定：<cwd>/plugins/tool-sql-host/dsp/*.dsp（随包示例的布局，
	// 即 builder 把插件目录下的 dsp/ 拷进发布包后的样子）；<cwd>/dsp 由 host 单测覆盖。
	pluginDir := filepath.Join(dir, "plugins", "tool-sql-host", "dsp")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := dsp.Pack(filepath.Join("examples", "hello"),
		filepath.Join(pluginDir, "hello.dsp"), nil); err != nil {
		t.Fatalf("pack example: %v", err)
	}

	// 构建插件 exe（独立 module 的完整独立开发者路径）
	exe := filepath.Join(dir, "tool-sql-host.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	cmd := exec.Command(exe)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DSC_MODE=creation")
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

	// 1. 元数据（SDK 自动提供）
	info, err := metadata.NewPluginMetadataClient(conn).GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "tool" || info.Name != "tool-sql-host" || info.Version == "" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	tc := proto.NewToolServiceClient(conn)

	// 2. 握手前：只应有静态工具（.dsp 尚未加载）
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools(握手前): %v", err)
	}
	if !hasTool(list, listToolName) || !hasTool(list, packToolName) {
		t.Fatalf("握手前应已有 %s/%s，实际 %v", listToolName, packToolName, toolNameList(list))
	}
	if hasTool(list, "sql_hello") {
		t.Fatal("握手前不应加载 .dsp 插件")
	}

	// 3. SetInterconnect 触发 .dsp 加载（宿主同款握手）
	if _, err := tc.SetInterconnect(ctx, &proto.InterconnectRequest{}); err != nil {
		t.Fatalf("SetInterconnect: %v", err)
	}
	list, err = tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, want := range []string{listToolName, packToolName, "sql_hello", "sql_note", "sql_stats"} {
		if !hasTool(list, want) {
			t.Fatalf("工具目录缺 %s，实际 %v", want, toolNameList(list))
		}
	}

	// 4. 执行 .dsp 内的工具：脚本注册 → ToolProvider → SDK ToolService 全链路。
	// 用 sql_note（自带表读写，不依赖宿主互通服务）——本测试宿主侧不挂 Notify 服务，
	// 而 dsc.notify.emit 在未注入时按设计硬报错（事件通知链路由 host 单测以替身覆盖）。
	resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{
		ToolName: "sql_note", ArgumentsJson: `{"text":"e2e"}`,
	})
	if err != nil {
		t.Fatalf("ExecuteTool(sql_note): %v", err)
	}
	if resp.Error != "" || !strings.Contains(resp.Content, "e2e") || !strings.Contains(resp.Content, "共 1 条") {
		t.Fatalf("sql_note 结果 = %+v", resp)
	}

	// 5. list_sql_plugins：插件的路径/语言/工具数如实反映
	resp, err = tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{
		ToolName: listToolName, ArgumentsJson: `{}`,
	})
	if err != nil || resp.Error != "" {
		t.Fatalf("ExecuteTool(%s) = %+v, err %v", listToolName, resp, err)
	}
	var payload struct {
		Count   int `json:"count"`
		Plugins []struct {
			Name     string   `json:"name"`
			Path     string   `json:"path"`
			Language string   `json:"language"`
			Tools    []string `json:"tools"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &payload); err != nil {
		t.Fatalf("list_sql_plugins 结果非法: %v (%s)", err, resp.Content)
	}
	if payload.Count != 1 || payload.Plugins[0].Name != "hello" || payload.Plugins[0].Language != "lua" {
		t.Fatalf("list_sql_plugins = %s", resp.Content)
	}
	if !strings.HasSuffix(payload.Plugins[0].Path, "hello.dsp") || len(payload.Plugins[0].Tools) != 3 {
		t.Fatalf("list_sql_plugins 字段不符 = %+v", payload.Plugins[0])
	}
}

// TestBaseToolsGateByCreationMode 覆盖 pack_dsp 的创造模式门控：
// 它写出新的插件载体，与「插件创造仅在创造模式允许」同一边界；非创造模式不得暴露。
func TestBaseToolsGateByCreationMode(t *testing.T) {
	if !hasToolName(baseTools(true), packToolName) {
		t.Fatal("创造模式应暴露 pack_dsp")
	}
	if hasToolName(baseTools(false), packToolName) {
		t.Fatal("非创造模式不应暴露 pack_dsp")
	}
	if !hasToolName(baseTools(false), listToolName) || !hasToolName(baseTools(true), listToolName) {
		t.Fatal("list_sql_plugins 应始终可用")
	}
}

func hasTool(list *proto.ListToolsResponse, name string) bool {
	for _, tl := range list.Tools {
		if tl.Name == name {
			return true
		}
	}
	return false
}

func hasToolName(tools []dsc.Tool, name string) bool {
	for _, tl := range tools {
		if tl.Name == name {
			return true
		}
	}
	return false
}

func toolNameList(list *proto.ListToolsResponse) []string {
	out := make([]string, 0, len(list.Tools))
	for _, tl := range list.Tools {
		out = append(out, tl.Name)
	}
	return out
}

// TestPackDspConfinesToWorkspace 覆盖 pack_dsp 的路径隔离：它写出文件，故源目录与
// 产物都必须落在宿主工作区（沙箱边界）内，越界的路径一律拒绝。
func TestPackDspConfinesToWorkspace(t *testing.T) {
	old := core.WorkspaceRoot
	t.Cleanup(func() { core.WorkspaceRoot = old })
	core.WorkspaceRoot = t.TempDir()

	if err := insideWorkspace(core.WorkspaceRoot + "/dsp/x.dsp"); err != nil {
		t.Fatalf("工作区内路径应通过: %v", err)
	}
	if err := insideWorkspace(core.WorkspaceRoot + "/../escape.dsp"); err == nil {
		t.Fatal("越出工作区的路径应被拒绝")
	}
	if err := insideWorkspace("/etc/passwd.dsp"); err == nil {
		t.Fatal("绝对外部路径应被拒绝")
	}
}

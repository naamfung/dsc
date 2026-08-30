package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// 本文件覆盖「视图链路」在宿主聚合层的衔接：真实工具插件 → RemoteTool →
// ToolGRPCServer → ExecuteToolResponse。此前测试只覆盖了插件直连（ViewJson 产生）
// 与 TUI 渲染器（视图 spec 绘制），唯独宿主聚合这一跳未接真实插件，导致
// RemoteTool.Execute 静默丢弃 ViewJson 未被发现。此处用真实插件进程补齐，
// 并对适配器做字段保真往返断言，避免同类「跨层丢字段」回归。

// buildToolBin 编译工具插件二进制到 out（go build 于插件目录执行，独立 module）。
func buildToolBin(t *testing.T, pluginDir, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = pluginDir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pluginDir, err, b)
	}
}

// TestHostToolChainPropagatesPluginViewJson 验证真实工具插件经宿主聚合链路执行时，
// 插件 Tool.ViewFn 声明的 ViewJson 必须穿透到 ExecuteToolResponse（agent 实际走
// ToolGRPCServer，这正是视图特性在真实运行的关键衔接点）。
func TestHostToolChainPropagatesPluginViewJson(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-filesystem.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "tool-filesystem"), exe)

	// 1. 以宿主侧客户端 spawn 真实工具插件（与生产 loadPluginWithBroker 同路径：
	// 经 grpcClient.Conn 直接建 ToolServiceClient，而非 Dispense 具名插件）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_WORKSPACE_ROOT="+dir)
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
	toolClient := proto.NewToolServiceClient(grpcClient.Conn)

	// 2. 复用生产 RemoteTool 构造（listStagedTools 即 stageToolPlugin 的清单路径）
	defs, _, err := listStagedTools(toolClient)
	if err != nil {
		t.Fatalf("listStagedTools: %v", err)
	}
	if len(defs) != 1 || defs[0].Name() != "shell" {
		t.Fatalf("staged tools = %+v", defs)
	}
	m := NewManager(&ManagerConfig{ExecDir: dir})
	if err := m.toolRegistry.Register(defs[0]); err != nil {
		t.Fatal(err)
	}

	// 3. 经宿主聚合 Tool 服务执行 shell（agent 实际走的路径）
	srv := NewToolGRPCServer(m)
	resp, err := srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{
		ToolName: "shell", ArgumentsJson: `{"command":"echo hello"}`,
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if resp.Error != "" || !strings.Contains(resp.Content, "hello") {
		t.Fatalf("resp = %+v", resp)
	}
	// 关键断言：插件 ViewFn 的 ViewJson 必须穿透聚合层（曾在此被丢弃）
	if resp.ViewJson == "" {
		t.Fatal("插件 ViewJson 未穿透宿主聚合链路（RemoteTool/聚合层丢弃）")
	}
	var v ToolView
	if err := json.Unmarshal([]byte(resp.ViewJson), &v); err != nil {
		t.Fatalf("ViewJson 非法: %v", err)
	}
	if v.Kind != "plain" || v.Title != "Shell" || v.Badge == nil || v.Badge.Text != "exit 0" || v.Badge.Tone != "green" {
		t.Fatalf("view = %+v", v)
	}
}

// stubToolClient 实现 proto.ToolServiceClient，注入预置响应以验证适配器字段保真。
type stubToolClient struct {
	proto.ToolServiceClient
	resp *proto.ExecuteToolResponse
}

func (s *stubToolClient) ExecuteTool(ctx context.Context, in *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	return s.resp, nil
}

// TestRemoteToolRoundTripPreservesFields 验证 RemoteTool 适配器字段保真：
// ExecuteWithView 必须同时返回 Content 与 ViewJson（曾只返回 Content 丢弃 ViewJson）；
// Execute 返回 Content。防止同类「适配器跨层丢字段」回归。
func TestRemoteToolRoundTripPreservesFields(t *testing.T) {
	pluginResp := &proto.ExecuteToolResponse{
		Content:  `{"ok":true}`,
		ViewJson: `{"kind":"card","title":"T","badge":{"text":"ok","tone":"green"}}`,
	}
	rt := &RemoteTool{name: "t", description: "d", schema: json.RawMessage(`{"type":"object"}`), client: &stubToolClient{resp: pluginResp}}

	content, view, err := rt.ExecuteWithView(context.Background(), json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("ExecuteWithView: %v", err)
	}
	if content != pluginResp.Content || view != pluginResp.ViewJson {
		t.Fatalf("ExecuteWithView 丢字段: content=%q view=%q", content, view)
	}

	content, err = rt.Execute(context.Background(), json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if content != pluginResp.Content {
		t.Fatalf("Execute 丢 Content: %q", content)
	}
}

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

// TestE2EWithHostClient 端到端验证 SDK 重写后的插件能被宿主侧正常拉起并调用：
// 以 go-core 客户端（宿主 Manager 同款协议路径）spawn 本插件 exe，经 gRPC
// 验证元数据、工具目录、上下文索引与工具执行（read/install/uninstall）。
func TestE2EWithHostClient(t *testing.T) {
	dir := t.TempDir()

	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	exe := filepath.Join(dir, "tool-skill.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 准备技能目录：内置 git-commit + 外置 flat-skill + 可安装候选 pkg-new
	skillsDir := filepath.Join(dir, "skills")
	builtin := filepath.Join(skillsDir, "builtin", "git-commit")
	if err := os.MkdirAll(builtin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(builtin, "SKILL.md"), "---\nname: git-commit\ndescription: 内置技能\n---\n正文内置\n")
	installed := filepath.Join(skillsDir, "installed", "flat-skill")
	writeTestFile(t, filepath.Join(installed, "SKILL.md"), "---\nname: flat-skill\ndescription: 外置技能\n---\n正文 B\n")
	candDir := filepath.Join(dir, "candidates", "pkg-new")
	writeTestFile(t, filepath.Join(candDir, "SKILL.md"), "---\nname: pkg-new\ndescription: 待安装技能\n---\n正文 C\n")

	// 3. 以宿主侧客户端拉起插件进程
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_SKILLS_DIR="+skillsDir)
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
	if err != nil || info.Type != "tool" || info.Name != "skill" || info.Version == "" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. 工具目录（SDK 自动聚合 3 个工具）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d: %+v", len(list.Tools), list.Tools)
	}
	names := map[string]bool{}
	for _, tl := range list.Tools {
		names[tl.Name] = true
		if tl.Description == "" || tl.ParametersJson == "" {
			t.Fatalf("tool %s 缺 description/schema", tl.Name)
		}
	}
	for _, want := range []string{"read_skill", "install_skill", "uninstall_skill"} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}

	// 6. 上下文索引（技能索引注入 system prompt）
	lc, err := tc.ListContext(ctx, &proto.ListContextRequest{})
	if err != nil {
		t.Fatalf("ListContext: %v", err)
	}
	if !strings.Contains(lc.Content, "git-commit") || !strings.Contains(lc.Content, "flat-skill") {
		t.Fatalf("ListContext 应含技能索引: %q", lc.Content)
	}

	// 7. 工具执行：read_skill
	execTool := func(name, args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: name, ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool(%s): %v", name, err)
		}
		return resp
	}
	if resp := execTool("read_skill", `{"name":"flat-skill"}`); resp.Error != "" || !strings.Contains(resp.Content, "正文 B") {
		t.Fatalf("read_skill = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "plain" || v.Title != "Skill" || v.Badge == nil || v.Badge.Text != "flat-skill" || !strings.Contains(v.Body, "正文 B") {
		t.Fatalf("read_skill view = %+v", v)
	}
	if resp := execTool("read_skill", `{"name":"git-commit"}`); resp.Error != "" || !strings.Contains(resp.Content, "正文内置") {
		t.Fatalf("read_skill builtin = %+v", resp)
	} else if v := assertView(t, resp); v.Badge.Text != "git-commit" {
		t.Fatalf("read_skill builtin view = %+v", v)
	}

	// 8. 安装新技能 → 立即可读，且技能索引动态更新（ContextFn 每调用重算）
	if resp := execTool("install_skill", `{"path":"`+filepath.ToSlash(candDir)+`"}`); resp.Error != "" || !strings.Contains(resp.Content, "pkg-new") {
		t.Fatalf("install_skill = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "card" || v.Badge == nil || v.Badge.Text != "1 installed" || v.Fields[0].Value != "pkg-new" {
		t.Fatalf("install_skill view = %+v", v)
	}
	if resp := execTool("read_skill", `{"name":"pkg-new"}`); resp.Error != "" || !strings.Contains(resp.Content, "正文 C") {
		t.Fatalf("read_skill pkg-new = %+v", resp)
	}
	lc2, err := tc.ListContext(ctx, &proto.ListContextRequest{})
	if err != nil || !strings.Contains(lc2.Content, "pkg-new") {
		t.Fatalf("安装后 ListContext 应含新技能（动态索引）: %q, err %v", lc2.Content, err)
	}

	// 9. 卸载 → 不再可读
	if resp := execTool("uninstall_skill", `{"name":"pkg-new"}`); resp.Error != "" {
		t.Fatalf("uninstall_skill = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "card" || v.Badge == nil || v.Badge.Text != "uninstalled" || v.Fields[0].Value != "pkg-new" {
		t.Fatalf("uninstall_skill view = %+v", v)
	}
	if resp := execTool("read_skill", `{"name":"pkg-new"}`); resp.Error == "" {
		t.Fatalf("卸载后 read_skill 应报错: %+v", resp)
	}

	// 10. 内置技能不可卸载
	if resp := execTool("uninstall_skill", `{"name":"git-commit"}`); resp.Error == "" {
		t.Fatalf("内置技能卸载应被拒绝: %+v", resp)
	}

	// 11. 钩子空实现：宿主调用无副作用（SDK 默认注册 PluginHookService）
	hook := proto.NewPluginHookServiceClient(conn)
	bt, err := hook.BeforeTool(ctx, &proto.BeforeToolRequest{ToolName: "read_skill", ArgumentsJson: `{"name":"flat-skill"}`})
	if err != nil || bt.Veto || bt.ArgumentsJson != `{"name":"flat-skill"}` {
		t.Fatalf("BeforeTool(空钩子) = %+v, err %v", bt, err)
	}
}

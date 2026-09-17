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

// TestE2EWithHostClient 端到端验证核心插件混合体的通用（dsc）类型 + 服务正交
// 形态：以宿主侧 go-core 客户端 spawn exe，经 gRPC 验证元数据（type=dsc，
// services 声明含 "policy" —— 宿主 registerDscCoreLocked 据此机械桥接工具流水线），
// 并经主连接直接调用 PolicyService.OnEvent 与 PluginHookService.OnEvent，覆盖
// 重复提醒阈值升级、用户插话重置（pre-step 事件）与 advisory allow 裁决。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "dsc-system.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程（同 registerDscCoreLocked 的主连接路径）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"DSC_REPEAT_THRESHOLDS=3,5",
		"DSC_REPEAT_EXCLUDE=todo_write",
	)
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

	// 3. 元数据：type=dsc + services 声明（服务正交的关键载体）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.GetType() != "dsc" || info.GetName() != "dsc-system" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}
	var hasPolicy bool
	for _, s := range info.GetServices() {
		if s == "policy" {
			hasPolicy = true
		}
	}
	if !hasPolicy {
		t.Fatalf("services must declare \"policy\" for host bridging, got %v", info.GetServices())
	}

	// 4. PolicyService：阈值升级（3 简短 / 5 详细）与重置
	policy := proto.NewPolicyServiceClient(conn)
	postExec := func(session, tool, args string) *proto.PolicyDecision {
		t.Helper()
		dec, err := policy.OnEvent(ctx, &proto.PolicyEvent{
			Kind: "tool/post-execute", Tool: tool, ArgumentsJson: args, Session: session,
		})
		if err != nil {
			t.Fatalf("OnEvent: %v", err)
		}
		return dec
	}
	if n := postExec("s1", "shell", `{"cmd":"ls"}`).GetNotice(); n != "" {
		t.Fatalf("call 1 must be silent, got %q", n)
	}
	if n := postExec("s1", "shell", `{"cmd":"ls"}`).GetNotice(); n != "" {
		t.Fatalf("call 2 must be silent, got %q", n)
	}
	third := postExec("s1", "shell", `{"cmd":"ls"}`).GetNotice()
	if !strings.Contains(third, "repeating the exact same tool call") {
		t.Fatalf("call 3 should be gentle, got %q", third)
	}
	if n := postExec("s1", "shell", `{"cmd":"ls"}`).GetNotice(); n != "" {
		t.Fatalf("call 4 must be silent (between thresholds), got %q", n)
	}
	detailed := postExec("s1", "shell", `{"cmd":"ls"}`)
	// advisory 形态：只产 notice，不占决策槽（action 恒空 = allow）
	if detailed.GetAction() != "" {
		t.Fatalf("advisory decision must be allow (empty action), got %q", detailed.GetAction())
	}
	if !strings.Contains(detailed.GetNotice(), "consecutive_calls: 5") {
		t.Fatalf("call 5 should be detailed with count 5, got %q", detailed.GetNotice())
	}

	// 5. HookService：pre-step「新用户输入」重置重复链（宿主 dispatchEventToPlugins 同款调用）
	hook := proto.NewPluginHookServiceClient(conn)
	if _, err := hook.OnEvent(ctx, &proto.OnEventRequest{
		Name: "agent/pre-step", DataJson: `{"agent":"agent-react-loop","session":"s1","user_input":true}`,
	}); err != nil {
		t.Fatalf("hook OnEvent: %v", err)
	}
	if n := postExec("s1", "shell", `{"cmd":"ls"}`).GetNotice(); n != "" {
		t.Fatalf("chain must restart after user interjection, got %q", n)
	}
}

// TestFsObservationE2E 端到端验证 fs-observation 驻留策略（自
// plugins/policy-fs-observation 迁入）：spawn dsc-system exe，经 gRPC 验证
// 混合体元数据（type=dsc + services 含 policy）与 PolicyService 读前改写、
// sha256 新鲜度、缺失记录与 per-session 属主语义（宿主 policy 桥接同款路径）。
func TestFsObservationE2E(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "dsc-system.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程：注入 DSC_WORKSPACE_ROOT 对齐宿主加载
	//    行为（插件进程经 core.WorkspaceRoot 解析相对路径观察键）
	ws := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
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

	// 3. 元数据：混合体恒为通用 dsc 类型 + services 声明含 policy
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "dsc" || info.Name != "dsc-system" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}
	var hasPolicy bool
	for _, s := range info.GetServices() {
		if s == "policy" {
			hasPolicy = true
		}
	}
	if !hasPolicy {
		t.Fatalf("services must declare \"policy\", got %v", info.GetServices())
	}

	// 4. 经主连接调用 PolicyService（宿主 policy 桥接同款路径）
	pc := proto.NewPolicyServiceClient(conn)
	file := filepath.Join(ws, "a.go")
	args := func(command string) string {
		return `{"command":"` + command + `","path":"` + filepath.ToSlash(file) + `"}`
	}
	pre := func(command, session string) (*proto.PolicyDecision, error) {
		return pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/pre-execute", Tool: "str_replace_editor", ArgumentsJson: args(command), Session: session})
	}
	post := func(command, session, toolErr string) (*proto.PolicyDecision, error) {
		return pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/post-execute", Tool: "str_replace_editor", ArgumentsJson: args(command), Session: session, Error: toolErr})
	}

	// 4.1 未读先改（str_replace）→ deny（读前改写）
	dec, err := pre("str_replace", "s1")
	if err != nil || dec.GetAction() != "deny" || !strings.Contains(dec.GetReason(), "has not been read") {
		t.Fatalf("未读先改应 deny: dec=%+v err=%v", dec, err)
	}

	// 4.2 非守护工具 / 未知命令 → 放行（空裁决）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/pre-execute", Tool: "shell", ArgumentsJson: args("str_replace"), Session: "s1"}); err != nil || dec.GetAction() != "" {
		t.Fatalf("非守护工具应放行: dec=%+v err=%v", dec, err)
	}
	if dec, err = pre("create", "s1"); err != nil || dec.GetAction() != "" {
		t.Fatalf("create 不受读前约束（工具自身保证不可覆盖）: dec=%+v err=%v", dec, err)
	}

	// 4.3 view 成功 → 记录 present；str_replace → 放行
	if err := os.WriteFile(file, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = post("view", "s1", ""); err != nil {
		t.Fatalf("post view: %v", err)
	}
	if dec, err = pre("str_replace", "s1"); err != nil || dec.GetAction() == "deny" {
		t.Fatalf("读后改写应放行: dec=%+v err=%v", dec, err)
	}

	// 4.4 观察后外部修改 → deny（sha256 新鲜度）
	if err := os.WriteFile(file, []byte("package a // changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dec, err = pre("str_replace", "s1"); err != nil || dec.GetAction() != "deny" || !strings.Contains(dec.GetReason(), "has changed") {
		t.Fatalf("过期观察应 deny: dec=%+v err=%v", dec, err)
	}

	// 4.5 per-session 属主隔离：s2 无观察记录，须重新读取
	if dec, err = pre("str_replace", "s2"); err != nil || dec.GetAction() != "deny" || !strings.Contains(dec.GetReason(), "has not been read") {
		t.Fatalf("跨会话应视为未读: dec=%+v err=%v", dec, err)
	}

	// 4.6 缺失记录：view 报 not found（tool error 非空）→ confirmed absent；
	//     同路径 create 放行（缺失语义），str_replace 拦截（不能编辑不存在的文件）
	missing := filepath.Join(ws, "absent.txt")
	absentArgs := `{"command":"view","path":"` + filepath.ToSlash(missing) + `"}`
	if _, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/post-execute", Tool: "str_replace_editor", ArgumentsJson: absentArgs, Session: "s1", Error: "no such file"}); err != nil {
		t.Fatalf("post view(缺失): %v", err)
	}
	createArgs := `{"command":"create","path":"` + filepath.ToSlash(missing) + `"}`
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/pre-execute", Tool: "str_replace_editor", ArgumentsJson: createArgs, Session: "s1"}); err != nil || dec.GetAction() == "deny" {
		t.Fatalf("缺失路径 create 应放行: dec=%+v err=%v", dec, err)
	}
	strArgs := `{"command":"str_replace","path":"` + filepath.ToSlash(missing) + `"}`
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/pre-execute", Tool: "str_replace_editor", ArgumentsJson: strArgs, Session: "s1"}); err != nil || dec.GetAction() != "deny" || !strings.Contains(dec.GetReason(), "does not exist") {
		t.Fatalf("confirmed absent 后 str_replace 应 deny: dec=%+v err=%v", dec, err)
	}

	// 4.7 观察后文件被外部删除 → deny（removed）
	if _, err = post("view", "s3", ""); err != nil {
		t.Fatalf("post view(s3): %v", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if dec, err = pre("str_replace", "s3"); err != nil || dec.GetAction() != "deny" || !strings.Contains(dec.GetReason(), "removed") {
		t.Fatalf("观察后删除应 deny: dec=%+v err=%v", dec, err)
	}
}

// TestSpillE2E 端到端验证 spill 外置策略驻留（自 plugins/policy-spill 迁入）：
// spawn dsc-system exe（env 注入 DSC_SPILL_DIR），经 gRPC 覆盖超长结果外置
// （replace + 全文落盘 + 定位符即路径）、取回豁免、失败放行与阈值禁用。
func TestSpillE2E(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "dsc-system.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	spillDir := filepath.Join(dir, "spill")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_SPILL_DIR="+spillDir)
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

	// 元数据：混合体 type=dsc（服务正交——policy 桥接对 spill 同样生效）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "dsc" || info.Name != "dsc-system" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	pc := proto.NewPolicyServiceClient(conn)
	content := longText(9000)

	// 1. 超长结果 → replace；全文落盘；定位符为绝对路径且内容逐字一致
	dec, err := pc.OnEvent(ctx, &proto.PolicyEvent{
		Kind: kindPostExecute, Tool: "shell", ArgumentsJson: `{"command":"cat big.log"}`,
		Result: content, Session: "s1",
	})
	if err != nil || dec.GetAction() != actionReplace || dec.GetResult() == "" {
		t.Fatalf("超长结果应 replace: dec=%+v err=%v", dec, err)
	}
	start := strings.Index(dec.GetResult(), "[内容已外置: ") + len("[内容已外置: ")
	locator := dec.GetResult()[start : strings.Index(dec.GetResult()[start:], "]")+start]
	if !filepath.IsAbs(locator) {
		t.Fatalf("定位符应为绝对路径: %q", locator)
	}
	got, err := os.ReadFile(locator)
	if err != nil || string(got) != content {
		t.Fatalf("外置文件应逐字保真: err=%v len=%d", err, len(got))
	}
	if strings.Contains(dec.GetResult(), content) {
		t.Fatal("完整内容不应残留于替换体")
	}

	// 2. 未达阈值 → 放行（空裁决）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: kindPostExecute, Tool: "shell", Result: "short", Session: "s1"}); err != nil || dec.GetAction() != "" {
		t.Fatalf("短结果应放行: dec=%+v err=%v", dec, err)
	}

	// 3. 取回路径豁免：view 命令超长结果不外置（防取回死循环）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{
		Kind: kindPostExecute, Tool: "str_replace_editor",
		ArgumentsJson: `{"command":"view","path":"/workspace/big.txt"}`,
		Result:        content, Session: "s1",
	}); err != nil || dec.GetAction() != "" {
		t.Fatalf("view 命令应豁免: dec=%+v err=%v", dec, err)
	}

	// 4. 失败结果放行（错误是权威观察；外置只塑造被接受的成功结果）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{
		Kind: kindPostExecute, Tool: "shell", Result: content,
		Error: "no such file", Session: "s1",
	}); err != nil || dec.GetAction() != "" {
		t.Fatalf("失败结果应放行: dec=%+v err=%v", dec, err)
	}

	// 5. 非 post-execute 槽一律放行（策略只在自己的领域发声）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/pre-execute", Tool: "shell"}); err != nil || dec.GetAction() != "" {
		t.Fatalf("pre 槽应放行: dec=%+v err=%v", dec, err)
	}
}

// TestSkillE2E 端到端验证 skill 工具驻留（自 plugins/tool-skill 迁入）：
// spawn dsc-system exe（env 注入 DSC_SKILLS_DIR），经 gRPC 验证混合体元数据
// （type=dsc + services 含 tool）、工具目录、上下文索引与工具执行
// （read/install/uninstall + ViewFn 视图透传 + 空钩子无副作用）。
func TestSkillE2E(t *testing.T) {
	dir := t.TempDir()

	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	exe := filepath.Join(dir, "dsc-system.exe")
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

	// 4. 元数据：混合体 type=dsc + services 声明含 tool（服务正交：skill 驻留叠加）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "dsc" || info.Name != "dsc-system" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}
	var hasTool bool
	for _, s := range info.GetServices() {
		if s == "tool" {
			hasTool = true
		}
	}
	if !hasTool {
		t.Fatalf("services must declare \"tool\", got %v", info.GetServices())
	}

	// 5. 工具目录（skill 驻留聚合 3 个工具）
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
	for _, want := range []string{"skill", "install_skill", "uninstall_skill"} {
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

	// 7. 工具执行：skill（对齐 DSH 工具名）
	execTool := func(name, args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: name, ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool(%s): %v", name, err)
		}
		return resp
	}
	if resp := execTool("skill", `{"name":"flat-skill"}`); resp.Error != "" || !strings.Contains(resp.Content, "正文 B") {
		t.Fatalf("skill = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "plain" || v.Title != "Skill" || v.Badge == nil || v.Badge.Text != "flat-skill" || !strings.Contains(v.Body, "正文 B") {
		t.Fatalf("skill view = %+v", v)
	}
	if resp := execTool("skill", `{"name":"git-commit"}`); resp.Error != "" || !strings.Contains(resp.Content, "正文内置") {
		t.Fatalf("skill builtin = %+v", resp)
	} else if v := assertView(t, resp); v.Badge.Text != "git-commit" {
		t.Fatalf("skill builtin view = %+v", v)
	}

	// 8. 安装新技能 → 立即可读，且技能索引动态更新（ContextFn 每调用重算）
	if resp := execTool("install_skill", `{"path":"`+filepath.ToSlash(candDir)+`"}`); resp.Error != "" || !strings.Contains(resp.Content, "pkg-new") {
		t.Fatalf("install_skill = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "card" || v.Badge == nil || v.Badge.Text != "1 installed" || v.Fields[0].Value != "pkg-new" {
		t.Fatalf("install_skill view = %+v", v)
	}
	if resp := execTool("skill", `{"name":"pkg-new"}`); resp.Error != "" || !strings.Contains(resp.Content, "正文 C") {
		t.Fatalf("skill pkg-new = %+v", resp)
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
	if resp := execTool("skill", `{"name":"pkg-new"}`); resp.Error == "" {
		t.Fatalf("卸载后 skill 应报错: %+v", resp)
	}

	// 10. 内置技能不可卸载
	if resp := execTool("uninstall_skill", `{"name":"git-commit"}`); resp.Error == "" {
		t.Fatalf("内置技能卸载应被拒绝: %+v", resp)
	}

	// 11. 钩子空实现：宿主调用无副作用（SDK 默认注册 PluginHookService）
	hook := proto.NewPluginHookServiceClient(conn)
	bt, err := hook.BeforeTool(ctx, &proto.BeforeToolRequest{ToolName: "skill", ArgumentsJson: `{"name":"flat-skill"}`})
	if err != nil || bt.Veto || bt.ArgumentsJson != `{"name":"flat-skill"}` {
		t.Fatalf("BeforeTool(空钩子) = %+v, err %v", bt, err)
	}
}

// TestCompactionBasicE2E 端到端验证基础压缩驻留（自宿主 core/compaction.go 迁入）：
// spawn dsc-system exe，经 gRPC PluginHookService.OnEvent 走宿主 agent/pre-step /
// agent/request-error 同款调用——溢出紧急压缩返回 {"retry": true}、重试的 pre-step
// 返回 {"messages": [...]} 改写（LLM 未互联 → 截断式退化路径）、非溢出错误码忽略。
func TestCompactionBasicE2E(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "dsc-system.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 拉起插件进程：小窗口（阈值 800）+ 状态目录隔离到临时区
	stateDir := filepath.Join(dir, "compaction-basic-state")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"DSC_COMPACTION_BASIC_CONTEXT_WINDOW=1000",
		"DSC_COMPACTION_BASIC_DIR="+stateDir,
	)
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
	hook := proto.NewPluginHookServiceClient(conn)

	// 3. pre-step：6 条约 300 token 的消息（共 1800 ≥ 800）——未走紧急压缩前
	//    不改写（默认保留预算 1024 未覆盖全部时不触发该分支，此处窗口 1000 下
	//    保留预算 max(160,1024)=1024 < 1800，会直接压缩；为验证紧急路径，
	//    用更低估算让首步走「未达阈值」分支不可行——改验：首步直接压缩也可，
	//    但为覆盖 request-error 路径，这里先跑 pre-step 缓存消息列表即可。
	msgs := make([]*proto.Message, 0, 6)
	for i := 0; i < 6; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, &proto.Message{Role: role, Content: strings.Repeat("x", 1200)})
	}
	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal msgs: %v", err)
	}
	preStep := func() string {
		t.Helper()
		data, err := json.Marshal(map[string]any{
			"agent": "agent-react-loop", "session": "e2e-compaction-basic",
			"messages_json": string(msgsJSON), "token_count": 1800,
		})
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		resp, err := hook.OnEvent(ctx, &proto.OnEventRequest{Name: "agent/pre-step", DataJson: string(data)})
		if err != nil {
			t.Fatalf("pre-step OnEvent: %v", err)
		}
		return resp.GetResultJson()
	}
	requestError := func(code string) string {
		t.Helper()
		resp, err := hook.OnEvent(ctx, &proto.OnEventRequest{Name: "agent/request-error",
			DataJson: `{"agent":"a","code":"` + code + `"}`})
		if err != nil {
			t.Fatalf("request-error OnEvent: %v", err)
		}
		return resp.GetResultJson()
	}

	// 4. 首步 pre-step：窗口 1000、保留预算 1024 → 保留区盖到 3 条（900），
	//    第 4 条越界 → 直接压缩 [0,3) 为截断式摘要（LLM 未互联）
	first := preStep()
	if first == "" {
		t.Fatalf("over-threshold pre-step must rewrite")
	}
	var rewritten struct {
		Messages []*proto.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(first), &rewritten); err != nil {
		t.Fatalf("parse rewrite: %v", err)
	}
	if len(rewritten.Messages) != 4 {
		t.Fatalf("rewrite = %d messages, want 4 (summary + 3 tail)", len(rewritten.Messages))
	}
	if !strings.Contains(rewritten.Messages[0].GetContent(), "[压缩摘要]") {
		t.Fatalf("head must be truncate summary (LLM 未互联), got %q", rewritten.Messages[0].GetContent())
	}
	if rewritten.Messages[3].GetContent() != msgs[5].GetContent() {
		t.Fatalf("tail must preserve last message verbatim")
	}

	// 5. 同载荷重放：指纹命中复用状态，改写确定性一致
	if again := preStep(); again != first {
		t.Fatalf("replay must be deterministic")
	}

	// 6. 紧急路径：窗口 1000 场景下 [0,3) 已压缩，request-error 压缩 [3,5)
	//    （保留最后 1 条）→ {"retry": true}
	if res := requestError("context_window_exceeded"); res != `{"retry": true}` {
		t.Fatalf("emergency must request retry, got %q", res)
	}
	// 重试的 pre-step：累计两条摘要 + 尾段
	second := preStep()
	var afterRetry struct {
		Messages []*proto.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(second), &afterRetry); err != nil {
		t.Fatalf("parse rewrite after retry: %v", err)
	}
	if len(afterRetry.Messages) != 3 {
		t.Fatalf("after emergency rewrite = %d messages, want 3 (2 summaries + last)", len(afterRetry.Messages))
	}
	if !strings.Contains(afterRetry.Messages[1].GetContent(), "emergency") {
		t.Fatalf("second summary must be emergency-marked, got %q", afterRetry.Messages[1].GetContent())
	}
	if afterRetry.Messages[2].GetContent() != msgs[5].GetContent() {
		t.Fatalf("last message must be preserved")
	}

	// 7. 非溢出错误码：忽略
	if res := requestError("rate_limited"); res != "" {
		t.Fatalf("non-overflow code must be ignored, got %q", res)
	}

	// 8. 状态落盘（per-session 状态文件存在）
	if _, err := os.Stat(filepath.Join(stateDir, "e2e-compaction-basic.json")); err != nil {
		t.Fatalf("session state file must persist: %v", err)
	}
}

// TestImageOffloadE2E 请求面图像预算卸载端到端（真实 Hook 多路复用链）：
// A) 纯卸载——压缩未触发时直接投影消息列表（占位文本 + 最旧清空）；
// B) 链式——压缩改写 [0,3) 后，卸载作用于改写结果（结构改写在前、请求面投影在后）。
func TestImageOffloadE2E(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "dsc-system.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	spawn := func(env ...string) proto.PluginHookServiceClient {
		t.Helper()
		cmd := exec.Command(exe)
		cmd.Env = append(os.Environ(), env...)
		client := plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig:  core.Handshake,
			Plugins:          map[string]plugin.Plugin{},
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Cmd:              cmd,
		})
		t.Cleanup(client.Kill)
		rpcClient, err := client.Client()
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		grpcClient, ok := rpcClient.(*plugin.GRPCClient)
		if !ok {
			t.Fatalf("unexpected client type %T", rpcClient)
		}
		return proto.NewPluginHookServiceClient(grpcClient.Conn)
	}
	preStep := func(t2 *testing.T, hook proto.PluginHookServiceClient, session string, msgs []*proto.Message, tokenCount int) string {
		t2.Helper()
		ctx := context.Background()
		msgsJSON, err := json.Marshal(msgs)
		if err != nil {
			t2.Fatalf("marshal msgs: %v", err)
		}
		data, err := json.Marshal(map[string]any{
			"agent": "agent-react-loop", "session": session,
			"messages_json": string(msgsJSON), "token_count": tokenCount,
		})
		if err != nil {
			t2.Fatalf("marshal event: %v", err)
		}
		resp, err := hook.OnEvent(ctx, &proto.OnEventRequest{Name: "agent/pre-step", DataJson: string(data)})
		if err != nil {
			t2.Fatalf("pre-step OnEvent: %v", err)
		}
		return resp.GetResultJson()
	}
	parse := func(t2 *testing.T, res string) []*proto.Message {
		t2.Helper()
		var out struct {
			Messages []*proto.Message `json:"messages"`
		}
		if err := json.Unmarshal([]byte(res), &out); err != nil {
			t2.Fatalf("parse rewrite %q: %v", res, err)
		}
		return out.Messages
	}

	// 2. 场景 A：纯卸载（预算 2，总量 4 → 最旧两张退役）
	imgA, imgB, imgC, imgD := "dsc-shot://e2e-a", "dsc-shot://e2e-b", "dsc-shot://e2e-c", "dsc-shot://e2e-d"
	msgsA := []*proto.Message{
		{Role: "user", Content: "u0", Images: []string{imgA}},
		{Role: "tool", Content: "t1", Images: []string{imgB, imgC}},
		{Role: "tool", Content: "t2", Images: []string{imgD}},
	}
	hookA := spawn("DSC_MAX_REQUEST_IMAGES=2")
	res := preStep(t, hookA, "e2e-image-offload", msgsA, 100)
	got := parse(t, res)
	if len(got) != 3 {
		t.Fatalf("A: messages = %d, want 3", len(got))
	}
	if len(got[0].Images) != 0 || !strings.Contains(got[0].Content, "[image omitted to fit request image limits; "+imgA+"]") {
		t.Fatalf("A: oldest not offloaded: %+v", got[0])
	}
	if len(got[1].Images) != 1 || got[1].Images[0] != imgC || !strings.Contains(got[1].Content, "[image omitted to fit request image limits; "+imgB+"]") {
		t.Fatalf("A: second oldest not offloaded: %+v", got[1])
	}
	if len(got[2].Images) != 1 || got[2].Images[0] != imgD || got[2].Content != "t2" {
		t.Fatalf("A: newest must stay verbatim: %+v", got[2])
	}

	// 3. 场景 B：链式——压缩窗口 1000 + 卸载预算 1；压缩先改写 [0,3) 为摘要，
	//    卸载再作用于改写结果（尾段三图退役最旧两张，最新一张保持在场）
	msgsB := make([]*proto.Message, 0, 6)
	for i := 0; i < 6; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		m := &proto.Message{Role: role, Content: strings.Repeat("x", 1200)}
		switch i {
		case 3:
			m.Images = []string{imgC}
		case 4:
			m.Images = []string{imgD}
		case 5:
			m.Images = []string{imgB}
		}
		msgsB = append(msgsB, m)
	}
	stateDir := filepath.Join(dir, "compaction-basic-state")
	hookB := spawn(
		"DSC_COMPACTION_BASIC_CONTEXT_WINDOW=1000",
		"DSC_COMPACTION_BASIC_DIR="+stateDir,
		"DSC_MAX_REQUEST_IMAGES=1",
	)
	res = preStep(t, hookB, "e2e-image-offload-chain", msgsB, 1800)
	got = parse(t, res)
	if len(got) != 4 {
		t.Fatalf("B: messages = %d, want 4 (summary + 3 tail)", len(got))
	}
	if !strings.Contains(got[0].Content, "[压缩摘要]") {
		t.Fatalf("B: head must be truncate summary, got %q", got[0].GetContent())
	}
	if !strings.Contains(got[1].Content, "[image omitted to fit request image limits; "+imgC+"]") || len(got[1].Images) != 0 {
		t.Fatalf("B: msg3 image must be offloaded: %+v", got[1])
	}
	if !strings.Contains(got[2].Content, "[image omitted to fit request image limits; "+imgD+"]") || len(got[2].Images) != 0 {
		t.Fatalf("B: msg4 image must be offloaded: %+v", got[2])
	}
	if len(got[3].Images) != 1 || got[3].Images[0] != imgB || !strings.Contains(got[3].Content, strings.Repeat("x", 1200)) {
		t.Fatalf("B: newest image + tail verbatim must stay: %+v", got[3])
	}
}

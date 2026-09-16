package main

import (
	"context"
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

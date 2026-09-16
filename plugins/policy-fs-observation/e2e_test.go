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

// TestE2EWithHostClient 端到端验证通用策略服务形态的 policy 插件：
// 以宿主侧 go-core 客户端 spawn exe，经 gRPC 验证元数据（type=policy），
// 并经主连接直接调用 PolicyService.OnEvent，覆盖读前改写、sha256 新鲜度、
// 缺失记录与 per-session 属主语义（宿主 policy 桥接同款路径）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "policy-fs-observation.exe")
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

	// 3. 元数据（SDK 自动提供，Type 由 Config 决定）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "policy" || info.Name != "fs-observation-policy" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
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

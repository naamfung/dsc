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

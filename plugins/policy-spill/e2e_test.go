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

// TestE2EWithHostClient 端到端验证外置策略插件的通用 PolicyService 形态：
// 以宿主侧 go-core 客户端 spawn exe，经 gRPC 验证元数据（type=policy），
// 并经主连接直接调用 PolicyService.OnEvent，覆盖超长结果外置（replace +
// 全文落盘 + 定位符即路径）、取回豁免、失败放行与阈值禁用语义
// （宿主 policy 桥接同款路径）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "policy-spill.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程：注入 DSC_SPILL_DIR 对齐宿主加载行为
	//    （插件进程经 env 白名单可见 DSC_* 宿主配置键）
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

	// 3. 元数据（SDK 自动提供，Type 由 Config 决定）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "policy" || info.Name != "spill-policy" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 4. 经主连接调用 PolicyService（宿主 policy 桥接同款路径）
	pc := proto.NewPolicyServiceClient(conn)
	content := longText(9000)

	// 4.1 超长结果 → replace；全文落盘；定位符为绝对路径且内容逐字一致
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

	// 4.2 未达阈值 → 放行（空裁决）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: kindPostExecute, Tool: "shell", Result: "short", Session: "s1"}); err != nil || dec.GetAction() != "" {
		t.Fatalf("短结果应放行: dec=%+v err=%v", dec, err)
	}

	// 4.3 取回路径豁免：view 命令超长结果不外置（防取回死循环）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{
		Kind: kindPostExecute, Tool: "str_replace_editor",
		ArgumentsJson: `{"command":"view","path":"/workspace/big.txt"}`,
		Result:        content, Session: "s1",
	}); err != nil || dec.GetAction() != "" {
		t.Fatalf("view 命令应豁免: dec=%+v err=%v", dec, err)
	}

	// 4.4 失败结果放行（错误是权威观察；外置只塑造被接受的成功结果）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{
		Kind: kindPostExecute, Tool: "shell", Result: content,
		Error: "no such file", Session: "s1",
	}); err != nil || dec.GetAction() != "" {
		t.Fatalf("失败结果应放行: dec=%+v err=%v", dec, err)
	}

	// 4.5 非 post-execute 槽一律放行（策略只在自己的领域发声）
	if dec, err = pc.OnEvent(ctx, &proto.PolicyEvent{Kind: "tool/pre-execute", Tool: "shell"}); err != nil || dec.GetAction() != "" {
		t.Fatalf("pre 槽应放行: dec=%+v err=%v", dec, err)
	}
}

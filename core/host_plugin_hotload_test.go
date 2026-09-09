package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
)

// TestHostLuaHotReloadReflectsNewScript 用真实 tool-lua-host 进程验证「运行中写入新
// 脚本 → 宿主经 ListTools 看到新工具」：插件在创造模式下热加载轮询扫描 scripts/，
// 新增的脚本注册的工具会动态出现在其 ListTools 中（宿主聚合工具目录的同步前提）。
func TestHostLuaHotReloadReflectsNewScript(t *testing.T) {
	dir := t.TempDir()
	// 启动时已有一个脚本 first（注册 lua_first）
	firstDir := filepath.Join(dir, "scripts", "first")
	if err := os.MkdirAll(firstDir, 0755); err != nil {
		t.Fatal(err)
	}
	firstScript := `dsc.register_tool("first", { description="first", parameters={ type="object", properties={}} }, function(): string
 return "first"
end)`
	if err := os.WriteFile(filepath.Join(firstDir, "main.lua"), []byte(firstScript), 0644); err != nil {
		t.Fatal(err)
	}

	exe := filepath.Join(dir, "tool-lua-host.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "tool-lua-host"), exe)
	cmd := exec.Command(exe)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DSC_MODE=creation")
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
	if _, err := toolClient.SetInterconnect(context.Background(), &proto.InterconnectRequest{}); err != nil {
		t.Fatalf("SetInterconnect: %v", err)
	}

	// 启动时：含 lua_first，不含 lua_second
	if !hasRegisteredLuaTool(t, toolClient, "lua_first") {
		t.Fatal("启动即应含 lua_first")
	}
	if hasRegisteredLuaTool(t, toolClient, "lua_second") {
		t.Fatal("启动时不应含 lua_second")
	}

	// 运行中写入第二个脚本 second（注册 lua_second）
	secondDir := filepath.Join(dir, "scripts", "second")
	if err := os.MkdirAll(secondDir, 0755); err != nil {
		t.Fatal(err)
	}
	secondScript := `dsc.register_tool("second", { description="second", parameters={ type="object", properties={}} }, function(): string
 return "second"
end)`
	if err := os.WriteFile(filepath.Join(secondDir, "main.lua"), []byte(secondScript), 0644); err != nil {
		t.Fatal(err)
	}

	// 等待热加载轮询（poll 间隔 2s）扫描到新脚本
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if hasRegisteredLuaTool(t, toolClient, "lua_second") {
			t.Log("热加载后 ListTools 已含新脚本工具 lua_second")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("热加载后 ListTools 未反映运行中新写入的脚本工具 lua_second")
}

func hasRegisteredLuaTool(t *testing.T, client proto.ToolServiceClient, name string) bool {
	t.Helper()
	defs, _, err := listStagedTools(client)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, d := range defs {
		if d.Name() == name {
			return true
		}
	}
	return false
}

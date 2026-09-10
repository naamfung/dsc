package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
)

// 本文件覆盖「Lua 脚本注册工具 → ToolProvider → SDK ToolService」在宿主层的
// 穿透链路：tool-lua-host 将脚本经 dsc.register_tool 注册的工具动态暴露给宿主。
// 这条链任何一处被 SDK 改动破坏（字段漂移/契约不匹配），模型用 Lua 写插件就会失效，
// 故用真实 lua-host 进程 + 自包含 Lua 脚本钉死「ListTools 可见 + ExecuteTool 可执行」。

// TestHostLuaChainToolProviderToSDK 验证真实 lua-host 加载一个自包含 Lua 脚本后，
// 脚本注册的 mytool 能经 ToolProvider 穿透到 SDK ToolService：ListTools 可见、
// ExecuteTool 返回脚本结果（字段保真：name/schema/description/result 全存活）。
func TestHostLuaChainToolProviderToSDK(t *testing.T) {
	dir := t.TempDir()
	// 脚本目录按 lua-host 约定：<cwd>/scripts/<name>/main.lua（cwd 即进程工作目录）
	scriptDir := filepath.Join(dir, "scripts", "mytool")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 自包含脚本：不依赖 LLM/tool/notify 互通，可在独立 spawn 下确定性执行
	script := `type MyArgs = {
  note: string?
}
local function mytool(args: MyArgs): string
    return "lua-ok:" .. (args.note or "")
end
dsc.register_tool("mytool", { description = "自包含测试工具", parameters = { type="object", properties={ note={ type="string", description="备注" } } } }, mytool)
`
	if err := os.WriteFile(filepath.Join(scriptDir, "main.lua"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	exe := filepath.Join(dir, "tool-lua-host.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "tool-lua-host"), exe)

	// 以宿主侧客户端 spawn 真实 lua-host；cwd = dir，使 ./scripts 落在上面脚本目录
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
	ctx := context.Background()

	// SetInterconnect 触发 lua-host 创建脚本宿主并同步加载脚本（宿主同款握手）
	if _, err := toolClient.SetInterconnect(ctx, &proto.InterconnectRequest{}); err != nil {
		t.Fatalf("SetInterconnect: %v", err)
	}

	// 脚本注册的 mytool 会被 lua-host 以 `lua_` 前缀暴露（对齐既有 lua_ping 等）
	const toolName = "lua_mytool"

	// 1. ListTools：脚本注册的 mytool 经 ToolProvider → SDK ToolService 可见
	defs, _, err := listStagedTools(toolClient)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var found ToolDefinition
	for i := range defs {
		if defs[i].Name() == toolName {
			found = defs[i]
			break
		}
	}
	if found == nil {
		names := make([]string, len(defs))
		for i, d := range defs {
			names[i] = d.Name()
		}
		t.Fatalf("ListTools 未含脚本注册的工具 %s，实际工具: %v", toolName, names)
	}
	if found.Description() == "" {
		t.Fatalf("%s 缺 description（Lua→Tool 描述未穿透）", toolName)
	}
	if len(found.ParametersSchema()) == 0 {
		t.Fatalf("%s 缺 parameters schema（Lua→Tool 参数未穿透）", toolName)
	}

	// 2. ExecuteTool：SDK ToolService 执行脚本函数并返回其结果（字段保真）
	resp, err := toolClient.ExecuteTool(ctx, &proto.ExecuteToolRequest{
		ToolName: toolName, ArgumentsJson: `{}`,
	})
	if err != nil {
		t.Fatalf("ExecuteTool(%s): %v", toolName, err)
	}
	if resp.Error != "" || resp.Content != "lua-ok:" {
		t.Fatalf("ExecuteTool 结果 = %+v, want 脚本返回 lua-ok:", resp)
	}
}

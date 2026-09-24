package core

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
)

// 本文件覆盖「.dsp 载体 → tool-sql-host 加载 → ToolProvider → SDK ToolService」
// 在宿主层的穿透链路。.dsp 是「程序即数据」的载体：插件的元数据、代码与运行期状态
// 同处一个 SQLite 库，宿主经 tool-sql-host 把它加载成模型可见的工具。
//
// 这条链任何一处被改动破坏（字段漂移、SDK 契约不匹配、加载时序错位），模型用 .dsp
// 造插件的能力就会静默失效，故用真实 .dsp + 真实插件进程钉死「ListTools 可见 +
// ExecuteTool 可执行 + 状态就地落回载体文件」三件事。

// TestHostSQLChainDspToSDK 验证真实 tool-sql-host 加载一个真实 .dsp 后：
//  1. 插件内脚本注册的工具经 ToolProvider 穿透到 SDK ToolService（ListTools 可见、
//     字段保真）；
//  2. ExecuteTool 返回脚本结果，且同一 VM 内自持状态跨调用累加；
//  3. 插件读写的是**自己那个文件**——执行后 .dsp 载体字节确有变化（状态就地落盘）。
func TestHostSQLChainDspToSDK(t *testing.T) {
	dir := t.TempDir()

	// 1. 用随插件分发的打包器把源目录打成 .dsp（对齐 SelfDB elf2self 的角色）
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `type MyArgs = {
  note: string?
}
local function mytool(args: MyArgs): string
    local n = dsc.store.get("count") or 0
    n = n + 1
    dsc.store.set("count", n)
    return "dsp-ok:" .. (args.note or "") .. ":" .. tostring(n)
end
dsc.register_tool("mytool", { description = "自包含测试工具", parameters = { type="object", properties={ note={ type="string", description="备注" } } } }, mytool)
`
	if err := os.WriteFile(filepath.Join(src, "main.lua"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	packer := filepath.Join(dir, "dsp-pack.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "tool-sql-host", "cmd", "dsp-pack"), packer)

	dspDir := filepath.Join(dir, "dsp")
	if err := os.MkdirAll(dspDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dspPath := filepath.Join(dspDir, "mytool.dsp")
	if out, err := exec.Command(packer, "-name", "mytool", "-o", dspPath, src).CombinedOutput(); err != nil {
		t.Fatalf("dsp-pack: %v\n%s", err, out)
	}
	before, err := os.ReadFile(dspPath)
	if err != nil {
		t.Fatal(err)
	}

	exe := filepath.Join(dir, "tool-sql-host.exe")
	buildToolBin(t, filepath.Join("..", "plugins", "tool-sql-host"), exe)

	// 2. 以宿主侧客户端 spawn 真实 tool-sql-host；cwd = dir，使 ./dsp 命中上面的载体
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

	// SetInterconnect 触发 sql-host 创建插件宿主并同步加载 .dsp（宿主同款握手）
	if _, err := toolClient.SetInterconnect(ctx, &proto.InterconnectRequest{}); err != nil {
		t.Fatalf("SetInterconnect: %v", err)
	}

	// 3. ListTools：.dsp 内脚本注册的 mytool 以 sql_ 前缀经 ToolProvider → SDK 可见
	const toolName = "sql_mytool"
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
		t.Fatalf("ListTools 未含 .dsp 内注册的工具 %s，实际工具: %v", toolName, names)
	}
	if found.Description() == "" {
		t.Fatalf("%s 缺 description（.dsp → Tool 描述未穿透）", toolName)
	}
	if len(found.ParametersSchema()) == 0 {
		t.Fatalf("%s 缺 parameters schema（.dsp → Tool 参数未穿透）", toolName)
	}

	// 4. ExecuteTool：SDK ToolService 执行脚本函数并返回其结果（字段保真）
	run := func() *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := toolClient.ExecuteTool(ctx, &proto.ExecuteToolRequest{
			ToolName: toolName, ArgumentsJson: `{"note":"n1"}`,
		})
		if err != nil {
			t.Fatalf("ExecuteTool(%s): %v", toolName, err)
		}
		return resp
	}
	first := run()
	if first.Error != "" || first.Content != "dsp-ok:n1:1" {
		t.Fatalf("ExecuteTool 结果 = %+v, want dsp-ok:n1:1", first)
	}
	// 自持状态（dsc.store）落在插件自身 .dsp 里，故同一 VM 内第二次调用累加
	second := run()
	if second.Error != "" || second.Content != "dsp-ok:n1:2" {
		t.Fatalf("ExecuteTool 第二次结果 = %+v, want dsp-ok:n1:2", second)
	}

	// 5. 插件读写自身：执行后载体文件确被就地写入（状态没有跑到别处去）
	after, err := os.ReadFile(dspPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal(".dsp 载体字节未变化：插件状态没有落回自身文件（.dsp 的「自持」不成立）")
	}
	if !strings.HasPrefix(string(after), "SQLite format 3\x00") {
		t.Fatal(".dsp 载体已不是合法 SQLite 库")
	}
}

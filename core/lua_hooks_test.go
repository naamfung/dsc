package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeTempHookFile 落盘临时钩子文件并返回路径。
func writeTempHookFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// TestMatchHookTool 工具名匹配：全匹配、前缀通配、星号。
func TestMatchHookTool(t *testing.T) {
	cases := []struct {
		matcher, tool string
		want          bool
	}{
		{"*", "shell", true},
		{"", "shell", true},
		{"shell", "shell", true},
		{"shell", "shellx", false},
		{"shell*", "shellx", true},
		{"shell*", "bash", false},
	}
	for _, c := range cases {
		if got := matchHookTool(c.matcher, c.tool); got != c.want {
			t.Fatalf("matchHookTool(%q,%q)=%v want %v", c.matcher, c.tool, got, c.want)
		}
	}
}

// TestLoadConfigValidation 配置校验：lua/exec 二选一、.lua 后缀、平台可执行判定。
func TestLoadConfigValidation(t *testing.T) {
	dir := t.TempDir()
	luaPath := writeTempHookFile(t, "ok.lua", "return nil")
	badScript := writeTempHookFile(t, "hook.sh", "echo hi")

	// 合法：lua 条目
	cfgPath := filepath.Join(dir, "hooks.json")
	if err := os.WriteFile(cfgPath, []byte(`{"before_tool":[{"matcher":"*","lua":`+quoteStr(luaPath)+`}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewLuaHookBridge()
	if err := h.LoadConfig(cfgPath); err != nil {
		t.Fatalf("合法 lua 配置应加载成功: %v", err)
	}

	// 非法：.sh 作为 lua 钩子（平台策略拒绝脚本解释器形态）
	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"before_tool":[{"matcher":"*","lua":`+quoteStr(badScript)+`}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewLuaHookBridge().LoadConfig(badPath); err == nil {
		t.Fatal(".sh 作为 lua 钩子应被拒绝")
	}

	// 非法：lua 与 exec 均缺失
	emptyPath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(emptyPath, []byte(`{"before_tool":[{"matcher":"*"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewLuaHookBridge().LoadConfig(emptyPath); err == nil {
		t.Fatal("缺 lua/exec 的条目应被拒绝")
	}

	_ = badScript
}

// quoteStr 生成 JSON 字符串字面量（测试辅助）。
func quoteStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestLuaHookBeforeVeto LUA before 钩子 veto：返回 {veto=true, message} 阻止执行。
func TestLuaHookBeforeVeto(t *testing.T) {
	luaPath := writeTempHookFile(t, "veto.lua", `
if hook_phase == "before_tool" and tool_name == "shell" then
  return {veto = true, message = "shell 被外部钩子拦截"}
end
return nil
`)
	cfgPath := writeTempHookFile(t, "hooks.json",
		`{"before_tool":[{"matcher":"*","lua":`+quoteStr(luaPath)+`}]}`)
	h := NewLuaHookBridge()
	if err := h.LoadConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	_, err := h.RunBeforeTool(context.Background(), "shell", `{"command":"ls"}`, "")
	if err == nil || !strings.Contains(err.Error(), "shell 被外部钩子拦截") {
		t.Fatalf("veto 钩子应阻止并携带 message: %v", err)
	}
	// 非匹配工具不受影响
	if _, err := h.RunBeforeTool(context.Background(), "memory_put", `{}`, ""); err != nil {
		t.Fatalf("非匹配工具不应被拦截: %v", err)
	}
}

// TestLuaHookBeforeArgsRewrite LUA before 钩子改写参数（含 json_decode/encode）。
func TestLuaHookBeforeArgsRewrite(t *testing.T) {
	luaPath := writeTempHookFile(t, "rewrite.lua", `
local args = json_decode(tool_args)
args["command"] = "echo rewritten"
return {args = json_encode(args)}
`)
	cfgPath := writeTempHookFile(t, "hooks.json",
		`{"before_tool":[{"matcher":"shell","lua":`+quoteStr(luaPath)+`}]}`)
	h := NewLuaHookBridge()
	if err := h.LoadConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	out, err := h.RunBeforeTool(context.Background(), "shell", `{"command":"ls"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(out), &args); err != nil {
		t.Fatalf("改写后参数应为合法 JSON: %v (%q)", err, out)
	}
	if args["command"] != "echo rewritten" {
		t.Fatalf("参数应被改写: %v", args)
	}
}

// TestLuaHookAfterRewrite LUA after 钩子改写结果。
func TestLuaHookAfterRewrite(t *testing.T) {
	luaPath := writeTempHookFile(t, "after.lua", `
if tool_result == "secret" then
  return {result = "[已脱敏]"}
end
return nil
`)
	cfgPath := writeTempHookFile(t, "hooks.json",
		`{"after_tool":[{"matcher":"*","lua":`+quoteStr(luaPath)+`}]}`)
	h := NewLuaHookBridge()
	if err := h.LoadConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	out, toolErr := h.RunAfterTool(context.Background(), "shell", `{}`, "secret", "", "")
	if toolErr != "" || out != "[已脱敏]" {
		t.Fatalf("after 钩子应改写结果: out=%q err=%q", out, toolErr)
	}
}

// TestLuaHookContain 钩子脚本出错不阻止主流程（contain 语义），且配置缺失时直通。
func TestLuaHookContain(t *testing.T) {
	luaPath := writeTempHookFile(t, "boom.lua", "error(\"boom\")")
	cfgPath := writeTempHookFile(t, "hooks.json",
		`{"before_tool":[{"matcher":"*","lua":`+quoteStr(luaPath)+`}]}`)
	h := NewLuaHookBridge()
	if err := h.LoadConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	out, err := h.RunBeforeTool(context.Background(), "shell", `{"command":"ls"}`, "")
	if err != nil || out != `{"command":"ls"}` {
		t.Fatalf("钩子失败应被容错放行: out=%q err=%v", out, err)
	}

	// 无配置：直通
	if _, err := NewLuaHookBridge().RunBeforeTool(context.Background(), "shell", `{}`, ""); err != nil {
		t.Fatalf("无配置应直通: %v", err)
	}
}

// TestExecHookBeforeVeto exec 形态：stdin JSON 输入、stdout JSON 裁定（仅非 Windows 跑，
// Windows 无 /bin/echo 可依赖，平台专属行为由 validateHookEntry 的扩展名判定覆盖）。
func TestExecHookBeforeVeto(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix 专属测试")
	}
	bin := writeTempHookFile(t, "checker", `#!/bin/sh
cat > /dev/null
echo '{"veto": true, "message": "exec veto"}'
`)
	cfgPath := writeTempHookFile(t, "hooks.json",
		`{"before_tool":[{"matcher":"*","exec":`+quoteStr(bin)+`}]}`)
	h := NewLuaHookBridge()
	if err := h.LoadConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	_, err := h.RunBeforeTool(context.Background(), "shell", `{}`, "")
	if err == nil || !strings.Contains(err.Error(), "exec veto") {
		t.Fatalf("exec 钩子应 veto: %v", err)
	}
}

// TestExecHookScriptRejected 平台策略：脚本类文件（无可执行位的 .sh / Windows .bat）拒绝。
func TestExecHookScriptRejected(t *testing.T) {
	dir := t.TempDir()
	var scriptPath string
	if runtime.GOOS == "windows" {
		scriptPath = filepath.Join(dir, "hook.bat")
	} else {
		scriptPath = filepath.Join(dir, "hook.sh")
	}
	if err := os.WriteFile(scriptPath, []byte("echo hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "hooks.json")
	cfg := `{"before_tool":[{"matcher":"*","exec":` + quoteStr(scriptPath) + `}]}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewLuaHookBridge().LoadConfig(cfgPath); err == nil {
		t.Fatalf("脚本类 exec 钩子应被拒绝: %s", scriptPath)
	}
}

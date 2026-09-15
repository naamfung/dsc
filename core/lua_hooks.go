package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	lua "github.com/wippyai/go-lua"
)

// 外部钩子桥（对齐 DSH packages/hooks 的 hooks-claude-code + hooks-codex 形态：
// 在工具执行前/后运行用户配置的钩子，可 veto 阻止或改写参数/结果）。
//
// 与插件 gRPC HookService（对内服务，proto BeforeTool/AfterTool）的命名区分：
// 本桥是「对外脚本钩子」，面向用户已掌握的 Claude Code / Codex hooks.json 生态，
// 类型名 LuaHookBridge 自带 Lua 标识，两者不混淆。
//
// 平台策略（用户裁定）：钩子只允许两种形态——
//  1. "lua"：严格 LUA 脚本，由本地 go-lua 库进程内解释执行（libs/go-lua，
//     与 workflow 引擎同一引擎、同一白名单库），零外部进程，天然跨平台；
//  2. "exec"：直接执行原生可执行文件（DSH 的钩子为 shell 命令行，可运行任意
//     程序——本桥按用户裁定跟进直接执行支持），不经 shell、不解析脚本，
//     Windows 仅接受 .exe/.com 并复用 ConfigureChildProcessAttrs 隐藏控制台。
//     .sh/.bat/.py 等脚本一律拒绝（那正是视窗下五花八门脚本引擎问题的根源，
//     这类诉求应改写为 LUA 钩子）。
//
// 配置格式（对齐 Claude Code hooks.json 习惯）：
//
//      {
//        "before_tool": [{"matcher": "shell", "lua": "/path/hook.lua"}],
//        "after_tool":  [{"matcher": "*", "exec": "/path/to/checker", "timeout_sec": 5}]
//      }
//
// 脚本契约（lua 与 exec 同一协议）：
//   - 输入全局（lua）或 stdin JSON（exec）：hook_phase / tool_name / tool_args /
//     tool_result / tool_error（before 阶段 result/error 为空串）
//   - 返回 nil（放行）或 table：
//     before_tool: {veto=true, message="阻止原因"} 或 {args="<改写后的参数 JSON>"}
//     after_tool:  {result="<改写后的结果>"} 或 {error="<置错原因>"}
//     exec 形态经 stdout 输出同一结构的 JSON。
//
// 失败语义对齐 DSH contain：单个钩子执行失败不阻止主流程，由调用方埋点留痕。

// HookEntry 一个外部钩子条目：lua 与 exec 二选一（同时给出以 lua 优先）。
type HookEntry struct {
	// Matcher 工具名匹配模式（"*" 匹配所有，支持前缀通配 "shell*"）。
	Matcher string `json:"matcher"`
	// Lua 严格 LUA 脚本路径（.lua，由本地 go-lua 解释执行）。
	Lua string `json:"lua,omitempty"`
	// Exec 原生可执行文件路径（不经 shell 直接执行）。
	Exec string `json:"exec,omitempty"`
	// TimeoutSec 单次钩子执行超时秒数（≤0 取默认 10s）。
	TimeoutSec int `json:"timeout_sec,omitempty"`
}

// HookConfig 外部钩子配置。
type HookConfig struct {
	BeforeTool []HookEntry `json:"before_tool"`
	AfterTool  []HookEntry `json:"after_tool"`
}

// LuaHookBridge 外部脚本钩子桥接器。
type LuaHookBridge struct {
	mu     sync.Mutex
	config *HookConfig
}

// NewLuaHookBridge 创建外部钩子桥接器。
func NewLuaHookBridge() *LuaHookBridge {
	return &LuaHookBridge{}
}

// defaultHookTimeout 单个钩子执行的默认超时。
const defaultHookTimeout = 10 * time.Second

// LoadConfig 从 JSON 文件加载钩子配置；路径不存在视为无配置（静默）。
func (h *LuaHookBridge) LoadConfig(path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 无配置文件不报错
		}
		return fmt.Errorf("lua hooks: read config: %w", err)
	}

	var cfg HookConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("lua hooks: parse config: %w", err)
	}

	for phase, entries := range map[string][]HookEntry{"before_tool": cfg.BeforeTool, "after_tool": cfg.AfterTool} {
		for i, e := range entries {
			if err := validateHookEntry(e); err != nil {
				return fmt.Errorf("lua hooks: %s[%d]: %w", phase, i, err)
			}
		}
	}

	h.config = &cfg
	return nil
}

// validateHookEntry 校验条目：lua/exec 二选一、路径存在且形态受支持。
func validateHookEntry(e HookEntry) error {
	if e.Matcher == "" {
		e.Matcher = "*"
	}
	if e.Lua != "" {
		if !strings.HasSuffix(strings.ToLower(e.Lua), ".lua") {
			return fmt.Errorf("lua hook %q: 仅支持 .lua 脚本（平台策略：不解释 shell/bat/python 等脚本）", e.Lua)
		}
		if _, err := os.Stat(e.Lua); err != nil {
			return fmt.Errorf("lua hook %q not found: %w", e.Lua, err)
		}
		return nil
	}
	if e.Exec != "" {
		if !isNativeExecutable(e.Exec) {
			return fmt.Errorf("exec hook %q: 仅支持原生可执行文件（Windows: .exe/.com；其余平台需可执行位）；脚本类请改用 lua 钩子", e.Exec)
		}
		if _, err := os.Stat(e.Exec); err != nil {
			return fmt.Errorf("exec hook %q not found: %w", e.Exec, err)
		}
		return nil
	}
	return fmt.Errorf("entry 需要 lua 或 exec 之一")
}

// isNativeExecutable 平台相关的原生可执行判定：
// Windows 仅 .exe/.com（.bat/.cmd 需 shell 解释，拒绝）；其余平台要求可执行位。
func isNativeExecutable(path string) bool {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".exe" || ext == ".com"
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&0111 != 0
}

// hookInput 一次钩子调用的输入载荷（lua 全局注入与 exec stdin 同构）。
type hookInput struct {
	Phase     string `json:"hook_phase"`
	ToolName  string `json:"tool_name"`
	Args      string `json:"tool_args"`
	Result    string `json:"tool_result"`
	ToolError string `json:"tool_error"`
	CallID    string `json:"call_id,omitempty"`
}

// hookOutcome 钩子返回的裁定（lua 返回 table 与 exec stdout JSON 同构）。
type hookOutcome struct {
	Veto    bool   `json:"veto,omitempty"`
	Message string `json:"message,omitempty"`
	Args    string `json:"args,omitempty"`   // before：改写后的参数 JSON
	Result  string `json:"result,omitempty"` // after：改写后的结果
	Error   string `json:"error,omitempty"`  // after：置错原因
}

// RunBeforeTool 执行工具前外部钩子（按配置顺序）。
// 返回 (可能改写后的参数, vetoError)：vetoError 非 nil 表示阻止执行。
func (h *LuaHookBridge) RunBeforeTool(ctx context.Context, toolName, argsJSON, callID string) (string, error) {
	h.mu.Lock()
	config := h.config
	h.mu.Unlock()
	if config == nil {
		return argsJSON, nil
	}
	for _, entry := range config.BeforeTool {
		if !matchHookTool(entry.Matcher, toolName) {
			continue
		}
		outcome, err := h.runEntry(ctx, entry, hookInput{
			Phase: "before_tool", ToolName: toolName, Args: argsJSON, CallID: callID,
		})
		if err != nil {
			continue // contain 语义：钩子失败不阻止主流程（调用方埋点留痕）
		}
		if outcome == nil {
			continue
		}
		if outcome.Veto {
			msg := outcome.Message
			if msg == "" {
				msg = "blocked by lua hook"
			}
			return argsJSON, fmt.Errorf("lua hook vetoed tool %s: %s", toolName, msg)
		}
		if outcome.Args != "" && outcome.Args != argsJSON {
			argsJSON = outcome.Args
		}
	}
	return argsJSON, nil
}

// RunAfterTool 执行工具后外部钩子（按配置顺序）。
// 返回 (可能改写后的结果, 可能置入的错误)。
func (h *LuaHookBridge) RunAfterTool(ctx context.Context, toolName, argsJSON, result, toolErr, callID string) (string, string) {
	h.mu.Lock()
	config := h.config
	h.mu.Unlock()
	if config == nil {
		return result, toolErr
	}
	for _, entry := range config.AfterTool {
		if !matchHookTool(entry.Matcher, toolName) {
			continue
		}
		outcome, err := h.runEntry(ctx, entry, hookInput{
			Phase: "after_tool", ToolName: toolName, Args: argsJSON,
			Result: result, ToolError: toolErr, CallID: callID,
		})
		if err != nil {
			continue
		}
		if outcome == nil {
			continue
		}
		if outcome.Error != "" {
			toolErr = outcome.Error
		} else if outcome.Result != "" {
			result = outcome.Result
			toolErr = ""
		}
	}
	return result, toolErr
}

// runEntry 分发执行单个钩子条目（lua 进程内解释 / exec 直接执行）。
// 返回 (nil, nil) 表示钩子无返回（放行）。
func (h *LuaHookBridge) runEntry(ctx context.Context, entry HookEntry, in hookInput) (*hookOutcome, error) {
	timeout := time.Duration(entry.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = defaultHookTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if entry.Lua != "" {
		return runLuaHook(runCtx, entry.Lua, in)
	}
	return runExecHook(runCtx, entry.Exec, in)
}

// runLuaHook 以严格 LUA 解释执行钩子脚本：
// 白名单仅 base/string/table/math（与 workflow 引擎同款严格配置，无 os/io），
// 注入输入全局后调用脚本，取其返回 table 解析裁定。
func runLuaHook(ctx context.Context, script string, in hookInput) (*hookOutcome, error) {
	L := lua.NewState()
	defer L.Close()
	lua.OpenBase(L)
	lua.OpenString(L)
	lua.OpenTable(L)
	lua.OpenMath(L)

	// 输入全局注入（字符串常量，脚本只读约定）
	L.SetGlobal("hook_phase", lua.LString(in.Phase))
	L.SetGlobal("tool_name", lua.LString(in.ToolName))
	L.SetGlobal("tool_args", lua.LString(in.Args))
	L.SetGlobal("tool_result", lua.LString(in.Result))
	L.SetGlobal("tool_error", lua.LString(in.ToolError))
	L.SetGlobal("call_id", lua.LString(in.CallID))
	// json_decode(s)→table / json_encode(t)→string：Go 侧实现，脚本可解析/构造参数 JSON
	L.SetGlobal("json_decode", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(jsonToLua(L, v))
		return 1
	}))
	L.SetGlobal("json_encode", L.NewFunction(func(L *lua.LState) int {
		b, err := json.Marshal(luaToJSON(L.Get(1)))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(b)))
		return 1
	}))

	// ctx 绑定解释器：超时/取消时 VM 指令循环自动中断（mainLoopWithContext），
	// 防止钩子脚本死循环拖住工具流水线。
	L.SetContext(ctx)

	// 执行脚本文件，取其返回值（MultRet 压栈，栈顶为首个返回值）。
	// 注意：本 fork 的 OpenBase 为沙箱配置，不含 dofile/loadfile，须走 Go 侧 DoFile。
	if err := L.DoFile(script); err != nil {
		return nil, fmt.Errorf("lua hook %s run: %w", script, err)
	}
	return outcomeFromLuaValue(L.Get(-1))
}

// outcomeFromLuaValue 把脚本返回值解析为裁定：nil/无 → 放行；table → 裁定字段。
func outcomeFromLuaValue(v lua.LValue) (*hookOutcome, error) {
	if v == nil || v.Type() == lua.LTNil {
		return nil, nil
	}
	t, ok := v.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("lua hook 返回值须为 table 或 nil，得到 %s", v.Type().String())
	}
	out := &hookOutcome{}
	if b, ok := t.RawGetString("veto").(lua.LBool); ok {
		out.Veto = bool(b)
	}
	if s, ok := t.RawGetString("message").(lua.LString); ok {
		out.Message = string(s)
	}
	if s, ok := t.RawGetString("args").(lua.LString); ok {
		out.Args = string(s)
	}
	if s, ok := t.RawGetString("result").(lua.LString); ok {
		out.Result = string(s)
	}
	if s, ok := t.RawGetString("error").(lua.LString); ok {
		out.Error = string(s)
	}
	return out, nil
}

// runExecHook 直接执行原生可执行文件（不经 shell）：输入经 stdin 传 JSON，
// 裁定经 stdout 返回同构 JSON。非 JSON 的 stdout 输出按 result 处理（方便
// 简单检查器只 echo 一个文本）。
func runExecHook(ctx context.Context, bin string, in hookInput) (*hookOutcome, error) {
	inputJSON, _ := json.Marshal(in)
	cmd := exec.CommandContext(ctx, bin)
	// Windows 隐藏钩子子进程控制台（Task 2 的统一子进程属性），避免 TUI 终端闪烁
	ConfigureChildProcessAttrs(cmd)
	cmd.Stdin = strings.NewReader(string(inputJSON))
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("exec hook %s: %w (stderr: %s)", bin, err, strings.TrimSpace(stderr.String()))
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	}
	var out hookOutcome
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		// 非 JSON 输出：before 阶段无从裁定，after 阶段视为结果改写
		return &hookOutcome{Result: output}, nil
	}
	return &out, nil
}

// matchHookTool 匹配工具名与钩子模式（"*"/"" 匹配所有，支持前缀通配 "shell*"）。
func matchHookTool(matcher, toolName string) bool {
	if matcher == "*" || matcher == "" {
		return true
	}
	if strings.HasSuffix(matcher, "*") {
		prefix := strings.TrimSuffix(matcher, "*")
		return strings.HasPrefix(toolName, prefix)
	}
	return matcher == toolName
}

// jsonToLua 把 Go 值（json.Unmarshal 产物）转为 LUA 值。
func jsonToLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case []any:
		t := L.NewTable()
		for i, item := range x {
			t.RawSetInt(i+1, jsonToLua(L, item))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, item := range x {
			t.RawSetString(k, jsonToLua(L, item))
		}
		return t
	default:
		return lua.LNil
	}
}

// luaToJSON 把 LUA 值转为 Go 值（供 encoding/json 序列化）：
// 数组型 table（连续 1..n）→ []any，其余 table → map[string]any。
func luaToJSON(v lua.LValue) any {
	switch x := v.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(x)
	case lua.LNumber:
		return float64(x)
	case lua.LString:
		return string(x)
	case *lua.LTable:
		if n := x.Len(); n > 0 {
			arr := make([]any, 0, n)
			isArray := true
			for i := 1; i <= n; i++ {
				item := x.RawGetInt(i)
				if item.Type() == lua.LTNil {
					isArray = false
					break
				}
				arr = append(arr, luaToJSON(item))
			}
			if isArray {
				return arr
			}
		}
		m := make(map[string]any)
		x.ForEach(func(k, val lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				m[string(ks)] = luaToJSON(val)
			}
		})
		return m
	default:
		return nil
	}
}

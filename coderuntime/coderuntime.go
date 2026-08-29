// Package coderuntime 提供 PTC（工具呈现）的 code-runtime：
// 执行模型编写的 Lua 脚本，脚本内以 tool(name, args) 按名调用宿主工具，
// 运行结果为结构化 Result——失败建模为结果字段（stop_reason/error），
// 亦不裸抛。程序语言与执行引擎为 go-lua（带类型检查的 Lua VM，本地 fork）。
//
// 执行模型：整段程序跑在一个 coroutine（thread）里。Go 侧 `tool` 绑定用
// `L.Yield(request)` 挂起线程并把工具请求抛回驱动方；驱动方执行宿主工具后
// 以 `L.Resume(co, ok, result)` 续行，续行的值即成脚本 `local ok, r = tool(...)`
// 的回传值——异步「await」由框架隐式完成，脚本不必感知 Promise/coroutine。
//
// 执行隔离：仅 open base/string/table/math（无 os/io → 不可碰宿主文件系统/进程），
// 并带 ctx 取消/超时；非安全边界（对齐 DSH containment 语义）。
package coderuntime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"dsc/lualib"
	lua "github.com/wippyai/go-lua"
)

// 停止原因。
const (
	StopCompleted = "completed"
	StopError     = "error"
	StopCancelled = "cancelled"
)

// ToolCaller 按名执行一个宿主工具：arguments 为 JSON，返回结果文本。
// 必须尊重传入 ctx（取消/超时时尽快返回）。
type ToolCaller func(ctx context.Context, name, argumentsJSON string) (string, error)

// LogSink 可选：消费脚本内 log(msg)/print(...)（nil 时仅收集进 Result.Logs）。
type LogSink func(line string)

// Options 一次运行所需配置。
type Options struct {
	Script   string         // 模型编写的 Lua 源码（顶层 return 即程序返回值）
	Tool     ToolCaller     // tool() 的执行目标；nil => tool() 不可用
	Bindings map[string]any // 额外全局（如 args 对象）
	Log      LogSink        // 可选日志回调
	Timeout  time.Duration  // <=0 不设超时
}

// ToolCallSnapshot 单次 tool() 调用快照（供呈现/日志/UI 观测）。
type ToolCallSnapshot struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Result 运行结果。Value 为程序最终值（JSON 物化）；
// 失败建模为字段，而非 Go error。
type Result struct {
	Value      any                `json:"value,omitempty"`
	StopReason string             `json:"stop_reason"`
	Error      string             `json:"error,omitempty"`
	ToolCalls  []ToolCallSnapshot `json:"tool_calls,omitempty"`
	Logs       []string           `json:"logs,omitempty"`
}

// Run 执行一次 Lua 程序。所有结果（成功/脚本报错/取消/不可序列化）都进 Result；
// 工具失败以 `false, errMsg` 呈现给脚本（tool 返回两值，失败作为数据而非 throw）。
func Run(ctx context.Context, opts Options) Result {
	if strings.TrimSpace(opts.Script) == "" {
		return Result{StopReason: StopError, Error: "empty script"}
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	L := lua.NewState()
	defer L.Close()
	lua.OpenBase(L)
	lua.OpenString(L)
	lua.OpenTable(L)
	lua.OpenMath(L)

	var (
		mu    sync.Mutex
		logs  []string
		calls []*ToolCallSnapshot
	)
	emitLog := func(line string) {
		mu.Lock()
		logs = append(logs, line)
		mu.Unlock()
		if opts.Log != nil {
			opts.Log(line)
		}
	}
	snapLogs := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), logs...)
	}
	snapCalls := func() []ToolCallSnapshot {
		mu.Lock()
		defer mu.Unlock()
		out := make([]ToolCallSnapshot, len(calls))
		for i, c := range calls {
			out[i] = *c
		}
		return out
	}

	// tool(name, args?)：按名调用宿主工具。返回两值 ok, result：成功 ok=true，
	// result 为（JSON 解析后的）工具结果；失败 ok=false，result 为错误信息。
	// 第一次调用 `L.Yield(req)` 把工具请求挂起抛给驱动方；驱动方以
	// `L.Resume(co, ok, result)` 续行时，这两值即成脚本侧 tool(...) 的回传值。
	L.SetGlobal("tool", L.NewFunction(func(L *lua.LState) int {
		if opts.Tool == nil {
			L.RaiseError("tool() unavailable: no tool caller configured")
		}
		name := strings.TrimSpace(L.CheckString(1))
		if name == "" {
			L.RaiseError("tool() requires a non-empty name")
		}
		argsJSON := "{}"
		if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
			if b, err := json.Marshal(lualib.FromLua(L.Get(2))); err == nil {
				argsJSON = string(b)
			} else {
				L.RaiseError("tool() args must be JSON-serializable: %v", err)
			}
		}
		snap := &ToolCallSnapshot{Name: name, Arguments: argsJSON}
		mu.Lock()
		calls = append(calls, snap)
		mu.Unlock()
		req := L.NewUserData()
		req.Value = snap
		return L.Yield(req)
	}))

	// log(msg)/print(...)：进度叙述（映射到日志收集，避免写 stdout 噪声）。
	emit := func(L *lua.LState) int {
		var parts []string
		for i := 1; i <= L.GetTop(); i++ {
			parts = append(parts, L.Get(i).String())
		}
		emitLog(strings.Join(parts, "\t"))
		return 0
	}
	L.SetGlobal("log", L.NewFunction(emit))
	L.SetGlobal("print", L.NewFunction(emit))

	// Bindings 作为全局暴露（如 args）。
	for k, v := range opts.Bindings {
		L.SetGlobal(k, lualib.ToLua(L, v))
	}

	// 把脚本包成 `__cr_entry` 函数（顶层 return 即函数 return），
	// 语法错误在此处即被 DoString 拦下。
	if err := L.DoString("function __cr_entry()\n" + opts.Script + "\nend"); err != nil {
		return Result{StopReason: StopError, Error: "script parse error: " + err.Error()}
	}
	entry := L.GetGlobal("__cr_entry").(*lua.LFunction)

	// coroutine 驱动：co 携带 ctx（CPU 死循环/同步段可被 mainLoopWithContext 截断）。
	co := L.NewThreadWithContext(ctx)
	state, vals, err := L.Resume(co, entry)
	for state == lua.ResumeYield {
		// run 内 yield 恒为 tool 请求（脚本不直接使用 coroutine）。
		ud, ok := vals[0].(*lua.LUserData)
		if !ok {
			return Result{StopReason: StopError, Error: "unexpected yield value", Logs: snapLogs(), ToolCalls: snapCalls()}
		}
		snap, ok := ud.Value.(*ToolCallSnapshot)
		if !ok {
			return Result{StopReason: StopError, Error: "unexpected tool request", Logs: snapLogs(), ToolCalls: snapCalls()}
		}
		if ctx.Err() != nil {
			return Result{StopReason: StopCancelled, Error: "program cancelled", Logs: snapLogs(), ToolCalls: snapCalls()}
		}
		result, toolErr := opts.Tool(ctx, snap.Name, snap.Arguments)
		if toolErr != nil {
			snap.Error = toolErr.Error()
			state, vals, err = L.Resume(co, entry, lua.LFalse, lua.LString(toolErr.Error()))
		} else {
			snap.Result = result
			state, vals, err = L.Resume(co, entry, lua.LTrue, lualib.ToLua(L, reviveJSON(result)))
		}
	}

	switch state {
	case lua.ResumeOK:
		value := materialize(vals)
		if _, jerr := json.Marshal(value); jerr != nil {
			return Result{StopReason: StopError, Error: "script returned non-JSON value: " + jerr.Error(), Logs: snapLogs(), ToolCalls: snapCalls()}
		}
		return Result{Value: value, StopReason: StopCompleted, Logs: snapLogs(), ToolCalls: snapCalls()}
	case lua.ResumeError:
		reason := StopError
		if ctx.Err() != nil {
			reason = StopCancelled
		}
		return Result{StopReason: reason, Error: err.Error(), Logs: snapLogs(), ToolCalls: snapCalls()}
	default:
		return Result{StopReason: StopError, Error: "unexpected resume state", Logs: snapLogs(), ToolCalls: snapCalls()}
	}
}

// materialize 把程序回传值物化：单值 -> 原值；多值 -> 数组。
func materialize(vals []lua.LValue) any {
	if len(vals) == 1 {
		return lualib.FromLua(vals[0])
	}
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = lualib.FromLua(v)
	}
	return out
}

// reviveJSON 尝试把工具返回串解析为 JSON（数值用 json.Number 保留精度）。
// 解析成功返回结构化值，否则原样返回字符串。
func reviveJSON(s string) any {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return s
	}
	return v
}

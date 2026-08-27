// Package bindings 把宿主互通服务（聚合 LLM / 聚合 Tool / 插件通知）以 LUA 内建
// 函数的形式暴露给脚本，并提供工具注册入口：
//
//	dsc.llm.chat({system=..., user=..., max_tokens=...})  → string（宿主聚合 LLM）
//	dsc.tool.call("name", {args...})                      → string（经宿主转发到任意工具插件）
//	dsc.tool.list()                                       → {name, description}[]
//	dsc.notify.emit("event", {data...})                   → 无（发布宿主事件总线）
//	dsc.register_tool("name", {description=..., parameters={...}}, handler) → 注册工具
package bindings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"dsc/core/llmclient"
	"dsc/core/notify"
	"dsc/core/toolclient"
	"dsc/proto"
	lua "github.com/wippyai/go-lua"
)

// Services 宿主互通服务的集合（由 tool-lua-host 在 SetInterconnect 时建立）。
type Services struct {
	LLM    *llmclient.Client
	Tool   *toolclient.Client
	Notify *notify.Notifier
	Store  *Store        // 插件进程内 KV（脚本间共享）
	Hook   *HookRegistry // 脚本注册的宿主钩子
	// Register 脚本注册工具的回调（由 host 提供）：script 为脚本名（去重命名空间）。
	Register func(script, name, desc, paramsJSON string, fn *lua.LFunction) error
	// HookRun 执行脚本钩子（由 host 提供，含 VM 串行化与回调）。
	HookRun HookRunner
	// SpawnJob 启动脚本后台任务（由 host 提供，含 VM 串行化）。
	SpawnJob func(script string, fn *lua.LFunction) (string, error)
	// JobStatus 查询后台任务状态（由 host 提供）。
	JobStatus func(id string) (string, error)
	// JobList 列出后台任务（由 host 提供）。
	JobList func() (map[string]string, error)
}

// HookRunner 执行某脚本钩子集合的回调（由 host 实现，保证 VM 串行）。
type HookRunner func(kind string, handlers []HookHandler, args ...any) []any

// Install 把 dsc.* 内建注入 LState。
func Install(L *lua.LState, s *Services) {
	dsc := L.NewTable()
	L.SetGlobal("dsc", dsc)

	llmT := L.NewTable()
	L.SetField(llmT, "chat", L.NewFunction(dscLLMChat(s)))
	L.SetField(dsc, "llm", llmT)

	toolT := L.NewTable()
	L.SetField(toolT, "call", L.NewFunction(dscToolCall(s)))
	L.SetField(toolT, "list", L.NewFunction(dscToolList(s)))
	L.SetField(dsc, "tool", toolT)

	notifyT := L.NewTable()
	L.SetField(notifyT, "emit", L.NewFunction(dscNotifyEmit(s)))
	L.SetField(dsc, "notify", notifyT)

	storeT := L.NewTable()
	L.SetField(storeT, "get", L.NewFunction(dscStoreGet(s)))
	L.SetField(storeT, "set", L.NewFunction(dscStoreSet(s)))
	L.SetField(storeT, "delete", L.NewFunction(dscStoreDelete(s)))
	L.SetField(dsc, "store", storeT)

	hookT := L.NewTable()
	L.SetField(hookT, "before_tool", L.NewFunction(dscHookBeforeTool(s)))
	L.SetField(hookT, "after_tool", L.NewFunction(dscHookAfterTool(s)))
	L.SetField(hookT, "on_event", L.NewFunction(dscHookOnEvent(s)))
	L.SetField(dsc, "hook", hookT)

	jobT := L.NewTable()
	L.SetField(jobT, "spawn", L.NewFunction(dscJobSpawn(s)))
	L.SetField(jobT, "status", L.NewFunction(dscJobStatus(s)))
	L.SetField(jobT, "list", L.NewFunction(dscJobList(s)))
	L.SetField(dsc, "job", jobT)

	L.SetField(dsc, "register_tool", L.NewFunction(dscRegisterTool(s)))
}

// ==================== dsc.llm.chat ====================

func dscLLMChat(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.LLM == nil {
			L.RaiseError("dsc.llm.chat: LLM service not injected by host")
		}
		tbl := L.CheckTable(1)
		system := tableString(L, tbl, "system")
		user := tableString(L, tbl, "user")
		if user == "" {
			L.RaiseError("dsc.llm.chat: 'user' is required")
		}
		maxTokens := int32(tableInt(L, tbl, "max_tokens"))
		if maxTokens <= 0 {
			maxTokens = 0 // 服务端默认
		}

		msgs := []*proto.Message{{Role: "system", Content: system}}
		if system == "" {
			msgs = msgs[:0]
		}
		msgs = append(msgs, &proto.Message{Role: "user", Content: user})

		// 用流式：thinking 模式下 unary Chat 的 text 可能为空，流式帧完整携带文本增量
		stream, err := s.LLM.ChatStream(context.Background(), msgs, maxTokens)
		if err != nil {
			L.RaiseError("dsc.llm.chat failed: %v", err)
		}
		var sb strings.Builder
		for {
			cr, rerr := stream.Recv()
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				L.RaiseError("dsc.llm.chat stream failed: %v", rerr)
			}
			if cr.GetError() != "" {
				L.RaiseError("dsc.llm.chat stream error: %s", cr.GetError())
			}
			sb.WriteString(cr.GetContent())
		}
		L.Push(lua.LString(sb.String()))
		return 1
	}
}

// ==================== dsc.tool.call / dsc.tool.list ====================

func dscToolCall(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Tool == nil {
			L.RaiseError("dsc.tool.call: tool service not injected by host")
		}
		name := L.CheckString(1)
		var args any
		if L.GetTop() >= 2 {
			args = luaToAny(L.Get(2))
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			L.RaiseError("dsc.tool.call: invalid args: %v", err)
		}
		out, err := s.Tool.ExecuteTool(context.Background(), name, string(argsJSON))
		if err != nil {
			L.RaiseError("dsc.tool.call(%s) failed: %v", name, err)
		}
		L.Push(lua.LString(out))
		return 1
	}
}

func dscToolList(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Tool == nil {
			L.RaiseError("dsc.tool.list: tool service not injected by host")
		}
		tools, err := s.Tool.ListTools(context.Background())
		if err != nil {
			L.RaiseError("dsc.tool.list failed: %v", err)
		}
		tbl := L.NewTable()
		for i, t := range tools {
			row := L.NewTable()
			L.SetField(row, "name", lua.LString(t.GetName()))
			L.SetField(row, "description", lua.LString(t.GetDescription()))
			L.RawSetInt(tbl, i+1, row)
		}
		L.Push(tbl)
		return 1
	}
}

// ==================== dsc.notify.emit ====================

func dscNotifyEmit(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Notify == nil {
			L.RaiseError("dsc.notify.emit: notify service not injected by host")
		}
		name := L.CheckString(1)
		dataJSON := ""
		if L.GetTop() >= 2 {
			b, err := json.Marshal(luaToAny(L.Get(2)))
			if err != nil {
				L.RaiseError("dsc.notify.emit: invalid data: %v", err)
			}
			dataJSON = string(b)
		}
		if err := s.Notify.Notify(context.Background(), name, dataJSON); err != nil {
			L.RaiseError("dsc.notify.emit(%s) failed: %v", name, err)
		}
		return 0
	}
}

// ==================== dsc.register_tool ====================

func dscRegisterTool(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Register == nil {
			L.RaiseError("dsc.register_tool: host not ready")
		}
		name := L.CheckString(1)
		spec := L.OptTable(2, nil)
		fn := L.CheckFunction(3)

		desc := ""
		paramsJSON := "{}"
		if spec != nil {
			desc = tableString(L, spec, "description")
			if p := L.GetField(spec, "parameters"); p != lua.LNil {
				if b, err := json.Marshal(luaToAny(p)); err == nil {
					paramsJSON = string(b)
				}
			}
		}
		if err := s.Register(scriptName(L), name, desc, paramsJSON, fn); err != nil {
			L.RaiseError("dsc.register_tool(%s) failed: %v", name, err)
		}
		return 0
	}
}

// ==================== dsc.store（进程内 KV，脚本间共享） ====================

func dscStoreGet(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Store == nil {
			L.RaiseError("dsc.store.get: store not available")
		}
		key := L.CheckString(1)
		v, ok := s.Store.Get(key)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(anyToLuaAny(L, v))
		return 1
	}
}

func dscStoreSet(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Store == nil {
			L.RaiseError("dsc.store.set: store not available")
		}
		key := L.CheckString(1)
		var v any
		if L.GetTop() >= 2 {
			v = luaToAny(L.Get(2))
		}
		s.Store.Set(key, v)
		return 0
	}
}

func dscStoreDelete(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Store == nil {
			L.RaiseError("dsc.store.delete: store not available")
		}
		s.Store.Delete(L.CheckString(1))
		return 0
	}
}

// anyToLuaAny 把 Go any 转回 LUA 值（供 store.get 使用；与 host.jsonToLua 同构）。
func anyToLuaAny(L *lua.LState, v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(t)
	case float64:
		return lua.LNumber(t)
	case int64:
		return lua.LInteger(t)
	case bool:
		return lua.LBool(t)
	case []any:
		tbl := L.NewTable()
		for i, item := range t {
			L.RawSetInt(tbl, i+1, anyToLuaAny(L, item))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k, item := range t {
			L.SetField(tbl, k, anyToLuaAny(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", t))
	}
}

// ==================== dsc.hook（脚本注册宿主钩子） ====================

func dscHookBeforeTool(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Hook == nil {
			L.RaiseError("dsc.hook.before_tool: hooks not available")
		}
		s.Hook.AddBefore(HookHandler{Script: scriptName(L), Fn: L.CheckFunction(1)})
		return 0
	}
}

func dscHookAfterTool(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Hook == nil {
			L.RaiseError("dsc.hook.after_tool: hooks not available")
		}
		s.Hook.AddAfter(HookHandler{Script: scriptName(L), Fn: L.CheckFunction(1)})
		return 0
	}
}

func dscHookOnEvent(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Hook == nil {
			L.RaiseError("dsc.hook.on_event: hooks not available")
		}
		s.Hook.AddOnEvent(HookHandler{Script: scriptName(L), Fn: L.CheckFunction(1)})
		return 0
	}
}

// ==================== dsc.job（脚本后台任务） ====================

func dscJobSpawn(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.SpawnJob == nil {
			L.RaiseError("dsc.job.spawn: host not ready")
		}
		fn := L.CheckFunction(1)
		id, err := s.SpawnJob(scriptName(L), fn)
		if err != nil {
			L.RaiseError("dsc.job.spawn failed: %v", err)
		}
		L.Push(lua.LString(id))
		return 1
	}
}

func dscJobStatus(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.JobStatus == nil {
			L.RaiseError("dsc.job.status: host not ready")
		}
		st, err := s.JobStatus(L.CheckString(1))
		if err != nil {
			L.RaiseError("dsc.job.status failed: %v", err)
		}
		L.Push(lua.LString(st))
		return 1
	}
}

func dscJobList(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.JobList == nil {
			L.RaiseError("dsc.job.list: host not ready")
		}
		jobs, err := s.JobList()
		if err != nil {
			L.RaiseError("dsc.job.list failed: %v", err)
		}
		tbl := L.NewTable()
		for id, st := range jobs {
			L.SetField(tbl, id, lua.LString(st))
		}
		L.Push(tbl)
		return 1
	}
}

// ==================== helpers ====================

// scriptName 从 registry 取当前脚本名（由 host 在加载脚本前注入）。
func scriptName(L *lua.LState) string {
	if v := L.GetGlobal("__dsc_script"); v != lua.LNil {
		return v.String()
	}
	return "?"
}

func tableString(L *lua.LState, tbl *lua.LTable, key string) string {
	v := L.GetField(tbl, key)
	if v == lua.LNil {
		return ""
	}
	return v.String()
}

func tableInt(L *lua.LState, tbl *lua.LTable, key string) int64 {
	v := L.GetField(tbl, key)
	if v == lua.LNil {
		return 0
	}
	if n, ok := v.(lua.LNumber); ok {
		return int64(n)
	}
	return 0
}

// luaToAny 把 LUA 值转成 Go any（table→map[string]any / []any，供 JSON 序列化）。
func luaToAny(v lua.LValue) any {
	switch t := v.(type) {
	case *lua.LTable:
		// 判断数组 vs 字典：首键为整数且无 string 键 → 数组
		isArray := true
		hasArrayKey := false
		var outMap map[string]any
		t.ForEach(func(k, val lua.LValue) {
			if n, ok := k.(lua.LNumber); ok && !hasArrayKey {
				hasArrayKey = true
				_ = n
			}
			if _, ok := k.(lua.LString); ok {
				isArray = false
			}
		})
		if isArray && hasArrayKey {
			var arr []any
			for i := 1; ; i++ {
				item := t.RawGetInt(i)
				if item == lua.LNil {
					break
				}
				arr = append(arr, luaToAny(item))
			}
			if arr != nil {
				return arr
			}
		}
		outMap = make(map[string]any)
		t.ForEach(func(k, val lua.LValue) {
			outMap[k.String()] = luaToAny(val)
		})
		return outMap
	case lua.LString:
		return string(t)
	case lua.LNumber:
		return float64(t)
	case lua.LInteger:
		return int64(t)
	case lua.LBool:
		return bool(t)
	case *lua.LFunction:
		return fmt.Sprintf("<function>")
	case nil:
		return nil
	default:
		// 不调 t.String()：LGoFunc.String 内部 fmt 打印指针可能触发栈溢出
		return "<" + v.Type().String() + ">"
	}
}

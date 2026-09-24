// Package bindings 把宿主互通服务（聚合 LLM / 聚合 Tool / 插件通知）与插件自持能力
// 以 LUA 内建函数的形式暴露给脚本：
//
//	dsc.llm.chat({system=..., user=..., max_tokens=...})   → string（宿主聚合 LLM）
//	dsc.tool.call("name", {args...})                       → string（经宿主转发到任意工具插件）
//	dsc.tool.list()                                        → {name, description}[]
//	dsc.notify.emit("event", {data...})                    → 无（发布宿主事件总线）
//	dsc.store.get/set/delete(key[, value])                 → 自持状态（落盘在插件自身 .dsp）
//	dsc.sql.query(stmt[, params])                          → 行数组（自身库）
//	dsc.sql.exec(stmt[, params])                           → 受影响行数（自身库）
//	dsc.sql.tables()                                       → 表名数组（自身库）
//	dsc.dsp.require("lib")                                 → 加载自身库内的脚本 blob
//	dsc.dsp.asset("logo.png")                              → 读取自身库内的静态内容
//	dsc.plugin.{name,path,readonly}                        → 插件自述
//	dsc.register_tool("name", {description=..., parameters={...}}, handler) → 注册工具
package bindings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"dsc/core/llmclient"
	"dsc/core/toolclient"
	"dsc/proto"
	lua "github.com/wippyai/go-lua"
)

// Services 宿主互通服务与插件自持能力的集合（由 tool-sql-host 在 SetInterconnect 时
// 按插件建立；每个插件一份副本，Register/Store/Self/Info 指向该插件自身）。
type Services struct {
	LLM    *llmclient.Client
	Tool   *toolclient.Client
	Notify Notifier
	// Store 插件自持状态（落盘在插件自身 .dsp）。
	Store StateStore
	// Self 插件对自身库的脚本侧 SQL / 静态内容访问。
	Self SelfDB
	// Info 插件自述（注入 dsc.plugin）。
	Info PluginInfo
	// Hook 插件注册的宿主钩子（进程内共享表，条目带 plugin 名）。
	Hook *HookRegistry
	// Register 插件注册工具的回调（由 host 提供）：plugin 为插件名（去重命名空间）。
	Register func(plugin, name, desc, paramsJSON string, fn *lua.LFunction) error
	// HookRun 执行插件钩子（由 host 提供，含 VM 串行化）。
	HookRun HookRunner
	// SpawnJob 启动插件后台任务（由 host 提供，含 VM 串行化）。
	SpawnJob func(plugin string, fn *lua.LFunction) (string, error)
	// JobStatus 查询后台任务状态（由 host 提供）。
	JobStatus func(id string) (string, error)
	// JobList 列出后台任务（由 host 提供）。
	JobList func() (map[string]string, error)
}

// HookRunner 执行某插件钩子集合的回调（由 host 实现，保证 VM 串行）。
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

	sqlT := L.NewTable()
	L.SetField(sqlT, "query", L.NewFunction(dscSQLQuery(s)))
	L.SetField(sqlT, "exec", L.NewFunction(dscSQLExec(s)))
	L.SetField(sqlT, "tables", L.NewFunction(dscSQLTables(s)))
	L.SetField(dsc, "sql", sqlT)

	dspT := L.NewTable()
	L.SetField(dspT, "require", L.NewFunction(dscDspRequire(s)))
	L.SetField(dspT, "asset", L.NewFunction(dscDspAsset(s)))
	L.SetField(dsc, "dsp", dspT)

	pluginT := L.NewTable()
	L.SetField(pluginT, "name", lua.LString(s.Info.Name))
	L.SetField(pluginT, "path", lua.LString(s.Info.Path))
	L.SetField(pluginT, "readonly", lua.LBool(s.Info.ReadOnly))
	L.SetField(dsc, "plugin", pluginT)

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

	// 脚本 blob 加载缓存（dsc.dsp.require）：同一模块只执行一次，返回其返回值。
	L.SetField(dsc, "loaded", L.NewTable())
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
		if err := s.Register(pluginName(L), name, desc, paramsJSON, fn); err != nil {
			L.RaiseError("dsc.register_tool(%s) failed: %v", name, err)
		}
		return 0
	}
}

// ==================== dsc.store（自持状态，落盘在插件自身 .dsp） ====================

func dscStoreGet(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Store == nil {
			L.RaiseError("dsc.store.get: store not available")
		}
		key := L.CheckString(1)
		v, ok, err := s.Store.StateGet(key)
		if err != nil {
			L.RaiseError("dsc.store.get(%s) failed: %v", key, err)
		}
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(anyToLua(L, v))
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
		if err := s.Store.StateSet(key, v); err != nil {
			L.RaiseError("dsc.store.set(%s) failed: %v", key, err)
		}
		return 0
	}
}

func dscStoreDelete(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Store == nil {
			L.RaiseError("dsc.store.delete: store not available")
		}
		key := L.CheckString(1)
		if err := s.Store.StateDelete(key); err != nil {
			L.RaiseError("dsc.store.delete(%s) failed: %v", key, err)
		}
		return 0
	}
}

// ==================== dsc.sql（作用于插件自身库） ====================

func dscSQLQuery(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		self := selfOf(L, s, "dsc.sql.query")
		rows, err := self.Query(L.CheckString(1), sqlParams(L))
		if err != nil {
			L.RaiseError("dsc.sql.query failed: %v", err)
		}
		tbl := L.NewTable()
		for i, row := range rows {
			r := L.NewTable()
			for _, k := range sortedKeysOf(row) {
				L.SetField(r, k, anyToLua(L, row[k]))
			}
			L.RawSetInt(tbl, i+1, r)
		}
		L.Push(tbl)
		return 1
	}
}

func dscSQLExec(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		self := selfOf(L, s, "dsc.sql.exec")
		n, err := self.Exec(L.CheckString(1), sqlParams(L))
		if err != nil {
			L.RaiseError("dsc.sql.exec failed: %v", err)
		}
		L.Push(lua.LInteger(n))
		return 1
	}
}

func dscSQLTables(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		self := selfOf(L, s, "dsc.sql.tables")
		names, err := self.Tables()
		if err != nil {
			L.RaiseError("dsc.sql.tables failed: %v", err)
		}
		tbl := L.NewTable()
		for i, n := range names {
			L.RawSetInt(tbl, i+1, lua.LString(n))
		}
		L.Push(tbl)
		return 1
	}
}

// ==================== dsc.dsp（插件自身库内的脚本与静态内容） ====================

// dscDspRequire 加载并缓存自身库内的脚本 blob（等价于 require）：
// 同一模块只执行一次，返回其返回值（无返回值时为 true，对齐 Lua 的 require 语义）。
func dscDspRequire(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		self := selfOf(L, s, "dsc.dsp.require")
		name := L.CheckString(1)
		loaded := L.GetField(L.GetGlobal("dsc"), "loaded").(*lua.LTable)
		if v := L.GetField(loaded, name); v != lua.LNil {
			L.Push(v)
			return 1
		}
		content, ok, err := loadScriptBlob(self, name)
		if err != nil {
			L.RaiseError("dsc.dsp.require(%s) failed: %v", name, err)
		}
		if !ok {
			L.RaiseError("dsc.dsp.require(%s): 自身库内无此脚本 blob", name)
		}
		base := L.GetTop()
		if err := L.DoString(content); err != nil {
			L.RaiseError("dsc.dsp.require(%s) 执行失败: %v", name, err)
		}
		var value lua.LValue = lua.LTrue
		if L.GetTop() > base {
			value = L.Get(-1)
		}
		L.SetTop(base)
		L.SetField(loaded, name, value)
		L.Push(value)
		return 1
	}
}

// loadScriptBlob 按名加载脚本 blob，兼容带/不带 .lua 后缀的写法。
func loadScriptBlob(self SelfDB, name string) (string, bool, error) {
	for _, candidate := range []string{name, name + ".lua"} {
		raw, ok, err := self.Blob(candidate, "script")
		if err != nil {
			return "", false, err
		}
		if ok {
			return string(raw), true, nil
		}
	}
	return "", false, nil
}

// dscDspAsset 读取自身库内的静态内容（kind=asset）；不存在返回 nil。
func dscDspAsset(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		self := selfOf(L, s, "dsc.dsp.asset")
		raw, ok, err := self.Blob(L.CheckString(1), "asset")
		if err != nil {
			L.RaiseError("dsc.dsp.asset failed: %v", err)
		}
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(string(raw)))
		return 1
	}
}

// ==================== dsc.hook（插件注册宿主钩子） ====================

func dscHookBeforeTool(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Hook == nil {
			L.RaiseError("dsc.hook.before_tool: hooks not available")
		}
		s.Hook.AddBefore(HookHandler{Plugin: pluginName(L), Fn: L.CheckFunction(1)})
		return 0
	}
}

func dscHookAfterTool(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Hook == nil {
			L.RaiseError("dsc.hook.after_tool: hooks not available")
		}
		s.Hook.AddAfter(HookHandler{Plugin: pluginName(L), Fn: L.CheckFunction(1)})
		return 0
	}
}

func dscHookOnEvent(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.Hook == nil {
			L.RaiseError("dsc.hook.on_event: hooks not available")
		}
		s.Hook.AddOnEvent(HookHandler{Plugin: pluginName(L), Fn: L.CheckFunction(1)})
		return 0
	}
}

// ==================== dsc.job（插件后台任务） ====================

func dscJobSpawn(s *Services) lua.LGFunction {
	return func(L *lua.LState) int {
		if s == nil || s.SpawnJob == nil {
			L.RaiseError("dsc.job.spawn: host not ready")
		}
		fn := L.CheckFunction(1)
		id, err := s.SpawnJob(pluginName(L), fn)
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

// selfOf 取插件自身库访问器（缺失即报错，调用方无需判空）。
func selfOf(L *lua.LState, s *Services, who string) SelfDB {
	if s == nil || s.Self == nil {
		L.RaiseError("%s: plugin self-database not available", who)
	}
	return s.Self
}

// sqlParams 把 LUA 参数（数组表或单值）转成 SQL 绑定参数。
func sqlParams(L *lua.LState) []any {
	if L.GetTop() < 2 {
		return nil
	}
	v := L.Get(2)
	if v == lua.LNil {
		return nil
	}
	t, ok := v.(*lua.LTable)
	if !ok {
		return []any{luaToAny(v)}
	}
	var out []any
	for i := 1; ; i++ {
		item := t.RawGetInt(i)
		if item == lua.LNil {
			break
		}
		out = append(out, luaToAny(item))
	}
	return out
}

// pluginName 从 registry 取当前插件名（由 host 在加载插件前注入）。
func pluginName(L *lua.LState) string {
	if v := L.GetGlobal("__dsc_plugin"); v != lua.LNil {
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

// sortedKeysOf 按列/键名升序返回行内键，保证同一行构造出的 Lua 表以确定顺序写入
// （对齐项目「进模型的有序产物须显式稳定排序」约定）。
func sortedKeysOf(row map[string]any) []string {
	out := make([]string, 0, len(row))
	for k := range row {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// anyToLua 把 Go 值转成 LUA 值（JSON 解码产物与 SQL 列值共用）。
func anyToLua(L *lua.LState, v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(t)
	case []byte:
		return lua.LString(string(t))
	case float64:
		return lua.LNumber(t)
	case int:
		return lua.LInteger(int64(t))
	case int64:
		return lua.LInteger(t)
	case bool:
		return lua.LBool(t)
	case []any:
		tbl := L.NewTable()
		for i, item := range t {
			L.RawSetInt(tbl, i+1, anyToLua(L, item))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for _, k := range sortedKeysOf(t) {
			L.SetField(tbl, k, anyToLua(L, t[k]))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", t))
	}
}

// luaToAny 把 LUA 值转成 Go any（table→map[string]any / []any，供 JSON 序列化）。
func luaToAny(v lua.LValue) any {
	switch t := v.(type) {
	case *lua.LTable:
		// 判断数组 vs 字典：首键为整数且无 string 键 → 数组
		isArray := true
		hasArrayKey := false
		t.ForEach(func(k, _ lua.LValue) {
			if _, ok := k.(lua.LNumber); ok && !hasArrayKey {
				hasArrayKey = true
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
		outMap := make(map[string]any)
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
		return "<function>"
	case nil:
		return nil
	default:
		// 不调 t.String()：LGoFunc.String 内部 fmt 打印指针可能触发栈溢出
		return "<" + v.Type().String() + ">"
	}
}

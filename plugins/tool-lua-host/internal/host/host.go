package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dsc/proto"
	"tool-lua-host/internal/bindings"

	lua "github.com/wippyai/go-lua"
)

// pollInterval 脚本目录轮询间隔（热加载）。
const pollInterval = 2 * time.Second

// JobEntry 脚本后台任务的状态。
type JobEntry struct {
	ID     string
	Script string
	Status string // running | completed | failed
	Result string
	Error  string
}

// Host LUA 脚本宿主：加载/热加载脚本，汇总脚本注册的工具。
type Host struct {
	mu       sync.Mutex
	dir      string
	services *bindings.Services
	scripts  map[string]*Script
	tools    map[string]*ToolDef // 全量工具表（key: 注册名，含 lua_ 前缀）
	stop     chan struct{}
	logf     func(string, ...any)

	store *bindings.Store        // 进程内 KV（脚本间共享）
	hook  *bindings.HookRegistry // 脚本注册的宿主钩子
	jobs  map[string]*JobEntry   // 后台任务表
	jobSeq int
}

// New 创建宿主（dir 为脚本目录，services 为宿主互通服务）。
func New(dir string, services *bindings.Services, logf func(string, ...any)) *Host {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	h := &Host{
		dir:      dir,
		services: services,
		scripts:  make(map[string]*Script),
		tools:    make(map[string]*ToolDef),
		stop:     make(chan struct{}),
		logf:     logf,
		store:    bindings.NewStore(),
		hook:     bindings.NewHookRegistry(),
		jobs:     make(map[string]*JobEntry),
	}
	// 宿主内建（KV/钩子/后台任务）由 host 提供实现
	services.Store = h.store
	services.Hook = h.hook
	services.HookRun = h.hookRun
	services.SpawnJob = h.spawnJob
	services.JobStatus = h.jobStatus
	services.JobList = h.jobList
	return h
}

// Start 初始加载脚本目录并启动热加载轮询。
func (h *Host) Start() error {
	if err := os.MkdirAll(h.dir, 0755); err != nil {
		return fmt.Errorf("lua-host: create scripts dir: %w", err)
	}
	h.scan()
	go h.poll()
	return nil
}

// Stop 停止热加载轮询。
func (h *Host) Stop() {
	close(h.stop)
}

// poll 轮询脚本目录（新脚本/变更/删除）。
func (h *Host) poll() {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.scan()
		}
	}
}

// scan 扫描脚本目录并增量加载/重载/卸载。
func (h *Host) scan() {
	h.mu.Lock()
	defer h.mu.Unlock()

	dirs := map[string]bool{}
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs[e.Name()] = true
		}
	}

	// 新增/变更
	for name := range dirs {
		path := filepath.Join(h.dir, name, "main.lua")
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		cur := hashContent(string(src))
		if s, ok := h.scripts[name]; ok {
			if s.hash != cur {
				h.logf("lua 脚本 %s 已变更，重载", name)
				h.unloadLocked(name)
				h.loadLocked(name)
			}
		} else {
			h.loadLocked(name)
		}
	}

	// 删除
	for name := range h.scripts {
		if !dirs[name] {
			h.logf("lua 脚本 %s 已删除，卸载", name)
			h.unloadLocked(name)
		}
	}
}

// loadLocked 加载脚本（需已持有 h.mu）。
func (h *Host) loadLocked(name string) {
	s, err := h.loadScript(name)
	if err != nil {
		h.logf("加载 lua 脚本 %s 失败: %v", name, err)
		return
	}
	h.scripts[name] = s
	h.logf("lua 脚本 %s 已加载（%d 个工具）", name, len(s.Tools))
}

// unloadLocked 卸载脚本（需已持有 h.mu）。
func (h *Host) unloadLocked(name string) {
	s, ok := h.scripts[name]
	if !ok {
		return
	}
	for _, t := range s.Tools {
		delete(h.tools, t)
	}
	s.mu.Lock()
	s.L.Close()
	s.mu.Unlock()
	delete(h.scripts, name)
}

// registerTool 脚本注册工具的回调：注册名统一加 lua_ 前缀避免与其他插件冲突。
// 注意：由脚本加载路径（scan 已持有 h.mu）调用，此处不再加锁。
func (h *Host) registerTool(script *Script, name, desc, paramsJSON string, fn *lua.LFunction) error {
	full := "lua_" + name
	if _, exists := h.tools[full]; exists {
		return fmt.Errorf("工具 %s 已被注册", full)
	}
	h.tools[full] = &ToolDef{
		Name: full, Script: script.Name,
		Description: desc, ParamsJSON: paramsJSON, Handler: fn,
	}
	script.Tools = append(script.Tools, full)
	return nil
}

// ListTools 汇总所有脚本注册的工具。
func (h *Host) ListTools() []*proto.Tool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*proto.Tool, 0, len(h.tools))
	for _, t := range h.tools {
		out = append(out, &proto.Tool{
			Name:           t.Name,
			Description:    t.Description,
			ParametersJson: t.ParamsJSON,
		})
	}
	return out
}

// ExecuteTool 分发到脚本注册的工具 handler（在对应 VM 上调用）。
func (h *Host) ExecuteTool(name string, args json.RawMessage) (string, error) {
	h.mu.Lock()
	t, ok := h.tools[name]
	if !ok {
		h.mu.Unlock()
		return "", fmt.Errorf("lua-host: unknown tool %q", name)
	}
	script, ok := h.scripts[t.Script]
	h.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("lua-host: script %q not loaded", t.Script)
	}

	script.mu.Lock()
	defer script.mu.Unlock()
	L := script.L
	top := L.GetTop()
	defer L.SetTop(top) // 恢复栈，避免污染后续执行
	argVal := jsonToLua(L, args)
	if err := L.CallByParam(lua.P{Fn: t.Handler, NRet: 1, Protect: true}, argVal); err != nil {
		return "", fmt.Errorf("lua 工具 %s 执行失败: %w", name, err)
	}
	ret := L.Get(-1)
	return luaResultToString(L, ret), nil
}

// ==================== 钩子执行（dsc.hook） ====================

// HookSnapshots 返回三类钩子 handler 的快照（供 PluginHookService 实现使用）。
func (h *Host) HookSnapshots() (before, after, onEvent []bindings.HookHandler) {
	return h.hook.Snapshots()
}

// RunHooks 在对应脚本 VM 上执行钩子集合（供 PluginHookService 实现使用）。
func (h *Host) RunHooks(kind string, handlers []bindings.HookHandler, args ...any) []any {
	return h.hookRun(kind, handlers, args...)
}

// hookRun 在对应脚本的 VM 上执行一批钩子 handler（按注册顺序），返回每个
// handler 的返回值（转 any）。kind 决定调用签名：
//   - "before_tool": fn(tool_name, args_table) → (veto, error, new_args)
//   - "after_tool":  fn(tool_name, args_table, result, error) → (new_result, new_error)
//   - "on_event":    fn(name, data) → 无
func (h *Host) hookRun(kind string, handlers []bindings.HookHandler, args ...any) []any {
	out := make([]any, 0, len(handlers))
	for _, hd := range handlers {
		h.mu.Lock()
		script, ok := h.scripts[hd.Script]
		h.mu.Unlock()
		if !ok {
			continue // 脚本已卸载
		}
		// TryLock：VM 忙（该脚本工具/任务正持锁执行，如嵌套 dsc.tool.call 触发
		// 宿主钩子回调）时跳过，避免同一 VM 重入死锁
		if !script.mu.TryLock() {
			h.logf("lua 钩子 %s（%s）跳过：VM 忙", kind, hd.Script)
			continue
		}
		L := script.L
		// 压入 handler 与参数（转 LUA 值）
		pushed := []lua.LValue{hd.Fn}
		for _, a := range args {
			pushed = append(pushed, anyToLua(L, a))
		}
		if err := L.CallByParam(lua.P{Fn: hd.Fn, NRet: lua.MultRet, Protect: true}, pushed[1:]...); err != nil {
			h.logf("lua 钩子 %s（%s）执行失败: %v", kind, hd.Script, err)
			script.mu.Unlock()
			continue
		}
		n := L.GetTop()
		var results []any
		for i := 1; i <= n; i++ {
			results = append(results, luaToAnySafeDepth(L.Get(i), 0))
		}
		L.SetTop(0)
		script.mu.Unlock()
		out = append(out, results)
	}
	return out
}

// luaToAnySafe 把 LUA 值转 any（table 递归；带深度限制防自引用环）。
func luaToAnySafe(v lua.LValue) any {
	return luaToAnySafeDepth(v, 0)
}

func luaToAnySafeDepth(v lua.LValue, depth int) any {
	if depth > 16 {
		return "<cycle>"
	}
	switch t := v.(type) {
	case *lua.LTable:
		var arr []any
		for i := 1; ; i++ {
			item := t.RawGetInt(i)
			if item == lua.LNil {
				break
			}
			arr = append(arr, luaToAnySafeDepth(item, depth+1))
		}
		if arr != nil {
			return arr
		}
		out := map[string]any{}
		t.ForEach(func(k, val lua.LValue) {
			out[k.String()] = luaToAnySafeDepth(val, depth+1)
		})
		return out
	case lua.LString:
		return string(t)
	case lua.LNumber:
		return float64(t)
	case lua.LInteger:
		return int64(t)
	case lua.LBool:
		return bool(t)
	case nil:
		return nil
	default:
		// 不调 v.String()：LGoFunc.String 内部 fmt 打印指针可能触发栈溢出
		return "<" + v.Type().String() + ">"
	}
}

// ==================== 后台任务（dsc.job） ====================

// spawnJob 在脚本的 VM 上后台执行 fn（goroutine；VM 由脚本锁串行化）。
func (h *Host) spawnJob(script string, fn *lua.LFunction) (string, error) {
	h.mu.Lock()
	h.jobSeq++
	id := fmt.Sprintf("job-%d", h.jobSeq)
	entry := &JobEntry{ID: id, Script: script, Status: "running"}
	h.jobs[id] = entry
	h.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				entry.Status = "failed"
				entry.Error = fmt.Sprintf("panic: %v", r)
				h.logf("lua job %s panic: %v", id, r)
			}
		}()
		h.mu.Lock()
		s, ok := h.scripts[script]
		h.mu.Unlock()
		if !ok {
			entry.Status = "failed"
			entry.Error = "script unloaded"
			return
		}
		s.mu.Lock()
		top := s.L.GetTop()
		defer func() {
			s.L.SetTop(top) // 恢复栈，避免污染其他工具/钩子执行
			s.mu.Unlock()
		}()
		if err := s.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
			entry.Status = "failed"
			entry.Error = err.Error()
			return
		}
		ret := s.L.Get(-1)
		entry.Status = "completed"
		entry.Result = luaResultToString(s.L, ret)
	}()
	return id, nil
}

// jobStatus 查询后台任务状态。
func (h *Host) jobStatus(id string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.jobs[id]
	if !ok {
		return "", fmt.Errorf("lua-host: no such job %q", id)
	}
	if e.Status == "completed" {
		return fmt.Sprintf("completed: %s", e.Result), nil
	}
	if e.Status == "failed" {
		return fmt.Sprintf("failed: %s", e.Error), nil
	}
	return "running", nil
}

// jobList 列出全部后台任务。
func (h *Host) jobList() (map[string]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]string, len(h.jobs))
	for id, e := range h.jobs {
		out[id] = e.Status
	}
	return out, nil
}

// jsonToLua 把 JSON 参数转成 LUA 值。
func jsonToLua(L *lua.LState, raw json.RawMessage) lua.LValue {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return lua.LNil
	}
	return anyToLua(L, v)
}

func anyToLua(L *lua.LState, v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(t)
	case float64:
		return lua.LNumber(t)
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
		for k, item := range t {
			L.SetField(tbl, k, anyToLua(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", t))
	}
}

// luaResultToString 把工具 handler 的返回值转成文本：字符串直用，table 序列化为 JSON。
func luaResultToString(L *lua.LState, v lua.LValue) string {
	if v == lua.LNil {
		return ""
	}
	switch t := v.(type) {
	case lua.LString:
		return string(t)
	case *lua.LTable:
		b, err := json.Marshal(luaTableToAny(L, t))
		if err != nil {
			return t.String()
		}
		return string(b)
	default:
		return v.String()
	}
}

// luaTableToAny 把 LUA table 转 any（供 JSON 序列化；简单实现，仅一层+嵌套）。
func luaTableToAny(L *lua.LState, tbl *lua.LTable) any {
	// 数组优先
	var arr []any
	for i := 1; ; i++ {
		item := tbl.RawGetInt(i)
		if item == lua.LNil {
			break
		}
		arr = append(arr, luaValueToAny(L, item))
	}
	if arr != nil {
		return arr
	}
	out := map[string]any{}
	tbl.ForEach(func(k, val lua.LValue) {
		out[k.String()] = luaValueToAny(L, val)
	})
	return out
}

func luaValueToAny(L *lua.LState, v lua.LValue) any {
	switch t := v.(type) {
	case *lua.LTable:
		return luaTableToAny(L, t)
	case lua.LString:
		return string(t)
	case lua.LNumber:
		return float64(t)
	case lua.LInteger:
		return int64(t)
	case lua.LBool:
		return bool(t)
	default:
		// 不调 v.String()：LGoFunc.String 内部 fmt 打印指针可能触发栈溢出
		return "<" + v.Type().String() + ">"
	}
}

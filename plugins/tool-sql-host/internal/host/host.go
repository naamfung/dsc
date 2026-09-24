// Package host 实现 tool-sql-host 的插件宿主（见 vm.go 顶部说明）。
package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	dsc "dsc-sdk"
	"dsc/proto"
	"tool-sql-host/internal/bindings"
	"tool-sql-host/internal/dsp"

	lua "github.com/wippyai/go-lua"
)

// pollInterval 插件目录轮询间隔（热加载）。
const pollInterval = 2 * time.Second

// JobEntry 插件后台任务的状态。
type JobEntry struct {
	ID     string
	Plugin string
	Status string // running | completed | failed
	Result string
	Error  string
}

// PluginInfo 是已加载 .dsp 插件的对外概览（供 list_dsp_plugins 展示/审计）。
type PluginInfo struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Language  string   `json:"language"`
	Entry     string   `json:"entry"`
	Artifact  string   `json:"artifact"`
	ReadOnly  bool     `json:"readonly"`
	Tools     []string `json:"tools"`
	StateKeys int      `json:"state_keys"`
	Blobs     int      `json:"blobs"`
}

// Host 插件宿主：加载/热加载 .dsp 插件，汇总插件注册的工具。
type Host struct {
	mu       sync.Mutex
	dirs     []string // 插件目录列表（.dsp 文件散落在目录内，多个目录等价）
	services *bindings.Services
	plugins  map[string]*Plugin
	tools    map[string]*ToolDef // 全量工具表（key: 注册名，含插件名前缀）
	stop     chan struct{}
	stopOne  sync.Once
	logf     func(string, ...any)
	creation bool // 创造模式：允许热加载新增/变更插件；否则只运行启动时已存在的插件

	hook   *bindings.HookRegistry // 插件注册的宿主钩子
	jobs   map[string]*JobEntry   // 后台任务表
	jobSeq int
}

// New 创建宿主（dirs 为插件目录列表，均可承载 .dsp 且等价；services 为宿主互通
// 服务，creation 表示是否创造模式——仅创造模式允许热加载新增/变更插件）。
func New(dirs []string, services *bindings.Services, creation bool, logf func(string, ...any)) *Host {
	if services == nil {
		services = &bindings.Services{}
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	h := &Host{
		dirs:     dirs,
		services: services,
		plugins:  make(map[string]*Plugin),
		tools:    make(map[string]*ToolDef),
		stop:     make(chan struct{}),
		logf:     logf,
		creation: creation,
		hook:     bindings.NewHookRegistry(),
		jobs:     make(map[string]*JobEntry),
	}
	// 宿主内建（钩子/后台任务）由 host 提供实现
	services.Hook = h.hook
	services.HookRun = h.hookRun
	services.SpawnJob = h.spawnJob
	services.JobStatus = h.jobStatus
	services.JobList = h.jobList
	return h
}

// Start 初始加载各插件目录并（创造模式下）启动热加载轮询。
func (h *Host) Start() error {
	for _, d := range h.dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("sql-host: create plugin dir %s: %w", d, err)
		}
	}
	h.scan()
	if h.creation {
		go h.poll()
	}
	return nil
}

// Stop 停止热加载轮询并卸载全部插件（幂等；释放 VM 与插件库句柄）。
func (h *Host) Stop() {
	h.stopOne.Do(func() {
		close(h.stop)
		h.mu.Lock()
		defer h.mu.Unlock()
		for name := range h.plugins {
			h.unloadLocked(name)
		}
	})
}

// poll 轮询插件目录（新增插件/内容变更/删除）。
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

// scan 扫描所有插件目录并增量加载/重载/卸载。
// 变更检测按「源文件路径」而非插件名回查：dsp_meta.name 允许与文件基名不同，
// 若按下标（基名）回查，两者不一致时每个轮询周期都会误判为「删除 + 新增」。
func (h *Host) scan() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 汇总各目录下的 .dsp 文件（绝对路径 → 存在）。只认后缀为 .dsp 的普通文件：
	// SQLite 的 -journal/-wal/-shm 伴生文件后缀不为 .dsp，天然被排除在候选之外。
	seen := map[string]bool{}
	for _, d := range h.dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), dsp.Ext) {
				continue
			}
			abs, err := dsc.PAbs(dsc.PJoin(d, e.Name()))
			if err != nil {
				continue
			}
			seen[abs] = true
		}
	}

	byPath := make(map[string]*Plugin, len(h.plugins))
	for _, p := range h.plugins {
		byPath[p.Path] = p
	}

	// 新增/变更：比对产物哈希（只覆盖元数据与代码，故插件写自己的状态不会被误判为变更）。
	for path := range seen {
		loaded := byPath[path]
		if loaded == nil {
			h.loadLocked(path)
			continue
		}
		d, err := dsp.Open(path)
		if err != nil {
			continue
		}
		if d.Artifact == loaded.Artifact {
			continue
		}
		h.logf("插件 %s 已变更（%s），重载", loaded.Name, path)
		h.unloadLocked(loaded.Name)
		h.loadLocked(path)
	}

	// 移除
	for name, p := range h.plugins {
		if !seen[p.Path] {
			h.logf("插件 %s 已移除，卸载", name)
			h.unloadLocked(name)
		}
	}
}

// loadLocked 加载插件（需已持有 h.mu）。path 为 .dsp 文件路径。
func (h *Host) loadLocked(path string) {
	p, err := h.loadPlugin(path)
	if err != nil {
		h.logf("加载插件 %s 失败: %v", path, err)
		return
	}
	if _, exists := h.plugins[p.Name]; exists {
		h.logf("插件名 %s 已被占用（%s），忽略 %s", p.Name, h.plugins[p.Name].Path, p.Path)
		p.L.Close()
		return
	}
	h.plugins[p.Name] = p
	h.logf("插件 %s 已加载（%s，%d 个工具%s）", p.Name, p.Path, len(p.Tools), readOnlyNote(p))
}

func readOnlyNote(p *Plugin) string {
	if p.Dsp.ReadOnly {
		return "，只读"
	}
	return ""
}

// unloadLocked 卸载插件（需已持有 h.mu）。
func (h *Host) unloadLocked(name string) {
	p, ok := h.plugins[name]
	if !ok {
		return
	}
	for _, t := range p.Tools {
		delete(h.tools, t)
	}
	h.hook.Drop(name)
	p.mu.Lock()
	p.L.Close()
	p.mu.Unlock()
	delete(h.plugins, name)
}

// registerTool 插件注册工具的回调：注册名加上插件名前缀（见 vm.go 顶部说明），
// 既避免与其他插件重名，也让模型从工具名直接看出它出自哪个插件。
// 注意：由插件加载路径（scan 已持有 h.mu）调用，此处不再加锁。
func (h *Host) registerTool(p *Plugin, name, desc, paramsJSON string, fn *lua.LFunction) error {
	full, err := dsc.QualifyToolName(p.Name, name)
	if err != nil {
		return err
	}
	if _, exists := h.tools[full]; exists {
		return fmt.Errorf("工具 %s 已被注册", full)
	}
	h.tools[full] = &ToolDef{
		Name: full, Plugin: p.Name,
		Description: desc, ParamsJSON: paramsJSON, Handler: fn,
	}
	p.Tools = append(p.Tools, full)
	return nil
}

// ListTools 汇总所有插件注册的工具（按名升序，保证工具目录前缀稳定）。
func (h *Host) ListTools() []*proto.Tool {
	h.mu.Lock()
	defer h.mu.Unlock()
	names := make([]string, 0, len(h.tools))
	for n := range h.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*proto.Tool, 0, len(names))
	for _, n := range names {
		t := h.tools[n]
		out = append(out, &proto.Tool{
			Name:           t.Name,
			Description:    t.Description,
			ParametersJson: t.ParamsJSON,
		})
	}
	return out
}

// ListPlugins 返回已加载插件的概览（按名升序），供 list_dsp_plugins 展示与审计。
func (h *Host) ListPlugins() []PluginInfo {
	h.mu.Lock()
	plugins := make([]*Plugin, 0, len(h.plugins))
	for _, p := range h.plugins {
		plugins = append(plugins, p)
	}
	h.mu.Unlock()

	out := make([]PluginInfo, 0, len(plugins))
	for _, p := range plugins {
		tools := append([]string(nil), p.Tools...)
		sort.Strings(tools)
		blobs, err := p.Dsp.Blobs()
		if err != nil {
			h.logf("读取插件 %s 的 blob 清单失败: %v", p.Name, err)
		}
		out = append(out, PluginInfo{
			Name: p.Name, Path: p.Path, Language: p.Language, Entry: p.Entry,
			Artifact: p.Artifact, ReadOnly: p.Dsp.ReadOnly, Tools: tools,
			StateKeys: p.Dsp.CountState(), Blobs: len(blobs),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ExecuteTool 分发到插件注册的工具 handler（在对应 VM 上调用）。
func (h *Host) ExecuteTool(name string, args json.RawMessage) (string, error) {
	h.mu.Lock()
	t, ok := h.tools[name]
	if !ok {
		h.mu.Unlock()
		return "", fmt.Errorf("sql-host: unknown tool %q", name)
	}
	p, ok := h.plugins[t.Plugin]
	h.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("sql-host: plugin %q not loaded", t.Plugin)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	L := p.L
	top := L.GetTop()
	defer L.SetTop(top) // 恢复栈，避免污染后续执行
	argVal := jsonToLua(L, args)
	if err := L.CallByParam(lua.P{Fn: t.Handler, NRet: 1, Protect: true}, argVal); err != nil {
		return "", fmt.Errorf("插件 %s 的工具 %s 执行失败: %w", p.Name, name, err)
	}
	ret := L.Get(-1)
	return luaResultToString(L, ret), nil
}

// ==================== 钩子执行（dsc.hook） ====================

// HookSnapshots 返回三类钩子 handler 的快照（供 PluginHookService 实现使用）。
func (h *Host) HookSnapshots() (before, after, onEvent []bindings.HookHandler) {
	return h.hook.Snapshots()
}

// RunHooks 在对应插件 VM 上执行钩子集合（供 PluginHookService 实现使用）。
func (h *Host) RunHooks(kind string, handlers []bindings.HookHandler, args ...any) []any {
	return h.hookRun(kind, handlers, args...)
}

// hookRun 在对应插件的 VM 上执行一批钩子 handler（按注册顺序），返回每个
// handler 的返回值（转 any）。kind 决定调用签名：
//   - "before_tool": fn(tool_name, args_table) → (veto, error, new_args)
//   - "after_tool":  fn(tool_name, args_table, result, error) → (new_result, new_error)
//   - "on_event":    fn(name, data) → 无
func (h *Host) hookRun(kind string, handlers []bindings.HookHandler, args ...any) []any {
	out := make([]any, 0, len(handlers))
	for _, hd := range handlers {
		h.mu.Lock()
		p, ok := h.plugins[hd.Plugin]
		h.mu.Unlock()
		if !ok {
			continue // 插件已卸载
		}
		// TryLock：VM 忙（该插件工具/任务正持锁执行，如嵌套 dsc.tool.call 触发
		// 宿主钩子回调）时跳过，避免同一 VM 重入死锁
		if !p.mu.TryLock() {
			h.logf("插件钩子 %s（%s）跳过：VM 忙", kind, hd.Plugin)
			continue
		}
		L := p.L
		pushed := make([]lua.LValue, 0, len(args))
		for _, a := range args {
			pushed = append(pushed, anyToLua(L, a))
		}
		if err := L.CallByParam(lua.P{Fn: hd.Fn, NRet: lua.MultRet, Protect: true}, pushed...); err != nil {
			h.logf("插件钩子 %s（%s）执行失败: %v", kind, hd.Plugin, err)
			p.mu.Unlock()
			continue
		}
		n := L.GetTop()
		var results []any
		for i := 1; i <= n; i++ {
			results = append(results, luaToAnySafeDepth(L.Get(i), 0))
		}
		L.SetTop(0)
		p.mu.Unlock()
		out = append(out, results)
	}
	return out
}

// luaToAnySafeDepth 把 LUA 值转 any（table 递归；带深度限制防自引用环）。
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

// spawnJob 在插件的 VM 上后台执行 fn（goroutine；VM 由插件锁串行化）。
func (h *Host) spawnJob(plugin string, fn *lua.LFunction) (string, error) {
	h.mu.Lock()
	h.jobSeq++
	id := fmt.Sprintf("job-%d", h.jobSeq)
	entry := &JobEntry{ID: id, Plugin: plugin, Status: "running"}
	h.jobs[id] = entry
	h.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				entry.Status = "failed"
				entry.Error = fmt.Sprintf("panic: %v", r)
				h.logf("插件 job %s panic: %v", id, r)
			}
		}()
		h.mu.Lock()
		p, ok := h.plugins[plugin]
		h.mu.Unlock()
		if !ok {
			entry.Status = "failed"
			entry.Error = "plugin unloaded"
			return
		}
		p.mu.Lock()
		top := p.L.GetTop()
		defer func() {
			p.L.SetTop(top) // 恢复栈，避免污染其他工具/钩子执行
			p.mu.Unlock()
		}()
		if err := p.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
			entry.Status = "failed"
			entry.Error = err.Error()
			return
		}
		ret := p.L.Get(-1)
		entry.Status = "completed"
		entry.Result = luaResultToString(p.L, ret)
	}()
	return id, nil
}

// jobStatus 查询后台任务状态。
func (h *Host) jobStatus(id string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.jobs[id]
	if !ok {
		return "", fmt.Errorf("sql-host: no such job %q", id)
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
		for _, k := range sortedKeys(t) {
			L.SetField(tbl, k, anyToLua(L, t[k]))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", t))
	}
}

// sortedKeys 按键升序返回 map 键（进模型/进 VM 的有序产物须显式稳定排序）。
func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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

// luaTableToAny 把 LUA table 转 any（供 JSON 序列化）。
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

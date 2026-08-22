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

// Host LUA 脚本宿主：加载/热加载脚本，汇总脚本注册的工具。
type Host struct {
	mu       sync.Mutex
	dir      string
	services *bindings.Services
	scripts  map[string]*Script
	tools    map[string]*ToolDef // 全量工具表（key: 注册名，含 lua_ 前缀）
	stop     chan struct{}
	logf     func(string, ...any)
}

// New 创建宿主（dir 为脚本目录，services 为宿主互通服务）。
func New(dir string, services *bindings.Services, logf func(string, ...any)) *Host {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Host{
		dir:      dir,
		services: services,
		scripts:  make(map[string]*Script),
		tools:    make(map[string]*ToolDef),
		stop:     make(chan struct{}),
		logf:     logf,
	}
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
	argVal := jsonToLua(L, args)
	if err := L.CallByParam(lua.P{Fn: t.Handler, NRet: 1, Protect: true}, argVal); err != nil {
		return "", fmt.Errorf("lua 工具 %s 执行失败: %w", name, err)
	}
	ret := L.Get(-1)
	L.Pop(1)
	return luaResultToString(L, ret), nil
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
	case lua.LBool:
		return bool(t)
	default:
		return v.String()
	}
}

package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tool-sql-host/internal/bindings"
	"tool-sql-host/internal/dsp"
)

// packSource 把给定源码打包为 <dir>/dsp/<name>.dsp，返回插件文件路径。
// 源目录内写一份 dsp.yaml 固定插件名（缺省名会回落到源目录基名，与插件名无关）。
func packSource(t *testing.T, dir, name, script string) string {
	t.Helper()
	src := filepath.Join(dir, name+"-src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name: " + name + "\nlanguage: lua\nentry: main.lua\n"
	if err := os.WriteFile(filepath.Join(src, "dsp.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.lua"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(dir, "dsp")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(pluginDir, name+".dsp")
	if _, err := dsp.Pack(src, out, nil); err != nil {
		t.Fatalf("pack %s: %v", name, err)
	}
	return out
}

// packExample 把仓库内的示例插件打包到 dir/dsp，返回插件目录。
func packExample(t *testing.T, dir string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, "dsp")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := dsp.Pack(filepath.Join("..", "..", "examples", "hello"),
		filepath.Join(pluginDir, "hello.dsp"), nil); err != nil {
		t.Fatalf("pack example: %v", err)
	}
	return pluginDir
}

// recorder 记录宿主事件（dsc.notify.emit 的落点），使示例插件的事件可被断言。
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) Notify(_ context.Context, name, dataJSON string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, name+" "+dataJSON)
	return nil
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// newHost 创建默认宿主：注入记录型 notifier，使示例插件的 dsc.notify.emit 可用。
func newHost(t *testing.T, dirs []string, creation bool) *Host {
	return newHostWith(t, dirs, creation, &bindings.Services{Notify: &recorder{}})
}

func newHostWith(t *testing.T, dirs []string, creation bool, svc *bindings.Services) *Host {
	t.Helper()
	h := New(dirs, svc, creation, t.Logf)
	t.Cleanup(h.Stop)
	if err := h.Start(); err != nil {
		t.Fatalf("host start: %v", err)
	}
	return h
}

// TestHostLoadsExample 验证示例 .dsp 被加载、工具注册到工具表，且三条内建路径
// （自持状态 / 自身库 SQL / 多文件脚本）在真实执行下都通。
func TestHostLoadsExample(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	h := newHostWith(t, []string{packExample(t, dir)}, true, &bindings.Services{Notify: rec})

	for _, want := range []string{"sql_hello", "sql_note", "sql_stats"} {
		if !h.hasTool(want) {
			t.Fatalf("工具 %s 未注册，实际：%v", want, toolNames(h))
		}
	}

	out, err := h.ExecuteTool("sql_hello", json.RawMessage(`{"name":"DSC"}`))
	if err != nil {
		t.Fatalf("sql_hello: %v", err)
	}
	if !strings.Contains(out, "你好，DSC") || !strings.Contains(out, "第 1 次调用") {
		t.Fatalf("sql_hello 结果 = %q", out)
	}
	// dsc.notify.emit 落到宿主事件总线
	events := rec.all()
	if len(events) != 1 || !strings.Contains(events[0], "sql/hello") || !strings.Contains(events[0], `"who":"DSC"`) {
		t.Fatalf("宿主事件 = %v", events)
	}

	// 自持状态：第二次调用计数递增（状态在插件自身 .dsp 内）
	out, err = h.ExecuteTool("sql_hello", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("sql_hello #2: %v", err)
	}
	if !strings.Contains(out, "第 2 次调用") || !strings.Contains(out, "你好，world") {
		t.Fatalf("sql_hello #2 结果 = %q", out)
	}

	// 自身库 SQL：自有表 note 由打包期 schema.sql 建立
	out, err = h.ExecuteTool("sql_note", json.RawMessage(`{"text":"第一条"}`))
	if err != nil {
		t.Fatalf("sql_note: %v", err)
	}
	if !strings.Contains(out, "第一条") || !strings.Contains(out, "共 1 条") {
		t.Fatalf("sql_note 结果 = %q", out)
	}

	// 自省
	out, err = h.ExecuteTool("sql_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("sql_stats: %v", err)
	}
	if !strings.Contains(out, "插件=hello") || !strings.Contains(out, "note") {
		t.Fatalf("sql_stats 结果 = %q", out)
	}
}

// TestStatePersistsAcrossHostRestart 钉死「插件读写自身」的核心闭环：
// 宿主重启（新 Host、新 VM）后，插件此前写入的状态仍在——因为状态就存在它自己那个文件里。
func TestStatePersistsAcrossHostRestart(t *testing.T) {
	dir := t.TempDir()
	pluginDir := packExample(t, dir)

	h1 := newHost(t, []string{pluginDir}, true)
	if _, err := h1.ExecuteTool("sql_hello", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := h1.ExecuteTool("sql_hello", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	h1.Stop()

	h2 := newHost(t, []string{pluginDir}, true)
	out, err := h2.ExecuteTool("sql_hello", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("重启后 sql_hello: %v", err)
	}
	if !strings.Contains(out, "第 3 次调用") {
		t.Fatalf("状态未随插件文件持久化：%q", out)
	}
}

// TestHotReload 验证热加载：替换 .dsp 后新工具生效，删除 .dsp 后工具卸载。
func TestHotReload(t *testing.T) {
	dir := t.TempDir()
	const script = `dsc.register_tool("hello", { description = "hello" }, function() return "hello" end)`
	packSource(t, dir, "demo", script)
	pluginDir := filepath.Join(dir, "dsp")

	h := newHost(t, []string{pluginDir}, true)
	assertHasTool(t, h, "sql_hello", true)

	// 重新打包同一 .dsp：新增 world 工具（内容哈希变化 → 轮询重载）
	src := filepath.Join(dir, "demo-src")
	updated := script + `
dsc.register_tool("world", { description = "world" }, function() return "world" end)`
	if err := os.WriteFile(filepath.Join(src, "main.lua"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dsp.Pack(src, filepath.Join(pluginDir, "demo.dsp"), nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return h.hasTool("sql_world") })
	out, err := h.ExecuteTool("sql_world", json.RawMessage(`{}`))
	if err != nil || out != "world" {
		t.Fatalf("重载后 sql_world = %q/%v", out, err)
	}

	// 删除 .dsp → 工具卸载
	if err := os.Remove(filepath.Join(pluginDir, "demo.dsp")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !h.hasTool("sql_hello") })
}

// TestRunOnlyMode 验证非创造模式约束：启动时已有插件被加载（可运行），
// 期间新增/替换 .dsp 不生效（热加载轮询被禁用）。
func TestRunOnlyMode(t *testing.T) {
	dir := t.TempDir()
	packSource(t, dir, "demo", `dsc.register_tool("existing", { description = "e" }, function() return "ok" end)`)
	pluginDir := filepath.Join(dir, "dsp")

	h := newHost(t, []string{pluginDir}, false) // 非创造模式
	assertHasTool(t, h, "sql_existing", true)
	if out, err := h.ExecuteTool("sql_existing", json.RawMessage(`{}`)); err != nil || out != "ok" {
		t.Fatalf("sql_existing = %q/%v", out, err)
	}

	// 期间新增另一个 .dsp：非创造模式不生效
	otherSrc := filepath.Join(dir, "other-src")
	if err := os.MkdirAll(otherSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherSrc, "main.lua"),
		[]byte(`dsc.register_tool("brandnew", { description = "n" }, function() return "new" end)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dsp.Pack(otherSrc, filepath.Join(pluginDir, "other.dsp"), nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second) // 超过 pollInterval
	assertHasTool(t, h, "sql_brandnew", false)

	// 替换已有 .dsp：非创造模式同样不生效
	src := filepath.Join(dir, "demo-src")
	if err := os.WriteFile(filepath.Join(src, "main.lua"),
		[]byte(`dsc.register_tool("existing", { description = "e" }, function() return "changed" end)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dsp.Pack(src, filepath.Join(pluginDir, "demo.dsp"), nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	if out, _ := h.ExecuteTool("sql_existing", json.RawMessage(`{}`)); out != "ok" {
		t.Fatalf("非创造模式下插件被热替换：%q", out)
	}
}

// TestUnsupportedLanguageRejected 覆盖载体语言分派：非 lua 载体被显式拒绝（不静默降级）。
func TestUnsupportedLanguageRejected(t *testing.T) {
	dir := t.TempDir()
	packSource(t, dir, "demo", `dsc.register_tool("x", { description = "x" }, function() return "x" end)`)
	pluginDir := filepath.Join(dir, "dsp")

	// 用 native 载体语言重新打包同一个 .dsp
	src := filepath.Join(dir, "demo-src")
	if _, err := dsp.Pack(src, filepath.Join(pluginDir, "demo.dsp"),
		&dsp.Manifest{Name: "demo", Language: "native"}); err != nil {
		t.Fatal(err)
	}

	h := newHost(t, []string{pluginDir}, true)
	if h.hasTool("sql_x") {
		t.Fatal("非 lua 载体的插件不应注册任何工具")
	}
	if len(h.ListPlugins()) != 0 {
		t.Fatalf("非 lua 载体的插件不应被加载：%+v", h.ListPlugins())
	}
}

// TestListPluginsOverview 覆盖 list_sql_plugins 依赖的概览字段（名称/路径/工具/状态键）。
func TestListPluginsOverview(t *testing.T) {
	dir := t.TempDir()
	h := newHost(t, []string{packExample(t, dir)}, true)

	if _, err := h.ExecuteTool("sql_hello", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	infos := h.ListPlugins()
	if len(infos) != 1 {
		t.Fatalf("应有 1 个已加载插件，实际 %+v", infos)
	}
	p := infos[0]
	if p.Name != "hello" || p.Language != "lua" || p.Entry != "main.lua" {
		t.Fatalf("插件概览 = %+v", p)
	}
	if !filepath.IsAbs(p.Path) || !strings.HasSuffix(p.Path, "hello.dsp") {
		t.Fatalf("插件路径 = %q", p.Path)
	}
	if p.ReadOnly {
		t.Fatal("示例插件应为可写（状态需落盘）")
	}
	if p.StateKeys != 1 || p.Blobs != 2 { // hello_count，以及 main.lua + lib.lua
		t.Fatalf("状态键 / blob 数 = %d/%d", p.StateKeys, p.Blobs)
	}
	if len(p.Tools) != 3 {
		t.Fatalf("工具 = %v", p.Tools)
	}
	if len(p.Artifact) != 64 {
		t.Fatalf("产物哈希 = %q", p.Artifact)
	}
}

// TestStoreHookJob 验证 dsc.store / dsc.hook / dsc.job 内建在 .dsp 载体上的行为：
// store 落盘到插件自身库、钩子可被宿主侧执行、后台任务记录状态。
func TestStoreHookJob(t *testing.T) {
	dir := t.TempDir()
	script := `dsc.store.set("k", "v")
dsc.hook.before_tool(function(name, args) return false, "", { x = 1 } end)
dsc.hook.after_tool(function(name, args, result, err) return result .. "!", err end)
dsc.hook.on_event(function(name, data) end)
dsc.register_tool("counter", { description = "counter" }, function(args)
    local n = dsc.store.get("count") or 0
    dsc.store.set("count", n + 1)
    local job = dsc.job.spawn(function() return 42 end)
    return "n=" .. tostring(n) .. " job=" .. job
end)`
	packSource(t, dir, "demo", script)
	pluginDir := filepath.Join(dir, "dsp")

	h := newHost(t, []string{pluginDir}, true)
	assertHasTool(t, h, "sql_counter", true)

	// store 初始值（插件加载时 set 的 "k"="v"）落盘在插件自身库
	p, err := dsp.Open(filepath.Join(pluginDir, "demo.dsp"))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := p.StateGet("k"); !ok || v != "v" {
		t.Fatalf("插件自身库中的 k = %v/%v，期望 v", v, ok)
	}

	out1, err := h.ExecuteTool("sql_counter", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("counter #1: %v", err)
	}
	out2, err := h.ExecuteTool("sql_counter", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("counter #2: %v", err)
	}
	if !strings.Contains(out1, "n=0") || !strings.Contains(out2, "n=1") {
		t.Fatalf("store 计数未生效：%q / %q", out1, out2)
	}
	jobID := out2[strings.LastIndex(out2, "job=")+4:]

	deadline := time.Now().Add(3 * time.Second)
	var jobOut string
	for time.Now().Before(deadline) {
		st, err := h.jobStatus(jobID)
		if err == nil && strings.HasPrefix(st, "completed") {
			jobOut = st
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(jobOut, "42") {
		t.Fatalf("job 状态 = %q，期望 completed: 42", jobOut)
	}

	// 钩子注册数
	before, after, onEvent := h.HookSnapshots()
	if len(before) != 1 || len(after) != 1 || len(onEvent) != 1 {
		t.Fatalf("钩子数 before=%d after=%d onEvent=%d，期望 1/1/1", len(before), len(after), len(onEvent))
	}
	results := h.RunHooks("before_tool", before, "some_tool", map[string]any{"a": 1})
	vals, _ := results[0].([]any)
	if len(vals) < 3 {
		t.Fatalf("before_tool 返回 = %v，期望 3 个值", vals)
	}
	if newArgs, ok := vals[2].(map[string]any); !ok || newArgs["x"] != int64(1) {
		t.Fatalf("before_tool new_args = %v", vals[2])
	}
	results = h.RunHooks("after_tool", after, "some_tool", map[string]any{}, "ok", "")
	vals, _ = results[0].([]any)
	if len(vals) < 1 || vals[0] != "ok!" {
		t.Fatalf("after_tool 结果 = %v，期望 ok!", vals)
	}

	// 插件卸载后钩子随之移除（不留悬空 handler）
	h.unloadLocked("demo")
	before, after, onEvent = h.HookSnapshots()
	if len(before)+len(after)+len(onEvent) != 0 {
		t.Fatalf("卸载后仍有残留钩子：%d/%d/%d", len(before), len(after), len(onEvent))
	}
}

// ==================== 测试辅助 ====================

func (h *Host) hasTool(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.tools[name]
	return ok
}

func toolNames(h *Host) []string {
	var out []string
	for _, t := range h.ListTools() {
		out = append(out, t.GetName())
	}
	return out
}

func assertHasTool(t *testing.T, h *Host, name string, want bool) {
	t.Helper()
	if got := h.hasTool(name); got != want {
		t.Fatalf("工具 %s 存在 = %v，期望 %v（实际工具：%v）", name, got, want, toolNames(h))
	}
}

// waitFor 轮询等待条件成立（热加载轮询 2s，上限 8s）。
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

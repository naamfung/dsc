package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tool-lua-host/internal/bindings"
)

// TestHostLoadsExample 验证 example 脚本被加载且工具注册到工具表。
func TestHostLoadsExample(t *testing.T) {
	h := New("../../scripts", &bindings.Services{}, true, t.Logf)
	defer h.Stop()
	if err := h.Start(); err != nil {
		t.Fatalf("host start: %v", err)
	}

	tools := h.ListTools()
	if len(tools) == 0 {
		t.Fatal("no tools registered from scripts")
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.GetName()] = true
		t.Logf("registered tool: %s", tl.GetName())
	}
	for _, want := range []string{"lua_summarize", "lua_ping"} {
		if !names[want] {
			t.Fatalf("tool %s not registered", want)
		}
	}

	// 执行 lua_ping（经 dsc.tool.call 调 shell——services 为 nil 时应报错）
	_, err := h.ExecuteTool("lua_ping", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("lua_ping should fail when tool service is nil")
	}
	t.Logf("lua_ping without host services error (expected): %v", err)
}

// TestHotReload 验证热加载：修改脚本文件后，轮询自动重载并使新工具生效。
func TestHotReload(t *testing.T) {
	dir := t.TempDir()
	demoDir := filepath.Join(dir, "demo")
	if err := os.MkdirAll(demoDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainLua := filepath.Join(demoDir, "main.lua")
	write := func(content string) {
		if err := os.WriteFile(mainLua, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(`dsc.register_tool("hello", { description = "hello tool" }, function() return "hello" end)`)

	h := New(dir, &bindings.Services{}, true, t.Logf)
	defer h.Stop()
	if err := h.Start(); err != nil {
		t.Fatalf("host start: %v", err)
	}
	assertHasTool(t, h, "lua_hello", true)

	// 修改脚本：新增 world 工具
	write(`dsc.register_tool("hello", { description = "hello tool" }, function() return "hello" end)
dsc.register_tool("world", { description = "world tool" }, function() return "world" end)`)

	// 等待轮询热加载（pollInterval 2s，上限 6s）
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if h.hasTool("lua_world") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assertHasTool(t, h, "lua_world", true)

	// 执行重载后的新工具
	out, err := h.ExecuteTool("lua_world", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute lua_world after reload: %v", err)
	}
	if out != "world" {
		t.Fatalf("lua_world = %q, want %q", out, "world")
	}

	// 删除脚本目录 → 工具被卸载
	if err := os.RemoveAll(demoDir); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if !h.hasTool("lua_hello") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assertHasTool(t, h, "lua_hello", false)
}

func (h *Host) hasTool(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.tools[name]
	return ok
}

func assertHasTool(t *testing.T, h *Host, name string, want bool) {
	t.Helper()
	if got := h.hasTool(name); got != want {
		t.Fatalf("tool %s present = %v, want %v", name, got, want)
	}
}

// TestStoreHookJob 验证 dsc.store / dsc.hook / dsc.job 内建：
//   - store：脚本间共享 KV 读写
//   - hook：脚本注册的 before/after/on_event 可被宿主侧执行
//   - job：脚本后台任务执行并记录状态
func TestStoreHookJob(t *testing.T) {
	dir := t.TempDir()
	demoDir := filepath.Join(dir, "demo")
	if err := os.MkdirAll(demoDir, 0755); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(filepath.Join(demoDir, "main.lua"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	h := New(dir, &bindings.Services{}, true, t.Logf)
	defer h.Stop()
	if err := h.Start(); err != nil {
		t.Fatalf("host start: %v", err)
	}
	assertHasTool(t, h, "lua_counter", true)

	// store 初始值（脚本加载时 set 的 "k"="v"）
	if v, ok := h.store.Get("k"); !ok || v != "v" {
		t.Fatalf("store k = %v/%v, want v", v, ok)
	}

	// 执行工具两次：store 计数共享递增 + 返回 job id
	out1, err := h.ExecuteTool("lua_counter", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("counter #1: %v", err)
	}
	t.Logf("counter #1: %s", out1)
	out2, err := h.ExecuteTool("lua_counter", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("counter #2: %v", err)
	}
	t.Logf("counter #2: %s", out2)
	if !strings.Contains(out1, "n=0") || !strings.Contains(out2, "n=1") {
		t.Fatalf("store counter not shared: %q / %q", out1, out2)
	}
	jobID := strings.TrimPrefix(out2[strings.LastIndex(out2, "job=")+4:], "")

	// job 完成（异步，轮询）
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
		t.Fatalf("job result = %q, want completed: 42", jobOut)
	}
	t.Logf("job status: %s", jobOut)

	// hook：before_tool 改写参数
	before, after, onEvent := h.HookSnapshots()
	if len(before) != 1 || len(after) != 1 || len(onEvent) != 1 {
		t.Fatalf("hook counts before=%d after=%d onEvent=%d, want 1/1/1", len(before), len(after), len(onEvent))
	}
	results := h.RunHooks("before_tool", before, "some_tool", map[string]any{"a": 1})
	if len(results) != 1 {
		t.Fatalf("before_tool results = %d, want 1", len(results))
	}
	vals, _ := results[0].([]any)
	if len(vals) < 3 {
		t.Fatalf("before_tool returns = %v, want 3 values", vals)
	}
	newArgs, ok := vals[2].(map[string]any)
	if !ok {
		t.Fatalf("before_tool new_args = %v, want map", vals[2])
	}
	if x, ok := newArgs["x"].(int64); !ok || x != 1 {
		t.Fatalf("before_tool new_args.x = %v, want int64(1)", newArgs["x"])
	}

	// after_tool 改写结果
	results = h.RunHooks("after_tool", after, "some_tool", map[string]any{}, "ok", "")
	vals, _ = results[0].([]any)
	if len(vals) < 1 || vals[0] != "ok!" {
		t.Fatalf("after_tool result = %v, want ok!", vals)
	}
}

// TestRunOnlyMode 验证非创造模式（creation=false）约束：
// 启动时已有脚本被加载（可运行），期间新增/修改脚本不生效（热加载轮询被禁用）。
func TestRunOnlyMode(t *testing.T) {
	dir := t.TempDir()
	demoDir := filepath.Join(dir, "demo")
	if err := os.MkdirAll(demoDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := `dsc.register_tool("existing", { description = "e" }, function() return "ok" end)`
	if err := os.WriteFile(filepath.Join(demoDir, "main.lua"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	h := New(dir, &bindings.Services{}, false, t.Logf) // 非创造模式
	defer h.Stop()
	if err := h.Start(); err != nil {
		t.Fatalf("host start: %v", err)
	}
	// 已有脚本被加载，工具可运行
	assertHasTool(t, h, "lua_existing", true)
	if out, err := h.ExecuteTool("lua_existing", json.RawMessage(`{}`)); err != nil || out != "ok" {
		t.Fatalf("lua_existing = %q/%v, want ok", out, err)
	}

	// 期间新增脚本：非创造模式不生效（无热加载轮询）
	newDir := filepath.Join(dir, "newone")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(newDir, "main.lua"),
		[]byte(`dsc.register_tool("brandnew", { description = "n" }, function() return "new" end)`), 0644)
	time.Sleep(3 * time.Second) // 超过 pollInterval
	assertHasTool(t, h, "lua_brandnew", false)

	// 修改已有脚本：非创造模式同样不生效
	os.WriteFile(filepath.Join(demoDir, "main.lua"),
		[]byte(`dsc.register_tool("existing", { description = "e" }, function() return "changed" end)`), 0644)
	time.Sleep(3 * time.Second)
	if out, _ := h.ExecuteTool("lua_existing", json.RawMessage(`{}`)); out != "ok" {
		t.Fatalf("lua_existing after edit = %q, want ok (unchanged)", out)
	}
}

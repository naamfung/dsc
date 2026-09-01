package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// mockAgent 实现 Agent 接口，用于测试 agent 再激活时捕获 RegisterServices 调用。
type mockAgent struct {
	mu            sync.Mutex
	registerCalls []struct{ llm, tool uint32 }
}

func (a *mockAgent) Run(context.Context, string, []string) (*AgentResult, error) { return nil, nil }
func (a *mockAgent) RunStream(context.Context, string, []string) (<-chan *RunStreamResponse, error) {
	return nil, nil
}
func (a *mockAgent) Name(context.Context) string    { return "mock" }
func (a *mockAgent) Version(context.Context) string { return "1.0.0" }
func (a *mockAgent) RegisterServices(_ context.Context, llm, tool uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.registerCalls = append(a.registerCalls, struct{ llm, tool uint32 }{llm, tool})
	return nil
}
func (a *mockAgent) SwitchSession(context.Context, string) error           { return nil }
func (a *mockAgent) SetPlanMode(context.Context, bool) error               { return nil }
func (a *mockAgent) SetHistoryInjection(context.Context, int) error        { return nil }
func (a *mockAgent) SetUserQuestionsService(context.Context, uint32) error { return nil }
func (a *mockAgent) Shutdown(context.Context, bool) error                  { return nil }
func (a *mockAgent) InjectMessage(context.Context, string, []string) error { return nil }
func (a *mockAgent) DebugSnapshot(context.Context) (*AgentDebugSnapshot, error) {
	return &AgentDebugSnapshot{SessionID: "mock"}, nil
}

func (a *mockAgent) calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.registerCalls)
}

// TestPersistInjectionUpsertAndRemoval 校验动态注入的配置持久化：同名注入覆盖为单条，
// 卸载移除后条目消失。
func TestPersistInjectionUpsertAndRemoval(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	m := NewManager(&ManagerConfig{})
	m.SetConfigPath(cfgPath)

	entry := PluginEntry{Name: "tool-foo", Type: "tool", BinaryPath: "./plugins/tool-foo/tool-foo.exe"}

	m.mu.Lock()
	if err := m.persistInjectionLocked(entry); err != nil {
		t.Fatalf("persist injection: %v", err)
	}
	// 再次注入同名（更新 env），应覆盖而非追加
	entry2 := entry
	entry2.Env = map[string]string{"K": "v"}
	if err := m.persistInjectionLocked(entry2); err != nil {
		t.Fatalf("persist injection (upsert): %v", err)
	}
	if err := m.persistRemovalLocked(entry.Name); err != nil {
		t.Fatalf("persist removal: %v", err)
	}
	m.mu.Unlock()

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config after ops: %v", err)
	}
	for _, e := range cfg.Plugins {
		if e.Name == entry.Name {
			t.Fatalf("entry %s should be removed from config", entry.Name)
		}
	}
}

// TestPersistInjectionNormalizesBinaryPath 校验写回 config.yaml 的 binary_path
// 归一为相对正斜杆形式（./plugins/<name>/<name>.exe）：load_dsc_plugin persist
// 会用 filepath.Join(ExecDir,...) 生成绝对反斜杆路径，原样写入会在把部署目录
// 拷贝/迁移到其他机器后失效；应改为相对路径。
func TestPersistInjectionNormalizesBinaryPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("plugins: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(&ManagerConfig{ExecDir: dir})
	m.SetConfigPath(cfgPath)

	// 插件根下的绝对路径（模拟 load_dsc_plugin persist 生成的 Windows 反斜杆绝对路径）
	abs := filepath.Join(dir, "plugins", "tool-musicplayer", "tool-musicplayer.exe")
	m.mu.Lock()
	if err := m.persistInjectionLocked(PluginEntry{Name: "tool-musicplayer", Type: "tool", BinaryPath: abs}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	m.mu.Unlock()

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	wantPrefix := "./plugins/tool-musicplayer/tool-musicplayer"
	if !strings.Contains(content, "binary_path: "+wantPrefix) {
		t.Fatalf("config should contain relative forward-slash binary_path:\n%s", content)
	}
	// .exe 后缀按平台调整（复用 normalizeBinaryPath）：Windows 保留、其它平台去掉
	if runtime.GOOS == "windows" && !strings.Contains(content, wantPrefix+".exe") {
		t.Fatalf("windows config should keep .exe suffix:\n%s", content)
	}
	if runtime.GOOS != "windows" && strings.Contains(content, wantPrefix+".exe") {
		t.Fatalf("non-windows config should drop .exe suffix:\n%s", content)
	}
	if strings.Contains(content, "\\") {
		t.Fatalf("config should not contain backslash paths:\n%s", content)
	}

	// 插件根外的绝对路径：保留绝对形态但复用 normalizeBinaryPath（正斜杆 + 平台扩展名）
	outside := filepath.Join(dir, "..", "elsewhere", "tool-x.exe")
	m.mu.Lock()
	if err := m.persistInjectionLocked(PluginEntry{Name: "tool-x", Type: "tool", BinaryPath: outside}); err != nil {
		t.Fatalf("persist outside: %v", err)
	}
	m.mu.Unlock()
	data, _ = os.ReadFile(cfgPath)
	content = string(data)
	// YAML 对含冒号的路径值会加引号，这里只断言 normalizeBinaryPath 后的路径本身出现
	wantOutside := normalizeBinaryPath(filepath.Clean(outside))
	if !strings.Contains(content, wantOutside) {
		t.Fatalf("outside-root absolute path should be kept (normalized):\n%s", content)
	}
	if strings.Contains(content, "\\") {
		t.Fatalf("config should not contain backslash paths:\n%s", content)
	}
}

// TestPersistInjectionPreservesFile 校验 yaml.Node 保留法：
// 注入写回只增改 plugins 序列——原文件的注释与未声明字段的缺省语义保留，
// 不再把零值字段补齐进 config.yaml。
func TestPersistInjectionPreservesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	original := "# 顶部注释\nworkspace_root: ./workspace\n" +
		"# context 窗口注释\n" +
		"plugins:\n" +
		"  - name: agent-a\n    type: agent\n" +
		"    depends_on:\n      llm: llm-x\n      tools: [tool-x]\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(&ManagerConfig{})
	m.SetConfigPath(cfgPath)
	m.mu.Lock()
	if err := m.persistInjectionLocked(PluginEntry{Name: "tool-new", Type: "tool", BinaryPath: "./x"}); err != nil {
		t.Fatalf("persist injection: %v", err)
	}
	m.mu.Unlock()

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, keep := range []string{"顶部注释", "# context 窗口注释", "workspace_root: ./workspace", "llm: llm-x", "agent-a"} {
		if !strings.Contains(content, keep) {
			t.Errorf("original content %q lost after persist:\n%s", keep, content)
		}
	}
	if strings.Contains(content, "workspace_protection_enabled") {
		t.Errorf("persist injected zero-value field not present in original file:\n%s", content)
	}
	if !strings.Contains(content, "tool-new") {
		t.Errorf("injected entry missing after persist:\n%s", content)
	}
	if strings.Contains(content, "depends_on: null") {
		t.Errorf("injected entry should not contain empty depends_on:\n%s", content)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config after persist: %v", err)
	}
	if len(cfg.Plugins) != 2 {
		t.Fatalf("plugins = %d, want 2 (agent-a + tool-new)", len(cfg.Plugins))
	}
	var newEntry *PluginEntry
	for i := range cfg.Plugins {
		if cfg.Plugins[i].Name == "tool-new" {
			newEntry = &cfg.Plugins[i]
		}
	}
	if newEntry == nil || !newEntry.Enabled {
		t.Fatalf("injected entry should persist as enabled, got %+v", newEntry)
	}
}

// TestDeferPendingRecordsAndPersists 校验依赖未满足的注入条目进入 PENDING、
// 记录待办并写回配置。
func TestDeferPendingRecordsAndPersists(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	m := NewManager(&ManagerConfig{})
	m.SetConfigPath(cfgPath)

	entry := PluginEntry{Name: "llm-late", Type: "llm", DependsOn: &PluginDepends{LLM: "no-such-llm"}}
	m.mu.Lock()
	if err := m.deferPendingLocked(entry); err != nil {
		t.Fatalf("defer pending: %v", err)
	}
	if _, ok := m.pendingEntries[entry.Name]; !ok {
		t.Error("pendingEntries should contain the deferred entry")
	}
	state := m.states[entry.Name].State
	m.mu.Unlock()

	if state != StatePending {
		t.Fatalf("state = %s, want pending", state)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0].Name != entry.Name {
		t.Fatalf("config should persist deferred entry, got %+v", cfg.Plugins)
	}
}

// TestEntryDepsSatisfied 校验声明式依赖判定：
// 指向已加载插件的依赖视为满足，指向非插件名（具体工具名）的引用不构成拓扑依赖。
func TestEntryDepsSatisfied(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.llmServiceIDs["llm-p"] = 41
	m.toolServiceIDs["tool-p"] = 42
	// 这两者「已登记但未加载」，构成阻塞性依赖引用；待其就绪后解除
	m.pendingEntries["llm-ghost"] = PluginEntry{Name: "llm-ghost", Type: "llm"}
	m.pendingEntries["tool-ghost"] = PluginEntry{Name: "tool-ghost", Type: "tool"}
	m.mu.Unlock()

	cases := []struct {
		name  string
		entry PluginEntry
		want  bool
	}{
		{"dep-llm-satisfied", PluginEntry{Type: "agent", DependsOn: &PluginDepends{LLM: "llm-p"}}, true},
		{"dep-tool-satisfied", PluginEntry{Type: "agent", DependsOn: &PluginDepends{Tools: []string{"tool-p"}}}, true},
		{"dep-llm-known-unloaded-unsatisfied", PluginEntry{Type: "agent", DependsOn: &PluginDepends{LLM: "llm-ghost"}}, false},
		{"dep-tool-known-unloaded-unsatisfied", PluginEntry{Type: "agent", DependsOn: &PluginDepends{Tools: []string{"tool-ghost"}}}, false},
		{"dep-none-satisfied", PluginEntry{Type: "tool"}, true},
		// 指向具体工具名（非插件名）不构成拓扑阻塞
		{"dep-non-core-tool-satisfied", PluginEntry{Type: "tool", DependsOn: &PluginDepends{Tools: []string{"read_file"}}}, true},
	}
	for _, tc := range cases {
		if got := m.entryDepsSatisfiedLocked(tc.entry); got != tc.want {
			t.Errorf("%s: depsSatisfied = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestAgentReactivate 校验 PENDING agent 在依赖 LLM 就绪后被重新注入 RegisterServices 并激活；
// 依赖未就绪时保持 PENDING 且不误激活。
func TestAgentReactivate(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	ag := &mockAgent{}

	m.mu.Lock()
	m.agents["agent-x"] = ag
	m.agentEntries["agent-x"] = PluginEntry{
		Name: "agent-x", Type: "agent",
		DependsOn: &PluginDepends{LLM: "llm-p"},
	}
	m.states["agent-x"] = &RuntimeState{Type: "agent", State: StatePending}
	m.mu.Unlock()

	// 依赖 LLM 尚未就绪 → 保持 PENDING
	m.mu.Lock()
	m.reactivateAgentLocked("agent-x")
	m.mu.Unlock()
	if ag.calls() != 0 {
		t.Fatalf("agent should not have been reactivated before its LLM dep is ready")
	}
	if st, _ := m.GetPluginState("agent-x"); st.State != StatePending {
		t.Fatalf("state = %s, want pending before dep ready", st.State)
	}
	if m.agentServiceIDs["agent-x"] != 0 {
		t.Fatalf("agentServiceID should stay 0 before reactivation")
	}

	// LLM 就绪 → 注入 RegisterServices(41, 0) 并激活。
	// 聚合 LLM 服务已预挂载（agentLLMServiceID=41），primary provider 就绪（llms["llm-p"]）
	m.mu.Lock()
	m.llms["llm-p"] = &mockLLMProvider{}
	m.agentLLMServiceID = 41
	m.reactivateAgentLocked("agent-x")
	m.mu.Unlock()

	if ag.calls() != 1 {
		t.Fatalf("agent should be reactivated once, got %d calls", ag.calls())
	}
	if st, _ := m.GetPluginState("agent-x"); st.State != StateActive {
		t.Fatalf("state = %s, want active after reactivation", st.State)
	}
	ag.mu.Lock()
	first := ag.registerCalls[0]
	ag.mu.Unlock()
	if first.llm != 41 || first.tool != 0 {
		t.Fatalf("RegisterServices got (llm=%d, tool=%d), want (41, 0)", first.llm, first.tool)
	}
	if m.agentServiceIDs["agent-x"] != 41 {
		t.Fatalf("agentServiceID = %d, want 41", m.agentServiceIDs["agent-x"])
	}
}

// TestReactivateNoopWhenNotPending 校验非 PENDING 的 agent 不会被再激活逻辑触碰。
func TestReactivateNoopWhenNotPending(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	ag := &mockAgent{}
	m.mu.Lock()
	m.agents["agent-y"] = ag
	m.llmServiceIDs["llm-p"] = 41
	m.agentEntries["agent-y"] = PluginEntry{
		Name: "agent-y", Type: "agent", DependsOn: &PluginDepends{LLM: "llm-p"},
	}
	m.states["agent-y"] = &RuntimeState{Type: "agent", State: StateActive}
	m.reactivateAgentLocked("agent-y")
	m.mu.Unlock()

	if ag.calls() != 0 {
		t.Fatalf("active agent should not be re-registered, got %d calls", ag.calls())
	}
}

// TestRepairPendingReactivateLoadedAgent 复现端到端验证发现的缺陷：
// PENDING agent 已随 LoadFromConfig 拉起（存在于 m.agents），但其 LLM 依赖缺失被记入
// pendingEntries。随后注入 LLM 触发 repairPendingLocked 时，必须对这类“已加载但未激活”
// 的 agent 执行 reactivateAgentLocked，而非仅因已加载就简单清除待办；且在 LLM 未就绪前
// 必须保留待办（reactivate 是幂等空操作），不能误移出导致永久丢失再激活机会。
func TestRepairPendingReactivateLoadedAgent(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	ag := &mockAgent{}
	agentEntry := PluginEntry{
		Name: "agent-x", Type: "agent", BinaryPath: "./agent",
		DependsOn: &PluginDepends{LLM: "anthropic"},
	}

	m.mu.Lock()
	m.agents["agent-x"] = ag               // 已拉起（与真实启动一致）
	m.agentEntries["agent-x"] = agentEntry // 记录含 DependsOn 的声明
	m.states["agent-x"] = &RuntimeState{Type: "agent", State: StatePending}
	m.pendingEntries["agent-x"] = agentEntry // 依赖缺失，待再激活
	m.mu.Unlock()

	// LLM 尚未就绪：repair 不应误激活（reactivate 幂等空操作），也不应清除待办
	if err := m.repairPendingLocked(); err != nil {
		t.Fatalf("repairPendingLocked errored: %v", err)
	}
	m.mu.RLock()
	_, stillPending := m.pendingEntries["agent-x"]
	m.mu.RUnlock()
	if ag.calls() != 0 {
		t.Fatalf("agent reactivated before LLM ready: %d calls", ag.calls())
	}
	if !stillPending {
		t.Fatalf("agent pending entry should remain while LLM dep is missing")
	}

	// 注入 LLM（primary provider 就绪、聚合服务已挂载）后再次 repair：
	// agent 必须被注入 RegisterServices 并置为 Active，且移出待办
	m.mu.Lock()
	m.llms["anthropic"] = &mockLLMProvider{}
	m.agentLLMServiceID = 55
	m.mu.Unlock()
	if err := m.repairPendingLocked(); err != nil {
		t.Fatalf("repairPendingLocked errored: %v", err)
	}
	if ag.calls() != 1 {
		t.Fatalf("loaded-but-pending agent should have been reactivated once, got %d calls", ag.calls())
	}
	if st, _ := m.GetPluginState("agent-x"); st.State != StateActive {
		t.Fatalf("state = %s, want active after repair with LLM ready", st.State)
	}
	m.mu.RLock()
	_, gone := m.pendingEntries["agent-x"]
	m.mu.RUnlock()
	if gone {
		t.Fatalf("agent-x still in pendingEntries after reactivation")
	}
	ag.mu.Lock()
	got := ag.registerCalls[0]
	ag.mu.Unlock()
	if got.llm != 55 {
		t.Fatalf("RegisterServices llm = %d, want 55", got.llm)
	}
}

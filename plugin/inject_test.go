package plugin

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

// mockAgent 实现 Agent 接口，用于测试 agent 再激活时捕获 RegisterServices 调用。
type mockAgent struct {
	mu            sync.Mutex
	registerCalls []struct{ llm, tool uint32 }
}

func (a *mockAgent) Run(context.Context, string) (*AgentResult, error) { return nil, nil }
func (a *mockAgent) RunStream(context.Context, string) (<-chan *RunStreamResponse, error) {
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
func (a *mockAgent) Shutdown(context.Context, bool) error { return nil }

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
		{"dep-non-plugin-tool-satisfied", PluginEntry{Type: "tool", DependsOn: &PluginDepends{Tools: []string{"read_file"}}}, true},
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

	// LLM 就绪 → 注入 RegisterServices(41, 0) 并激活
	m.mu.Lock()
	m.llmServiceIDs["llm-p"] = 41
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
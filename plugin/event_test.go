package plugin

import (
	"testing"
	"time"
)

// TestSubscribeReceivesTransition 校验事件总线：状态机每次推进都会发布 PluginEvent，
// 订阅者可实时收到状态迁移事件。
func TestSubscribeReceivesTransition(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	ch, cancel := m.Subscribe()
	defer cancel()

	m.mu.Lock()
	m.trackStateLocked("llm-a", "llm") // 预置 Spawned（不发布事件）
	m.transitionLocked("llm-a", StateConnecting, "")
	m.transitionLocked("llm-a", StateReady, "")
	m.mu.Unlock()

	var got []PluginState
	for len(got) < 2 {
		select {
		case ev := <-ch:
			got = append(got, ev.To)
		case <-time.After(time.Second):
			t.Fatalf("expected 2 events, got %d: %v", len(got), got)
		}
	}
	want := []PluginState{StateConnecting, StateReady}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("event[%d].To = %s, want %s", i, got[i], w)
		}
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2\n", len(got))
	}
}

// TestStopHookRunsAndClears 校验对称清理 hook：
// runStopHooks 会按序执行已注册的 hook，并在执行后清除，保证同一批 hook 只跑一次。
func TestStopHookRunsAndClears(t *testing.T) {
	m := NewManager(&ManagerConfig{})

	var ran int
	m.mu.Lock()
	m.addStopHookLocked("tool-x", func() error { ran++; return nil })
	m.addStopHookLocked("tool-x", func() error { ran++; return nil })
	m.runStopHooksLocked("tool-x")
	m.mu.Unlock()

	if ran != 2 {
		t.Fatalf("stop hooks executed %d times, want 2", ran)
	}

	m.mu.Lock()
	if _, ok := m.stopHooks["tool-x"]; ok {
		t.Error("stop hooks should be cleared after run")
	}
	m.mu.Unlock()
}
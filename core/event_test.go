package core

import (
	"runtime"
	"testing"
	"time"

	"dsc/session"
)

// TestDefaultSessionIDFollowsWorkspaceRoot 校验默认会话 id 与工作区根一致性：
// DefaultSessionID 应等于 SessionKeyForProject(WorkspaceRoot)，使 TUI 当前会话标识
// 与 agent 存档文件名（项目隔离）吻合，不再出现映射不到存档的假 "default"。
//
// 平台注意：SessionKeyForProject 在不同平台对反斜杠的处理不同——Linux/macOS 上
// `\` 是有效文件名字符（filepath.ToSlash 不转换），Windows 上才是分隔符。故
// Windows 路径样例（如 C:\Users\...）只在 Windows 上断言其转换结果；Linux/macOS
// 上用平台本地的样例（/home/user/DeepClean、/mnt/c/...），避免跨平台误失败。
func TestDefaultSessionIDFollowsWorkspaceRoot(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	orig := WorkspaceRoot
	defer func() { WorkspaceRoot = orig }()

	// 平台样例：选一个含分隔符的典型路径，断言其经 SessionKeyForProject 转换后的结果。
	// SessionKeyForProject 是平台无关的纯字符串变换，但 filepath.ToSlash 在不同平台
	// 对 `\` 的处理不同——Windows 上 `\` → `/`，Linux/macOS 上 `\` 不变（是合法文件名字符）。
	// 故 Windows 路径样例只在 Windows 上断言，其余平台用 Unix 样例。
	var samplePath, wantKey string
	if runtime.GOOS == "windows" {
		samplePath = `C:\Users\Administrator\Desktop\DeepClean`
		wantKey = "C--Users-Administrator-Desktop-DeepClean"
	} else {
		samplePath = "/home/jor/DeepClean"
		wantKey = "home-jor-DeepClean"
	}
	WorkspaceRoot = samplePath
	if got := m.DefaultSessionID(); got != wantKey {
		t.Errorf("DefaultSessionID() = %q, want %q (root=%q, platform=%s)",
			got, wantKey, samplePath, runtime.GOOS)
	}
	// 同时校验 DefaultSessionID 与 SessionKeyForProject(WorkspaceRoot) 等价——
	// 这是该测试的核心断言（不再有假 "default"），跨平台都应成立。
	WorkspaceRoot = samplePath
	if got, want := m.DefaultSessionID(), session.SessionKeyForProject(WorkspaceRoot); got != want {
		t.Errorf("DefaultSessionID() = %q, want SessionKeyForProject(WorkspaceRoot) = %q", got, want)
	}

	// 空根回退为 "default"（与 SessionKeyForProject 语义一致）
	WorkspaceRoot = ""
	if got := m.DefaultSessionID(); got != "default" {
		t.Errorf("DefaultSessionID() empty root = %q, want default", got)
	}
}

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

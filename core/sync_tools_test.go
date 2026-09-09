package core

import (
	"testing"
)

// TestApplyPluginToolsSync 校验宿主在插件工具变化时把最新工具集同步进共享 registry：
// 新增的工具被注册、移除的工具被注销、其他插件/内置工具不受影响。
func TestApplyPluginToolsSync(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	// 预置插件 tool-x 的既有工具集 t1,t2（模拟插件加载时注册）
	m.mu.Lock()
	m.toolServiceIDs["tool-x"] = 42
	m.coreToolNames["tool-x"] = []string{"t1", "t2"}
	m.mu.Unlock()
	for _, n := range []string{"t1", "t2"} {
		if err := m.toolRegistry.Register(&RemoteTool{name: n}); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
	}
	// 另一来源（内置/其他插件）的工具，不应因 tool-x 同步而被移除
	if err := m.toolRegistry.Register(&RemoteTool{name: "other-tool"}); err != nil {
		t.Fatal(err)
	}

	// tool-x 热加载后工具集变为 t1,t3（新增 t3、移除 t2）
	m.applyPluginTools("tool-x", []ToolDefinition{
		&RemoteTool{name: "t1"},
		&RemoteTool{name: "t3"},
	}, []string{"t1", "t3"})

	for _, want := range []string{"t1", "t3", "other-tool"} {
		if _, ok := m.toolRegistry.Get(want); !ok {
			t.Fatalf("registry 应含 %s", want)
		}
	}
	if _, ok := m.toolRegistry.Get("t2"); ok {
		t.Fatal("t2 应已被注销（插件不再提供）")
	}
	// 同步后该插件工具名表更新
	m.mu.RLock()
	got := m.coreToolNames["tool-x"]
	m.mu.RUnlock()
	if len(got) != 2 || got[0] != "t1" || got[1] != "t3" {
		t.Fatalf("coreToolNames[tool-x] = %v, want [t1 t3]", got)
	}
}

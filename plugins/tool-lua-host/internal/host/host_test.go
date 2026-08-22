package host

import (
	"encoding/json"
	"testing"

	"tool-lua-host/internal/bindings"
)

// TestHostLoadsExample 验证 example 脚本被加载且工具注册到工具表。
func TestHostLoadsExample(t *testing.T) {
	h := New("../../scripts", &bindings.Services{}, t.Logf)
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

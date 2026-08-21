package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fixedPolicy 返回固定策略的读取函数（测试简化用）。
func fixedPolicy(p SandboxPolicy) func() SandboxPolicy {
	return func() SandboxPolicy { return p }
}

func TestSandboxReadOnlyBlocksWrite(t *testing.T) {
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxReadOnly)))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	_, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		json.RawMessage(`{"command":"str_replace","path":"/tmp/x.txt","old_str":"a","new_str":"b"}`))
	if err == nil || !strings.Contains(err.Error(), "read-only policy blocks write") {
		t.Fatalf("err = %v, want read-only block", err)
	}
}

func TestSandboxReadOnlyAllowsView(t *testing.T) {
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxReadOnly)))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		json.RawMessage(`{"command":"view","path":"/tmp/x.txt"}`)); err != nil {
		t.Fatalf("view should pass read-only policy: %v", err)
	}
}

func TestSandboxWorkspaceAllowsInside(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir() + "/ws"
	defer func() { WorkspaceRoot = orig }()
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxWorkspaceWrite)))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		json.RawMessage(`{"command":"str_replace","path":"`+WorkspaceRoot+`/a.txt","old_str":"a","new_str":"b"}`)); err != nil {
		t.Fatalf("write inside workspace should pass: %v", err)
	}
}

func TestSandboxWorkspaceBlocksOutside(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir() + "/ws"
	defer func() { WorkspaceRoot = orig }()
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxWorkspaceWrite)))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	_, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		json.RawMessage(`{"command":"insert","path":"/etc/hosts","new_str":"x","insert_line":1}`))
	if err == nil || !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("err = %v, want workspace-write block", err)
	}
}

func TestSandboxFullAllowsWrite(t *testing.T) {
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxFullAccess)))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		json.RawMessage(`{"command":"str_replace","path":"/tmp/x.txt","old_str":"a","new_str":"b"}`)); err != nil {
		t.Fatalf("full access should allow write: %v", err)
	}
}

func TestSandboxIgnoresNonWriteTools(t *testing.T) {
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxReadOnly)))
	_ = m.toolRegistry.Register(&mockTool{name: "plain-tool"})

	if _, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("non-write tool should not be blocked by sandbox: %v", err)
	}
}

func TestSandboxDynamicSwitch(t *testing.T) {
	m := newRouterManager()
	m.sandboxPolicyVal.Store(int32(SandboxWorkspaceWrite))
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(m.GetSandboxPolicy))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	write := json.RawMessage(`{"command":"str_replace","path":"/tmp/x.txt","old_str":"a","new_str":"b"}`)
	// 初始 workspace：/tmp 写被拦
	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor", write); err == nil {
		t.Fatal("workspace policy should block /tmp write")
	}
	// 切到 full：放行
	m.SetSandboxPolicy(SandboxFullAccess)
	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor", write); err != nil {
		t.Fatalf("after switch to full, write should pass: %v", err)
	}
	// 切到 readonly：拦截
	m.SetSandboxPolicy(SandboxReadOnly)
	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor", write); err == nil {
		t.Fatal("after switch to read-only, write should be blocked")
	}
}

func TestParseSandboxPolicy(t *testing.T) {
	cases := map[string]SandboxPolicy{
		"full":      SandboxFullAccess,
		"readonly":  SandboxReadOnly,
		"workspace": SandboxWorkspaceWrite,
		"":          SandboxWorkspaceWrite,
		"bogus":     SandboxWorkspaceWrite,
	}
	for in, want := range cases {
		if got := ParseSandboxPolicy(in); got != want {
			t.Errorf("ParseSandboxPolicy(%q) = %v, want %v", in, got, want)
		}
	}
}

package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// fixedPolicy 返回固定策略的读取函数（测试简化用）。
func fixedPolicy(p SandboxPolicy) func() SandboxPolicy {
	return func() SandboxPolicy { return p }
}

// TestWorkspacePathToRootBoundary 验证 /workspace 别名前缀须跟分隔符或结尾，
// /workspacefoo 之类不应被映射。
func TestWorkspacePathToRootBoundary(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = "/root/ws"
	defer func() { WorkspaceRoot = orig }()

	cases := []struct{ in, want string }{
		{"/workspace", "/root/ws"},
		{"/workspace/a.txt", filepath.Join("/root/ws", "a.txt")},
		{`\workspace\b.txt`, filepath.Join("/root/ws", "b.txt")},
		// 前綴後直接接字符 → 唔係別名，維持原樣
		{"/workspacefoo/x", "/workspacefoo/x"},
		{"/workspacex", "/workspacex"},
	}
	for _, c := range cases {
		if got := workspacePathToRoot(c.in); got != c.want {
			t.Errorf("workspacePathToRoot(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// jsonPath 将路径转义后嵌入 JSON 字面量：Windows 的 t.TempDir() 含反斜杆，
// 直接拼进 JSON 会是非法转义序列，导致沙箱把解析失败当写拦截（fail-closed）。
func jsonPath(p string) string {
	return strings.ReplaceAll(strings.ReplaceAll(p, `\`, `\\`), `"`, `\"`)
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

func TestSandboxReadOnlyBlocksShellExecutor(t *testing.T) {
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxReadOnly)))
	_ = m.toolRegistry.Register(&mockTool{name: "shell"})

	// shell 是解释器/执行器，read-only 下即使命令看似只读也须整体禁用（无法判定是否写文件）。
	if _, err := m.ExecuteTool(context.Background(), "shell",
		json.RawMessage(`{"command":"echo hello"}`)); err == nil || !strings.Contains(err.Error(), "read-only policy blocks write") {
		t.Fatalf("shell should be blocked under read-only: %v", err)
	}
}

func TestSandboxWorkspaceAllowsShellExecutor(t *testing.T) {
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxWorkspaceWrite)))
	_ = m.toolRegistry.Register(&mockTool{name: "shell"})

	// workspace-write（缺省档）下 shell 放行：运行 shell 是其主要用途。
	if _, err := m.ExecuteTool(context.Background(), "shell",
		json.RawMessage(`{"command":"echo hi"}`)); err != nil {
		t.Fatalf("shell should pass workspace-write policy: %v", err)
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
		json.RawMessage(`{"command":"str_replace","path":"`+jsonPath(WorkspaceRoot)+`/a.txt","old_str":"a","new_str":"b"}`)); err != nil {
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

func TestSandboxWorkspaceAcceptsVirtualPrefix(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir() + "/ws"
	defer func() { WorkspaceRoot = orig }()
	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxWorkspaceWrite)))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	// 模型按工具描述传入 /workspace/<rel> 虚拟根前缀，应与真实 WorkspaceRoot 等价放行
	for _, p := range []string{"/workspace/a.txt", `\workspace\b.txt`, "/workspace"} {
		if _, err := m.ExecuteTool(context.Background(), "str_replace_editor",
			json.RawMessage(`{"command":"str_replace","path":"`+jsonPath(p)+`","old_str":"a","new_str":"b"}`)); err != nil {
			t.Fatalf("write under virtual /workspace prefix %q should pass: %v", p, err)
		}
	}
}

// TestSandboxWorkspaceAllowsRealPathCaseInsensitive 回归「换成真实路径被拦截」死循环：
// Windows 文件系统大小写不敏感，模型按 pwd 或自身推断传回的盘符/目录名大小写可能与
// WorkspaceRoot 不一致（如小写盘符 d:\agents\dsc\...）。修复前 inWorkspace 的词法前缀
// 比较区分大小写，导致真实路径在 workspace 内却被误拦，模型只好退回 /workspace 虚拟
// 前缀（该前缀在 shell 中又不存在），陷入死循环，最终被迫切换 full-access。
func TestSandboxWorkspaceAllowsRealPathCaseInsensitive(t *testing.T) {
	orig := WorkspaceRoot
	WorkspaceRoot = t.TempDir() + "/ws"
	defer func() { WorkspaceRoot = orig }()

	// 以与 canonical 根大小写不同的真实路径写 workspace 内文件（模拟模型回传路径）
	root := canonicalWorkspaceRoot()
	alt := strings.ToLower(root) + "/inside.txt"
	if alt == root+"/inside.txt" {
		t.Skip("当前系统大小写不敏感或路径无大小写差异，跳过")
	}
	// JSON 中反斜杆需转义（真实请求由模型产出合法 JSON，这里等价构造）
	escaped := strings.ReplaceAll(alt, `\`, `\\`)

	m := newRouterManager()
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(fixedPolicy(SandboxWorkspaceWrite)))
	_ = m.toolRegistry.Register(&mockTool{name: "str_replace_editor"})

	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor",
		json.RawMessage(`{"command":"str_replace","path":"`+escaped+`","old_str":"a","new_str":"b"}`)); err != nil {
		t.Fatalf("write to real path with case variant should pass on case-insensitive FS: %v", err)
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

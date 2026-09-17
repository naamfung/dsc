package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setWorkspaceRoot 在测试期内覆写统一根 WorkspaceRoot（包变量在进程 init 时冻结，
// t.Setenv 改不了它），测试结束后恢复。各插件测试亦用此模式。
func setWorkspaceRoot(t *testing.T, root string) {
	t.Helper()
	old := WorkspaceRoot
	WorkspaceRoot = root
	t.Cleanup(func() { WorkspaceRoot = old })
}

// TestMapWorkspacePathForWorkspaces 前缀规则（所有平台一致）：/workspace → 根；
// 前缀后必须是分隔符或结尾（/workspacefoo 不作别名，保持真实根语义）；反斜杆
// 形态同样识别。相对路径与盘符路径不受影响。
func TestMapWorkspacePathForWorkspaces(t *testing.T) {
	common := []struct{ in, want string }{
		{"/workspace", "G:/ws"},
		{"/workspace/", "G:/ws"},
		{"/workspace/a/b.txt", "G:/ws/a/b.txt"},
		{`\workspace\x`, "G:/ws/x"},
		// 边界：/workspacefoo 不作别名（所有平台，真实根语义下原样返回）
		{"/workspacefoo/x", "/workspacefoo/x"},
		{"/workspacex", "/workspacex"},
		// 相对路径与盘符路径不受影响
		{"rel/x.txt", "rel/x.txt"},
		{"C:/out/x.txt", "C:/out/x.txt"},
		{`C:\out\x.txt`, `C:\out\x.txt`},
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, c := range common {
			if got := mapWorkspacePathFor(c.in, "G:/ws", goos); got != c.want {
				t.Errorf("goos=%s mapWorkspacePathFor(%q) = %q, want %q", goos, c.in, got, c.want)
			}
		}
	}
}

// TestMapWorkspacePathForBarePosixRoot 裸 POSIX 绝对路径保持原样（所有平台，真实根
// 语义）：Linux/macOS 上 / 是真实根；Windows 上无盘符裸 /x 或 \x 不再锚定工作区根，
// 由下游 filepath.Abs 解析为当前盘根（与 Linux 真实根行为一致）。/workspace 与
// /mnt/<drive> 是显式别名；/dev/null（shell 重定向特判）与 // UNC 前缀保持原样。
func TestMapWorkspacePathForBarePosixRoot(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		for _, c := range []struct{ in, want string }{
			{"/", "/"},
			{"/docs/architecture.md", "/docs/architecture.md"},
			{"/dev/null", "/dev/null"},
			{"//server/share", "//server/share"},
		} {
			if got := mapWorkspacePathFor(c.in, "G:/ws", goos); got != c.want {
				t.Errorf("goos=%s mapWorkspacePathFor(%q) = %q, want %q", goos, c.in, got, c.want)
			}
		}
	}
}

// TestMapWorkspacePathForWSL WSL 风格路径 /mnt/<drive>/... → <drive>:/（仅 Windows）；
// 非法盘符（/mnt/zz）不匹配映射，保持原样（真实根语义，不再落入工作区根）。
// Linux/macOS 原样返回。
func TestMapWorkspacePathForWSL(t *testing.T) {
	if got := mapWorkspacePathFor("/mnt/c/Users/foo", "G:/ws", "windows"); got != "C:/Users/foo" {
		t.Errorf("windows WSL map = %q, want C:/Users/foo", got)
	}
	if got := mapWorkspacePathFor("/mnt/c", "G:/ws", "windows"); got != "C:/" {
		t.Errorf("windows /mnt/c = %q, want C:/", got)
	}
	if got := mapWorkspacePathFor("/mnt/zz/x", "G:/ws", "windows"); got != "/mnt/zz/x" {
		t.Errorf("windows 非法盘符应原样（真实根语义）: got %q", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := mapWorkspacePathFor("/mnt/c/Users/foo", "G:/ws", goos); got != "/mnt/c/Users/foo" {
			t.Errorf("goos=%s /mnt/c 不应改写: got %q", goos, got)
		}
	}
}

// TestMapWorkspacePathForNoRoot 根为空串时一律原样返回（防御分支；
// 生产中 WorkspaceRoot 经 init 回退链恒非空）。
func TestMapWorkspacePathForNoRoot(t *testing.T) {
	for _, p := range []string{"/workspace/x", "/", "/docs/a.md", "rel"} {
		if got := mapWorkspacePathFor(p, "", "windows"); got != p {
			t.Errorf("root 空时 %q 应原样，got %q", p, got)
		}
	}
}

// TestMapWorkspacePathWiring 验证包级入口读 WorkspaceRoot 变量（init 冻结语义，
// 宿主与插件进程同源）。
func TestMapWorkspacePathWiring(t *testing.T) {
	setWorkspaceRoot(t, filepath.ToSlash(t.TempDir()))
	if got := MapWorkspacePath("/workspace/a.md"); !strings.HasPrefix(got, WorkspaceRoot) || !strings.HasSuffix(got, "/a.md") {
		t.Errorf("MapWorkspacePath(/workspace/a.md) = %q, want 前缀 %s", got, WorkspaceRoot)
	}
}

// TestResolveWorkspacePath 解析语义：相对路径锚定工作空间根（非进程 cwd——
// 插件进程 cwd 是 ExecDir，锚 cwd 会把工作空间相对路径落到安装目录）；
// /workspace 前缀映射到根下；盘符绝对路径原样求净。
func TestResolveWorkspacePath(t *testing.T) {
	root := t.TempDir()
	setWorkspaceRoot(t, root)

	rel, err := ResolveWorkspacePath("docs/architecture.md")
	if err != nil {
		t.Fatalf("relative resolve: %v", err)
	}
	if want := filepath.Join(root, "docs", "architecture.md"); rel != want {
		t.Errorf("relative = %q, want %q", rel, want)
	}

	ws, err := ResolveWorkspacePath("/workspace/docs/architecture.md")
	if err != nil {
		t.Fatalf("/workspace resolve: %v", err)
	}
	if ws != filepath.Join(root, "docs", "architecture.md") {
		t.Errorf("/workspace = %q, want %q", ws, filepath.Join(root, "docs", "architecture.md"))
	}

	abs, err := ResolveWorkspacePath("/docs/architecture.md")
	if err != nil {
		t.Fatalf("bare root resolve: %v", err)
	}
	// 真实根语义（所有平台）：/docs 是根下的路径，不得锚定工作空间根。Linux/macOS
	// 上 / 是真实根、原样求净；Windows 上无盘符裸 / 由 filepath.Abs 解析为当前盘根
	//（纯函数分支由 TestMapWorkspacePathForBarePosixRoot 覆盖）。
	wantAbs, err := filepath.Abs("/docs/architecture.md")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if abs != wantAbs {
		t.Errorf("bare / = %q, want %q（真实根语义，非工作区锚定）", abs, wantAbs)
	}
	if abs == filepath.Join(root, "docs", "architecture.md") {
		t.Errorf("bare / 不得再锚定工作空间根（真实根语义）: %q", abs)
	}
}

// TestResolveWorkspacePathWindowsBareRoot 报告场景的 Windows 真实根语义回归（经纯函数
// 注入 goos=windows 验证，Linux 测试机亦可覆盖）：/docs/architecture.md 保持原样
// 传递（映射层不再锚定工作区根），由下游 filepath.Abs 解析为当前盘根——与 Linux
// 把 /docs 解析到真实根行为一致（「错了也一致」）。
func TestResolveWorkspacePathWindowsBareRoot(t *testing.T) {
	got := mapWorkspacePathFor("/docs/architecture.md", "G:/Agents/deepseek-harness", "windows")
	if got != "/docs/architecture.md" {
		t.Fatalf("Windows 裸 / 必须原样传递（真实根语义）: got %q", got)
	}
}

// TestWorkspaceRootFallback 根回退链：无注入时取 cwd（与宿主 resolveWorkspaceRoot
// 的默认一致）。init 已在包加载时执行，此处只验证变量非空（回退链本体已执行过）。
func TestWorkspaceRootFallback(t *testing.T) {
	if WorkspaceRoot == "" {
		t.Fatal("WorkspaceRoot 经 init 回退链后不应为空")
	}
	if _, err := os.Stat(WorkspaceRoot); err != nil {
		t.Errorf("WorkspaceRoot %s 不可访问: %v", WorkspaceRoot, err)
	}
}

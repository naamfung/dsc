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
// 前缀后必须是分隔符或结尾（/workspacefoo 不作别名）；反斜杆形态同样识别。
// /workspacefoo 与 /workspacex 的边界外溢在 Windows 上被裸 / 规则接住（锚定根），
// Linux/macOS 原样返回。
func TestMapWorkspacePathForWorkspaces(t *testing.T) {
	common := []struct{ in, want string }{
		{"/workspace", "G:/ws"},
		{"/workspace/", "G:/ws"},
		{"/workspace/a/b.txt", "G:/ws/a/b.txt"},
		{`\workspace\x`, "G:/ws/x"},
		// 相对路径与盘符路径不受影响
		{"rel/x.txt", "rel/x.txt"},
		{"C:/out/x.txt", "C:/out/x.txt"},
		{`C:\out\x.txt`, `C:\out\x.txt`},
	}
	windowsOnly := []struct{ in, want string }{
		{"/workspacefoo/x", "G:/ws/workspacefoo/x"},
		{"/workspacex", "G:/ws/workspacex"},
	}
	posixOnly := []struct{ in, want string }{
		{"/workspacefoo/x", "/workspacefoo/x"},
		{"/workspacex", "/workspacex"},
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		extra := posixOnly
		if goos == "windows" {
			extra = windowsOnly
		}
		for _, c := range append(common, extra...) {
			if got := mapWorkspacePathFor(c.in, "G:/ws", goos); got != c.want {
				t.Errorf("goos=%s mapWorkspacePathFor(%q) = %q, want %q", goos, c.in, got, c.want)
			}
		}
	}
}

// TestMapWorkspacePathForBarePosixRoot 裸 / 前缀规则（仅 Windows）：锚定工作空间根。
// 真实案例：str_replace_editor 收到 /docs/architecture.md，经 filepath.Abs 落到
// 进程 cwd 所在盘的盘根（D:/docs），而非 <workspace>/docs/architecture.md——
// 虚拟根未转换。规则 3 修复之。例外：/dev/null（mvdan 重定向特判依赖）与
// // UNC 前缀不改写。Linux/macOS 上 / 是真实根，不启用。
func TestMapWorkspacePathForBarePosixRoot(t *testing.T) {
	windows := []struct{ in, want string }{
		{"/", "G:/ws"},
		{"/docs/architecture.md", "G:/ws/docs/architecture.md"},
		{"/Agents/pkg/fs/src/index.ts", "G:/ws/Agents/pkg/fs/src/index.ts"},
		{`\docs\a.md`, "G:/ws/docs/a.md"},
		{"/dev/null", "/dev/null"},
		{"//server/share", "//server/share"},
		{`\\server\share`, `\\server\share`},
	}
	nonWindows := []struct{ in, want string }{
		{"/", "/"},
		{"/docs/architecture.md", "/docs/architecture.md"},
		{"/dev/null", "/dev/null"},
		{"//server/share", "//server/share"},
	}
	for _, tc := range []struct {
		goos  string
		cases []struct{ in, want string }
	}{
		{"windows", windows},
		{"linux", nonWindows},
		{"darwin", nonWindows},
	} {
		for _, c := range tc.cases {
			if got := mapWorkspacePathFor(c.in, "G:/ws", tc.goos); got != c.want {
				t.Errorf("goos=%s mapWorkspacePathFor(%q) = %q, want %q", tc.goos, c.in, got, c.want)
			}
		}
	}
}

// TestMapWorkspacePathForWSL WSL 风格路径 /mnt/<drive>/... → <drive>:/（仅 Windows，
// 且先于裸 / 规则）；非法盘符（/mnt/zz）落入裸 / 规则。Linux/macOS 原样返回。
func TestMapWorkspacePathForWSL(t *testing.T) {
	if got := mapWorkspacePathFor("/mnt/c/Users/foo", "G:/ws", "windows"); got != "C:/Users/foo" {
		t.Errorf("windows WSL map = %q, want C:/Users/foo", got)
	}
	if got := mapWorkspacePathFor("/mnt/c", "G:/ws", "windows"); got != "C:/" {
		t.Errorf("windows /mnt/c = %q, want C:/", got)
	}
	if got := mapWorkspacePathFor("/mnt/zz/x", "G:/ws", "windows"); got != "G:/ws/mnt/zz/x" {
		t.Errorf("windows 非法盘符应落入裸 / 规则: got %q", got)
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
	// Linux 上 /docs 是真实根语义（规则 3 仅 Windows 启用），原样求净；
	// Windows 的虚拟根锚定由 TestResolveWorkspacePathWindowsBareRoot 覆盖。
	if abs != "/docs/architecture.md" {
		t.Errorf("bare / = %q, want /docs/architecture.md（Linux 真实根语义）", abs)
	}
}

// TestResolveWorkspacePathWindowsBareRoot 报告场景的 Windows 语义回归（经纯函数
// 注入 goos=windows 验证，Linux 测试机亦可覆盖）：/docs/architecture.md 必须锚定
// 工作空间根，绝不落到盘根 D:/docs。
func TestResolveWorkspacePathWindowsBareRoot(t *testing.T) {
	got := mapWorkspacePathFor("/docs/architecture.md", "G:/Agents/deepseek-harness", "windows")
	if got != "G:/Agents/deepseek-harness/docs/architecture.md" {
		t.Fatalf("报告场景回归: got %q, want G:/Agents/deepseek-harness/docs/architecture.md", got)
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

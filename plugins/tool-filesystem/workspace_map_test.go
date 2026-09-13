package main

import (
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestMapWorkspacePath(t *testing.T) {
	t.Setenv("DSC_WORKSPACE_ROOT", "/tmp/myws")

	cases := []struct{ in, want string }{
		{"/workspace", "/tmp/myws"},
		{"/workspace/", "/tmp/myws"},
		{"/workspace/a/b.txt", "/tmp/myws/a/b.txt"},
		{"/workspace/x", "/tmp/myws/x"},
		// 边界：/workspacefoo 不作为别名
		{"/workspacefoo/x", "/workspacefoo/x"},
		{"/workspacex", "/workspacex"},
	}

	for _, c := range cases {
		if got := mapWorkspacePath(c.in); got != c.want {
			t.Fatalf("mapWorkspacePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMapWorkspacePathNoRoot(t *testing.T) {
	t.Setenv("DSC_WORKSPACE_ROOT", "")
	if got := mapWorkspacePath("/workspace/x"); got != "/workspace/x" {
		t.Fatalf("未注入根时不应改写: got %q", got)
	}
}

func TestMapWorkspacePathInvalidPrefix(t *testing.T) {
	t.Setenv("DSC_WORKSPACE_ROOT", "/tmp/myws")
	if got := mapWorkspacePath("workspacerel"); got != "workspacerel" {
		t.Fatalf("相对路径应原样: got %q", got)
	}
	if got := mapWorkspacePath("/my/workspace/x"); got != "/my/workspace/x" {
		t.Fatalf("非前缀路径应原样: got %q", got)
	}
}

// TestMapWorkspacePathWSLGating 验证 WSL/GIT BASH 风格路径 /mnt/<drive>/... 的映射严格按
// 宿主 GOOS 分支：
//   - Windows：/mnt/c/Users/... → C:/Users/...（mvdan POSIX 解释器非 WSL/GIT BASH，无法访问 /mnt/c/）
//   - Linux/macOS：原样返回，/mnt/c/... 是合法 POSIX 路径（可能是真实挂载点），不得改写
//
// 这是 DSC 跨平台根本约束的具体落地：路径映射绝不可在能合法访问 /mnt/c/ 的系统上破坏真实路径。
func TestMapWorkspacePathWSLGating(t *testing.T) {
	t.Setenv("DSC_WORKSPACE_ROOT", "/tmp/myws")
	cases := []struct{ in, want string }{
		{"/mnt/c/Users/foo", "/mnt/c/Users/foo"},
		{"/mnt/d/projects/x", "/mnt/d/projects/x"},
		{"/mnt/c", "/mnt/c"},
		{"/mnt/z/path/to/file", "/mnt/z/path/to/file"},
	}
	if runtime.GOOS == "windows" {
		// Windows: WSL 习惯路径映射到 Windows 盘符
		cases = []struct{ in, want string }{
			{"/mnt/c/Users/foo", "C:/Users/foo"},
			{"/mnt/d/projects/x", "D:/projects/x"},
			{"/mnt/c", "C:/"},
			{"/mnt/z/path/to/file", "Z:/path/to/file"},
		}
	}
	for _, c := range cases {
		if got := mapWorkspacePath(c.in); got != c.want {
			t.Fatalf("mapWorkspacePath(%q) = %q, want %q (GOOS=%s)", c.in, got, c.want, runtime.GOOS)
		}
	}
}

// TestMapWorkspaceAST 校验 AST 层重写：裸词、单/双引号里的 /workspace 被映射，
// 变量展开/命令替换等复杂词不改；边界 /workspacefoo 不改。
func TestMapWorkspaceAST(t *testing.T) {
	t.Setenv("DSC_WORKSPACE_ROOT", "/tmp/myws")
	cases := []struct {
		in, want string
	}{
		{`cd /workspace`, `cd /tmp/myws`},
		{`ls -la "/workspace/a b"`, `ls -la "/tmp/myws/a b"`},
		{`cat '/workspace/x.txt'`, `cat '/tmp/myws/x.txt'`},
		{`cd /workspace/sub && pwd`, `cd /tmp/myws/sub && pwd`},
		{`touch /workspacefoo/x`, `touch /workspacefoo/x`}, // 边界不改
		{`echo $HOME /workspace`, `echo $HOME /tmp/myws`},  // 变量展开不改，裸词映射
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			parser := syntax.NewParser()
			file, err := parser.Parse(strings.NewReader(c.in+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			mapWorkspacePaths(file)
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, file); err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(sb.String())
			if got != c.want {
				t.Fatalf("mapWorkspacePaths(%q)\n got: %q\nwant: %q", c.in, got, c.want)
			}
		})
	}
}

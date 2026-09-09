package main

import (
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

package main

import (
	"path/filepath"
	"testing"
)

// TestResolveWorkspaceRoot 默认以启动目录（cwd）为 workspace 根；
// 仅显式绝对路径配置覆盖；相对路径配置不再参与决定根。
func TestResolveWorkspaceRoot(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "project")
	customRoot := filepath.Join(t.TempDir(), "custom")

	cases := []struct {
		name    string
		cwd     string
		cfgRoot string
		want    string
	}{
		{"默认以启动目录为根", cwd, "", cwd},
		{"相对路径配置忽略", cwd, "./workspace", cwd},
		{"相对路径配置忽略2", cwd, "sub/dir", cwd},
		{"绝对路径覆盖", cwd, customRoot, customRoot},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveWorkspaceRoot(c.cwd, c.cfgRoot); got != c.want {
				t.Fatalf("resolveWorkspaceRoot(%q, %q) = %q, want %q", c.cwd, c.cfgRoot, got, c.want)
			}
		})
	}
}

// TestResolveWorkspaceRootEmptyCwd cwd 为空时回退到进程当前工作目录。
func TestResolveWorkspaceRootEmptyCwd(t *testing.T) {
	got := resolveWorkspaceRoot("", "")
	if got == "" {
		t.Fatal("cwd 为空时应回退到 os.Getwd()，不能为空")
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("workspace 根应为绝对路径: %q", got)
	}
}

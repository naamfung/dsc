package main

import (
	"path/filepath"
	"testing"
)

// TestResolveWorkspaceRoot 默认以启动目录（cwd）为 workspace 根；
// 仅显式绝对路径配置覆盖；相对路径配置不再参与决定根。
// DSC_WORKSPACE_ROOT 环境变量显式设置时作为统一根（宿主与插件同源，优先于 cwd）。
func TestResolveWorkspaceRoot(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "project")
	customRoot := filepath.Join(t.TempDir(), "custom")
	envRoot := filepath.Join(t.TempDir(), "env-root")

	cases := []struct {
		name    string
		cwd     string
		cfgRoot string
		envRoot string
		want    string
	}{
		{"默认以启动目录为根", cwd, "", "", cwd},
		{"相对路径配置忽略", cwd, "./workspace", "", cwd},
		{"相对路径配置忽略2", cwd, "sub/dir", "", cwd},
		{"绝对路径覆盖", cwd, customRoot, "", customRoot},
		{"环境变量覆盖 cwd", cwd, "", envRoot, envRoot},
		{"绝对路径配置仍优先于环境变量", cwd, customRoot, envRoot, customRoot},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DSC_WORKSPACE_ROOT", c.envRoot)
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

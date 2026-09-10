package tui

import (
	"path/filepath"
	"runtime"
	"testing"

	"dsc/core"
)

// TestScopeLabel 校验状态栏工作范围显示：full-access → 「文件系统」；
// 其余 → 工作区目录基础名（含限长与根路径兜底）。
//
// 跨平台：Windows 上反斜杠是路径分隔符，Linux/macOS 上 filepath.Base
// 不会拆反斜杠路径——故按 runtime.GOOS 选路径分隔符，避免 Linux 上误判。
func TestScopeLabel(t *testing.T) {
	orig := core.WorkspaceRoot
	defer func() { core.WorkspaceRoot = orig }()
	var deepCleanPath string
	if runtime.GOOS == "windows" {
		deepCleanPath = `C:\Users\Admin\DeepClean`
	} else {
		deepCleanPath = `/home/admin/DeepClean`
	}
	core.WorkspaceRoot = deepCleanPath

	m := &Model{}
	if got := m.scopeLabel(); got != "DeepClean" {
		t.Fatalf("scopeLabel (workspace-write) = %q, want DeepClean", got)
	}

	m.manager = &core.Manager{}
	m.manager.SetSandboxPolicy(core.SandboxFullAccess)
	if got := m.scopeLabel(); got != "文件系统" {
		t.Fatalf("scopeLabel (full-access) = %q, want 文件系统", got)
	}

	m.manager.SetSandboxPolicy(core.SandboxWorkspaceWrite)
	if got := m.scopeLabel(); got != "DeepClean" {
		t.Fatalf("scopeLabel (workspace-write) = %q, want DeepClean", got)
	}

	// 超长目录名截断
	longName := "这是一个非常非常长的真实工作目录名称用于测试截断行为"
	core.WorkspaceRoot = filepath.Join(string(filepath.Separator), "a", longName)
	if got := m.scopeLabel(); len([]rune(got)) != 17 { // 16 字符 + "…"
		t.Fatalf("scopeLabel (long) = %q (len %d), want 16+…", got, len([]rune(got)))
	}
}

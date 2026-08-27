package tui

import (
	"testing"

	"dsc/core"
)

// TestScopeLabel 校验状态栏工作范围显示：full-access → 「文件系统」；
// 其余 → 工作区目录基础名（含限长与根路径兜底）。
func TestScopeLabel(t *testing.T) {
	orig := core.WorkspaceRoot
	defer func() { core.WorkspaceRoot = orig }()
	core.WorkspaceRoot = `C:\Users\Admin\DeepClean`

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
	core.WorkspaceRoot = `C:\a\` + string(rune('长')+0)
	core.WorkspaceRoot = `C:\a\这是一个非常非常长的真实工作目录名称用于测试截断行为`
	if got := m.scopeLabel(); len([]rune(got)) != 17 { // 16 字符 + "…"
		t.Fatalf("scopeLabel (long) = %q (len %d), want 16+…", got, len([]rune(got)))
	}
}

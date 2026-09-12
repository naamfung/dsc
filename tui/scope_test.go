package tui

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dsc/core"
)

// TestScopeLabel 校验状态栏工作范围显示：
// 始终展示当前工作区的真实目录基础名（限长，避免超长真实目录破坏布局），
// 沙箱模式经后缀标记呈现（🔒 read-only / ✎ workspace-write / ⚡ full-access），
// 不再因 full-access 而把目录名替换为「文件系统」——
// full-access 仅意味写权限放开，不意味当前目录改变。
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
	// 无 manager（启动初态）：仅目录名，无沙箱后缀
	if got := m.scopeLabel(); got != "DeepClean" {
		t.Fatalf("scopeLabel (no manager) = %q, want DeepClean", got)
	}

	m.manager = &core.Manager{}

	// full-access：目录名 + ⚡（不再替换为「文件系统」）
	m.manager.SetSandboxPolicy(core.SandboxFullAccess)
	if got := m.scopeLabel(); got != "DeepClean ⚡" {
		t.Fatalf("scopeLabel (full-access) = %q, want \"DeepClean ⚡\"", got)
	}

	// workspace-write：目录名 + ✎
	m.manager.SetSandboxPolicy(core.SandboxWorkspaceWrite)
	if got := m.scopeLabel(); got != "DeepClean ✎" {
		t.Fatalf("scopeLabel (workspace-write) = %q, want \"DeepClean ✎\"", got)
	}

	// read-only：目录名 + 🔒
	m.manager.SetSandboxPolicy(core.SandboxReadOnly)
	if got := m.scopeLabel(); got != "DeepClean 🔒" {
		t.Fatalf("scopeLabel (read-only) = %q, want \"DeepClean 🔒\"", got)
	}

	// 超长目录名截断（含后缀标记）
	longName := "这是一个非常非常长的真实工作目录名称用于测试截断行为"
	core.WorkspaceRoot = filepath.Join(string(filepath.Separator), "a", longName)
	got := m.scopeLabel()
	// 16 字符 + "…" + " 🔒"（后缀标记不计入截断长度限制）
	if !strings.Contains(got, "…") {
		t.Fatalf("scopeLabel (long) = %q, want contains …", got)
	}
	if !strings.Contains(got, "🔒") {
		t.Fatalf("scopeLabel (long, read-only) = %q, want contains 🔒", got)
	}

	// 空工作区根 → 兜底「工作区」
	core.WorkspaceRoot = ""
	m.manager.SetSandboxPolicy(core.SandboxWorkspaceWrite)
	if got := m.scopeLabel(); got != "工作区 ✎" {
		t.Fatalf("scopeLabel (empty root) = %q, want \"工作区 ✎\"", got)
	}
}

package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox 进程效应策略层（对齐 DSH sandbox 的 fail-closed 语义，Windows
// 兼容的工具级实现）：工具流水线 pre-execute 阶段按策略拦截文件写操作。
//
//	ReadOnly       只允许读，任何文件写操作一律拒绝
//	WorkspaceWrite 允许 workspace 内写，拒绝 workspace 外写（缺省）
//	FullAccess     不额外拦截（workspace 保护等既有约束仍生效）
//
// 策略未显式配置时缺省 WorkspaceWrite；写操作判定失败时拒绝（fail-closed）。

// SandboxPolicy 沙箱策略档位。
type SandboxPolicy int

const (
	// SandboxFullAccess 不额外拦截写操作。
	SandboxFullAccess SandboxPolicy = iota
	// SandboxWorkspaceWrite 仅允许 workspace 内写（缺省）。
	SandboxWorkspaceWrite
	// SandboxReadOnly 拒绝一切文件写操作。
	SandboxReadOnly
)

// ParseSandboxPolicy 解析策略名：full / workspace / readonly；空或未知回退 workspace。
func ParseSandboxPolicy(s string) SandboxPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "full", "full-access":
		return SandboxFullAccess
	case "readonly", "read-only":
		return SandboxReadOnly
	default:
		return SandboxWorkspaceWrite
	}
}

// sandboxCheck 判定一次工具调用是否被当前策略允许。返回允许时的 nil，或拒绝原因。
func sandboxCheck(policy SandboxPolicy, toolName, argsJSON string) error {
	if policy == SandboxFullAccess {
		return nil
	}
	path, write := writeCallInfo(toolName, argsJSON)
	if !write {
		return nil
	}
	switch policy {
	case SandboxReadOnly:
		return fmt.Errorf("sandbox: read-only policy blocks write to %q via %s", path, toolName)
	case SandboxWorkspaceWrite:
		if path == "" || !inWorkspace(path) {
			return fmt.Errorf("sandbox: workspace-write policy blocks write outside workspace to %q via %s", path, toolName)
		}
	}
	return nil
}

// writeCallInfo 提取工具调用的目标路径与写操作判定。
// 仅对已知写语义的工具生效：str_replace_editor 的 command != view 视为写。
// 其余工具视为非写（不做额外拦截，避免误伤读操作）。
func writeCallInfo(toolName, argsJSON string) (path string, write bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return "", false
	}
	switch toolName {
	case "str_replace_editor":
		path, _ = m["path"].(string)
		cmd, _ := m["command"].(string)
		return path, cmd != "view"
	default:
		return "", false
	}
}

// workspacePathToRoot 将模型按工具描述传入的虚拟根前缀 /workspace（或 \workspace）
// 映射到真实 WorkspaceRoot，使 sandbox 与 tool-str-replace-editor 对同一路径约定
// 使用同一根目录（对齐 DSH 单一策略归属：fs 围栏与流水线判定不漂移）。
func workspacePathToRoot(p string) string {
	for _, prefix := range []string{"/workspace", `\workspace`} {
		if strings.HasPrefix(p, prefix) {
			rest := strings.TrimPrefix(p, prefix)
			if rest == "" {
				return WorkspaceRoot
			}
			return filepath.Join(WorkspaceRoot, strings.TrimLeft(rest, `/\\`))
		}
	}
	return p
}

// inWorkspace 判断绝对路径是否位于 WorkspaceRoot 之下。
func inWorkspace(path string) bool {
	abs, err := filepath.Abs(workspacePathToRoot(path))
	if err != nil {
		return false
	}
	absBase, err := filepath.Abs(WorkspaceRoot)
	if err != nil {
		return false
	}
	return abs == absBase || strings.HasPrefix(abs, absBase+string(os.PathSeparator))
}

// SetSandboxPolicy 运行时切换沙箱策略（TUI /sandbox 命令用；线程安全）。
func (m *Manager) SetSandboxPolicy(p SandboxPolicy) {
	m.sandboxPolicyVal.Store(int32(p))
}

// GetSandboxPolicy 读取当前沙箱策略。
func (m *Manager) GetSandboxPolicy() SandboxPolicy {
	return SandboxPolicy(m.sandboxPolicyVal.Load())
}

// sandboxPolicy 工具流水线 pre-execute 瀑布策略：fail-closed 拦截写操作。
// 策略经 get 动态读取，支持运行时切换（TUI /sandbox 命令）。
func sandboxPolicy(get func() SandboxPolicy) WaterfallListener {
	return func(ctx EventContext, next func(EventContext) error) error {
		inv, _ := ctx.Data.(*ToolInvocation)
		if inv == nil {
			return next(ctx)
		}
		if err := sandboxCheck(get(), inv.ToolName, inv.ArgumentsJSON); err != nil {
			return err // fail-closed：拒绝，不执行
		}
		return next(ctx)
	}
}

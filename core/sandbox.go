package core

import (
	"encoding/json"
	"fmt"
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

// writeCapableExecutors 是无法从参数路径判断写入目标、却可对文件系统任意写/执行
// 命令的解释器/执行器工具（如 tool-filesystem 的 shell，内部为 mvdan/sh）。
// read-only 下必须整体禁用——因为无法从命令文本判定某条命令是否只读，放行即等于
// 允许「echo x > /anywhere」绕开只读沙箱（P0-1）。workspace-write 下放行（缺省档，
// 运行 shell 是其主要用途；其工作区写入边界由工具/OS 层约束）。
//
// 长期正解：由工具插件自声明「可执行的写能力」（自声明式写入语义），替换此处的
// 名称枚举。暂以此作为 read-only 缺口的最小封堵。
var writeCapableExecutors = map[string]bool{
	"shell": true,
}

// isWriteCapableExecutor 判断工具是否为不可定位写入路径的解释器/执行器。
func isWriteCapableExecutor(name string) bool {
	return writeCapableExecutors[name]
}

// sandboxCheck 判定一次工具调用是否被当前策略允许。返回允许时的 nil，或拒绝原因。
func sandboxCheck(policy SandboxPolicy, toolName, argsJSON string) error {
	if policy == SandboxFullAccess {
		return nil
	}
	path, write := writeCallInfo(toolName, argsJSON)
	switch policy {
	case SandboxReadOnly:
		// read-only：任何可写操作一律拒绝；无法定位写路径的解释器/执行器整体禁用。
		if write || isWriteCapableExecutor(toolName) {
			return fmt.Errorf("sandbox: read-only policy blocks write via %s", toolName)
		}
	case SandboxWorkspaceWrite:
		if write {
			if path == "" || !inWorkspace(path) {
				return fmt.Errorf("sandbox: workspace-write policy blocks write outside workspace to %q via %s", path, toolName)
			}
		}
		// 不可定位写路径的执行器在 workspace-write 下放行（缺省档，见 writeCapableExecutors 注释）。
	}
	return nil
}

// writeCallInfo 提取工具调用的目标路径与写操作判定。
// 仅对已知写语义的工具生效：str_replace_editor 的 command != view 视为写。
// 其余工具视为非写（不做额外拦截，避免误伤读操作）。
// 参数无法解析为合法 JSON 时按「写」处理（fail-closed），避免解析失败被当成
// 非写放行而与沙箱 fail-closed 声称矛盾。
func writeCallInfo(toolName, argsJSON string) (path string, write bool) {
	switch toolName {
	case "str_replace_editor":
		var m map[string]any
		if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
			// 无法判定写意图：fail-closed 视作写，sandboxCheck 会据此拒绝。
			return "", true
		}
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
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		// 边界检查：前綴後必須是分隔符或結尾，否則 /workspacefoo 之類唔應被當作
		// /workspace 別名，維持其係獨立絕對路徑嘅語義。
		if rest != "" && !strings.HasPrefix(rest, "/") && !strings.HasPrefix(rest, `\`) {
			continue
		}
		if rest == "" {
			return WorkspaceRoot
		}
		return filepath.Join(WorkspaceRoot, strings.TrimLeft(rest, `/\\`))
	}
	return p
}

// canonicalWorkspaceRoot 解析 WorkspaceRoot 的真实路径：绝对化后解析符号链接
// （失败时回退绝对化结果）。Windows 下盘符/路径大小写不敏感且可能存在 8.3 短名、
// 符号链接等别名，统一解析可避免真实路径与根因大小写/别名差异被误判为越界。
func canonicalWorkspaceRoot() string {
	abs, err := filepath.Abs(WorkspaceRoot)
	if err != nil {
		return WorkspaceRoot
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

// inWorkspace 判断路径是否位于 WorkspaceRoot 之下（含 /workspace 虚拟根映射）。
// 先把根与目标都经 CanonicalPath 解析到真实路径（Windows 上用
// GetFinalPathNameByHandle 穿透 junction/symlink，Unix 用 EvalSymlinks），
// 再做包含判定——从而避免 workspace 内指向外部的 junction/symlink 令词法前缀
// 比较误判为「在根内」而写穿沙箱（P0-3）。
func inWorkspace(path string) bool {
	abs, err := filepath.Abs(workspacePathToRoot(path))
	if err != nil {
		return false
	}
	root, rerr := filepath.Abs(WorkspaceRoot)
	if rerr != nil {
		root = WorkspaceRoot
	}
	realRoot, cerr := CanonicalPath(root)
	if cerr != nil {
		realRoot = root
	}
	realTarget, terr := CanonicalPath(abs)
	if terr != nil {
		return false
	}
	return containsPath(realRoot, realTarget)
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

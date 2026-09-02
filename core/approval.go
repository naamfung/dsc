package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"dsc/userquestions"
)

// ApprovalPolicy 审批策略（对齐 DSH ApprovalPolicy 'ask'|'never'）：
// 'ask' 经用户评审通道询问（默认）；'never' 自动拒绝、不问人。
//
// 审批链 = 沙箱升级审批（对齐 DSH approveEscalation）：受限调用被沙箱拒绝时，
// 模型携 sandbox_permissions（更宽档）+ justification 重试同一个调用，触发对人
// 的审批；'ask' 问人、'never' 自动拒。非加宽/参数不配对/无评审通道 fail-closed。
type ApprovalPolicy int

const (
	// ApprovalNever 自动拒绝，不问人（CI/无人值守用）。
	ApprovalNever ApprovalPolicy = iota
	// ApprovalAsk 经用户评审通道询问（默认）。
	ApprovalAsk
)

// approvalAllowLabel / approvalRejectLabel 审批提问的两个既定选项。
const (
	approvalAllowLabel  = "Allow once"
	approvalRejectLabel = "Reject"
)

// ParseApprovalPolicy 解析审批策略名：ask / never；空或未知回退 ask（默认）。
func ParseApprovalPolicy(s string) ApprovalPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "never", "off":
		return ApprovalNever
	default:
		return ApprovalAsk
	}
}

// ApprovalPolicyName 返回审批策略名（ask/never），供 env 注入与日志。
func ApprovalPolicyName(p ApprovalPolicy) string {
	if p == ApprovalNever {
		return "never"
	}
	return "ask"
}

// DefaultApprovalPolicy 返回进程默认审批策略（DSC_APPROVAL，空回退 ask）。
func DefaultApprovalPolicy() ApprovalPolicy {
	return ParseApprovalPolicy(os.Getenv("DSC_APPROVAL"))
}

// SetApprovalPolicy 运行时切换审批策略（TUI /approval 命令用；线程安全）。
func (m *Manager) SetApprovalPolicy(p ApprovalPolicy) {
	m.approvalPolicyVal.Store(int32(p))
}

// GetApprovalPolicy 读取当前审批策略（全局/缺省）。
func (m *Manager) GetApprovalPolicy() ApprovalPolicy {
	return ApprovalPolicy(m.approvalPolicyVal.Load())
}

// SetSessionApprovalPolicy 为指定会话设置审批策略覆盖（per-session，对齐 DSH
// approval/policy 会话态；TUI /approval 对当前会话调用）。空会话视为设置全局。
func (m *Manager) SetSessionApprovalPolicy(sessionID string, p ApprovalPolicy) {
	if sessionID == "" {
		m.SetApprovalPolicy(p)
		return
	}
	m.sessionApprovalMu.Lock()
	if m.sessionApproval == nil {
		m.sessionApproval = map[string]ApprovalPolicy{}
	}
	m.sessionApproval[sessionID] = p
	m.sessionApprovalMu.Unlock()
}

// GetSessionApprovalPolicy 读取指定会话的审批策略（覆盖 ?? 全局缺省）。
func (m *Manager) GetSessionApprovalPolicy(sessionID string) ApprovalPolicy {
	if sessionID != "" {
		m.sessionApprovalMu.RLock()
		p, ok := m.sessionApproval[sessionID]
		m.sessionApprovalMu.RUnlock()
		if ok {
			return p
		}
	}
	return m.GetApprovalPolicy()
}

// approvalPolicyFor 审批门决定所用策略：会话覆盖 ?? 全局缺省。
func (m *Manager) approvalPolicyFor(sessionID string) ApprovalPolicy {
	return m.GetSessionApprovalPolicy(sessionID)
}

// sandboxModeString 把沙箱档位转成 DSH 模式名。
func sandboxModeString(p SandboxPolicy) string {
	switch p {
	case SandboxReadOnly:
		return "read-only"
	case SandboxWorkspaceWrite:
		return "workspace-write"
	default:
		return "danger-full-access"
	}
}

// sandboxDenialMarker 对齐 DSH：被沙箱拒绝的模型可见标记。
func sandboxDenialMarker(mode string) string {
	return fmt.Sprintf("[sandbox: file access denied under %s mode]", mode)
}

// escalationHintMarker 对齐 DSH：被拒后同回合的升级提示。
func escalationHintMarker(subject string) string {
	return "[sandbox: escalation available — retry this exact " + subject +
		" once with sandbox_permissions (the narrowest wider mode that suffices) + justification; the approval prompt asks the user]"
}

// parseEscalationTarget 解析模型请求的升级目标档（封闭目标词表：
// workspace-write / danger-full-access；read-only 是地板，不可升到它）。
func parseEscalationTarget(s string) (SandboxPolicy, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "workspace", "workspace-write":
		return SandboxWorkspaceWrite, true
	case "full", "full-access", "danger-full-access", "danger":
		return SandboxFullAccess, true
	default:
		return 0, false
	}
}

// widenable 判断从有效档 effective 是否可**严格升宽**到 target（封闭阶梯表，
// 对齐 DSH WIDER_MODES）：read-only→{workspace-write, danger-full-access}；
// workspace-write→{danger-full-access}；danger-full-access 已是顶档无更宽。
func widenable(effective, target SandboxPolicy) bool {
	switch effective {
	case SandboxReadOnly:
		return target == SandboxWorkspaceWrite || target == SandboxFullAccess
	case SandboxWorkspaceWrite:
		return target == SandboxFullAccess
	default:
		return false
	}
}

// escalationArgs 从参数 JSON 提取 sandbox_permissions 与 justification（须同时出现才有效）。
func escalationArgs(argsJSON string) (permissions, justification string) {
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return "", ""
	}
	p, _ := m["sandbox_permissions"].(string)
	j, _ := m["justification"].(string)
	return p, j
}

// validateEscalationArgs 对齐 DSH：sandbox_permissions 与 justification 必须配对、
// 理由须为非空句——既无理由的请求、也无理由的提示都是畸形 ask，执行前拒绝。
func validateEscalationArgs(sandboxPermissions, justification string) error {
	if sandboxPermissions != "" && justification == "" {
		return fmt.Errorf("invalid escalation: sandbox_permissions requires a justification")
	}
	if justification != "" && sandboxPermissions == "" {
		return fmt.Errorf("invalid escalation: justification is only valid together with sandbox_permissions")
	}
	if justification != "" && strings.TrimSpace(justification) == "" {
		return fmt.Errorf("invalid justification: expected a non-empty sentence")
	}
	return nil
}

// EventApprovalAsked 审批提问已发出（emit，宿主事件总线：UI/插件可观审计）。
const EventApprovalAsked EventName = "approval/asked"

// EventApprovalDecided 审批已有结论（emit）。
const EventApprovalDecided EventName = "approval/decided"

// approvalEscalation 工具流水线 pre-execute 瀑布审批门（对齐 DSH approveEscalation）：
// 在沙箱判定**之前**运行——检测被拒工具族的升级重试（sandbox_permissions+justification），
// 校验配对与严格加宽、按审批策略问人，allowed-once 则为本**这一次**调用打上更宽档标记
// （随后的 sandboxPolicy 以更宽档复审放行），其余路径 fail-closed 返回各自错误。
// 必须在 sandboxPolicy 之前注册。
func (m *Manager) approvalEscalation() WaterfallListener {
	return func(ev EventContext, next func(EventContext) error) error {
		inv, _ := ev.Data.(*ToolInvocation)
		if inv == nil {
			return next(ev)
		}
		// 只有沙箱强制（写语义/执行器）的工具族承载升级；非强制工具忽略升级参数。
		_, write := writeCallInfo(inv.ToolName, inv.ArgumentsJSON)
		if !write && !isWriteCapableExecutor(inv.ToolName) {
			return next(ev)
		}
		perm, just := escalationArgs(inv.ArgumentsJSON)
		if perm == "" {
			return next(ev) // 非升级重试：交给沙箱正常判定
		}
		// 升级路径：严格校验 → 按策略审批 → 放行/拒绝（任何执行前）。
		target, ok := parseEscalationTarget(perm)
		if !ok {
			return fmt.Errorf("sandbox escalation to %q is not a valid wider mode (workspace-write or danger-full-access)", perm)
		}
		effective := m.GetSandboxPolicy()
		if !widenable(effective, target) {
			return fmt.Errorf("sandbox escalation to %q is not strictly wider than this call's current %q mode",
				sandboxModeString(target), sandboxModeString(effective))
		}
		if err := validateEscalationArgs(perm, just); err != nil {
			return err
		}
		if m.approvalPolicyFor(inv.SessionID) == ApprovalNever {
			// 'never'：不问人，自动拒绝（对齐 DSH NEVER 语义）。
			return fmt.Errorf("approval prompts are disabled in this session: actions that require approval are rejected automatically")
		}
		outcome := m.approveAsk(inv, target, just)
		switch outcome {
		case "allowed-once":
			inv.Escalated = true
			inv.EscalatedMode = target
			return next(ev) // 随后 sandboxPolicy 以更宽档复审，放行本次
		case "rejected":
			return fmt.Errorf("the user rejected escalating this operation to %q", sandboxModeString(target))
		case "cancelled":
			return fmt.Errorf("approval for escalating to %q was cancelled", sandboxModeString(target))
		default:
			return fmt.Errorf("sandbox escalation to %q requires approval, but no approval channel is available", sandboxModeString(target))
		}
	}
}

// approveAsk 发起一次审批提问并映射结果：先广播 approval/asked，经用户评审通道
// 阻塞等待用户选择，再广播 approval/decided。返回 closed 结果
// （allowed-once / rejected / cancelled / unavailable）。无评审通道时 fail-closed。
func (m *Manager) approveAsk(inv *ToolInvocation, target SandboxPolicy, justification string) string {
	outcome := "unavailable"
	m.events.Emit(EventApprovalAsked, EventContext{Data: map[string]string{
		"session": inv.SessionID, "tool": inv.ToolName, "mode": sandboxModeString(target), "reason": justification,
	}})
	defer func() {
		m.events.Emit(EventApprovalDecided, EventContext{Data: map[string]string{
			"session": inv.SessionID, "tool": inv.ToolName, "mode": sandboxModeString(target), "outcome": outcome,
		}})
	}()

	req := &userquestions.Request{Questions: []userquestions.Question{{
		ID:       "approval",
		Header:   "Approval",
		Question: fmt.Sprintf("Approve escalating sandbox to %s for %s?", sandboxModeString(target), inv.ToolName),
		Detail:   justification,
		Options: []userquestions.Option{
			{Label: approvalAllowLabel, Description: "Allow this one-time escalation; the operation runs once under the wider mode."},
			{Label: approvalRejectLabel, Description: "Reject this escalation; the operation is not run."},
		},
	}}}
	ans, err := m.Ask(context.Background(), req)
	if err != nil {
		if ue, ok := err.(*userquestions.Error); ok {
			switch ue.Code {
			case userquestions.ErrAskAborted, userquestions.ErrCanceled:
				outcome = "cancelled"
				return outcome
			}
		}
		outcome = "unavailable" // 含 NO_PROVIDER：无评审通道 fail-closed
		return outcome
	}
	for _, item := range ans.Answers {
		if item.ID != "approval" {
			continue
		}
		if len(item.Selected) == 1 && item.Selected[0] == approvalAllowLabel && item.Custom == "" {
			outcome = "allowed-once"
		} else if len(item.Selected) == 1 && item.Selected[0] == approvalRejectLabel {
			outcome = "rejected"
		} else {
			outcome = "cancelled"
		}
		return outcome
	}
	return outcome
}

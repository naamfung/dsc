package session

// ApprovalPolicyData approval/policy 事件载荷（log-only）：会话级审批策略（"ask"|"never"）。
// 整值替换，最后一条生效；FoldApprovalPolicy 折叠为当前会话策略（缺省回退部署值）。
type ApprovalPolicyData struct {
	Policy string `json:"policy"`
}

// FoldApprovalPolicy 折叠会话审批策略：返回最后一条 approval/policy 记录值；
// 无记录时返回空串，由调用方结合部署缺省（DSC_APPROVAL，默认 ask）决定生效值。
func FoldApprovalPolicy(events []*Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type == ApprovalPolicy {
			if d, ok := ev.Data.(*ApprovalPolicyData); ok && d.Policy != "" {
				return d.Policy
			}
		}
	}
	return ""
}

// ApprovalAskedData approval/asked 事件载荷（log-only）：一次沙箱升级审批提问。
// 对齐 DSH approval/asked：模型携 sandbox_permissions 升级重试时触发，记录目标档/理由。
type ApprovalAskedData struct {
	Tool   string `json:"tool"`
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
}

// ApprovalDecidedData approval/decided 事件载荷（log-only）：该次审批的结论。
// 对齐 DSH approval/decided：allowed-once / rejected / cancelled / unavailable。
type ApprovalDecidedData struct {
	Tool    string `json:"tool"`
	Mode    string `json:"mode"`
	Outcome string `json:"outcome"`
}

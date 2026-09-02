package session

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

package core

import "fmt"

// CapabilityRequiresNeverApproval 声明式能力标签：声明该能力的工具要求前置审批策略为
// never。宿主据此按「能力」而非「插件名」对这一族工具做前置门控（对齐 DSH 能力缝的
// 声明式识别——新增同类工具只需在声明里加该能力，宿主零改动）。与 sdk 侧
// dsc.CapabilityRequiresNeverApproval 同值，对齐同一 wire token（见 proto.Tool.capabilities 注释）。
const CapabilityRequiresNeverApproval = "requires-never-approval"

// neverApprovalHint 提示文案，落在闸门拒绝错误里。
const neverApprovalHint = "bench 评测要求审批策略为 never，请先设 approval=never（DSC_APPROVAL=never 或 TUI /approval never）再重试"

// neverApprovalGate 前置审批策略闸门（pre-execute 瀑布）：当被调工具声明了
// CapabilityRequiresNeverApproval 能力、而会话生效审批策略非 never 时，于执行前拒绝并给出
// 明确提示，避免无人值守评测在 ask 下逐工具反复弹窗授权。未声明该能力，或策略为 never 时
// 放行。经 Manager 挂载到 EventToolPreExecute（loader.go 初始化水位）。
func (m *Manager) neverApprovalGate() WaterfallListener {
	return func(ev EventContext, next func(EventContext) error) error {
		inv, _ := ev.Data.(*ToolInvocation)
		if inv == nil {
			return next(ev)
		}
		tool, ok := m.toolRegistry.Get(inv.ToolName)
		if !ok {
			return next(ev)
		}
		ct, ok := tool.(CapabilityTool)
		if !ok || !ct.HasCapability(CapabilityRequiresNeverApproval) {
			return next(ev) // 未声明该能力：放行
		}
		if m.executionApprovalPolicy(inv) == ApprovalNever {
			return next(ev) // never：放行（无人值守 / TUI /approval never）
		}
		return fmt.Errorf("工具 %q 需审批策略为 never 才可调用：%s", inv.ToolName, neverApprovalHint)
	}
}

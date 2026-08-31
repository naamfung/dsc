package core

// 本文件定义「agent 回合状态」宿主事件（对齐 DSH 的 agent/status 原生事件）：
// 主 agent 每次 RunStream 回合完成（success/error 帧）时，宿主 EventBus 广播
// agent/status 事件（status=idle），routinely 广播给订阅插件（Hook.OnEvent），
// 供通知插件等程序性驱动（非模型工具）。running 事件在回合开始时广播，未作
// 为通知依据时宿主仍按同语义发布，保持与 DSH 行为对齐。

const (
	// EventAgentStatus agent 回合状态事件（对齐 DSH agent/status）。
	// 宿主主 agent 每回合（RunStream）广播一次：
	//   running — 回合开始；
	//   idle    — 回合完成（success 或 error 帧后）。
	EventAgentStatus EventName = "agent/status"
)

// AgentStatusValue agent/status 事件的状态取值（对齐 DSH AgentStatus）。
type AgentStatusValue string

const (
	// AgentStatusRunning 回合进行中。
	AgentStatusRunning AgentStatusValue = "running"
	// AgentStatusIdle 回合完成，agent 回到空闲。
	AgentStatusIdle AgentStatusValue = "idle"
)

// AgentStatusEvent agent/status 事件的载荷（对齐 DSH { agent, status }，
// 此处 agent 名用字符串，status 为运行态取值）。
type AgentStatusEvent struct {
	Agent  string           `json:"agent"`  // agent 插件名
	Status AgentStatusValue `json:"status"` // running | idle
}

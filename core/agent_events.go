package core

// 本文件定义「agent 回合生命周期」宿主事件（对齐 DSH 的 agent/status 与
// agent/error 原生事件）：主 agent 每回合 RunStream 结束时，宿主 EventBus 广播
// 通用事件——成功（success 终帧）发 agent/status(idle)，失败（error 终帧）发
// agent/error；回合开始发 agent/status(running)。事件经 EventBus 广播给订阅
// 插件（Hook.OnEvent），是任何插件皆可独立订阅的系统级能力（如通知插件区分
// 成败音效），绝非某插件专属。

const (
	// EventAgentStatus agent 回合状态事件（对齐 DSH agent/status）。
	// 宿主主 agent 每回合（RunStream）广播一次：
	//   running — 回合开始；
	//   idle    — 回合成功完成（success 终帧后）。
	EventAgentStatus EventName = "agent/status"

	// EventAgentError agent 回合报错事件（对齐 DSH agent/error）。
	// 回合以 error 终帧失败收尾时广播，供插件区分成功/失败（如通知音效）。
	EventAgentError EventName = "agent/error"
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

// AgentErrorEvent agent/error 事件的载荷（对齐 DSH { agent, turn, step, error }）。
type AgentErrorEvent struct {
	Agent string `json:"agent"` // agent 插件名
	Error string `json:"error"` // 失败原因文本
}

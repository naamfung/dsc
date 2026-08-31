package core

import (
	"context"
)

// 本文件实现「agent 回合状态事件」的统一包装：宿主把主 agent 包一层，使其
// RunStream 在回合完成（success/error 终帧）时向事件总线广播 agent/status
// （idle），回合开始时广播 running——对齐 DSH 由 agent-loop 原生发射 agent/
// status 的语义，而非依赖外部插件。TUI / -input / 插件桥等主对话路径统一经
// 此包装取得 agent（见 Manager.EventAgent），保证不遗漏任何回合。
//
// 事件经 EventBus 的 OnAny → broadcastEventToPlugins 自动广播给所有注册
// PluginHookService.OnEvent 的插件（如 notify），由订阅插件程序性响应
// （播放音效等），不涉及模型。

// eventWrappedAgent 包装一个 Agent，在 RunStream 流上插入 agent/status 事件。
type eventWrappedAgent struct {
	Agent
	name string
	m    *Manager
}

// EventAgent 返回主 agent 的事件包装实例；未加载主 agent 时返回 nil、false。
// 包装仅改变 RunStream（透传所有帧 + 发回合状态事件），其余方法直接透传。
func (m *Manager) EventAgent() (Agent, bool) {
	name := m.GetMainAgentName()
	if name == "" {
		return nil, false
	}
	agent, ok := m.GetAgent(name)
	if !ok {
		return nil, false
	}
	return &eventWrappedAgent{Agent: agent, name: name, m: m}, true
}

// EventAgentFor 按名称返回指定 agent 的事件包装（供非主 agent 场景）。
func (m *Manager) EventAgentFor(name string) (Agent, bool) {
	agent, ok := m.GetAgent(name)
	if !ok {
		return nil, false
	}
	return &eventWrappedAgent{Agent: agent, name: name, m: m}, true
}

// RunStream 透传底层 agent 的流帧，并在回合边界广播 agent/status 事件：
// 收到首帧后广播 running；遇到 success/error 终帧（agent 结束一轮）广播 idle。
func (w *eventWrappedAgent) RunStream(ctx context.Context, input string) (<-chan *RunStreamResponse, error) {
	ch, err := w.Agent.RunStream(ctx, input)
	if err != nil {
		return nil, err
	}
	// 广播 running（回合开始）
	w.m.emitAgentStatus(w.name, AgentStatusRunning)
	out := make(chan *RunStreamResponse)
	go func() {
		defer close(out)
		for item := range ch {
			out <- item
			// 回合终态：success 广播 agent/status(idle)，error 广播 agent/error
			switch item.Status {
			case "success":
				w.m.emitAgentStatus(w.name, AgentStatusIdle)
			case "error":
				w.m.emitAgentError(w.name, item.Error)
			}
		}
	}()
	return out, nil
}

// emitAgentStatus 向宿主事件总线广播 agent/status 事件（线程安全）。
func (m *Manager) emitAgentStatus(agent string, status AgentStatusValue) {
	m.events.Emit(EventAgentStatus, EventContext{Data: AgentStatusEvent{
		Agent:  agent,
		Status: status,
	}})
}

// emitAgentError 向宿主事件总线广播 agent/error 事件（线程安全）。
func (m *Manager) emitAgentError(agent, errMsg string) {
	m.events.Emit(EventAgentError, EventContext{Data: AgentErrorEvent{
		Agent: agent,
		Error: errMsg,
	}})
}

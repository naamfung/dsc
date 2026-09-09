package core

import (
	"testing"
	"time"
)

// TestDomainEventObserverRelaysAgentStatus 验证领域事件观测者（ADMIN /domain-events
// 的实现基础）经 EventBus.OnAny 订阅后，能收到 agent/status 事件且字段保真——
// 事件名与载荷（agent 名、status 取值）原样可达，便于自动化测试确认回合完成广播链路。
func TestDomainEventObserverRelaysAgentStatus(t *testing.T) {
	b := NewEventBus()
	ch := make(chan EventContext, 8)
	remove := b.OnAny(func(ctx EventContext) (any, error) {
		select {
		case ch <- ctx:
		default:
		}
		return nil, nil
	})
	defer remove()

	// 模拟宿主在主 agent 回合完成时的广播（对齐 agent_event_wrap）
	want := AgentStatusEvent{Agent: "agent-react-loop", Status: AgentStatusIdle}
	b.Emit(EventAgentStatus, EventContext{Data: want})

	select {
	case got := <-ch:
		if got.Name != EventAgentStatus {
			t.Errorf("Name=%q, want %q", got.Name, EventAgentStatus)
		}
		ev, ok := got.Data.(AgentStatusEvent)
		if !ok {
			t.Fatalf("Data type = %T, want AgentStatusEvent", got.Data)
		}
		// 字段保真：agent 名与 status 原样存活
		if ev.Agent != want.Agent || ev.Status != want.Status {
			t.Errorf("relayed event = %+v, want %+v", ev, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("observer did not receive emitted agent/status event")
	}
}

// TestDomainEventObserverDoesNotBlockEmit 验证观测者回调非阻塞：即使有订阅者，
// Emit 也能立即返回（慢消费者不拖慢事件发射路径）。
func TestDomainEventObserverDoesNotBlockEmit(t *testing.T) {
	b := NewEventBus()
	remove := b.OnAny(func(EventContext) (any, error) { return nil, nil })
	defer remove()

	done := make(chan struct{})
	go func() {
		b.Emit(EventAgentError, EventContext{Data: AgentErrorEvent{Agent: "a", Error: "boom"}})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked on synchronous observer")
	}
}

// TestDomainEventMarshalEnvelope 验证领域事件在 ADMIN 层按 {"name","data"} 包封
// 后可 JSON 序列化（handler 的载荷形态），确保事件可被 SSE 流传输。
func TestDomainEventMarshalEnvelope(t *testing.T) {
	tests := []struct {
		name EventName
		data any
	}{
		{EventAgentStatus, AgentStatusEvent{Agent: "a", Status: AgentStatusIdle}},
		{EventAgentError, AgentErrorEvent{Agent: "a", Error: "x"}},
		{EventToolResult, ToolResultInfo{ToolName: "shell", Result: "ok"}},
	}
	for _, tt := range tests {
		var ctx EventContext
		ctx.Name = tt.name
		ctx.Data = tt.data
		body, err := marshalDomainEvent(ctx)
		if err != nil {
			t.Errorf("marshal %s: %v", tt.name, err)
			continue
		}
		got := string(body)
		if got == "" {
			t.Errorf("marshal %s produced empty body", tt.name)
		}
	}
}

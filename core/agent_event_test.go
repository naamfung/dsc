package core

import (
	"context"
	"testing"
)

// streamAgent 一个可发射指定帧序列的 test agent。
type streamAgent struct {
	mockAgent
	frames []*RunStreamResponse
}

func (s *streamAgent) RunStream(ctx context.Context, input string) (<-chan *RunStreamResponse, error) {
	ch := make(chan *RunStreamResponse)
	go func() {
		defer close(ch)
		for _, f := range s.frames {
			ch <- f
		}
	}()
	return ch, nil
}

// TestEventAgentEmitsAgentStatusOnTurnComplete 验证事件包装：主 agent 回合结束
// （success 终帧）时，宿主 EventBus 广播 agent/status(idle) 事件；运行期间不发。
func TestEventAgentEmitsAgentStatusOnTurnComplete(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})
	// 注册 listeners 收集 agent/status 事件
	var got []AgentStatusEvent
	m.events.On(EventAgentStatus, func(ctx EventContext) (any, error) {
		if ev, ok := ctx.Data.(AgentStatusEvent); ok {
			got = append(got, ev)
		}
		return nil, nil
	})

	agent := &eventWrappedAgent{
		Agent: &streamAgent{frames: []*RunStreamResponse{
			{Status: "streaming", Output: "hi"},
			{Status: "success", Usage: &Usage{TotalTokens: 10}},
		}},
		name: "agent-react-loop",
		m:    m,
	}

	ch, err := agent.RunStream(context.Background(), "hello")
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	var outputs []string
	for f := range ch {
		if f.Output != "" {
			outputs = append(outputs, f.Output)
		}
	}
	if len(outputs) != 1 || outputs[0] != "hi" {
		t.Fatalf("流透传异常: %+v", outputs)
	}

	if len(got) != 2 {
		t.Fatalf("应广播 running + idle 两个 agent/status 事件, got %d: %+v", len(got), got)
	}
	if got[0].Status != AgentStatusRunning {
		t.Fatalf("首个事件应为 running, got %+v", got[0])
	}
	if got[1].Status != AgentStatusIdle || got[1].Agent != "agent-react-loop" {
		t.Fatalf("回合完成应广播 idle, got %+v", got[1])
	}
}

// TestEventAgentErrorEmitsAgentError 验证 error 终帧广播 agent/error 事件
// （失败回合触发 error，而非 idle——供通知区分成败音效）。
func TestEventAgentErrorEmitsAgentError(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})
	var errs []AgentErrorEvent
	var statuses []AgentStatusEvent
	m.events.On(EventAgentError, func(ctx EventContext) (any, error) {
		if ev, ok := ctx.Data.(AgentErrorEvent); ok {
			errs = append(errs, ev)
		}
		return nil, nil
	})
	m.events.On(EventAgentStatus, func(ctx EventContext) (any, error) {
		if ev, ok := ctx.Data.(AgentStatusEvent); ok {
			statuses = append(statuses, ev)
		}
		return nil, nil
	})
	agent := &eventWrappedAgent{
		Agent: &streamAgent{frames: []*RunStreamResponse{{Status: "error", Error: "boom"}}},
		name:  "agent-react-loop",
		m:     m,
	}
	ch, _ := agent.RunStream(context.Background(), "x")
	var n int
	for range ch {
		n++
	}
	if n != 1 {
		t.Fatalf("帧透传数 = %d, want 1", n)
	}
	// error 终帧：广播 agent/error，且不再广播 agent/status(idle)
	if len(errs) != 1 || errs[0].Agent != "agent-react-loop" || errs[0].Error != "boom" {
		t.Fatalf("error 终帧应广播 %d 个 agent/error, got %+v", 1, errs)
	}
	for _, s := range statuses {
		if s.Status == AgentStatusIdle {
			t.Fatalf("失败回合不应广播 idle, got %+v", statuses)
		}
	}
}

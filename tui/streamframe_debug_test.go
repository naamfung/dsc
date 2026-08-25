package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"dsc/plugin"
)

// stubAgent 返回一个预填充的流通道（reasoning → streaming → success）
type stubAgent struct {
	frames       []*plugin.RunStreamResponse
	switchCalls  []string
	planCalls    []bool
	historyCalls []int
}

func (s *stubAgent) RunStream(_ context.Context, _ string) (<-chan *plugin.RunStreamResponse, error) {
	ch := make(chan *plugin.RunStreamResponse)
	go func() {
		defer close(ch)
		for _, f := range s.frames {
			ch <- f
		}
	}()
	return ch, nil
}

func (s *stubAgent) Run(context.Context, string) (*plugin.AgentResult, error) {
	return nil, nil
}

func (s *stubAgent) Name(context.Context) string                            { return "stub" }
func (s *stubAgent) Version(context.Context) string                         { return "0" }
func (s *stubAgent) RegisterServices(context.Context, uint32, uint32) error { return nil }
func (s *stubAgent) SwitchSession(_ context.Context, id string) error {
	s.switchCalls = append(s.switchCalls, id)
	return nil
}
func (s *stubAgent) SetPlanMode(_ context.Context, active bool) error {
	s.planCalls = append(s.planCalls, active)
	return nil
}
func (s *stubAgent) SetHistoryInjection(_ context.Context, count int) error {
	s.historyCalls = append(s.historyCalls, count)
	return nil
}
func (s *stubAgent) SetUserQuestionsService(context.Context, uint32) error { return nil }
func (s *stubAgent) Shutdown(context.Context, bool) error                  { return nil }
func (s *stubAgent) InjectMessage(context.Context, string) error           { return nil }
func (s *stubAgent) DebugSnapshot(context.Context) (*plugin.AgentDebugSnapshot, error) {
	return &plugin.AgentDebugSnapshot{SessionID: "stub"}, nil
}

func TestPumpLoopRealFlow(t *testing.T) {
	m := New(&stubAgent{frames: []*plugin.RunStreamResponse{
		{Status: "reasoning", Reasoning: "1. 用户发了好\n2. 需要回应\n回应策略"},
		{Status: "reasoning", Reasoning: "之一：礼貌回答"},
		{Status: "streaming", Output: "我在的"},
		{Status: "streaming", Output: "，有咩帮你？"},
		{Status: "success"},
	}}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 通过真实消息队列执行：submitCmd → first 帧 → pump 循环
	var model tea.Model = m
	var cmd tea.Cmd
	cmd = model.(*Model).submitCmd("你好")
	firstMsg := cmd()
	model, cmd = model.Update(firstMsg)

	// 反复执行返回的 cmd，收集后续帧
	for i := 0; i < 20 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	m2 := model.(*Model)
	t.Logf("streamBuffer=%q", m2.streamBuffer)
	t.Logf("streaming=%v streamOpen=%v", m2.streaming, m2.streamOpen)
	for i, l := range m2.lines {
		t.Logf("line[%d]=%q", i, l)
	}
	full := strings.Join(m2.lines, "\n")
	if !strings.Contains(full, "我在的") {
		t.Fatalf("正文未渲染: %q", full)
	}
}

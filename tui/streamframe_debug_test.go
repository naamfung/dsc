package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"dsc/core"
)

// stubAgent 返回一个预填充的流通道（reasoning → streaming → success）
type stubAgent struct {
	frames       []*core.RunStreamResponse
	switchCalls  []string
	planCalls    []bool
	historyCalls []int
}

func (s *stubAgent) RunStream(_ context.Context, _ string, _ []string) (<-chan *core.RunStreamResponse, error) {
	ch := make(chan *core.RunStreamResponse)
	go func() {
		defer close(ch)
		for _, f := range s.frames {
			ch <- f
		}
	}()
	return ch, nil
}

func (s *stubAgent) Run(context.Context, string, []string) (*core.AgentResult, error) {
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
func (s *stubAgent) InjectMessage(context.Context, string, []string) error { return nil }
func (s *stubAgent) DebugSnapshot(context.Context) (*core.AgentDebugSnapshot, error) {
	return &core.AgentDebugSnapshot{SessionID: "stub"}, nil
}

func TestPumpLoopRealFlow(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
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
	cmd = model.(*Model).submitCmd("你好", nil)
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

// TestInterruptThenNewTurnStreams 回归：Ctrl+C 中断本轮后，新开启一轮的 first 帧必须
// 被放行并复位 streamCancelled，否则该标记永不复位、之后所有轮的模型输出帧都被丢弃
// （用户「中断对话后就再见不到模型输出」的自锁缺陷）。
func TestInterruptThenNewTurnStreams(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
		{Status: "reasoning", Reasoning: "思考"},
		{Status: "streaming", Output: "第一轮正文"},
		{Status: "success"},
	}}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	var model tea.Model = m
	// 第一轮：提交并处理 first 帧，进入流式状态
	cmd := model.(*Model).submitCmd("第一轮", nil)
	firstMsg := cmd()
	var streamDecided tea.Cmd
	model, streamDecided = model.Update(firstMsg)
	if model.(*Model).streaming != true {
		t.Fatalf("第一轮 first 帧后应进入流式: streaming=%v", model.(*Model).streaming)
	}
	_ = streamDecided

	// 模拟流式中 Ctrl+C：中断本轮 → streamCancelled=true 并清流
	model, _ = model.(*Model).interruptTurn()
	if !model.(*Model).streamCancelled {
		t.Fatalf("中断后 streamCancelled 应为 true")
	}

	// 新一轮：first 帧必须放行（走 first 分支复位 streamCancelled=false 并回到流式），
	// 而非被 854 的 streamCancelled 检查丢弃
	next := model.(*Model).submitCmd("第二轮", nil)
	nextMsg := next()
	model, nextCmd := model.Update(nextMsg)
	m2 := model.(*Model)
	if m2.streamCancelled {
		t.Fatalf("新一轮 first 帧被误丢弃：streamCancelled 未复位，后续轮将无声")
	}
	if !m2.streaming {
		t.Fatalf("新一轮 first 帧后应回到流式输出: streaming=%v", m2.streaming)
	}
	if nextCmd == nil {
		t.Fatalf("新一轮 first 帧后应继续泵取流")
	}
}

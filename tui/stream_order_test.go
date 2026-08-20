package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"dsc/plugin"
)

// 复现实际服务端的乱序流：先发 text（正文），后发 thinking（思考块）。
// 修复前，迟到的思考块会覆盖已渲染的正文，导致流结束时屏幕上只剩思考块、正文丢失。
func TestPumpLoopReversedOrderContentBeforeReasoning(t *testing.T) {
	m := New(&stubAgent{frames: []*plugin.RunStreamResponse{
		{Status: "streaming", Output: "你好！我在的，请问有什么我可以帮你呢？"},
		{Status: "reasoning", Reasoning: "思考過程：\n1. 用戶問好\n2. 直接回答即可"},
		{Status: "success"},
	}}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	var model tea.Model = m
	var cmd tea.Cmd
	cmd = model.(*Model).submitCmd("你好")
	firstMsg := cmd()
	model, cmd = model.Update(firstMsg)
	for i := 0; i < 20 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	m2 := model.(*Model)
	full := strings.Join(m2.lines, "\n")
	if !strings.Contains(full, "你好！我在的") {
		t.Fatalf("正文应保留但未渲染: %q", full)
	}
	if !strings.Contains(full, "思考過程") {
		t.Fatalf("思考块应渲染: %q", full)
	}
}

// 思考块与正文有序到达（先思考后正文）时，两者都应当完整保留。
func TestPumpLoopOrderedReasoningThenContent(t *testing.T) {
	m := New(&stubAgent{frames: []*plugin.RunStreamResponse{
		{Status: "reasoning", Reasoning: "思考過程：\n1. 分析"},
		{Status: "streaming", Output: "正文内容"},
		{Status: "success"},
	}}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	var model tea.Model = m
	var cmd tea.Cmd
	cmd = model.(*Model).submitCmd("你好")
	firstMsg := cmd()
	model, cmd = model.Update(firstMsg)
	for i := 0; i < 20 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	m2 := model.(*Model)
	full := strings.Join(m2.lines, "\n")
	if !strings.Contains(full, "思考過程") {
		t.Fatalf("思考块未渲染: %q", full)
	}
	if !strings.Contains(full, "正文内容") {
		t.Fatalf("正文未渲染: %q", full)
	}
	// 思考块应位于正文之前
	if strings.Index(full, "思考過程") > strings.Index(full, "正文内容") {
		t.Fatalf("思考块应位于正文前: %q", full)
	}
}

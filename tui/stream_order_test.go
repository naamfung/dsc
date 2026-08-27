package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"dsc/core"
)

// 复现实际服务端的乱序流：先发 text（正文），后发 thinking（思考块）。
// 修复前，迟到的思考块会覆盖已渲染的正文，导致流结束时屏幕上只剩思考块、正文丢失。
func TestPumpLoopReversedOrderContentBeforeReasoning(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
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
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
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

// 思考块内容应当经过 markdown 渲染（如加粗），而不是原样纯文本。
func TestRenderReasoningMarkdown(t *testing.T) {
	out := renderReasoning("关键决策：**必须**重试，代码 `go run`。", 60)
	if !strings.Contains(out, "关键决策") {
		t.Fatalf("思考文本缺失: %q", out)
	}
	if !strings.Contains(out, "必须") {
		t.Fatalf("加粗文本缺失: %q", out)
	}
	if !strings.Contains(out, "▎ ") {
		t.Fatalf("思考块应带 ▎ 标记: %q", out)
	}
	// 加粗应被渲染为 ANSI 粗体（含转义序列），而非原样输出 ** 双星号
	if strings.Contains(out, "**必须**") {
		t.Fatalf("加粗未被 markdown 渲染，仍为字面量: %q", out)
	}
	if !strings.ContainsRune(out, '\x1b') {
		t.Fatalf("思考块应含 ANSI 样式（markdown 渲染）: %q", out)
	}
	// 暗色应在每次行内样式复位后重新套用，确保弱化不被加粗等样式清掉
	reapply := "\x1b[m" + reasonReapply
	if !strings.Contains(out, reapply) {
		t.Fatalf("行内样式复位后未重新套用暗色: %q", out)
	}
	if !strings.HasPrefix(out, reasonReapply) {
		t.Fatalf("思考块整体应带暗色前缀: %q", out)
	}
	// 末尾必须复位，避免开着的暗色泄漏到随后的正文
	if !strings.HasSuffix(out, "\x1b[m\n") {
		t.Fatalf("思考块末尾应复位，防止弱化泄漏到正文: %q", out)
	}
}

// 思考块弱化不应泄漏到紧随其后的正文。
func TestPumpLoopReasoningDimDoesNotLeakToBody(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
		{Status: "streaming", Output: "正文示例"},
		{Status: "reasoning", Reasoning: "思考：**重点**分析"},
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
	// 正文应保持普通样式：其位置不应紧随一个“开着的”暗色前缀
	bodyIdx := strings.Index(full, "正文示例")
	if bodyIdx < 0 {
		t.Fatalf("正文缺失: %q", full)
	}
	// 正文前应已复位：上一个 \x1b[m 复位应在暗色前缀之后，且正文前没有挂起的 reasonReapply
	pre := full[:bodyIdx]
	if strings.HasSuffix(pre, reasonReapply) {
		t.Fatalf("正文前残留开着的暗色，正文被弱化: %q", pre)
	}
}

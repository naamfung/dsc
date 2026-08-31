package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"dsc/core"
)

// TestVirtualWindowBoundsViewportContent 性能回归：无论会话内容多大，喂给 viewport 的
// 内容始终只含可视窗口 h 行（而非全部行）。修复前每帧 SetContent 全量内容并在 viewport
// 内部 O(n) 重扫，长会话/拖拽/流式卡顿；虚拟窗口后 viewport 只装 [viewStart, +h)。
func TestVirtualWindowBoundsViewportContent(t *testing.T) {
	var frames []*core.RunStreamResponse
	for i := 0; i < 200; i++ {
		frames = append(frames, &core.RunStreamResponse{
			Status: "streaming",
			Output: fmt.Sprintf("内容行%03d ", i) + strings.Repeat("x", 60) + "\n" + fmt.Sprintf("内容行%03d-2", i) + "\n",
		})
	}
	frames = append(frames, &core.RunStreamResponse{Status: "success"})

	m := New(&stubAgent{frames: frames}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	var model tea.Model = m
	var cmd tea.Cmd
	cmd = model.(*Model).submitCmd("你好")
	firstMsg := cmd()
	model, cmd = model.Update(firstMsg)
	for i := 0; i < 50 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	m2 := model.(*Model)

	// 内容已远超一屏：全量可视行应远大于窗口高度
	if total := len(m2.wrappedLines); total <= m2.viewport.Height() {
		t.Fatalf("前置条件：内容应溢出视口（total=%d, height=%d）", total, m2.viewport.Height())
	}
	// 行号映射完整：每个语义行都有起始可视行
	if len(m2.lineRowStart) != len(m2.lines) {
		t.Fatalf("lineRowStart 未覆盖全部语义行: %d != %d", len(m2.lineRowStart), len(m2.lines))
	}
	// viewport 内容被虚拟窗口钳制在屏幕高度内（核心性能性质）
	gotLines := strings.Split(m2.viewport.GetContent(), "\n")
	if len(gotLines) > m2.viewport.Height() {
		t.Fatalf("viewport 内容行数 = %d, 超过窗口高度 %d（应只装可视窗口）", len(gotLines), m2.viewport.Height())
	}
	// 钉在底部时窗口恰好显示末尾 h 行
	expectFirst := m2.virtualMaxYOffset()
	if m2.viewStart != expectFirst {
		t.Fatalf("流式结束后应钉底: viewStart=%d, want %d", m2.viewStart, expectFirst)
	}
	if got := strings.TrimSpace(ansi.Strip(gotLines[0])); !strings.Contains(got, "内容行") {
		t.Fatalf("窗口首行意外: %q", got)
	}
}

// TestVirtualWindowStreamingKeepsLastLineLive 流式时末行就地替换（reflattenFrom 尾部重排），
// 窗口仍跟随且 viewport 内容保持 h 行；修复前流式逐字卡顿的根源是全量重 wrap + 全量喂入。
func TestVirtualWindowStreamingKeepsLastLineLive(t *testing.T) {
	var frames []*core.RunStreamResponse
	for i := 0; i < 120; i++ {
		frames = append(frames, &core.RunStreamResponse{
			Status: "streaming",
			Output: strings.Repeat("内容", 30) + fmt.Sprintf(" %d", i) + "\n",
		})
	}
	frames = append(frames, &core.RunStreamResponse{Status: "success"})

	m := New(&stubAgent{frames: frames}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	var model tea.Model = m
	var cmd tea.Cmd
	cmd = model.(*Model).submitCmd("你好")
	firstMsg := cmd()
	model, cmd = model.Update(firstMsg)
	for i := 0; i < 40 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	m2 := model.(*Model)
	if !m2.streaming {
		t.Fatal("前置条件：应处于流式状态")
	}
	// 流式中窗口内容仍受窗口高度限制
	if got := strings.Split(m2.viewport.GetContent(), "\n"); len(got) > m2.viewport.Height() {
		t.Fatalf("流式中 viewport 内容行数 = %d, 超过窗口高度 %d", len(got), m2.viewport.Height())
	}
	// 末行内容在视口中可见（流式末尾行仍被渲染）
	last := m2.wrappedLines[len(m2.wrappedLines)-1]
	if strings.Contains(strings.TrimSpace(ansi.Strip(last)), "内容") == false {
		t.Fatalf("流式末行应渲染可见: %q", ansi.Strip(last))
	}
	// 行号映射仍完整
	if len(m2.lineRowStart) != len(m2.lines) {
		t.Fatalf("流式后 lineRowStart 未覆盖全部语义行: %d != %d", len(m2.lineRowStart), len(m2.lines))
	}
}

// TestVirtualWindowScrollShiftsWindow 滚轮滚动后 viewStart 变化，且 viewport 内容仍为 h 行窗口。
func TestVirtualWindowScrollShiftsWindow(t *testing.T) {
	var frames []*core.RunStreamResponse
	for i := 0; i < 100; i++ {
		frames = append(frames, &core.RunStreamResponse{
			Status: "streaming",
			Output: fmt.Sprintf("行%03d ", i) + strings.Repeat("y", 60) + "\n",
		})
	}
	frames = append(frames, &core.RunStreamResponse{Status: "success"})
	m := pumpFrames(t, frames)

	before := m.viewStart
	model, _ := m.Update(tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelUp})
	m2 := model.(*Model)
	if m2.viewStart >= before {
		t.Fatalf("滚轮向上应减小 viewStart: before=%d after=%d", before, m2.viewStart)
	}
	if got := strings.Split(m2.viewport.GetContent(), "\n"); len(got) > m2.viewport.Height() {
		t.Fatalf("滚动后 viewport 内容行数 = %d, 超过窗口高度 %d", len(got), m2.viewport.Height())
	}
	// 窗口顶部与 viewStart 一致：可见首行即 viewStart 处的可视行
	wantFirst := strings.TrimSpace(ansi.Strip(m2.wrappedLines[m2.viewStart]))
	gotFirst := strings.TrimSpace(ansi.Strip(strings.Split(m2.viewport.GetContent(), "\n")[0]))
	if wantFirst != gotFirst {
		t.Fatalf("窗口首行应等于 viewStart 处内容: got %q, want %q", gotFirst, wantFirst)
	}
}

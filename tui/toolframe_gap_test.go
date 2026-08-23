package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dsc/plugin"
	"fmt"
	"github.com/charmbracelet/x/ansi"
)

// TestToolFrameGapVisual 用真实 lipgloss 渲染两个连续工具结果帧，
// 观察它们之间的视觉间隔，并写入 tui/toolframe_gap_visual.txt 供人工核对。
func TestToolFrameGapVisual(t *testing.T) {
	m := New(&stubAgent{frames: []*plugin.RunStreamResponse{
		{Status: "streaming", Output: "助手正文第一行\n助手正文第二行"},
		{Status: "tool", ToolResult: "A结果行1\nA结果行2\nA结果行3"},
		{Status: "tool", ToolResult: "B结果行1\nB结果行2"},
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

	// 真实 lipgloss 渲染（与 render() 一致），去掉 ANSI 控制码后写入文件人工核对。
	w := 100
	var rows []string
	for _, line := range m2.lines {
		rendered := lipgloss.NewStyle().Width(w).Render(line)
		rows = append(rows, strings.Split(rendered, "\n")...)
	}
	var b strings.Builder
	b.WriteString("=== 实际视觉行（去掉 ANSI 控制码后）===\n")
	for i, r := range rows {
		b.WriteString(fmt.Sprintf("%2d | %s\n", i, ansi.Strip(r)))
	}
	if err := os.WriteFile("tui/toolframe_gap_visual.txt", []byte(b.String()), 0644); err != nil {
		t.Fatalf("写视觉文件失败: %v", err)
	}
}

// TestToolFrameGapStructural 结构断言：两个连续工具结果帧之间必须有双空行分隔（\n\n）。
func TestToolFrameGapStructural(t *testing.T) {
	m := New(&stubAgent{frames: []*plugin.RunStreamResponse{
		{Status: "streaming", Output: "助手正文第一行"},
		{Status: "tool", ToolResult: "A结果行1\nA结果行2"},
		{Status: "tool", ToolResult: "B结果行1\nB结果行2"},
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

	// 两个连续工具结果帧之间必须有双空行（\n\n\n：帧尾换行 + 双空行）。
	// 使用 ansi.Strip 移除 ANSI 控制码后再进行结构断言。
	stripFull := ansi.Strip(full)
	if !strings.Contains(stripFull, "A结果行2\n\n\n  └ B结果行1") {
		t.Fatalf("两个连续工具结果帧之间应有双空行分隔，实际:\n%q", stripFull)
	}
}

package tui

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dsc/core"
	"fmt"
	"github.com/charmbracelet/x/ansi"
)

// TestToolFrameGapVisual 用真实 lipgloss 渲染两个连续工具结果帧，
// 观察它们之间的视觉间隔，并写入 tui/toolframe_gap_visual.txt 供人工核对。
func TestToolFrameGapVisual(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
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
	if err := os.WriteFile("toolframe_gap_visual.txt", []byte(b.String()), 0644); err != nil {
		t.Fatalf("写视觉文件失败: %v", err)
	}
}

// TestToolFrameGapStructural 结构断言：两个连续工具结果帧之间以单空行分隔（紧凑展示）。
func TestToolFrameGapStructural(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
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

	// 两个连续工具结果帧之间以单空行分隔（帧尾换行 + 帧间空行，共 \n\n）。
	// 使用 ansi.Strip 移除 ANSI 控制码后再进行结构断言。
	stripFull := ansi.Strip(full)
	if !strings.Contains(stripFull, "A结果行2\n\n  └ B结果行1") {
		t.Fatalf("两个连续工具结果帧之间应为单空行分隔，实际:\n%q", stripFull)
	}
}

// TestToolCallResultContiguous 结构断言：工具「调用标题」帧与其紧跟的「结果」帧
// 之间不留空行（对齐 REX 的紧凑卡片布局），而非像普通帧间那样以空行分隔。
func TestToolCallResultContiguous(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
		{Status: "streaming", Output: "助手正文"},
		{Status: "tool", ToolName: "shell", ToolArgs: `{"command":"cat a.go"}`},
		{Status: "tool", ToolName: "shell", ToolArgs: `{"command":"cat a.go"}`, ToolResult: "=====SUM=====\ncat: No such file\n\n[exit_code: 1]\n"},
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
	stripFull := ansi.Strip(strings.Join(m2.lines, "\n"))

	// 调用标题与结果首行紧贴：标题行后直接是「└ 结果」，之间不该有空行。
	if !strings.Contains(stripFull, "● Shell(cat a.go)\n  └ =====SUM=====") {
		t.Fatalf("调用标题与结果之间应无空行（紧贴展示），实际:\n%q", stripFull)
	}
	// 退出码 [exit_code: 1] 应独占一行（结果最后非空行）。
	if !hasTrimmedLine(stripFull, "[exit_code: 1]") {
		t.Fatalf("退出码应在结果中独占一行，实际:\n%q", stripFull)
	}
}

// hasTrimmedLine 报告多行字符串中是否存在某一行去掉首尾空白后与 want 相等。
func hasTrimmedLine(s, want string) bool {
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == want {
			return true
		}
	}
	return false
}

// TestScrollbarDragDuringStreaming 验证模型工作（流式）期间滚动条仍可鼠标拖动，
// 且拖动/滚轮向上会「脱离底部自动跟随」——后续流式帧不再把视口硬拉回底（对齐
// REX pinnedToBottom 语义）；正文拖选复制在流式期间仍被禁用（不回归原设计）。
func TestScrollbarDragDuringStreaming(t *testing.T) {
	// 足够多的行使视口溢出（出现滚动条），且余量保证流式仍在进行中
	var frames []*core.RunStreamResponse
	for i := 0; i < 40; i++ {
		frames = append(frames, &core.RunStreamResponse{
			Status: "streaming",
			Output: fmt.Sprintf("内容行%02d ", i) + strings.Repeat("x", 60) + "\n" + fmt.Sprintf("内容行%02d-2", i) + "\n",
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
	// 泵足够多帧使内容溢出视口，同时保持流式进行中（通道仍有余帧）
	for i := 0; i < 25 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	m2 := model.(*Model)
	if !m2.streaming {
		t.Fatal("前置条件：应处于流式状态")
	}
	atBottomYoff := m2.viewport.YOffset()
	if atBottomYoff == 0 {
		t.Fatalf("前置条件：内容应溢出视口（YOffset=%d，应有滚动条）", atBottomYoff)
	}

	// 工作期间点按滚动条：应进入拖拽并脱离自动跟随
	sx := m2.viewport.Width() // 滚动条列（内容区右侧一列）
	model, _ = model.Update(tea.MouseClickMsg{X: sx, Y: 8, Button: tea.MouseLeft})
	m2 = model.(*Model)
	if !m2.scrollbarDrag {
		t.Fatal("模型工作期间点按滚动条应进入拖拽模式")
	}
	if m2.pinnedToBottom {
		t.Fatal("进入滚动条拖拽应脱离底部自动跟随（pinnedToBottom=false）")
	}

	// 向上拖拽：YOffset 应减小
	model, _ = model.Update(tea.MouseMotionMsg{X: sx, Y: 3, Button: tea.MouseLeft})
	m2 = model.(*Model)
	if !m2.scrollbarDrag {
		t.Fatal("拖拽中应保持 scrollbarDrag")
	}
	detachedYoff := m2.viewport.YOffset()
	if detachedYoff >= atBottomYoff {
		t.Fatalf("向上拖拽应减小 YOffset: got %d, want < %d", detachedYoff, atBottomYoff)
	}

	// 继续泵一帧流式：脱离后视口不应被拉回底部
	if cmd == nil {
		t.Fatal("流式通道应仍打开")
	}
	msg := cmd()
	model, cmd = model.Update(msg)
	m2 = model.(*Model)
	if got := m2.viewport.YOffset(); got != detachedYoff {
		t.Fatalf("脱离跟随后流式帧不应把视口拉回底: got %d, want %d", got, detachedYoff)
	}

	// 松开：退出拖拽；未回到底部则保持脱离
	model, _ = model.Update(tea.MouseReleaseMsg{X: sx, Y: 3, Button: tea.MouseLeft})
	m2 = model.(*Model)
	if m2.scrollbarDrag {
		t.Fatal("松开后应退出拖拽")
	}
	if m2.pinnedToBottom {
		t.Fatal("未回到底部松开后应保持脱离（pinnedToBottom=false）")
	}
}

// TestWheelDetachesAndRepins 验证滚轮向上翻阅脱离底部跟随、向下滚回底部重新钉住。
func TestWheelDetachesAndRepins(t *testing.T) {
	var frames []*core.RunStreamResponse
	for i := 0; i < 40; i++ {
		frames = append(frames, &core.RunStreamResponse{
			Status: "streaming",
			Output: fmt.Sprintf("内容行%02d ", i) + strings.Repeat("y", 60) + "\n",
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
	for i := 0; i < 25 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	m2 := model.(*Model)
	if m2.viewport.YOffset() == 0 {
		t.Fatal("前置条件：内容应溢出视口")
	}
	if !m2.pinnedToBottom {
		t.Fatal("前置条件：流式开始时应钉在底部")
	}

	// 滚轮向上 → 脱离
	model, _ = model.Update(tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelUp})
	m2 = model.(*Model)
	if m2.pinnedToBottom {
		t.Fatal("滚轮向上应脱离底部自动跟随")
	}
	upYoff := m2.viewport.YOffset()
	if upYoff == 0 {
		t.Fatal("滚轮向上应滚动到更早内容（YOffset>0 且非底部）")
	}

	// 滚轮向下足够多次滚回底部 → 重新钉住
	for i := 0; i < 50 && !m2.pinnedToBottom; i++ {
		model, _ = model.Update(tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelDown})
		m2 = model.(*Model)
	}
	if !m2.pinnedToBottom {
		t.Fatal("滚回底部应重新钉住（pinnedToBottom=true）")
	}
	if !m2.viewport.AtBottom() {
		t.Fatal("滚回底部后视口应位于底部")
	}
}

// TestToolResultReasoningSeparated 结构断言：工具结果块（末行无结尾换行）与紧随其后的
// 助手思考块之间应有空行分隔，避免上一块末行与 ✦ DSC 身份头紧贴粘连。
func TestToolResultReasoningSeparated(t *testing.T) {
	m := New(&stubAgent{frames: []*core.RunStreamResponse{
		{Status: "tool", Output: "60.0K src/llama-grammar.cpp\n32.0K src/llama-model.h"},
		{Status: "reasoning", Reasoning: "分析"},
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
	stripFull := ansi.Strip(strings.Join(m2.lines, "\n"))
	// 末行可能被宽度对齐补尾随空格，故以「非换行字符 + 空行 + ✦ DSC」判定分隔。
	sep := regexp.MustCompile(`llama-model\.h[^\n]*\n\n\s*✦ DSC`)
	if !sep.MatchString(stripFull) {
		t.Fatalf("工具结果与思考块之间应有空行分隔，实际:\n%q", stripFull)
	}
}

// TestEnsureExitCodeLine 结构断言：工具结果中紧贴上一行输出（未补换行）的
// [exit_code: N] 会被补换行独占一行；已位于行首的保持不变。
func TestEnsureExitCodeLine(t *testing.T) {
	glued := renderToolResult("done writing output [exit_code: 0]", false)
	if !hasTrimmedLine(ansi.Strip(glued), "[exit_code: 0]") {
		t.Fatalf("紧贴的退出码未换行独占一行，实际:\n%q", glued)
	}
	lineStart := renderToolResult("a\n[exit_code: 1]", true)
	if !hasTrimmedLine(ansi.Strip(lineStart), "[exit_code: 1]") {
		t.Fatalf("行首退出码应保持不变，实际:\n%q", lineStart)
	}
}

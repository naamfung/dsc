package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"dsc/core"
)

func TestScrollbarThumb(t *testing.T) {
	cases := []struct {
		name                string
		height, yoff, total int
		wantStart           int
		wantSize            int
	}{
		{"no overflow no thumb", 10, 0, 5, 0, 0},
		{"at top", 10, 0, 100, 0, 1},
		{"at bottom thumb touches edge", 10, 90, 100, 9, 1},
		{"mid", 10, 45, 100, 4, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, size := scrollbarThumb(c.height, c.yoff, c.total)
			if start != c.wantStart || size != c.wantSize {
				t.Fatalf("scrollbarThumb(%d,%d,%d) = (%d,%d), want (%d,%d)",
					c.height, c.yoff, c.total, start, size, c.wantStart, c.wantSize)
			}
		})
	}
}

// TestScrollbarCell 内容不满一屏时无滑块；滑块行与轨道行字符不同。
func TestScrollbarCell(t *testing.T) {
	if got := scrollbarCell(0, 5, 10, 0, 0); got != " " {
		t.Fatalf("no-overflow cell = %q, want space", got)
	}
	thumb := scrollbarCell(0, 100, 10, 0, 1)
	track := scrollbarCell(1, 100, 10, 0, 1)
	if thumb == track {
		t.Fatalf("thumb %q 与 track %q 应不同", thumb, track)
	}
	if !strings.Contains(thumb, "█") || !strings.Contains(track, "│") {
		t.Fatalf("thumb=%q track=%q 应分别含 █ 与 │", thumb, track)
	}
}

// pumpFrames 提交输入并泵取 stubAgent 的帧，得到渲染完成的 Model。
func pumpFrames(t *testing.T, frames []*core.RunStreamResponse) *Model {
	t.Helper()
	m := New(&stubAgent{frames: frames}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})

	var model tea.Model = m
	var cmd tea.Cmd
	cmd = model.(*Model).submitCmd("你好", nil)
	firstMsg := cmd()
	model, cmd = model.Update(firstMsg)
	for i := 0; i < 20 && cmd != nil; i++ {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	return model.(*Model)
}

// TestViewportViewAppendsScrollbar 验证 viewportView 在末列拼接滚动条：
// 内容溢出时出现轨道/滑块列；布局总宽 = 终端宽。
func TestViewportViewAppendsScrollbar(t *testing.T) {
	long := strings.Repeat("第%d行内容\n", 0)
	_ = long
	var frames []*core.RunStreamResponse
	frames = append(frames, &core.RunStreamResponse{Status: "streaming", Output: strings.Repeat("填充内容行\n", 30)})
	frames = append(frames, &core.RunStreamResponse{Status: "success"})
	m := pumpFrames(t, frames)

	v := m.viewportView()
	rows := strings.Split(v, "\n")
	if len(rows) != m.viewport.Height() {
		t.Fatalf("viewportView 行数 = %d, want %d", len(rows), m.viewport.Height())
	}
	// 内容已溢出：末列应为轨道/滑块字符（非空格）
	lastCol := make([]rune, len(rows))
	for i, r := range rows {
		s := ansi.Strip(r)
		if s == "" {
			lastCol[i] = ' '
			continue
		}
		lastCol[i] = []rune(s)[len([]rune(s))-1]
	}
	hasThumb := false
	for _, c := range lastCol {
		if c == '█' {
			hasThumb = true
			break
		}
	}
	if !hasThumb {
		t.Fatalf("滚动条应出现滑块 █，末列=%q", string(lastCol))
	}
	// 每行宽度 = 终端宽（内容宽 + 滚动条列）
	if w := ansi.StringWidth(rows[0]); w != m.width {
		t.Fatalf("viewport 行宽 = %d, want 终端宽 %d", w, m.width)
	}
}

// TestScrollbarNoOverflowNoThumb 内容不满一屏时末列为空格（无滚动条）。
func TestScrollbarNoOverflowNoThumb(t *testing.T) {
	m := pumpFrames(t, []*core.RunStreamResponse{
		{Status: "streaming", Output: "短内容"},
		{Status: "success"},
	})
	v := m.viewportView()
	for _, r := range strings.Split(v, "\n") {
		s := ansi.Strip(r)
		if s != "" && strings.HasSuffix(s, "█") {
			t.Fatalf("内容未溢出时不应出现滑块: %q", s)
		}
	}
}

// TestScrollbarDrag 点按滚动条列进入拖拽，拖动改变 YOffset，松开结束拖拽；
// 点按正文不进入拖拽。
func TestScrollbarDrag(t *testing.T) {
	var frames []*core.RunStreamResponse
	frames = append(frames, &core.RunStreamResponse{Status: "streaming", Output: strings.Repeat("填充内容行\n", 30)})
	frames = append(frames, &core.RunStreamResponse{Status: "success"})
	m := pumpFrames(t, frames)

	// 点按滚动条列（x = viewport.Width()，viewport 从屏幕 y=1 开始）
	click := tea.MouseClickMsg{X: m.viewport.Width(), Y: 1 + 1, Button: tea.MouseLeft}
	model, _ := m.Update(click)
	m2 := model.(*Model)
	if !m2.scrollbarDrag {
		t.Fatal("点按滚动条列应进入拖拽模式")
	}
	before := m2.viewStart

	// 拖到滚动条底部 → viewStart 应增大
	drag := tea.MouseMotionMsg{X: m.viewport.Width(), Y: 1 + m.viewport.Height() - 1, Button: tea.MouseLeft}
	model, _ = m2.Update(drag)
	m3 := model.(*Model)
	if m3.viewStart <= before {
		t.Fatalf("拖到滚动条底部 viewStart 应增大: before=%d after=%d", before, m3.viewStart)
	}

	// 松开结束拖拽
	release := tea.MouseReleaseMsg{X: m.viewport.Width(), Y: 1 + m.viewport.Height() - 1, Button: tea.MouseLeft}
	model, _ = m3.Update(release)
	m4 := model.(*Model)
	if m4.scrollbarDrag {
		t.Fatal("松开左键应结束滚动条拖拽")
	}
}

// TestScrollbarClickBodyNotDrag 点按正文列（滚动条左侧）不进入拖拽，而是开启选区。
func TestScrollbarClickBodyNotDrag(t *testing.T) {
	var frames []*core.RunStreamResponse
	frames = append(frames, &core.RunStreamResponse{Status: "streaming", Output: strings.Repeat("填充内容行\n", 30)})
	frames = append(frames, &core.RunStreamResponse{Status: "success"})
	m := pumpFrames(t, frames)

	click := tea.MouseClickMsg{X: 0, Y: 1 + 1, Button: tea.MouseLeft}
	model, _ := m.Update(click)
	m2 := model.(*Model)
	if m2.scrollbarDrag {
		t.Fatal("点按正文不应进入滚动条拖拽")
	}
	if !m2.sel.active {
		t.Fatal("点按正文应开启选区")
	}
}

// TestInScrollbar 坐标判断：滚动条列位于 x == viewport.Width()，且 y 在正文区内。
func TestInScrollbar(t *testing.T) {
	m := pumpFrames(t, []*core.RunStreamResponse{
		{Status: "streaming", Output: strings.Repeat("填充内容行\n", 30)},
		{Status: "success"},
	})
	if !m.inScrollbar(m.viewport.Width(), 1) {
		t.Fatal("滚动条列 x == viewport.Width() 应判定为滚动条")
	}
	if m.inScrollbar(m.viewport.Width()-1, 1) {
		t.Fatal("正文列不应判定为滚动条")
	}
	if m.inScrollbar(m.viewport.Width(), 0) {
		t.Fatal("标题行不应判定为滚动条")
	}
}

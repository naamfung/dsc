package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"dsc/plugin"
)

func TestRenderTodoPanel(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	m.todoArgs = `{"todos":[{"content":"任务一","status":"completed"},{"content":"任务二","status":"in_progress"},{"content":"任务三","status":"pending"}]}`

	panel := m.renderTodoPanel()
	strip := ansi.Strip(panel)
	for _, want := range []string{"待办 1/3", "✔ 任务一", "▶ 任务二", "○ 任务三"} {
		if !strings.Contains(strip, want) {
			t.Fatalf("面板应含 %q: %q", want, strip)
		}
	}

	// 全部完成 → 面板清除
	m.todoArgs = `{"todos":[{"content":"a","status":"completed"},{"content":"b","status":"completed"}]}`
	if got := m.renderTodoPanel(); got != "" {
		t.Fatalf("全部完成后面板应为空: %q", got)
	}
	// 无列表 → 空
	m.todoArgs = ""
	if got := m.renderTodoPanel(); got != "" {
		t.Fatalf("无列表面板应为空: %q", got)
	}
	// 非法 JSON → 空
	m.todoArgs = "not json"
	if got := m.renderTodoPanel(); got != "" {
		t.Fatalf("非法 JSON 面板应为空: %q", got)
	}
}

func TestTodoPanelWindow(t *testing.T) {
	items := func(n int) []todoItem {
		out := make([]todoItem, n)
		for i := range out {
			out[i] = todoItem{Content: "t", Status: "pending"}
		}
		return out
	}
	// 少于 8 项全显
	start, end := todoPanelWindow(items(5))
	if start != 0 || end != 5 {
		t.Fatalf("5 项窗口 = (%d,%d), want (0,5)", start, end)
	}
	// 12 项无活跃项 → 显示前 8
	start, end = todoPanelWindow(items(12))
	if start != 0 || end != 8 {
		t.Fatalf("无活跃 12 项窗口 = (%d,%d), want (0,8)", start, end)
	}
	// 12 项活跃在中间 → 以活跃项为中心
	twelve := items(12)
	twelve[7].Status = "in_progress"
	start, end = todoPanelWindow(twelve)
	if end-start != 8 {
		t.Fatalf("窗口大小 = %d, want 8", end-start)
	}
	if start < 0 || end > 12 {
		t.Fatalf("窗口越界: (%d,%d)", start, end)
	}
}

func TestTodoPanelRows(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	if got := m.todoPanelRows(); got != 0 {
		t.Fatalf("无列表行数 = %d, want 0", got)
	}
	// 4 项：头部 1 + 4 项 = 5 行
	m.todoArgs = `{"todos":[{"content":"a","status":"pending"},{"content":"b","status":"in_progress"},{"content":"c","status":"pending"},{"content":"d","status":"pending"}]}`
	if got := m.todoPanelRows(); got != 5 {
		t.Fatalf("4 项行数 = %d, want 5", got)
	}
}

// TestTodoPanelFrameUpdate todo_write 成功结果帧更新面板；调用帧/失败帧/其他工具不更新。
func TestTodoPanelFrameUpdate(t *testing.T) {
	frames := []*plugin.RunStreamResponse{
		{Status: "tool", ToolName: "todo_write", ToolArgs: `{"todos":[{"content":"x","status":"in_progress"}]}`, ToolResult: "Updated todo list: 1 in progress"},
		{Status: "success"},
	}
	m := pumpFrames(t, frames)
	if m.todoArgs == "" {
		t.Fatal("todo_write 成功结果帧应更新 todoArgs")
	}

	// 调用帧（ToolResult 空）不更新——工具尚未确认执行
	m.todoArgs = ""
	framesCall := []*plugin.RunStreamResponse{
		{Status: "tool", ToolName: "todo_write", ToolArgs: `{"todos":[{"content":"planned","status":"in_progress"}]}`},
		{Status: "success"},
	}
	m2 := pumpFrames(t, framesCall)
	if m2.todoArgs != "" {
		t.Fatalf("todo_write 调用帧不应更新 todoArgs: %q", m2.todoArgs)
	}

	// 失败帧（Error 非空）不更新
	m.todoArgs = `{"todos":[{"content":"keep","status":"in_progress"}]}`
	frames2 := []*plugin.RunStreamResponse{
		{Status: "tool", ToolName: "todo_write", ToolArgs: `{"todos":[{"content":"rejected","status":"pending"}]}`, ToolResult: "rejected", Error: "validation failed"},
		{Status: "success"},
	}
	m3 := pumpFrames(t, frames2)
	if m3.todoArgs != "" {
		t.Fatalf("todo_write 失败帧不应更新 todoArgs: %q", m3.todoArgs)
	}
}

// TestTodoPanelViewLayout 面板进入 View（输入框上方）且布局行数预算一致。
func TestTodoPanelViewLayout(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.Focus()

	baseVP := m.viewport.Height()
	m.todoArgs = `{"todos":[{"content":"任务一","status":"in_progress"},{"content":"任务二","status":"pending"}]}`
	rows := m.todoPanelRows()
	if rows != 3 {
		t.Fatalf("2 项面板行数 = %d, want 3（头部+2 项）", rows)
	}
	// 模拟帧处理后布局同步：viewport 高度应扣除面板行数
	m.syncInputHeight()
	if m.viewport.Height() != baseVP-rows {
		t.Fatalf("viewport 高度 %d, want %d（扣除 %d 行面板）", m.viewport.Height(), baseVP-rows, rows)
	}
	// View 含待办面板，且位置在输入框上方（composer 边框行之前）
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "待办 0/2") {
		t.Fatalf("View 应含待办面板: %q", content)
	}
	// 光标锚定应计入面板行数：输入内容行 Y = 标题(1) + viewport 高 + 面板行 + 顶边框(1)
	cur := m.inputCursorAbs()
	if cur == nil {
		t.Fatal("inputCursorAbs 不应为 nil")
	}
	wantY := 1 + m.viewport.Height() + rows + 1
	if cur.Y != wantY {
		t.Fatalf("光标 Y = %d, want %d", cur.Y, wantY)
	}
}

// TestTodoProjectionFrame 待办投影帧（Status "todo"）：新一轮 turn/start 清空面板，
// 携带 ToolArgs 时填充（对齐 DSH FoldTodos：无需 /todo 手动清理，也不信任模型）。
func TestTodoProjectionFrame(t *testing.T) {
	// 清空：上一轮残留的面板在新一轮启动时被投影帧清除
	frames := []*plugin.RunStreamResponse{
		{Status: "todo"},
		{Status: "success"},
	}
	m := pumpFrames(t, frames)
	if m.todoArgs != "" {
		t.Fatalf("todo 投影帧（空）应清空面板: %q", m.todoArgs)
	}

	// 携带列表时填充（预留：恢复会话展示场景）
	frames2 := []*plugin.RunStreamResponse{
		{Status: "todo", ToolArgs: `{"todos":[{"content":"恢复的任务","status":"in_progress"}]}`},
		{Status: "success"},
	}
	m2 := pumpFrames(t, frames2)
	if m2.todoArgs == "" || !strings.Contains(m2.todoArgs, "恢复的任务") {
		t.Fatalf("todo 投影帧（带列表）应填充面板: %q", m2.todoArgs)
	}
}

// TestTodoPanelRealisticFlow 真实帧序：每轮启动先发 todo 投影帧（清空），
// 模型随后 todo_write 结果帧填充，success 结束；面板不残留也不需要 /todo。
func TestTodoPanelRealisticFlow(t *testing.T) {
	frames := []*plugin.RunStreamResponse{
		{Status: "todo"},
		{Status: "tool", ToolName: "todo_write", ToolArgs: `{"todos":[{"content":"任务","status":"in_progress"}]}`, ToolResult: "Updated todo list: 1 in progress"},
		{Status: "success"},
	}
	m := pumpFrames(t, frames)
	if m.todoArgs == "" || !strings.Contains(m.todoArgs, "任务") {
		t.Fatalf("真实帧序后应填充面板: %q", m.todoArgs)
	}
}

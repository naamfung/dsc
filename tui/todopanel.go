package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// 输入框上方的待办进度面板（对齐 REX 的 renderTodoPanel；数据结构为 DSH todo
// 语义：pending / in_progress / completed 整表）。数据源：宿主托管 todo_write
// 工具的成功结果帧（ToolArgs 整表 JSON），见 streamFrame 处理；
// 全部完成时面板自动清除（REX 同款：工作完成即让出空间）。
const todoPanelMaxRows = 8

var (
	todoHeaderSty = lipgloss.NewStyle().Foreground(accent).Bold(true)
	todoDoneSty   = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
	todoActiveSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")).Bold(true)
)

// todoItem DSH todo 语义：content + status。
type todoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// renderTodoPanel 渲染待办进度面板；无列表或全部完成时返回 ""（面板让出空间）。
func (m *Model) renderTodoPanel() string {
	var p struct {
		Todos []todoItem `json:"todos"`
	}
	if m.todoArgs == "" {
		return ""
	}
	if err := json.Unmarshal([]byte(m.todoArgs), &p); err != nil || len(p.Todos) == 0 {
		return ""
	}
	done := 0
	for _, t := range p.Todos {
		if t.Status == "completed" {
			done++
		}
	}
	if done == len(p.Todos) {
		return "" // 全部完成：清除面板
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", todoHeaderSty.Render("待办"), dimSty.Render(fmt.Sprintf("%d/%d", done, len(p.Todos))))
	start, end := todoPanelWindow(p.Todos)
	if start > 0 {
		b.WriteString(dimSty.Render(fmt.Sprintf("  +%d above", start)) + "\n")
	}
	for _, t := range p.Todos[start:end] {
		// 单行裁剪：长内容/换行会破坏行数预算（vpHeight/光标偏移），压成单行
		content := ansi.Truncate(strings.ReplaceAll(t.Content, "\n", " "), max(m.width-6, 10), "…")
		switch t.Status {
		case "completed":
			b.WriteString(dimSty.Render("  ✔ "+content) + "\n")
		case "in_progress":
			b.WriteString(todoActiveSty.Render("  ▶ "+content) + "\n")
		default:
			b.WriteString(dimSty.Render("  ○ "+content) + "\n")
		}
	}
	if end < len(p.Todos) {
		b.WriteString(dimSty.Render(fmt.Sprintf("  +%d more", len(p.Todos)-end)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// todoPanelWindow 以 in_progress 项为中心截取窗口（最多 todoPanelMaxRows 项），
// 窗口之外以 +N above / +N more 提示（REX 同款）。
func todoPanelWindow(todos []todoItem) (int, int) {
	if len(todos) <= todoPanelMaxRows {
		return 0, len(todos)
	}
	active := -1
	for i, t := range todos {
		if t.Status == "in_progress" {
			active = i
			break
		}
	}
	if active < 0 {
		return 0, todoPanelMaxRows
	}
	start := active - todoPanelMaxRows/2
	if start < 0 {
		start = 0
	}
	if maxStart := len(todos) - todoPanelMaxRows; start > maxStart {
		start = maxStart
	}
	return start, start + todoPanelMaxRows
}

// todoPanelRows 返回待办面板占用的行数（布局预算：vpHeight 与光标锚定偏移），
// 无列表或全部完成时为 0。
func (m *Model) todoPanelRows() int {
	if m.todoArgs == "" {
		return 0
	}
	var p struct {
		Todos []todoItem `json:"todos"`
	}
	if err := json.Unmarshal([]byte(m.todoArgs), &p); err != nil || len(p.Todos) == 0 {
		return 0
	}
	done := 0
	for _, t := range p.Todos {
		if t.Status == "completed" {
			done++
		}
	}
	if done == len(p.Todos) {
		return 0
	}
	rows := 1 // 头部
	start, end := todoPanelWindow(p.Todos)
	if start > 0 {
		rows++
	}
	rows += end - start
	if end < len(p.Todos) {
		rows++
	}
	return rows
}

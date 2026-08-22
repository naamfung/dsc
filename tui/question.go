package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dsc/userquestions"
)

// questionBoxSty 问题覆盖层边框样式。
var questionBoxSty = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(accent).
	Padding(0, 1)

// accentSty 高亮（当前选中项）。
var accentSty = lipgloss.NewStyle().Foreground(accent)

// 用户评审通道的 TUI 端：注册为 Manager 的 UserQuestionProvider。
// askProvider 在 Manager（gRPC handler）goroutine 中阻塞等待，把问题经
// bubbletea 事件循环渲染给用户，用户选择后把回答回传。

// questionMsg 把评审请求送进 bubbletea 事件循环；request 为 nil 表示清除当前问题。
type questionMsg struct {
	request *userquestions.Request
	answer  chan *userquestions.Answer
	err     chan error
}

// pendingQuestion 当前待回答的问题。
type pendingQuestion struct {
	request *userquestions.Request
	answer  chan *userquestions.Answer
	err     chan error
	cursor  int // 当前选中项索引（单选）
}

// askProvider 宿主注册的 UI provider：把问题发给 TUI 并阻塞等待回答。
func (m *Model) askProvider(ctx context.Context, req *userquestions.Request) (*userquestions.Answer, error) {
	answer := make(chan *userquestions.Answer, 1)
	errc := make(chan error, 1)
	if m.program == nil {
		return nil, &userquestions.Error{Code: userquestions.ErrNoProvider, Err: fmt.Errorf("tui program not running")}
	}
	m.program.Send(questionMsg{request: req, answer: answer, err: errc})
	defer func() { m.program.Send(questionMsg{}) }() // 无论结果如何，清除 TUI 覆盖层
	select {
	case ans := <-answer:
		return ans, nil
	case e := <-errc:
		return nil, e
	case <-ctx.Done():
		return nil, &userquestions.Error{Code: userquestions.ErrAskAborted, Err: ctx.Err()}
	}
}

// handleQuestionKey 问题覆盖层激活时的按键处理：方向选择 + Enter 确认 + Esc 放弃。
func (m *Model) handleQuestionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	q := m.question
	if q == nil {
		return m, nil
	}
	opts := m.questionOptions(q)
	switch msg.String() {
	case "ctrl+q":
		return m, tea.Quit
	case "esc", "ctrl+c":
		m.question = nil
		if q.err != nil {
			q.err <- &userquestions.Error{Code: userquestions.ErrCanceled, Err: fmt.Errorf("user dismissed the question")}
		}
		m.render()
		return m, nil
	case "up", "k", "ctrl+p":
		if len(opts) > 0 {
			q.cursor = (q.cursor - 1 + len(opts)) % len(opts)
		}
		m.render()
		return m, nil
	case "down", "j", "ctrl+n", "tab":
		if len(opts) > 0 {
			q.cursor = (q.cursor + 1) % len(opts)
		}
		m.render()
		return m, nil
	case "enter":
		m.question = nil
		if len(opts) > 0 && q.cursor < len(opts) && q.answer != nil {
			q.answer <- &userquestions.Answer{Answers: []userquestions.AnswerItem{{
				ID:       q.request.Questions[0].ID,
				Selected: []string{opts[q.cursor].Label},
			}}}
		}
		m.render()
		return m, nil
	}
	return m, nil
}

// questionOptions 返回当前问题的可选项（取第一个问题的选项；多问题 v1 暂缓）。
func (m *Model) questionOptions(q *pendingQuestion) []userquestions.Option {
	if q == nil || len(q.request.Questions) == 0 {
		return nil
	}
	return q.request.Questions[0].Options
}

// questionView 渲染问题覆盖层（无问题时返回空串）。
func (m *Model) questionView() string {
	q := m.question
	if q == nil || len(q.request.Questions) == 0 {
		return ""
	}
	qq := q.request.Questions[0]
	var b strings.Builder
	title := "问题"
	if qq.Header != "" {
		title = qq.Header
	}
	b.WriteString(questionBoxSty.Render(
		assistantMark + " DSC · " + title + "\n\n" +
			qq.Question + "\n\n" +
			m.renderQuestionOptions(q) +
			"\n" + dimSty.Render("↑/↓ 选择 · Enter 确认 · Esc 放弃"),
	))
	return b.String()
}

// renderQuestionOptions 渲染选项列表（当前项高亮）。
func (m *Model) renderQuestionOptions(q *pendingQuestion) string {
	opts := m.questionOptions(q)
	var b strings.Builder
	for i, o := range opts {
		marker := "  "
		if i == q.cursor {
			marker = "❯ "
		}
		line := marker + o.Label
		if i == q.cursor {
			line = accentSty.Render(line)
		}
		b.WriteString(line)
		if o.Description != "" {
			b.WriteString("  " + dimSty.Render(o.Description))
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

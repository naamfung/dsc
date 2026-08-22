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

// pendingQuestion 当前待回答的问题队列（ask_user_question 可一次携带多个问题，
// TUI 逐个呈现；单问题即队列长度为 1，plan-review 评审同样走此通道）。
type pendingQuestion struct {
	request *userquestions.Request
	answer  chan *userquestions.Answer
	err     chan error
	current int                        // 当前问题索引（队列推进）
	answers []userquestions.AnswerItem // 已收集的回答
	cursor  int                        // 当前问题选中项索引（单选高亮）
	multi   map[int]bool               // 多选：已勾选的选项索引
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

// handleQuestionKey 问题覆盖层激活时的按键处理：方向选择、Space 勾选（多选）、
// Enter 确认（队列中回答完当前问题后推进下一个）、Esc 放弃。
func (m *Model) handleQuestionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	q := m.question
	if q == nil || len(q.request.Questions) == 0 {
		return m, nil
	}
	opts := m.questionOptions(q)
	multi := q.request.Questions[q.current].MultiSelect
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
	case "space", "x":
		if multi && len(opts) > 0 {
			if q.multi == nil {
				q.multi = map[int]bool{}
			}
			q.multi[q.cursor] = !q.multi[q.cursor]
		}
		m.render()
		return m, nil
	case "enter":
		if q.answer == nil {
			m.question = nil
			m.render()
			return m, nil
		}
		q.answers = append(q.answers, m.buildAnswerItem(q, multi))
		// 队列还有下一个问题 → 推进并重置当前问题状态
		if q.current+1 < len(q.request.Questions) {
			q.current++
			q.cursor = 0
			q.multi = nil
			m.render()
			return m, nil
		}
		m.question = nil
		q.answer <- &userquestions.Answer{Answers: q.answers}
		m.render()
		return m, nil
	}
	return m, nil
}

// buildAnswerItem 按当前问题与交互模式构造回答：单选取光标项；
// 多选取已勾选项（按选项顺序）；无选项时视为空选择确认（selected 为 []）。
func (m *Model) buildAnswerItem(q *pendingQuestion, multi bool) userquestions.AnswerItem {
	qq := q.request.Questions[q.current]
	item := userquestions.AnswerItem{ID: qq.ID}
	opts := m.questionOptions(q)
	if multi {
		for i, o := range opts {
			if q.multi[i] {
				item.Selected = append(item.Selected, o.Label)
			}
		}
	} else if len(opts) > 0 && q.cursor < len(opts) {
		item.Selected = []string{opts[q.cursor].Label}
	}
	return item
}

// questionOptions 返回当前问题的可选项。
func (m *Model) questionOptions(q *pendingQuestion) []userquestions.Option {
	if q == nil || len(q.request.Questions) == 0 {
		return nil
	}
	return q.request.Questions[q.current].Options
}

// questionView 渲染问题覆盖层（无问题时返回空串）。
func (m *Model) questionView() string {
	q := m.question
	if q == nil || len(q.request.Questions) == 0 {
		return ""
	}
	qq := q.request.Questions[q.current]
	var b strings.Builder
	title := "问题"
	if qq.Header != "" {
		title = qq.Header
	}
	if len(q.request.Questions) > 1 {
		title = fmt.Sprintf("%s (%d/%d)", title, q.current+1, len(q.request.Questions))
	}
	hint := "↑/↓ 选择 · Enter 确认 · Esc 放弃"
	if qq.MultiSelect {
		hint = "↑/↓ 选择 · Space 勾选 · Enter 确认 · Esc 放弃"
	}
	b.WriteString(questionBoxSty.Render(
		assistantMark + " DSC · " + title + "\n\n" +
			qq.Question + "\n\n" +
			m.renderQuestionOptions(q) +
			"\n" + dimSty.Render(hint),
	))
	return b.String()
}

// renderQuestionOptions 渲染当前问题选项列表（当前项高亮；多选显示勾选标记）。
func (m *Model) renderQuestionOptions(q *pendingQuestion) string {
	opts := m.questionOptions(q)
	multi := q.request.Questions[q.current].MultiSelect
	var b strings.Builder
	for i, o := range opts {
		var line string
		if multi {
			mark := "  "
			if q.multi[i] {
				mark = "✓ "
			}
			line = mark + o.Label
		} else {
			marker := "  "
			if i == q.cursor {
				marker = "❯ "
			}
			line = marker + o.Label
		}
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

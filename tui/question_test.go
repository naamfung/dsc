package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"dsc/userquestions"
)

// TestQuestionFlow 校验问题覆盖层的渲染与键盘选择/放弃。
func TestQuestionFlow(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)

	// 送问题进事件循环
	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID:       "plan-review",
		Question: "Approve this plan and leave plan mode?",
		Options:  []userquestions.Option{{Label: "Approve"}, {Label: "Keep planning"}},
	}}}, answer: ansCh, err: errCh})
	if m.question == nil {
		t.Fatal("question should be pending")
	}
	if v := m.questionView(); !strings.Contains(v, "Approve this plan and leave plan mode?") {
		t.Fatalf("questionView should contain the question, got %q", v)
	}

	// 向下选择 → Enter 确认（选中 Keep planning）
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.question != nil {
		t.Fatal("question should be cleared after answer")
	}
	select {
	case ans := <-ansCh:
		if len(ans.Answers) != 1 || ans.Answers[0].Selected[0] != "Keep planning" {
			t.Fatalf("answer = %+v", ans)
		}
	default:
		t.Fatal("answer should be delivered")
	}

	// 放弃（Esc）→ 错误通道收到 CANCELLED
	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID: "q", Question: "x", Options: []userquestions.Option{{Label: "A"}},
	}}}, answer: ansCh, err: errCh})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.question != nil {
		t.Fatal("question should be cleared after dismiss")
	}
	select {
	case e := <-errCh:
		if !strings.Contains(e.Error(), userquestions.ErrCanceled) {
			t.Fatalf("dismiss error = %v", e)
		}
	default:
		t.Fatal("dismiss should deliver an error")
	}
}

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

// TestQuestionMultiSelectAndQueue 校验多问题队列逐个呈现与 multi_select 勾选。
func TestQuestionMultiSelectAndQueue(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)

	// 两个问题：第一个多选（勾 a、c），第二个单选
	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{
		{ID: "multi", Question: "pick any", MultiSelect: true,
			Options: []userquestions.Option{{Label: "a"}, {Label: "b"}, {Label: "c"}}},
		{ID: "single", Question: "which one",
			Options: []userquestions.Option{{Label: "x"}, {Label: "y"}}},
	}}, answer: ansCh, err: errCh})
	if m.question == nil {
		t.Fatal("first question should be pending")
	}
	if v := m.questionView(); !strings.Contains(v, "pick any") || !strings.Contains(v, "(1/2)") {
		t.Fatalf("first question view = %q", v)
	}

	// 勾选 a（光标默认第 0 项）→ 移到 c 并勾选 → Enter 推进到第二问
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.question == nil {
		t.Fatal("second question should be pending after advancing")
	}
	if v := m.questionView(); !strings.Contains(v, "which one") || !strings.Contains(v, "(2/2)") {
		t.Fatalf("second question view = %q", v)
	}

	// 第二问：Enter 默认选中第一项 x → 队列结束，回答交付
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.question != nil {
		t.Fatal("question should be cleared after all answered")
	}
	select {
	case ans := <-ansCh:
		if len(ans.Answers) != 2 {
			t.Fatalf("answers = %+v", ans)
		}
		if ans.Answers[0].ID != "multi" || len(ans.Answers[0].Selected) != 2 ||
			ans.Answers[0].Selected[0] != "a" || ans.Answers[0].Selected[1] != "c" {
			t.Fatalf("multi answer = %+v", ans.Answers[0])
		}
		if ans.Answers[1].ID != "single" || len(ans.Answers[1].Selected) != 1 ||
			ans.Answers[1].Selected[0] != "x" {
			t.Fatalf("single answer = %+v", ans.Answers[1])
		}
	default:
		t.Fatal("answer should be delivered")
	}
}

// TestQuestionCustomInput 校验自定义文字输入通道：模型选项不合适时，
// 按 c 复用主输入框输入，Enter 提交后经 AnswerItem.Custom 返回模型。
func TestQuestionCustomInput(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)

	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID:       "q",
		Question: "how to proceed?",
		Options:  []userquestions.Option{{Label: "Option A"}, {Label: "Option B"}},
	}}}, answer: ansCh, err: errCh})
	if m.question == nil {
		t.Fatal("question should be pending")
	}

	// 按 c 进入自定义输入模式，主输入框清空并聚焦
	m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.question.customMode {
		t.Fatal("customMode should be active after pressing c")
	}
	if v := m.questionView(); !strings.Contains(v, "自定义回答") {
		t.Fatalf("custom mode view should hint custom input, got %q", v)
	}

	// 在主输入框输入字符（复用 composer）
	for _, r := range []rune("换个方向做") {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := m.input.Value(); got != "换个方向做" {
		t.Fatalf("input value = %q, want custom text", got)
	}

	// Enter 提交 → 答案经 Custom 字段交付，Selected 为空（明确"选项都不合适"）
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.question != nil {
		t.Fatal("question should be cleared after custom submit")
	}
	select {
	case ans := <-ansCh:
		if len(ans.Answers) != 1 {
			t.Fatalf("answers = %+v", ans)
		}
		a := ans.Answers[0]
		if a.ID != "q" || len(a.Selected) != 0 || a.Custom != "换个方向做" {
			t.Fatalf("custom answer = %+v, want selected=[] custom=换个方向做", a)
		}
	default:
		t.Fatal("answer should be delivered")
	}
}

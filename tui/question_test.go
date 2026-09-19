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
	// 注意：选项列表末尾会自动追加"✎ 其他（手动输入）"兜底项，
	// 所以从默认 0（Approve）按一下 ↓ 到 1（Keep planning），再按 ↓ 到 2（兜底项）。
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
	// 注意：末尾追加兜底项，所以 a=0,b=1,c=2,兜底=3
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

// TestQuestionCustomOptionBySelection 校验：选中末尾追加的"✎ 其他（手动输入）"
// 兜底项 + Enter → 进入自定义输入模式（覆盖层消失，焦点落到主输入框）。
// 此为用户记忆中"原始实现"的行为：兜底项作为可见选项存在，而非仅靠 c 键。
func TestQuestionCustomOptionBySelection(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)

	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID:       "q",
		Question: "how to proceed?",
		Options: []userquestions.Option{
			{Label: "Option A"},
			{Label: "Option B"},
			{Label: "Option C"},
		},
	}}}, answer: ansCh, err: errCh})
	if m.question == nil {
		t.Fatal("question should be pending")
	}

	// 验证：兜底项已追加到末尾
	opts := m.questionOptions(m.question)
	if len(opts) != 4 {
		t.Fatalf("expected 4 options (3 model + 1 custom), got %d", len(opts))
	}
	if opts[3].Label != customOptionLabel {
		t.Fatalf("last option should be custom, got %q", opts[3].Label)
	}

	// 渲染应包含兜底项与所有模型选项
	v := m.questionView()
	for _, label := range []string{"Option A", "Option B", "Option C", customOptionLabel} {
		if !strings.Contains(v, label) {
			t.Errorf("questionView missing option: %q", label)
		}
	}

	// 按 ↓ 三次移到兜底项 → Enter 进入自定义输入模式
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.question.cursor != 3 {
		t.Fatalf("cursor should be at custom option index 3, got %d", m.question.cursor)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.question == nil {
		t.Fatal("question should NOT be cleared when entering custom mode (still waiting for input)")
	}
	if !m.question.customMode {
		t.Fatal("customMode should be active after selecting custom option + Enter")
	}

	// 输入自定义文本
	for _, r := range []rune("my own approach") {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	// Enter 提交 → 答案经 Custom 字段交付
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
		if a.ID != "q" || len(a.Selected) != 0 || a.Custom != "my own approach" {
			t.Fatalf("custom answer = %+v, want selected=[] custom=my own approach", a)
		}
	default:
		t.Fatal("answer should be delivered")
	}
}

// TestQuestionLeftRightArrowNav 校验左/右方向键同样可导航选项（不会因未处理
// 而被误判为 Esc 等导致覆盖层消失）。
func TestQuestionLeftRightArrowNav(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)

	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID:       "q",
		Question: "pick",
		Options: []userquestions.Option{
			{Label: "A"}, {Label: "B"}, {Label: "C"},
		},
	}}}, answer: ansCh, err: errCh})
	if m.question == nil {
		t.Fatal("question should be pending")
	}

	// 初始 cursor=0（A）。按 → 移到 1（B），再按 → 移到 2（C），再按 → 移到 3（兜底），
	// 再按 → 应 wrap 回 0（A）。覆盖层在整个过程中不应消失。
	for _, want := range []int{1, 2, 3, 0} {
		m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if m.question == nil {
			t.Fatalf("question disappeared after pressing Right (want cursor=%d)", want)
		}
		if m.question.cursor != want {
			t.Fatalf("after Right: cursor = %d, want %d", m.question.cursor, want)
		}
	}

	// 按 ← 应 wrap 回 3（兜底），再按 ← 到 2（C）
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.question == nil || m.question.cursor != 3 {
		t.Fatalf("after Left: question=%v cursor=%d, want cursor=3", m.question != nil, func() int {
			if m.question != nil {
				return m.question.cursor
			}
			return -1
		}())
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.question == nil || m.question.cursor != 2 {
		t.Fatal("after Left: cursor should be 2")
	}

	// 连续按 ← 3 次：2 → 1 → 0 → 3（wrap）。覆盖层不应消失。
	for _, want := range []int{1, 0, 3} {
		m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		if m.question == nil {
			t.Fatalf("question disappeared after Left (want cursor=%d)", want)
		}
		if m.question.cursor != want {
			t.Fatalf("after Left: cursor = %d, want %d", m.question.cursor, want)
		}
	}
}

// TestQuestionOverlayShrinksViewport 回归：问题覆盖层占独立行（View 组装位于
// viewport 之后），viewport 高度必须等量收缩，否则覆盖层被推到终端可视区之外
// （AltScreen 截断）——Windows 实测表现为 ask_user_question 提问后界面无任何
// 可见变化。此前 inputCursorAbs 计入该行数而 vpHeight 漏计，二者不对称。
func TestQuestionOverlayShrinksViewport(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	m.high = 60
	m.viewport.SetHeight(m.vpHeight())
	before := m.vpHeight()

	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)
	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID: "q", Question: "Approve this plan?",
		Options: []userquestions.Option{{Label: "Approve"}, {Label: "Keep planning"}},
	}}}, answer: ansCh, err: errCh})

	q := m.questionView()
	if q == "" {
		t.Fatal("question overlay should render")
	}
	rows := strings.Count(q, "\n") + 1
	if got := m.vpHeight(); got != before-rows {
		t.Fatalf("vpHeight with overlay = %d, want %d (before %d - overlay rows %d)", got, before-rows, before, rows)
	}
	// 覆盖层清除后恢复原高度
	m.Update(questionMsg{})
	if got := m.vpHeight(); got != before {
		t.Fatalf("vpHeight after clear = %d, want %d", got, before)
	}
}

// TestQuestionCustomModeOverlayHides 校验进入 customMode 后覆盖层主体消失
// （questionView 返回的是一行轻量提示而非边框盒子），满足用户对"原始实现
// 行为"的描述——选中末项后提问层消失、焦点落到主输入框。
func TestQuestionCustomModeOverlayHides(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)

	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID:       "q",
		Question: "how?",
		Options:  []userquestions.Option{{Label: "A"}, {Label: "B"}},
	}}}, answer: ansCh, err: errCh})

	// 渲染前应包含边框盒子
	before := m.questionView()
	if !strings.Contains(before, "╭") {
		t.Fatalf("overlay should render with border before customMode, got %q", before)
	}

	// 按 c 进入自定义模式
	m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.question.customMode {
		t.Fatal("customMode should be active")
	}
	after := m.questionView()
	if strings.Contains(after, "╭") {
		t.Fatalf("overlay border should disappear in customMode, got %q", after)
	}
	if !strings.Contains(after, "自定义回答") {
		t.Fatalf("customMode should show a one-line hint, got %q", after)
	}
}

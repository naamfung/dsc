package plugin

import (
	"context"
	"strings"
	"testing"

	"dsc/userquestions"
)

func TestUserQuestionsProvider(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})

	// 未注册 provider → NO_PROVIDER
	if _, err := m.Ask(context.Background(), &userquestions.Request{
		Questions: []userquestions.Question{{ID: "q", Question: "x", Options: []userquestions.Option{{Label: "Yes"}}}},
	}); err == nil || !strings.Contains(err.Error(), userquestions.ErrNoProvider) {
		t.Fatalf("no provider should fail with NO_PROVIDER, got %v", err)
	}

	// 注册 provider
	got := make(chan string, 1)
	err := m.RegisterUserQuestionProvider(func(ctx context.Context, req *userquestions.Request) (*userquestions.Answer, error) {
		if len(req.Questions) != 1 || req.Questions[0].ID != "plan-review" {
			t.Fatalf("unexpected request: %+v", req)
		}
		got <- req.Questions[0].Intent.Approve
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "plan-review", Selected: []string{"Approve"}}}}, nil
	})
	if err != nil {
		t.Fatalf("register provider: %v", err)
	}
	// 重复注册 → DUPLICATE_PROVIDER
	if err := m.RegisterUserQuestionProvider(func(ctx context.Context, r *userquestions.Request) (*userquestions.Answer, error) { return nil, nil }); err == nil {
		t.Fatal("duplicate provider should fail")
	}

	ans, err := m.Ask(context.Background(), &userquestions.Request{Questions: []userquestions.Question{{
		ID: "plan-review", Question: "Approve?", Options: []userquestions.Option{{Label: "Approve"}, {Label: "Keep planning"}},
		Intent: &userquestions.Intent{Kind: "plan-review", Approve: "Approve"},
	}}})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if len(ans.Answers) != 1 || ans.Answers[0].Selected[0] != "Approve" {
		t.Fatalf("answer = %+v", ans)
	}
	if <-got != "Approve" {
		t.Fatal("provider should see the approve label")
	}

	// 非法请求 → EMPTY_QUESTIONS
	if _, err := m.Ask(context.Background(), &userquestions.Request{}); err == nil ||
		!strings.Contains(err.Error(), userquestions.ErrEmptyQuestions) {
		t.Fatalf("empty questions should fail, got %v", err)
	}
}

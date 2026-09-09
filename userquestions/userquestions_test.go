package userquestions

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	if err := Validate(&Request{Questions: []Question{{
		ID: "q", Question: "Approve?", Options: []Option{{Label: "Yes"}, {Label: "No"}},
	}}}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	// 空问题列表
	if err := Validate(&Request{}); err == nil || !strings.Contains(err.Error(), ErrEmptyQuestions) {
		t.Fatalf("empty questions should fail, got %v", err)
	}
	// 缺 id/文本
	if err := Validate(&Request{Questions: []Question{{ID: "", Question: "x"}}}); err == nil {
		t.Fatal("missing id should fail")
	}
	// plan-review 缺 approve
	if err := Validate(&Request{Questions: []Question{{
		ID: "q", Question: "x", Options: []Option{{Label: "Yes"}}, Intent: &Intent{Kind: "plan-review"},
	}}}); err == nil {
		t.Fatal("plan-review without approve should fail")
	}
	// approve 未命名存在的选项
	if err := Validate(&Request{Questions: []Question{{
		ID: "q", Question: "x", Options: []Option{{Label: "Yes"}},
		Intent: &Intent{Kind: "plan-review", Approve: "Nope"},
	}}}); err == nil {
		t.Fatal("plan-review approve not naming an option should fail")
	}
	// 合法 plan-review
	if err := Validate(&Request{Questions: []Question{{
		ID: "q", Question: "x", Options: []Option{{Label: "Approve"}, {Label: "Keep planning"}},
		Intent: &Intent{Kind: "plan-review", Approve: "Approve"},
	}}}); err != nil {
		t.Fatalf("valid plan-review rejected: %v", err)
	}
}

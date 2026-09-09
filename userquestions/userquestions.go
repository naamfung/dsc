// Package userquestions 提供向用户提问并等待回答的评审通道（对齐 DSH user-questions）。
//
// 宿主注册一个 UI provider（TUI）；agent 侧（如 exit_plan_mode）经 gRPC 调用
// Ask 阻塞等待用户在 TUI 的选择/输入。错误码对齐 DSH UserQuestionError。
package userquestions

import "fmt"

// Option 一个可选项。
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Intent 调用方声明的呈现意图：只影响 UI 呈现，不改变回答编码。
type Intent struct {
	// Kind 意图类型："plan-review"（计划评审：detail 为计划 markdown，Approve 批准）。
	Kind string `json:"kind"`
	// Approve 批准计划的选项标签；其余选项视为否决（plan-review 必填且须命名存在的选项）。
	Approve string `json:"approve,omitempty"`
}

// Question 一个问题。
type Question struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Detail      string   `json:"detail,omitempty"`
	Header      string   `json:"header,omitempty"`
	Options     []Option `json:"options,omitempty"`
	MultiSelect bool     `json:"multi_select,omitempty"`
	Intent      *Intent  `json:"intent,omitempty"`
}

// AnswerItem 一个问题的回答。
type AnswerItem struct {
	ID       string   `json:"id"`
	Selected []string `json:"selected"` // 选中的选项标签
	Custom   string   `json:"custom,omitempty"`
}

// Answer 用户回答。
type Answer struct {
	Answers []AnswerItem `json:"answers"`
}

// Request 一次提问请求。
type Request struct {
	Questions []Question `json:"questions"`
}

// 错误码（对齐 DSH UserQuestionError codes）。
const (
	ErrEmptyQuestions = "EMPTY_QUESTIONS"
	ErrNoProvider     = "NO_PROVIDER"
	ErrInvalidIntent  = "INVALID_INTENT"
	ErrAskAborted     = "ASK_ABORTED"
	ErrCanceled       = "CANCELLED"
)

// Error 评审通道错误。
type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code
}

// Validate 校验请求：至少一个问题；plan-review 意图的 approve 必须命名该问题的某个选项。
func Validate(r *Request) error {
	if len(r.Questions) == 0 {
		return &Error{Code: ErrEmptyQuestions, Err: fmt.Errorf("at least one question is required")}
	}
	for _, q := range r.Questions {
		if q.ID == "" || q.Question == "" {
			return &Error{Code: ErrInvalidIntent, Err: fmt.Errorf("question requires non-empty id and text")}
		}
		if q.Intent != nil && q.Intent.Kind == "plan-review" {
			if q.Intent.Approve == "" {
				return &Error{Code: ErrInvalidIntent, Err: fmt.Errorf("plan-review intent requires an approve option label")}
			}
			found := false
			for _, o := range q.Options {
				if o.Label == q.Intent.Approve {
					found = true
					break
				}
			}
			if !found {
				return &Error{Code: ErrInvalidIntent,
					Err: fmt.Errorf("plan-review approve %q does not name any option of question %q", q.Intent.Approve, q.ID)}
			}
		}
	}
	return nil
}

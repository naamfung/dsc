package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"dsc/proto"
	"dsc/session"
)

// 宿主工具：对齐 DSH plan-mode + goal/tool-goal + tool-ask-user 设计。
//
// 这些工具由 react-loop 直接托管（模型可见、执行时拦截）：plan/goal 状态读写
// 当前会话的事件溯源日志（plan/mode 与 goal/change），ask_user_question 经宿主
// 挂载的 UserQuestionsService 向用户提问。均无需经过工具插件进程——
// 与 DSH 中 plan-mode / goal 服务 / ctx.userQuestions 注册等价。

const (
	toolExitPlanMode    = "exit_plan_mode"
	toolGetGoal         = "get_goal"
	toolCreateGoal      = "create_goal"
	toolUpdateGoal      = "update_goal"
	toolAskUserQuestion = "ask_user_question"
	toolTodoWrite       = "todo_write"
)

// plan 模式激活时注入 system prompt 的部署方引导文案（可由 DSC_PLAN_SECTION 覆盖）。
const defaultPlanSection = "You are in plan mode. Explore and design before presenting the complete plan through exit_plan_mode."

// goal 策略提示词（DSH tool-goal 固定指引；%d 为 blockedAfterConsecutiveRounds）。
const goalPolicyPrompt = "Use goal tools for one long-running completion objective in the current session. " +
	"create_goal may infer goal intent from a direct human request in any language; do not create a goal for routine single-turn work. " +
	"Call get_goal before update_goal and copy its exact goal_id and revision. " +
	"After session resume or fork, an active goal is disarmed: when a human asks to continue or resume in any wording or language, use update_goal action resume to rearm it. " +
	"Mark complete only when the objective is actually achieved. " +
	"Mark blocked only after the same blocking condition persists for at least %d consecutive goal rounds, " +
	"and report that concrete condition in blocked_reason; difficulty, uncertainty, or useful remaining work is not blocked."

// isLocalTool 判断工具名是否为宿主托管的工具（执行时拦截，不走工具插件）。
func isLocalTool(name string) bool {
	switch name {
	case toolExitPlanMode, toolGetGoal, toolCreateGoal, toolUpdateGoal, toolAskUserQuestion, toolTodoWrite:
		return true
	}
	return false
}

// hostTools 返回宿主托管的工具定义，追加进模型可见的工具目录
// （对齐 DSH：exit_plan_mode 始终注册，plan 模式转换只改变提示词段，不改变工具目录）。
func (a *ReactLoopAgent) hostTools() []*proto.Tool {
	return []*proto.Tool{
		{
			Name:           toolExitPlanMode,
			Description:    "Use only in plan mode. Present your COMPLETE plan as markdown, starting with a # heading that names it, then leave plan mode to carry it out.",
			ParametersJson: `{"type":"object","properties":{"plan":{"type":"string","description":"The complete plan, as markdown, starting with a # heading that names it."}},"required":["plan"],"additionalProperties":false}`,
		},
		{
			Name:           toolGetGoal,
			Description:    "Return the current goal or null, including its id, revision, durable phase, admitted/limit goal rounds, any blocker reason, and the current process-local continuation state.",
			ParametersJson: `{"type":"object","properties":{},"additionalProperties":false}`,
		},
		{
			Name:           toolCreateGoal,
			Description:    "Create a goal for one long-running completion objective in the current session, inferred from a direct human request in any language; do not create a goal for routine single-turn work.",
			ParametersJson: `{"type":"object","properties":{"objective":{"type":"string","description":"The long-running completion objective."},"max_goal_rounds":{"type":"integer","description":"Optional cap on total admitted goal rounds; defaults to the deployment value."}},"required":["objective"],"additionalProperties":false}`,
		},
		{
			Name:           toolUpdateGoal,
			Description:    "Update the current goal: edit its objective or max_goal_rounds, pause, resume, complete, or mark blocked (blocked requires blocked_reason). Copy goal_id and revision exactly from get_goal.",
			ParametersJson: `{"type":"object","properties":{"goal_id":{"type":"string"},"revision":{"type":"integer"},"action":{"type":"string","enum":["edit","pause","resume","complete","blocked"]},"objective":{"type":"string"},"max_goal_rounds":{"type":"integer"},"blocked_reason":{"type":"string"}},"required":["goal_id","revision","action"],"additionalProperties":false}`,
		},
		{
			Name: toolAskUserQuestion,
			Description: "Ask the user a concise question when you need confirmation, a choice, or missing information to continue. " +
				"Pass a non-empty questions array; each question needs a stable id and text, and may carry a short header, options (label with optional description), and multi_select. " +
				"To recommend an option, put it first and append \"(Recommended)\" to its label.",
			ParametersJson: `{"type":"object","properties":{"questions":{"type":"array","minItems":1,"items":{"type":"object","properties":{"id":{"type":"string"},"question":{"type":"string"},"header":{"type":"string"},"options":{"type":"array","items":{"type":"object","properties":{"label":{"type":"string"},"description":{"type":"string"}},"required":["label"]}},"multi_select":{"type":"boolean"}},"required":["id","question"]}}},"required":["questions"],"additionalProperties":false}`,
		},
		{
			Name: toolTodoWrite,
			Description: "Replace the agent's full todo list for the current session (no partial updates, no read-back). " +
				"Send the complete list on every call; each todo needs non-empty content and a status of pending, in_progress, or completed. " +
				parallelInProgressNote(a.todoAllowParallel) + " The list is cleared when a new turn starts.",
			ParametersJson: `{"type":"object","properties":{"todos":{"type":"array","items":{"type":"object","properties":{"content":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]}},"required":["content","status"]}}},"required":["todos"],"additionalProperties":false}`,
		},
	}
}

// parallelInProgressNote 依部署配置生成 in_progress 并发纪律说明（对齐 DSH 的
// allowParallelInProgress 开关：开关同时改变面向模型的指令与接受的输入）。
func parallelInProgressNote(allow bool) string {
	if allow {
		return "Multiple tasks may be in_progress when work runs in parallel."
	}
	return "At most one task may be in_progress at a time."
}

// executeLocalTool 执行宿主托管的工具。第二个返回值表示是否需要在
// 本步骤后结束物理轮次（对齐 DSH concludeTurn：complete/blocked 后停止）。
func (a *ReactLoopAgent) executeLocalTool(ctx context.Context, tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	switch tc.Name {
	case toolExitPlanMode:
		return a.execExitPlanMode(ctx, tc)
	case toolGetGoal:
		return a.execGetGoal(tc)
	case toolCreateGoal:
		return a.execCreateGoal(tc)
	case toolUpdateGoal:
		return a.execUpdateGoal(tc)
	case toolAskUserQuestion:
		return a.execAskUserQuestion(ctx, tc)
	case toolTodoWrite:
		return a.execTodoWrite(tc)
	}
	return &proto.ExecuteToolResponse{Error: "unknown local tool " + tc.Name}, false
}

// 评审问题常量（对齐 DSH plan-mode 的 plan-review）。
const (
	reviewID            = "plan-review"
	approveLabel        = "Approve"
	keepPlanningLabel   = "Keep planning"
	keepPlanningMessage = "The user chose to keep planning; revise the plan and present it again."
)

// execExitPlanMode 呈现完整计划并经用户评审后退出 plan 模式（对齐 DSH exit_plan_mode）：
// 经 UserQuestionsService 向用户展示计划，用户批准（Approve）才退出并开始执行；
// 选择 Keep planning（可附反馈）则留在 plan 模式；用户放弃评审则停下等待其消息。
// 无评审通道（headless/-input）时报错，模型据此引导用户切换会话模式。
func (a *ReactLoopAgent) execExitPlanMode(ctx context.Context, tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	var args struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("exit_plan_mode: %v", err)}, false
	}
	a.sessMu.Lock()
	inPlanMode := session.FoldPlanMode(a.sess.Events())
	a.sessMu.Unlock()
	if !inPlanMode {
		return &proto.ExecuteToolResponse{Error: "exit_plan_mode is only available in plan mode"}, false
	}
	if !regexp.MustCompile(`^#\s+\S`).MatchString(strings.TrimSpace(args.Plan)) {
		return &proto.ExecuteToolResponse{Error: "exit_plan_mode requires a non-empty markdown plan starting with a # heading"}, false
	}

	client := a.ensureUserQuestionsClient()
	if client == nil {
		return &proto.ExecuteToolResponse{
			Error: "no user-questions channel is available to review the plan; ask the user to switch the session mode instead",
		}, false
	}
	resp, err := client.Ask(ctx, &proto.AskRequest{Questions: []*proto.AskQuestion{{
		Id:       reviewID,
		Header:   "Plan review",
		Question: "Approve this plan and leave plan mode?",
		Detail:   args.Plan,
		Options: []*proto.AskOption{
			{Label: approveLabel, Description: "Leave plan mode; the plan is carried out from the next step."},
			{Label: keepPlanningLabel, Description: "Stay in plan mode; feedback goes back to the model."},
		},
		Intent: &proto.AskIntent{Kind: "plan-review", Approve: approveLabel},
	}}})
	if err != nil {
		return &proto.ExecuteToolResponse{Error: "exit_plan_mode review failed: " + err.Error()}, false
	}
	if resp.GetError() != "" {
		return &proto.ExecuteToolResponse{Error: mapAskError(resp.GetError(), resp.GetMessage())}, false
	}

	approved, feedback := parseReview(resp)
	if !approved {
		if feedback != "" {
			return &proto.ExecuteToolResponse{Error: keepPlanningMessage + " Their feedback: " + feedback}, false
		}
		return &proto.ExecuteToolResponse{Error: keepPlanningMessage}, false
	}

	// 批准：退出 plan 模式（log-only 事件落盘），后续步骤由模型开始执行计划
	a.sessMu.Lock()
	a.sess.Append(session.PlanMode, &session.PlanModeData{Active: false}, nil)
	err = a.store.Save(a.sess)
	a.planActive = false
	a.sysPromptNeedsUpdate = true
	a.sessMu.Unlock()
	if err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("exit plan mode: %v", err)}, false
	}
	return &proto.ExecuteToolResponse{Content: `{"approved":true}`}, false
}

// parseReview 解析评审回答：唯一选中 Approve 且无自定义文本视为批准；
// 否则视为继续规划（附带自定义反馈）。
func parseReview(resp *proto.AskResponse) (approved bool, feedback string) {
	for _, ans := range resp.GetAnswers() {
		if ans.GetId() != reviewID {
			continue
		}
		if len(ans.GetSelected()) == 1 && ans.GetSelected()[0] == approveLabel && ans.GetCustom() == "" {
			return true, ""
		}
		return false, ans.GetCustom()
	}
	return false, ""
}

// mapAskError 把宿主评审通道错误码转成对模型可行动的提示（对齐 DSH 的提问失败措辞，
// exit_plan_mode 评审与 ask_user_question 共用）。
func mapAskError(code, message string) string {
	switch code {
	case "ASK_ABORTED", "CANCELLED":
		return "The user dismissed the question to speak instead; stop here and wait for their message."
	case "EMPTY_QUESTIONS":
		return "ask_user_question requires a non-empty questions array"
	case "NO_PROVIDER":
		return "no user-questions channel is available; ask the user to switch the session mode instead"
	default:
		if message != "" {
			return message
		}
		return "ask_user_question failed with code " + code
	}
}

// execAskUserQuestion 向用户提出简明问题并等待回答（对齐 DSH tool-ask-user）：
// 参数 questions[] 原样经 UserQuestionsService 交给宿主 UI，回答以规范紧凑 JSON
// {answers:[{id,selected,custom?}]} 返回（custom 为空时省略；selected 可含零至多个标签，
// 单选时取唯一选中项，多选时含全部勾选项）。
func (a *ReactLoopAgent) execAskUserQuestion(ctx context.Context, tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	var args struct {
		Questions []*proto.AskQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("ask_user_question: %v", err)}, false
	}
	if len(args.Questions) == 0 {
		return &proto.ExecuteToolResponse{Error: "ask_user_question requires a non-empty questions array"}, false
	}
	client := a.ensureUserQuestionsClient()
	if client == nil {
		return &proto.ExecuteToolResponse{
			Error: "no user-questions channel is available; ask the user to switch the session mode instead",
		}, false
	}
	resp, err := client.Ask(ctx, &proto.AskRequest{Questions: args.Questions})
	if err != nil {
		return &proto.ExecuteToolResponse{Error: "ask_user_question failed: " + err.Error()}, false
	}
	if resp.GetError() != "" {
		return &proto.ExecuteToolResponse{Error: mapAskError(resp.GetError(), resp.GetMessage())}, false
	}
	// 规范紧凑 JSON：selected 恒存在（零选择为 []），custom 为空时省略（对齐 DSH）
	type answerView struct {
		ID       string   `json:"id"`
		Selected []string `json:"selected"`
		Custom   string   `json:"custom,omitempty"`
	}
	var views []answerView
	for _, ans := range resp.GetAnswers() {
		sel := ans.GetSelected()
		if sel == nil {
			sel = []string{}
		}
		views = append(views, answerView{ID: ans.GetId(), Selected: sel, Custom: ans.GetCustom()})
	}
	b, err := json.Marshal(map[string]any{"answers": views})
	if err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("ask_user_question: encode answers: %v", err)}, false
	}
	return &proto.ExecuteToolResponse{Content: string(b)}, false
}

// execTodoWrite 整表替换当前会话的任务清单（对齐 DSH tool-todo）：
// 每次调用发送完整列表（无部分更新、无回读）；校验 content 非空、无重复、
// 无额外键、status 三态，并按部署配置（DSC_TODO_ALLOW_PARALLEL）约束同时
// in_progress 的数量。写 todo/write 事件（log-only）并落盘，返回统计确认。
func (a *ReactLoopAgent) execTodoWrite(tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	var args struct {
		Todos []map[string]json.RawMessage `json:"todos"`
	}
	if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("todo_write: %v", err)}, false
	}
	todos, err := validateTodos(args.Todos, a.todoAllowParallel)
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, false
	}
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if a.sess == nil {
		return &proto.ExecuteToolResponse{Error: "todo_write: session not loaded"}, false
	}
	a.sess.Append(session.TodoWrite, &session.TodoWriteData{Todos: todos}, nil)
	if err := a.store.Save(a.sess); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("todo_write: persist: %v", err)}, false
	}
	pending, inProgress, completed := 0, 0, 0
	for _, t := range todos {
		switch t.Status {
		case session.TodoPending:
			pending++
		case session.TodoInProgress:
			inProgress++
		case session.TodoCompleted:
			completed++
		}
	}
	return &proto.ExecuteToolResponse{
		Content: fmt.Sprintf("Updated todo list: %d pending, %d in progress, %d completed.", pending, inProgress, completed),
	}, false
}

// validateTodos 校验并构造任务清单（对齐 DSH 稳定失败文本）：
// content 非空且不重复、status 三态、拒绝 content/status 之外的键；
// allowParallel 为 false 时最多一个 in_progress。
func validateTodos(items []map[string]json.RawMessage, allowParallel bool) ([]session.TodoItem, error) {
	todos := make([]session.TodoItem, 0, len(items))
	seen := map[string]bool{}
	inProgress := 0
	for _, item := range items {
		var content, status string
		for k, v := range item {
			switch k {
			case "content":
				if err := json.Unmarshal(v, &content); err != nil {
					return nil, fmt.Errorf("invalid todo: `content` must be a non-empty string")
				}
			case "status":
				if err := json.Unmarshal(v, &status); err != nil {
					return nil, fmt.Errorf("invalid todo: `status` must be one of pending, in_progress, completed")
				}
			default:
				return nil, fmt.Errorf("invalid todos: unexpected key %q", k)
			}
		}
		if strings.TrimSpace(content) == "" {
			return nil, fmt.Errorf("invalid todo: `content` must be a non-empty string")
		}
		if seen[content] {
			return nil, fmt.Errorf("invalid todos: duplicate content %q", content)
		}
		seen[content] = true
		switch status {
		case session.TodoPending, session.TodoInProgress, session.TodoCompleted:
		default:
			return nil, fmt.Errorf("invalid todo: `status` must be one of pending, in_progress, completed")
		}
		if status == session.TodoInProgress {
			inProgress++
		}
		todos = append(todos, session.TodoItem{Content: content, Status: status})
	}
	if !allowParallel && inProgress > 1 {
		return nil, fmt.Errorf("invalid todos: at most one task may be in_progress (got %d)", inProgress)
	}
	return todos, nil
}

// execGetGoal 返回当前目标视图或 null（对齐 DSH get_goal）。
func (a *ReactLoopAgent) execGetGoal(tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	g := session.FoldGoal(a.sess.Events())
	if g == nil {
		return &proto.ExecuteToolResponse{Content: `{"goal":null}`}, false
	}
	content, err := goalViewJSON(g, a.goalActivation, a.goalRounds)
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, false
	}
	return &proto.ExecuteToolResponse{Content: content}, false
}

// execCreateGoal 为长程完成目标创建 goal（对齐 DSH create_goal）。
// v1：react-loop 的轮次均由 TUI 用户直接发起，故不再区分人类/非人类来源；
// subagent 走宿主侧聚合 LLM，不经 react-loop，天然不触发此门控。
func (a *ReactLoopAgent) execCreateGoal(tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	var args struct {
		Objective     string `json:"objective"`
		MaxGoalRounds int    `json:"max_goal_rounds"`
	}
	if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("create_goal: %v", err)}, false
	}
	maxRounds := args.MaxGoalRounds
	if maxRounds <= 0 {
		maxRounds = a.defaultMaxGoalRounds // 物化部署默认值（对齐 DSH）
	}
	return a.applyGoalOp(session.GoalOp{Action: "create", Objective: args.Objective, MaxGoalRounds: maxRounds})
}

// execUpdateGoal 变更当前目标（对齐 DSH update_goal：edit/pause/resume/complete/blocked）。
func (a *ReactLoopAgent) execUpdateGoal(tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	var args struct {
		GoalID        string `json:"goal_id"`
		Revision      int    `json:"revision"`
		Action        string `json:"action"`
		Objective     string `json:"objective"`
		MaxGoalRounds int    `json:"max_goal_rounds"`
		BlockedReason string `json:"blocked_reason"`
	}
	if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("update_goal: %v", err)}, false
	}
	if args.GoalID != session.GoalID {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("unknown goal_id %q (current goal is %q)", args.GoalID, session.GoalID)}, false
	}
	return a.applyGoalOp(session.GoalOp{
		Action: args.Action, Revision: args.Revision,
		Objective: args.Objective, MaxGoalRounds: args.MaxGoalRounds,
		BlockedReason: args.BlockedReason,
	})
}

// applyGoalOp 在会话日志上应用一次目标变更并落盘，返回目标视图 JSON。
// complete/blocked 后返回 conclude=true，由调用方结束物理轮次（对齐 DSH concludeTurn）。
func (a *ReactLoopAgent) applyGoalOp(op session.GoalOp) (*proto.ExecuteToolResponse, bool) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if a.sess == nil {
		return &proto.ExecuteToolResponse{Error: "goal: session not loaded"}, false
	}
	cur := session.FoldGoal(a.sess.Events())
	// resume 预算与「active 且已 armed」判定需要进程本地值
	op.RoundsStarted = a.goalRounds
	op.Activation = a.goalActivation
	next, armed, err := session.ApplyGoalOp(cur, op)
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, false
	}
	if op.Action == "clear" {
		a.sess.Append(session.GoalChange, &session.GoalChangeData{Deleted: true}, nil)
	} else {
		a.sess.Append(session.GoalChange, &session.GoalChangeData{Snapshot: next}, nil)
	}
	if err := a.store.Save(a.sess); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("goal: persist: %v", err)}, false
	}
	a.goalActivation = armed
	conclude := op.Action == "complete" || op.Action == "blocked"
	content, err := goalViewJSON(next, armed, a.goalRounds)
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, false
	}
	return &proto.ExecuteToolResponse{Content: content}, conclude
}

// goalViewJSON 渲染 goal 工具的规范结果 JSON（对齐 DSH 紧凑 JSON 形状）：
// {goal:{id,revision,objective,phase,roundsStarted,maxGoalRounds,blockedReason?}, activation}
func goalViewJSON(g *session.GoalSnapshot, activation bool, rounds int) (string, error) {
	goal := map[string]any{
		"id":            g.ID,
		"revision":      g.Revision,
		"objective":     g.Objective,
		"phase":         g.Phase,
		"roundsStarted": rounds,
		"maxGoalRounds": g.MaxGoalRounds,
	}
	if g.BlockedReason != nil {
		goal["blockedReason"] = map[string]string{"code": g.BlockedReason.Code, "message": g.BlockedReason.Message}
	}
	act := "disarmed"
	if activation {
		act = "armed"
	}
	b, err := json.Marshal(map[string]any{"goal": goal, "activation": act})
	return string(b), err
}

// goalRoundDriver 同会话 goal 续行驱动器判定（对齐 DSH goal-round-driver）：
// phase 为 active、已启用续行（armed）且 Round 预算未耗尽时准入下一轮。
// 人类消息不消耗预算；pause/complete/blocked/clear 停用续行后天然阻止。
func goalRoundDriver(goal *session.GoalSnapshot, activation bool, rounds int) bool {
	if goal == nil {
		return false
	}
	return goal.Phase == session.GoalPhaseActive && activation && rounds < goal.MaxGoalRounds
}

// goalRoundPrompt 渲染下一轮 goal-round 用户消息（对齐 DSH goal-round-driver
// 的提示词：JSON 引用的目标 + 正数轮次编号 + 续行指引）。
func goalRoundPrompt(goal *session.GoalSnapshot, round int) string {
	objective, _ := json.Marshal(goal.Objective)
	return "<goal_round>\n" +
		"Objective: " + string(objective) + "\n" +
		fmt.Sprintf("Round: %d/%d\n\n", round, goal.MaxGoalRounds) +
		"Continue working toward the objective in this same session. Treat the current workspace, " +
		"tool results, and durable session state as authoritative; inspect them instead of assuming " +
		"earlier narration is still current. Make concrete progress and verify the result. Before " +
		"claiming completion, gather evidence that the whole objective is achieved, read the current " +
		"goal, and mark it complete. If work remains, leave the goal active for the next round. Follow " +
		"the configured goal-tool policy before reporting a blocker.\n" +
		"</goal_round>"
}

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

// plan/goal 宿主工具：对齐 DSH plan-mode + goal/tool-goal 设计。
//
// 这些工具由 react-loop 直接托管（模型可见、执行时拦截），状态读写当前会话的
// 事件溯源日志（plan/mode 与 goal/change），无需经过工具插件进程——
// 与 DSH 中 plan-mode / goal 服务注册 ctx.tools 等价。

const (
	toolExitPlanMode = "exit_plan_mode"
	toolGetGoal      = "get_goal"
	toolCreateGoal   = "create_goal"
	toolUpdateGoal   = "update_goal"
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

// isLocalTool 判断工具名是否为宿主托管的 plan/goal 工具（执行时拦截，不走工具插件）。
func isLocalTool(name string) bool {
	switch name {
	case toolExitPlanMode, toolGetGoal, toolCreateGoal, toolUpdateGoal:
		return true
	}
	return false
}

// planGoalTools 返回宿主托管的 plan/goal 工具定义，追加进模型可见的工具目录
// （对齐 DSH：exit_plan_mode 始终注册，plan 模式转换只改变提示词段，不改变工具目录）。
func planGoalTools() []*proto.Tool {
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
	}
}

// executeLocalTool 执行宿主托管的 plan/goal 工具。第二个返回值表示是否需要在
// 本步骤后结束物理轮次（对齐 DSH concludeTurn：complete/blocked 后停止）。
func (a *ReactLoopAgent) executeLocalTool(ctx context.Context, tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	switch tc.Name {
	case toolExitPlanMode:
		return a.execExitPlanMode(tc)
	case toolGetGoal:
		return a.execGetGoal(tc)
	case toolCreateGoal:
		return a.execCreateGoal(tc)
	case toolUpdateGoal:
		return a.execUpdateGoal(tc)
	}
	return &proto.ExecuteToolResponse{Error: "unknown local tool " + tc.Name}, false
}

// execExitPlanMode 呈现完整计划并退出 plan 模式（对齐 DSH exit_plan_mode）。
// v1 简化：计划已作为助手消息流式呈现给用户，无独立审批通道——执行即视为批准退出；
// 用户可随时打断，沙箱/批准限制由各自的策略层独立强制执行。
func (a *ReactLoopAgent) execExitPlanMode(tc *proto.ToolCall) (*proto.ExecuteToolResponse, bool) {
	var args struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("exit_plan_mode: %v", err)}, false
	}
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if !session.FoldPlanMode(a.sess.Events()) {
		return &proto.ExecuteToolResponse{Error: "exit_plan_mode is only available in plan mode"}, false
	}
	if !regexp.MustCompile(`^#\s+\S`).MatchString(strings.TrimSpace(args.Plan)) {
		return &proto.ExecuteToolResponse{Error: "exit_plan_mode requires a non-empty markdown plan starting with a # heading"}, false
	}
	a.sess.Append(session.PlanMode, &session.PlanModeData{Active: false}, nil)
	if err := a.store.Save(a.sess); err != nil {
		return &proto.ExecuteToolResponse{Error: fmt.Sprintf("exit plan mode: %v", err)}, false
	}
	a.planActive = false
	a.sysPromptNeedsUpdate = true
	// 对齐 DSH 规范返回值；后续步骤由模型开始执行计划
	return &proto.ExecuteToolResponse{Content: `{"approved":true}`}, false
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

package main

import (
	"context"
	"strings"
	"testing"

	"dsc/proto"
	"dsc/session"
)

// newTestAgent 创建带临时 store 与已加载 default 会话的测试 agent。
func newTestAgent(t *testing.T) *ReactLoopAgent {
	t.Helper()
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	sess, err := store.Ensure("default")
	if err != nil {
		t.Fatalf("ensure session: %v", err)
	}
	return &ReactLoopAgent{
		store:                         store,
		sess:                          sess,
		planSection:                   defaultPlanSection,
		defaultMaxGoalRounds:          256,
		blockedAfterConsecutiveRounds: 3,
	}
}

func callTool(t *testing.T, a *ReactLoopAgent, name, args string) (*proto.ExecuteToolResponse, bool) {
	t.Helper()
	return a.executeLocalTool(context.Background(), &proto.ToolCall{Id: "t1", Name: name, ArgumentsJson: args})
}

func TestGoalLifecycle(t *testing.T) {
	a := newTestAgent(t)

	// 无目标时 get_goal 返回 null
	resp, _ := callTool(t, a, toolGetGoal, "{}")
	if resp.Error != "" || !strings.Contains(resp.Content, `"goal":null`) {
		t.Fatalf("get_goal on empty = %+v", resp)
	}

	// create_goal：省略 max_goal_rounds 时物化部署默认值
	resp, concluded := callTool(t, a, toolCreateGoal, `{"objective":"build the dsc plan feature"}`)
	if resp.Error != "" || concluded {
		t.Fatalf("create_goal = %+v (concluded=%v)", resp, concluded)
	}
	if !strings.Contains(resp.Content, `"phase":"active"`) || !strings.Contains(resp.Content, `"activation":"armed"`) {
		t.Fatalf("create_goal result = %s", resp.Content)
	}
	// 事件已落盘，可折叠
	g := session.FoldGoal(a.sess.Events())
	if g == nil || g.Revision != 1 || g.Phase != session.GoalPhaseActive || g.MaxGoalRounds != 256 {
		t.Fatalf("folded goal = %+v", g)
	}

	// 重复创建被拒绝
	resp, _ = callTool(t, a, toolCreateGoal, `{"objective":"duplicate"}`)
	if resp.Error == "" {
		t.Fatal("duplicate create_goal should fail")
	}

	// get_goal 返回当前视图
	resp, _ = callTool(t, a, toolGetGoal, "{}")
	if resp.Error != "" || !strings.Contains(resp.Content, `"objective":"build the dsc plan feature"`) {
		t.Fatalf("get_goal = %s", resp.Content)
	}

	// edit（CAS revision 1）
	resp, _ = callTool(t, a, toolUpdateGoal, `{"goal_id":"goal","revision":1,"action":"edit","objective":"build plan and goal"}`)
	if resp.Error != "" || !strings.Contains(resp.Content, `"objective":"build plan and goal"`) {
		t.Fatalf("edit = %s (err=%v)", resp.Content, resp.Error)
	}

	// 过期 revision 被拒绝
	resp, _ = callTool(t, a, toolUpdateGoal, `{"goal_id":"goal","revision":1,"action":"pause"}`)
	if resp.Error == "" {
		t.Fatal("stale revision should fail")
	}

	// blocked 需要 blocked_reason
	resp, _ = callTool(t, a, toolUpdateGoal, `{"goal_id":"goal","revision":2,"action":"blocked"}`)
	if resp.Error == "" {
		t.Fatal("blocked without reason should fail")
	}

	// blocked → conclude 轮次
	resp, concluded = callTool(t, a, toolUpdateGoal, `{"goal_id":"goal","revision":2,"action":"blocked","blocked_reason":"api quota exhausted"}`)
	if resp.Error != "" || !concluded {
		t.Fatalf("blocked = %+v (concluded=%v)", resp, concluded)
	}
	if !strings.Contains(resp.Content, `"phase":"blocked"`) || !strings.Contains(resp.Content, `"code":"model-reported"`) {
		t.Fatalf("blocked result = %s", resp.Content)
	}

	// resume → active + armed（清除 blocker reason）
	resp, concluded = callTool(t, a, toolUpdateGoal, `{"goal_id":"goal","revision":3,"action":"resume"}`)
	if resp.Error != "" || concluded {
		t.Fatalf("resume = %+v (concluded=%v)", resp, concluded)
	}
	if !strings.Contains(resp.Content, `"phase":"active"`) || !strings.Contains(resp.Content, `"activation":"armed"`) {
		t.Fatalf("resume result = %s", resp.Content)
	}

	// active 且 armed 时 resume 是冗余操作
	resp, _ = callTool(t, a, toolUpdateGoal, `{"goal_id":"goal","revision":4,"action":"resume"}`)
	if resp.Error == "" {
		t.Fatal("resume on active+armed should fail")
	}

	// complete → conclude 轮次
	resp, concluded = callTool(t, a, toolUpdateGoal, `{"goal_id":"goal","revision":4,"action":"complete"}`)
	if resp.Error != "" || !concluded || !strings.Contains(resp.Content, `"phase":"complete"`) {
		t.Fatalf("complete = %+v (concluded=%v)", resp, concluded)
	}

	// 已完成目标可被新目标替换（对齐 DSH）
	resp, _ = callTool(t, a, toolCreateGoal, `{"objective":"next objective","max_goal_rounds":128}`)
	if resp.Error != "" || !strings.Contains(resp.Content, `"revision":1`) || !strings.Contains(resp.Content, `"maxGoalRounds":128`) {
		t.Fatalf("create after complete = %s (err=%v)", resp.Content, resp.Error)
	}
}

func TestExitPlanMode(t *testing.T) {
	a := newTestAgent(t)

	// plan 模式外调用被拒绝
	resp, _ := callTool(t, a, toolExitPlanMode, `{"plan":"# My Plan\n\nstep 1"}`)
	if resp.Error == "" {
		t.Fatal("exit_plan_mode outside plan mode should fail")
	}

	// 进入 plan 模式（RPC 语义：追加 plan/mode 事件并落盘）
	if err := a.SetPlanMode(context.Background(), true); err != nil {
		t.Fatalf("set plan mode: %v", err)
	}
	if !session.FoldPlanMode(a.sess.Events()) {
		t.Fatal("plan mode should be active after SetPlanMode")
	}

	// 计划必须以 # 标题开头
	resp, _ = callTool(t, a, toolExitPlanMode, `{"plan":"no heading"}`)
	if resp.Error == "" {
		t.Fatal("plan without # heading should fail")
	}

	// 合法计划 → approved，退出 plan 模式
	resp, concluded := callTool(t, a, toolExitPlanMode, `{"plan":"# My Plan\n\n1. explore\n2. implement"}`)
	if resp.Error != "" || concluded || resp.Content != `{"approved":true}` {
		t.Fatalf("exit_plan_mode = %+v (concluded=%v)", resp, concluded)
	}
	if session.FoldPlanMode(a.sess.Events()) {
		t.Fatal("plan mode should be inactive after exit_plan_mode")
	}
}

func TestLocalToolRouting(t *testing.T) {
	for _, name := range []string{toolGetGoal, toolUpdateGoal, toolCreateGoal, toolExitPlanMode} {
		if !isLocalTool(name) {
			t.Fatalf("%s should route locally", name)
		}
	}
	for _, name := range []string{"shell", "read_file"} {
		if isLocalTool(name) {
			t.Fatalf("%s should route remotely", name)
		}
	}
}

// TestGoalRoundDriver 校验续行驱动判定与 goal_round 提示词。
func TestGoalRoundDriver(t *testing.T) {
	active := &session.GoalSnapshot{ID: session.GoalID, Revision: 1, Objective: "build x", Phase: session.GoalPhaseActive, MaxGoalRounds: 3}

	// active + armed + 未耗尽 → 续行
	if !goalRoundDriver(active, true, 0) {
		t.Fatal("active+armed with budget should continue")
	}
	// 预算耗尽 → 停止
	if goalRoundDriver(active, true, 3) {
		t.Fatal("exhausted budget should stop")
	}
	// 未 armed（disarmed）→ 停止
	if goalRoundDriver(active, false, 0) {
		t.Fatal("disarmed goal should stop")
	}
	// 非 active phase → 停止
	if goalRoundDriver(&session.GoalSnapshot{ID: session.GoalID, Revision: 1, Objective: "x", Phase: session.GoalPhasePaused, MaxGoalRounds: 3}, true, 0) {
		t.Fatal("paused goal should stop")
	}
	// 无目标 → 停止
	if goalRoundDriver(nil, true, 0) {
		t.Fatal("no goal should stop")
	}

	// 提示词含 JSON 引用的目标与轮次编号
	p := goalRoundPrompt(active, 2)
	if !strings.Contains(p, "<goal_round>") || !strings.Contains(p, `"build x"`) ||
		!strings.Contains(p, "Round: 2/3") || !strings.Contains(p, "</goal_round>") {
		t.Fatalf("prompt = %q", p)
	}
}

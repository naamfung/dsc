package main

import (
	"context"
	"strings"
	"testing"

	"dsc/proto"
	"dsc/session"
	"google.golang.org/grpc"
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

// fakeUQClient 评审通道假客户端：返回预设回答。
type fakeUQClient struct {
	proto.UserQuestionsServiceClient // 嵌入接口以满足 mustEmbed（Ask 被覆盖）
	resp                             *proto.AskResponse
	err                              error
}

func (f *fakeUQClient) Ask(_ context.Context, _ *proto.AskRequest, _ ...grpc.CallOption) (*proto.AskResponse, error) {
	return f.resp, f.err
}

func TestExitPlanMode(t *testing.T) {
	review := func(resp *proto.AskResponse) *ReactLoopAgent {
		a := newTestAgent(t)
		a.uqServiceID = 1 // 声明有通道，ensureUserQuestionsClient 直接返回预设 uqClient
		a.uqClient = &fakeUQClient{resp: resp}
		if err := a.SetPlanMode(context.Background(), true); err != nil {
			t.Fatalf("set plan mode: %v", err)
		}
		return a
	}
	plan := `{"plan":"# My Plan\n\n1. explore\n2. implement"}`

	// plan 模式外调用被拒绝
	a := newTestAgent(t)
	resp, _ := callTool(t, a, toolExitPlanMode, plan)
	if resp.Error == "" {
		t.Fatal("exit_plan_mode outside plan mode should fail")
	}
	// 无评审通道 → 报错（headless 场景）
	a = newTestAgent(t)
	if err := a.SetPlanMode(context.Background(), true); err != nil {
		t.Fatalf("set plan mode: %v", err)
	}
	resp, _ = callTool(t, a, toolExitPlanMode, plan)
	if resp.Error == "" || !strings.Contains(resp.Error, "no user-questions channel") {
		t.Fatalf("no-channel should fail with guidance, got %q", resp.Error)
	}
	// 计划必须以 # 标题开头
	a = review(&proto.AskResponse{})
	resp, _ = callTool(t, a, toolExitPlanMode, `{"plan":"no heading"}`)
	if resp.Error == "" {
		t.Fatal("plan without # heading should fail")
	}

	// 批准 → approved，退出 plan 模式
	a = review(&proto.AskResponse{Answers: []*proto.AskAnswer{{Id: reviewID, Selected: []string{approveLabel}}}})
	resp, concluded := callTool(t, a, toolExitPlanMode, plan)
	if resp.Error != "" || concluded || resp.Content != `{"approved":true}` {
		t.Fatalf("approve = %+v (concluded=%v)", resp, concluded)
	}
	if session.FoldPlanMode(a.sess.Events()) {
		t.Fatal("plan mode should be inactive after approval")
	}

	// Keep planning → 留在 plan 模式并带反馈
	a = review(&proto.AskResponse{Answers: []*proto.AskAnswer{{Id: reviewID, Selected: []string{keepPlanningLabel}, Custom: "需要更多细节"}}})
	resp, _ = callTool(t, a, toolExitPlanMode, plan)
	if resp.Error == "" || !strings.Contains(resp.Error, keepPlanningMessage) ||
		!strings.Contains(resp.Error, "需要更多细节") {
		t.Fatalf("keep planning = %q", resp.Error)
	}
	if !session.FoldPlanMode(a.sess.Events()) {
		t.Fatal("plan mode should stay active after keep planning")
	}

	// 用户放弃评审（ASK_ABORTED）→ 停下等待消息，plan 模式保持
	a = review(&proto.AskResponse{Error: "ASK_ABORTED", Message: "dismissed"})
	resp, _ = callTool(t, a, toolExitPlanMode, plan)
	if resp.Error == "" || !strings.Contains(resp.Error, "dismissed") {
		t.Fatalf("dismissed = %q", resp.Error)
	}
	if !session.FoldPlanMode(a.sess.Events()) {
		t.Fatal("plan mode should stay active after dismiss")
	}
}

func TestLocalToolRouting(t *testing.T) {
	for _, name := range []string{toolGetGoal, toolUpdateGoal, toolCreateGoal, toolExitPlanMode, toolAskUserQuestion} {
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

func TestAskUserQuestion(t *testing.T) {
	with := func(resp *proto.AskResponse) *ReactLoopAgent {
		a := newTestAgent(t)
		a.uqServiceID = 1 // 声明有通道，ensureUserQuestionsClient 直接返回预设 uqClient
		a.uqClient = &fakeUQClient{resp: resp}
		return a
	}
	args := `{"questions":[{"id":"q1","question":"which one?","options":[{"label":"Option A (Recommended)"},{"label":"Option B"}]},{"id":"q2","question":"pick any","multi_select":true,"options":[{"label":"a"},{"label":"b"}]}]}`

	// 无通道 → 报错引导（headless 场景）
	a := newTestAgent(t)
	resp, _ := callTool(t, a, toolAskUserQuestion, args)
	if resp.Error == "" || !strings.Contains(resp.Error, "no user-questions channel") {
		t.Fatalf("no-channel should fail with guidance, got %q", resp.Error)
	}
	// 空 questions → 报错
	a = with(&proto.AskResponse{})
	resp, _ = callTool(t, a, toolAskUserQuestion, `{"questions":[]}`)
	if resp.Error == "" || !strings.Contains(resp.Error, "non-empty questions") {
		t.Fatalf("empty questions should fail, got %q", resp.Error)
	}
	// 参数非法 JSON → 报错
	a = with(&proto.AskResponse{})
	resp, _ = callTool(t, a, toolAskUserQuestion, `{"questions":`)
	if resp.Error == "" {
		t.Fatal("bad arguments should fail")
	}

	// 成功：规范紧凑 JSON，保留 id/selected/custom，多问题原样透传
	a = with(&proto.AskResponse{Answers: []*proto.AskAnswer{
		{Id: "q1", Selected: []string{"Option A (Recommended)"}},
		{Id: "q2", Selected: []string{"a", "b"}, Custom: "notes"},
	}})
	resp, concluded := callTool(t, a, toolAskUserQuestion, args)
	if resp.Error != "" || concluded {
		t.Fatalf("ask = %+v (concluded=%v)", resp, concluded)
	}
	if !strings.Contains(resp.Content, `"id":"q1"`) || !strings.Contains(resp.Content, "Option A (Recommended)") ||
		!strings.Contains(resp.Content, `"id":"q2"`) || !strings.Contains(resp.Content, `"custom":"notes"`) {
		t.Fatalf("ask content = %s", resp.Content)
	}

	// 零选择：selected 恒存在为 []
	a = with(&proto.AskResponse{Answers: []*proto.AskAnswer{{Id: "q1", Selected: nil}}})
	resp, _ = callTool(t, a, toolAskUserQuestion, args)
	if resp.Error != "" || !strings.Contains(resp.Content, `"selected":[]`) {
		t.Fatalf("zero selection = %s (err=%v)", resp.Content, resp.Error)
	}

	// 用户放弃（CANCELLED）→ 停下等待消息
	a = with(&proto.AskResponse{Error: "CANCELLED", Message: "dismissed"})
	resp, _ = callTool(t, a, toolAskUserQuestion, args)
	if resp.Error == "" || !strings.Contains(resp.Error, "dismissed") {
		t.Fatalf("dismissed = %q", resp.Error)
	}
	// 无 provider（NO_PROVIDER）→ 引导切换会话模式
	a = with(&proto.AskResponse{Error: "NO_PROVIDER"})
	resp, _ = callTool(t, a, toolAskUserQuestion, args)
	if resp.Error == "" || !strings.Contains(resp.Error, "no user-questions channel") {
		t.Fatalf("no-provider = %q", resp.Error)
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

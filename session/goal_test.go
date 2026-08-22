package session

import (
	"strings"
	"testing"
)

// appendGoal 追加一条 goal/change 事件（log-only）。
func appendGoalChange(t *testing.T, s *Session, snap *GoalSnapshot, deleted bool) {
	t.Helper()
	s.Append(GoalChange, &GoalChangeData{Snapshot: snap, Deleted: deleted}, nil)
}

func TestFoldPlanMode(t *testing.T) {
	s := New()
	if FoldPlanMode(s.Events()) != false {
		t.Fatal("empty log should fold to inactive")
	}
	s.Append(PlanMode, &PlanModeData{Active: true}, nil)
	s.Append(PlanMode, &PlanModeData{Active: false}, nil)
	s.Append(PlanMode, &PlanModeData{Active: true}, nil)
	if !FoldPlanMode(s.Events()) {
		t.Fatal("last plan/mode should win (active)")
	}
	s.Append(PlanMode, &PlanModeData{Active: false}, nil)
	if FoldPlanMode(s.Events()) {
		t.Fatal("last plan/mode should win (inactive)")
	}
}

func TestFoldGoal(t *testing.T) {
	s := New()
	if FoldGoal(s.Events()) != nil {
		t.Fatal("empty log should fold to nil goal")
	}
	appendGoalChange(t, s, &GoalSnapshot{ID: GoalID, Revision: 1, Objective: "build x", Phase: GoalPhaseActive, MaxGoalRounds: 256}, false)
	g := FoldGoal(s.Events())
	if g == nil || g.Revision != 1 || g.Phase != GoalPhaseActive || g.Objective != "build x" {
		t.Fatalf("fold after create = %+v", g)
	}
	// clear tombstone → 无当前目标
	appendGoalChange(t, s, nil, true)
	if g := FoldGoal(s.Events()); g != nil {
		t.Fatalf("fold after clear should be nil, got %+v", g)
	}
}

func TestApplyGoalCreate(t *testing.T) {
	cur, armed, err := ApplyGoalOp(nil, GoalOp{Action: "create", Objective: "build x", MaxGoalRounds: 256})
	if err != nil {
		t.Fatalf("create on empty log: %v", err)
	}
	if cur.ID != GoalID || cur.Revision != 1 || cur.Phase != GoalPhaseActive || cur.MaxGoalRounds != 256 || !armed {
		t.Fatalf("created goal = %+v (armed=%v)", cur, armed)
	}
	// 已有非完成目标时重复创建被拒绝
	if _, _, err := ApplyGoalOp(cur, GoalOp{Action: "create", Objective: "y", MaxGoalRounds: 10}); err == nil {
		t.Fatal("duplicate create should fail")
	}
	// 空 objective 被拒绝
	if _, _, err := ApplyGoalOp(nil, GoalOp{Action: "create", Objective: "  ", MaxGoalRounds: 10}); err == nil {
		t.Fatal("empty objective should fail")
	}
	// 非正 MaxGoalRounds 被拒绝
	if _, _, err := ApplyGoalOp(nil, GoalOp{Action: "create", Objective: "x", MaxGoalRounds: 0}); err == nil {
		t.Fatal("non-positive max_goal_rounds should fail")
	}
	// 已完成目标可被替换（对齐 DSH）
	done, _, _ := ApplyGoalOp(cur, GoalOp{Action: "complete", Revision: 1})
	cur, _, err = ApplyGoalOp(done, GoalOp{Action: "create", Objective: "new", MaxGoalRounds: 100})
	if err != nil {
		t.Fatalf("create after complete: %v", err)
	}
	if cur.Revision != 1 || cur.Objective != "new" {
		t.Fatalf("recreated goal = %+v", cur)
	}
}

func TestApplyGoalEdit(t *testing.T) {
	cur, _, _ := ApplyGoalOp(nil, GoalOp{Action: "create", Objective: "build x", MaxGoalRounds: 256})
	// CAS：过期 revision 被拒绝
	if _, _, err := ApplyGoalOp(cur, GoalOp{Action: "edit", Revision: 99, Objective: "z"}); err == nil {
		t.Fatal("stale revision should fail")
	}
	// 只替换 objective（edit 保持 activation）
	next, armed, err := ApplyGoalOp(cur, GoalOp{Action: "edit", Revision: 1, Objective: "build y", Activation: true})
	if err != nil {
		t.Fatalf("edit objective: %v", err)
	}
	if next.Revision != 2 || next.Objective != "build y" || next.MaxGoalRounds != 256 || next.Phase != GoalPhaseActive || !armed {
		t.Fatalf("edited goal = %+v (armed=%v)", next, armed)
	}
	// 只替换 max_goal_rounds
	next, _, err = ApplyGoalOp(next, GoalOp{Action: "edit", Revision: 2, MaxGoalRounds: 512})
	if err != nil {
		t.Fatalf("edit rounds: %v", err)
	}
	if next.Revision != 3 || next.Objective != "build y" || next.MaxGoalRounds != 512 {
		t.Fatalf("edited goal = %+v", next)
	}
	// 全部省略视为无变更，被拒绝
	if _, _, err := ApplyGoalOp(next, GoalOp{Action: "edit", Revision: 3}); err == nil {
		t.Fatal("no-op edit should fail")
	}
}

func TestApplyGoalPhaseTransitions(t *testing.T) {
	cur, _, _ := ApplyGoalOp(nil, GoalOp{Action: "create", Objective: "build x", MaxGoalRounds: 256})

	// 暂停 → disarmed
	paused, armed, err := ApplyGoalOp(cur, GoalOp{Action: "pause", Revision: 1})
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	if paused.Phase != GoalPhasePaused || paused.Revision != 2 || armed {
		t.Fatalf("paused = %+v (armed=%v)", paused, armed)
	}
	// 重复暂停被拒绝
	if _, _, err := ApplyGoalOp(paused, GoalOp{Action: "pause", Revision: 2}); err == nil {
		t.Fatal("pause on paused goal should fail")
	}
	// 阻塞需要 blocked_reason
	if _, _, err := ApplyGoalOp(paused, GoalOp{Action: "blocked", Revision: 2}); err == nil {
		t.Fatal("blocked without reason should fail")
	}
	// 阻塞仅允许 active
	if _, _, err := ApplyGoalOp(paused, GoalOp{Action: "blocked", Revision: 2, BlockedReason: "api 配额耗尽"}); err == nil {
		t.Fatal("blocked on paused goal should fail")
	}
	// active → blocked
	blocked, armed, err := ApplyGoalOp(cur, GoalOp{Action: "blocked", Revision: 1, BlockedReason: "api 配额耗尽"})
	if err != nil {
		t.Fatalf("blocked: %v", err)
	}
	if blocked.Phase != GoalPhaseBlocked || blocked.BlockedReason == nil ||
		blocked.BlockedReason.Code != "model-reported" || blocked.BlockedReason.Message != "api 配额耗尽" || armed {
		t.Fatalf("blocked = %+v (armed=%v)", blocked, armed)
	}
	// resume → active + armed，清除 blocker reason
	resumed, armed, err := ApplyGoalOp(blocked, GoalOp{Action: "resume", Revision: 2})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Phase != GoalPhaseActive || resumed.BlockedReason != nil || resumed.Revision != 3 || !armed {
		t.Fatalf("resumed = %+v (armed=%v)", resumed, armed)
	}
	// active 且已 armed 时 resume 是冗余操作，被拒绝
	if _, _, err := ApplyGoalOp(resumed, GoalOp{Action: "resume", Revision: 3, Activation: true}); err == nil {
		t.Fatal("resume on active+armed goal should fail")
	}
	// Round 预算耗尽时 resume 被拒绝
	exhausted, _, _ := ApplyGoalOp(resumed, GoalOp{Action: "edit", Revision: 3, MaxGoalRounds: 1})
	pausedEx, _, err := ApplyGoalOp(exhausted, GoalOp{Action: "pause", Revision: 4})
	if err != nil {
		t.Fatalf("pause exhausted goal: %v", err)
	}
	if _, _, err := ApplyGoalOp(pausedEx, GoalOp{Action: "resume", Revision: 5, RoundsStarted: 1}); err == nil {
		t.Fatal("resume with exhausted rounds should fail")
	}
	// complete（可从 blocked 完成）→ disarmed
	done, armed, err := ApplyGoalOp(blocked, GoalOp{Action: "complete", Revision: 2})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if done.Phase != GoalPhaseComplete || armed {
		t.Fatalf("completed = %+v (armed=%v)", done, armed)
	}
	// 已完成不可 resume
	if _, _, err := ApplyGoalOp(done, GoalOp{Action: "resume", Revision: 3}); err == nil {
		t.Fatal("resume on complete goal should fail")
	}
	// clear → tombstone（CAS 校验，对齐 DSH expectCurrent）
	if _, _, err := ApplyGoalOp(done, GoalOp{Action: "clear"}); err == nil {
		t.Fatal("clear without matching revision should fail stale check")
	}
	cleared, _, err := ApplyGoalOp(done, GoalOp{Action: "clear", Revision: 3})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.Revision != 4 {
		t.Fatalf("cleared = %+v", cleared)
	}
	// 无目标时 clear 被拒绝
	if _, _, err := ApplyGoalOp(nil, GoalOp{Action: "clear", Revision: 1}); err == nil {
		t.Fatal("clear on empty log should fail")
	}
	// 未知 action
	if _, _, err := ApplyGoalOp(cur, GoalOp{Action: "explode", Revision: 1}); err == nil ||
		!strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("unknown action should fail, got %v", err)
	}
}

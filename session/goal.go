package session

import (
	"fmt"
	"strings"
)

// Goal 生命周期阶段（对齐 DSH goal phase：active/paused/blocked/complete）。
const (
	GoalPhaseActive   = "active"
	GoalPhasePaused   = "paused"
	GoalPhaseBlocked  = "blocked"
	GoalPhaseComplete = "complete"
)

// GoalID 当前目标固定 id（单目标会话，对齐 DSH「只有一个当前目标」）。
const GoalID = "goal"

// GoalBlockReason 阻塞说明：策略自有稳定代码 + 规范化自由文本（对齐 DSH）。
type GoalBlockReason struct {
	Code    string // 固定 "model-reported"（v1 由模型报告，无独立评估器）
	Message string
}

// GoalSnapshot 目标持久快照（goal/change 事件携带完整快照，整值替换）。
// 续行启用状态（activation）与已准入 Round 数绝不持久化：
// 恢复/fork 后一律停用，需显式 resume 重新启用。
type GoalSnapshot struct {
	ID            string
	Revision      int
	Objective     string
	Phase         string
	BlockedReason *GoalBlockReason // 仅 phase=blocked 时非 nil
	MaxGoalRounds int
}

// GoalChangeData goal/change 事件载荷（log-only）。
// clear 使用 tombstone（Deleted=true，Snapshot 为 nil）；Snapshot 非 nil 时携带变更后的完整快照。
type GoalChangeData struct {
	Snapshot      *GoalSnapshot
	Deleted       bool // clear tombstone：目标已清除，快照历史仍在日志中可读
	RoundsStarted int  // 已准入的 Goal Round 数（v1 无驱动器，恒为 0；jobs/workflow 落地后推进）
}

// FoldGoal 从事件日志折叠目标状态：最后一条 goal/change 生效；
// 无记录或最后为 clear tombstone 时返回 nil（无当前目标）。
func FoldGoal(events []*Event) *GoalSnapshot {
	var cur *GoalSnapshot
	for _, ev := range events {
		if ev.Type == GoalChange {
			if d, ok := ev.Data.(*GoalChangeData); ok {
				if d.Deleted {
					cur = nil
				} else if d.Snapshot != nil {
					s := *d.Snapshot
					cur = &s
				}
			}
		}
	}
	return cur
}

// GoalOp 一次目标变更请求（对齐 DSH goal 服务动词）。
type GoalOp struct {
	Action        string // "create" | "edit" | "pause" | "resume" | "complete" | "blocked" | "clear"
	Revision      int    // 期望的当前 revision（CAS；create 忽略）
	Objective     string // create/edit 的目标描述（空 = 省略）
	MaxGoalRounds int    // create/edit 的 Round 上限（<=0 = 省略，create 由调用方物化默认值）
	BlockedReason string // blocked 必填（作为 message 持久化，code 固定 "model-reported"）
	RoundsStarted int    // 当前已准入 Round 数（resume 预算判定用）
	Activation    bool   // 当前进程本地续行启用状态（resume 拒绝 active 且已 armed 的冗余操作）
}

// ApplyGoalOp 在 cur 快照上应用一次目标变更。
// cur 为 nil 表示无当前目标（仅 create 允许）；校验失败返回错误，不产生变更。
// 返回新的持久快照与新的续行启用状态（activation 绝不持久化，由调用方单独持有）。
func ApplyGoalOp(cur *GoalSnapshot, op GoalOp) (*GoalSnapshot, bool, error) {
	fail := func(msg string) (*GoalSnapshot, bool, error) { return nil, false, fmt.Errorf("goal: %s", msg) }
	switch op.Action {
	case "create":
		// 无当前目标或已完成目标可被替换（对齐 DSH）
		if cur != nil && cur.Phase != GoalPhaseComplete {
			return fail(fmt.Sprintf("goal %q already exists with phase %q", cur.ID, cur.Phase))
		}
		if strings.TrimSpace(op.Objective) == "" {
			return fail("create requires a non-empty objective")
		}
		if op.MaxGoalRounds <= 0 {
			return fail("max_goal_rounds must be a positive integer")
		}
		return &GoalSnapshot{
			ID: GoalID, Revision: 1, Objective: op.Objective,
			Phase: GoalPhaseActive, MaxGoalRounds: op.MaxGoalRounds,
		}, true, nil
	}
	if cur == nil {
		return fail("no current goal")
	}
	// CAS：期望 revision 必须与当前一致（对齐 DSH 的比较并设置防护；clear 亦校验）
	if op.Revision != cur.Revision {
		return fail(fmt.Sprintf("stale goal ref %q revision %d; current is %q revision %d",
			cur.ID, op.Revision, cur.ID, cur.Revision))
	}
	next := *cur
	next.Revision++
	next.BlockedReason = nil
	switch op.Action {
	case "clear":
		return &GoalSnapshot{ID: cur.ID, Revision: cur.Revision + 1}, false, nil
	case "edit":
		// 至少一个替换字段；phase 保持不变（对齐 DSH）
		hasChange := false
		if strings.TrimSpace(op.Objective) != "" {
			next.Objective = op.Objective
			hasChange = true
		}
		if op.MaxGoalRounds > 0 {
			next.MaxGoalRounds = op.MaxGoalRounds
			hasChange = true
		}
		if !hasChange {
			return fail("edit requires objective and/or max_goal_rounds")
		}
		return &next, op.Activation, nil
	case "pause":
		if cur.Phase != GoalPhaseActive {
			return fail(fmt.Sprintf("cannot pause goal from phase %q; expected active", cur.Phase))
		}
		next.Phase = GoalPhasePaused
		return &next, false, nil
	case "resume":
		if cur.Phase != GoalPhaseActive && cur.Phase != GoalPhasePaused && cur.Phase != GoalPhaseBlocked {
			return fail(fmt.Sprintf("cannot resume goal from phase %q; expected active, paused or blocked", cur.Phase))
		}
		if cur.Phase == GoalPhaseActive && op.Activation {
			return fail("goal is already active and armed")
		}
		if op.RoundsStarted >= cur.MaxGoalRounds {
			return fail(fmt.Sprintf("goal exhausted %d goal rounds; increase max_goal_rounds before resuming", cur.MaxGoalRounds))
		}
		next.Phase = GoalPhaseActive
		return &next, true, nil
	case "complete":
		if cur.Phase != GoalPhaseActive && cur.Phase != GoalPhasePaused && cur.Phase != GoalPhaseBlocked {
			return fail(fmt.Sprintf("cannot complete goal from phase %q; expected active, paused or blocked", cur.Phase))
		}
		next.Phase = GoalPhaseComplete
		return &next, false, nil
	case "blocked":
		if cur.Phase != GoalPhaseActive {
			return fail(fmt.Sprintf("cannot block goal from phase %q; expected active", cur.Phase))
		}
		if strings.TrimSpace(op.BlockedReason) == "" {
			return fail("blocked requires a non-empty blocked_reason")
		}
		next.Phase = GoalPhaseBlocked
		next.BlockedReason = &GoalBlockReason{Code: "model-reported", Message: op.BlockedReason}
		return &next, false, nil
	}
	return fail(fmt.Sprintf("unknown action %q", op.Action))
}

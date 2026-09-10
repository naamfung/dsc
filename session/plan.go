package session

// PlanModeData plan/mode 事件载荷（log-only）：整值替换，最后一条生效。
// 对齐 DSH plan-mode 的持久状态设计：plan 状态仅存在于日志中，恢复/fork/压缩
// 都能直接从会话日志折叠回 plan 状态。
type PlanModeData struct {
	Active bool
}

// FoldPlanMode 从事件日志（或任意前缀）折叠 plan 模式状态：
// 最后一条 plan/mode 生效，无记录则未激活。
func FoldPlanMode(events []*Event) bool {
	active := false
	for _, ev := range events {
		if ev.Type == PlanMode {
			if d, ok := ev.Data.(*PlanModeData); ok {
				active = d.Active
			}
		}
	}
	return active
}

// FoldHistoryLimit 从事件日志（或任意前缀）折叠历史注入条数上限：
// 最后一条 history/limit 生效；返回该值以及是否存在记录（无记录时返回的值为 -1，
// 调用方应回退部署默认）。
func FoldHistoryLimit(events []*Event) (int, bool) {
	limit := -1
	found := false
	for _, ev := range events {
		if ev.Type == HistoryLimit {
			if d, ok := ev.Data.(*HistoryLimitData); ok {
				limit = d.Count
				found = true
			}
		}
	}
	return limit, found
}

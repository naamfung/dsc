package session

// Todo 任务清单（对齐 DSH tool-todo）：模型每次调用 todo_write 发送完整列表，
// 整表替换、无部分更新。清单属于当前会话，随轮次消逝（turn/start 使上一轮的
// 计划失效），仅作投影/UI 状态，不进入派生历史。

// TodoStatus 任务状态（三态，对齐 DSH）。
const (
	TodoPending    = "pending"
	TodoInProgress = "in_progress"
	TodoCompleted  = "completed"
)

// TodoItem 一个待办项。
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

// TodoWriteData todo/write 事件载荷（log-only：完整列表快照，后写覆盖先写）。
type TodoWriteData struct {
	Todos []TodoItem `json:"todos"`
}

// FoldTodos 折叠当前有效 todo 清单（对齐 DSH 会话投影）：
// 每个 todo/write 取整表，每个 turn/start 清空（当前有效计划）；
// turn/end 保留刚完成的清单。无有效写入时返回 nil。
func FoldTodos(events []*Event) []TodoItem {
	var cur []TodoItem
	for _, ev := range events {
		switch ev.Type {
		case TodoWrite:
			if d, ok := ev.Data.(*TodoWriteData); ok {
				cur = d.Todos
			}
		case TurnStart:
			cur = nil
		}
	}
	return cur
}

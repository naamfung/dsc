package session

import "testing"

func TestFoldTodos(t *testing.T) {
	s := New()

	// 无写入 → nil
	if ts := FoldTodos(s.Events()); ts != nil {
		t.Fatalf("no writes should fold nil, got %+v", ts)
	}

	// 写入清单 → 整表返回
	s.Append(TodoWrite, &TodoWriteData{Todos: []TodoItem{
		{Content: "a", Status: TodoPending},
		{Content: "b", Status: TodoInProgress},
	}}, nil)
	ts := FoldTodos(s.Events())
	if len(ts) != 2 || ts[0].Content != "a" || ts[1].Status != TodoInProgress {
		t.Fatalf("folded todos = %+v", ts)
	}

	// 后写覆盖先写（整表替换）
	s.Append(TodoWrite, &TodoWriteData{Todos: []TodoItem{{Content: "c", Status: TodoCompleted}}}, nil)
	ts = FoldTodos(s.Events())
	if len(ts) != 1 || ts[0].Content != "c" {
		t.Fatalf("later write should replace, got %+v", ts)
	}

	// turn/start 使当前有效计划失效（上一轮清单作废）
	s.Append(TurnStart, &TurnData{Turn: 2}, nil)
	if ts := FoldTodos(s.Events()); ts != nil {
		t.Fatalf("turn/start should clear todos, got %+v", ts)
	}

	// turn/end 保留刚完成的清单
	s.Append(TurnEnd, &TurnData{Turn: 2, Reason: "completed"}, nil)
	s.Append(TodoWrite, &TodoWriteData{Todos: []TodoItem{{Content: "x", Status: TodoPending}}}, nil)
	s.Append(TurnEnd, &TurnData{Turn: 3, Reason: "completed"}, nil)
	ts = FoldTodos(s.Events())
	if len(ts) != 1 || ts[0].Content != "x" {
		t.Fatalf("turn/end should keep todos, got %+v", ts)
	}
}

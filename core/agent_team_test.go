package core

import (
	"testing"
)

func TestTaskBoardAddGet(t *testing.T) {
	tb := NewTaskBoard()
	task := tb.AddTask("实现登录接口", 1)
	if task.ID == "" {
		t.Error("task ID should not be empty")
	}
	if task.Title != "实现登录接口" {
		t.Errorf("title = %q", task.Title)
	}
	if task.Status != TaskStatusTodo {
		t.Errorf("status = %q, want todo", task.Status)
	}

	got, ok := tb.GetTask(task.ID)
	if !ok {
		t.Fatal("GetTask should find the task")
	}
	if got.Title != task.Title {
		t.Errorf("got title = %q, want %q", got.Title, task.Title)
	}
}

func TestTaskBoardClaimComplete(t *testing.T) {
	tb := NewTaskBoard()
	task := tb.AddTask("写测试", 2)

	// 认领
	if err := tb.ClaimTask(task.ID, "agent-a"); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	got, _ := tb.GetTask(task.ID)
	if got.Status != TaskStatusInProgress {
		t.Errorf("status = %q, want in_progress", got.Status)
	}
	if got.Assignee != "agent-a" {
		t.Errorf("assignee = %q, want agent-a", got.Assignee)
	}

	// 完成
	if err := tb.CompleteTask(task.ID); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	got, _ = tb.GetTask(task.ID)
	if got.Status != TaskStatusDone {
		t.Errorf("status = %q, want done", got.Status)
	}
}

func TestTaskBoardClaimNotTodo(t *testing.T) {
	tb := NewTaskBoard()
	task := tb.AddTask("task1", 1)
	tb.ClaimTask(task.ID, "agent-a")

	// 已被认领，不能再次认领
	err := tb.ClaimTask(task.ID, "agent-b")
	if err == nil {
		t.Error("should not be able to claim a non-todo task")
	}
}

func TestTaskBoardListSorted(t *testing.T) {
	tb := NewTaskBoard()
	tb.AddTask("low", 1)
	tb.AddTask("high", 3)
	tb.AddTask("mid", 2)

	tasks := tb.ListTasks()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "high" {
		t.Errorf("first task should be highest priority, got %q", tasks[0].Title)
	}
	if tasks[1].Title != "mid" {
		t.Errorf("second task should be mid priority, got %q", tasks[1].Title)
	}
}

func TestTaskBoardRemove(t *testing.T) {
	tb := NewTaskBoard()
	task := tb.AddTask("to-remove", 1)
	tb.RemoveTask(task.ID)
	_, ok := tb.GetTask(task.ID)
	if ok {
		t.Error("task should be removed")
	}
}

func TestMessageQueueSendReceive(t *testing.T) {
	mq := NewMessageQueue()
	mq.Send("agent-a", "agent-b", "hello from a")
	mq.Send("agent-b", "agent-a", "hi from b")
	mq.Send("agent-a", "", "broadcast message") // 广播

	msgs := mq.Receive("agent-b")
	if len(msgs) != 2 {
		t.Fatalf("agent-b should have 2 messages (direct + broadcast), got %d", len(msgs))
	}
}

func TestMessageQueueListAll(t *testing.T) {
	mq := NewMessageQueue()
	mq.Send("a", "b", "msg1")
	mq.Send("b", "a", "msg2")

	all := mq.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(all))
	}
}

func TestAgentTeamRegister(t *testing.T) {
	team := NewAgentTeam("test-team")
	team.RegisterAgent("agent-a")
	team.RegisterAgent("agent-b")

	agents := team.Agents()
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	team.UnregisterAgent("agent-a")
	agents = team.Agents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent after unregister, got %d", len(agents))
	}
}

func TestAgentTeamSharedTaskBoard(t *testing.T) {
	team := NewAgentTeam("test-team")
	tb := team.TaskBoard()

	task := tb.AddTask("shared task", 1)
	tb.ClaimTask(task.ID, "agent-a")

	got, _ := tb.GetTask(task.ID)
	if got.Assignee != "agent-a" {
		t.Errorf("assignee = %q, want agent-a", got.Assignee)
	}
}

func TestFormatTaskBoard(t *testing.T) {
	tb := NewTaskBoard()
	tb.AddTask("task 1", 1)
	tb.AddTask("task 2", 2)

	formatted := FormatTaskBoard(tb)
	if formatted == "" {
		t.Error("formatted should not be empty")
	}
}

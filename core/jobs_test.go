package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dsc/jobs"
)

// waitJobStatus 轮询等待后台任务落定为指定状态。
func waitJobStatus(t *testing.T, m *Manager, id string, want jobs.JobStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snap, err := m.jobs.Get(id, ""); err == nil && snap.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", id, want)
}

func TestJobToolsRegistered(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	for _, name := range []string{"job_output", "job_list", "job_kill"} {
		tool, ok := m.toolRegistry.Get(name)
		if !ok || tool.Name() != name {
			t.Fatalf("%s should be registered", name)
		}
		if len(tool.ParametersSchema()) == 0 {
			t.Fatalf("%s should have a schema", name)
		}
	}
}

func TestJobTools(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	get := func(name string) *jobTool {
		tool, _ := m.toolRegistry.Get(name)
		return tool.(*jobTool)
	}

	// 空列表
	out, err := get("job_list").Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil || out != "(no background jobs)" {
		t.Fatalf("empty list = %q, %v", out, err)
	}

	// 后台启动一个运行中的任务（无 owner）
	id, _ := m.jobs.Start(jobs.StartSpec{
		Kind:  "workflow",
		Label: "tally",
		Start: func() (jobs.JobHooks, error) {
			done := make(chan jobs.JobOutcome, 1)
			go func() {
				time.Sleep(30 * time.Millisecond)
				done <- jobs.JobOutcome{Status: jobs.StatusCompleted, Output: "done"}
			}()
			return jobs.JobHooks{Done: done}, nil
		},
	})

	// job_list 显示运行中的任务
	out, err = get("job_list").Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil || !strings.Contains(out, id) || !strings.Contains(out, "[workflow] running — tally") {
		t.Fatalf("list = %q, %v", out, err)
	}

	// job_output 非阻塞：运行中 → 无输出 + 存活状态
	out, err = get("job_output").Execute(context.Background(), []byte(`{"job_id":"`+id+`"}`))
	if err != nil || !strings.Contains(out, "(no output yet)") || !strings.Contains(out, "[status: running]") {
		t.Fatalf("output running = %q, %v", out, err)
	}

	// 等待完成 → job_output 返回最终输出
	waitJobStatus(t, m, id, jobs.StatusCompleted)
	out, err = get("job_output").Execute(context.Background(), []byte(`{"job_id":"`+id+`"}`))
	if err != nil || !strings.Contains(out, "done") || !strings.Contains(out, "[status: completed]") {
		t.Fatalf("output done = %q, %v", out, err)
	}

	// 已结束的任务 kill → already finished
	out, err = get("job_kill").Execute(context.Background(), []byte(`{"job_id":"`+id+`"}`))
	if err != nil || !strings.Contains(out, "already finished [status: completed]") {
		t.Fatalf("kill finished = %q, %v", out, err)
	}

	// 未知 job → 错误
	if _, err := get("job_output").Execute(context.Background(), json.RawMessage(`{"job_id":"nope"}`)); err == nil {
		t.Fatal("unknown job should fail")
	}
	if _, err := get("job_kill").Execute(context.Background(), json.RawMessage(`{"job_id":"nope"}`)); err == nil {
		t.Fatal("kill unknown job should fail")
	}

	// 运行中的任务 kill → 请求取消 → 落定 killed
	id2, _ := m.jobs.Start(jobs.StartSpec{
		Kind:  "workflow",
		Label: "hang",
		Start: func() (jobs.JobHooks, error) {
			done := make(chan jobs.JobOutcome, 1)
			cancel := func(reason string) {
				go func() { done <- jobs.JobOutcome{Status: jobs.StatusKilled} }()
			}
			return jobs.JobHooks{Cancel: cancel, Done: done}, nil
		},
	})
	out, err = get("job_kill").Execute(context.Background(), []byte(`{"job_id":"`+id2+`"}`))
	if err != nil || !strings.Contains(out, "requested cancellation of job "+id2) {
		t.Fatalf("kill running = %q, %v", out, err)
	}
	waitJobStatus(t, m, id2, jobs.StatusKilled)
	out, err = get("job_output").Execute(context.Background(), []byte(`{"job_id":"`+id2+`"}`))
	if err != nil || !strings.Contains(out, "[status: killed]") {
		t.Fatalf("output killed = %q, %v", out, err)
	}
}

// TestJobDoneEventBus 验证通用送达：jobs 落定 → 宿主事件总线 JobDoneEvent，
// 任何订阅者（TUI/web/novelforge）都能收到。
func TestJobDoneEventBus(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	got := make(chan jobs.JobSnapshot, 1)
	unsub := m.OnEvent(JobDoneEvent, func(ctx EventContext) (any, error) {
		if s, ok := ctx.Data.(jobs.JobSnapshot); ok {
			got <- s
		}
		return nil, nil
	})
	defer unsub()

	id, _ := m.jobs.Start(jobs.StartSpec{
		Kind: "workflow", Label: "x",
		Start: func() (jobs.JobHooks, error) {
			done := make(chan jobs.JobOutcome, 1)
			go func() { done <- jobs.JobOutcome{Status: jobs.StatusCompleted, Output: "ok"} }()
			return jobs.JobHooks{Done: done}, nil
		},
	})
	select {
	case s := <-got:
		if s.ID != id || s.Status != jobs.StatusCompleted {
			t.Fatalf("event snapshot = %+v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("JobDoneEvent should be delivered")
	}
}

// TestJobToolsOwnerIsolation 验证工具层 owner 隔离：caller 经 ctx 注入，
// 外来会话被拒绝、无 owner 任务开放。
func TestJobToolsOwnerIsolation(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	get := func(name string) *jobTool {
		tool, _ := m.toolRegistry.Get(name)
		return tool.(*jobTool)
	}
	// 有 owner 的任务（owner=sess-a）
	id, _ := m.jobs.Start(jobs.StartSpec{
		Kind: "workflow", Label: "private", Owner: "sess-a",
		Start: func() (jobs.JobHooks, error) {
			done := make(chan jobs.JobOutcome, 1)
			go func() { done <- jobs.JobOutcome{Status: jobs.StatusCompleted, Output: "secret"} }()
			return jobs.JobHooks{Done: done}, nil
		},
	})
	// owner 可读（先等落定，避免立即完成竞态）
	if _, err := m.jobs.Wait(id, 2*time.Second, "sess-a"); err != nil {
		t.Fatalf("owner wait: %v", err)
	}
	out, err := get("job_output").Execute(WithCaller(context.Background(), "sess-a"), []byte(`{"job_id":"`+id+`"}`))
	if err != nil || !strings.Contains(out, "secret") {
		t.Fatalf("owner read = %q, %v", out, err)
	}
	// 外来会话（含无会话）被拒绝
	for _, caller := range []string{"sess-b", ""} {
		ctx := context.Background()
		if caller != "" {
			ctx = WithCaller(ctx, caller)
		}
		if _, err := get("job_output").Execute(ctx, []byte(`{"job_id":"`+id+`"}`)); err == nil {
			t.Fatalf("caller %q should be denied", caller)
		}
		if _, err := get("job_kill").Execute(ctx, []byte(`{"job_id":"`+id+`"}`)); err == nil {
			t.Fatalf("caller %q should be denied kill", caller)
		}
	}
	// job_list：外来会话看不到私有任务
	out, err = get("job_list").Execute(WithCaller(context.Background(), "sess-b"), json.RawMessage(`{}`))
	if err != nil || strings.Contains(out, id) {
		t.Fatalf("foreign list = %q, %v", out, err)
	}
}

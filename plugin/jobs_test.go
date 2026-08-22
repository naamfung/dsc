package plugin

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
		if j, ok := m.jobs.Get(id); ok && j.Status == want {
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

	// 后台启动一个运行中的任务
	job := m.jobs.Start("workflow", "tally", func(ctx context.Context) (string, error) {
		time.Sleep(30 * time.Millisecond)
		return "done", nil
	})

	// job_list 显示运行中的任务
	out, err = get("job_list").Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil || !strings.Contains(out, job.ID) || !strings.Contains(out, "[workflow] running — tally") {
		t.Fatalf("list = %q, %v", out, err)
	}

	// job_output 非阻塞：运行中 → 无输出 + 存活状态
	out, err = get("job_output").Execute(context.Background(), []byte(`{"job_id":"`+job.ID+`"}`))
	if err != nil || !strings.Contains(out, "(no output yet)") || !strings.Contains(out, "[status: running]") {
		t.Fatalf("output running = %q, %v", out, err)
	}

	// 等待完成 → job_output 返回最终输出
	waitJobStatus(t, m, job.ID, jobs.StatusSucceeded)
	out, err = get("job_output").Execute(context.Background(), []byte(`{"job_id":"`+job.ID+`"}`))
	if err != nil || !strings.Contains(out, "done") || !strings.Contains(out, "[status: succeeded]") {
		t.Fatalf("output done = %q, %v", out, err)
	}

	// 已结束的任务 kill → already finished
	out, err = get("job_kill").Execute(context.Background(), []byte(`{"job_id":"`+job.ID+`"}`))
	if err != nil || !strings.Contains(out, "already finished [status: succeeded]") {
		t.Fatalf("kill finished = %q, %v", out, err)
	}

	// 未知 job → 错误
	if _, err := get("job_output").Execute(context.Background(), json.RawMessage(`{"job_id":"nope"}`)); err == nil {
		t.Fatal("unknown job should fail")
	}
	if _, err := get("job_kill").Execute(context.Background(), json.RawMessage(`{"job_id":"nope"}`)); err == nil {
		t.Fatal("kill unknown job should fail")
	}

	// 运行中的任务 kill → 请求取消 → 落定 cancelled
	job2 := m.jobs.Start("workflow", "hang", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	out, err = get("job_kill").Execute(context.Background(), []byte(`{"job_id":"`+job2.ID+`"}`))
	if err != nil || !strings.Contains(out, "requested cancellation of job "+job2.ID) {
		t.Fatalf("kill running = %q, %v", out, err)
	}
	waitJobStatus(t, m, job2.ID, jobs.StatusCancelled)
	out, err = get("job_output").Execute(context.Background(), []byte(`{"job_id":"`+job2.ID+`"}`))
	if err != nil || !strings.Contains(out, "[status: cancelled]") {
		t.Fatalf("output cancelled = %q, %v", out, err)
	}
}

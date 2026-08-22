package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

// waitStatus 轮询等待任务落定为指定状态。
func waitStatus(t *testing.T, r *Registry, id string, want JobStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := r.Get(id); ok && j.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if j, ok := r.Get(id); ok {
		t.Fatalf("job %s = %s (%s), want %s", id, j.Status, j.Output, want)
	}
	t.Fatalf("job %s not found", id)
}

func TestRegistryLifecycle(t *testing.T) {
	r := NewRegistry()

	// 成功：登记即 running，完成后 succeeded + 输出
	j := r.Start("workflow", "tally", func(ctx context.Context) (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "ok", nil
	})
	if j.ID != "workflow-1" || j.Status != StatusRunning {
		t.Fatalf("start = %+v", j)
	}
	waitStatus(t, r, j.ID, StatusSucceeded)
	if j.Output != "ok" || j.Error != "" {
		t.Fatalf("succeeded job = %+v", j)
	}

	// 失败：fn 返回错误 → failed + 错误文本
	j2 := r.Start("workflow", "boom", func(ctx context.Context) (string, error) {
		return "", errors.New("boom")
	})
	waitStatus(t, r, j2.ID, StatusFailed)
	if j2.Error != "boom" {
		t.Fatalf("failed job = %+v", j2)
	}

	// 取消：Kill 经 ctx 传播，fn 感知后落定 cancelled
	j3 := r.Start("workflow", "hang", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	if !r.Kill(j3.ID) {
		t.Fatal("kill running job should succeed")
	}
	waitStatus(t, r, j3.ID, StatusCancelled)

	// 已结束的任务不可再 kill
	if r.Kill(j3.ID) {
		t.Fatal("kill finished job should fail")
	}
	// 不存在的任务
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown job should not be found")
	}
	if r.Kill("nope") {
		t.Fatal("kill unknown job should fail")
	}

	// List：三个任务按启动顺序
	all := r.List()
	if len(all) != 3 || all[0].ID != "workflow-1" || all[2].ID != "workflow-3" {
		t.Fatalf("list = %+v", all)
	}
}

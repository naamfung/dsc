package jobs

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestStore 创建临时目录下的任务存储。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func TestStorePersist(t *testing.T) {
	s := newTestStore(t)
	j := &Job{ID: "job-1", Name: "daily", Cron: "0 8 * * *", Prompt: "写日报", Enabled: true, CreatedAt: 100}
	if err := s.Save(j); err != nil {
		t.Fatalf("save: %v", err)
	}
	// 用同一目录重新打开（模拟重启），任务应从磁盘恢复
	s2, err := NewStore(filepath.Dir(s.path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := s2.Get("job-1")
	if !ok || got.Name != "daily" || got.Cron != "0 8 * * *" || !got.Enabled {
		t.Fatalf("restored job = %+v (ok=%v)", got, ok)
	}
}

func TestStoreRemoveMissing(t *testing.T) {
	s := newTestStore(t)
	if s.Remove("nope") {
		t.Fatal("removing missing job should return false")
	}
}

func TestSchedulerAddInvalidCron(t *testing.T) {
	s := newTestStore(t)
	sch := NewScheduler(s, nil, 0)
	err := sch.Add(&Job{Name: "bad", Cron: "not-a-cron", Prompt: "x", Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "invalid cron") {
		t.Fatalf("invalid cron should fail, got %v", err)
	}
}

func TestSchedulerFireRunsRunner(t *testing.T) {
	s := newTestStore(t)
	var mu sync.Mutex
	ran := make(chan string, 1)
	sch := NewScheduler(s, func(ctx context.Context, j *Job) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		ran <- "ran:" + j.Prompt
		return "output-ok", nil
	}, time.Second)
	if err := sch.Add(&Job{Name: "daily", Cron: "0 8 * * *", Prompt: "do it", Enabled: true}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := sch.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sch.Stop()

	// cron 条目应已注册（5 段表达式无法秒级触发，这里直接验证注册与执行路径）
	sch.mu.Lock()
	registered := len(sch.entries)
	sch.mu.Unlock()
	if registered != 1 {
		t.Fatalf("registered entries = %d, want 1", registered)
	}

	id := s.List()[0].ID
	sch.fire(id)

	select {
	case got := <-ran:
		if got != "ran:do it" {
			t.Fatalf("runner got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fire did not run the runner")
	}

	// 状态落盘异步，轮询等待
	deadline := time.Now().Add(2 * time.Second)
	for {
		list := sch.List()
		if len(list) == 1 && list[0].LastStatus == "success" {
			if list[0].LastOutput != "output-ok" {
				t.Fatalf("job output = %q", list[0].LastOutput)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job state did not settle: %+v", list)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSchedulerSetEnabledPersists(t *testing.T) {
	s := newTestStore(t)
	sch := NewScheduler(s, func(ctx context.Context, j *Job) (string, error) {
		return "ok", nil
	}, time.Second)
	if err := sch.Add(&Job{Name: "every-sec", Cron: "* * * * *", Prompt: "x", Enabled: true}); err != nil {
		t.Fatalf("add: %v", err)
	}
	id := s.List()[0].ID
	if err := sch.SetEnabled(id, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	got, _ := sch.Get(id)
	if got.Enabled {
		t.Fatal("job should be disabled")
	}
	// 重启后仍停用（持久化生效）
	sch.Stop()
	if err := sch.Start(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer sch.Stop()
	got, _ = sch.Get(id)
	if got.Enabled {
		t.Fatal("job should stay disabled after restart")
	}
}

func TestSchedulerRemove(t *testing.T) {
	s := newTestStore(t)
	sch := NewScheduler(s, nil, 0)
	if err := sch.Add(&Job{Name: "x", Cron: "0 8 * * *", Prompt: "x", Enabled: true}); err != nil {
		t.Fatalf("add: %v", err)
	}
	id := s.List()[0].ID
	if err := sch.Remove(id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := sch.Get(id); ok {
		t.Fatal("job should be gone")
	}
	if err := sch.Remove(id); err == nil {
		t.Fatal("removing missing job should error")
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("a", maxOutputLen+100)
	if got := truncate(long); len(got) != maxOutputLen {
		t.Fatalf("truncate len = %d, want %d", len(got), maxOutputLen)
	}
	if truncate("short") != "short" {
		t.Fatal("short string should be unchanged")
	}
}

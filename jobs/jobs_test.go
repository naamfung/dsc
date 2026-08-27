package jobs

import (
	"errors"
	"testing"
	"time"
)

// waitStatus 轮询等待任务落定为指定状态。
func waitStatus(t *testing.T, r *Registry, id string, want JobStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snap, err := r.Get(id, ""); err == nil && snap.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if snap, err := r.Get(id, ""); err == nil {
		t.Fatalf("job %s = %s (%s), want %s", id, snap.Status, snap.Detail, want)
	}
	t.Fatalf("job %s not found", id)
}

// finishOutcome 便捷构造：立即交付终态。
func finishOutcome(status JobStatus, detail, output string) JobHooks {
	done := make(chan JobOutcome, 1)
	go func() { done <- JobOutcome{Status: status, Detail: detail, Output: output} }()
	return JobHooks{Done: done}
}

func TestRegistryLifecycle(t *testing.T) {
	r := NewRegistry()

	// 成功（final-output）：登记即 running，落定后 completed + 终态幂等读
	id, err := r.Start(StartSpec{
		Kind:  "workflow",
		Label: "tally",
		Start: func() (JobHooks, error) {
			done := make(chan JobOutcome, 1)
			go func() { time.Sleep(10 * time.Millisecond); done <- JobOutcome{Status: StatusCompleted, Output: "ok"} }()
			return JobHooks{Done: done}, nil
		},
	})
	if err != nil || id != "workflow-1" {
		t.Fatalf("start = %q, %v", id, err)
	}
	if snap, _ := r.Get(id, ""); snap.Status != StatusRunning {
		t.Fatalf("started job should be running, got %s", snap.Status)
	}
	waitStatus(t, r, id, StatusCompleted)
	rd, err := r.Read(id, "")
	if err != nil || rd.Text != "ok" || rd.Snapshot.Status != StatusCompleted {
		t.Fatalf("read = %+v, %v", rd, err)
	}
	if !rd.Snapshot.Reported {
		t.Fatal("terminal read should mark reported")
	}
	// 终态读取幂等（不消费）
	rd2, _ := r.Read(id, "")
	if rd2.Text != "ok" {
		t.Fatalf("terminal read should be idempotent, got %q", rd2.Text)
	}

	// 失败
	id2, _ := r.Start(StartSpec{Kind: "workflow", Label: "boom", Start: func() (JobHooks, error) {
		return finishOutcome(StatusFailed, "boom", ""), nil
	}})
	waitStatus(t, r, id2, StatusFailed)
	if snap, _ := r.Get(id2, ""); snap.Detail != "boom" {
		t.Fatalf("failed detail = %q", snap.Detail)
	}

	// 取消（killed）：kill 标记 stopping + reported，生产方 settle killed
	id3, _ := r.Start(StartSpec{Kind: "workflow", Label: "hang", Start: func() (JobHooks, error) {
		done := make(chan JobOutcome, 1)
		cancel := func(reason string) {
			go func() { done <- JobOutcome{Status: StatusKilled} }()
		}
		return JobHooks{Cancel: cancel, Done: done}, nil
	}})
	if res, err := r.Kill(id3, "", "too slow"); err != nil || res != KillRequested {
		t.Fatalf("kill = %q, %v", res, err)
	}
	waitStatus(t, r, id3, StatusKilled)
	if res, _ := r.Kill(id3, "", ""); res != KillAlreadyFinished {
		t.Fatalf("kill finished = %q", res)
	}

	// Start 抛错不登记
	if _, err := r.Start(StartSpec{Kind: "workflow", Label: "bad", Start: func() (JobHooks, error) {
		return JobHooks{}, errors.New("boom")
	}}); err == nil {
		t.Fatal("starter error should propagate")
	}
	// 无效 spec
	if _, err := r.Start(StartSpec{Label: "no-kind"}); err == nil {
		t.Fatal("missing kind should fail")
	}

	// 未知任务
	if _, err := r.Get("nope", ""); err == nil {
		t.Fatal("unknown job should fail")
	}

	// List 按启动顺序
	all := r.List("")
	if len(all) != 3 || all[0].ID != "workflow-1" || all[2].ID != "workflow-3" {
		t.Fatalf("list = %+v", all)
	}
}

func TestRegistryOwnerIsolation(t *testing.T) {
	r := NewRegistry()

	// 有 owner 的任务：仅 owner 可访问
	id, _ := r.Start(StartSpec{
		Kind: "workflow", Label: "private", Owner: "sess-a",
		Start: func() (JobHooks, error) {
			return finishOutcome(StatusCompleted, "", "secret"), nil
		},
	})
	if _, err := r.Get(id, "sess-a"); err != nil {
		t.Fatalf("owner should access own job: %v", err)
	}
	for _, caller := range []string{"sess-b", ""} {
		if _, err := r.Get(id, caller); err == nil {
			t.Fatalf("caller %q should be denied", caller)
		}
		if _, err := r.Read(id, caller); err == nil {
			t.Fatalf("caller %q should be denied read", caller)
		}
		if _, err := r.Kill(id, caller, ""); err == nil {
			t.Fatalf("caller %q should be denied kill", caller)
		}
	}
	if all := r.List("sess-b"); len(all) != 0 {
		t.Fatalf("foreign caller should see no owned jobs, got %+v", all)
	}
	if all := r.List("sess-a"); len(all) != 1 {
		t.Fatalf("owner should see own job, got %+v", all)
	}
	// 无 owner 的任务：所有调用方（含空）可访问
	id2, _ := r.Start(StartSpec{Kind: "bash", Label: "open", Start: func() (JobHooks, error) {
		return finishOutcome(StatusCompleted, "", "open"), nil
	}})
	waitStatus(t, r, id2, StatusCompleted)
	for _, caller := range []string{"sess-b", ""} {
		if snap, err := r.Get(id2, caller); err != nil || snap.Status != StatusCompleted {
			t.Fatalf("unowned job should be open to %q: %v", caller, err)
		}
	}
	if all := r.List("sess-b"); len(all) != 1 || all[0].ID != id2 {
		t.Fatalf("unowned job should be visible to foreign caller, got %+v", all)
	}
}

func TestRegistryStreamCursor(t *testing.T) {
	r := NewRegistry()
	chunks := []string{"a", "b", "c"}
	pos := 0
	id, _ := r.Start(StartSpec{
		Kind: "bash", Label: "stream",
		Start: func() (JobHooks, error) {
			done := make(chan JobOutcome, 1)
			go func() { time.Sleep(20 * time.Millisecond); done <- JobOutcome{Status: StatusCompleted} }()
			return JobHooks{
				Done: done,
				ReadOutput: func() string {
					if pos < len(chunks) {
						c := chunks[pos]
						pos++
						return c
					}
					return ""
				},
			}, nil
		},
	})
	// 消费式游标：每次返回下一个增量
	rd1, _ := r.Read(id, "")
	if rd1.Text != "a" {
		t.Fatalf("first delta = %q", rd1.Text)
	}
	rd2, _ := r.Read(id, "")
	if rd2.Text != "b" {
		t.Fatalf("second delta = %q", rd2.Text)
	}
	waitStatus(t, r, id, StatusCompleted)
	rd3, _ := r.Read(id, "")
	if rd3.Text != "c" {
		t.Fatalf("final delta = %q", rd3.Text)
	}
	rd4, _ := r.Read(id, "")
	if rd4.Text != "" {
		t.Fatalf("consumed cursor should read empty, got %q", rd4.Text)
	}
}

func TestRegistryWait(t *testing.T) {
	r := NewRegistry()
	id, _ := r.Start(StartSpec{
		Kind: "workflow", Label: "slow",
		Start: func() (JobHooks, error) {
			done := make(chan JobOutcome, 1)
			go func() { time.Sleep(50 * time.Millisecond); done <- JobOutcome{Status: StatusCompleted, Output: "done"} }()
			return JobHooks{Done: done}, nil
		},
	})
	// 短超时：返回存活快照
	snap, err := r.Wait(id, 5*time.Millisecond, "")
	if err != nil || snap.Status != StatusRunning {
		t.Fatalf("short wait = %+v, %v", snap, err)
	}
	// 长超时：落定后返回终态 + reported
	snap, err = r.Wait(id, 2*time.Second, "")
	if err != nil || snap.Status != StatusCompleted || !snap.Reported {
		t.Fatalf("wait settled = %+v, %v", snap, err)
	}
	// 已终态立即返回
	snap, _ = r.Wait(id, 0, "")
	if snap.Status != StatusCompleted {
		t.Fatalf("already-settled wait = %+v", snap)
	}
}

func TestRegistryOnJobDone(t *testing.T) {
	r := NewRegistry()
	got := make(chan JobSnapshot, 4)
	unsub := r.OnJobDone(func(s JobSnapshot) { got <- s })

	// 落定 → 通知监听器（含 status）
	id, _ := r.Start(StartSpec{
		Kind: "workflow", Label: "x",
		Start: func() (JobHooks, error) {
			return finishOutcome(StatusCompleted, "", "ok"), nil
		},
	})
	select {
	case s := <-got:
		if s.ID != id || s.Status != StatusCompleted {
			t.Fatalf("notified snapshot = %+v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener should be notified on settlement")
	}

	// 监听器异常被隔离（不影响落定与其他监听器）
	panicFlag := false
	r.OnJobDone(func(s JobSnapshot) { panic("boom") })
	r.OnJobDone(func(s JobSnapshot) { panicFlag = true })
	id2, _ := r.Start(StartSpec{Kind: "bash", Label: "y", Start: func() (JobHooks, error) {
		return finishOutcome(StatusCompleted, "", "y"), nil
	}})
	waitStatus(t, r, id2, StatusCompleted)
	select {
	case s := <-got:
		if s.ID != id2 {
			t.Fatalf("second notified snapshot = %+v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second listener should be notified")
	}
	if !panicFlag {
		t.Fatal("contained listener should still run after a panicking sibling")
	}

	// 取消订阅后不再通知
	unsub()
	id3, _ := r.Start(StartSpec{Kind: "bash", Label: "z", Start: func() (JobHooks, error) {
		return finishOutcome(StatusCompleted, "", "z"), nil
	}})
	waitStatus(t, r, id3, StatusCompleted)
	select {
	case s := <-got:
		t.Fatalf("unsubscribed listener should not fire, got %+v", s)
	case <-time.After(30 * time.Millisecond):
	}
}

// TestRegistryHostView 校验宿主管理视图（TUI /jobs）：ListAll 不做 owner 隔离，
// ReadHost/KillHost 可读/取消任意任务（含他会话 owner 任务）。
func TestRegistryHostView(t *testing.T) {
	r := NewRegistry()

	// owner 任务（模型经会话调用产生的 workflow background）
	id, err := r.Start(StartSpec{
		Kind:  "workflow",
		Label: "tally",
		Owner: "sess-a",
		Start: func() (JobHooks, error) {
			done := make(chan JobOutcome, 1)
			go func() { time.Sleep(10 * time.Millisecond); done <- JobOutcome{Status: StatusCompleted, Output: "ok"} }()
			return JobHooks{Done: done}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 普通调用方看不到 owner 任务，宿主视图（ListAll）能看到
	if all := r.List(""); len(all) != 0 {
		t.Fatalf("List('') should hide owner job, got %d", len(all))
	}
	if all := r.ListAll(); len(all) != 1 || all[0].ID != id {
		t.Fatalf("ListAll = %+v, want 1 job %s", all, id)
	}

	// owner 任务不能用空 caller 的 Get/Wait 等待，用宿主视图 ReadHost 轮询
	waitHost := func(id string, want JobStatus) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if rd, err := r.ReadHost(id); err == nil && rd.Snapshot.Status == want {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("job %s not settled to %s", id, want)
	}
	waitHost(id, StatusCompleted)
	// 普通 Read(id, "") 对 owner 任务报错；ReadHost 可读
	if _, err := r.Read(id, ""); err == nil {
		t.Fatal("Read('') on owner job should be rejected")
	}
	rd, err := r.ReadHost(id)
	if err != nil || rd.Text != "ok" {
		t.Fatalf("ReadHost = %+v, %v", rd, err)
	}

	// kill：普通 Kill 拒绝，KillHost 可取消（Start 提供 Cancel 钩子才能落定 killed）
	id2, _ := r.Start(StartSpec{
		Kind:  "workflow",
		Label: "hang",
		Owner: "sess-b",
		Start: func() (JobHooks, error) {
			done := make(chan JobOutcome, 1)
			cancel := func(reason string) {
				select {
				case done <- JobOutcome{Status: StatusKilled}:
				default:
				}
			}
			return JobHooks{Cancel: cancel, Done: done}, nil
		},
	})
	if _, err := r.Kill(id2, "", "x"); err == nil {
		t.Fatal("Kill('') on owner job should be rejected")
	}
	if res, err := r.KillHost(id2, "user"); err != nil || res != KillRequested {
		t.Fatalf("KillHost = %v, %v", res, err)
	}
	waitHost(id2, StatusKilled)
}

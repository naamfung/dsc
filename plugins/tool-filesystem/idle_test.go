package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRunWithIdleTimeoutActiveKeepsAlive 验证：只要持续有输出就不断续命，运行到
// 正常结束也不会超时。
func TestRunWithIdleTimeoutActiveKeepsAlive(t *testing.T) {
	old := shellIdleInitial
	shellIdleInitial = 200 * time.Millisecond
	defer func() { shellIdleInitial = old }()

	sess := &Session{StdoutBuf: &syncedBuilder{}, StderrBuf: &syncedBuilder{}}
	done := make(chan error, 1)
	go func() {
		done <- runWithIdleTimeout(context.Background(), sess, func(ctx context.Context) error {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for i := 0; i < 10; i++ { // 50ms*10=500ms，远超 200ms idle budget，但全程有输出
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					sess.StdoutBuf.Write([]byte("x"))
				}
			}
			return nil
		})
	}()

	if err := <-done; err != nil {
		t.Fatalf("active command with output should complete normally, got %v", err)
	}
}

// TestRunWithIdleTimeoutFiresWhenSilent 验证：长时间完全无新输出才触发 idle 超时，
// 且触发足够及时（此处用 2s 运行但 idle budget 仅 200ms）。
func TestRunWithIdleTimeoutFiresWhenSilent(t *testing.T) {
	old := shellIdleInitial
	shellIdleInitial = 200 * time.Millisecond
	defer func() { shellIdleInitial = old }()

	sess := &Session{StdoutBuf: &syncedBuilder{}, StderrBuf: &syncedBuilder{}}
	start := time.Now()
	err := runWithIdleTimeout(context.Background(), sess, func(ctx context.Context) error {
		<-ctx.Done() // 静默阻塞，等被 idle 取消
		return ctx.Err()
	})
	if !errors.Is(err, errShellIdleTimeout) {
		t.Fatalf("silent run should idle-timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("idle timeout fired too late: %v", elapsed)
	}
}

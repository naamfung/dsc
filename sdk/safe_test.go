package dsc

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSafeGoroutineRuns(t *testing.T) {
	done := make(chan struct{})
	SafeGoroutine(func() { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fn not executed in goroutine")
	}
}

func TestSafeGoroutineRecoversPanic(t *testing.T) {
	// 重定向 stderr 到临时文件捕获 panic 日志（SafeGoroutine 经 stderr 记录）
	old := os.Stderr
	f, err := os.CreateTemp("", "safe_goroutine_*.log")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	logPath := f.Name()
	os.Stderr = f
	defer func() { os.Stderr = old; f.Close(); os.Remove(logPath) }()

	done := make(chan struct{})
	SafeGoroutine(func() {
		close(done) // 先通知测试，随后 panic 触发 recover 日志
		panic("boom")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not start")
	}
	// recover defer 在 close(done) 之后执行（defer LIFO），轮询等日志落盘
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(logPath)
		if len(data) > 0 {
			if !strings.Contains(string(data), "panicked") || !strings.Contains(string(data), "boom") {
				t.Fatalf("stderr = %q, want panic log containing 'panicked' and 'boom'", data)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("panic log not written to stderr (recover failed)")
}

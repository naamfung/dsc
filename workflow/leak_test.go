package workflow

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// ctxWaitingRunner 阻塞直到 ctx 取消才返回（正确响应用户取消的子代理）。
type ctxWaitingRunner struct{}

func (ctxWaitingRunner) RunAgent(ctx context.Context, prompt string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// leakScript 含 parallel 组 + 顶层 agent，驱动多个 runAgent goroutine。
func leakScript() string {
	return `local a = parallel({function() return agent("a1") end, function() return agent("a2") end}) local b = agent("a3") return {a, b}`
}

func leakSettleRequest() StartRequest {
	return StartRequest{
		Meta:   Meta{Name: "leak-settle", Description: "leak"},
		Script: leakScript(),
		Runner: &fakeRunner{resps: map[string]string{"a1": "1", "a2": "2", "a3": "3"}},
	}
}

func waitResult(t *testing.T, r *Run) Result {
	t.Helper()
	select {
	case res := <-r.Result:
		return res
	case <-time.After(5 * time.Second):
		t.Fatal("run did not settle within 5s")
		return Result{}
	}
}

// settleLeak 循环跑，断言 goroutine 数稳定（无随跑次的泄漏）。
func settleLeak(t *testing.T, iters int) {
	t.Helper()
	for i := 0; i < 10; i++ { // 预热
		r, err := Start(context.Background(), leakSettleRequest())
		if err != nil {
			t.Fatal(err)
		}
		waitResult(t, r)
	}
	runtime.GC()
	base := runtime.NumGoroutine()
	for i := 0; i < iters; i++ {
		r, err := Start(context.Background(), leakSettleRequest())
		if err != nil {
			t.Fatal(err)
		}
		waitResult(t, r)
	}
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	runtime.GC()
	if leak := runtime.NumGoroutine(); leak > base+4 {
		t.Fatalf("goroutine 泄漏: base=%d after=%d (iters=%d)", base, leak, iters)
	}
}

func TestNoGoroutineLeakSettle(t *testing.T) {
	settleLeak(t, 80)
}

// cancelLeak 循环：start 后立刻取消，断言取消后在途 agent goroutine 都退出、
// goroutine 数回到基线。
func cancelLeak(t *testing.T, iters int) {
	t.Helper()
	req := func() StartRequest {
		return StartRequest{
			Meta:   Meta{Name: "leak-cancel", Description: "leak"},
			Script: leakScript(),
			Runner: ctxWaitingRunner{},
		}
	}
	// 预热
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		r, err := Start(ctx, req())
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		waitResult(t, r)
	}
	runtime.GC()
	base := runtime.NumGoroutine()
	for i := 0; i < iters; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		r, err := Start(ctx, req())
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		waitResult(t, r)
	}
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	runtime.GC()
	if leak := runtime.NumGoroutine(); leak > base+4 {
		t.Fatalf("goroutine 泄漏(取消): base=%d after=%d (iters=%d)", base, leak, iters)
	}
}

func TestNoGoroutineLeakCancel(t *testing.T) {
	cancelLeak(t, 80)
}

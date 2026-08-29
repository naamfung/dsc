package luaproc

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeRunner 带并发峰值统计的假 agent 运行器。
type fakeRunner struct {
	mu     sync.Mutex
	active int
	max    int
	sleep  time.Duration
}

func (f *fakeRunner) RunAgent(_ context.Context, prompt string) (string, error) {
	f.mu.Lock()
	f.active++
	if f.active > f.max {
		f.max = f.active
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	if f.sleep > 0 {
		time.Sleep(f.sleep)
	}
	return "R:" + prompt, nil
}

func (f *fakeRunner) maxActive() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.max
}

// 1) agent()：顶层顺序跑两个子代理，结果逐一 resume 回来。
func TestSequentialAgents(t *testing.T) {
	r := Run(context.Background(), Options{
		Script: `local a = agent("x"); local b = agent("y"); return a .. "," .. b`,
		Runner: &fakeRunner{},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	if r.Value != "R:x,R:y" {
		t.Fatalf("value = %v, want R:x,R:y", r.Value)
	}
}

// 2) parallel()：并发跑一组 thunk，结果按原序返回数组。
func TestParallelResultsInOrder(t *testing.T) {
	r := Run(context.Background(), Options{
		Script: `return parallel({
				function() return agent("a") end,
				function() return agent("b") end,
				function() return agent("c") end
			})`,
		Runner: &fakeRunner{sleep: 10 * time.Millisecond},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	want := []any{
		map[string]any{"ok": true, "value": "R:a"},
		map[string]any{"ok": true, "value": "R:b"},
		map[string]any{"ok": true, "value": "R:c"},
	}
	if !reflect.DeepEqual(r.Value, want) {
		t.Fatalf("value = %v, want %v", r.Value, want)
	}
}

// 3) parallel() 受并发上限约束。
func TestParallelRespectsConcurrencyCap(t *testing.T) {
	runner := &fakeRunner{sleep: 30 * time.Millisecond}
	r := Run(context.Background(), Options{
		Script: `local fns = {}
				for i = 1, 5 do
					fns[i] = function() return agent("p" .. i) end
				end
				return parallel(fns)`,
		Runner:              runner,
		MaxConcurrentAgents: 2,
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	if arr, ok := r.Value.([]any); !ok || len(arr) != 5 {
		t.Fatalf("value = %v, want 5 results", r.Value)
	}
	if got := runner.maxActive(); got > 2 {
		t.Fatalf("max concurrent agents = %d, want <= 2", got)
	}
}

// 4) pipeline()：逐 item 并发；某 item 的 stage 抛错 → 该 item 置 null，其余正常。
func TestPipelineErrorIsolatesItem(t *testing.T) {
	r := Run(context.Background(), Options{
		Script: `local f1 = function(v, it, i) return v * 2 end
				local f2 = function(v, it, i) if it == 2 then error("boom") end return v end
				return pipeline({1, 2, 3}, f1, f2)`,
		Runner: &fakeRunner{},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop_reason = %q, error=%q", r.StopReason, r.Error)
	}
	want := []any{
		map[string]any{"ok": true, "value": int64(2)},
		map[string]any{"ok": false}, // item2 失败 → {ok=false}，仍占位保位置
		map[string]any{"ok": true, "value": int64(6)},
	}
	if !reflect.DeepEqual(r.Value, want) {
		t.Fatalf("value = %v, want %v", r.Value, want)
	}
}

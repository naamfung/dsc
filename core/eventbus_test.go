package core

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const testEv = EventName("test/event")

func TestEmitRunsInOrderAndIgnoresReturn(t *testing.T) {
	b := NewEventBus()
	var order []string
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "a"); return 1, nil })
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "b"); return nil, errors.New("ignored") })
	b.Emit(testEv, EventContext{})
	if strings.Join(order, "") != "ab" {
		t.Fatalf("order = %v, want [a b]", order)
	}
	// 错误与返回值均被忽略
}

func TestParallelRunsAllAndJoinsErrors(t *testing.T) {
	b := NewEventBus()
	var mu sync.Mutex
	count := 0
	for i := 0; i < 4; i++ {
		b.On(testEv, func(EventContext) (any, error) {
			mu.Lock()
			count++
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			return nil, nil
		})
	}
	b.On(testEv, func(EventContext) (any, error) { return nil, errors.New("boom") })
	start := time.Now()
	err := b.Parallel(testEv, EventContext{})
	if err == nil {
		t.Fatal("expected joined error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want boom", err)
	}
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("parallel took too long (%v), listeners not concurrent", time.Since(start))
	}
}

func TestSerialStopsOnError(t *testing.T) {
	b := NewEventBus()
	var order []string
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "a"); return nil, nil })
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "b"); return nil, errors.New("stop") })
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "c"); return nil, nil })
	err := b.Serial(testEv, EventContext{})
	if err == nil || err.Error() != "stop" {
		t.Fatalf("err = %v, want stop", err)
	}
	if strings.Join(order, "") != "ab" {
		t.Fatalf("order = %v, want [a b] (c not run)", order)
	}
}

func TestBailShortCircuits(t *testing.T) {
	b := NewEventBus()
	var order []string
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "a"); return nil, nil })
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "b"); return 42, nil })
	b.On(testEv, func(EventContext) (any, error) { order = append(order, "c"); return nil, nil })
	v, err := b.Bail(testEv, EventContext{})
	if err != nil || v != 42 {
		t.Fatalf("bail = (%v, %v), want (42, nil)", v, err)
	}
	if strings.Join(order, "") != "ab" {
		t.Fatalf("order = %v, want [a b]", order)
	}
}

func TestBailNoListener(t *testing.T) {
	b := NewEventBus()
	v, err := b.Bail(testEv, EventContext{})
	if v != nil || err != nil {
		t.Fatalf("bail on empty = (%v, %v), want (nil, nil)", v, err)
	}
}

func TestWaterfallOnionDelegation(t *testing.T) {
	b := NewEventBus()
	var order []string
	b.OnWaterfall(testEv, func(ctx EventContext, next func(EventContext) error) error {
		order = append(order, "outer-before")
		err := next(ctx)
		order = append(order, "outer-after")
		return err
	})
	b.OnWaterfall(testEv, func(ctx EventContext, next func(EventContext) error) error {
		order = append(order, "inner")
		return fmt.Errorf("inner result")
	})
	err := b.Waterfall(testEv, EventContext{}, func(EventContext) error {
		order = append(order, "fallback")
		return nil
	})
	if err == nil || err.Error() != "inner result" {
		t.Fatalf("err = %v, want inner result", err)
	}
	want := "outer-beforeinnerouter-after"
	if strings.Join(order, "") != want {
		t.Fatalf("order = %v, want %s", order, want)
	}
}

func TestWaterfallVetoWithoutNext(t *testing.T) {
	b := NewEventBus()
	b.OnWaterfall(testEv, func(ctx EventContext, next func(EventContext) error) error {
		return errors.New("vetoed")
	})
	b.OnWaterfall(testEv, func(ctx EventContext, next func(EventContext) error) error {
		return errors.New("must not run")
	})
	err := b.Waterfall(testEv, EventContext{}, func(EventContext) error {
		return errors.New("fallback must not run")
	})
	if err == nil || err.Error() != "vetoed" {
		t.Fatalf("err = %v, want vetoed", err)
	}
}

func TestWaterfallNoListenerCallsFallback(t *testing.T) {
	b := NewEventBus()
	called := false
	err := b.Waterfall(testEv, EventContext{}, func(EventContext) error {
		called = true
		return nil
	})
	if err != nil || !called {
		t.Fatalf("fallback not called (err=%v, called=%v)", err, called)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewEventBus()
	count := 0
	off := b.On(testEv, func(EventContext) (any, error) { count++; return nil, nil })
	b.Emit(testEv, EventContext{})
	off()
	b.Emit(testEv, EventContext{})
	if count != 1 {
		t.Fatalf("count = %d, want 1 after unsubscribe", count)
	}
}

func TestUnsubscribeWaterfall(t *testing.T) {
	b := NewEventBus()
	called := false
	off := b.OnWaterfall(testEv, func(ctx EventContext, next func(EventContext) error) error {
		called = true
		return next(ctx)
	})
	off()
	_ = b.Waterfall(testEv, EventContext{}, func(EventContext) error { return nil })
	if called {
		t.Fatal("unsubscribed waterfall listener still called")
	}
}

func TestContextNameAssigned(t *testing.T) {
	b := NewEventBus()
	var got EventName
	b.On(testEv, func(ctx EventContext) (any, error) { got = ctx.Name; return nil, nil })
	b.Emit(testEv, EventContext{})
	if got != testEv {
		t.Fatalf("ctx.Name = %q, want %q", got, testEv)
	}
}

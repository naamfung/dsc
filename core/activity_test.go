package core

import (
	"context"
	"testing"
	"time"
)

// TestWithIdleDeadlineFiresWhenSilent 静默路径：起步预算耗尽（持续无活动）
// 后 ctx 以 ErrIdleDeadline 为 cause 取消——看门狗机械语义。
func TestWithIdleDeadlineFiresWhenSilent(t *testing.T) {
	ctx, cancel := WithIdleDeadline(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	select {
	case <-ctx.Done():
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("看门狗触发过晚: %v", elapsed)
		}
		if cause := context.Cause(ctx); cause != ErrIdleDeadline {
			t.Fatalf("cause = %v, want ErrIdleDeadline", cause)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("silent ctx should be cancelled by the idle watchdog")
	}
	if !IdleDeadlineExceeded(ctx) {
		t.Fatal("IdleDeadlineExceeded should report true after the watchdog fired")
	}
}

// TestTouchActivityExtendsDeadline 活动续命：每次 TouchActivity 重置计时，
// 持续活动的执行远超单笔预算仍不被取消；停止活动后按时触发。
func TestTouchActivityExtendsDeadline(t *testing.T) {
	ctx, cancel := WithIdleDeadline(context.Background(), 150*time.Millisecond)
	defer cancel()
	// 每 50ms 活动一次，共 400ms（远超 150ms 预算）：不应被取消
	for i := 0; i < 8; i++ {
		select {
		case <-ctx.Done():
			t.Fatalf("持续活动不应触发看门狗（第 %d 次活动后取消）", i)
		case <-time.After(50 * time.Millisecond):
		}
		TouchActivity(ctx)
	}
	// 停止活动：150ms 预算内触发
	select {
	case <-ctx.Done():
		if context.Cause(ctx) != ErrIdleDeadline {
			t.Fatalf("cause = %v, want ErrIdleDeadline", context.Cause(ctx))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("停止活动后看门狗应触发")
	}
}

// TestTouchActivityWithoutHandleIsNoOp 无执行域的 ctx：TouchActivity 为 no-op
// （工具在无超时策略环境下照常工作，活动上报不依赖策略在场）。
func TestTouchActivityWithoutHandleIsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	TouchActivity(ctx) // 不得 panic
	TouchActivity(context.Background())
	select {
	case <-ctx.Done():
		t.Fatal("no-op touch must not cancel the context")
	default:
	}
}

// TestIdleDeadlineCancelStopsWatchdog 正常完成路径：cancel 停表并取消；
// 迟到的看门狗触发不得覆盖既有 cause（首次 cancel 定格 cause）。
func TestIdleDeadlineCancelStopsWatchdog(t *testing.T) {
	ctx, cancel := WithIdleDeadline(context.Background(), 80*time.Millisecond)
	cancel() // 立即收尾
	if err := ctx.Err(); err == nil {
		t.Fatal("cancelled ctx should report Err")
	}
	if IdleDeadlineExceeded(ctx) {
		t.Fatal("cancel(nil) 的 cause 不应被误报为空闲看门狗触发")
	}
	time.Sleep(200 * time.Millisecond) // 越过原预算：迟到触发不得改写 cause
	if context.Cause(ctx) == ErrIdleDeadline {
		t.Fatal("迟到触发不得覆盖首次 cancel 的 cause")
	}
}

// TestWithIdleDeadlineZeroDisables idle <= 0：不安装执行域（ctx 原样返回，
// cancel 为 no-op）——对应 timeout-policy 裁决「禁用该工具超时」。
func TestWithIdleDeadlineZeroDisables(t *testing.T) {
	base := context.Background()
	ctx, cancel := WithIdleDeadline(base, 0)
	defer cancel()
	if ctx != base {
		t.Fatal("idle=0 时应原样返回调用方 ctx")
	}
	TouchActivity(ctx) // no-op，不得 panic
}

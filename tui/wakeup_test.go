package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"dsc/jobs"
	"dsc/plugin"
)

// wakeSnap 便捷构造完成快照。
func wakeSnap(id string) jobs.JobSnapshot {
	return jobs.JobSnapshot{ID: id, Kind: "workflow", Label: "x", Status: jobs.StatusCompleted}
}

func TestWakeup(t *testing.T) {
	m := New(&stubAgent{frames: []*plugin.RunStreamResponse{{Status: "success"}}}, nil, context.Background(), "m", "minimal", 131072)

	// 空闲时收到完成通知 → 自动开启一轮，预算扣减，通知清空
	_, cmd := m.Update(jobDoneMsg{snapshot: wakeSnap("workflow-1")})
	if !m.thinking {
		t.Fatal("idle wakeup should start a turn")
	}
	if cmd == nil {
		t.Fatal("wakeup should return a start cmd")
	}
	if m.wakeBudget != maxConsecutiveWakes-1 {
		t.Fatalf("budget = %d, want %d", m.wakeBudget, maxConsecutiveWakes-1)
	}
	if len(m.pendingWakeups) != 0 {
		t.Fatalf("notifications should be consumed by wakeup, got %+v", m.pendingWakeups)
	}
	// 通知渲染为系统提示行
	if full := strings.Join(m.lines, "\n"); !strings.Contains(full, "background job workflow-1") {
		t.Fatalf("notice should render, got: %q", full)
	}

	// 忙碌时收到通知 → 排队，不自动发
	m.thinking = false // 上一轮已结束
	m.streaming = true // 模拟新一轮忙碌
	_, cmd = m.Update(jobDoneMsg{snapshot: wakeSnap("workflow-2")})
	if cmd != nil {
		t.Fatal("busy should not auto-wake")
	}
	if len(m.pendingWakeups) != 1 {
		t.Fatalf("busy notification should queue, got %+v", m.pendingWakeups)
	}
	m.streaming = false

	// 预算耗尽 → 通知排队，不自动发
	m.wakeBudget = 0
	_, cmd = m.Update(jobDoneMsg{snapshot: wakeSnap("workflow-3")})
	if cmd != nil || m.thinking {
		t.Fatal("exhausted budget should not wake")
	}
	if len(m.pendingWakeups) != 2 {
		t.Fatalf("queued notifications = %d, want 2", len(m.pendingWakeups))
	}

	// 用户消息 → 前置注入排队通知 + 恢复预算
	m.input.SetValue("继续")
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.wakeBudget != maxConsecutiveWakes {
		t.Fatalf("user message should restore budget, got %d", m.wakeBudget)
	}
	if len(m.pendingWakeups) != 0 {
		t.Fatalf("notifications should be injected, got %+v", m.pendingWakeups)
	}
}

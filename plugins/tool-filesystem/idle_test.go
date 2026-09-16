package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"dsc/core"
)

// TestShellOutputTouchesKeepsAlive 活动信号链路端到端（插件侧）：命令持续产生
// 输出时，每次写入经 activityWriter 上报活动（core.TouchActivity）——总运行
// 时长（~500ms）远超空闲预算（200ms），全程有输出故能跑完不被看门狗取消。
func TestShellOutputTouchesKeepsAlive(t *testing.T) {
	sess, err := getOrCreateSession("idle-test-alive", "")
	if err != nil {
		t.Fatalf("getOrCreateSession: %v", err)
	}
	ctx, cancel := core.WithIdleDeadline(context.Background(), 200*time.Millisecond)
	defer cancel()

	out, code, err := execSessionCommand(ctx, sess, "for i in 1 2 3 4 5 6 7 8 9 10; do echo tick-$i; sleep 0.05; done")
	if err != nil {
		t.Fatalf("持续输出的命令不应被看门狗取消: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "tick-10") {
		t.Fatalf("output 缺失末尾输出: %q", out)
	}
	if core.IdleDeadlineExceeded(ctx) {
		t.Fatal("看门狗不应触发")
	}
}

// TestShellSilentCommandHitsIdleDeadline 看门狗触发路径（插件侧）：静默命令
// 超过空闲预算即被取消，execSessionCommand 上抛取消错误——文案由工具流水线桥
// 按裁决替换（见 core 层 bridge 测试）。
func TestShellSilentCommandHitsIdleDeadline(t *testing.T) {
	sess, err := getOrCreateSession("idle-test-silent", "")
	if err != nil {
		t.Fatalf("getOrCreateSession: %v", err)
	}
	ctx, cancel := core.WithIdleDeadline(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err = execSessionCommand(ctx, sess, "sleep 5")
	if err == nil {
		t.Fatal("静默命令应触发空闲截止")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("看门狗触发过晚: %v", elapsed)
	}
	if !core.IdleDeadlineExceeded(ctx) {
		t.Fatal("取消 cause 应为空闲看门狗触发")
	}
}

// TestShellNoDeadlineRunsFine 无执行域（策略缺失/禁用）：TouchActivity 为
// no-op，shell 照常执行——策略缺失降级为无超时，而非工具不可用。
func TestShellNoDeadlineRunsFine(t *testing.T) {
	sess, err := getOrCreateSession("idle-test-plain", "")
	if err != nil {
		t.Fatalf("getOrCreateSession: %v", err)
	}
	out, code, err := execSessionCommand(context.Background(), sess, "echo plain-run")
	if err != nil || code != 0 {
		t.Fatalf("plain run: %v (exit %d)", err, code)
	}
	if !strings.Contains(out, "plain-run") {
		t.Fatalf("output = %q", out)
	}
}

package core

import (
	"context"
	"strings"
	"testing"
)

// TestTokenMeterUpdateUsage 校验：UpdateUsage 正确存储用量并计算提示缓存感知的总量。
func TestTokenMeterUpdateUsage(t *testing.T) {
	tm := NewTokenMeter(10000)

	// 无 usage 时应回退到 estimate
	msgs := []*Message{{Role: "user", Content: "hello world"}}
	m := tm.Measure(msgs)
	if m.Source != "estimate" {
		t.Errorf("无 usage 时 Source 应为 estimate，got %q", m.Source)
	}
	if m.TotalTokens <= 0 {
		t.Error("estimate 应返回 > 0 tokens")
	}

	// 更新 usage
	tm.UpdateUsage(&Usage{
		PromptTokens:         5000,
		CompletionTokens:     200,
		CacheReadInputTokens: 3000, // 缓存命中 3000 tokens
	})

	m = tm.Measure(msgs)
	if m.Source != "usage" {
		t.Errorf("有 usage 时 Source 应为 usage，got %q", m.Source)
	}
	// 提示缓存感知：完整请求 = prompt(5000) + cache_read(3000) = 8000
	// 总量 = 8000 + completion(200) = 8200
	if m.TotalTokens != 8200 {
		t.Errorf("TotalTokens 应为 8200（prompt+cache_read+completion），got %d", m.TotalTokens)
	}
	if m.Pressure < 0.81 || m.Pressure > 0.83 {
		t.Errorf("Pressure 应约 0.82，got %.4f", m.Pressure)
	}
}

// TestTokenMeterShouldCompact 校验：压力超过阈值时 ShouldCompact 返回 true。
func TestTokenMeterShouldCompact(t *testing.T) {
	tm := NewTokenMeter(10000)

	// 无 usage → 不压缩
	if tm.ShouldCompact(0.80) {
		t.Error("无 usage 时不应压缩")
	}

	// 压力 82% → 超过 80% 阈值
	tm.UpdateUsage(&Usage{
		PromptTokens:         5000,
		CompletionTokens:     200,
		CacheReadInputTokens: 3000,
	})
	if !tm.ShouldCompact(0.80) {
		t.Error("压力 82% 应触发压缩")
	}
	if tm.ShouldCompact(0.90) {
		t.Error("压力 82% 不应触发 90% 阈值")
	}
}

// TestTokenMeterEstimateTextTokens 校验文本估算：英文按 /4，CJK 按 rune 数。
func TestTokenMeterEstimateTextTokens(t *testing.T) {
	// 英文："hello" 5 字节 → 5/4 = 1.25 → 2 tokens；rune 数 = 5 → 取大者 = 5
	if got := EstimateTextTokens("hello"); got != 5 {
		t.Errorf("EstimateTextTokens(\"hello\") = %d, want 5", got)
	}
	// CJK："你好" 6 字节 → 6/4 = 1.5 → 2 tokens；rune 数 = 2 → 取大者 = 2
	if got := EstimateTextTokens("你好"); got != 2 {
		t.Errorf("EstimateTextTokens(\"你好\") = %d, want 2", got)
	}
	// 混合："hi你好" 8 字节 → 2；rune 数 = 4 → 取大者 = 4
	if got := EstimateTextTokens("hi你好"); got != 4 {
		t.Errorf("EstimateTextTokens(\"hi你好\") = %d, want 4", got)
	}
	// 空串
	if got := EstimateTextTokens(""); got != 0 {
		t.Errorf("EstimateTextTokens(\"\") = %d, want 0", got)
	}
}

// TestTokenMeterEstimateMessageTokens 校验消息估算含结构开销。
func TestTokenMeterEstimateMessageTokens(t *testing.T) {
	msg := &Message{Role: "user", Content: "hello"}
	got := EstimateMessageTokens(msg)
	// content = 5 tokens + 4 (role+delimiter) = 9
	if got != 9 {
		t.Errorf("EstimateMessageTokens = %d, want 9 (5 content + 4 overhead)", got)
	}

	// tool 消息有不同开销
	toolMsg := &Message{Role: "tool", Content: "result"}
	got = EstimateMessageTokens(toolMsg)
	// content = 6 + 6 (tool overhead) = 12
	if got != 12 {
		t.Errorf("tool message tokens = %d, want 12", got)
	}
}

// TestTokenMeterEstimateMessagesTokens 校验多条消息总估算。
func TestTokenMeterEstimateMessagesTokens(t *testing.T) {
	msgs := []*Message{
		{Role: "user", Content: "hello"},   // 5 + 4 = 9
		{Role: "assistant", Content: "hi"}, // 2 + 4 = 6
		{Role: "user", Content: "bye"},     // 3 + 4 = 7
	}
	got := EstimateMessagesTokens(msgs)
	if got != 22 {
		t.Errorf("EstimateMessagesTokens = %d, want 22", got)
	}
}

// TestTokenMeterContextWindowUpdate 校验动态更新窗口大小。
func TestTokenMeterContextWindowUpdate(t *testing.T) {
	tm := NewTokenMeter(10000)
	tm.UpdateUsage(&Usage{PromptTokens: 8000, CompletionTokens: 0})

	m := tm.Measure(nil)
	if m.ContextWindow != 10000 {
		t.Errorf("ContextWindow = %d, want 10000", m.ContextWindow)
	}
	if m.Pressure < 0.79 || m.Pressure > 0.81 {
		t.Errorf("Pressure should be ~0.80, got %.4f", m.Pressure)
	}

	// 更新窗口
	tm.UpdateContextWindow(20000)
	m = tm.Measure(nil)
	if m.ContextWindow != 20000 {
		t.Errorf("ContextWindow after update = %d, want 20000", m.ContextWindow)
	}
	if m.Pressure != 0.4 {
		t.Errorf("Pressure after window update should be 0.40, got %.4f", m.Pressure)
	}
}

// TestTokenMeterNoCacheRead 校验：无缓存命中时正常计算。
func TestTokenMeterNoCacheRead(t *testing.T) {
	tm := NewTokenMeter(10000)
	tm.UpdateUsage(&Usage{
		PromptTokens:     6000,
		CompletionTokens: 500,
		// CacheReadInputTokens = 0 (默认零值)
	})

	m := tm.Measure(nil)
	// 无缓存命中：total = prompt(6000) + completion(500) = 6500
	if m.TotalTokens != 6500 {
		t.Errorf("TotalTokens without cache = %d, want 6500", m.TotalTokens)
	}
	if m.CacheReadTokens != 0 {
		t.Errorf("CacheReadTokens should be 0, got %d", m.CacheReadTokens)
	}
}

// TestTokenMeterNilUsage 校验：nil usage 不崩溃。
func TestTokenMeterNilUsage(t *testing.T) {
	tm := NewTokenMeter(10000)
	tm.UpdateUsage(nil) // 不应 panic

	m := tm.Measure([]*Message{{Role: "user", Content: "test"}})
	if m.Source != "estimate" {
		t.Errorf("nil usage 后应回退到 estimate，got %q", m.Source)
	}
}

// TestTokenMeterFormatPressure 校验压力格式化。
func TestTokenMeterFormatPressure(t *testing.T) {
	if got := FormatPressure(0.78); got != "78%" {
		t.Errorf("FormatPressure(0.78) = %q, want '78%%'", got)
	}
	if got := FormatPressure(1.0); got != "100%" {
		t.Errorf("FormatPressure(1.0) = %q, want '100%%'", got)
	}
}

// TestTokenMeterEstimateToolTokens 校验工具定义 token 估算。
func TestTokenMeterEstimateToolTokens(t *testing.T) {
	tools := []Tool{
		{Name: "shell", Description: "Execute a shell command", ParametersJSON: `{"type":"object","properties":{"command":{"type":"string"}}}`},
	}
	got := EstimateToolTokens(tools)
	if got <= 0 {
		t.Error("EstimateToolTokens should return > 0")
	}
	// 验证包含 name + description 的 token 估算
	nameTokens := EstimateTextTokens("shell")
	descTokens := EstimateTextTokens("Execute a shell command")
	if got < nameTokens+descTokens {
		t.Errorf("tool tokens %d should >= name(%d) + desc(%d) = %d", got, nameTokens, descTokens, nameTokens+descTokens)
	}
}

// 消除未使用 import 警告
var _ = context.Background
var _ = strings.Contains

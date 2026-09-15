package tui

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"dsc/core"
)

// TestThinkingLineShowsElapsedHintAndDownstream 思考中行显示耗时、取消快捷键与下行数据。
func TestThinkingLineShowsElapsedHintAndDownstream(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.thinking = true
	m.elapsed = 5
	m.turnTokens = 2048

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "思考中... (5 秒 · Ctrl+C 取消)") {
		t.Fatalf("思考中行应含耗时与取消提示: %q", content)
	}
	if !strings.Contains(content, "↓2K") {
		t.Fatalf("思考中行应含下行数据: %q", content)
	}
}

// TestThinkingLineWithoutDownstream 尚无下行数据时不显示 ↓。
func TestThinkingLineWithoutDownstream(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.thinking = true

	content := ansi.Strip(m.View().Content)
	if strings.Contains(content, "↓") {
		t.Fatalf("无下行数据时不应显示 ↓: %q", content)
	}
}

// TestElapsedTickIncrements 运行中 elapsedTick 按 runStart 刷新耗时并继续 tick。
func TestElapsedTickIncrements(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.thinking = true
	m.runStart = time.Now().Add(-3 * time.Second)

	model, cmd := m.Update(elapsedTickMsg{})
	m2 := model.(*Model)
	if m2.elapsed < 3 {
		t.Fatalf("elapsed = %d, want >= 3", m2.elapsed)
	}
	if cmd == nil {
		t.Fatal("运行中 elapsedTick 应继续返回下一个 tick")
	}
}

// TestRunInfoLineCacheRate 服务端报告缓存字段时显示命中率；否则不显示。
func TestRunInfoLineCacheRate(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 无缓存字段：不显示
	if strings.Contains(m.runInfoLine(), "缓存命中") {
		t.Fatalf("无缓存数据时不应显示缓存命中: %q", m.runInfoLine())
	}

	// 有缓存字段：显示命中率
	m.cacheHit = 90
	m.cacheMiss = 10
	if !strings.Contains(m.runInfoLine(), "缓存命中 90%") {
		t.Fatalf("runInfoLine 应显示缓存命中 90%%: %q", m.runInfoLine())
	}
}

// TestTrackTurnUsage 累计下行生成 token；缓存字段按最近一次请求覆盖。
func TestTrackTurnUsage(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)

	m.trackTurnUsage(&core.Usage{CompletionTokens: 100, CacheReadInputTokens: 80, CacheCreationInputTokens: 20}, 1, 0)
	if m.turnTokens != 100 || m.cacheHit != 80 || m.cacheMiss != 20 {
		t.Fatalf("trackTurnUsage 后: turn=%d hit=%d miss=%d", m.turnTokens, m.cacheHit, m.cacheMiss)
	}

	// 第二步：下行累计，缓存覆盖
	m.trackTurnUsage(&core.Usage{CompletionTokens: 50, CacheReadInputTokens: 95, CacheCreationInputTokens: 5}, 2, 0)
	if m.turnTokens != 150 {
		t.Fatalf("turnTokens 应累计为 150, got %d", m.turnTokens)
	}
	if m.cacheHit != 95 || m.cacheMiss != 5 {
		t.Fatalf("cache 应覆盖为 hit=95 miss=5, got hit=%d miss=%d", m.cacheHit, m.cacheMiss)
	}
}

// TestRunInfoLineTurnStepFormat 指标行轮步段采用「N 轮 M 步」紧凑格式（对齐 DSH
// 「N turns M steps」）；无速率数据时不出现每秒/初速段。
func TestRunInfoLineTurnStepFormat(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.curTurn = 4
	m.curStep = 12

	line := ansi.Strip(m.runInfoLine())
	if !strings.Contains(line, "4 轮 12 步") {
		t.Fatalf("指标行应含「4 轮 12 步」: %q", line)
	}
	if strings.Contains(line, "每秒") || strings.Contains(line, "初速") {
		t.Fatalf("无速率数据时不应显示每秒/初速: %q", line)
	}
}

// TestRunInfoLineThroughput 有速率快照时显示「每秒 X 词元 · 初速 Y 词元」；
// 初速（含首 token 延迟）应低于每秒值（纯解码吞吐）。
func TestRunInfoLineThroughput(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// 1160 词元：解码 10s → 每秒 116；全程 11.6s → 初速 100
	m.decodeTPS = 116
	m.startTPS = 100

	line := ansi.Strip(m.runInfoLine())
	if !strings.Contains(line, "每秒 116 词元") {
		t.Fatalf("指标行应含「每秒 116 词元」: %q", line)
	}
	if !strings.Contains(line, "初速 100 词元") {
		t.Fatalf("指标行应含「初速 100 词元」: %q", line)
	}
	if strings.Index(line, "初速") < strings.Index(line, "每秒") {
		t.Fatalf("初速应位于每秒之后: %q", line)
	}
}

// TestSettleStepMetrics 速率结算：TTFT 越大初速越低；同步多帧不重复结算；
// 无流式内容帧的步跳过保持上一次读数。
func TestSettleStepMetrics(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)

	// 无打点（步开始/首 token 缺失）：跳过
	m.settleStepMetrics(&core.Usage{CompletionTokens: 100})
	if m.decodeTPS != 0 || m.startTPS != 0 {
		t.Fatalf("无打点时不应结算: decode=%v start=%v", m.decodeTPS, m.startTPS)
	}

	// 正常结算：900 词元，首 token 在 9s 前（解码 9s → 每秒 100），步开始 10s 前（全程 10s → 初速 90）
	// time.Since 受调度抖动影响带微小浮点误差，用 0.01 容差断言。
	m.stepStart = time.Now().Add(-10 * time.Second)
	m.firstTokenAt = time.Now().Add(-9 * time.Second)
	m.settleStepMetrics(&core.Usage{CompletionTokens: 900})
	if math.Abs(m.decodeTPS-100) > 0.01 || math.Abs(m.startTPS-90) > 0.01 {
		t.Fatalf("结算后 decodeTPS=%v startTPS=%v, want ~100/~90", m.decodeTPS, m.startTPS)
	}

	// 同步的 tool 结果帧与 success 帧重复携带 Usage：不重复结算（读数不变）
	m.settleStepMetrics(&core.Usage{CompletionTokens: 99999})
	if math.Abs(m.decodeTPS-100) > 0.01 || math.Abs(m.startTPS-90) > 0.01 {
		t.Fatalf("重复结算应被忽略: decodeTPS=%v startTPS=%v", m.decodeTPS, m.startTPS)
	}

	// 新步开始（编号变化）：打点重置，无内容帧的步不结算，读数保持
	m.stepStart = time.Now()
	m.firstTokenAt = time.Time{}
	m.stepSettled = false
	m.settleStepMetrics(&core.Usage{CompletionTokens: 500})
	if math.Abs(m.decodeTPS-100) > 0.01 || math.Abs(m.startTPS-90) > 0.01 {
		t.Fatalf("无首 token 的步不应结算: decodeTPS=%v startTPS=%v", m.decodeTPS, m.startTPS)
	}
}

// TestFormatTokensPerSecond 速率格式化：≥10 取整，<10 保留一位小数（对齐 DSH）。
func TestFormatTokensPerSecond(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{116.4, "116"},
		{9.96, "10"},
		{9.54, "9.5"},
		{0.5, "0.5"},
	}
	for _, c := range cases {
		if got := formatTokensPerSecond(c.in); got != c.want {
			t.Fatalf("formatTokensPerSecond(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

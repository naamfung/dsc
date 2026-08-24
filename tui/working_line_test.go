package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"dsc/plugin"
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

	m.trackTurnUsage(&plugin.Usage{CompletionTokens: 100, CacheReadInputTokens: 80, CacheCreationInputTokens: 20}, 1, 0)
	if m.turnTokens != 100 || m.cacheHit != 80 || m.cacheMiss != 20 {
		t.Fatalf("trackTurnUsage 后: turn=%d hit=%d miss=%d", m.turnTokens, m.cacheHit, m.cacheMiss)
	}

	// 第二步：下行累计，缓存覆盖
	m.trackTurnUsage(&plugin.Usage{CompletionTokens: 50, CacheReadInputTokens: 95, CacheCreationInputTokens: 5}, 2, 0)
	if m.turnTokens != 150 {
		t.Fatalf("turnTokens 应累计为 150, got %d", m.turnTokens)
	}
	if m.cacheHit != 95 || m.cacheMiss != 5 {
		t.Fatalf("cache 应覆盖为 hit=95 miss=5, got hit=%d miss=%d", m.cacheHit, m.cacheMiss)
	}
}

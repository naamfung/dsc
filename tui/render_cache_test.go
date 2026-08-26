package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
)

// newRenderCacheModel 构造一个就绪的 Model（含初始化 viewport），用于测试渲染行缓存。
func newRenderCacheModel(t *testing.T) *Model {
	t.Helper()
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	m.ready = true
	m.width = 50
	m.high = 20
	m.viewport = viewport.New(viewport.WithWidth(49), viewport.WithHeight(15))
	return m
}

// TestRenderedCacheInvalidation 校验渲染行缓存：内容不变时复用已渲染行，
// 就地替换并 invalidate 后增量重算，宽度变化时全量失效。
func TestRenderedCacheInvalidation(t *testing.T) {
	m := newRenderCacheModel(t)
	m.lines = []string{"first", "second"}

	m.render()
	joined := strings.Join(m.wrappedLines, "\n")
	if !strings.Contains(joined, "first") || !strings.Contains(joined, "second") {
		t.Fatalf("initial render missing lines: %q", joined)
	}
	// 渲染后 dirtyFrom 应已推进到末尾（无脏行）
	if m.dirtyFrom != len(m.lines) {
		t.Fatalf("dirtyFrom = %d, want %d", m.dirtyFrom, len(m.lines))
	}

	// 就地替换第 1 行并标记脏 → 只重算该行，缓存其余
	m.lines[1] = "replaced-line"
	m.invalidateLines(1)
	if m.dirtyFrom != 1 {
		t.Fatalf("dirtyFrom after invalidate = %d, want 1", m.dirtyFrom)
	}
	m.render()
	joined = strings.Join(m.wrappedLines, "\n")
	if !strings.Contains(joined, "replaced-line") {
		t.Fatalf("replaced line not rendered: %q", joined)
	}
	if strings.Contains(joined, "second") {
		t.Fatalf("stale content should be gone: %q", joined)
	}
	// 行 0 未变化，仍保留
	if !strings.Contains(joined, "first") {
		t.Fatalf("unchanged line should be reused: %q", joined)
	}

	// 宽度变化 → 全量失效重算
	m.viewport.SetWidth(10)
	m.render()
	if m.renderWidth != 10 {
		t.Fatalf("renderWidth = %d, want 10", m.renderWidth)
	}
	if m.dirtyFrom != len(m.lines) {
		t.Fatalf("dirtyFrom after width change = %d, want %d", m.dirtyFrom, len(m.lines))
	}
	if len(m.wrappedLines) == 0 {
		t.Fatal("render after width change should produce rows")
	}

	// 追加新行（append 场景）→ 新行被渲染，历史行缓存命中
	m.lines = append(m.lines, "third-line")
	m.render()
	joined = strings.Join(m.wrappedLines, "\n")
	if !strings.Contains(joined, "third-line") {
		t.Fatalf("appended line not rendered: %q", joined)
	}
}

// TestRenderedCacheClear 校验 /clear 清空历史后缓存同步清空。
func TestRenderedCacheClear(t *testing.T) {
	m := newRenderCacheModel(t)
	m.lines = []string{"aaa", "bbb"}
	m.render()
	if len(m.lineRendered) == 0 {
		t.Fatal("cache should be populated after render")
	}

	m.lines = nil
	m.lineRendered = nil
	m.dirtyFrom = 0
	m.render()
	if len(m.wrappedLines) != 0 {
		t.Fatalf("wrappedLines after clear = %d, want 0", len(m.wrappedLines))
	}
	if len(m.lineRendered) != 0 {
		t.Fatalf("lineRendered after clear = %d, want 0", len(m.lineRendered))
	}
}

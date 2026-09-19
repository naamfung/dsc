package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"dsc/userquestions"
)

// TestQuestionMsgDoesNotSyncViewportHeight 回归：questionMsg 处理时只调用
// m.render()，未调用 m.viewport.SetHeight(m.vpHeight()) 同步实际 viewport
// 高度。vpHeight() 函数本身计算正确（扣减覆盖层行数），但 m.viewport
// 内部缓存的 Height 仍是覆盖层激活前的旧值，导致 View() 输出多余行数，
// 把覆盖层推到终端可视区之外（AltScreen 截断）。
func TestQuestionMsgDoesNotSyncViewportHeight(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
	m.high = 30
	m.width = 80
	m.viewport = viewport.New(viewport.WithWidth(80), viewport.WithHeight(m.vpHeight()))
	m.viewport.SetWidth(80)
	m.viewport.SetHeight(m.vpHeight())
	m.ready = true
	m.input.SetWidth(72)

	before := m.vpHeight()
	// 模拟 WindowSizeMsg 同步 viewport 高度
	m.viewport.SetHeight(before)
	if m.viewport.Height() != before {
		t.Fatalf("setup: viewport.Height=%d, want %d", m.viewport.Height(), before)
	}

	ansCh := make(chan *userquestions.Answer, 1)
	errCh := make(chan error, 1)
	m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
		ID: "q", Question: "Approve?",
		Options: []userquestions.Option{
			{Label: "A"}, {Label: "B"}, {Label: "C"},
		},
	}}}, answer: ansCh, err: errCh})

	want := m.vpHeight() // 函数计算正确（已扣减覆盖层行数）
	if m.viewport.Height() != want {
		t.Fatalf("BUG: after questionMsg, viewport.Height=%d, want %d (vpHeight calc) → viewport renders %d extra rows → overlay clipped",
			m.viewport.Height(), want, m.viewport.Height()-want)
	}

	// 同时校验：覆盖层激活后，viewport 实际高度应 < 原始（因为要给覆盖层让位）
	if m.viewport.Height() >= before {
		t.Fatalf("viewport.Height=%d should be < before=%d (overlay should shrink viewport)", m.viewport.Height(), before)
	}

	// 清除问题时应恢复原始高度
	m.Update(questionMsg{})
	if m.viewport.Height() != before {
		t.Fatalf("after clear: viewport.Height=%d, want %d", m.viewport.Height(), before)
	}

	_ = strings.Count // silence unused import if test shrinks
}

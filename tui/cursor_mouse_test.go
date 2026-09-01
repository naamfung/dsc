package tui

import (
	"context"
	"testing"

	"charm.land/bubbletea/v2"
)

// TestViewAnchorsRealCursor 验证 SetVirtualCursor(false) 后 View 显式锚定真实光标：
// 修复前 viewOf 未设置 v.Cursor，输入框光标丢失。
func TestViewAnchorsRealCursor(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.Focus()

	v := m.View()
	if v.Cursor == nil {
		t.Fatal("View 应锚定真实光标（SetVirtualCursor(false) 后不锚定则输入框光标丢失）")
	}
	// Y：标题(1) + viewport + composer 顶边框(1)，输入框内容区第一行
	wantY := 1 + m.viewport.Height() + 1
	if v.Cursor.Y != wantY {
		t.Fatalf("View cursor Y = %d, want %d (viewport.Height=%d)", v.Cursor.Y, wantY, m.viewport.Height())
	}
	// X：prompt「❯ 」（2 列）+ 外层 composer 左侧 padding（1 列）
	if v.Cursor.X != 3 {
		t.Fatalf("View cursor X = %d, want 3", v.Cursor.X)
	}
}

// TestViewCursorTracksMultiLineInput 多行输入时光标仍落在输入框内容区内（首行 prompt 偏移）。
func TestViewCursorTracksMultiLineInput(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.Focus()
	m.input.SetValue("第一行\n第二行")
	m.input.CursorEnd()

	cur := m.inputCursorAbs()
	if cur == nil {
		t.Fatal("inputCursorAbs 不应返回 nil")
	}
	// 光标在第二行：Y = 标题 + viewport + 顶边框 + 1（第二行）
	wantY := 1 + m.viewport.Height() + 1 + 1
	if cur.Y != wantY {
		t.Fatalf("cursor Y = %d, want %d", cur.Y, wantY)
	}
	// X 随行内光标列变化：prompt(2) + 「第二行」3 个全角字符(6) + padding(1)
	if cur.X != 9 {
		t.Fatalf("cursor X = %d, want 9", cur.X)
	}
}

// TestViewMouseModeFollowsToggle 验证 /settings mouse on|off 切换释放鼠标（MouseModeNone），
// 恢复终端原生选中/复制（模型工作期间也可用）。
func TestViewMouseModeFollowsToggle(t *testing.T) {
	m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 默认：应用内鼠标捕获
	if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("默认 MouseMode = %v, want CellMotion", v.MouseMode)
	}

	// /settings mouse off → 释放
	handled, _ := m.runSlashCommand("/settings mouse off")
	if !handled {
		t.Fatal("/settings mouse off 应被 runSlashCommand 处理")
	}
	if !m.mouseCaptureOff {
		t.Fatal("off 后 mouseCaptureOff 应为 true")
	}
	if v := m.View(); v.MouseMode != tea.MouseModeNone {
		t.Fatalf("/settings mouse off 后 MouseMode = %v, want None", v.MouseMode)
	}

	// /settings mouse on → 恢复捕获
	handled, _ = m.runSlashCommand("/settings mouse on")
	if !handled {
		t.Fatal("/settings mouse on 应被处理")
	}
	if m.mouseCaptureOff {
		t.Fatal("on 后 mouseCaptureOff 应为 false")
	}
	if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("恢复后 MouseMode = %v, want CellMotion", v.MouseMode)
	}
}

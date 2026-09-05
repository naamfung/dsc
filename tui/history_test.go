package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestNavigateHistoryMultiLineReturnsToDraft 校验：翻到含换行的多行历史后，
// 按 ↓ 仍能翻回草稿位置。回归：旧实现以「输入框是否含换行」判定 ↑/↓ 是否翻历史，
// 导致翻到多行历史后 ↓ 被当成移动光标吞掉，永远回不到翻之前的状态。
func TestNavigateHistoryMultiLineReturnsToDraft(t *testing.T) {
	m := newRenderCacheModel(t)
	m.history = []string{"single-line", "multi\nline\nhistory"}
	m.histPos = len(m.history)
	m.draft = "my draft"
	m.input.SetValue(m.draft)

	// ↑ 进入历史，翻到多行消息
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.input.Value(); got != "multi\nline\nhistory" {
		t.Fatalf("按 ↑ 后输入框 = %q, want 多行历史", got)
	}

	// ↓ 应回到草稿位置（旧实现此处被吞为移动光标，输入框仍停在多行历史）
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.input.Value(); got != "my draft" {
		t.Fatalf("按 ↓ 后输入框 = %q, want 草稿 %q", got, "my draft")
	}

	// 回到草稿后再按 ↑ 仍能再次进入历史
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.input.Value(); got != "multi\nline\nhistory" {
		t.Fatalf("再次按 ↑ 后输入框 = %q, want 多行历史", got)
	}
}

// TestNavigateHistoryEditingExitsBrowse 校验：浏览历史时直接编辑输入内容，
// 即退出历史浏览（histPos 回到末尾），避免编辑后按 ↓ 把刚编辑的内容覆盖成下一条历史。
func TestNavigateHistoryEditingExitsBrowse(t *testing.T) {
	m := newRenderCacheModel(t)
	m.history = []string{"single", "multi\nline"}
	m.histPos = 1 // 正浏览第二条历史（多行）
	m.draft = "my draft"
	m.input.SetValue(m.history[1])
	m.input.Focus()

	// 输入字符 → 退出历史浏览，编辑内容保留
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.histPos != len(m.history) {
		t.Fatalf("编辑后 histPos = %d, want %d（应退出历史浏览）", m.histPos, len(m.history))
	}
	if !strings.Contains(m.input.Value(), "x") {
		t.Fatalf("编辑内容未保留: %q", m.input.Value())
	}

	// 退出浏览后按 ↓ 不应再把输入框内容覆盖成历史
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.input.Value(); !strings.Contains(got, "x") {
		t.Fatalf("按 ↓ 后编辑内容被覆盖: %q", got)
	}
}

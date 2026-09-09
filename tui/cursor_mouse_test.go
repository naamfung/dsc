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

// TestViewMouseModeAlwaysCapture 验证鼠标捕获策略：始终应用内捕获（CellMotion），
// 滚轮滚动、滚动条拖拽、正文拖选复制均内建工作；DSC_DISABLE_MOUSE（mouseCaptureOff）
// 永久释放给终端。
//
// 修复前：模型工作期间（thinking/streaming）会切到 MouseModeNone 把鼠标释放给终端，
// 但这导致滚轮事件被终端吃掉——鼠标在输入框上时滚的是 textarea 历史缓冲、在正文
// 区也无法翻阅历史。现统一保持 CellMotion：模型工作期间滚轮、滚动条拖拽均正常；
// 正文拖选复制在 streaming 期间由 MouseClickMsg 处理路径单独禁用（避免行号变化导致
// 选区漂移），与鼠标捕获开关解耦。
func TestViewMouseModeAlwaysCapture(t *testing.T) {
        m := New(&stubAgent{}, nil, context.Background(), "Agentic-Turbo-Coder", "minimal", 131072)
        m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

        // 空闲：应用内鼠标捕获
        if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
                t.Fatalf("空闲 MouseMode = %v, want CellMotion", v.MouseMode)
        }

        // 模型工作（thinking）：仍保持 CellMotion（滚轮、滚动条可用）
        m.thinking = true
        if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
                t.Fatalf("thinking 时 MouseMode = %v, want CellMotion (滚轮/滚动条应可用)", v.MouseMode)
        }
        m.thinking = false

        // 模型工作（streaming）：仍保持 CellMotion
        m.streaming = true
        if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
                t.Fatalf("streaming 时 MouseMode = %v, want CellMotion (滚轮/滚动条应可用)", v.MouseMode)
        }
        m.streaming = false

        // 工作结束：仍是 CellMotion（无变化）
        if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
                t.Fatalf("空闲恢复后 MouseMode = %v, want CellMotion", v.MouseMode)
        }

        // DSC_DISABLE_MOUSE 永久释放
        m.mouseCaptureOff = true
        if v := m.View(); v.MouseMode != tea.MouseModeNone {
                t.Fatalf("mouseCaptureOff 时 MouseMode = %v, want None", v.MouseMode)
        }
}

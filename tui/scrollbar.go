package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// 自定义滚动条（对齐 REX 的 transcript 滚动条）：viewport 内容区右侧保留一列，
// 滑块用主题 accent 实心块、轨道用暗灰竖线。内容不满一屏时不显示（thumb 为空）。
// 滚动条在 View() 层拼接（见 viewportView），拖拽交互见 MouseClick/Motion/Release。
var (
	scrollThumbStyle = lipgloss.NewStyle().Foreground(accent)
	scrollTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4A4A4A"))
)

// scrollbarThumb 返回滑块占 [start, start+size) 行（REX 同款算法）：
// 滑块高度按「可视窗 / 总内容」比例，位置随 YOffset 线性映射到轨道上。
func scrollbarThumb(height, yoff, total int) (start, size int) {
	if total <= height {
		return 0, 0 // 内容不满一屏 → 无滑块
	}
	size = height * height / total
	if size < 1 {
		size = 1
	}
	maxYoff := total - height
	start = yoff * (height - size) / maxYoff
	if start > height-size {
		start = height - size
	}
	return start, size
}

// scrollbarCell 返回滚动条列某行的字符：滑块行用 accent 实心块，轨道行用暗色竖线。
func scrollbarCell(row, total, height, thumbStart, thumbSize int) string {
	if total <= height {
		return " "
	}
	if row >= thumbStart && row < thumbStart+thumbSize {
		return scrollThumbStyle.Render("█")
	}
	return scrollTrackStyle.Render("│")
}

// viewportView 渲染 viewport 窗口，并在末列拼接自定义滚动条（颜色随主题）。
// viewport 自身宽度已减 1 保留滚动条列（见 WindowSizeMsg），其 View() 输出恰好
// width×height 行，逐行追加一个滚动条字符即完成拼接；内容不满时不显示。
func (m *Model) viewportView() string {
	v := m.viewport.View()
	h := m.viewport.Height()
	if h <= 0 {
		return v
	}
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	total := len(m.wrappedLines)
	yoff := m.viewport.YOffset()
	thumbStart, thumbSize := scrollbarThumb(h, yoff, total)
	for i, line := range lines {
		lines[i] = line + scrollbarCell(i, total, h, thumbStart, thumbSize)
	}
	return strings.Join(lines, "\n")
}

// inScrollbar 报告屏幕坐标 (x, y) 是否位于滚动条列。viewport 顶部位于屏幕第 1 行
// （标题占第 0 行），滚动条列 x == viewport.Width()（内容区右侧一列）。
func (m *Model) inScrollbar(x, y int) bool {
	h := m.viewport.Height()
	return h > 0 && y >= 1 && y < 1+h && x == m.viewport.Width()
}

// scrollbarGrabRowOffset 记录抓取点在滑块内的行偏移，拖拽时保持相对位置不跳变。
// row 为相对 viewport 顶的行号（屏幕 y - 1）。
func (m *Model) scrollbarGrabRowOffset(row int) int {
	thumbStart, thumbSize := scrollbarThumb(m.viewport.Height(), m.viewport.YOffset(), len(m.wrappedLines))
	if row >= thumbStart && row < thumbStart+thumbSize {
		return row - thumbStart
	}
	return thumbSize / 2
}

// dragScrollbar 把滚动条列行号映射为 viewport 偏移并应用（拖拽中调用）。
func (m *Model) dragScrollbar(row int) {
	m.viewport.SetYOffset(scrollbarYOffset(m.viewport.Height(), row, len(m.wrappedLines), m.scrollbarGrabOffset))
}

// scrollbarYOffset 由滚动条列行号反推 viewport 应滚动到的 YOffset。
// 这是 scrollbarThumb 映射的反向近似：两者都做整数取整，拖拽往返可能有 ±1 行
// 误差，属可接受范围（REX 同款算法）。
func scrollbarYOffset(height, row, total, grabOffset int) int {
	if total <= height {
		return 0
	}
	_, thumbSize := scrollbarThumb(height, 0, total)
	maxTop := height - thumbSize
	if maxTop <= 0 {
		return 0
	}
	top := row - grabOffset
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}
	maxYoff := total - height
	return (top*maxYoff + maxTop/2) / maxTop
}

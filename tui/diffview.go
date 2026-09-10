package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// 工具结果中的 unified diff 彩色渲染（对齐 REX 的 diffview）：加行绿底、删行红底，
// hunk 头与文件头暗色，上下文行原样。颜色取自 DSC 深色主题（深底配亮字，与 REX
// dark 主题的 diffAddBG/diffDelBG 一致观感）。
var (
	diffAddSty = lipgloss.NewStyle().Background(lipgloss.Color("#14351D")).Foreground(lipgloss.Color("#4ADE80"))
	diffDelSty = lipgloss.NewStyle().Background(lipgloss.Color("#3A1619")).Foreground(lipgloss.Color("#F87171"))
)

// diffBlockStart 定位结果文本中 unified diff 块的起始行号（0-based；未找到返回 -1）。
// 判定：存在「--- 」文件头行，其下一行为「+++ 」，且后续出现「@@ -」hunk 头，
// 避免把正文里偶然出现的 ---/+++ 误判为 diff。
func diffBlockStart(lines []string) int {
	for i := 0; i+1 < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "--- ") || !strings.HasPrefix(lines[i+1], "+++ ") {
			continue
		}
		for _, later := range lines[i+2:] {
			if strings.HasPrefix(later, "@@ -") {
				return i
			}
		}
	}
	return -1
}

// renderDiffLine 渲染一行 unified diff：加行绿底、删行红底、hunk 头/文件头/
// no-newline 标记暗色，上下文行原样。行首的 +/- 符号随行保留（着色整行）。
func renderDiffLine(ln string) string {
	switch {
	case strings.HasPrefix(ln, "@@") || strings.HasPrefix(ln, "\\"):
		return dimSty.Render(ln)
	case strings.HasPrefix(ln, "--- ") || strings.HasPrefix(ln, "+++ "):
		return dimSty.Render(ln)
	case strings.HasPrefix(ln, "+"):
		return diffAddSty.Render(ln)
	case strings.HasPrefix(ln, "-"):
		return diffDelSty.Render(ln)
	default:
		return ln
	}
}

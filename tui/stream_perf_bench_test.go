package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// benchmarkStreamBody 模拟流式正文单 token 到达时的完整渲染路径：
// renderMarkdown 全文 → joinReasoningAnswer → renderAssistant(indentBlock) →
// lipgloss 全块 wrap（buildWrappedLines 对末行执行的相同操作）。
func benchmarkStreamBody(b *testing.B, runes int) {
	raw := strings.Repeat("这是流式正文测试内容，包含**加粗**与 `代码` 片段。", runes/20)
	if len([]rune(raw)) < runes {
		raw += strings.Repeat("填充", (runes-len([]rune(raw)))/2)
	}
	// 预留全部 token，模拟逐 token 累加过程的平均成本（每次渲染当前累计长度）
	total := 0
	w := 80
	for i := 1; i <= b.N; i++ {
		body := renderMarkdown(raw, w)
		assistant := indentBlock(body, assistantGutter)
		_ = lipgloss.NewStyle().Width(w).Render(assistant)
		total += len([]rune(raw))
	}
	b.SetBytes(int64(total) / int64(b.N))
	_ = raw
}

func BenchmarkStreamBody1k(b *testing.B)  { benchmarkStreamBody(b, 1000) }
func BenchmarkStreamBody5k(b *testing.B)  { benchmarkStreamBody(b, 5000) }
func BenchmarkStreamBody10k(b *testing.B) { benchmarkStreamBody(b, 10000) }

// BenchmarkRenderMarkdown 单独测量 goldmark 渲染成本（不含 wrap/indent）。
func BenchmarkRenderMarkdown(b *testing.B) {
	raw := strings.Repeat("流式正文测试**加粗**与`代码`以及链接 https://example.com 内容填充。", 200)
	_ = fmt.Sprintf("%d", len(raw))
	for i := 0; i < b.N; i++ {
		_ = renderMarkdown(raw, 80)
	}
}

// BenchmarkLipglossWrap 单独测量对已按宽度 wrap 的正文再次 lipgloss 全块 wrap 的成本
// （buildWrappedLines 对流式末行执行的同一操作；由于正文已 ≤ 宽度，该 wrap 实为无改动）。
func BenchmarkLipglossWrap(b *testing.B) {
	raw := strings.Repeat("这是流式正文测试内容，包含**加粗**与 `代码` 片段，用于衡量末行全块 wrap 开销。", 60)
	body := renderMarkdown(raw, 76)
	for i := 0; i < b.N; i++ {
		_ = lipgloss.NewStyle().Width(80).Render(body)
	}
}

// BenchmarkIndentBlock 单独测量 indentBlock 对长正文的成本。
func BenchmarkIndentBlock(b *testing.B) {
	raw := strings.Repeat("这是流式正文测试内容，包含**加粗**与 `代码` 片段，用于衡量缩进开销。", 60)
	body := renderMarkdown(raw, 76)
	for i := 0; i < b.N; i++ {
		_ = indentBlock(body, assistantGutter)
	}
}

// BenchmarkStreamFrameCached 模拟采用增量 markdown 缓存后的真实单帧开销：
// 每帧 append 一段并走 cache.render + indent + 全块 lipgloss wrap（wrap 是剩余大头）。
func BenchmarkStreamFrameCached(b *testing.B) {
	md := newStreamMarkdown(76)
	raw := ""
	chunk := "这是流式正文测试段落，包含**加粗**与`代码`片段，用于衡量单帧渲染成本。\n\n"
	for i := 0; i < b.N; i++ {
		raw += chunk
		body := md.render(raw)
		assistant := indentBlock(body, assistantGutter)
		_ = lipgloss.NewStyle().Width(80).Render(assistant)
	}
}

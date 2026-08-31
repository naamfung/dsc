package tui

import "strings"

// mdSeg 是一段已稳定渲染的 markdown 分块：text 为渲染输出（无尾随换行），
// sep 为该块与下一块之间的空行分隔（"\n\n"）。goldmark 渲染器在每个块后写一个空行，
// 故拼接 seg.text+seg.sep 可还原「整段渲染」的输出。
type mdSeg struct {
	text string
	sep  string
}

// streamMarkdown 增量渲染流式 markdown 正文：以「空行且不在围栏代码块内」为安全切点，
// 把已稳定部分切成若干分块，每块只渲染一次并缓存；每帧仅重渲染未稳定尾部，
// 把每帧渲染成本从 O(全文) 降到 O(尾部)。该缓存只持有分块与尾部渲染结果，
// 原文由调用方（如 m.streamBuffer）持有并在 render 时传入。
// 最终提交（success/done）会做一次全量渲染，保证最终版式与逐 token 全量渲染一致；
// 增量缓存仅影响流式过程中的瞬时显示。
type streamMarkdown struct {
	width    int
	segs     []mdSeg
	cut      int    // 已被 segs 覆盖的原文字节长度
	tailRend string // 未稳定尾部原文的渲染输出（每帧重渲染）
}

// newStreamMarkdown 构造增量 markdown 渲染缓存。
func newStreamMarkdown(width int) *streamMarkdown {
	return &streamMarkdown{width: width}
}

// setWidth 更新渲染宽度；宽度变化时整体失效（旧分块按旧宽度渲染，不能混用）。
func (s *streamMarkdown) setWidth(w int) {
	if s.width != w {
		s.width = w
		s.reset()
	}
}

// render 以当前原文 raw 更新缓存并返回全量渲染输出：先固化可稳定的新增分块，
// 再重渲染未稳定尾部。raw 变短（如 /clear）时整体重置。
func (s *streamMarkdown) render(raw string) string {
	if len(raw) < s.cut {
		s.reset()
	}
	if cut := safeMarkdownCut(raw, s.cut); cut > s.cut {
		segRaw := raw[s.cut:cut]
		trimmed := strings.TrimRight(segRaw, "\n")
		if strings.TrimSpace(trimmed) != "" {
			s.segs = append(s.segs, mdSeg{text: renderMarkdown(trimmed, s.width), sep: "\n\n"})
		}
		s.cut = cut
	}
	s.tailRend = renderMarkdown(raw[s.cut:], s.width)
	return s.text()
}

// text 拼接缓存分块与尾部渲染，还原当前原文的全量渲染输出。
func (s *streamMarkdown) text() string {
	if len(s.segs) == 0 {
		return s.tailRend
	}
	var b strings.Builder
	for i := range s.segs {
		b.WriteString(s.segs[i].text)
		// 分隔只在「分块之间」或「末分块与尾部之间」出现；尾部无可视内容（空串或纯空白/未
		// 闭合围栏渲染为空）时省略末分隔，与 renderMarkdown 的 TrimRight 一致，避免块尾多空行。
		if i < len(s.segs)-1 || s.tailRend != "" {
			b.WriteString(s.segs[i].sep)
		}
	}
	b.WriteString(s.tailRend)
	return b.String()
}

// reset 清空缓存（新一轮/宽度变化/清屏）。
func (s *streamMarkdown) reset() {
	s.segs = nil
	s.cut = 0
	s.tailRend = ""
}

// safeMarkdownCut 返回 raw 中 ≥ from 的最大安全切点（字节下标）：切点位于「空行之后」且
// 不在围栏代码块内。切点处必然处于围栏外，供下次从该处继续扫描。无新切点时返回 from。
func safeMarkdownCut(raw string, from int) int {
	if from < 0 {
		from = 0
	}
	if from >= len(raw) {
		return len(raw)
	}
	best := from
	inFence := false
	pos := from
	for {
		nl := strings.IndexByte(raw[pos:], '\n')
		lineEnd := len(raw)
		if nl >= 0 {
			lineEnd = pos + nl
		}
		trimmed := strings.TrimSpace(raw[pos:lineEnd])
		switch {
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			inFence = !inFence
		case trimmed == "" && !inFence:
			// 空行是块边界：其后的位置可作为切点（该空行归入前一分块）。
			// 空行位于缓冲末尾时暂不切：它可能是段内软换行（单 \n）或分隔符只发了一半，
			// 需等后续内容确认真的是块边界。
			if lineEnd+1 < len(raw) {
				best = lineEnd + 1
			}
		}
		if nl < 0 {
			break
		}
		pos = lineEnd + 1
		if pos > len(raw) {
			break
		}
	}
	if best <= from {
		return from
	}
	return best
}

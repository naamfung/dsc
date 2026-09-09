package tui

import (
	"strings"
	"testing"
)

// TestStreamMarkdownEqualsFullWidth 增量渲染必须在流式过程任一中间态都与「整段全量渲染」
// 一致：最终版式不被增量缓存改变。覆盖多个段落、代码围栏、标题、列表、引用、加粗/代码行内。
func TestStreamMarkdownEqualsFullWidth(t *testing.T) {
	docs := []string{
		// 多段落（空行分隔 → 有稳定切点）
		"第一段：这是流式正文，包含 **加粗** 与 `行内代码`。\n第二行继续第一段内容，没有空行分隔。\n\n第二段：\n\n第三段：\n结尾。",
		// 代码围栏（围栏内空行不可作为切点）
		"```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n\nx := 1\n```\n\n围栏后的普通段落。\n\n再来一段。",
		// 标题与列表
		"## 标题一\n\n- 项一\n- 项二\n\n## 标题二\n正文。",
		// 引用块
		"> 引用第一行\n> 引用第二行\n\n普通段落。\n\n> 引用二\n",
		// 单行无空行（无切点 → 增量退化为整段重渲染但仍须一致）
		"只有一个超长段落没有空行分隔，" + strings.Repeat("填充文本", 200) + "。",
		// 块末尾未闭合围栏（尾部渲染为空时末分隔须省略，避免块尾多空行）
		"段落一。\n\n段落二。\n\n```go\nx := 1",
		// 块后带多余空行
		"段落一。\n\n段落二。\n\n\n\n",
		// 空串
		"",
	}
	const width = 60
	for _, doc := range docs {
		want := renderMarkdown(doc, width)

		md := newStreamMarkdown(width)
		var got string
		// 把原文按 rune 逐段追加（模拟逐 token 流式），每次都校验增量输出。
		var prev string
		for i, r := range doc {
			prev += string(r)
			if i%3 == 0 { // 每 3 个字符校验一次（覆盖多个中间态）
				got = md.render(prev)
				if got != renderMarkdown(prev, width) {
					t.Fatalf("增量渲染与全量不一致 @len=%d\n--- got ---\n%q\n--- want ---\n%q", len(prev), got, renderMarkdown(prev, width))
				}
			}
		}
		got = md.render(doc) // 最终态
		if got != want {
			t.Fatalf("最终态增量渲染与全量不一致\n--- got ---\n%q\n--- want ---\n%q", got, want)
		}
	}
}

// TestStreamReasoningEqualsFullRender 思考块增量渲染（markdown 缓存 + 暗色 ▎ 变换）在任一
// 中间态都必须与整段全量渲染逐字节一致：最终版式不被增量缓存改变。
func TestStreamReasoningEqualsFullRender(t *testing.T) {
	docs := []string{
		"思考过程：\n1. 用户发了好\n2. 需要回应策略\n\n**关键**与`代码`以及围栏：\n\n```go\nx := 1\n```\n\n更多思考。",
		// 思考块末尾未闭合围栏
		"思考一\n\n思考二\n\n```go\ncode",
		// 思考块带多余空行
		"思考一。\n\n思考二。\n\n\n",
	}
	const width = 60
	for _, doc := range docs {
		md := newStreamMarkdown(width)
		raw := ""
		for i, r := range doc {
			raw += string(r)
			if i%3 == 0 { // 每 3 个字符校验一次（覆盖多个中间态）
				got := renderReasoningRendered(md.render(raw))
				if got != renderReasoning(raw, width) {
					t.Fatalf("思考增量渲染与全量不一致 @len=%d\n--- got ---\n%q\n--- want ---\n%q", len(raw), got, renderReasoning(raw, width))
				}
			}
		}
		if got := renderReasoningRendered(md.render(raw)); got != renderReasoning(raw, width) {
			t.Fatalf("思考最终态增量渲染与全量不一致\n--- got ---\n%q\n--- want ---\n%q", got, renderReasoning(raw, width))
		}
	}
}

// TestStreamMarkdownCutPoints safeMarkdownCut 的切点必须在围栏外，且位于空行之后。
func TestStreamMarkdownCutPoints(t *testing.T) {
	cases := []struct {
		src  string
		from int
		want int
	}{
		// "para\n\n" 之后是首个稳定切点
		{"para\n\nnext", 0, 6},
		// 围栏内的空行不是稳定切点：只切到段一之后，围栏保持完整
		{"para\n\n```\ncode\n\ncode\n```", 0, 6},
		// 无空行 → 无切点（返回 from）
		{"para\nnext", 0, 0},
		// 多个空行段落取最后一个空行后的切点
		{"para\n\nnext\n\nend", 0, 12},
		// 末尾空行：尚未确认是块边界，暂不切（等后续内容）
		{"para\n\nnext\n\n", 0, 6},
		// from 已越过内容 → 返回 len(raw)
		{"para\n\nnext", 10, 10},
		// 围栏已关闭后的空行可切
		{"```\ncode\n```\n\npara", 0, 14},
	}
	for _, c := range cases {
		if got := safeMarkdownCut(c.src, c.from); got != c.want {
			t.Fatalf("safeMarkdownCut(%q, %d) = %d, want %d", c.src, c.from, got, c.want)
		}
	}
}

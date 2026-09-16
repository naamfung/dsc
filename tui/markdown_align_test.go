package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// 模型输出的 GFM 表格含 CJK：列宽按显示宽度（CJK 记 2 格）计算，
// 表头分隔线与数据行的右缘必须逐行对齐。
func TestRenderTableCJKRightEdgeAlignment(t *testing.T) {
	doc := "| 模块 | 职责 | 状态 |\n" +
		"|---|---|---|\n" +
		"| pipeline | 執行器、結果序列化 | 穩定 |\n" +
		"| optimizer | 雙向優化 | 開發中 |\n" +
		"| fidelity | Fidelity Search | 穩定 |\n"
	out := renderMarkdown(doc, 80)
	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("渲染行数不足: %q", out)
	}
	want := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		w := ansi.StringWidth(l)
		if want == -1 {
			want = w
			continue
		}
		if w != want {
			t.Fatalf("第 %d 行右缘错位: got=%d want=%d\nline=%q\nout=\n%s", i+1, w, want, l, out)
		}
	}
}

// 围栏代码块逐字透传（仅加固定「│ 」两格前缀）：TUI 不做任何宽度重排或补齐，
// 盒图右缘漂移系模型生成侧按「CJK 记 1 格」计数造成，渲染计算既不引入亦不放大。
func TestRenderFencedCodeBlockVerbatim(t *testing.T) {
	art := []string{
		"┌──────────────────┐",
		"│ pipeline（核心鏈路）     │",
		"│ 執行器、結果序列化      │",
		"└──────────────────┘",
	}
	src := "```\n" + strings.Join(art, "\n") + "\n```\n"
	out := renderMarkdown(src, 80)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(art) {
		t.Fatalf("应透传 %d 行, got %d: %q", len(art), len(lines), out)
	}
	for i, l := range lines {
		plain := ansi.Strip(l)
		if !strings.HasPrefix(plain, "│ ") {
			t.Fatalf("第 %d 行缺少固定前缀「│ 」: %q", i+1, plain)
		}
		if body := strings.TrimPrefix(plain, "│ "); body != art[i] {
			t.Fatalf("第 %d 行内容被改写\n got=%q\nwant=%q", i+1, body, art[i])
		}
	}
}

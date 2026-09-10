package tui

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// markdown 渲染器：把模型的 markdown 回答转成带 ANSI 样式的终端文本。
// 支持标题、段落、列表、围栏代码块、引用、表格、分隔线及行内样式（加粗/斜体/行内代码/链接）。
// 换行按 CJK 显示宽度处理（ansi.Wrap），ANSI 转义不占宽度。
var (
	mdAccentSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")) // 强调色：标题/标记/代码
	mdDimSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")) // 弱化色：引用/链接地址/表格线
	mdBoldSty   = lipgloss.NewStyle().Bold(true)
	mdItalicSty = lipgloss.NewStyle().Italic(true)
	mdCodeSty   = lipgloss.NewStyle().Foreground(lipgloss.Color("#05A5A5")) // 行内代码/代码块内容
)

func mdAccent(s string) string { return mdAccentSty.Render(s) }
func mdDim(s string) string    { return mdDimSty.Render(s) }
func mdBold(s string) string   { return mdBoldSty.Render(s) }
func mdItalic(s string) string { return mdItalicSty.Render(s) }

// mdParser 启用 GFM 表格扩展，使 | 行 | 列 | 被解析为表格节点。
var mdParser = goldmark.New(
	goldmark.WithExtensions(extension.Table),
)

// mdRender 是单次渲染的上下文。src 是解析用的源文本，代码块/行内节点的行段偏移都基于它。
type mdRender struct {
	width int
	src   []byte
}

// renderMarkdown 把 markdown 渲染为 ANSI 样式文本（尾部无多余换行）。
func renderMarkdown(raw string, width int) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if width < 8 {
		width = 8
	}
	raw = fixCJKEmphasis(raw)
	src := []byte(raw)
	doc := mdParser.Parser().Parse(text.NewReader(src))
	var b strings.Builder
	r := &mdRender{width: width, src: src}
	r.renderBlocks(&b, doc)
	return strings.TrimRight(b.String(), "\n")
}

func (r *mdRender) renderBlocks(buf *strings.Builder, parent ast.Node) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		r.renderBlock(buf, c)
	}
}

func (r *mdRender) renderBlock(buf *strings.Builder, node ast.Node) {
	switch n := node.(type) {
	case *ast.Heading:
		r.renderHeading(buf, n)
	case *ast.Paragraph:
		r.renderInlineBlock(buf, n, true)
	case *ast.TextBlock:
		// TextBlock 是紧凑列表项的行内容器，不需要段后空行。
		r.renderInlineBlock(buf, n, false)
	case *ast.List:
		r.renderList(buf, n)
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		r.renderFenced(buf, n)
	case *ast.Blockquote:
		r.renderBlockquote(buf, n)
	case *extast.Table:
		r.renderTable(buf, n)
	case *ast.ThematicBreak:
		buf.WriteString(mdDim(strings.Repeat("─", r.width)))
		buf.WriteString("\n\n")
	default:
		// 未知块：下钻到子节点而不是丢内容。
		r.renderBlocks(buf, node)
	}
}

func (r *mdRender) renderHeading(buf *strings.Builder, n *ast.Heading) {
	inline := r.collectInline(n)
	buf.WriteString(mdBold(mdAccent(inline)))
	buf.WriteString("\n")
	// 一级标题加下划线；更深层级靠加粗+颜色区分。
	if n.Level == 1 {
		buf.WriteString(mdAccent(strings.Repeat("─", ansi.StringWidth(inline))))
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
}

func (r *mdRender) renderInlineBlock(buf *strings.Builder, n ast.Node, trailingBlank bool) {
	inline := r.collectInline(n)
	wrapped := ansi.Wrap(inline, r.width, "")
	for _, line := range strings.Split(wrapped, "\n") {
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	if trailingBlank {
		buf.WriteString("\n")
	}
}

func (r *mdRender) renderList(buf *strings.Builder, n *ast.List) {
	idx := 1
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		var marker string
		if n.IsOrdered() {
			marker = fmt.Sprintf("%d.", idx)
			idx++
		} else {
			marker = "•"
		}
		markerW := ansi.StringWidth(marker) + 1

		first := item.FirstChild()
		host := inlineCarrier(first)
		if host != nil {
			inline := r.collectInline(host)
			wrapped := ansi.Wrap(inline, max(r.width-markerW, 4), "")
			lines := strings.Split(wrapped, "\n")
			buf.WriteString(mdAccent(marker) + " " + lines[0] + "\n")
			for _, l := range lines[1:] {
				buf.WriteString(strings.Repeat(" ", markerW) + l + "\n")
			}
			for s := first.NextSibling(); s != nil; s = s.NextSibling() {
				r.renderBlock(buf, s)
			}
		} else {
			buf.WriteString(mdAccent(marker) + "\n")
			r.renderBlocks(buf, item)
		}
	}
	buf.WriteString("\n")
}

func (r *mdRender) renderFenced(buf *strings.Builder, n ast.Node) {
	switch v := n.(type) {
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		for i := 0; i < v.Lines().Len(); i++ {
			l := v.Lines().At(i)
			line := strings.TrimRight(string(l.Value(r.src)), "\n")
			buf.WriteString(mdDim("│ ") + mdCodeSty.Render(line) + "\n")
		}
	}
	buf.WriteString("\n")
}

func (r *mdRender) renderBlockquote(buf *strings.Builder, n *ast.Blockquote) {
	var inner strings.Builder
	r.renderBlocks(&inner, n)
	for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
		buf.WriteString(mdDim("▎ ") + mdDim(line) + "\n")
	}
	buf.WriteString("\n")
}

// collectInline 遍历行内子树并返回带 ANSI 样式的扁平文本。
func (r *mdRender) collectInline(n ast.Node) string {
	var b strings.Builder
	r.appendInline(&b, n)
	return b.String()
}

func (r *mdRender) appendInline(b *strings.Builder, n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(r.src))
			switch {
			case v.HardLineBreak():
				b.WriteByte('\n')
			case v.SoftLineBreak():
				b.WriteByte(' ')
			}
		case *ast.Emphasis:
			var inner strings.Builder
			r.appendInline(&inner, v)
			if v.Level == 2 {
				b.WriteString(mdBold(inner.String()))
			} else {
				b.WriteString(mdItalic(inner.String()))
			}
		case *ast.CodeSpan:
			var inner strings.Builder
			r.appendInline(&inner, v)
			b.WriteString(mdCodeSty.Render(inner.String()))
		case *ast.Link:
			var inner strings.Builder
			r.appendInline(&inner, v)
			b.WriteString(inner.String())
			b.WriteString(mdDim(" (" + string(v.Destination) + ")"))
		case *ast.AutoLink:
			b.WriteString(string(v.URL(r.src)))
		case *ast.RawHTML:
			// 丢弃：聊天输出中少见，原样输出只会打出转义字面量。
		case *ast.String:
			b.Write(v.Value)
		default:
			r.appendInline(b, c)
		}
	}
}

// renderTable 把 GFM 表格排版为终端列：列宽自适应最宽单元格并封顶为终端宽度的公平份额，
// 单元格超宽时按行折行而非截断，保证内容不丢失。
func (r *mdRender) renderTable(buf *strings.Builder, n *extast.Table) {
	var header []string
	var rows [][]string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *extast.TableHeader:
			header = r.collectCells(row)
		case *extast.TableRow:
			rows = append(rows, r.collectCells(row))
		}
	}
	if len(header) == 0 && len(rows) == 0 {
		return
	}
	cols := len(header)
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return
	}

	widths := make([]int, cols)
	pick := func(i, w int) {
		if i < cols && w > widths[i] {
			widths[i] = w
		}
	}
	for i, h := range header {
		pick(i, ansi.StringWidth(h))
	}
	for _, row := range rows {
		for i, c := range row {
			pick(i, ansi.StringWidth(c))
		}
	}

	// 列宽按比例收缩以适配终端宽度（含 3 字符列分隔与空隙）。
	available := r.width - 3*(cols-1)
	if available < cols*3 {
		available = cols * 3
	}
	total := 0
	for _, w := range widths {
		total += w
	}
	if total > available {
		for i := range widths {
			widths[i] = widths[i] * available / total
			if widths[i] < 3 {
				widths[i] = 3
			}
		}
	}

	sep := mdDim(" │ ")
	if len(header) > 0 {
		r.renderTableRow(buf, sep, header, widths, true)
		for i := range widths {
			if i > 0 {
				buf.WriteString(mdDim("─┼─"))
			}
			buf.WriteString(mdDim(strings.Repeat("─", widths[i])))
		}
		buf.WriteByte('\n')
	}
	for _, row := range rows {
		r.renderTableRow(buf, sep, row, widths, false)
	}
	buf.WriteByte('\n')
}

// renderTableRow 把一个逻辑行铺成多行视觉行（单元格折行时行高取最高单元格）。
func (r *mdRender) renderTableRow(buf *strings.Builder, sep string, cells []string, widths []int, isHeader bool) {
	cols := len(widths)
	wrapped := make([][]string, cols)
	maxLines := 1
	for i := 0; i < cols; i++ {
		var text string
		if i < len(cells) {
			text = cells[i]
		}
		wrapped[i] = strings.Split(ansi.Wrap(text, widths[i], ""), "\n")
		if len(wrapped[i]) > maxLines {
			maxLines = len(wrapped[i])
		}
	}
	for line := 0; line < maxLines; line++ {
		for i := 0; i < cols; i++ {
			if i > 0 {
				buf.WriteString(sep)
			}
			var cell string
			if line < len(wrapped[i]) {
				cell = wrapped[i][line]
			}
			padded := padRight(cell, widths[i])
			if isHeader {
				padded = mdBold(padded)
			}
			buf.WriteString(padded)
		}
		buf.WriteByte('\n')
	}
}

// collectCells 抽取表头/表行各单元格的行内内容。
func (r *mdRender) collectCells(parent ast.Node) []string {
	var out []string
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*extast.TableCell); ok {
			out = append(out, strings.TrimSpace(r.collectInline(cell)))
		}
	}
	return out
}

// inlineCarrier 返回承载行内内容的段落/文本块节点（列表标记行的载体）。
func inlineCarrier(n ast.Node) ast.Node {
	switch n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return n
	}
	return nil
}

// padRight 把 s 右补空格至占 w 个终端列（按显示宽度而非字节数）。
func padRight(s string, w int) string {
	pad := w - ansi.StringWidth(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

// fixCJKEmphasis 修正 goldmark 对 CJK 标点的加粗边界判定：**X，**Y 的闭合 **
// 在 CJK 标点后不会被识别为右闭合，需在闭合符后补一个空格。
func fixCJKEmphasis(s string) string {
	runes := []rune(s)
	n := len(runes)
	var b strings.Builder
	b.Grow(len(s) + 16)

	inFenced := false
	inCode := false
	inEmphasis := false

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == '`' && i+2 < n && runes[i+1] == '`' && runes[i+2] == '`' {
			inFenced = !inFenced
			b.WriteString("```")
			i += 2
			continue
		}
		if r == '`' && !inFenced {
			inCode = !inCode
			b.WriteRune(r)
			continue
		}
		if inCode || inFenced {
			b.WriteRune(r)
			continue
		}
		if r == '\n' {
			inEmphasis = false
			b.WriteRune(r)
			continue
		}
		if r == '*' && i+1 < n && runes[i+1] == '*' {
			b.WriteString("**")
			i++
			inEmphasis = !inEmphasis
			if !inEmphasis && i >= 2 && !isSpace(runes[i-2]) && isCJKPunct(runes[i-2]) {
				b.WriteByte(' ')
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isCJKPunct(r rune) bool {
	if r <= 0x7F {
		return false
	}
	switch {
	case r >= 0x3000 && r <= 0x303F:
		return true
	case r >= 0xFF01 && r <= 0xFF0F:
		return true
	case r >= 0xFF1A && r <= 0xFF20:
		return true
	case r >= 0xFF3B && r <= 0xFF3F:
		return true
	case r >= 0xFF5B && r <= 0xFF65:
		return true
	}
	return unicode.IsPunct(r)
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

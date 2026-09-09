package diag

import (
	"fmt"
	"strconv"
	"strings"
)

// SourceLines holds source code split by lines for rendering diagnostics.
//
// Lines are 0-indexed internally, but GetLine accepts 1-indexed line numbers
// to match Position.Line convention.
type SourceLines []string

// GetLine returns the 1-indexed line, or empty string if out of bounds.
func (s SourceLines) GetLine(n int) string {
	if n < 1 || n > len(s) {
		return ""
	}
	return s[n-1]
}

// ParseSource splits source code into lines for diagnostic rendering.
func ParseSource(source string) SourceLines {
	return strings.Split(source, "\n")
}

type colorScheme struct {
	reset, bold, error, warning, hint, gutter string
}

var noColors = colorScheme{}

var ansiColors = colorScheme{
	reset:   "\033[0m",
	bold:    "\033[1m",
	error:   "\033[1;31m",
	warning: "\033[1;33m",
	hint:    "\033[1;36m",
	gutter:  "\033[1;34m",
}

const tabWidth = 8

// Render formats the diagnostic in Rust-style without colors.
//
// The output format includes:
//   - Severity and error code header
//   - Source location with file:line:column
//   - Source line with underline highlighting
//   - Secondary labels (if any)
//   - Explanation note (if set)
//   - Help suggestion (if set)
func (d Diagnostic) Render(source SourceLines) string {
	return d.render(source, noColors)
}

// RenderColored formats the diagnostic in Rust-style with ANSI colors.
//
// Uses ANSI escape codes for terminal color output:
//   - Red for errors
//   - Yellow for warnings
//   - Cyan for hints
//   - Blue for line numbers and markers
func (d Diagnostic) RenderColored(source SourceLines) string {
	return d.render(source, ansiColors)
}

func (d Diagnostic) render(source SourceLines, c colorScheme) string {
	var b strings.Builder

	severityColor := c.error
	severityName := "error"

	switch d.Severity {
	case SeverityWarning:
		severityColor = c.warning
		severityName = "warning"
	case SeverityHint:
		severityColor = c.hint
		severityName = "hint"
	}

	b.WriteString(severityColor)
	b.WriteString(severityName)
	b.WriteString("[")
	b.WriteString(d.Code.Name())
	b.WriteString("]")
	b.WriteString(c.reset)
	b.WriteString(c.bold)
	b.WriteString(": ")
	b.WriteString(d.Message)
	b.WriteString(c.reset)
	b.WriteString("\n")

	lineRaw := source.GetLine(d.Position.Line)
	line := expandTabs(lineRaw)
	lineNumStr := strconv.Itoa(d.Position.Line)
	gutterWidth := len(lineNumStr) + 1
	gutter := strings.Repeat(" ", gutterWidth)

	b.WriteString(gutter)
	b.WriteString(c.gutter)
	b.WriteString("--> ")
	b.WriteString(c.reset)

	if d.Position.File != "" {
		b.WriteString(d.Position.File)
		b.WriteString(":")
	}

	b.WriteString(strconv.Itoa(d.Position.Line))
	b.WriteString(":")
	b.WriteString(strconv.Itoa(d.Position.Column))
	b.WriteString("\n")

	if line != "" {
		b.WriteString(gutter)
		b.WriteString(c.gutter)
		b.WriteString("|")
		b.WriteString(c.reset)
		b.WriteString("\n")

		b.WriteString(c.gutter)
		b.WriteString(fmt.Sprintf("%*d | ", gutterWidth-1, d.Position.Line))
		b.WriteString(c.reset)
		b.WriteString(line)
		b.WriteString("\n")

		b.WriteString(gutter)
		b.WriteString(c.gutter)
		b.WriteString("| ")
		b.WriteString(c.reset)

		col := displayColumn(lineRaw, d.Position.Column)
		if col < 1 {
			col = 1
		}

		b.WriteString(strings.Repeat(" ", col-1))

		underlineLen := 1

		if d.Span.Valid() && d.Span.SingleLine() {
			startCol := displayColumn(lineRaw, d.Span.StartCol)
			endCol := displayColumn(lineRaw, d.Span.EndCol)
			if endCol >= startCol {
				underlineLen = endCol - startCol + 1
			} else if col <= len(line) {
				underlineLen = len(line) - col + 1
			}
		}

		b.WriteString(severityColor)
		b.WriteString("^")

		if underlineLen > 1 {
			b.WriteString(strings.Repeat("~", underlineLen-1))
		}

		b.WriteString(c.reset)
		b.WriteString("\n")
	}

	for _, label := range d.Labels {
		if label.Span.StartLine > 0 {
			labelLineRaw := source.GetLine(label.Span.StartLine)
			labelLine := expandTabs(labelLineRaw)
			if labelLine != "" {
				b.WriteString(c.gutter)
				b.WriteString(fmt.Sprintf("%*d | ", gutterWidth-1, label.Span.StartLine))
				b.WriteString(c.reset)
				b.WriteString(labelLine)
				b.WriteString("\n")
				b.WriteString(gutter)
				b.WriteString(c.gutter)
				b.WriteString("| ")
				b.WriteString(c.reset)

				if label.Span.StartCol > 0 {
					labelCol := displayColumn(labelLineRaw, label.Span.StartCol)
					if labelCol < 1 {
						labelCol = 1
					}
					b.WriteString(strings.Repeat(" ", labelCol-1))
				}

				b.WriteString(c.hint)
				b.WriteString("- ")
				b.WriteString(label.Message)
				b.WriteString(c.reset)
				b.WriteString("\n")
			}
		}
	}

	if d.Explanation != "" {
		b.WriteString(gutter)
		b.WriteString(c.bold)
		b.WriteString("= note: ")
		b.WriteString(c.reset)
		b.WriteString(d.Explanation)
		b.WriteString("\n")
	}

	if d.Help != "" {
		b.WriteString(gutter)
		b.WriteString(c.bold)
		b.WriteString("= help: ")
		b.WriteString(c.reset)
		b.WriteString(d.Help)
		b.WriteString("\n")
	}

	return b.String()
}

func expandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	var b strings.Builder
	col := 1
	for _, r := range line {
		if r == '\t' {
			spaces := tabWidth - ((col - 1) % tabWidth)
			b.WriteString(strings.Repeat(" ", spaces))
			col += spaces
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

func displayColumn(line string, col int) int {
	if col < 1 {
		return 1
	}
	if !strings.Contains(line, "\t") {
		return col
	}
	display := 1
	current := 1
	for _, r := range line {
		if current == col {
			return display
		}
		if r == '\t' {
			display += tabWidth - ((display - 1) % tabWidth)
		} else {
			display++
		}
		current++
	}
	return display
}

// RenderAll formats all diagnostics without colors.
func (c *Collector) RenderAll(source SourceLines) string {
	var b strings.Builder
	for _, d := range c.All() {
		b.WriteString(d.Render(source))
		b.WriteString("\n")
	}

	return b.String()
}

// RenderAllColored formats all diagnostics with colors.
func (c *Collector) RenderAllColored(source SourceLines) string {
	var b strings.Builder
	for _, d := range c.All() {
		b.WriteString(d.RenderColored(source))
		b.WriteString("\n")
	}

	return b.String()
}

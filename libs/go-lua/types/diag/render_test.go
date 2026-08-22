package diag

import (
	"strings"
	"testing"
)

func TestParseSource(t *testing.T) {
	source := "line1\nline2\nline3"
	lines := ParseSource(source)

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	if lines[0] != "line1" {
		t.Errorf("line 0: got %q, want %q", lines[0], "line1")
	}
}

func TestSourceLinesGetLine(t *testing.T) {
	lines := ParseSource("a\nb\nc")

	if got := lines.GetLine(1); got != "a" {
		t.Errorf("GetLine(1) = %q, want %q", got, "a")
	}

	if got := lines.GetLine(2); got != "b" {
		t.Errorf("GetLine(2) = %q, want %q", got, "b")
	}

	if got := lines.GetLine(3); got != "c" {
		t.Errorf("GetLine(3) = %q, want %q", got, "c")
	}
}

func TestSourceLinesGetLineOutOfBounds(t *testing.T) {
	lines := ParseSource("a\nb")

	if got := lines.GetLine(0); got != "" {
		t.Errorf("GetLine(0) should be empty, got %q", got)
	}

	if got := lines.GetLine(5); got != "" {
		t.Errorf("GetLine(5) should be empty, got %q", got)
	}

	if got := lines.GetLine(-1); got != "" {
		t.Errorf("GetLine(-1) should be empty, got %q", got)
	}
}

func TestDiagnosticRender(t *testing.T) {
	source := ParseSource("local x = 1 + \"hello\"")
	d := Diagnostic{
		Code:     ErrTypeMismatch,
		Severity: SeverityError,
		Message:  "cannot add number and string",
		Position: Position{Line: 1, Column: 15},
	}

	output := d.Render(source)

	if !strings.Contains(output, "error[E0000]") {
		t.Error("should contain error code")
	}

	if !strings.Contains(output, "cannot add number and string") {
		t.Error("should contain message")
	}

	if !strings.Contains(output, "1:15") {
		t.Error("should contain line:column")
	}

	if !strings.Contains(output, "^") {
		t.Error("should contain caret marker")
	}
}

func TestDiagnosticRenderWithFile(t *testing.T) {
	source := ParseSource("local x = nil")
	d := Diagnostic{
		Code:     ErrUndefined,
		Severity: SeverityError,
		Message:  "undefined variable",
		Position: Position{File: "test.lua", Line: 1, Column: 7},
	}

	output := d.Render(source)

	if !strings.Contains(output, "test.lua:1:7") {
		t.Error("should contain file:line:column")
	}
}

func TestDiagnosticRenderWarning(t *testing.T) {
	source := ParseSource("local unused = 1")
	d := Diagnostic{
		Code:     ErrUndefined,
		Severity: SeverityWarning,
		Message:  "unused variable",
		Position: Position{Line: 1, Column: 7},
	}

	output := d.Render(source)

	if !strings.HasPrefix(output, "warning[") {
		t.Error("should start with warning")
	}
}

func TestDiagnosticRenderHint(t *testing.T) {
	source := ParseSource("local x = 1")
	d := Diagnostic{
		Code:     ErrUndefined,
		Severity: SeverityHint,
		Message:  "consider using _",
		Position: Position{Line: 1, Column: 7},
	}

	output := d.Render(source)

	if !strings.HasPrefix(output, "hint[") {
		t.Error("should start with hint")
	}
}

func TestDiagnosticRenderWithExplanation(t *testing.T) {
	source := ParseSource("local x = y")
	d := Diagnostic{
		Code:        ErrUndefined,
		Severity:    SeverityError,
		Message:     "undefined 'y'",
		Explanation: "declare y before use",
		Position:    Position{Line: 1, Column: 11},
	}

	output := d.Render(source)

	if !strings.Contains(output, "= note:") {
		t.Error("should contain note prefix")
	}

	if !strings.Contains(output, "declare y before use") {
		t.Error("should contain explanation")
	}
}

func TestDiagnosticRenderWithSpan(t *testing.T) {
	source := ParseSource("local x = longword")
	d := Diagnostic{
		Code:     ErrUndefined,
		Severity: SeverityError,
		Message:  "undefined",
		Position: Position{Line: 1, Column: 11},
		Span:     Span{StartLine: 1, StartCol: 11, EndLine: 1, EndCol: 19},
	}

	output := d.Render(source)

	// Should have underline for span length
	if !strings.Contains(output, "^") {
		t.Error("should contain caret")
	}

	if !strings.Contains(output, "^~") {
		t.Error("should contain extended underline")
	}
}

func TestDiagnosticRenderUnderlineToLineEnd(t *testing.T) {
	source := ParseSource("local x = y")
	d := Diagnostic{
		Code:     ErrUndefined,
		Severity: SeverityError,
		Message:  "undefined",
		Position: Position{Line: 1, Column: 9},
		Span:     Span{StartLine: 1, StartCol: 9, EndLine: 1, EndCol: 0},
	}

	output := d.Render(source)

	if !strings.Contains(output, "^~") {
		t.Error("should underline to end of line when span has no end column")
	}
}

func TestDiagnosticRenderColored(t *testing.T) {
	source := ParseSource("local x = nil")
	d := Diagnostic{
		Code:     ErrTypeMismatch,
		Severity: SeverityError,
		Message:  "type error",
		Position: Position{Line: 1, Column: 1},
	}

	output := d.RenderColored(source)

	// Should contain ANSI escape sequences
	if !strings.Contains(output, "\033[") {
		t.Error("colored output should contain ANSI codes")
	}
}

func TestCollectorRenderAll(t *testing.T) {
	source := ParseSource("line1\nline2")
	c := NewCollector("test.lua")
	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error 1")
	c.Add(mockNode{line: 2}, ErrUndefined, "error 2")

	output := c.RenderAll(source)

	if !strings.Contains(output, "error 1") {
		t.Error("should contain first error")
	}

	if !strings.Contains(output, "error 2") {
		t.Error("should contain second error")
	}
}

func TestCollectorRenderAllColored(t *testing.T) {
	source := ParseSource("line1\nline2")
	c := NewCollector("test.lua")
	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error 1")
	c.Add(mockNode{line: 2}, ErrUndefined, "error 2")

	output := c.RenderAllColored(source)

	if !strings.Contains(output, "\033[") {
		t.Error("colored output should contain ANSI codes")
	}

	if !strings.Contains(output, "error 1") {
		t.Error("should contain first error")
	}

	if !strings.Contains(output, "error 2") {
		t.Error("should contain second error")
	}
}

func TestDiagnosticRenderMultipleLabels(t *testing.T) {
	source := ParseSource("local x = y")
	d := Diagnostic{
		Code:        ErrUndefined,
		Severity:    SeverityError,
		Message:     "undefined 'y'",
		Explanation: "variable not declared",
		Position:    Position{Line: 1, Column: 11},
	}

	output := d.Render(source)

	if !strings.Contains(output, "= note:") {
		t.Error("should contain note prefix")
	}

	if !strings.Contains(output, "variable not declared") {
		t.Error("should contain explanation text")
	}
}

func TestDiagnosticRenderWithLabels(t *testing.T) {
	source := ParseSource("local x = y + z")
	d := Diagnostic{
		Code:     ErrTypeMismatch,
		Severity: SeverityError,
		Message:  "type mismatch",
		Position: Position{Line: 1, Column: 11},
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 11, EndLine: 1, EndCol: 12}, Message: "this is number"},
			{Span: Span{StartLine: 1, StartCol: 15, EndLine: 1, EndCol: 16}, Message: "this is string"},
		},
	}

	output := d.Render(source)

	if !strings.Contains(output, "this is number") {
		t.Error("should contain first label")
	}

	if !strings.Contains(output, "this is string") {
		t.Error("should contain second label")
	}
}

func TestDiagnosticRenderColoredWithExplanation(t *testing.T) {
	source := ParseSource("local x = y")
	d := Diagnostic{
		Code:        ErrUndefined,
		Severity:    SeverityError,
		Message:     "undefined 'y'",
		Explanation: "variable not declared",
		Help:        "declare the variable first",
		Position:    Position{Line: 1, Column: 11},
	}

	output := d.RenderColored(source)

	if !strings.Contains(output, "\033[") {
		t.Error("colored output should contain ANSI codes")
	}

	if !strings.Contains(output, "variable not declared") {
		t.Error("should contain explanation")
	}
}

func TestDiagnosticRenderColoredWithLabels(t *testing.T) {
	source := ParseSource("local x = y + z")
	d := Diagnostic{
		Code:     ErrTypeMismatch,
		Severity: SeverityError,
		Message:  "type mismatch",
		Position: Position{Line: 1, Column: 11},
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 11, EndLine: 1, EndCol: 12}, Message: "this is number"},
		},
	}

	output := d.RenderColored(source)

	if !strings.Contains(output, "\033[") {
		t.Error("colored output should contain ANSI codes")
	}

	if !strings.Contains(output, "this is number") {
		t.Error("should contain label")
	}
}

func TestDiagnosticRenderEmptySource(t *testing.T) {
	source := ParseSource("")
	d := Diagnostic{
		Code:     ErrTypeMismatch,
		Severity: SeverityError,
		Message:  "error",
		Position: Position{Line: 1, Column: 1},
	}

	output := d.Render(source)

	if !strings.Contains(output, "error") {
		t.Error("should still contain message")
	}
}

func TestDiagnosticRenderColoredWarning(t *testing.T) {
	source := ParseSource("local x = 1")
	d := Diagnostic{
		Code:     ErrUnreachable,
		Severity: SeverityWarning,
		Message:  "unreachable",
		Position: Position{Line: 1, Column: 1},
	}

	output := d.RenderColored(source)

	if !strings.Contains(output, "\033[1;33m") {
		t.Error("warning should use bold yellow color code")
	}
}

func TestDiagnosticRenderColoredHint(t *testing.T) {
	source := ParseSource("local x = 1")
	d := Diagnostic{
		Code:     ErrUndefined,
		Severity: SeverityHint,
		Message:  "hint",
		Position: Position{Line: 1, Column: 1},
	}

	output := d.RenderColored(source)

	if !strings.Contains(output, "\033[1;36m") {
		t.Error("hint should use bold cyan color code")
	}
}

package diag

import (
	"strings"
	"testing"
)

func TestSeverityString(t *testing.T) {
	tests := []struct {
		s    Severity
		want string
	}{
		{SeverityError, "error"},
		{SeverityWarning, "warning"},
		{SeverityHint, "hint"},
		{Severity(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestSeverityOrdering(t *testing.T) {
	if SeverityError >= SeverityWarning {
		t.Error("SeverityError should be less than SeverityWarning")
	}
	if SeverityWarning >= SeverityHint {
		t.Error("SeverityWarning should be less than SeverityHint")
	}
}

func TestPosition_String(t *testing.T) {
	tests := []struct {
		pos  Position
		want string
	}{
		{Position{File: "test.lua", Line: 10, Column: 5}, "test.lua:10:5"},
		{Position{Line: 10, Column: 5}, "10:5"},
		{Position{File: "", Line: 1, Column: 1}, "1:1"},
	}

	for _, tt := range tests {
		if got := tt.pos.String(); got != tt.want {
			t.Errorf("Position.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestSpan_Valid(t *testing.T) {
	tests := []struct {
		span Span
		want bool
	}{
		{Span{StartLine: 1, StartCol: 1}, true},
		{Span{StartLine: 0, StartCol: 1}, false},
		{Span{StartLine: 1, StartCol: 0}, false},
		{Span{}, false},
	}

	for _, tt := range tests {
		if got := tt.span.Valid(); got != tt.want {
			t.Errorf("Span.Valid() = %v, want %v for %+v", got, tt.want, tt.span)
		}
	}
}

func TestSpan_SingleLine(t *testing.T) {
	tests := []struct {
		span Span
		want bool
	}{
		{Span{StartLine: 1, EndLine: 1}, true},
		{Span{StartLine: 1, EndLine: 0}, true},
		{Span{StartLine: 1, EndLine: 2}, false},
	}

	for _, tt := range tests {
		if got := tt.span.SingleLine(); got != tt.want {
			t.Errorf("Span.SingleLine() = %v, want %v for %+v", got, tt.want, tt.span)
		}
	}
}

func TestCodeName(t *testing.T) {
	tests := []struct {
		code Code
		want string
	}{
		{ErrTypeMismatch, "E0000"},
		{ErrUndefined, "E0001"},
		{ErrNotCallable, "E0002"},
		{ErrInvalidOperand, "E0019"},
	}

	for _, tt := range tests {
		if got := tt.code.Name(); got != tt.want {
			t.Errorf("Code(%d).Name() = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeNameFormat(t *testing.T) {
	for c := ErrTypeMismatch; c <= ErrInvalidOperand; c++ {
		name := c.Name()
		if !strings.HasPrefix(name, "E") {
			t.Errorf("Code(%d).Name() = %q, should start with E", c, name)
		}
		if len(name) != 5 {
			t.Errorf("Code(%d).Name() = %q, should be 5 chars", c, name)
		}
	}
}

func TestCodeInfo(t *testing.T) {
	info := ErrTypeMismatch.Info()
	if info.Title == "" {
		t.Error("ErrTypeMismatch should have a title")
	}
	if info.Explanation == "" {
		t.Error("ErrTypeMismatch should have an explanation")
	}
}

func TestCodeInfoKnownCodes(t *testing.T) {
	knownCodes := []Code{
		ErrTypeMismatch, ErrUndefined, ErrNotCallable,
		ErrWrongArity, ErrNoField, ErrNotIndexable,
		ErrReadonly, ErrMissingReturn, ErrNonExhaustive,
		ErrUseBeforeAssign, ErrDuplicateDeclaration,
		ErrInvalidIndexType, ErrInvalidOperand,
	}

	for _, c := range knownCodes {
		info := c.Info()
		if info.Title == "" || info.Title == "error" {
			t.Errorf("Code(%d) should have specific title, got %q", c, info.Title)
		}
	}
}

func TestCodeInfoUnknown(t *testing.T) {
	unknownCode := Code(9999)
	info := unknownCode.Info()
	if info.Title != "error" {
		t.Errorf("unknown code should return default title, got %q", info.Title)
	}
}

func TestDiagnostic_StringMethod(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "test.lua", Line: 10, Column: 5},
		Message:  "test error",
	}
	got := d.String()
	want := "test.lua:10:5: test error"
	if got != want {
		t.Errorf("Diagnostic.String() = %q, want %q", got, want)
	}
}

func TestDiagnostic_ErrorMethod(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 1},
		Message:  "syntax error",
	}
	got := d.Error()
	want := "main.lua:1:1: syntax error"
	if got != want {
		t.Errorf("Diagnostic.Error() = %q, want %q", got, want)
	}
}

func TestLabel(t *testing.T) {
	label := Label{
		Span:    Span{StartLine: 1, StartCol: 0, EndLine: 1, EndCol: 10},
		Message: "here",
	}
	if label.Message != "here" {
		t.Errorf("Label.Message = %q, want %q", label.Message, "here")
	}
}

package diag

import "testing"

// mockNode implements PositionHolder for testing.
type mockNode struct {
	line int
}

func (m mockNode) Line() int { return m.line }

// mockSpanNode implements SpanHolder for testing.
type mockSpanNode struct {
	line, col, lastLine, lastCol int
}

func (m mockSpanNode) Line() int       { return m.line }
func (m mockSpanNode) Column() int     { return m.col }
func (m mockSpanNode) LastLine() int   { return m.lastLine }
func (m mockSpanNode) LastColumn() int { return m.lastCol }

func TestCollector_Add(t *testing.T) {
	c := NewCollector("test.lua")

	c.Add(mockNode{line: 10}, ErrTypeMismatch, "expected %s, got %s", "number", "string")

	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1", c.Len())
	}

	all := c.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d diagnostics, want 1", len(all))
	}

	d := all[0]
	if d.Position.Line != 10 {
		t.Errorf("Line = %d, want 10", d.Position.Line)
	}

	if d.Code != ErrTypeMismatch {
		t.Errorf("Code = %d, want %d", d.Code, ErrTypeMismatch)
	}

	if d.Severity != SeverityError {
		t.Errorf("Severity = %d, want %d", d.Severity, SeverityError)
	}
}

func TestCollector_AddWarning(t *testing.T) {
	c := NewCollector("test.lua")
	c.AddWarning(mockNode{line: 5}, ErrUnreachable, "unreachable code")

	all := c.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d diagnostics, want 1", len(all))
	}

	if all[0].Severity != SeverityWarning {
		t.Errorf("Severity = %d, want %d", all[0].Severity, SeverityWarning)
	}
}

func TestCollector_Deduplication(t *testing.T) {
	c := NewCollector("test.lua")

	c.Add(mockNode{line: 10}, ErrTypeMismatch, "same message")
	c.Add(mockNode{line: 10}, ErrTypeMismatch, "same message")
	c.Add(mockNode{line: 10}, ErrTypeMismatch, "same message")

	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (should deduplicate)", c.Len())
	}
}

func TestCollector_HasErrors(t *testing.T) {
	c := NewCollector("test.lua")

	if c.HasErrors() {
		t.Error("HasErrors() = true for empty collector")
	}

	c.AddWarning(mockNode{line: 1}, ErrUnreachable, "warning")

	if c.HasErrors() {
		t.Error("HasErrors() = true with only warnings")
	}

	c.Add(mockNode{line: 2}, ErrTypeMismatch, "error")

	if !c.HasErrors() {
		t.Error("HasErrors() = false with error present")
	}
}

func TestCollector_Truncate(t *testing.T) {
	c := NewCollector("test.lua")

	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error 1")
	c.Add(mockNode{line: 2}, ErrTypeMismatch, "error 2")
	c.Add(mockNode{line: 3}, ErrTypeMismatch, "error 3")

	c.Truncate(1)

	if c.Len() != 1 {
		t.Errorf("Len() = %d after Truncate(1), want 1", c.Len())
	}
}

func TestCollector_Clear(t *testing.T) {
	c := NewCollector("test.lua")

	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error")
	c.Clear()

	if c.Len() != 0 {
		t.Errorf("Len() = %d after Clear(), want 0", c.Len())
	}
}

func TestCollector_NilSafety(_ *testing.T) {
	var c *Collector

	// These should not panic
	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error")
	c.AddWarning(mockNode{line: 1}, ErrTypeMismatch, "warning")
	_ = c.All()
	_ = c.HasErrors()
	_ = c.Len()
	c.Truncate(0)
	c.Clear()
	_ = c.Source()
}

func TestCollector_Source(t *testing.T) {
	c := NewCollector("test.lua")
	if c.Source() != "test.lua" {
		t.Errorf("Source() = %q, want %q", c.Source(), "test.lua")
	}
}

func TestCollector_AddHint(t *testing.T) {
	c := NewCollector("test.lua")
	c.AddHint(mockNode{line: 5}, ErrUndefined, "consider using _")

	all := c.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d diagnostics, want 1", len(all))
	}

	if all[0].Severity != SeverityHint {
		t.Errorf("Severity = %d, want %d", all[0].Severity, SeverityHint)
	}
}

func TestCollector_HasWarnings(t *testing.T) {
	c := NewCollector("test.lua")

	if c.HasWarnings() {
		t.Error("HasWarnings() = true for empty collector")
	}

	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error")

	if c.HasWarnings() {
		t.Error("HasWarnings() = true with only errors")
	}

	c.AddWarning(mockNode{line: 2}, ErrUnreachable, "warning")

	if !c.HasWarnings() {
		t.Error("HasWarnings() = false with warning present")
	}
}

func TestCollector_SpanHolder(t *testing.T) {
	c := NewCollector("test.lua")
	node := mockSpanNode{line: 10, col: 5, lastLine: 10, lastCol: 15}
	c.Add(node, ErrTypeMismatch, "error with span")

	all := c.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d diagnostics, want 1", len(all))
	}

	d := all[0]
	if d.Position.Column != 5 {
		t.Errorf("Column = %d, want 5", d.Position.Column)
	}

	if d.Span.StartLine != 10 || d.Span.EndLine != 10 {
		t.Errorf("Span lines wrong: %d-%d, want 10-10", d.Span.StartLine, d.Span.EndLine)
	}

	if d.Span.StartCol != 5 || d.Span.EndCol != 15 {
		t.Errorf("Span cols wrong: %d-%d, want 5-15", d.Span.StartCol, d.Span.EndCol)
	}
}

func TestCollector_NilNode(t *testing.T) {
	c := NewCollector("test.lua")
	c.Add(nil, ErrTypeMismatch, "error with nil node")

	all := c.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d diagnostics, want 1", len(all))
	}

	if all[0].Position.Line != 1 {
		t.Errorf("Line = %d, want 1 (default for nil node)", all[0].Position.Line)
	}
}

func TestCollector_TruncateEdgeCases(t *testing.T) {
	c := NewCollector("test.lua")
	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error 1")
	c.Add(mockNode{line: 2}, ErrTypeMismatch, "error 2")

	c.Truncate(-1)

	if c.Len() != 0 {
		t.Errorf("Truncate(-1) should result in 0 items, got %d", c.Len())
	}

	c.Add(mockNode{line: 1}, ErrTypeMismatch, "error 1")
	c.Truncate(100)

	if c.Len() != 1 {
		t.Errorf("Truncate(100) on 1 item should keep 1, got %d", c.Len())
	}
}

func TestDiagnostic_String(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "test.lua", Line: 10, Column: 5},
		Message:  "type mismatch",
	}

	s := d.String()
	if s != "test.lua:10:5: type mismatch" {
		t.Errorf("String() = %q, want %q", s, "test.lua:10:5: type mismatch")
	}
}

func TestDiagnostic_Error(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "test.lua", Line: 10, Column: 5},
		Message:  "type mismatch",
	}

	e := d.Error()
	if e != "test.lua:10:5: type mismatch" {
		t.Errorf("Error() = %q, want %q", e, "test.lua:10:5: type mismatch")
	}
}

func TestContextualHelp(t *testing.T) {
	tests := []struct {
		format      string
		wantExplain string
		wantHelp    string
	}{
		{
			format:      "cannot concatenate %s and %s",
			wantExplain: "The .. operator requires string operands in strict mode.",
			wantHelp:    "Convert the value to string using tostring() before concatenation.",
		},
		{
			format:      "cannot compare %s and %s",
			wantExplain: "Relational operators (<, <=, >, >=) require operands of the same type.",
			wantHelp:    "Compare values of the same type, or convert them explicitly.",
		},
		{
			format:      "map key type mismatch",
			wantExplain: "Map indexing requires a key of the declared key type.",
		},
		{
			format:      "array index must be integer",
			wantExplain: "Arrays can only be indexed with numeric values.",
		},
		{
			format:      "arithmetic requires number",
			wantExplain: "Arithmetic operators (+, -, *, /, etc.) require numeric operands.",
		},
		{
			format:      "cannot call method on optional type",
			wantExplain: "The value may be nil. Methods cannot be called on nil values.",
			wantHelp:    "Add a nil check before calling the method.",
		},
		{
			format:      "cannot access field on optional type",
			wantExplain: "The value may be nil. Fields cannot be accessed on nil values.",
			wantHelp:    "Add a nil check before accessing the field.",
		},
		{
			format:      "not enough arguments",
			wantExplain: "The function requires more arguments than provided.",
		},
		{
			format:      "too many arguments",
			wantExplain: "The function was called with more arguments than it accepts.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			expl, help := ContextualHelp(ErrTypeMismatch, tt.format, "msg")
			if expl != tt.wantExplain {
				t.Errorf("explanation = %q, want %q", expl, tt.wantExplain)
			}

			if tt.wantHelp != "" && help != tt.wantHelp {
				t.Errorf("help = %q, want %q", help, tt.wantHelp)
			}
		})
	}
}

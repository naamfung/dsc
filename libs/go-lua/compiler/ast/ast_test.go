package ast

import (
	"testing"

	"github.com/wippyai/go-lua/types/diag"
)

func TestNodeColumn(t *testing.T) {
	n := &Node{}
	if n.Column() != 0 {
		t.Errorf("Column() = %d, want 0", n.Column())
	}
	n.SetColumn(5)
	if n.Column() != 5 {
		t.Errorf("Column() = %d, want 5", n.Column())
	}
}

func TestNodeLastColumn(t *testing.T) {
	n := &Node{}
	if n.LastColumn() != 0 {
		t.Errorf("LastColumn() = %d, want 0", n.LastColumn())
	}
	n.SetLastColumn(10)
	if n.LastColumn() != 10 {
		t.Errorf("LastColumn() = %d, want 10", n.LastColumn())
	}
}

func TestNodeSetPosFromToken(t *testing.T) {
	n := &Node{}
	pos := Position{Line: 5, Column: 10, EndLine: 5, EndColumn: 12}
	n.SetPosFromToken(pos)
	if n.Line() != 5 {
		t.Errorf("Line() = %d, want 5", n.Line())
	}
	if n.Column() != 10 {
		t.Errorf("Column() = %d, want 10", n.Column())
	}
	if n.LastLine() != 5 || n.LastColumn() != 12 {
		t.Errorf("Last = %d:%d, want 5:12", n.LastLine(), n.LastColumn())
	}
}

func TestNodeSetLastPosFromToken(t *testing.T) {
	n := &Node{}
	pos := Position{Line: 15, Column: 20, EndLine: 15, EndColumn: 22}
	n.SetLastPosFromToken(pos)
	if n.LastLine() != 15 {
		t.Errorf("LastLine() = %d, want 15", n.LastLine())
	}
	if n.LastColumn() != 22 {
		t.Errorf("LastColumn() = %d, want 22", n.LastColumn())
	}
}

func TestNodeCopyPos(t *testing.T) {
	src := &Node{}
	src.SetLine(7)
	src.SetColumn(14)

	dst := &Node{}
	dst.CopyPos(src)
	if dst.Line() != 7 {
		t.Errorf("Line() = %d, want 7", dst.Line())
	}
	if dst.Column() != 14 {
		t.Errorf("Column() = %d, want 14", dst.Column())
	}
}

func TestNodeCopyLastPos(t *testing.T) {
	src := &Node{}
	src.SetLastLine(25)
	src.SetLastColumn(30)

	dst := &Node{}
	dst.CopyLastPos(src)
	if dst.LastLine() != 25 {
		t.Errorf("LastLine() = %d, want 25", dst.LastLine())
	}
	if dst.LastColumn() != 30 {
		t.Errorf("LastColumn() = %d, want 30", dst.LastColumn())
	}
}

func TestSpanValid(t *testing.T) {
	tests := []struct {
		name string
		span diag.Span
		want bool
	}{
		{"zero span", diag.Span{}, false},
		{"only start line", diag.Span{StartLine: 1}, false},
		{"only start col", diag.Span{StartCol: 1}, false},
		{"valid span", diag.Span{StartLine: 1, StartCol: 1}, true},
		{"full span", diag.Span{StartLine: 1, StartCol: 5, EndLine: 10, EndCol: 15}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.span.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpanSingleLine(t *testing.T) {
	tests := []struct {
		name string
		span diag.Span
		want bool
	}{
		{"same line", diag.Span{StartLine: 5, EndLine: 5}, true},
		{"end line zero", diag.Span{StartLine: 5, EndLine: 0}, true},
		{"different lines", diag.Span{StartLine: 5, EndLine: 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.span.SingleLine(); got != tt.want {
				t.Errorf("SingleLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpanOf(t *testing.T) {
	t.Run("nil holder", func(t *testing.T) {
		span := SpanOf(nil)
		if span.StartLine != 0 || span.StartCol != 0 || span.EndLine != 0 || span.EndCol != 0 {
			t.Errorf("SpanOf(nil) = %+v, want zero span", span)
		}
	})

	t.Run("valid holder", func(t *testing.T) {
		n := &Node{}
		n.SetLine(1)
		n.SetColumn(5)
		n.SetLastLine(3)
		n.SetLastColumn(10)

		span := SpanOf(n)
		if span.StartLine != 1 || span.StartCol != 5 || span.EndLine != 3 || span.EndCol != 10 {
			t.Errorf("SpanOf() = %+v, want {1 5 3 10}", span)
		}
	})
}

func TestTokenString(t *testing.T) {
	tok := &Token{
		Type: 1,
		Name: "IDENT",
		Str:  "foo",
	}
	s := tok.String()
	if s != "<type:IDENT, str:foo>" {
		t.Errorf("String() = %q, want %q", s, "<type:IDENT, str:foo>")
	}
}

func TestExprMarker(t *testing.T) {
	var _ Expr = &ExprBase{}
	var _ Expr = &TrueExpr{}
	var _ Expr = &FalseExpr{}
	var _ Expr = &NilExpr{}
	var _ Expr = &NumberExpr{}
	var _ Expr = &StringExpr{}

	e := &ExprBase{}
	e.exprMarker()
}

func TestConstExprMarker(t *testing.T) {
	var _ ConstExpr = &ConstExprBase{}
	var _ ConstExpr = &TrueExpr{}

	ce := &ConstExprBase{}
	ce.constExprMarker()
}

func TestStmtMarker(t *testing.T) {
	var _ Stmt = &StmtBase{}
	var _ Stmt = &AssignStmt{}
	var _ Stmt = &LocalAssignStmt{}

	s := &StmtBase{}
	s.stmtMarker()
}

func TestTypeExprMarker(t *testing.T) {
	var _ TypeExpr = &TypeExprBase{}
	var _ TypeExpr = &PrimitiveTypeExpr{}

	te := &TypeExprBase{}
	te.typeExprMarker()
}

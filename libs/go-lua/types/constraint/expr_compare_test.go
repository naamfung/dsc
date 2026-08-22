package constraint

import (
	"testing"
)

func TestExprRel_String(t *testing.T) {
	tests := []struct {
		rel  ExprRel
		want string
	}{
		{ExprEq, "=="},
		{ExprNe, "!="},
		{ExprLt, "<"},
		{ExprLe, "<="},
		{ExprGt, ">"},
		{ExprGe, ">="},
		{ExprRel(99), "?"},
		{ExprRel(255), "?"},
	}
	for _, tt := range tests {
		got := tt.rel.String()
		if got != tt.want {
			t.Errorf("ExprRel(%d).String() = %q, want %q", tt.rel, got, tt.want)
		}
	}
}

func TestExprRel_AllValues(t *testing.T) {
	// Ensure all defined values have proper string representations
	rels := []ExprRel{ExprEq, ExprNe, ExprLt, ExprLe, ExprGt, ExprGe}
	for _, rel := range rels {
		s := rel.String()
		if s == "?" {
			t.Errorf("ExprRel(%d) should have a defined string, got %q", rel, s)
		}
	}
}

func TestExprCompare_String(t *testing.T) {
	left := Var{Name: "x"}
	right := Const{Value: 5}

	tests := []struct {
		cmp  ExprCompare
		want string
	}{
		{EqExpr(left, right), "(x == 5)"},
		{NeExpr(left, right), "(x != 5)"},
		{LtExpr(left, right), "(x < 5)"},
		{LeExpr(left, right), "(x <= 5)"},
		{GtExpr(left, right), "(x > 5)"},
		{GeExpr(left, right), "(x >= 5)"},
	}
	for _, tt := range tests {
		got := tt.cmp.String()
		if got != tt.want {
			t.Errorf("ExprCompare.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestExprCompare_Equals(t *testing.T) {
	x := Var{Name: "x"}
	y := Var{Name: "y"}
	c5 := Const{Value: 5}
	c10 := Const{Value: 10}

	tests := []struct {
		name string
		a, b ExprCompare
		want bool
	}{
		{"same eq", EqExpr(x, c5), EqExpr(x, c5), true},
		{"same ne", NeExpr(x, c5), NeExpr(x, c5), true},
		{"same lt", LtExpr(x, c5), LtExpr(x, c5), true},
		{"same le", LeExpr(x, c5), LeExpr(x, c5), true},
		{"same gt", GtExpr(x, c5), GtExpr(x, c5), true},
		{"same ge", GeExpr(x, c5), GeExpr(x, c5), true},

		{"diff rel", EqExpr(x, c5), NeExpr(x, c5), false},
		{"diff left", EqExpr(x, c5), EqExpr(y, c5), false},
		{"diff right", EqExpr(x, c5), EqExpr(x, c10), false},
		{"diff all", EqExpr(x, c5), NeExpr(y, c10), false},
	}
	for _, tt := range tests {
		got := tt.a.Equals(tt.b)
		if got != tt.want {
			t.Errorf("%s: Equals() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExprCompare_Equals_Symmetric(t *testing.T) {
	x := Var{Name: "x"}
	c5 := Const{Value: 5}

	cmps := []ExprCompare{
		EqExpr(x, c5),
		NeExpr(x, c5),
		LtExpr(x, c5),
		LeExpr(x, c5),
		GtExpr(x, c5),
		GeExpr(x, c5),
	}

	for i, a := range cmps {
		for j, b := range cmps {
			eq1 := a.Equals(b)
			eq2 := b.Equals(a)
			if eq1 != eq2 {
				t.Errorf("Equals not symmetric: cmps[%d].Equals(cmps[%d])=%v but cmps[%d].Equals(cmps[%d])=%v",
					i, j, eq1, j, i, eq2)
			}
		}
	}
}

func TestEqExpr(t *testing.T) {
	left := Var{Name: "a"}
	right := Const{Value: 1}

	cmp := EqExpr(left, right)

	if cmp.Rel != ExprEq {
		t.Errorf("EqExpr().Rel = %v, want ExprEq", cmp.Rel)
	}
	if !ExprEquals(cmp.Left, left) {
		t.Error("EqExpr().Left does not match")
	}
	if !ExprEquals(cmp.Right, right) {
		t.Error("EqExpr().Right does not match")
	}
}

func TestNeExpr(t *testing.T) {
	left := Var{Name: "a"}
	right := Const{Value: 1}

	cmp := NeExpr(left, right)

	if cmp.Rel != ExprNe {
		t.Errorf("NeExpr().Rel = %v, want ExprNe", cmp.Rel)
	}
}

func TestLtExpr(t *testing.T) {
	left := Var{Name: "a"}
	right := Const{Value: 1}

	cmp := LtExpr(left, right)

	if cmp.Rel != ExprLt {
		t.Errorf("LtExpr().Rel = %v, want ExprLt", cmp.Rel)
	}
}

func TestLeExpr(t *testing.T) {
	left := Var{Name: "a"}
	right := Const{Value: 1}

	cmp := LeExpr(left, right)

	if cmp.Rel != ExprLe {
		t.Errorf("LeExpr().Rel = %v, want ExprLe", cmp.Rel)
	}
}

func TestGtExpr(t *testing.T) {
	left := Var{Name: "a"}
	right := Const{Value: 1}

	cmp := GtExpr(left, right)

	if cmp.Rel != ExprGt {
		t.Errorf("GtExpr().Rel = %v, want ExprGt", cmp.Rel)
	}
}

func TestGeExpr(t *testing.T) {
	left := Var{Name: "a"}
	right := Const{Value: 1}

	cmp := GeExpr(left, right)

	if cmp.Rel != ExprGe {
		t.Errorf("GeExpr().Rel = %v, want ExprGe", cmp.Rel)
	}
}

func TestExprCompare_FactoryRoundTrip(t *testing.T) {
	left := Var{Name: "x"}
	right := Const{Value: 42}

	factories := []struct {
		name string
		fn   func(Expr, Expr) ExprCompare
		rel  ExprRel
	}{
		{"EqExpr", EqExpr, ExprEq},
		{"NeExpr", NeExpr, ExprNe},
		{"LtExpr", LtExpr, ExprLt},
		{"LeExpr", LeExpr, ExprLe},
		{"GtExpr", GtExpr, ExprGt},
		{"GeExpr", GeExpr, ExprGe},
	}

	for _, f := range factories {
		cmp := f.fn(left, right)
		if cmp.Rel != f.rel {
			t.Errorf("%s: Rel = %v, want %v", f.name, cmp.Rel, f.rel)
		}
		if !ExprEquals(cmp.Left, left) {
			t.Errorf("%s: Left mismatch", f.name)
		}
		if !ExprEquals(cmp.Right, right) {
			t.Errorf("%s: Right mismatch", f.name)
		}
	}
}

func TestExprCompare_Equals_NilExprs(t *testing.T) {
	// Test behavior with nil expressions
	cmp1 := ExprCompare{Rel: ExprEq, Left: nil, Right: nil}
	cmp2 := ExprCompare{Rel: ExprEq, Left: nil, Right: nil}

	// Both nil should be equal
	if !cmp1.Equals(cmp2) {
		t.Error("ExprCompare with nil exprs should equal itself")
	}

	// One nil, one non-nil should not be equal
	cmp3 := ExprCompare{Rel: ExprEq, Left: Var{Name: "x"}, Right: nil}
	if cmp1.Equals(cmp3) {
		t.Error("ExprCompare with different nil/non-nil should not be equal")
	}
}

func TestExprCompare_ComplexExprs(t *testing.T) {
	// Test with more complex expression types using BinOp
	add := Add(Var{Name: "x"}, Const{Value: 1})
	sub := Sub(Var{Name: "y"}, Const{Value: 2})

	cmp := LtExpr(add, sub)

	if cmp.Rel != ExprLt {
		t.Error("Complex expr comparison should preserve relation")
	}

	// String should still work
	s := cmp.String()
	if s == "" {
		t.Error("Complex expr comparison String() should not be empty")
	}
}

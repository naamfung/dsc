package numconst_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/numconst"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestNegateConstraints_Nil(t *testing.T) {
	result := numconst.NegateConstraints(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestNegateConstraints_EmptySlice(t *testing.T) {
	result := numconst.NegateConstraints([]constraint.Constraint{})
	if result != nil {
		t.Error("expected nil for empty input")
	}
}

func TestNegateConstraints_IsNil(t *testing.T) {
	items := []constraint.Constraint{
		constraint.IsNil{Path: constraint.Path{Root: "x"}},
	}
	result := numconst.NegateConstraints(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if _, ok := result[0].(constraint.NotNil); !ok {
		t.Errorf("expected NotNil constraint, got %T", result[0])
	}
}

func TestNegateConstraints_NotNil(t *testing.T) {
	items := []constraint.Constraint{
		constraint.NotNil{Path: constraint.Path{Root: "x"}},
	}
	result := numconst.NegateConstraints(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if _, ok := result[0].(constraint.IsNil); !ok {
		t.Errorf("expected IsNil constraint, got %T", result[0])
	}
}

func TestNegateNumericConstraint_NilInput(t *testing.T) {
	result := numconst.NegateNumericConstraint(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestNegateNumericConstraint_Lt(t *testing.T) {
	c := constraint.Lt{X: constraint.Path{Root: "a"}, Y: constraint.Path{Root: "b"}}
	result := numconst.NegateNumericConstraint(c)
	if _, ok := result.(constraint.Ge); !ok {
		t.Errorf("expected Ge constraint, got %T", result)
	}
}

func TestNegateNumericConstraint_Gt(t *testing.T) {
	c := constraint.Gt{X: constraint.Path{Root: "a"}, Y: constraint.Path{Root: "b"}}
	result := numconst.NegateNumericConstraint(c)
	if _, ok := result.(constraint.Le); !ok {
		t.Errorf("expected Le constraint, got %T", result)
	}
}

func TestNegateNumericConstraint_Le(t *testing.T) {
	c := constraint.Le{X: constraint.Path{Root: "a"}, Y: constraint.Path{Root: "b"}}
	result := numconst.NegateNumericConstraint(c)
	if _, ok := result.(constraint.Gt); !ok {
		t.Errorf("expected Gt constraint, got %T", result)
	}
}

func TestNegateNumericConstraint_Ge(t *testing.T) {
	c := constraint.Ge{X: constraint.Path{Root: "a"}, Y: constraint.Path{Root: "b"}}
	result := numconst.NegateNumericConstraint(c)
	if _, ok := result.(constraint.Lt); !ok {
		t.Errorf("expected Lt constraint, got %T", result)
	}
}

func TestNegateNumericConstraint_LeConst(t *testing.T) {
	c := constraint.LeConst{X: constraint.Path{Root: "a"}, C: 10}
	result := numconst.NegateNumericConstraint(c)
	geConst, ok := result.(constraint.GeConst)
	if !ok {
		t.Fatalf("expected GeConst constraint, got %T", result)
	}
	if geConst.C != 11 {
		t.Errorf("expected C=11, got C=%d", geConst.C)
	}
}

func TestNegateNumericConstraint_GeConst(t *testing.T) {
	c := constraint.GeConst{X: constraint.Path{Root: "a"}, C: 10}
	result := numconst.NegateNumericConstraint(c)
	leConst, ok := result.(constraint.LeConst)
	if !ok {
		t.Fatalf("expected LeConst constraint, got %T", result)
	}
	if leConst.C != 9 {
		t.Errorf("expected C=9, got C=%d", leConst.C)
	}
}

func TestIntConstFromExpr_NumberExpr(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	val, ok := numconst.IntConstFromExpr(expr)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestIntConstFromExpr_NegativeNumber(t *testing.T) {
	expr := &ast.UnaryMinusOpExpr{Expr: &ast.NumberExpr{Value: "42"}}
	val, ok := numconst.IntConstFromExpr(expr)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != -42 {
		t.Errorf("expected -42, got %d", val)
	}
}

func TestIntConstFromExpr_NonNumber(t *testing.T) {
	expr := &ast.StringExpr{Value: "42"}
	_, ok := numconst.IntConstFromExpr(expr)
	if ok {
		t.Error("expected ok=false for string expr")
	}
}

func TestIntConstFromExpr_InvalidNumber(t *testing.T) {
	expr := &ast.NumberExpr{Value: "not_a_number"}
	_, ok := numconst.IntConstFromExpr(expr)
	if ok {
		t.Error("expected ok=false for invalid number")
	}
}

func TestIntConstFromExpr_FloatRejected(t *testing.T) {
	expr := &ast.NumberExpr{Value: "3.14"}
	_, ok := numconst.IntConstFromExpr(expr)
	if ok {
		t.Error("expected ok=false for float number")
	}
}

func TestNumericConstraintFromComparisonWithBindings_LtPaths(t *testing.T) {
	lhs := &ast.IdentExpr{Value: "a"}
	rhs := &ast.IdentExpr{Value: "b"}
	result := numconst.NumericConstraintFromComparisonWithBindings("<", lhs, rhs, 0, nil, nil)
	if _, ok := result.(constraint.Lt); !ok {
		t.Errorf("expected Lt constraint, got %T", result)
	}
}

func TestNumericConstraintFromComparisonWithBindings_LtWithConst(t *testing.T) {
	lhs := &ast.IdentExpr{Value: "a"}
	rhs := &ast.NumberExpr{Value: "10"}
	result := numconst.NumericConstraintFromComparisonWithBindings("<", lhs, rhs, 0, nil, nil)
	leConst, ok := result.(constraint.LeConst)
	if !ok {
		t.Fatalf("expected LeConst constraint, got %T", result)
	}
	if leConst.C != 9 {
		t.Errorf("expected C=9 (10-1), got C=%d", leConst.C)
	}
}

func TestNumericConstraintFromComparisonWithBindings_GtPaths(t *testing.T) {
	lhs := &ast.IdentExpr{Value: "a"}
	rhs := &ast.IdentExpr{Value: "b"}
	result := numconst.NumericConstraintFromComparisonWithBindings(">", lhs, rhs, 0, nil, nil)
	if _, ok := result.(constraint.Gt); !ok {
		t.Errorf("expected Gt constraint, got %T", result)
	}
}

func TestNumericConstraintFromComparisonWithBindings_LePaths(t *testing.T) {
	lhs := &ast.IdentExpr{Value: "a"}
	rhs := &ast.IdentExpr{Value: "b"}
	result := numconst.NumericConstraintFromComparisonWithBindings("<=", lhs, rhs, 0, nil, nil)
	if _, ok := result.(constraint.Le); !ok {
		t.Errorf("expected Le constraint, got %T", result)
	}
}

func TestNumericConstraintFromComparisonWithBindings_GePaths(t *testing.T) {
	lhs := &ast.IdentExpr{Value: "a"}
	rhs := &ast.IdentExpr{Value: "b"}
	result := numconst.NumericConstraintFromComparisonWithBindings(">=", lhs, rhs, 0, nil, nil)
	if _, ok := result.(constraint.Ge); !ok {
		t.Errorf("expected Ge constraint, got %T", result)
	}
}

func TestNumericConstraintFromComparisonWithBindings_UnknownOp(t *testing.T) {
	lhs := &ast.IdentExpr{Value: "a"}
	rhs := &ast.IdentExpr{Value: "b"}
	result := numconst.NumericConstraintFromComparisonWithBindings("==", lhs, rhs, 0, nil, nil)
	if result != nil {
		t.Error("expected nil for unknown operator")
	}
}

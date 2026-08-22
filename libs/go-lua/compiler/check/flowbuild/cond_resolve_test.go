package flowbuild_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/stdlib"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestClassifyReturnExpr_True(t *testing.T) {
	expr := &ast.TrueExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnTrue {
		t.Errorf("resolve.ClassifyReturnExpr(true) = %v, want ReturnTrue", kind)
	}
}

func TestClassifyReturnExpr_False(t *testing.T) {
	expr := &ast.FalseExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnFalse {
		t.Errorf("resolve.ClassifyReturnExpr(false) = %v, want ReturnFalse", kind)
	}
}

func TestClassifyReturnExpr_IdentTrue(t *testing.T) {
	expr := &ast.IdentExpr{Value: "true"}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnTrue {
		t.Errorf("resolve.ClassifyReturnExpr(ident 'true') = %v, want ReturnTrue", kind)
	}
}

func TestClassifyReturnExpr_IdentFalse(t *testing.T) {
	expr := &ast.IdentExpr{Value: "false"}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnFalse {
		t.Errorf("resolve.ClassifyReturnExpr(ident 'false') = %v, want ReturnFalse", kind)
	}
}

func TestClassifyReturnExpr_Unknown(t *testing.T) {
	tests := []ast.Expr{
		nil,
		&ast.IdentExpr{Value: "foo"},
		&ast.StringExpr{Value: "hello"},
		&ast.NumberExpr{Value: "42"},
	}

	for _, expr := range tests {
		kind := resolve.ClassifyReturnExpr(expr)
		if kind != flow.ReturnUnknown {
			t.Errorf("resolve.ClassifyReturnExpr(%T) = %v, want ReturnUnknown", expr, kind)
		}
	}
}

func TestConstraintsFromBranch_Nil(t *testing.T) {
	result := (&cond.ConditionExtractor{}).ConstraintsFromBranch(nil)
	if result.OnTrue.HasConstraints() || result.OnFalse.HasConstraints() {
		t.Error("constraintsFromBranch(nil) should return empty constraints")
	}
}

func TestConstraintsFromBranch_EmptyCondVar(t *testing.T) {
	info := &cfg.BranchInfo{CondVar: "", CondCheck: cfg.CondCheck{Kind: cfg.CheckNil}}
	result := (&cond.ConditionExtractor{}).ConstraintsFromBranch(info)
	if result.OnTrue.HasConstraints() || result.OnFalse.HasConstraints() {
		t.Error("constraintsFromBranch(empty condvar) should return empty constraints")
	}
}

func TestConstraintsFromConditionExpr_ConstFalse(t *testing.T) {
	expr := &ast.FalseExpr{}
	result := (&cond.ConditionExtractor{}).ConstraintsFromConditionExpr(expr)
	if !result.OnTrue.IsFalse() {
		t.Error("onTrue should be false for constant false condition")
	}
	if !result.OnFalse.IsTrue() {
		t.Error("onFalse should be true for constant false condition")
	}
}

func TestConstraintsFromConditionExpr_ConstTrue(t *testing.T) {
	expr := &ast.TrueExpr{}
	result := (&cond.ConditionExtractor{}).ConstraintsFromConditionExpr(expr)
	if !result.OnTrue.IsTrue() {
		t.Error("onTrue should be true for constant true condition")
	}
	if !result.OnFalse.IsFalse() {
		t.Error("onFalse should be false for constant true condition")
	}
}

func TestConstraintsFromBranch_NilCheck(t *testing.T) {
	info := &cfg.BranchInfo{CondVar: "x", CondSymbol: 1, CondCheck: cfg.CondCheck{Kind: cfg.CheckNil}}
	result := (&cond.ConditionExtractor{}).ConstraintsFromBranch(info)

	if !result.OnTrue.HasConstraints() || !result.OnFalse.HasConstraints() {
		t.Fatal("expected constraints on both branches")
	}

	trueItems := result.OnTrue.MustConstraints()
	if _, ok := trueItems[0].(constraint.IsNil); !ok {
		t.Errorf("onTrue[0] = %T, want IsNil", trueItems[0])
	}

	falseItems := result.OnFalse.MustConstraints()
	if _, ok := falseItems[0].(constraint.NotNil); !ok {
		t.Errorf("onFalse[0] = %T, want NotNil", falseItems[0])
	}
}

func TestConstraintsFromBranch_NotNilCheck(t *testing.T) {
	info := &cfg.BranchInfo{CondVar: "x", CondSymbol: 1, CondCheck: cfg.CondCheck{Kind: cfg.CheckNotNil}}
	result := (&cond.ConditionExtractor{}).ConstraintsFromBranch(info)

	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}

	trueItems := result.OnTrue.MustConstraints()
	if _, ok := trueItems[0].(constraint.NotNil); !ok {
		t.Errorf("onTrue[0] = %T, want NotNil", trueItems[0])
	}

	falseItems := result.OnFalse.MustConstraints()
	if _, ok := falseItems[0].(constraint.IsNil); !ok {
		t.Errorf("onFalse[0] = %T, want IsNil", falseItems[0])
	}
}

func TestConstraintsFromBranch_TruthyCheck(t *testing.T) {
	info := &cfg.BranchInfo{CondVar: "x", CondSymbol: 1, CondCheck: cfg.CondCheck{Kind: cfg.CheckTruthy}}
	result := (&cond.ConditionExtractor{}).ConstraintsFromBranch(info)

	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}

	trueItems := result.OnTrue.MustConstraints()
	if _, ok := trueItems[0].(constraint.Truthy); !ok {
		t.Errorf("onTrue[0] = %T, want Truthy", trueItems[0])
	}

	falseItems := result.OnFalse.MustConstraints()
	if _, ok := falseItems[0].(constraint.Falsy); !ok {
		t.Errorf("onFalse[0] = %T, want Falsy", falseItems[0])
	}
}

func TestConstraintsFromBranch_FalsyCheck(t *testing.T) {
	info := &cfg.BranchInfo{CondVar: "x", CondSymbol: 1, CondCheck: cfg.CondCheck{Kind: cfg.CheckFalsy}}
	result := (&cond.ConditionExtractor{}).ConstraintsFromBranch(info)

	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}

	trueItems := result.OnTrue.MustConstraints()
	if _, ok := trueItems[0].(constraint.Falsy); !ok {
		t.Errorf("onTrue[0] = %T, want Falsy", trueItems[0])
	}

	falseItems := result.OnFalse.MustConstraints()
	if _, ok := falseItems[0].(constraint.Truthy); !ok {
		t.Errorf("onFalse[0] = %T, want Truthy", falseItems[0])
	}
}

func TestConstraintsFromConditionExpr_NilCheck(t *testing.T) {
	// x == nil
	expr := &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      &ast.IdentExpr{Value: "x"},
		Rhs:      &ast.NilExpr{},
	}
	result := (&cond.ConditionExtractor{}).ConstraintsFromConditionExpr(expr)

	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}

	trueItems := result.OnTrue.MustConstraints()
	if _, ok := trueItems[0].(constraint.IsNil); !ok {
		t.Errorf("onTrue[0] = %T, want IsNil", trueItems[0])
	}
}

func TestConstraintsFromConditionExpr_NotNilCheck(t *testing.T) {
	// x ~= nil
	expr := &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      &ast.IdentExpr{Value: "x"},
		Rhs:      &ast.NilExpr{},
	}
	result := (&cond.ConditionExtractor{}).ConstraintsFromConditionExpr(expr)

	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}

	trueItems := result.OnTrue.MustConstraints()
	if _, ok := trueItems[0].(constraint.NotNil); !ok {
		t.Errorf("onTrue[0] = %T, want NotNil", trueItems[0])
	}
}

func TestConstraintsFromConditionExpr_TypeCheck(t *testing.T) {
	// type(x) == "string"
	expr := &ast.RelationalOpExpr{
		Operator: "==",
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		},
		Rhs: &ast.StringExpr{Value: "string"},
	}
	synth := func(e ast.Expr, _ cfg.Point) typ.Type {
		if ident, ok := e.(*ast.IdentExpr); ok && ident.Value == "type" {
			return stdlib.Type
		}
		return nil
	}
	result := (&cond.ConditionExtractor{Synth: synth}).ConstraintsFromConditionExpr(expr)

	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}

	trueItems := result.OnTrue.MustConstraints()
	if _, ok := trueItems[0].(constraint.HasType); !ok {
		t.Errorf("onTrue[0] = %T, want HasType", trueItems[0])
	}
}

func TestConstraintsFromConditionExpr_NumberComparisonNarrowsOperand(t *testing.T) {
	expr := &ast.RelationalOpExpr{
		Operator: ">",
		Lhs:      &ast.IdentExpr{Value: "x"},
		Rhs:      &ast.NumberExpr{Value: "0"},
	}
	result := (&cond.ConditionExtractor{}).ConstraintsFromConditionExpr(expr)
	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}
	items := result.OnTrue.MustConstraints()
	got, ok := items[0].(constraint.HasType)
	if !ok {
		t.Fatalf("onTrue[0] = %T, want HasType", items[0])
	}
	if got.Type != narrow.BuiltinTypeKey("number") {
		t.Fatalf("expected number type key, got %v", got.Type)
	}
}

func TestConstraintsFromConditionExpr_StringComparisonNarrowsOperand(t *testing.T) {
	expr := &ast.RelationalOpExpr{
		Operator: "<=",
		Lhs:      &ast.IdentExpr{Value: "name"},
		Rhs:      &ast.StringExpr{Value: "zz"},
	}
	result := (&cond.ConditionExtractor{}).ConstraintsFromConditionExpr(expr)
	if !result.OnTrue.HasConstraints() {
		t.Fatal("onTrue should have constraints")
	}
	items := result.OnTrue.MustConstraints()
	got, ok := items[0].(constraint.HasType)
	if !ok {
		t.Fatalf("onTrue[0] = %T, want HasType", items[0])
	}
	if got.Type != narrow.BuiltinTypeKey("string") {
		t.Fatalf("expected string type key, got %v", got.Type)
	}
}

func TestConstraintsFromConditionExpr_And(t *testing.T) {
	// x ~= nil and y ~= nil
	expr := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs: &ast.RelationalOpExpr{
			Operator: "~=",
			Lhs:      &ast.IdentExpr{Value: "x"},
			Rhs:      &ast.NilExpr{},
		},
		Rhs: &ast.RelationalOpExpr{
			Operator: "~=",
			Lhs:      &ast.IdentExpr{Value: "y"},
			Rhs:      &ast.NilExpr{},
		},
	}
	result := (&cond.ConditionExtractor{}).ConstraintsFromConditionExpr(expr)

	must := result.OnTrue.MustConstraints()
	if len(must) != 2 {
		t.Fatalf("onTrue must len = %d, want 2", len(must))
	}
}

func TestNegateConstraint(t *testing.T) {
	tests := []struct {
		input constraint.Constraint
		want  constraint.Kind
	}{
		{constraint.Truthy{}, constraint.KindFalsy},
		{constraint.Falsy{}, constraint.KindTruthy},
		{constraint.IsNil{}, constraint.KindNotNil},
		{constraint.NotNil{}, constraint.KindIsNil},
	}

	for _, tt := range tests {
		got, ok := constraint.NegateConstraint(tt.input)
		if !ok {
			t.Errorf("negateConstraint(%T) returned false", tt.input)
			continue
		}
		if got.Kind() != tt.want {
			t.Errorf("negateConstraint(%T).Kind() = %v, want %v", tt.input, got.Kind(), tt.want)
		}
	}
}

func TestConstraintsFromEquality_FieldEqualsPath(t *testing.T) {
	lhs := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "result"},
		Key:    &ast.StringExpr{Value: "channel"},
	}
	rhs := &ast.IdentExpr{Value: "ch1"}

	c := (&cond.ConditionExtractor{}).ConditionFromEquality(lhs, rhs)
	constraints := c.MustConstraints()
	if len(constraints) != 1 {
		t.Fatalf("constraintsFromEquality returned %d constraints, want 1", len(constraints))
	}

	fep, ok := constraints[0].(constraint.FieldEqualsPath)
	if !ok {
		t.Fatalf("constraint type = %T, want FieldEqualsPath", constraints[0])
	}

	if fep.Target.Root != "result" {
		t.Errorf("Target.Root = %q, want %q", fep.Target.Root, "result")
	}
	if fep.Field != "channel" {
		t.Errorf("Field = %q, want %q", fep.Field, "channel")
	}
	if fep.Value.Root != "ch1" {
		t.Errorf("Value.Root = %q, want %q", fep.Value.Root, "ch1")
	}
}

func TestConstraintsFromDynamicIndex_LiteralKeyLiteralValue(t *testing.T) {
	// Test: if x[y] == "foo" where y has type "kind" (literal string)
	// Without bindings, dynamic index extraction requires bindings to resolve symbols.
	// This test verifies the function handles nil bindings gracefully.

	lhs := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "x"},
		Key:    &ast.IdentExpr{Value: "y"},
	}
	rhs := &ast.StringExpr{Value: "foo"}

	// Without bindings, dynamic index constraints cannot be extracted
	c := (&cond.ConditionExtractor{}).ConditionFromEquality(lhs, rhs)
	constraints := c.MustConstraints()
	if len(constraints) != 0 {
		t.Fatalf("constraintsFromEquality returned %d constraints, want 0 (no bindings)", len(constraints))
	}
}

func TestConstraintsFromDynamicIndex_StringKeyPathValue(t *testing.T) {
	// Test: if x[y] == v where y has type string (not literal)
	// Without bindings, dynamic index extraction requires bindings to resolve symbols.

	lhs := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "x"},
		Key:    &ast.IdentExpr{Value: "y"},
	}
	rhs := &ast.IdentExpr{Value: "v"}

	// Without bindings, dynamic index constraints cannot be extracted
	// AttrGetExpr with variable key produces empty path, so no constraint
	c := (&cond.ConditionExtractor{}).ConditionFromEquality(lhs, rhs)
	constraints := c.MustConstraints()
	if len(constraints) != 0 {
		t.Fatalf("constraintsFromEquality returned %d constraints, want 0 (no bindings for dynamic key)", len(constraints))
	}
}

func TestConstraintsFromDynamicIndex_Inequality(t *testing.T) {
	// Test: if x[y] ~= v where y has type "kind" literal
	// Without bindings, dynamic index extraction requires bindings to resolve symbols.

	lhs := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "x"},
		Key:    &ast.IdentExpr{Value: "y"},
	}
	rhs := &ast.IdentExpr{Value: "v"}

	// Without bindings, falls through to negated equality
	c := (&cond.ConditionExtractor{}).ConditionFromInequality(lhs, rhs)
	// Inequality of paths produces Not(EqPath) which doesn't have simple constraints
	if c.IsFalse() {
		t.Fatal("condition should not be false")
	}
}

func TestConstraintsFromDynamicIndex_UnionKeyType(t *testing.T) {
	// Test: if x[y] == "foo" where y has type "a" | "b" (union of literals)
	// Without bindings, dynamic index extraction requires bindings to resolve symbols.

	lhs := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "x"},
		Key:    &ast.IdentExpr{Value: "y"},
	}
	rhs := &ast.StringExpr{Value: "foo"}

	// Without bindings, no dynamic index constraints
	c := (&cond.ConditionExtractor{}).ConditionFromEquality(lhs, rhs)
	constraints := c.MustConstraints()
	if len(constraints) != 0 {
		t.Fatalf("constraintsFromEquality returned %d constraints, want 0 (no bindings)", len(constraints))
	}
}

func TestConstraintsFromDynamicIndex_NoSymbolType(t *testing.T) {
	// Test: if x[y] == "foo" where y has no type in SymbolTypes
	// Without bindings, returns no constraints

	lhs := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "x"},
		Key:    &ast.IdentExpr{Value: "y"},
	}
	rhs := &ast.StringExpr{Value: "foo"}

	c := (&cond.ConditionExtractor{}).ConditionFromEquality(lhs, rhs)
	constraints := c.MustConstraints()
	if len(constraints) != 0 {
		t.Fatalf("constraintsFromEquality returned %d constraints, want 0 (no bindings)", len(constraints))
	}
}

func TestConstraintsFromDynamicIndex_NilResolver(t *testing.T) {
	// Test: if x[y] == "foo" with nil symTypeResolver
	// Should return nil

	lhs := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "x"},
		Key:    &ast.IdentExpr{Value: "y"},
	}
	rhs := &ast.StringExpr{Value: "foo"}

	c := (&cond.ConditionExtractor{}).ConditionFromEquality(lhs, rhs)
	constraints := c.MustConstraints()
	if len(constraints) != 0 {
		t.Fatalf("constraintsFromEquality returned %d constraints, want 0 (nil resolver)", len(constraints))
	}
}

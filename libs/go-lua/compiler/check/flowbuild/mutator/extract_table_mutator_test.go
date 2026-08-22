package mutator_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/literal"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractTableMutatorAssignments_NilGraph(t *testing.T) {
	mutator.ExtractTableMutatorAssignments(&core.FlowContext{}, nil)
	// Should not panic
}

func TestExtractTableMutatorAssignments_NilInputs(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	mutator.ExtractTableMutatorAssignments(&core.FlowContext{Graph: graph}, nil)
	// Should not panic
}

func TestExtractTableMutatorAssignments_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:             make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		TableMutatorAssignments: []flow.TableMutatorAssignment{},
	}
	scopes := map[cfg.Point]*scope.State{}
	mutator.ExtractTableMutatorAssignments(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	if len(inputs.TableMutatorAssignments) != 0 {
		t.Error("expected no assignments for empty graph")
	}
}

func TestRootNameFromBindings_TableMutator_NilBindings(t *testing.T) {
	result := resolve.RootNameFromBindings(nil, 1, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", result)
	}
}

func TestRootNameFromBindings_TableMutator_ZeroSymbol(t *testing.T) {
	result := resolve.RootNameFromBindings(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", result)
	}
}

func TestTableMutatorFromCall_NilInfo(t *testing.T) {
	result := mutator.TableMutatorFromCall(nil, 0, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}

func TestTableMutatorFromCall_NoCallee(t *testing.T) {
	info := &cfg.CallInfo{}
	result := mutator.TableMutatorFromCall(info, 0, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for info without callee")
	}
}

func TestTableMutatorFromCall_MethodCall(t *testing.T) {
	info := &cfg.CallInfo{
		Method:   "insert",
		Receiver: &ast.IdentExpr{Value: "table"},
	}
	result := mutator.TableMutatorFromCall(info, 0, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for method call (not plain function)")
	}
}

func TestTableMutatorFromCall_NonFunction(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String // Not a function
	}
	result := mutator.TableMutatorFromCall(info, 0, synth, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for non-function type")
	}
}

func TestTableMutatorFromCall_FunctionWithoutSpec(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return &typ.Function{} // No spec
	}
	result := mutator.TableMutatorFromCall(info, 0, synth, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for function without spec")
	}
}

func TestPositionalArgAt_ValidIndices(t *testing.T) {
	args := []ast.Expr{
		&ast.StringExpr{Value: "a"},
		&ast.StringExpr{Value: "b"},
		&ast.StringExpr{Value: "c"},
	}

	tests := []struct {
		idx      int
		expected string
	}{
		{0, "a"},
		{1, "b"},
		{2, "c"},
		{-1, "c"},
		{-2, "b"},
		{-3, "a"},
	}

	for _, tt := range tests {
		result := callsite.PositionalArgAt(args, tt.idx)
		str, ok := result.(*ast.StringExpr)
		if !ok || str.Value != tt.expected {
			t.Errorf("callsite.PositionalArgAt(args, %d): expected '%s', got %v", tt.idx, tt.expected, result)
		}
	}
}

func TestPositionalArgAt_InvalidIndices(t *testing.T) {
	args := []ast.Expr{&ast.StringExpr{Value: "a"}}

	tests := []int{5, -5, 100}
	for _, idx := range tests {
		result := callsite.PositionalArgAt(args, idx)
		if result != nil {
			t.Errorf("callsite.PositionalArgAt(args, %d): expected nil, got %v", idx, result)
		}
	}
}

func TestKeyTypeFromExpr_NilExpr(t *testing.T) {
	result := literal.KeyTypeFromExpr(nil, nil)
	if result != nil {
		t.Error("expected nil for nil expr")
	}
}

func TestKeyTypeFromExpr_StringLiteral(t *testing.T) {
	expr := &ast.StringExpr{Value: "key"}
	result := literal.KeyTypeFromExpr(expr, nil)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestKeyTypeFromExpr_IntegerLiteral(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	result := literal.KeyTypeFromExpr(expr, nil)
	if result != typ.Integer {
		t.Errorf("expected Integer, got %v", result)
	}
}

func TestKeyTypeFromExpr_FloatLiteral(t *testing.T) {
	expr := &ast.NumberExpr{Value: "3.14"}
	result := literal.KeyTypeFromExpr(expr, nil)
	if result != typ.Number {
		t.Errorf("expected Number, got %v", result)
	}
}

func TestKeyTypeFromExpr_BoolLiteral(t *testing.T) {
	expr := &ast.TrueExpr{}
	result := literal.KeyTypeFromExpr(expr, nil)
	if result != typ.Boolean {
		t.Errorf("expected Boolean, got %v", result)
	}
}

func TestKeyTypeFromExpr_FalseLiteral(t *testing.T) {
	expr := &ast.FalseExpr{}
	result := literal.KeyTypeFromExpr(expr, nil)
	if result != typ.Boolean {
		t.Errorf("expected Boolean, got %v", result)
	}
}

func TestKeyTypeFromExpr_IdentWithConstResolver(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "key" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "test"}
		}
		return nil
	}
	expr := &ast.IdentExpr{Value: "key"}
	result := literal.KeyTypeFromExpr(expr, constResolver)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestKeyTypeFromExpr_IdentWithIntConstResolver(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "idx" {
			return &flow.ConstValue{Kind: flow.ConstInt, Int: 5}
		}
		return nil
	}
	expr := &ast.IdentExpr{Value: "idx"}
	result := literal.KeyTypeFromExpr(expr, constResolver)
	if result != typ.Integer {
		t.Errorf("expected Integer, got %v", result)
	}
}

func TestKeyTypeFromExpr_UnknownIdent(t *testing.T) {
	expr := &ast.IdentExpr{Value: "unknown"}
	result := literal.KeyTypeFromExpr(expr, nil)
	if result != nil {
		t.Error("expected nil for unknown ident without resolver")
	}
}

func TestKeyTypeFromExpr_NilConstResult(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "n" {
			return &flow.ConstValue{Kind: flow.ConstNil}
		}
		return nil
	}
	expr := &ast.IdentExpr{Value: "n"}
	result := literal.KeyTypeFromExpr(expr, constResolver)
	if result != nil {
		t.Error("expected nil for nil const")
	}
}

func TestTableMutatorAssignment_Fields(t *testing.T) {
	assign := flow.TableMutatorAssignment{
		Point: 1,
		Target: constraint.Path{
			Root:   "arr",
			Symbol: 42,
		},
		ValueType: typ.String,
	}
	if assign.Point != 1 {
		t.Errorf("expected point 1, got %d", assign.Point)
	}
	if assign.Target.Root != "arr" {
		t.Errorf("expected root 'arr', got '%s'", assign.Target.Root)
	}
	if assign.ValueType != typ.String {
		t.Errorf("expected String, got %v", assign.ValueType)
	}
}

func TestTableMutatorAssignment_WithKeyInfo(t *testing.T) {
	assign := flow.TableMutatorAssignment{
		Point: 1,
		Target: constraint.Path{
			Root:   "obj",
			Symbol: 42,
		},
		KeyVar:    "key",
		KeySymbol: 10,
		KeyType:   typ.String,
		ValueType: typ.Integer,
	}
	if assign.KeyVar != "key" {
		t.Errorf("expected key var 'key', got '%s'", assign.KeyVar)
	}
	if assign.KeySymbol != 10 {
		t.Errorf("expected key symbol 10, got %d", assign.KeySymbol)
	}
}

func TestTableMutatorAssignment_WithValuePath(t *testing.T) {
	assign := flow.TableMutatorAssignment{
		Point: 1,
		Target: constraint.Path{
			Root:   "arr",
			Symbol: 42,
		},
		ValuePath: constraint.Path{
			Root:   "val",
			Symbol: 100,
		},
		ValueType: typ.String,
	}
	if assign.ValuePath.Root != "val" {
		t.Errorf("expected value path root 'val', got '%s'", assign.ValuePath.Root)
	}
}

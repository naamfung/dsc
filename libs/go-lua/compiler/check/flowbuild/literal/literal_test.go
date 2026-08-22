package literal_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/literal"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIsNilExpr_NilExpr(t *testing.T) {
	if !literal.IsNilExpr(&ast.NilExpr{}) {
		t.Error("expected true for NilExpr")
	}
}

func TestIsNilExpr_NonNil(t *testing.T) {
	tests := []ast.Expr{
		&ast.StringExpr{Value: "hello"},
		&ast.NumberExpr{Value: "42"},
		&ast.TrueExpr{},
		&ast.FalseExpr{},
		&ast.IdentExpr{Value: "x"},
	}
	for _, expr := range tests {
		if literal.IsNilExpr(expr) {
			t.Errorf("expected false for %T", expr)
		}
	}
}

func TestFromExpr_StringExpr(t *testing.T) {
	expr := &ast.StringExpr{Value: "hello"}
	lit, ok := literal.FromExpr(expr)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Fatal("expected non-nil literal")
	}
	if lit.Base != kind.String {
		t.Errorf("expected string kind, got %v", lit.Base)
	}
}

func TestFromExpr_NumberExpr_Integer(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	lit, ok := literal.FromExpr(expr)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Fatal("expected non-nil literal")
	}
	if lit.Base != kind.Integer {
		t.Errorf("expected integer kind, got %v", lit.Base)
	}
}

func TestFromExpr_NumberExpr_Float(t *testing.T) {
	expr := &ast.NumberExpr{Value: "3.14"}
	lit, ok := literal.FromExpr(expr)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Fatal("expected non-nil literal")
	}
	if lit.Base != kind.Number {
		t.Errorf("expected number kind, got %v", lit.Base)
	}
}

func TestFromExpr_NumberExpr_ZeroFloat(t *testing.T) {
	expr := &ast.NumberExpr{Value: "0.0"}
	lit, ok := literal.FromExpr(expr)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Fatal("expected non-nil literal")
	}
	if lit.Base != kind.Number {
		t.Errorf("expected number kind for 0.0, got %v", lit.Base)
	}
}

func TestFromExpr_TrueExpr(t *testing.T) {
	lit, ok := literal.FromExpr(&ast.TrueExpr{})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit != typ.True {
		t.Error("expected typ.True")
	}
}

func TestFromExpr_FalseExpr(t *testing.T) {
	lit, ok := literal.FromExpr(&ast.FalseExpr{})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit != typ.False {
		t.Error("expected typ.False")
	}
}

func TestFromExpr_Unsupported(t *testing.T) {
	_, ok := literal.FromExpr(&ast.IdentExpr{Value: "x"})
	if ok {
		t.Error("expected ok=false for ident expr")
	}

	_, ok = literal.FromExpr(&ast.NilExpr{})
	if ok {
		t.Error("expected ok=false for nil expr")
	}
}

func TestFromExprWithConst_DirectLiteral(t *testing.T) {
	lit, ok := literal.FromExprWithConst(&ast.StringExpr{Value: "direct"}, nil)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Error("expected non-nil literal")
	}
}

func TestFromExprWithConst_StringConst(t *testing.T) {
	resolver := func(name string) *flow.ConstValue {
		if name == "MY_CONST" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "resolved"}
		}
		return nil
	}
	lit, ok := literal.FromExprWithConst(&ast.IdentExpr{Value: "MY_CONST"}, resolver)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Error("expected non-nil literal")
	}
}

func TestFromExprWithConst_BoolConst(t *testing.T) {
	resolver := func(name string) *flow.ConstValue {
		if name == "TRUE_CONST" {
			return &flow.ConstValue{Kind: flow.ConstBool, Bool: true}
		}
		if name == "FALSE_CONST" {
			return &flow.ConstValue{Kind: flow.ConstBool, Bool: false}
		}
		return nil
	}

	lit, ok := literal.FromExprWithConst(&ast.IdentExpr{Value: "TRUE_CONST"}, resolver)
	if !ok || lit != typ.True {
		t.Error("expected typ.True for TRUE_CONST")
	}

	lit, ok = literal.FromExprWithConst(&ast.IdentExpr{Value: "FALSE_CONST"}, resolver)
	if !ok || lit != typ.False {
		t.Error("expected typ.False for FALSE_CONST")
	}
}

func TestFromExprWithConst_IntConst(t *testing.T) {
	resolver := func(name string) *flow.ConstValue {
		if name == "INT_CONST" {
			return &flow.ConstValue{Kind: flow.ConstInt, Int: 100}
		}
		return nil
	}
	lit, ok := literal.FromExprWithConst(&ast.IdentExpr{Value: "INT_CONST"}, resolver)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Error("expected non-nil literal")
	}
}

func TestFromExprWithConst_FloatConstValue(t *testing.T) {
	resolver := func(name string) *flow.ConstValue {
		if name == "FLOAT_CONST" {
			return &flow.ConstValue{Kind: flow.ConstFloat, Float: 2.718}
		}
		return nil
	}
	lit, ok := literal.FromExprWithConst(&ast.IdentExpr{Value: "FLOAT_CONST"}, resolver)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Error("expected non-nil literal")
	}
}

func TestFromExprWithSymType_NilResolvers(t *testing.T) {
	_, ok := literal.FromExprWithSymType(&ast.IdentExpr{Value: "x"}, nil, nil, nil, 0)
	if ok {
		t.Error("expected ok=false for nil resolvers")
	}
}

func TestFromExprWithSymType_FallsBackToConst(t *testing.T) {
	resolver := func(name string) *flow.ConstValue {
		if name == "CONST" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "const_value"}
		}
		return nil
	}
	lit, ok := literal.FromExprWithSymType(&ast.IdentExpr{Value: "CONST"}, resolver, nil, nil, 0)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Error("expected non-nil literal")
	}
}

func TestFromExprWithSymType_UsesSymResolver(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"myVar"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "myVar"}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	retStmt := fn.Stmts[0].(*ast.ReturnStmt)
	ident := retStmt.Exprs[0].(*ast.IdentExpr)

	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		t.Skip("cannot resolve symbol")
	}

	symResolver := func(p cfg.Point, s cfg.SymbolID) (typ.Type, bool) {
		if s == sym {
			return typ.LiteralString("fromSymResolver"), true
		}
		return nil, false
	}

	lit, ok := literal.FromExprWithSymType(ident, nil, bindings, symResolver, 1)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if lit == nil {
		t.Error("expected non-nil literal")
	}
}

func TestKeyTypeFromExpr_NilExpr(t *testing.T) {
	result := literal.KeyTypeFromExpr(nil, nil)
	if result != nil {
		t.Error("expected nil for nil expr")
	}
}

func TestKeyTypeFromExpr_StringLiteral(t *testing.T) {
	result := literal.KeyTypeFromExpr(&ast.StringExpr{Value: "key"}, nil)
	if result != typ.String {
		t.Error("expected typ.String")
	}
}

func TestKeyTypeFromExpr_IntegerLiteral(t *testing.T) {
	result := literal.KeyTypeFromExpr(&ast.NumberExpr{Value: "5"}, nil)
	if result != typ.Integer {
		t.Error("expected typ.Integer")
	}
}

func TestKeyTypeFromExpr_BoolLiteral(t *testing.T) {
	result := literal.KeyTypeFromExpr(&ast.TrueExpr{}, nil)
	if result != typ.Boolean {
		t.Error("expected typ.Boolean")
	}
}

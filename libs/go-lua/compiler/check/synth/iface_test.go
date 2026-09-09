package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFieldAccessResult_ZeroValue(t *testing.T) {
	var r FieldAccessResult
	if r.Found {
		t.Fatal("expected Found to be false")
	}
	if r.SkipCheck {
		t.Fatal("expected SkipCheck to be false")
	}
	if r.NotIndexable {
		t.Fatal("expected NotIndexable to be false")
	}
	if r.Type != nil {
		t.Fatal("expected Type to be nil")
	}
}

func TestFieldAccessResult_WithValues(t *testing.T) {
	r := FieldAccessResult{
		Type:         typ.String,
		Found:        true,
		SkipCheck:    true,
		NotIndexable: false,
	}
	if !r.Found {
		t.Fatal("expected Found to be true")
	}
	if !r.SkipCheck {
		t.Fatal("expected SkipCheck to be true")
	}
	if r.NotIndexable {
		t.Fatal("expected NotIndexable to be false")
	}
	if r.Type != typ.String {
		t.Fatal("expected Type to be string")
	}
}

func TestExprSynthType(t *testing.T) {
	synth := api.ExprSynth(func(e ast.Expr, p cfg.Point) typ.Type {
		return typ.Integer
	})
	result := synth(&ast.NumberExpr{Value: "42"}, 0)
	if result != typ.Integer {
		t.Fatal("expected integer type")
	}
}

func TestExprSynthType_NilExpr(t *testing.T) {
	synth := api.ExprSynth(func(e ast.Expr, p cfg.Point) typ.Type {
		if e == nil {
			return typ.Nil
		}
		return typ.Unknown
	})
	result := synth(nil, 0)
	if result != typ.Nil {
		t.Fatal("expected nil type for nil expr")
	}
}

func TestBaseSynth_Interface(t *testing.T) {
	var _ api.BaseSynth = (*mockBaseSynth)(nil)
}

func TestSynth_Interface(t *testing.T) {
	var _ Synth = (*mockSynth)(nil)
}

func TestLiteralSynth_Interface(t *testing.T) {
	var _ LiteralSynth = (*mockLiteralSynth)(nil)
}

type mockBaseSynth struct{}

func (m *mockBaseSynth) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	return typ.Unknown
}

func (m *mockBaseSynth) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	return typ.Unknown
}

func (m *mockBaseSynth) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	return nil
}

func (m *mockBaseSynth) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return nil
}

func (m *mockBaseSynth) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	return nil
}

func (m *mockBaseSynth) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	return nil
}

func (m *mockBaseSynth) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	return typ.Unknown
}

func (m *mockBaseSynth) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	return nil
}

type mockSynth struct {
	mockBaseSynth
}

func (m *mockSynth) ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return nil
}

func (m *mockSynth) ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
	return typ.Unknown
}

func (m *mockSynth) Narrow() api.BaseSynth {
	return &m.mockBaseSynth
}

func (m *mockSynth) Method(t typ.Type, name string) (typ.Type, bool) {
	return nil, false
}

func (m *mockSynth) Field(t typ.Type, name string) (typ.Type, bool) {
	return nil, false
}

func (m *mockSynth) SynthWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	return typ.Unknown
}

func (m *mockSynth) ResolveFieldAccess(fullExpr *ast.AttrGetExpr, objType typ.Type, fieldName string, p cfg.Point) FieldAccessResult {
	return FieldAccessResult{}
}

func (m *mockSynth) CallQuery() core.TypeOps {
	return nil
}

func (m *mockSynth) AllowReturnTransforms() bool {
	return false
}

func (m *mockSynth) Context() *db.QueryContext {
	return nil
}

type mockLiteralSynth struct{}

func (m *mockLiteralSynth) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	return typ.Unknown
}

func (m *mockLiteralSynth) SynthFunctionTypeWithExpected(fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function) *typ.Function {
	return nil
}

func (m *mockLiteralSynth) Scopes() api.ScopeMap {
	return nil
}

func (m *mockLiteralSynth) Entry() cfg.Point {
	return 0
}

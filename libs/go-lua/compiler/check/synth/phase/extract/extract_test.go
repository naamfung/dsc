package extract

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

func TestSynth_Interface(t *testing.T) {
	var _ Synth = (*mockSynthInterface)(nil)
}

type mockSynthInterface struct{}

func (m *mockSynthInterface) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	return typ.Unknown
}

func (m *mockSynthInterface) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	return typ.Unknown
}

func (m *mockSynthInterface) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	return nil
}

func (m *mockSynthInterface) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return nil
}

func (m *mockSynthInterface) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	return nil
}

func (m *mockSynthInterface) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	return nil
}

func (m *mockSynthInterface) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	return typ.Unknown
}

func (m *mockSynthInterface) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	return nil
}

func (m *mockSynthInterface) ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return nil
}

func (m *mockSynthInterface) ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
	return typ.Unknown
}

func (m *mockSynthInterface) ExpandValuesWithSpecTypes(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	return nil
}

func (m *mockSynthInterface) InferIterVarsWithSpecTypes(exprs []ast.Expr, count int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	return nil
}

func (m *mockSynthInterface) SynthExprAt(expr ast.Expr, p cfg.Point, sc *scope.State) typ.Type {
	return typ.Unknown
}

func (m *mockSynthInterface) Method(t typ.Type, name string) (typ.Type, bool) {
	return nil, false
}

func (m *mockSynthInterface) Field(t typ.Type, name string) (typ.Type, bool) {
	return nil, false
}

func (m *mockSynthInterface) AllowReturnTransforms() bool {
	return false
}

func (m *mockSynthInterface) Context() *db.QueryContext {
	return nil
}

func (m *mockSynthInterface) Narrow() api.BaseSynth {
	return nil
}

func (m *mockSynthInterface) SynthWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	return typ.Unknown
}

func (m *mockSynthInterface) CallQuery() core.TypeOps {
	return nil
}

func TestSpecTypes(t *testing.T) {
	specTypes := make(api.SpecTypes)
	sym := cfg.SymbolID(1)
	specTypes[sym] = typ.String

	if specTypes[sym] != typ.String {
		t.Fatal("expected string type")
	}
}

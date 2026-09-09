package hooks

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSynth_InterfaceExists(t *testing.T) {
	var _ api.Synth = (*synthImpl)(nil)
}

func TestBaseSynth_InterfaceExists(t *testing.T) {
	var _ api.BaseSynth = (*baseSynthImpl)(nil)
}

func TestFlowQuery_InterfaceExists(t *testing.T) {
	var _ api.FlowQuery = (*flowQueryImpl)(nil)
}

type synthImpl struct{}

func (s *synthImpl) TypeOf(expr ast.Expr, p cfg.Point) typ.Type { return nil }
func (s *synthImpl) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	return nil
}
func (s *synthImpl) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type { return nil }
func (s *synthImpl) SynthWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	return nil
}
func (s *synthImpl) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type { return nil }
func (s *synthImpl) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	return nil
}
func (s *synthImpl) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function { return nil }
func (s *synthImpl) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	return nil
}
func (s *synthImpl) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	return nil
}
func (s *synthImpl) Field(t typ.Type, name string) (typ.Type, bool)  { return nil, false }
func (s *synthImpl) Method(t typ.Type, name string) (typ.Type, bool) { return nil, false }
func (s *synthImpl) CallQuery() core.TypeOps                         { return nil }
func (s *synthImpl) Context() *db.QueryContext                       { return nil }
func (s *synthImpl) ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return nil
}
func (s *synthImpl) ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
	return nil
}
func (s *synthImpl) Narrow() api.BaseSynth       { return nil }
func (s *synthImpl) AllowReturnTransforms() bool { return false }

type baseSynthImpl struct{}

func (b *baseSynthImpl) TypeOf(expr ast.Expr, p cfg.Point) typ.Type { return nil }
func (b *baseSynthImpl) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	return nil
}
func (b *baseSynthImpl) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type { return nil }
func (b *baseSynthImpl) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return nil
}
func (b *baseSynthImpl) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	return nil
}
func (b *baseSynthImpl) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	return nil
}
func (b *baseSynthImpl) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type { return nil }
func (b *baseSynthImpl) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	return nil
}

type flowQueryImpl struct{}

func (f *flowQueryImpl) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{}
}
func (f *flowQueryImpl) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type { return nil }
func (f *flowQueryImpl) ExcludesTypeAt(p cfg.Point, path constraint.Path, declared typ.Type) bool {
	return false
}

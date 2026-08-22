package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Interface compliance tests

func TestBaseSynth_InterfaceDefinition(t *testing.T) {
	var _ BaseSynth = (*mockBaseSynth)(nil)
}

func TestSynth_InterfaceDefinition(t *testing.T) {
	var _ Synth = (*mockSynth)(nil)
}

func TestFlowQuery_InterfaceDefinition(t *testing.T) {
	var _ FlowQuery = (*mockFlowQuery)(nil)
}

func TestFlowOps_InterfaceDefinition(t *testing.T) {
	var _ FlowOps = (*mockFlowOps)(nil)
}

func TestLiteralSynth_InterfaceDefinition(t *testing.T) {
	var _ LiteralSynth = (*mockLiteralSynth)(nil)
}

func TestSynthAPI_InterfaceDefinition(t *testing.T) {
	var _ SynthAPI = (*mockSynthAPI)(nil)
}

// Mock implementations for interface compliance

type mockBaseSynth struct{}

func (m *mockBaseSynth) TypeOf(ast.Expr, cfg.Point) typ.Type                        { return nil }
func (m *mockBaseSynth) TypeOfWithExpected(ast.Expr, cfg.Point, typ.Type) typ.Type  { return nil }
func (m *mockBaseSynth) MultiTypeOf(ast.Expr, cfg.Point) []typ.Type                 { return nil }
func (m *mockBaseSynth) FunctionType(*ast.FunctionExpr, *scope.State) *typ.Function { return nil }
func (m *mockBaseSynth) ExpandValues([]ast.Expr, int, cfg.Point) []typ.Type         { return nil }
func (m *mockBaseSynth) InferIterVars([]ast.Expr, int, cfg.Point) []typ.Type        { return nil }
func (m *mockBaseSynth) ResolveType(ast.TypeExpr, *scope.State) typ.Type            { return nil }
func (m *mockBaseSynth) ResolveReturnTypes([]ast.TypeExpr, *scope.State) []typ.Type { return nil }

type mockSynth struct{ mockBaseSynth }

func (m *mockSynth) ResolveFunctionSignature(*ast.FunctionExpr, *scope.State) *typ.Function {
	return nil
}
func (m *mockSynth) ResolveTypeDef(string, ast.TypeExpr, []ast.TypeParamExpr, *scope.State) typ.Type {
	return nil
}
func (m *mockSynth) Narrow() BaseSynth                                        { return nil }
func (m *mockSynth) Method(typ.Type, string) (typ.Type, bool)                 { return nil, false }
func (m *mockSynth) Field(typ.Type, string) (typ.Type, bool)                  { return nil, false }
func (m *mockSynth) SynthWithExpected(ast.Expr, cfg.Point, typ.Type) typ.Type { return nil }
func (m *mockSynth) CallQuery() core.TypeOps                                  { return nil }
func (m *mockSynth) AllowReturnTransforms() bool                              { return false }
func (m *mockSynth) Context() *db.QueryContext                                { return nil }

type mockFlowQuery struct{}

func (m *mockFlowQuery) EffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{}
}
func (m *mockFlowQuery) NarrowedTypeAt(cfg.Point, constraint.Path) typ.Type       { return nil }
func (m *mockFlowQuery) ExcludesTypeAt(cfg.Point, constraint.Path, typ.Type) bool { return false }

type mockFlowOps struct{}

func (m *mockFlowOps) NarrowedTypeAt(cfg.Point, constraint.Path) typ.Type { return nil }
func (m *mockFlowOps) BoundsAt(cfg.Point, string) (int64, int64, bool)    { return 0, 0, false }
func (m *mockFlowOps) ArrayLenBoundAt(cfg.Point, string) (string, bool)   { return "", false }
func (m *mockFlowOps) ArrayLenBoundWithOffsetAt(cfg.Point, string) (string, int64, bool) {
	return "", 0, false
}
func (m *mockFlowOps) IsPointDead(cfg.Point) bool                                { return false }
func (m *mockFlowOps) HasKeyOf(cfg.Point, constraint.Path, constraint.Path) bool { return false }

type mockLiteralSynth struct{}

func (m *mockLiteralSynth) TypeOf(ast.Expr, cfg.Point) typ.Type { return nil }
func (m *mockLiteralSynth) SynthFunctionTypeWithExpected(*ast.FunctionExpr, *scope.State, *typ.Function) *typ.Function {
	return nil
}
func (m *mockLiteralSynth) Scopes() ScopeMap { return nil }
func (m *mockLiteralSynth) Entry() cfg.Point { return 0 }

type mockSynthAPI struct{}

func (m *mockSynthAPI) TypeOf(ast.Expr, cfg.Point) typ.Type                 { return nil }
func (m *mockSynthAPI) ExpandValues([]ast.Expr, int, cfg.Point) []typ.Type  { return nil }
func (m *mockSynthAPI) InferIterVars([]ast.Expr, int, cfg.Point) []typ.Type { return nil }
func (m *mockSynthAPI) ExpandValuesWithSpecTypes([]ast.Expr, int, cfg.Point, SpecTypes) []typ.Type {
	return nil
}
func (m *mockSynthAPI) InferIterVarsWithSpecTypes([]ast.Expr, int, cfg.Point, SpecTypes) []typ.Type {
	return nil
}

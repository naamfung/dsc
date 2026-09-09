package sibling_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/sibling"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConstraintsForIdent_NilIdentParam(t *testing.T) {
	result := sibling.ConstraintsForIdent(nil, 0, nil, true)
	if result != nil {
		t.Error("expected nil for nil ident")
	}
}

func TestConstraintsForIdent_NilInputsParam(t *testing.T) {
	result := sibling.ConstraintsForIdent(&ast.IdentExpr{Value: "x"}, 0, nil, true)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestConstraintsForIdent_NilSiblingAssignmentsMap(t *testing.T) {
	inputs := &flow.Inputs{
		SiblingAssignments: nil,
	}
	result := sibling.ConstraintsForIdent(&ast.IdentExpr{Value: "x"}, 0, inputs, true)
	if result != nil {
		t.Error("expected nil for nil sibling assignments")
	}
}

func TestConstraintsForIdent_NilGraphInInputs(t *testing.T) {
	inputs := &flow.Inputs{
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
		Graph:              nil,
	}
	result := sibling.ConstraintsForIdent(&ast.IdentExpr{Value: "x"}, 0, inputs, true)
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestConstraintsForSymbol_ZeroSymbolID(t *testing.T) {
	result := sibling.ConstraintsForSymbol(0, 1, nil, true, nil)
	if result != nil {
		t.Error("expected nil for zero symbol")
	}
}

func TestConstraintsForSymbol_NilInputsParam(t *testing.T) {
	result := sibling.ConstraintsForSymbol(1, 1, nil, true, nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestConstraintsForSymbol_NilSiblingAssignmentsMap(t *testing.T) {
	inputs := &flow.Inputs{
		SiblingAssignments: nil,
	}
	result := sibling.ConstraintsForSymbol(1, 1, inputs, true, nil)
	if result != nil {
		t.Error("expected nil for nil sibling assignments")
	}
}

func TestConstraintsForSymbol_NotFoundInMap(t *testing.T) {
	inputs := &flow.Inputs{
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	result := sibling.ConstraintsForSymbol(1, 1, inputs, true, nil)
	if result != nil {
		t.Error("expected nil when sibling not found")
	}
}

func TestConstraintsForSymbol_SingleVariableOnly(t *testing.T) {
	sym := cfg.SymbolID(1)
	inputs := &flow.Inputs{
		SiblingAssignments: map[flow.SiblingKey]*flow.SiblingAssignment{
			{Symbol: sym, VersionID: 1}: {
				Names:   []string{"val"},
				Symbols: []cfg.SymbolID{sym},
			},
		},
	}
	result := sibling.ConstraintsForSymbol(sym, 1, inputs, true, nil)
	if result != nil {
		t.Error("expected nil for single variable (needs at least 2)")
	}
}

func TestConstraintsForSymbol_LegacyNarrowingIsNil(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"val", "err"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) < 2 {
		t.Skip("need 2 param symbols")
	}
	valSym := paramSyms[0]
	errSym := paramSyms[1]

	inputs := &flow.Inputs{
		SiblingAssignments: map[flow.SiblingKey]*flow.SiblingAssignment{
			{Symbol: errSym, VersionID: 1}: {
				Names:   []string{"val", "err"},
				Symbols: []cfg.SymbolID{valSym, errSym},
				Types:   []typ.Type{typ.NewOptional(typ.String), typ.NewOptional(typ.String)},
			},
		},
	}

	result := sibling.ConstraintsForSymbol(errSym, 1, inputs, true, bindings)
	if result != nil {
		t.Fatalf("expected nil constraints for legacy narrowing, got %v", result)
	}
}

func TestConstraintsForSymbol_LegacyNarrowingNotNilResult(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"val", "err"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) < 2 {
		t.Skip("need 2 param symbols")
	}
	valSym := paramSyms[0]
	errSym := paramSyms[1]

	inputs := &flow.Inputs{
		SiblingAssignments: map[flow.SiblingKey]*flow.SiblingAssignment{
			{Symbol: errSym, VersionID: 1}: {
				Names:   []string{"val", "err"},
				Symbols: []cfg.SymbolID{valSym, errSym},
				Types:   []typ.Type{typ.NewOptional(typ.String), typ.NewOptional(typ.String)},
			},
		},
	}

	result := sibling.ConstraintsForSymbol(errSym, 1, inputs, false, bindings)
	if result != nil {
		t.Fatalf("expected nil constraints for legacy narrowing, got %v", result)
	}
}

func TestConstraintsForSymbol_CorrelatedConstraintsResult(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"val", "err"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) < 2 {
		t.Skip("need 2 param symbols")
	}
	valSym := paramSyms[0]
	errSym := paramSyms[1]

	inputs := &flow.Inputs{
		SiblingAssignments: map[flow.SiblingKey]*flow.SiblingAssignment{
			{Symbol: errSym, VersionID: 1}: {
				Names:   []string{"val", "err"},
				Symbols: []cfg.SymbolID{valSym, errSym},
				Correlations: []flow.ReturnCorrelation{
					{ValueIndex: 0, ErrorIndex: 1},
				},
			},
		},
	}

	result := sibling.ConstraintsForSymbol(errSym, 1, inputs, true, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(result))
	}
	if _, ok := result[0].(constraint.IsNil); !ok {
		t.Errorf("expected IsNil constraint, got %T", result[0])
	}
}

func TestConstraintsForSymbol_CorrelatedConstraintsNotNil(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"val", "err"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) < 2 {
		t.Skip("need 2 param symbols")
	}
	valSym := paramSyms[0]
	errSym := paramSyms[1]

	inputs := &flow.Inputs{
		SiblingAssignments: map[flow.SiblingKey]*flow.SiblingAssignment{
			{Symbol: errSym, VersionID: 1}: {
				Names:   []string{"val", "err"},
				Symbols: []cfg.SymbolID{valSym, errSym},
				Correlations: []flow.ReturnCorrelation{
					{ValueIndex: 0, ErrorIndex: 1},
				},
			},
		},
	}

	result := sibling.ConstraintsForSymbol(errSym, 1, inputs, false, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(result))
	}
	if _, ok := result[0].(constraint.NotNil); !ok {
		t.Errorf("expected NotNil constraint, got %T", result[0])
	}
}

func TestConstraintsForSymbol_CoCorrelatedConstraintsResult(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"a", "b"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) < 2 {
		t.Skip("need 2 param symbols")
	}
	aSym := paramSyms[0]
	bSym := paramSyms[1]

	inputs := &flow.Inputs{
		SiblingAssignments: map[flow.SiblingKey]*flow.SiblingAssignment{
			{Symbol: aSym, VersionID: 1}: {
				Names:   []string{"a", "b"},
				Symbols: []cfg.SymbolID{aSym, bSym},
				CoCorrelations: []flow.ReturnCorrelation{
					{ValueIndex: 0, ErrorIndex: 1},
				},
			},
		},
	}

	result := sibling.ConstraintsForSymbol(aSym, 1, inputs, true, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(result))
	}
	if _, ok := result[0].(constraint.NotNil); !ok {
		t.Errorf("expected NotNil constraint, got %T", result[0])
	}
}

func TestConstraintsForIdent_WithGraphAndUnknownIdent(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NilExpr{}}}},
	}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Skip("cannot build graph")
	}

	inputs := &flow.Inputs{
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
		Graph:              graph,
	}

	ident := &ast.IdentExpr{Value: "unknown"}
	result := sibling.ConstraintsForIdent(ident, graph.Entry(), inputs, true)
	if result != nil {
		t.Error("expected nil for unknown ident")
	}
}

func TestConstraintsForSymbol_GuardedTypeCorrelationOnTruthy(t *testing.T) {
	okSym := cfg.SymbolID(1)
	resSym := cfg.SymbolID(2)
	inputs := &flow.Inputs{
		SiblingAssignments: map[flow.SiblingKey]*flow.SiblingAssignment{
			{Symbol: okSym, VersionID: 1}: {
				Names:   []string{"ok", "result"},
				Symbols: []cfg.SymbolID{okSym, resSym},
				GuardedCorrelations: []flow.GuardedTypeCorrelation{
					{
						GuardIndex:    0,
						TargetIndex:   1,
						GuardOnTruthy: true,
						TargetType:    typ.String,
					},
				},
			},
		},
	}

	result := sibling.ConstraintsForSymbol(okSym, 1, inputs, true, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(result))
	}
	hasType, ok := result[0].(constraint.HasType)
	if !ok {
		t.Fatalf("expected HasType constraint, got %T", result[0])
	}
	if hasType.Path.Root != "result" || hasType.Path.Symbol != resSym {
		t.Fatalf("unexpected path in HasType constraint: %+v", hasType.Path)
	}
	if hasType.Type.Hash != typ.String.Hash() {
		t.Fatalf("expected string hash key, got %+v", hasType.Type)
	}

	result = sibling.ConstraintsForSymbol(okSym, 1, inputs, false, nil)
	if len(result) != 0 {
		t.Fatalf("expected no falsy constraints, got %v", result)
	}
}

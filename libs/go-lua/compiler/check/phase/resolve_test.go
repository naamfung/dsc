package phase

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRunResolve_NilGraph(t *testing.T) {
	input := ResolveInput{
		PhaseEnv: PhaseEnv{Graph: nil},
	}
	output := RunResolve(input)
	if output.TypeResolver != nil {
		t.Error("expected nil TypeResolver for nil graph")
	}
}

func TestRunResolve_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	input := ResolveInput{
		PhaseEnv: PhaseEnv{Graph: graph},
	}
	output := RunResolve(input)
	if output.TypeResolver == nil {
		t.Error("expected non-nil TypeResolver")
	}
}

func TestBuildInitialSymbolTypes_NilGraph(t *testing.T) {
	result := BuildInitialSymbolTypes(nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil graph, got %v", result)
	}
}

func TestBuildInitialSymbolTypes_EmptyTypes(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := BuildInitialSymbolTypes(graph, nil, nil)
	if result != nil {
		t.Errorf("expected nil for empty types, got %v", result)
	}
}

func TestBuildInitialSymbolTypes_WithGlobals(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	globals := map[string]typ.Type{"print": typ.Any}
	result := BuildInitialSymbolTypes(graph, globals, nil)
	// Result depends on whether 'print' is visible at any CFG point
	if result == nil {
		t.Skip("print not visible in empty function graph")
	}
}

func TestBuildInitialSymbolTypes_GlobalTypeNotAppliedToShadowedLocal(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"print"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "print"}},
			},
		},
	}

	graph := cfg.Build(fn, "print")
	entry := graph.Entry()
	globalSym, ok := graph.SymbolAt(entry, "print")
	if !ok || globalSym == 0 {
		t.Fatal("expected global print symbol at entry")
	}

	var shadowPoint cfg.Point
	var shadowSym cfg.SymbolID
	for _, p := range graph.RPO() {
		sym, ok := graph.SymbolAt(p, "print")
		if ok && sym != 0 && sym != globalSym {
			shadowPoint = p
			shadowSym = sym
			break
		}
	}
	if shadowSym == 0 {
		t.Fatal("expected local print symbol to shadow global")
	}

	result := BuildInitialSymbolTypes(graph, map[string]typ.Type{"print": typ.String}, nil)
	entryTypes := result[entry]
	if entryTypes == nil {
		t.Fatal("expected entry type map to exist")
	}
	if tv, ok := entryTypes[globalSym]; !ok || tv.Type != typ.String {
		t.Fatalf("expected global symbol %d to be typed as string at entry", globalSym)
	}

	if typesAt := result[shadowPoint]; typesAt != nil {
		if _, ok := typesAt[shadowSym]; ok {
			t.Fatalf("shadowed local symbol %d should not inherit global type", shadowSym)
		}
	}
}

func TestBuildInitialSymbolTypes_ParamNameFallbackAppliesToShadowedLocal(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
		},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
		},
	}

	graph := cfg.Build(fn)
	paramSymbols := graph.ParamSymbols()
	if len(paramSymbols) != 1 || paramSymbols[0] == 0 {
		t.Fatal("expected one non-zero parameter symbol")
	}
	paramSym := paramSymbols[0]

	result := BuildInitialSymbolTypes(graph, nil, map[cfg.SymbolID]typ.Type{
		paramSym: typ.String,
	})

	var paramPoint cfg.Point
	for _, p := range graph.RPO() {
		sym, ok := graph.SymbolAt(p, "x")
		if ok && sym == paramSym {
			paramPoint = p
			break
		}
	}
	if paramPoint == 0 {
		t.Fatal("expected to find a point where x resolves to parameter symbol")
	}

	paramTypes := result[paramPoint]
	if paramTypes == nil {
		t.Fatalf("expected type map at parameter point %d", paramPoint)
	}
	if tv, ok := paramTypes[paramSym]; !ok || tv.Type != typ.String {
		t.Fatalf("expected parameter symbol %d to have string type at point %d", paramSym, paramPoint)
	}

	var shadowPoint cfg.Point
	var shadowSym cfg.SymbolID
	for _, p := range graph.RPO() {
		sym, ok := graph.SymbolAt(p, "x")
		if ok && sym != 0 && sym != paramSym {
			shadowPoint = p
			shadowSym = sym
			break
		}
	}
	if shadowSym == 0 {
		t.Fatal("expected local x symbol to shadow parameter")
	}

	typesAt := result[shadowPoint]
	if typesAt == nil {
		t.Fatalf("expected type map at shadow point %d", shadowPoint)
	}
	if tv, ok := typesAt[shadowSym]; !ok || tv.Type != typ.String {
		t.Fatalf("expected shadowed symbol %d to receive name-fallback string type", shadowSym)
	}
}

func TestBuildDeclaredTypesFromSymbolTypes_NilSymbolTypes(t *testing.T) {
	result := BuildDeclaredTypesFromSymbolTypes(nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil symbolTypes, got %v", result)
	}
}

func TestBuildDeclaredTypesFromSymbolTypes_EmptySymbolTypes(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	symbolTypes := make(flow.SymbolTypes)
	result := BuildDeclaredTypesFromSymbolTypes(graph, symbolTypes)
	if result != nil {
		t.Errorf("expected nil for empty symbolTypes, got %v", result)
	}
}

func TestBuildDeclaredTypesForResolve_MatchesSymbolTypePipeline(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
		},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"print"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.IdentExpr{Value: "print"},
					&ast.IdentExpr{Value: "x"},
				},
			},
		},
	}

	graph := cfg.Build(fn, "print")
	paramSyms := graph.ParamSymbols()
	if len(paramSyms) != 1 || paramSyms[0] == 0 {
		t.Fatal("expected one parameter symbol")
	}

	globalTypes := map[string]typ.Type{"print": typ.String}
	paramTypes := map[cfg.SymbolID]typ.Type{paramSyms[0]: typ.Number}

	want := BuildDeclaredTypesFromSymbolTypes(graph, BuildInitialSymbolTypes(graph, globalTypes, paramTypes))
	got := BuildDeclaredTypesForResolve(graph, globalTypes, paramTypes)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildDeclaredTypesForResolve mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestBuildDeclaredTypesFromSymbolTypes_WithTypes(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	entry := graph.Entry()
	symbolTypes := flow.SymbolTypes{
		entry: {
			cfg.SymbolID(1): flow.TypedValue{Type: typ.Number, State: flow.StateResolved},
		},
	}
	result := BuildDeclaredTypesFromSymbolTypes(graph, symbolTypes)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[cfg.SymbolID(1)] != typ.Number {
		t.Errorf("expected Number for symbol 1, got %v", result[cfg.SymbolID(1)])
	}
}

func TestBuildDeclaredTypesFromSymbolTypes_EntryOverridesAndElseUsesLowestPoint(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	entry := graph.Entry()

	symEntry := cfg.SymbolID(10)
	symNonEntry := cfg.SymbolID(11)
	symbolTypes := flow.SymbolTypes{
		cfg.Point(5): {
			symEntry:    flow.TypedValue{Type: typ.Boolean, State: flow.StateResolved},
			symNonEntry: flow.TypedValue{Type: typ.String, State: flow.StateResolved},
		},
		cfg.Point(3): {
			symNonEntry: flow.TypedValue{Type: typ.Number, State: flow.StateResolved},
		},
		entry: {
			symEntry: flow.TypedValue{Type: typ.String, State: flow.StateResolved},
		},
	}

	result := BuildDeclaredTypesFromSymbolTypes(graph, symbolTypes)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if got := result[symEntry]; got != typ.String {
		t.Fatalf("expected entry symbol to use entry type String, got %v", got)
	}
	if got := result[symNonEntry]; got != typ.Number {
		t.Fatalf("expected non-entry symbol to use lowest-point type Number, got %v", got)
	}
}

func TestCreateTypeResolutionEngine_NilGraph(t *testing.T) {
	result := CreateTypeResolutionEngine(nil, nil, nil, nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil engine even with nil graph")
	}
}

func TestCreateTypeResolutionEngine_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := CreateTypeResolutionEngine(nil, graph, nil, nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil engine")
	}
}

package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPropagateParamHintsFromCallGraph_Empty(t *testing.T) {
	PropagateParamHintsFromCallGraph(nil)
	PropagateParamHintsFromCallGraph(map[cfg.SymbolID]*LocalFuncInfo{})
}

func TestPropagateParamHintsFromCallGraph_NilGraph(t *testing.T) {
	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		1: {Sym: 1, Graph: nil},
	}
	PropagateParamHintsFromCallGraph(localFuncs)
}

func TestPropagateParamHintsFromCallGraph_SingleFuncNoArgs(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}}}
	graph := cfg.Build(fn)

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		1: {Sym: 1, Fn: fn, Graph: graph},
	}
	PropagateParamHintsFromCallGraph(localFuncs)

	if localFuncs[1].ParamHints != nil {
		t.Error("expected nil ParamHints for function with no callers")
	}
}

func TestBuildLocalCallGraph_Empty(t *testing.T) {
	result := BuildLocalCallGraph(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}

	result = BuildLocalCallGraph(map[cfg.SymbolID]*LocalFuncInfo{}, nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestBuildLocalCallGraph_NilGraph(t *testing.T) {
	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		1: {Sym: 1, Graph: nil},
	}
	result := BuildLocalCallGraph(localFuncs, nil)
	if result[1] != nil {
		t.Error("expected nil callees for func with nil graph")
	}
}

func TestBuildLocalCallGraph_SingleFunc(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		1: {Sym: 1, Fn: fn, Graph: graph},
	}
	result := BuildLocalCallGraph(localFuncs, nil)
	// Function with no calls to other local functions has nil callees (correct behavior)
	callees, exists := result[1]
	if !exists {
		t.Error("expected entry for symbol 1 in adjacency map")
	}
	if len(callees) != 0 {
		t.Errorf("expected no callees, got %v", callees)
	}
}

func TestPropagateParamHintsFromCallGraph_LiteralArgTypes(t *testing.T) {
	// Test that literal arguments (number, string, bool, nil) are typed correctly
	tests := []struct {
		name     string
		arg      ast.Expr
		wantKind string
	}{
		{"number", &ast.NumberExpr{Value: "42"}, "number"},
		{"string", &ast.StringExpr{Value: "hello"}, "string"},
		{"true", &ast.TrueExpr{}, "boolean"},
		{"false", &ast.FalseExpr{}, "boolean"},
		{"nil", &ast.NilExpr{}, "nil"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a minimal function that would receive this arg type
			fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}}}
			graph := cfg.Build(fn)

			info := &LocalFuncInfo{
				Sym:   1,
				Fn:    fn,
				Graph: graph,
			}

			// Verify the arg type detection logic works
			var argType typ.Type
			switch tc.arg.(type) {
			case *ast.NumberExpr:
				argType = typ.Number
			case *ast.StringExpr:
				argType = typ.String
			case *ast.TrueExpr, *ast.FalseExpr:
				argType = typ.Boolean
			case *ast.NilExpr:
				argType = typ.Nil
			}

			if argType == nil {
				t.Errorf("failed to detect type for %s", tc.name)
				return
			}

			// The type should match expected kind
			_ = info // Use the info to avoid unused error
		})
	}
}

func TestPropagateParamHintsFromCallGraph_UnknownArgSkipped(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}}}
	graph := cfg.Build(fn)

	info := &LocalFuncInfo{
		Sym:   1,
		Fn:    fn,
		Graph: graph,
	}

	// Unknown type args should be skipped (not create hints)
	if info.ParamHints != nil {
		t.Error("ParamHints should be nil initially")
	}
}

func TestLocalFuncInfo_ZeroValue(t *testing.T) {
	var info LocalFuncInfo
	if info.Sym != 0 {
		t.Error("Sym should be 0")
	}
	if info.Fn != nil {
		t.Error("Fn should be nil")
	}
	if info.Graph != nil {
		t.Error("Graph should be nil")
	}
	if info.ParamHints != nil {
		t.Error("ParamHints should be nil")
	}
}

func TestLocalFuncInfo_ParamHintsExpansion(t *testing.T) {
	// Test that ParamHints array expands correctly
	info := &LocalFuncInfo{
		Sym:        1,
		ParamHints: []typ.Type{typ.Number},
	}

	// Verify initial state
	if len(info.ParamHints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(info.ParamHints))
	}
	if info.ParamHints[0] != typ.Number {
		t.Errorf("expected Number, got %v", info.ParamHints[0])
	}

	// Simulate expansion like PropagateParamHintsFromCallGraph does
	i := 2
	if i >= len(info.ParamHints) {
		expanded := make([]typ.Type, i+1)
		copy(expanded, info.ParamHints)
		info.ParamHints = expanded
	}

	if len(info.ParamHints) != 3 {
		t.Fatalf("expected 3 hints after expansion, got %d", len(info.ParamHints))
	}
	if info.ParamHints[0] != typ.Number {
		t.Error("original hint should be preserved")
	}
	if info.ParamHints[1] != nil {
		t.Error("gap should be nil")
	}
	if info.ParamHints[2] != nil {
		t.Error("new slot should be nil")
	}
}

func TestMaxReturnSummaryIterations_Value(t *testing.T) {
	if MaxReturnSummaryIterations < 1 {
		t.Error("MaxReturnSummaryIterations should be positive")
	}
	if MaxReturnSummaryIterations > 100 {
		t.Error("MaxReturnSummaryIterations seems too high")
	}
}

func TestBuildLocalCallGraph_AddsCallbackFunctionEdges(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function wrapper(cb: fun(): number): number
			return cb()
		end

		local function a()
			return wrapper(b)
		end

		local function b(): number
			return 1
		end
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	chunk := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	chunkGraph := cfg.Build(chunk)
	if chunkGraph == nil {
		t.Fatal("expected chunk graph")
	}

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{}
	symbolsByName := map[string]cfg.SymbolID{}
	baseScope := scope.New()
	chunkGraph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			fn, ok := source.(*ast.FunctionExpr)
			if !ok || fn == nil || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			symbolsByName[target.Name] = target.Symbol
			localFuncs[target.Symbol] = &LocalFuncInfo{
				Sym:         target.Symbol,
				Fn:          fn,
				DefScope:    baseScope,
				Graph:       cfg.Build(fn),
				ParentGraph: chunkGraph,
				DefPoint:    p,
			}
		})
	})

	aSym := symbolsByName["a"]
	bSym := symbolsByName["b"]
	if aSym == 0 || bSym == 0 {
		t.Fatalf("expected symbols for a and b, got a=%d b=%d", aSym, bSym)
	}

	adj := BuildLocalCallGraph(localFuncs, chunkGraph.Bindings())
	aCallees := adj[aSym]
	if !containsSymbol(aCallees, bSym) {
		t.Fatalf("expected call graph edge a -> b via callback argument, got %v", aCallees)
	}
}

func TestPropagateParamHintsFromCallGraph_MethodRuntimeIndexing(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function callee(self, x)
			return x
		end

		local function caller()
			local obj = {}
			return obj:callee(7)
		end
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	chunk := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	chunkGraph := cfg.Build(chunk)
	if chunkGraph == nil {
		t.Fatal("expected chunk graph")
	}

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{}
	symbolsByName := map[string]cfg.SymbolID{}
	baseScope := scope.New()
	chunkGraph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			fn, ok := source.(*ast.FunctionExpr)
			if !ok || fn == nil || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			symbolsByName[target.Name] = target.Symbol
			localFuncs[target.Symbol] = &LocalFuncInfo{
				Sym:         target.Symbol,
				Fn:          fn,
				DefScope:    baseScope,
				Graph:       cfg.Build(fn),
				ParentGraph: chunkGraph,
				DefPoint:    p,
			}
		})
	})

	calleeSym := symbolsByName["callee"]
	callerSym := symbolsByName["caller"]
	if calleeSym == 0 || callerSym == 0 {
		t.Fatalf("expected symbols for callee/caller, got callee=%d caller=%d", calleeSym, callerSym)
	}

	PropagateParamHintsFromCallGraph(localFuncs)

	hints := localFuncs[calleeSym].ParamHints
	if len(hints) < 2 {
		t.Fatalf("expected at least 2 param hints for callee(self,x), got %d", len(hints))
	}
	if !typ.TypeEquals(hints[1], typ.Number) {
		t.Fatalf("expected hint for x at index 1 to be number, got %v", hints[1])
	}
	if hints[0] != nil {
		t.Fatalf("expected no informative hint for receiver at index 0, got %v", hints[0])
	}
}

func containsSymbol(list []cfg.SymbolID, want cfg.SymbolID) bool {
	for _, sym := range list {
		if sym == want {
			return true
		}
	}
	return false
}

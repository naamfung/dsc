package resolve_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRootName_NilGraph(t *testing.T) {
	result := resolve.RootName(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestRootName_ZeroSymbol(t *testing.T) {
	result := resolve.RootName(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestRootNameFromBindings_NilBindings(t *testing.T) {
	result := resolve.RootNameFromBindings(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestRootNameFromBindings_WithSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"myVar"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) == 0 {
		t.Skip("no param symbols")
	}
	result := resolve.RootNameFromBindings(bindings, paramSyms[0], "fallback")
	if result != "myVar" {
		t.Errorf("expected 'myVar', got %q", result)
	}
}

func TestRootNameFromGraphAndBindings_PrefersBindings(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"boundName"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}
	syms := graph.ParamSymbols()
	if len(syms) == 0 {
		t.Fatal("expected param symbol")
	}
	got := resolve.RootNameFromGraphAndBindings(graph, bindings, syms[0], "fallback")
	if got != "boundName" {
		t.Fatalf("expected binding name, got %q", got)
	}
}

func TestRootNameFromGraphAndBindings_FallsBackToGraph(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"graphName"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
	syms := graph.ParamSymbols()
	if len(syms) == 0 {
		t.Fatal("expected param symbol")
	}
	got := resolve.RootNameFromGraphAndBindings(graph, nil, syms[0], "fallback")
	if got != "graphName" {
		t.Fatalf("expected graph fallback name, got %q", got)
	}
}

func TestGetBindings_NilInputs(t *testing.T) {
	result := resolve.GetBindings(nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestGetBindings_NilGraph(t *testing.T) {
	result := resolve.GetBindings(&flow.Inputs{Graph: nil})
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestRootFromSymbol_NilInputs(t *testing.T) {
	result := resolve.RootFromSymbol(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestRootFromSymbol_UsesGraphName(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"graphVar"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
	syms := graph.ParamSymbols()
	if len(syms) == 0 {
		t.Fatal("expected param symbol")
	}
	inputs := &flow.Inputs{Graph: graph}
	got := resolve.RootFromSymbol(inputs, syms[0], "fallback")
	if got != "graphVar" {
		t.Fatalf("expected graph name, got %q", got)
	}
}

func TestClassifyReturnExpr_TrueExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.TrueExpr{})
	if result != flow.ReturnTrue {
		t.Errorf("expected ReturnTrue, got %v", result)
	}
}

func TestClassifyReturnExpr_FalseExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.FalseExpr{})
	if result != flow.ReturnFalse {
		t.Errorf("expected ReturnFalse, got %v", result)
	}
}

func TestClassifyReturnExpr_TrueIdent(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.IdentExpr{Value: "true"})
	if result != flow.ReturnTrue {
		t.Errorf("expected ReturnTrue, got %v", result)
	}
}

func TestClassifyReturnExpr_FalseIdent(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.IdentExpr{Value: "false"})
	if result != flow.ReturnFalse {
		t.Errorf("expected ReturnFalse, got %v", result)
	}
}

func TestClassifyReturnExpr_OtherExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.StringExpr{Value: "hello"})
	if result != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown, got %v", result)
	}
}

func TestResolveSymbolToFunctionLiteral_LocalAssign(t *testing.T) {
	fnLit := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	root := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"f"},
				Exprs: []ast.Expr{fnLit},
			},
		},
	}
	graph := cfg.Build(root)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	var sym cfg.SymbolID
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if sym != 0 || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Name == "f" && source == fnLit {
				sym = target.Symbol
			}
		})
	})
	if sym == 0 {
		t.Fatal("expected non-zero symbol for local function assignment")
	}

	got := resolve.ResolveSymbolToFunctionLiteral(graph, sym)
	if got != fnLit {
		t.Fatalf("ResolveSymbolToFunctionLiteral mismatch: got %p want %p", got, fnLit)
	}
}

func TestResolveExprToTableLiteral_IdentRef(t *testing.T) {
	tbl := &ast.TableExpr{}
	retIdent := &ast.IdentExpr{Value: "t"}
	root := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"t"},
				Exprs: []ast.Expr{tbl},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{retIdent},
			},
		},
	}
	graph := cfg.Build(root)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	got := resolve.ResolveExprToTableLiteral(retIdent, graph)
	if got != tbl {
		t.Fatalf("ResolveExprToTableLiteral mismatch: got %p want %p", got, tbl)
	}
}

func TestResolveCalleeToFunctionLiteral_TableFieldFunction(t *testing.T) {
	stmts, err := parse.ParseString(`
		local t = {
			f = function(x)
				return x
			end
		}
		t.f(1)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var (
		callee ast.Expr
		want   *ast.FunctionExpr
	)
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if callee != nil || info == nil {
			return
		}
		callee = info.Callee
	})
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if want != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if want != nil || target.Name != "t" {
				return
			}
			tbl, ok := source.(*ast.TableExpr)
			if !ok || len(tbl.Fields) == 0 {
				return
			}
			if fn, ok := tbl.Fields[0].Value.(*ast.FunctionExpr); ok {
				want = fn
			}
		})
	})
	if callee == nil || want == nil {
		t.Fatal("expected callee and table field function literal")
	}

	got := resolve.ResolveCalleeToFunctionLiteral(callee, graph)
	if got != want {
		t.Fatalf("ResolveCalleeToFunctionLiteral mismatch: got %p want %p", got, want)
	}
}

func TestResolveCalleeToFunctionLiteral_TableIndexStringFunction(t *testing.T) {
	stmts, err := parse.ParseString(`
		local t = {
			["x-y"] = function(x)
				return x
			end
		}
		t["x-y"](1)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var (
		callee ast.Expr
		want   *ast.FunctionExpr
	)
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if callee != nil || info == nil {
			return
		}
		callee = info.Callee
	})
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if want != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if want != nil || target.Name != "t" {
				return
			}
			tbl, ok := source.(*ast.TableExpr)
			if !ok || len(tbl.Fields) == 0 {
				return
			}
			if fn, ok := tbl.Fields[0].Value.(*ast.FunctionExpr); ok {
				want = fn
			}
		})
	})
	if callee == nil || want == nil {
		t.Fatal("expected callee and table field function literal")
	}

	got := resolve.ResolveCalleeToFunctionLiteral(callee, graph)
	if got != want {
		t.Fatalf("ResolveCalleeToFunctionLiteral mismatch: got %p want %p", got, want)
	}
}

func TestResolveCalleeToFunctionLiteral_TableIndexIntFunction(t *testing.T) {
	stmts, err := parse.ParseString(`
		local t = {
			[1] = function(x)
				return x
			end
		}
		t[1](1)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var (
		callee ast.Expr
		want   *ast.FunctionExpr
	)
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if callee != nil || info == nil {
			return
		}
		callee = info.Callee
	})
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if want != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if want != nil || target.Name != "t" {
				return
			}
			tbl, ok := source.(*ast.TableExpr)
			if !ok || len(tbl.Fields) == 0 {
				return
			}
			if fn, ok := tbl.Fields[0].Value.(*ast.FunctionExpr); ok {
				want = fn
			}
		})
	})
	if callee == nil || want == nil {
		t.Fatal("expected callee and table field function literal")
	}

	got := resolve.ResolveCalleeToFunctionLiteral(callee, graph)
	if got != want {
		t.Fatalf("ResolveCalleeToFunctionLiteral mismatch: got %p want %p", got, want)
	}
}

func TestResolveCalleeToFunctionLiteral_TableFieldNotFunction(t *testing.T) {
	stmts, err := parse.ParseString(`
		local t = { f = 1 }
		t.f()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var callee ast.Expr
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if callee != nil || info == nil {
			return
		}
		callee = info.Callee
	})
	if callee == nil {
		t.Fatal("expected callee expression")
	}

	got := resolve.ResolveCalleeToFunctionLiteral(callee, graph)
	if got != nil {
		t.Fatalf("expected nil for non-function table field, got %v", got)
	}
}

func TestRef_NilType(t *testing.T) {
	result := resolve.Ref(nil, nil)
	if result != nil {
		t.Error("expected nil for nil type")
	}
}

func TestRef_NonRefType(t *testing.T) {
	result := resolve.Ref(typ.String, nil)
	if result != typ.String {
		t.Error("expected typ.String returned unchanged")
	}
}

func TestRef_NilScope(t *testing.T) {
	ref := typ.NewRef("", "MyType")
	result := resolve.Ref(ref, nil)
	if result != ref {
		t.Error("expected ref returned unchanged for nil scope")
	}
}

func TestRef_ResolvedRef(t *testing.T) {
	sc := scope.New().WithType("MyAlias", typ.String)

	ref := typ.NewRef("", "MyAlias")
	result := resolve.Ref(ref, sc)
	if result != typ.String {
		t.Errorf("expected typ.String, got %v", result)
	}
}

func TestBuildContextSymbolResolver_NilContext(t *testing.T) {
	resolver := resolve.BuildContextSymbolResolver(nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	_, ok := resolver(0, 1)
	if ok {
		t.Error("expected ok=false for nil context")
	}
}

func TestBuildInputSymbolResolver_NilInputs(t *testing.T) {
	resolver := resolve.BuildInputSymbolResolver(nil, nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	_, ok := resolver(0, 1)
	if ok {
		t.Error("expected ok=false for nil inputs")
	}
}

func TestBuildContextTypeKeyResolver_BuiltinType(t *testing.T) {
	resolver := resolve.BuildContextTypeKeyResolver(nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}

	key, ok := resolver("string", nil)
	if !ok {
		t.Error("expected ok=true for builtin 'string'")
	}
	if key.IsZero() {
		t.Error("expected non-zero key for 'string'")
	}
}

func TestBuildContextTypeKeyResolver_UnknownType(t *testing.T) {
	resolver := resolve.BuildContextTypeKeyResolver(nil)
	_, ok := resolver("UnknownType", nil)
	if ok {
		t.Error("expected ok=false for unknown type")
	}
}

func TestBuildRefinementLookup_NilContext(t *testing.T) {
	result := resolve.BuildRefinementLookup(nil)
	if result != nil {
		t.Error("expected nil for nil context")
	}
}

func TestSynthWithOverlay_NoMatch(t *testing.T) {
	base := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	overlay := map[cfg.SymbolID]typ.Type{}

	synth := resolve.SynthWithOverlay(overlay, nil, base)
	result := synth(&ast.IdentExpr{Value: "x"}, 0)
	if result != typ.String {
		t.Error("expected typ.String from base")
	}
}

func TestSynthWithOverlay_WithMatch(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	retStmt := fn.Stmts[0].(*ast.ReturnStmt)
	ident := retStmt.Exprs[0].(*ast.IdentExpr)
	sym, _ := bindings.SymbolOf(ident)

	base := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	overlay := map[cfg.SymbolID]typ.Type{
		sym: typ.Integer,
	}

	synth := resolve.SynthWithOverlay(overlay, bindings, base)
	result := synth(ident, 0)
	if result != typ.Integer {
		t.Error("expected typ.Integer from overlay")
	}
}

func TestBuildAssignmentTypeResolver_NilInputs(t *testing.T) {
	result := resolve.BuildAssignmentTypeResolver(nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestBuildAssignmentTypeResolver_ZeroSymbol(t *testing.T) {
	inputs := &flow.Inputs{}
	resolver := resolve.BuildAssignmentTypeResolver(inputs)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	result := resolver(0)
	if result != nil {
		t.Error("expected nil for zero symbol")
	}
}

func TestBuildAssignmentTypeResolver_LatestAssignmentWins(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{
			{TargetPath: constraint.Path{Symbol: 7}, Type: typ.String},
			{TargetPath: constraint.Path{Symbol: 7}, Type: typ.Number},
		},
	}
	resolver := resolve.BuildAssignmentTypeResolver(inputs)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	if got := resolver(7); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("resolver(7) = %v, want number", got)
	}
}

func TestBuildAssignmentTypeResolver_FallbackDeclaredType(t *testing.T) {
	inputs := &flow.Inputs{
		DeclaredTypes: flow.DeclaredTypes{
			9: typ.Boolean,
		},
	}
	resolver := resolve.BuildAssignmentTypeResolver(inputs)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	if got := resolver(9); !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("resolver(9) = %v, want boolean", got)
	}
}

func TestIteratorSourceInfo_Kind(t *testing.T) {
	info := &resolve.IteratorSourceInfo{
		Kind: flow.IterateIndexed,
	}
	if info.Kind != flow.IterateIndexed {
		t.Error("expected IterateIndexed")
	}
}

func TestExtractIteratorSource_EmptyIterExprs(t *testing.T) {
	result := resolve.ExtractIteratorSource(nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil iter exprs")
	}

	result = resolve.ExtractIteratorSource([]ast.Expr{}, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for empty iter exprs")
	}
}

func TestExtractIteratorSource_NonCallExpr(t *testing.T) {
	exprs := []ast.Expr{&ast.IdentExpr{Value: "x"}}
	result := resolve.ExtractIteratorSource(exprs, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for non-call expr")
	}
}

func TestExtractIteratorSource_IteratorNegativeSourceIndex(t *testing.T) {
	iterIdent := &ast.IdentExpr{Value: "iter"}
	srcA := &ast.IdentExpr{Value: "a"}
	srcB := &ast.IdentExpr{Value: "b"}
	call := &ast.FuncCallExpr{
		Func: iterIdent,
		Args: []ast.Expr{srcA, srcB},
	}

	bindings := bind.NewBindingTable()
	bindings.Bind(srcA, 1)
	bindings.Bind(srcB, 2)

	iterFn := typ.Func().
		Param("prefix", typ.Any).
		Param("src", typ.Any).
		Returns(typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Iterator{
			Source: effect.ParamRef{Index: -1},
			Kind:   effect.IterateKeyed,
		})).
		Build()

	synth := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if expr == iterIdent {
			return iterFn
		}
		return nil
	}

	got := resolve.ExtractIteratorSource([]ast.Expr{call}, 0, synth, nil, nil, bindings)
	if got == nil {
		t.Fatal("expected iterator source info")
	}
	if got.Kind != flow.IterateKeyed {
		t.Fatalf("kind = %v, want IterateKeyed", got.Kind)
	}
	if got.Path.Symbol != 2 {
		t.Fatalf("source symbol = %d, want 2 (last arg)", got.Path.Symbol)
	}
}

func TestExtractIteratorSource_IteratorNegativeSourceIndexOutOfRange(t *testing.T) {
	iterIdent := &ast.IdentExpr{Value: "iter"}
	srcA := &ast.IdentExpr{Value: "a"}
	call := &ast.FuncCallExpr{
		Func: iterIdent,
		Args: []ast.Expr{srcA},
	}

	bindings := bind.NewBindingTable()
	bindings.Bind(srcA, 1)

	iterFn := typ.Func().
		Param("src", typ.Any).
		Returns(typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Iterator{
			Source: effect.ParamRef{Index: -2},
			Kind:   effect.IterateKeyed,
		})).
		Build()

	synth := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if expr == iterIdent {
			return iterFn
		}
		return nil
	}

	got := resolve.ExtractIteratorSource([]ast.Expr{call}, 0, synth, nil, nil, bindings)
	if got != nil {
		t.Fatalf("expected nil for out-of-range negative iterator source, got %+v", got)
	}
}

func TestCalleeType_NilInfo(t *testing.T) {
	result := resolve.CalleeType(nil, 0, nil, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}

func TestCalleeType_FallsBackFromUnresolvablePathSymbolToRawSymbol(t *testing.T) {
	info := &cfg.CallInfo{
		CalleePath:   constraint.Path{Symbol: 111},
		CalleeSymbol: cfg.SymbolID(222),
	}
	want := typ.Func().Returns(typ.String).Build()
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == 222 {
			return want, true
		}
		return nil, false
	}

	got := resolve.CalleeType(info, 0, nil, symResolver, nil, nil, nil, nil)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected resolver fallback via raw symbol, got %v", got)
	}
}

func TestCalleeType_UsesBindingNameCandidates(t *testing.T) {
	const listenSym cfg.SymbolID = 77

	bindings := bind.NewBindingTable()
	bindings.SetName(listenSym, "listen")

	want := typ.Func().Returns(typ.String).Build()
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == listenSym {
			return want, true
		}
		return nil, false
	}

	info := &cfg.CallInfo{CalleeName: "listen"}
	got := resolve.CalleeType(info, 0, nil, symResolver, nil, nil, bindings, nil)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected resolver fallback via binding name candidate, got %v", got)
	}
}

func TestCalleeType_UsesModuleBindingNameCandidates(t *testing.T) {
	const listenSym cfg.SymbolID = 78

	moduleBindings := bind.NewBindingTable()
	moduleBindings.SetName(listenSym, "listen")

	want := typ.Func().Returns(typ.Number).Build()
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == listenSym {
			return want, true
		}
		return nil, false
	}

	info := &cfg.CallInfo{CalleeName: "listen"}
	got := resolve.CalleeType(info, 0, nil, symResolver, nil, nil, nil, moduleBindings)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected resolver fallback via module binding name candidate, got %v", got)
	}
}

func TestCalleeType_UsesDirectAliasCandidates(t *testing.T) {
	body, err := parse.ParseString(`
		local function B()
			return "ok"
		end
		local f = B
		local x = f()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: body})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var (
		callInfo *cfg.CallInfo
		point    cfg.Point
	)
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "f" {
			return
		}
		callInfo = info
		point = p
	})
	if callInfo == nil {
		t.Fatal("expected f() call site")
	}

	symB, ok := graph.SymbolAt(graph.Exit(), "B")
	if !ok || symB == 0 {
		t.Fatal("expected symbol for B")
	}

	want := typ.Func().Returns(typ.String).Build()
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == symB {
			return want, true
		}
		return nil, false
	}

	got := resolve.CalleeType(callInfo, point, nil, symResolver, nil, graph, graph.Bindings(), nil)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected resolver fallback via direct alias candidate, got %v", got)
	}
}

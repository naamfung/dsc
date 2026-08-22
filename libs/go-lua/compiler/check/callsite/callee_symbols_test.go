package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestCalleeSymbolCandidates_UsesCanonicalAndNameFallback(t *testing.T) {
	callee := &ast.IdentExpr{Value: "collect"}
	primary := bind.NewBindingTable()
	fallback := bind.NewBindingTable()

	const primarySym cfg.SymbolID = 11
	const fallbackNamedSym cfg.SymbolID = 22
	primary.Bind(callee, primarySym)
	fallback.SetName(fallbackNamedSym, "collect")

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:     callee,
		CalleeName: "collect",
	}, primary, fallback)
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2 (%v)", len(candidates), candidates)
	}
	if candidates[0] != primarySym || candidates[1] != fallbackNamedSym {
		t.Fatalf("candidates = %v, want [%d %d]", candidates, primarySym, fallbackNamedSym)
	}
}

func TestCalleeSymbolCandidates_DeduplicatesRawAndCanonical(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const sym cfg.SymbolID = 31
	primary.Bind(callee, sym)

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:       callee,
		CalleeName:   "f",
		CalleeSymbol: sym,
	}, primary, nil)
	if len(candidates) != 1 || candidates[0] != sym {
		t.Fatalf("candidates = %v, want [%d]", candidates, sym)
	}
}

func TestCalleeSymbolCandidates_IncludesPrimaryExprSymbolWhenRawDiffers(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const (
		rawSym  cfg.SymbolID = 51
		exprSym cfg.SymbolID = 52
	)
	primary.Bind(callee, exprSym)

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:       callee,
		CalleeSymbol: rawSym,
	}, primary, nil)
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2 (%v)", len(candidates), candidates)
	}
	if candidates[0] != rawSym || candidates[1] != exprSym {
		t.Fatalf("candidates = %v, want [%d %d]", candidates, rawSym, exprSym)
	}
}

func TestCalleeSymbolCandidates_IncludesPrimaryNameMatches(t *testing.T) {
	primary := bind.NewBindingTable()
	const byName cfg.SymbolID = 41
	primary.SetName(byName, "f")

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		CalleeName: "f",
	}, primary, nil)
	if len(candidates) != 1 || candidates[0] != byName {
		t.Fatalf("candidates = %v, want [%d]", candidates, byName)
	}
}

func TestCalleeSymbolCandidates_IncludeMethodSymbolFromReceiver(t *testing.T) {
	primary := bind.NewBindingTable()
	receiver := &ast.IdentExpr{Value: "T"}
	const (
		receiverSym cfg.SymbolID = 81
		rawSym      cfg.SymbolID = 82
	)
	primary.Bind(receiver, receiverSym)
	methodSym := primary.GetOrCreateFieldSymbol(receiverSym, "foo")

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Method:       "foo",
		Receiver:     receiver,
		CalleeSymbol: rawSym,
	}, primary, nil)
	if len(candidates) < 2 {
		t.Fatalf("expected at least raw and method candidates, got %v", candidates)
	}
	if candidates[0] != rawSym || candidates[1] != methodSym {
		t.Fatalf("candidates = %v, want prefix [%d %d]", candidates, rawSym, methodSym)
	}
}

func TestResolverCalleeSymbolCandidates_PrefersCalleePathSymbol(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const (
		pathSym cfg.SymbolID = 91
		exprSym cfg.SymbolID = 92
	)
	primary.Bind(callee, exprSym)

	candidates := ResolverCalleeSymbolCandidates(&cfg.CallInfo{
		Callee:     callee,
		CalleePath: constraint.Path{Symbol: pathSym},
	}, nil, primary, nil)
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2 (%v)", len(candidates), candidates)
	}
	if candidates[0] != pathSym || candidates[1] != exprSym {
		t.Fatalf("candidates = %v, want [%d %d]", candidates, pathSym, exprSym)
	}
}

func TestPreferredCalleeSymbol_PrefersMatchingCandidate(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const (
		rawSym  cfg.SymbolID = 61
		exprSym cfg.SymbolID = 62
	)
	primary.Bind(callee, exprSym)

	got := SelectPreferredSymbol(CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:       callee,
		CalleeSymbol: rawSym,
	}, primary, nil), func(sym cfg.SymbolID) bool { return sym == exprSym })
	if got != exprSym {
		t.Fatalf("preferred symbol = %d, want %d", got, exprSym)
	}
}

func TestPreferredCalleeSymbol_FallsBackToFirstCandidate(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const (
		rawSym  cfg.SymbolID = 71
		exprSym cfg.SymbolID = 72
	)
	primary.Bind(callee, exprSym)

	got := SelectPreferredSymbol(CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:       callee,
		CalleeSymbol: rawSym,
	}, primary, nil), func(sym cfg.SymbolID) bool { return false })
	if got != rawSym {
		t.Fatalf("fallback symbol = %d, want %d", got, rawSym)
	}
}

func TestCallableCalleeSymbolCandidates_NilGraphFallsBackToBaseCandidates(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	fallback := bind.NewBindingTable()
	const (
		rawSym      cfg.SymbolID = 73
		primarySym  cfg.SymbolID = 74
		fallbackSym cfg.SymbolID = 75
	)
	primary.Bind(callee, primarySym)
	fallback.Bind(callee, fallbackSym)

	info := &cfg.CallInfo{
		Callee:       callee,
		CalleeName:   "f",
		CalleeSymbol: rawSym,
	}

	base := CalleeSymbolCandidates(info, primary, fallback)
	withAliases := CallableCalleeSymbolCandidates(info, nil, primary, fallback)
	if len(withAliases) != len(base) {
		t.Fatalf("len(withAliases) = %d, want %d", len(withAliases), len(base))
	}
	for i := range base {
		if withAliases[i] != base[i] {
			t.Fatalf("withAliases[%d] = %d, want %d (base=%v withAliases=%v)", i, withAliases[i], base[i], base, withAliases)
		}
	}
}

func TestCallableCalleeSymbolCandidates_IncludesDirectAliasSource(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function B()
			return 1
		end
		local f = B
		local x = f()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}

	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	var (
		callInfo  *cfg.CallInfo
		callPoint cfg.Point
	)
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Callee == nil || callInfo != nil {
			return
		}
		callPoint = p
		callInfo = info
	})
	if callInfo == nil {
		t.Fatal("expected callsite")
	}

	calleeSym := SymbolFromExpr(callInfo.Callee, bindings)
	if calleeSym == 0 {
		t.Fatal("expected non-zero callee symbol from expression")
	}
	aliasSym := graph.DirectAliasSymbol(calleeSym)
	if aliasSym == 0 {
		t.Fatal("expected non-zero direct alias symbol for local f = B")
	}

	if byName, ok := graph.SymbolAt(callPoint, "B"); ok && byName != 0 && byName != aliasSym {
		t.Fatalf("alias symbol = %d, want %d", aliasSym, byName)
	}

	candidates := CallableCalleeSymbolCandidates(callInfo, graph, bindings, nil)
	if len(candidates) < 2 {
		t.Fatalf("expected alias-expanded candidates, got %v", candidates)
	}
	if candidates[0] != calleeSym || candidates[1] != aliasSym {
		t.Fatalf("candidates = %v, want prefix [%d %d]", candidates, calleeSym, aliasSym)
	}
}

func TestPreferredCallableCalleeSymbol_PrefersAliasCandidate(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function B()
			return 1
		end
		local f = B
		local x = f()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}

	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "f" {
			return
		}
		callInfo = info
	})
	if callInfo == nil {
		t.Fatal("expected f() call site")
	}

	calleeSym := SymbolFromExpr(callInfo.Callee, bindings)
	if calleeSym == 0 {
		t.Fatal("expected callee symbol for f")
	}
	aliasSym := graph.DirectAliasSymbol(calleeSym)
	if aliasSym == 0 {
		t.Fatal("expected alias symbol for f")
	}

	got := SelectPreferredSymbol(CallableCalleeSymbolCandidates(callInfo, graph, bindings, nil), func(sym cfg.SymbolID) bool {
		return sym == aliasSym
	})
	if got != aliasSym {
		t.Fatalf("preferred symbol = %d, want %d", got, aliasSym)
	}
}

func TestCallableCalleeSymbolCandidates_ExpandsTransitiveAliasChain(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function Target()
			return 1
		end
		local a = Target
		local b = a
		local c = b
		local x = c()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}
	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	var (
		callInfo *cfg.CallInfo
		callSym  cfg.SymbolID
		rootSym  cfg.SymbolID
	)
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "c" {
			return
		}
		callInfo = info
		callSym, _ = graph.SymbolAt(p, "c")
		rootSym, _ = graph.SymbolAt(p, "Target")
	})
	if callInfo == nil {
		t.Fatal("expected c() call site")
	}
	if callSym == 0 || rootSym == 0 {
		t.Fatalf("expected non-zero symbols for c and Target, got c=%d Target=%d", callSym, rootSym)
	}

	candidates := CallableCalleeSymbolCandidates(callInfo, graph, bindings, nil)
	if !containsSymbol(candidates, callSym) {
		t.Fatalf("candidates = %v, missing call symbol %d", candidates, callSym)
	}
	if !containsSymbol(candidates, rootSym) {
		t.Fatalf("candidates = %v, missing transitive alias root symbol %d", candidates, rootSym)
	}
}

func TestCallableCalleeSymbolCandidates_ResolvesMethodSymbolThroughAliasBase(t *testing.T) {
	stmts, err := parse.ParseString(`
		local T = {}
		function T.run(x: number): number
			return x + 1
		end
		local Alias = T
		local y = Alias:run(1)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}
	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Method != "run" {
			return
		}
		callInfo = info
	})
	if callInfo == nil {
		t.Fatal("expected Alias:run call site")
	}

	if _, ok := methodCalleeSymbolFromCall(bindings, nil, callInfo); ok {
		t.Fatal("expected non-alias method symbol resolution to miss alias receiver base")
	}
	methodSym, ok := methodCalleeSymbolFromCall(bindings, graph, callInfo)
	if !ok || methodSym == 0 {
		t.Fatal("expected alias-aware method symbol resolution")
	}

	candidates := CallableCalleeSymbolCandidates(callInfo, graph, bindings, nil)
	if !containsSymbol(candidates, methodSym) {
		t.Fatalf("candidates = %v, missing method symbol %d", candidates, methodSym)
	}
}

func TestMethodCalleeSymbolFromCall_ResolvesDirectBaseWithoutGraph(t *testing.T) {
	stmts, err := parse.ParseString(`
		local T = {}
		function T.run(x: number): number
			return x + 1
		end
		local y = T:run(1)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}
	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Method != "run" {
			return
		}
		callInfo = info
	})
	if callInfo == nil {
		t.Fatal("expected T:run call site")
	}

	methodSymNoGraph, ok := methodCalleeSymbolFromCall(bindings, nil, callInfo)
	if !ok || methodSymNoGraph == 0 {
		t.Fatal("expected direct-base method resolution to work without graph aliases")
	}
	methodSymWithGraph, ok := methodCalleeSymbolFromCall(bindings, graph, callInfo)
	if !ok || methodSymWithGraph == 0 {
		t.Fatal("expected method resolution with graph")
	}
	if methodSymWithGraph != methodSymNoGraph {
		t.Fatalf("method symbol mismatch without graph: got %d, with graph: %d", methodSymNoGraph, methodSymWithGraph)
	}
}

func containsSymbol(symbols []cfg.SymbolID, want cfg.SymbolID) bool {
	for _, sym := range symbols {
		if sym == want {
			return true
		}
	}
	return false
}

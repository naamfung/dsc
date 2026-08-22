package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCollectCalledNestedFieldAssignments(t *testing.T) {
	t.Run("nil graph returns empty map", func(t *testing.T) {
		result := CollectCalledNestedFieldAssignments(nil, nil, nil, nil)
		if len(result) != 0 {
			t.Error("expected empty result")
		}
	})
}

func TestCollectCalledNestedContainerMutatorAssignments(t *testing.T) {
	t.Run("nil graph returns empty slice", func(t *testing.T) {
		result := CollectCalledNestedContainerMutatorAssignments(nil, nil, nil, nil)
		if len(result) != 0 {
			t.Error("expected empty result")
		}
	})
}

func TestRuntimeArgAt(t *testing.T) {
	t.Run("direct call positional mapping", func(t *testing.T) {
		a := &ast.NumberExpr{Value: "1"}
		b := &ast.NumberExpr{Value: "2"}
		info := &cfg.CallInfo{Args: []ast.Expr{a, b}}
		if got := checkcallsite.RuntimeArgAt(info, 0); got != a {
			t.Fatal("expected first arg at index 0")
		}
		if got := checkcallsite.RuntimeArgAt(info, -1); got != b {
			t.Fatal("expected last arg at index -1")
		}
	})

	t.Run("method call runtime mapping", func(t *testing.T) {
		recv := &ast.IdentExpr{Value: "self"}
		a := &ast.NumberExpr{Value: "1"}
		b := &ast.NumberExpr{Value: "2"}
		info := &cfg.CallInfo{
			Method:   "m",
			Receiver: recv,
			Args:     []ast.Expr{a, b},
		}
		if got := checkcallsite.RuntimeArgAt(info, 0); got != recv {
			t.Fatal("expected receiver at index 0 for method call")
		}
		if got := checkcallsite.RuntimeArgAt(info, 1); got != a {
			t.Fatal("expected first positional arg at runtime index 1")
		}
		if got := checkcallsite.RuntimeArgAt(info, -3); got != recv {
			t.Fatal("expected receiver from negative runtime index")
		}
	})
}

func TestCalledSymbolsFromCall_PrefersTrackedCanonicalSymbol(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "f"}
	const (
		rawSym     cfg.SymbolID = 101
		trackedSym cfg.SymbolID = 202
	)
	bindings.Bind(ident, trackedSym)

	info := &cfg.CallInfo{
		Callee:       ident,
		CalleeSymbol: rawSym,
	}

	got := calledSymbolsFromCall(info, 0, nil, bindings, nil, func(sym cfg.SymbolID) bool {
		return sym == trackedSym
	})

	if !got[trackedSym] {
		t.Fatalf("expected tracked canonical symbol %d to be selected, got %v", trackedSym, got)
	}
	if got[rawSym] {
		t.Fatalf("expected raw symbol %d to be excluded when tracked symbol is preferred, got %v", rawSym, got)
	}
}

func TestCalledSymbolsFromCall_UsesCalleeNameCandidatesWhenRawAndExprMissing(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "f"}
	const trackedSym cfg.SymbolID = 303
	bindings.Bind(ident, trackedSym)
	bindings.SetName(trackedSym, "f")

	info := &cfg.CallInfo{
		Callee:       nil,
		CalleeSymbol: 0,
		CalleeName:   "f",
	}

	got := calledSymbolsFromCall(info, 0, nil, bindings, nil, func(sym cfg.SymbolID) bool {
		return sym == trackedSym
	})
	if !got[trackedSym] {
		t.Fatalf("expected tracked symbol %d via callee-name candidates, got %v", trackedSym, got)
	}
}

func TestCalledSymbolsFromCall_UsesAliasExpandedCandidates(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function runner()
			return 1
		end
		local f = runner
		local _ = f()
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
	runnerSym, ok := graph.SymbolAt(graph.Exit(), "runner")
	if !ok || runnerSym == 0 {
		t.Fatal("expected symbol for runner")
	}

	var info *cfg.CallInfo
	graph.EachCallSite(func(_ cfg.Point, ci *cfg.CallInfo) {
		if ci == nil || ci.CalleeName != "f" {
			return
		}
		info = ci
	})
	if info == nil {
		t.Fatal("expected f() call site")
	}

	got := calledSymbolsFromCall(info, 0, graph, bindings, nil, func(sym cfg.SymbolID) bool {
		return sym == runnerSym
	})
	if !got[runnerSym] {
		t.Fatalf("expected tracked runner symbol %d via alias-expanded candidates, got %v", runnerSym, got)
	}
}

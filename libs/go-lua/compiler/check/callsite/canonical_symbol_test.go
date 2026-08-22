package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCanonicalSymbolFromExpr_PrefersCandidateByPredicate(t *testing.T) {
	ident := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	fallback := bind.NewBindingTable()
	primary.Bind(ident, 11)
	fallback.Bind(ident, 22)

	got := SelectPreferredSymbol(
		exprSymbolCandidates(ident, 33, primary, fallback),
		func(sym cfg.SymbolID) bool { return sym == 22 },
	)
	if got != 22 {
		t.Fatalf("CanonicalSymbolFromExpr(...) = %d, want 22", got)
	}
}

func TestCanonicalSymbolFromExpr_FallsBackToFirstNonZero(t *testing.T) {
	ident := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	primary.Bind(ident, 11)

	got := SelectPreferredSymbol(exprSymbolCandidates(ident, 0, primary, nil), nil)
	if got != 11 {
		t.Fatalf("CanonicalSymbolFromExpr(...) = %d, want 11", got)
	}
}

func TestCanonicalSymbolFromExpr_UsesFunctionLiteralSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{}
	primary := bind.NewBindingTable()
	fallback := bind.NewBindingTable()
	primary.SetFuncLitSymbol(fn, 41)
	fallback.SetFuncLitSymbol(fn, 42)

	got := SelectPreferredSymbol(
		exprSymbolCandidates(fn, 0, primary, fallback),
		func(sym cfg.SymbolID) bool { return sym == 42 },
	)
	if got != 42 {
		t.Fatalf("CanonicalSymbolFromExpr(function) = %d, want 42", got)
	}
}

func TestCanonicalSymbolFromExprWithAliases_PrefersDirectAliasCandidate(t *testing.T) {
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

	var callee *ast.IdentExpr
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "f" {
			return
		}
		if ident, ok := info.Callee.(*ast.IdentExpr); ok {
			callee = ident
		}
	})
	if callee == nil {
		t.Fatal("expected f() callsite callee ident")
	}

	raw := SymbolFromExpr(callee, bindings)
	if raw == 0 {
		t.Fatal("expected non-zero raw symbol for f")
	}
	alias := graph.DirectAliasSymbol(raw)
	if alias == 0 {
		t.Fatal("expected alias symbol for f")
	}

	got := CanonicalSymbolFromExprWithAliases(callee, raw, graph, bindings, nil, func(sym cfg.SymbolID) bool {
		return sym == alias
	})
	if got != alias {
		t.Fatalf("CanonicalSymbolFromExprWithAliases(...) = %d, want %d", got, alias)
	}
}

func TestCanonicalSymbolFromExprWithAliases_PrefersTransitiveAliasCandidate(t *testing.T) {
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
		callee *ast.IdentExpr
		root   cfg.SymbolID
	)
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "c" {
			return
		}
		if ident, ok := info.Callee.(*ast.IdentExpr); ok {
			callee = ident
		}
		root, _ = graph.SymbolAt(p, "Target")
	})
	if callee == nil || root == 0 {
		t.Fatalf("expected c() callee ident and Target symbol, got callee=%v root=%d", callee, root)
	}

	raw := SymbolFromExpr(callee, bindings)
	if raw == 0 {
		t.Fatal("expected non-zero raw symbol for c")
	}

	got := CanonicalSymbolFromExprWithAliases(callee, raw, graph, bindings, nil, func(sym cfg.SymbolID) bool {
		return sym == root
	})
	if got != root {
		t.Fatalf("CanonicalSymbolFromExprWithAliases(...) = %d, want %d", got, root)
	}
}

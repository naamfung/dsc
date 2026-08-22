package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func buildGraphForDirectAliasTest(t *testing.T, code string) *Graph {
	t.Helper()

	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	g := Build(&ast.FunctionExpr{Stmts: stmts})
	if g == nil {
		t.Fatal("expected graph")
	}
	return g
}

func symbolAtExit(t *testing.T, g *Graph, name string) SymbolID {
	t.Helper()

	sym, ok := g.SymbolAt(g.Exit(), name)
	if !ok || sym == 0 {
		t.Fatalf("expected symbol for %q at exit", name)
	}
	return sym
}

func TestDirectAliasSymbol_LocalAlias(t *testing.T) {
	g := buildGraphForDirectAliasTest(t, `
		local function B()
			return 1
		end
		local f = B
		return f()
	`)

	fSym := symbolAtExit(t, g, "f")
	bSym := symbolAtExit(t, g, "B")

	if got := g.DirectAliasSymbol(fSym); got != bSym {
		t.Fatalf("DirectAliasSymbol(f) = %d, want %d", got, bSym)
	}
}

func TestDirectAliasSymbol_ReassignmentDifferentSourceInvalidates(t *testing.T) {
	g := buildGraphForDirectAliasTest(t, `
		local function B()
			return 1
		end
		local function C()
			return 2
		end
		local f = B
		f = C
		return f()
	`)

	fSym := symbolAtExit(t, g, "f")
	if got := g.DirectAliasSymbol(fSym); got != 0 {
		t.Fatalf("DirectAliasSymbol(f) = %d, want 0 after reassignment", got)
	}
}

func TestDirectAliasSymbol_NonIdentifierSourceInvalidates(t *testing.T) {
	g := buildGraphForDirectAliasTest(t, `
		local function B()
			return 1
		end
		local f = B
		f = get_func()
		return f()
	`)

	fSym := symbolAtExit(t, g, "f")
	if got := g.DirectAliasSymbol(fSym); got != 0 {
		t.Fatalf("DirectAliasSymbol(f) = %d, want 0 for non-ident source", got)
	}
}

func TestEachAliasSymbol_VisitsTransitiveChain(t *testing.T) {
	g := buildGraphForDirectAliasTest(t, `
		local function Target()
			return 1
		end
		local a = Target
		local b = a
		local c = b
		return c()
	`)

	cSym := symbolAtExit(t, g, "c")
	bSym := symbolAtExit(t, g, "b")
	aSym := symbolAtExit(t, g, "a")
	targetSym := symbolAtExit(t, g, "Target")

	var got []SymbolID
	g.EachAliasSymbol(cSym, func(sym SymbolID) bool {
		got = append(got, sym)
		return false
	})

	want := []SymbolID{cSym, bSym, aSym, targetSym}
	if len(got) != len(want) {
		t.Fatalf("alias chain len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("alias chain[%d] = %d, want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestEachAliasSymbol_StopsOnCycle(t *testing.T) {
	g := &Graph{
		directAliases: map[SymbolID]SymbolID{
			1: 2,
			2: 1,
		},
	}

	var got []SymbolID
	g.EachAliasSymbol(1, func(sym SymbolID) bool {
		got = append(got, sym)
		return false
	})

	want := []SymbolID{1, 2}
	if len(got) != len(want) {
		t.Fatalf("alias chain len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("alias chain[%d] = %d, want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestEachAliasSymbol_NilGraphVisitsSeedOnly(t *testing.T) {
	var g *Graph
	var got []SymbolID
	g.EachAliasSymbol(42, func(sym SymbolID) bool {
		got = append(got, sym)
		return false
	})

	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("nil graph alias walk = %v, want [42]", got)
	}
}

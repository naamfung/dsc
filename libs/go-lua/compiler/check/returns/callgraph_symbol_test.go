package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCanonicalLocalSymbol_ResolvesStaticFieldPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(1001)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	localSym := bindings.GetOrCreateFieldSymbol(baseSym, "handlers.run")
	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.StringExpr{Value: "handlers"},
		},
		Key: &ast.StringExpr{Value: "run"},
	}

	got := canonicalLocalSymbol(localFuncs, nil, nil, bindings, expr, 0)
	if got != localSym {
		t.Fatalf("canonicalLocalSymbol(M.handlers.run) = %d, want %d", got, localSym)
	}
}

func TestCanonicalLocalSymbol_PrefersKnownLocalOverRaw(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(2001)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	localSym := bindings.GetOrCreateFieldSymbol(baseSym, "f")
	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	expr := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "f"}}
	rawNonLocal := cfg.SymbolID(9999)
	got := canonicalLocalSymbol(localFuncs, nil, nil, bindings, expr, rawNonLocal)
	if got != localSym {
		t.Fatalf("canonicalLocalSymbol should prefer local symbol %d, got %d", localSym, got)
	}
}

func TestCanonicalLocalCalleeSymbol_UsesCalleeNameFallback(t *testing.T) {
	bindings := bind.NewBindingTable()
	const localSym cfg.SymbolID = 3001
	bindings.SetName(localSym, "runner")

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	callInfo := &cfg.CallInfo{
		CalleeName: "runner",
	}
	got := canonicalLocalCalleeSymbol(localFuncs, nil, nil, bindings, callInfo)
	if got != localSym {
		t.Fatalf("canonicalLocalCalleeSymbol via name fallback = %d, want %d", got, localSym)
	}
}

func TestCanonicalLocalCalleeSymbol_PrefersKnownLocalOverRaw(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "f"}
	const (
		localSym    cfg.SymbolID = 4001
		rawNonLocal cfg.SymbolID = 4999
	)
	bindings.Bind(ident, localSym)

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	callInfo := &cfg.CallInfo{
		Callee:       ident,
		CalleeSymbol: rawNonLocal,
		CalleeName:   "f",
	}
	got := canonicalLocalCalleeSymbol(localFuncs, nil, nil, bindings, callInfo)
	if got != localSym {
		t.Fatalf("canonicalLocalCalleeSymbol should prefer local symbol %d, got %d", localSym, got)
	}
}

func TestCanonicalLocalCalleeSymbol_ResolvesMethodFromReceiverPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	recv := &ast.IdentExpr{Value: "T"}
	const (
		recvSym     cfg.SymbolID = 5001
		rawNonLocal cfg.SymbolID = 5999
	)
	bindings.Bind(recv, recvSym)
	bindings.SetName(recvSym, "T")
	methodSym := bindings.GetOrCreateFieldSymbol(recvSym, "foo")

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		methodSym: {Sym: methodSym},
	}

	callInfo := &cfg.CallInfo{
		Method:       "foo",
		Receiver:     recv,
		CalleeSymbol: rawNonLocal,
	}

	got := canonicalLocalCalleeSymbol(localFuncs, nil, nil, bindings, callInfo)
	if got != methodSym {
		t.Fatalf("canonicalLocalCalleeSymbol should resolve method symbol %d from receiver path, got %d", methodSym, got)
	}
}

func TestCanonicalLocalCalleeSymbol_UsesDirectAliasCandidates(t *testing.T) {
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

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		runnerSym: {Sym: runnerSym},
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

	got := canonicalLocalCalleeSymbol(localFuncs, graph, nil, bindings, callInfo)
	if got != runnerSym {
		t.Fatalf("canonicalLocalCalleeSymbol via alias-expanded candidates = %d, want %d", got, runnerSym)
	}
}

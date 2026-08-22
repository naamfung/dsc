package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestSymbolOrCreateFieldFromExpr_StaticFieldPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "state"}
	baseSym := cfg.SymbolID(11)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.StringExpr{Value: "users"},
		},
		Key: &ast.StringExpr{Value: "active"},
	}

	got := SymbolOrCreateFieldFromExpr(expr, bindings)
	if got == 0 {
		t.Fatal("expected field symbol")
	}
	want := bindings.GetOrCreateFieldSymbol(baseSym, "users.active")
	if got != want {
		t.Fatalf("SymbolOrCreateFieldFromExpr(...) = %d, want %d", got, want)
	}
}

func TestRuntimeArgSymbolAt_MethodCallUsesReceiver(t *testing.T) {
	bindings := bind.NewBindingTable()
	recv := &ast.IdentExpr{Value: "state"}
	recvSym := cfg.SymbolID(22)
	bindings.Bind(recv, recvSym)
	arg := &ast.IdentExpr{Value: "x"}
	argSym := cfg.SymbolID(23)
	bindings.Bind(arg, argSym)

	info := &cfg.CallInfo{
		Method:   "sorted_keys",
		Receiver: recv,
		Args:     []ast.Expr{arg},
	}

	if got := SymbolOrCreateFieldFromExpr(RuntimeArgAt(info, 0), bindings); got != recvSym {
		t.Fatalf("SymbolOrCreateFieldFromExpr(RuntimeArgAt,0) = %d, want receiver %d", got, recvSym)
	}
	if got := SymbolOrCreateFieldFromExpr(RuntimeArgAt(info, 1), bindings); got != argSym {
		t.Fatalf("SymbolOrCreateFieldFromExpr(RuntimeArgAt,1) = %d, want arg %d", got, argSym)
	}
}

func TestSymbolOrCreateFieldFromExpr_StaticIndexStringAndInt_DoNotCollide(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "state"}
	baseSym := cfg.SymbolID(31)
	bindings.Bind(base, baseSym)

	stringExpr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "1"},
	}
	intExpr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.NumberExpr{Value: "1"},
	}

	stringSym := SymbolOrCreateFieldFromExpr(stringExpr, bindings)
	intSym := SymbolOrCreateFieldFromExpr(intExpr, bindings)

	if stringSym == 0 || intSym == 0 {
		t.Fatalf("expected non-zero symbols, got string=%d int=%d", stringSym, intSym)
	}
	if stringSym == intSym {
		t.Fatalf("expected distinct symbols for [\"1\"] and [1], got %d", stringSym)
	}
}

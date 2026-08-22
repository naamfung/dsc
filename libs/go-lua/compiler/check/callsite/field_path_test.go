package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestFieldPathWithBaseSymbol_Nested(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(77)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.StringExpr{Value: "a"},
		},
		Key: &ast.StringExpr{Value: "b"},
	}

	sym, path, ok := FieldPathWithBaseSymbol(bindings, expr)
	if !ok {
		t.Fatal("expected static nested path resolution")
	}
	if sym != baseSym {
		t.Fatalf("base symbol = %d, want %d", sym, baseSym)
	}
	if path != ".a.b" {
		t.Fatalf("path = %q, want .a.b", path)
	}
}

func TestFieldPathWithBaseSymbol_StaticIntIndex(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(88)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.NumberExpr{Value: "1"},
	}

	sym, path, ok := FieldPathWithBaseSymbol(bindings, expr)
	if !ok {
		t.Fatal("expected static numeric key to be accepted")
	}
	if sym != baseSym {
		t.Fatalf("base symbol = %d, want %d", sym, baseSym)
	}
	if path != "[1]" {
		t.Fatalf("path = %q, want [1]", path)
	}
}

func TestFieldPathWithBaseSymbol_NonIdentifierStringSegment(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(99)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "a.b"},
	}
	sym, path, ok := FieldPathWithBaseSymbol(bindings, expr)
	if !ok {
		t.Fatal("expected non-identifier string segment to be accepted as index-string")
	}
	if sym != baseSym {
		t.Fatalf("base symbol = %d, want %d", sym, baseSym)
	}
	if path != "[\"a.b\"]" {
		t.Fatalf("path = %q, want [\"a.b\"]", path)
	}
}

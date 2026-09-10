package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestStaticPathWithBaseSymbol_Ident(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "x"}
	sym := cfg.SymbolID(7)
	bindings.Bind(ident, sym)

	gotSym, segs, ok := StaticPathWithBaseSymbol(bindings, ident)
	if !ok {
		t.Fatal("expected static path")
	}
	if gotSym != sym {
		t.Fatalf("symbol = %d, want %d", gotSym, sym)
	}
	if len(segs) != 0 {
		t.Fatalf("expected empty segments, got %v", segs)
	}
}

func TestStaticPathWithBaseSymbol_StaticAttrChain(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(11)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.StringExpr{Value: "a"},
		},
		Key: &ast.StringExpr{Value: "b"},
	}

	gotSym, segs, ok := StaticPathWithBaseSymbol(bindings, expr)
	if !ok {
		t.Fatal("expected static path")
	}
	if gotSym != baseSym {
		t.Fatalf("symbol = %d, want %d", gotSym, baseSym)
	}
	if len(segs) != 2 {
		t.Fatalf("segments len = %d, want 2", len(segs))
	}
	if segs[0] != (constraint.Segment{Kind: constraint.SegmentField, Name: "a"}) {
		t.Fatalf("unexpected first segment: %+v", segs[0])
	}
	if segs[1] != (constraint.Segment{Kind: constraint.SegmentField, Name: "b"}) {
		t.Fatalf("unexpected second segment: %+v", segs[1])
	}
}

func TestStaticPathWithBaseSymbol_StaticIndexString(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(13)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "x-y"},
	}

	gotSym, segs, ok := StaticPathWithBaseSymbol(bindings, expr)
	if !ok {
		t.Fatal("expected static path")
	}
	if gotSym != baseSym {
		t.Fatalf("symbol = %d, want %d", gotSym, baseSym)
	}
	if len(segs) != 1 {
		t.Fatalf("segments len = %d, want 1", len(segs))
	}
	if segs[0] != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"}) {
		t.Fatalf("unexpected segment: %+v", segs[0])
	}
}

func TestStaticPathWithBaseSymbol_DynamicIdentKeyRejected(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(17)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.IdentExpr{Value: "k"},
	}

	if _, _, ok := StaticPathWithBaseSymbol(bindings, expr); ok {
		t.Fatal("expected dynamic key to be rejected")
	}
}

func TestStaticPathWithBaseSymbol_StaticIndexInt(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(19)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.NumberExpr{Value: "1"},
	}

	gotSym, segs, ok := StaticPathWithBaseSymbol(bindings, expr)
	if !ok {
		t.Fatal("expected static path")
	}
	if gotSym != baseSym {
		t.Fatalf("symbol = %d, want %d", gotSym, baseSym)
	}
	if len(segs) != 1 {
		t.Fatalf("segments len = %d, want 1", len(segs))
	}
	if segs[0] != (constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}) {
		t.Fatalf("unexpected segment: %+v", segs[0])
	}
}

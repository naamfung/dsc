package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestSymbolFromExpr_Ident(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "x"}
	want := cfg.SymbolID(11)
	bindings.Bind(ident, want)

	if got := SymbolFromExpr(ident, bindings); got != want {
		t.Fatalf("SymbolFromExpr(ident) = %d, want %d", got, want)
	}
}

func TestSymbolFromExpr_FunctionLiteral(t *testing.T) {
	bindings := bind.NewBindingTable()
	fn := &ast.FunctionExpr{}
	want := cfg.SymbolID(23)
	bindings.SetFuncLitSymbol(fn, want)

	if got := SymbolFromExpr(fn, bindings); got != want {
		t.Fatalf("SymbolFromExpr(function) = %d, want %d", got, want)
	}
}

func TestSymbolFromExpr_UnsupportedOrMissing(t *testing.T) {
	if got := SymbolFromExpr(nil, nil); got != 0 {
		t.Fatalf("SymbolFromExpr(nil,nil) = %d, want 0", got)
	}
	if got := SymbolFromExpr(&ast.NumberExpr{Value: "1"}, bind.NewBindingTable()); got != 0 {
		t.Fatalf("SymbolFromExpr(number) = %d, want 0", got)
	}
}

func TestSymbolFromExpr_StaticFieldPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(31)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	fieldSym := bindings.GetOrCreateFieldSymbol(baseSym, "f")
	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "f"},
	}

	if got := SymbolFromExpr(expr, bindings); got != fieldSym {
		t.Fatalf("SymbolFromExpr(M.f) = %d, want %d", got, fieldSym)
	}
}

func TestSymbolFromExpr_StaticFieldPathNested(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(41)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	nestedSym := bindings.GetOrCreateFieldSymbol(baseSym, "a.b")
	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.StringExpr{Value: "a"},
		},
		Key: &ast.StringExpr{Value: "b"},
	}

	if got := SymbolFromExpr(expr, bindings); got != nestedSym {
		t.Fatalf("SymbolFromExpr(M.a.b) = %d, want %d", got, nestedSym)
	}
}

func TestSymbolFromExpr_StaticFieldPathMissingSymbol(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(51)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "missing"},
	}
	if got := SymbolFromExpr(expr, bindings); got != 0 {
		t.Fatalf("SymbolFromExpr(M.missing) = %d, want 0", got)
	}
}

func TestSymbolFromExpr_StaticIndexStringPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(61)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	path, ok := bind.FieldPathKeyFromSegments([]constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "a.b"},
	})
	if !ok {
		t.Fatal("expected canonical index-string path")
	}

	indexSym := bindings.GetOrCreateFieldSymbol(baseSym, path)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "a.b"},
	}

	if got := SymbolFromExpr(expr, bindings); got != indexSym {
		t.Fatalf("SymbolFromExpr(M[\"a.b\"]) = %d, want %d", got, indexSym)
	}
}

func TestSymbolFromExpr_StaticIndexIntPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(71)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	path, ok := bind.FieldPathKeyFromSegments([]constraint.Segment{
		{Kind: constraint.SegmentIndexInt, Index: 1},
	})
	if !ok {
		t.Fatal("expected canonical int-index path")
	}

	indexSym := bindings.GetOrCreateFieldSymbol(baseSym, path)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.NumberExpr{Value: "1"},
	}

	if got := SymbolFromExpr(expr, bindings); got != indexSym {
		t.Fatalf("SymbolFromExpr(M[1]) = %d, want %d", got, indexSym)
	}
}

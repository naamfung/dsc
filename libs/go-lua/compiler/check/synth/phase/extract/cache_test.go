package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCache_PutGet(t *testing.T) {
	cache := make(api.Cache)
	expr := &ast.NumberExpr{Value: "42"}
	point := cfg.Point(1)

	cache.Put(expr, point, typ.Integer)

	got, ok := cache.Get(expr, point)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != typ.Integer {
		t.Fatalf("got %v, want integer", got)
	}
}

func TestCache_GetMiss(t *testing.T) {
	cache := make(api.Cache)
	expr := &ast.NumberExpr{Value: "42"}

	_, ok := cache.Get(expr, cfg.Point(1))
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestCache_DifferentPoints(t *testing.T) {
	cache := make(api.Cache)
	expr := &ast.IdentExpr{Value: "x"}

	cache.Put(expr, cfg.Point(1), typ.Integer)
	cache.Put(expr, cfg.Point(2), typ.String)

	t1, ok := cache.Get(expr, cfg.Point(1))
	if !ok || t1 != typ.Integer {
		t.Fatalf("point 1: got %v, want integer", t1)
	}

	t2, ok := cache.Get(expr, cfg.Point(2))
	if !ok || t2 != typ.String {
		t.Fatalf("point 2: got %v, want string", t2)
	}
}

func TestCache_DifferentExprs(t *testing.T) {
	cache := make(api.Cache)
	expr1 := &ast.IdentExpr{Value: "x"}
	expr2 := &ast.IdentExpr{Value: "y"}
	point := cfg.Point(1)

	cache.Put(expr1, point, typ.Integer)
	cache.Put(expr2, point, typ.String)

	t1, _ := cache.Get(expr1, point)
	t2, _ := cache.Get(expr2, point)

	if t1 != typ.Integer {
		t.Fatalf("expr1: got %v, want integer", t1)
	}
	if t2 != typ.String {
		t.Fatalf("expr2: got %v, want string", t2)
	}
}

func TestCache_NilSafe(t *testing.T) {
	var cache api.Cache

	_, ok := cache.Get(&ast.NilExpr{}, cfg.Point(1))
	if ok {
		t.Fatal("nil cache should return false")
	}

	cache.Put(&ast.NilExpr{}, cfg.Point(1), typ.Nil)
}

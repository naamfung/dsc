package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSynthExprWithUnionExpected_NotUnion(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthExprWithUnionExpected(
		&ast.StringExpr{Value: "hello"},
		sc, 0, recurse,
		typ.String,
	)

	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != "hello" {
		t.Fatalf("got %v, want hello", lit.Value)
	}
}

func TestSynthExprWithUnionExpected_DiscriminatedUnion(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	successType := typ.NewRecord().
		Field("kind", typ.LiteralString("success")).
		Field("value", typ.String).
		Build()
	errorType := typ.NewRecord().
		Field("kind", typ.LiteralString("error")).
		Field("message", typ.String).
		Build()
	unionType := typ.NewUnion(successType, errorType)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "kind"}, Value: &ast.StringExpr{Value: "success"}},
			{Key: &ast.StringExpr{Value: "value"}, Value: &ast.StringExpr{Value: "done"}},
		},
	}

	result := s.synthExprWithUnionExpected(table, sc, 0, recurse, unionType)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	kindField := rec.GetField("kind")
	if kindField == nil {
		t.Fatal("missing kind field")
	}
}

func TestSynthExprWithUnionExpected_Function(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	fn1 := typ.Func().Param("x", typ.Integer).Returns(typ.String).Build()
	fn2 := typ.Func().Param("x", typ.Integer).Param("y", typ.Integer).Returns(typ.Integer).Build()
	unionType := typ.NewUnion(fn1, fn2)

	fnExpr := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}

	result := s.synthExprWithUnionExpected(fnExpr, sc, 0, recurse, unionType)

	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("got %T, want function", result)
	}
	if len(fn.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(fn.Params))
	}
}

func TestSynthExprWithUnionExpected_NoMatch(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	unionType := typ.NewUnion(typ.String, typ.Integer)
	result := s.synthExprWithUnionExpected(&ast.NumberExpr{Value: "42"}, sc, 0, recurse, unionType)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSynthExprWithExpectedSingle_Table(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	expected := typ.NewRecord().
		Field("name", typ.String).
		Build()

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "test"}},
		},
	}

	result := s.synthExprWithExpectedSingle(table, sc, 0, recurse, expected)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	if rec.GetField("name") == nil {
		t.Fatal("missing name field")
	}
}

func TestSynthExprWithExpectedSingle_Function(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	expected := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()

	fnExpr := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}

	result := s.synthExprWithExpectedSingle(fnExpr, sc, 0, recurse, expected)

	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("got %T, want function", result)
	}
	if len(fn.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(fn.Params))
	}
}

func TestSynthExprWithExpectedSingle_FuncCall(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	fnType := typ.Func().Returns(typ.String).Build()
	fnIdent := &ast.IdentExpr{Value: "getStr"}
	s.deps.PreCache.Put(fnIdent, 0, fnType)

	call := &ast.FuncCallExpr{
		Func: fnIdent,
	}

	result := s.synthExprWithExpectedSingle(call, sc, 0, recurse, typ.String)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSynthExprWithExpectedSingle_Other(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthExprWithExpectedSingle(&ast.NumberExpr{Value: "42"}, sc, 0, recurse, typ.Integer)

	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != int64(42) {
		t.Fatalf("got %v, want 42", lit.Value)
	}
}

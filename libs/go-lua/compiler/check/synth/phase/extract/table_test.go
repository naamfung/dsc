package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSynthTableCore_Empty(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	table := &ast.TableExpr{Fields: []*ast.Field{}}
	result := s.SynthTableCore(table, sc, recurse)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	if len(rec.Fields) != 0 {
		t.Fatalf("got %d fields, want 0", len(rec.Fields))
	}
}

func TestSynthTableCore_WithFields(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "test"}},
			{Key: &ast.StringExpr{Value: "count"}, Value: &ast.NumberExpr{Value: "42"}},
		},
	}
	result := s.SynthTableCore(table, sc, recurse)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(rec.Fields))
	}
}

func TestSynthTableCore_ArrayLike(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Value: &ast.NumberExpr{Value: "1"}},
			{Value: &ast.NumberExpr{Value: "2"}},
			{Value: &ast.NumberExpr{Value: "3"}},
		},
	}
	result := s.SynthTableCore(table, sc, recurse)

	tuple, ok := result.(*typ.Tuple)
	if !ok {
		t.Fatalf("got %T, want tuple", result)
	}
	if len(tuple.Elements) != 3 {
		t.Fatalf("got %d elements, want 3", len(tuple.Elements))
	}
}

func TestSynthTableWithExpected_Record(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	expected := typ.NewRecord().
		Field("name", typ.String).
		Field("count", typ.Integer).
		Build()

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "test"}},
			{Key: &ast.StringExpr{Value: "count"}, Value: &ast.NumberExpr{Value: "42"}},
		},
	}
	result := s.SynthTableWithExpected(table, sc, recurse, expected)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(rec.Fields))
	}
}

func TestSynthTableWithExpected_DiscriminatedUnion(t *testing.T) {
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
	result := s.SynthTableWithExpected(table, sc, recurse, unionType)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	kindField := rec.GetField("kind")
	if kindField == nil {
		t.Fatal("missing kind field")
	}
}

func TestSynthTableWithExpected_Function(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	expected := typ.NewRecord().
		Field("callback", typ.Func().Param("x", typ.Integer).Returns(typ.String).Build()).
		Build()

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key: &ast.StringExpr{Value: "callback"},
				Value: &ast.FunctionExpr{
					ParList: &ast.ParList{Names: []string{"x"}},
				},
			},
		},
	}
	result := s.SynthTableWithExpected(table, sc, recurse, expected)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	if rec.GetField("callback") == nil {
		t.Fatal("missing callback field")
	}
}

func TestSynthTableCore_IdentKey(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "name"}, Value: &ast.StringExpr{Value: "test"}},
		},
	}
	result := s.SynthTableCore(table, sc, recurse)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	if rec.GetField("name") == nil {
		t.Fatal("missing name field")
	}
}

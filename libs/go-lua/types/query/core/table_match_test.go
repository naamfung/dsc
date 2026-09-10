package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTryDiscriminatedUnionMember_NonUnion(t *testing.T) {
	table := &ast.TableExpr{}
	result := TryDiscriminatedUnionMember(table, typ.String)
	if result != nil {
		t.Error("non-union should return nil")
	}
}

func TestTryDiscriminatedUnionMember_EmptyTable(t *testing.T) {
	table := &ast.TableExpr{}
	union := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("a")).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("b")).Build(),
	)
	result := TryDiscriminatedUnionMember(table, union)
	if result != nil {
		t.Error("empty table should return nil")
	}
}

func TestTryDiscriminatedUnionMember_WithDiscriminant(t *testing.T) {
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "kind"},
				Value: &ast.StringExpr{Value: "a"},
			},
		},
	}
	recA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.Number).Build()
	recB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("y", typ.String).Build()
	union := typ.NewUnion(recA, recB)

	result := TryDiscriminatedUnionMember(table, union)
	if result == nil {
		t.Fatal("should find matching member")
	}
	if result.Member == nil {
		t.Error("Member should not be nil")
	}
}

func TestTryDiscriminatedUnionMember_NoMatch(t *testing.T) {
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "kind"},
				Value: &ast.StringExpr{Value: "c"},
			},
		},
	}
	recA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	recB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()
	union := typ.NewUnion(recA, recB)

	result := TryDiscriminatedUnionMember(table, union)
	if result != nil {
		t.Error("non-matching discriminant should return nil")
	}
}

func TestTryDiscriminatedUnionMember_MultipleMatches(t *testing.T) {
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "kind"},
				Value: &ast.StringExpr{Value: "a"},
			},
		},
	}
	recA1 := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.Number).Build()
	recA2 := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("y", typ.String).Build()
	union := typ.NewUnion(recA1, recA2)

	result := TryDiscriminatedUnionMember(table, union)
	if result != nil {
		t.Error("multiple matches should return nil")
	}
}

func TestTableMatchResult(t *testing.T) {
	r := &TableMatchResult{
		Member:      typ.String,
		MemberIndex: 1,
	}
	if r.Member != typ.String {
		t.Error("Member should be set")
	}
	if r.MemberIndex != 1 {
		t.Error("MemberIndex should be set")
	}
}

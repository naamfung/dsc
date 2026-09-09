package transform

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplySpecReturnCases_NilFn(t *testing.T) {
	result := ApplySpecReturnCases(nil, nil)
	if result != nil {
		t.Fatal("expected nil for nil function")
	}
}

func TestApplySpecReturnCases_NilSpec(t *testing.T) {
	fn := typ.Func().Build()
	result := ApplySpecReturnCases(fn, nil)
	if result != nil {
		t.Fatal("expected nil for nil spec")
	}
}

func TestApplySpecReturnCases_TrueCondition(t *testing.T) {
	spec := &contract.Spec{
		Return: &contract.ReturnSpec{
			Cases: []contract.ReturnCase{
				{When: constraint.TrueCondition(), Type: typ.String},
			},
		},
	}
	fn := typ.Func().Build()
	fn.Spec = spec

	result := ApplySpecReturnCases(fn, nil)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestApplySpecReturnCases_Default(t *testing.T) {
	spec := &contract.Spec{
		Return: &contract.ReturnSpec{
			Cases:   []contract.ReturnCase{},
			Default: typ.Integer,
		},
	}
	fn := typ.Func().Build()
	fn.Spec = spec

	result := ApplySpecReturnCases(fn, nil)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSpecReturnCaseMatchesTypes_True(t *testing.T) {
	if !specReturnCaseMatchesTypes(constraint.TrueCondition(), nil) {
		t.Fatal("expected true")
	}
}

func TestSpecReturnCaseMatchesTypes_False(t *testing.T) {
	if specReturnCaseMatchesTypes(constraint.FalseCondition(), nil) {
		t.Fatal("expected false")
	}
}

func TestTypeFieldMatchesLiteral_Record(t *testing.T) {
	rec := typ.NewRecord().
		Field("kind", typ.LiteralString("success")).
		Build()

	if !typeFieldMatchesLiteral(rec, "kind", typ.LiteralString("success")) {
		t.Fatal("expected match")
	}
}

func TestTypeFieldMatchesLiteral_RecordMismatch(t *testing.T) {
	rec := typ.NewRecord().
		Field("kind", typ.LiteralString("error")).
		Build()

	if typeFieldMatchesLiteral(rec, "kind", typ.LiteralString("success")) {
		t.Fatal("expected mismatch")
	}
}

func TestTypeFieldMatchesLiteral_Nil(t *testing.T) {
	if typeFieldMatchesLiteral(nil, "field", typ.LiteralString("val")) {
		t.Fatal("expected false for nil type")
	}
}

func TestTypeFieldMatchesLiteral_EmptyField(t *testing.T) {
	rec := typ.NewRecord().Build()
	if typeFieldMatchesLiteral(rec, "", typ.LiteralString("val")) {
		t.Fatal("expected false for empty field")
	}
}

func TestTypeFieldMatchesLiteral_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	if typeFieldMatchesLiteral(opt, "field", typ.LiteralString("val")) {
		t.Fatal("expected false for optional")
	}
}

func TestTypeFieldMatchesLiteral_Union(t *testing.T) {
	rec1 := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	rec2 := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	union := typ.NewUnion(rec1, rec2)

	if !typeFieldMatchesLiteral(union, "kind", typ.LiteralString("a")) {
		t.Fatal("expected match for union where all members match")
	}
}

func TestTypeFieldMatchesLiteral_UnionMismatch(t *testing.T) {
	rec1 := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	rec2 := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()
	union := typ.NewUnion(rec1, rec2)

	if typeFieldMatchesLiteral(union, "kind", typ.LiteralString("a")) {
		t.Fatal("expected mismatch for union where not all members match")
	}
}

package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSynthNumber_Integer(t *testing.T) {
	result := ops.ParseNumber("42")
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != int64(42) {
		t.Fatalf("got %v, want 42", lit.Value)
	}
}

func TestSynthNumber_Float(t *testing.T) {
	result := ops.ParseNumber("3.14")
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != 3.14 {
		t.Fatalf("got %v, want 3.14", lit.Value)
	}
}

func TestSynthNumber_Hex(t *testing.T) {
	result := ops.ParseNumber("0x10")
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != int64(16) {
		t.Fatalf("got %v, want 16", lit.Value)
	}
}

func TestSynthLogicalOpCore_And(t *testing.T) {
	s := newTestSynthesizer()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthLogicalOpCore(&ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      &ast.TrueExpr{},
		Rhs:      &ast.StringExpr{Value: "test"},
	}, recurse)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSynthLogicalOpCore_Or(t *testing.T) {
	s := newTestSynthesizer()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthLogicalOpCore(&ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      &ast.FalseExpr{},
		Rhs:      &ast.StringExpr{Value: "default"},
	}, recurse)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSynthLogicalOpCore_Unknown(t *testing.T) {
	s := newTestSynthesizer()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthLogicalOpCore(&ast.LogicalOpExpr{
		Operator: "xor",
		Lhs:      &ast.TrueExpr{},
		Rhs:      &ast.FalseExpr{},
	}, recurse)

	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

func TestSynthArithmeticOpCore(t *testing.T) {
	s := newTestSynthesizer()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthArithmeticOpCore(&ast.ArithmeticOpExpr{
		Operator: "+",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}, recurse)

	if result != typ.Number {
		t.Fatalf("got %v, want number", result)
	}
}

func TestSynthUnaryMinusCore(t *testing.T) {
	s := newTestSynthesizer()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthUnaryMinusCore(&ast.UnaryMinusOpExpr{
		Expr: &ast.NumberExpr{Value: "42"},
	}, recurse)

	if result != typ.Number {
		t.Fatalf("got %v, want number", result)
	}
}

func TestExpandValues_Normal(t *testing.T) {
	s := newTestSynthesizer()
	exprs := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.StringExpr{Value: "hello"},
	}

	result := s.expandValues(exprs, 4, 0, nil)
	if len(result) != 4 {
		t.Fatalf("got %d types, want 4", len(result))
	}
	if result[2] != typ.Nil {
		t.Fatal("expected nil padding")
	}
	if result[3] != typ.Nil {
		t.Fatal("expected nil padding")
	}
}

func TestExpandValues_Empty(t *testing.T) {
	s := newTestSynthesizer()
	result := s.expandValues(nil, 3, 0, nil)
	if result != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestMapValueType_Map(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Integer)
	result := mapValueType(m)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestMapValueType_Optional(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Integer)
	opt := typ.NewOptional(m)
	result := mapValueType(opt)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestMapValueType_Nil(t *testing.T) {
	result := mapValueType(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestMapValueType_NonMap(t *testing.T) {
	result := mapValueType(typ.String)
	if result != nil {
		t.Fatal("expected nil for non-map")
	}
}

func TestFieldOnPartialUnion_NotUnion(t *testing.T) {
	result := fieldOnPartialUnion(typ.String, "foo", mockTypeQuerier{}, nil)
	if result != nil {
		t.Fatal("expected nil for non-union")
	}
}

func TestFieldOnPartialUnion_WithField(t *testing.T) {
	rec1 := typ.NewRecord().Field("name", typ.String).Build()
	rec2 := typ.NewRecord().Build()
	union := typ.NewUnion(rec1, rec2)

	result := fieldOnPartialUnion(union, "name", mockTypeQuerier{}, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestFieldOnPartialUnion_NoField(t *testing.T) {
	rec1 := typ.NewRecord().Build()
	rec2 := typ.NewRecord().Build()
	union := typ.NewUnion(rec1, rec2)

	result := fieldOnPartialUnion(union, "name", mockTypeQuerier{}, nil)
	if result != nil {
		t.Fatal("expected nil when no member has field")
	}
}

func TestNarrowTupleIndex_NotTuple(t *testing.T) {
	s := newTestSynthesizer()
	result := s.narrowTupleIndex(typ.String, "i", typ.Integer, 0, nil)
	if result != nil {
		t.Fatal("expected nil for non-tuple")
	}
}

func TestNarrowTupleIndex_NilNarrower(t *testing.T) {
	s := newTestSynthesizer()
	tuple := typ.NewTuple(typ.String, typ.Integer)
	result := s.narrowTupleIndex(tuple, "i", typ.Integer, 0, nil)
	if result != nil {
		t.Fatal("expected nil without narrower")
	}
}

func TestSynthAttrGetCore_StringKey(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	rec := typ.NewRecord().Field("name", typ.String).Build()
	objExpr := &ast.TableExpr{}
	s.deps.PreCache.Put(objExpr, 0, rec)

	result := s.synthAttrGetCore(&ast.AttrGetExpr{
		Object: objExpr,
		Key:    &ast.StringExpr{Value: "name"},
	}, 0, sc, nil, recurse)

	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestSynthAttrGetCore_UnknownKey(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	recurse := func(ex ast.Expr) typ.Type { return s.TypeOf(ex, 0) }

	result := s.synthAttrGetCore(&ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "missing"},
	}, 0, sc, nil, recurse)

	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSynthTableCore_Empty(t *testing.T) {
	e := newTestEngine()
	expr := &ast.TableExpr{Fields: []*ast.Field{}}

	result := e.TypeOf(expr, 0)
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}
	if len(rec.Fields) != 0 {
		t.Fatalf("got %d fields, want 0", len(rec.Fields))
	}
}

func TestSynthTableCore_StringKeys(t *testing.T) {
	e := newTestEngine()
	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "test"}},
			{Key: &ast.StringExpr{Value: "count"}, Value: &ast.NumberExpr{Value: "42"}},
		},
	}

	result := e.TypeOf(expr, 0)
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}

	nameField := rec.GetField("name")
	if nameField == nil {
		t.Fatal("missing name field")
	}

	countField := rec.GetField("count")
	if countField == nil {
		t.Fatal("missing count field")
	}
}

func TestSynthTableCore_IdentKeys(t *testing.T) {
	e := newTestEngine()
	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "x"}, Value: &ast.NumberExpr{Value: "1"}},
			{Key: &ast.IdentExpr{Value: "y"}, Value: &ast.NumberExpr{Value: "2"}},
		},
	}

	result := e.TypeOf(expr, 0)
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}

	if rec.GetField("x") == nil {
		t.Fatal("missing x field")
	}
	if rec.GetField("y") == nil {
		t.Fatal("missing y field")
	}
}

func TestSynthTableCore_ArrayLiteral(t *testing.T) {
	e := newTestEngine()
	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: nil, Value: &ast.NumberExpr{Value: "1"}},
			{Key: nil, Value: &ast.NumberExpr{Value: "2"}},
			{Key: nil, Value: &ast.NumberExpr{Value: "3"}},
		},
	}

	result := e.TypeOf(expr, 0)
	tup, ok := result.(*typ.Tuple)
	if !ok {
		t.Fatalf("got %T, want tuple", result)
	}

	if len(tup.Elements) != 3 {
		t.Fatalf("got %d elements, want 3", len(tup.Elements))
	}

	for i, elem := range tup.Elements {
		lit, ok := elem.(*typ.Literal)
		if !ok || lit.Base != kind.Integer {
			t.Fatalf("element %d: got %v, want integer literal", i, elem)
		}
	}
}

func TestSynthTableCore_MixedArrayElements(t *testing.T) {
	e := newTestEngine()
	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: nil, Value: &ast.NumberExpr{Value: "1"}},
			{Key: nil, Value: &ast.StringExpr{Value: "two"}},
		},
	}

	result := e.TypeOf(expr, 0)
	tup, ok := result.(*typ.Tuple)
	if !ok {
		t.Fatalf("got %T, want tuple", result)
	}

	if len(tup.Elements) != 2 {
		t.Fatalf("got %d elements, want 2", len(tup.Elements))
	}

	if tup.Elements[0].Kind() != kind.Literal {
		t.Fatalf("element 0: got %v, want integer literal", tup.Elements[0])
	}
	if tup.Elements[1].Kind() != kind.Literal {
		t.Fatalf("element 1: got %v, want string literal", tup.Elements[1])
	}
}

func TestSynthTableCore_RecordNotArray(t *testing.T) {
	e := newTestEngine()
	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "a"}, Value: &ast.NumberExpr{Value: "1"}},
			{Key: nil, Value: &ast.NumberExpr{Value: "2"}},
		},
	}

	result := e.TypeOf(expr, 0)
	_, isRecord := result.(*typ.Record)
	if !isRecord {
		t.Fatalf("got %T, want record (mixed keys should produce record)", result)
	}
}

func TestSynthFieldValueCore_Function(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
		},
		ReturnTypes: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
	}

	result := e.TypeOf(fn, 0)
	if result.Kind() != kind.Function {
		t.Fatalf("got %v, want function", result)
	}
}

func TestSynthFieldValueCore_TruePreservesLiteral(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.TrueExpr{}, 0)

	if result != typ.True {
		t.Fatalf("got %v, want true", result)
	}
}

func TestSynthFieldValueCore_FalsePreservesLiteral(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.FalseExpr{}, 0)

	if result != typ.False {
		t.Fatalf("got %v, want false", result)
	}
}

func TestSynthFieldValueCore_NumberStaysLiteral(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.NumberExpr{Value: "42"}, 0)

	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("got %v, want integer literal", result)
	}
}

func TestUnionTypes_Empty(t *testing.T) {
	result := typ.NewUnion()
	if result != typ.Never {
		t.Fatalf("got %v, want never", result)
	}
}

func TestUnionTypes_Single(t *testing.T) {
	result := typ.NewUnion(typ.String)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestUnionTypes_Multiple(t *testing.T) {
	result := typ.NewUnion(typ.String, typ.Integer)

	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("got %T, want union", result)
	}
	if len(union.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(union.Members))
	}
}

func TestSynthTableCore_NestedTable(t *testing.T) {
	e := newTestEngine()
	inner := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "value"}, Value: &ast.NumberExpr{Value: "100"}},
		},
	}
	outer := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "inner"}, Value: inner},
		},
	}

	result := e.TypeOf(outer, 0)
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}

	innerField := rec.GetField("inner")
	if innerField == nil {
		t.Fatal("missing inner field")
	}

	innerRec, ok := innerField.Type.(*typ.Record)
	if !ok {
		t.Fatalf("inner: got %T, want record", innerField.Type)
	}
	if innerRec.GetField("value") == nil {
		t.Fatal("inner missing value field")
	}
}

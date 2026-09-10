package subtype

// Tests for type widening, simplification, and constructibility.

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestWiden_Literals(t *testing.T) {
	tests := []struct {
		input typ.Type
		want  typ.Type
	}{
		{typ.LiteralBool(true), typ.Boolean},
		{typ.LiteralBool(false), typ.Boolean},
		{typ.LiteralInt(42), typ.Integer},
		{typ.LiteralNumber(3.14), typ.Number},
		{typ.LiteralString("hello"), typ.String},
	}
	for _, tt := range tests {
		got := Widen(tt.input)
		if got != tt.want {
			t.Errorf("Widen(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestWiden_NonLiterals(t *testing.T) {
	types := []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.String,
		typ.Any,
		typ.Never,
	}
	for _, ty := range types {
		if Widen(ty) != ty {
			t.Errorf("Widen(%v) should return same type", ty)
		}
	}
}

func TestWiden_Nil(t *testing.T) {
	if Widen(nil) != nil {
		t.Error("Widen(nil) should return nil")
	}
}

func TestWiden_Union(t *testing.T) {
	// Union of different literal types - stays union
	lit1 := typ.LiteralInt(1)
	lit2 := typ.LiteralString("x")
	union := typ.NewUnion(lit1, lit2)

	result := Widen(union)
	resultUnion, ok := result.(*typ.Union)

	if !ok {
		t.Fatalf("expected Union, got %T", result)
	}

	hasInt := false
	hasStr := false

	for _, m := range resultUnion.Members {
		if m == typ.Integer {
			hasInt = true
		}

		if m == typ.String {
			hasStr = true
		}
	}

	if !hasInt || !hasStr {
		t.Error("union members should be widened")
	}
}

func TestWiden_UnionSameLiterals(t *testing.T) {
	// Union of same-type literals normalizes to single type
	lit1 := typ.LiteralInt(1)
	lit2 := typ.LiteralInt(2)
	union := typ.NewUnion(lit1, lit2)

	result := Widen(union)
	// After widening both become Integer, union normalizes
	if result != typ.Integer {
		t.Errorf("union of integer literals should widen to Integer, got %v", result)
	}
}

func TestWiden_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.LiteralInt(42))
	result := Widen(opt)

	resultOpt, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}

	if resultOpt.Inner != typ.Integer {
		t.Errorf("optional inner should be widened to Integer, got %v", resultOpt.Inner)
	}
}

func TestWidenForInference_Basic(t *testing.T) {
	got := WidenForInference(typ.LiteralInt(42))
	if got != typ.Integer {
		t.Errorf("expected Integer, got %v", got)
	}
}

func TestWidenForInference_Tuple(t *testing.T) {
	tuple := typ.NewTuple(typ.LiteralInt(1), typ.LiteralString("x"))
	result := WidenForInference(tuple)

	resultTuple, ok := result.(*typ.Tuple)
	if !ok {
		t.Fatalf("expected Tuple, got %T", result)
	}

	if resultTuple.Elements[0] != typ.Integer {
		t.Error("first element should be widened to Integer")
	}

	if resultTuple.Elements[1] != typ.String {
		t.Error("second element should be widened to String")
	}
}

func TestWidenForInference_Array(t *testing.T) {
	arr := typ.NewArray(typ.LiteralInt(42))
	result := WidenForInference(arr)

	resultArr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}

	if resultArr.Element != typ.Integer {
		t.Error("element should be widened to Integer")
	}
}

func TestWidenForInference_Map(t *testing.T) {
	m := typ.NewMap(typ.LiteralString("key"), typ.LiteralInt(42))
	result := WidenForInference(m)

	resultMap, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("expected Map, got %T", result)
	}

	if resultMap.Key != typ.String {
		t.Error("key should be widened to String")
	}

	if resultMap.Value != typ.Integer {
		t.Error("value should be widened to Integer")
	}
}

func TestWidenForInference_Nil(t *testing.T) {
	if WidenForInference(nil) != nil {
		t.Error("WidenForInference(nil) should return nil")
	}
}

func TestWidenForInference_Record(t *testing.T) {
	rec := typ.NewRecord().
		Field("x", typ.LiteralInt(42)).
		OptField("y", typ.LiteralString("hello")).
		Build()

	result := WidenForInference(rec)
	resultRec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}

	xField := resultRec.GetField("x")
	if xField == nil || xField.Type != typ.Integer {
		t.Error("x field should be widened to Integer")
	}

	yField := resultRec.GetField("y")
	if yField == nil || yField.Type != typ.String {
		t.Error("y field should be widened to String")
	}
}

func TestWidenForInference_RecordWithReadonly(t *testing.T) {
	rec := typ.NewRecord().
		ReadonlyField("x", typ.LiteralInt(42)).
		Build()

	result := WidenForInference(rec)
	resultRec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}

	xField := resultRec.GetField("x")
	if xField == nil || !xField.Readonly {
		t.Error("x field should remain readonly")
	}
}

func TestWidenForInference_RecordWithOptReadonly(t *testing.T) {
	rec := typ.NewRecord().
		OptReadonlyField("x", typ.LiteralInt(42)).
		Build()

	result := WidenForInference(rec)
	resultRec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}

	xField := resultRec.GetField("x")
	if xField == nil || !xField.Optional || !xField.Readonly {
		t.Error("x field should remain optional and readonly")
	}
}

func TestWidenForInference_RecordWithMetatable(t *testing.T) {
	meta := typ.NewRecord().Field("__index", typ.Func().Returns(typ.Any).Build()).Build()
	rec := typ.NewRecord().
		Field("x", typ.LiteralInt(42)).
		Metatable(meta).
		Build()

	result := WidenForInference(rec)
	resultRec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}

	if resultRec.Metatable == nil {
		t.Error("metatable should be preserved")
	}
}

func TestWidenForInference_RecordWithMapComponent(t *testing.T) {
	rec := typ.NewRecord().
		Field("name", typ.LiteralString("test")).
		MapComponent(typ.LiteralString("key"), typ.LiteralInt(1)).
		Build()

	result := WidenForInference(rec)
	resultRec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}

	if !resultRec.HasMapComponent() {
		t.Error("map component should be preserved")
	}
	if resultRec.MapKey != typ.String {
		t.Error("map key should be widened to String")
	}
	if resultRec.MapValue != typ.Integer {
		t.Error("map value should be widened to Integer")
	}
}

func TestWidenForInference_OpenRecord(t *testing.T) {
	rec := typ.NewRecord().
		SetOpen(true).
		Field("x", typ.LiteralInt(42)).
		Build()

	result := WidenForInference(rec)
	resultRec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}

	if !resultRec.Open {
		t.Error("open flag should be preserved")
	}
}

func TestWidenForInference_TupleUnchanged(t *testing.T) {
	tuple := typ.NewTuple(typ.Number, typ.String)
	result := WidenForInference(tuple)

	if result != tuple {
		t.Error("tuple with no literals should be unchanged")
	}
}

func TestWidenForInference_ArrayUnchanged(t *testing.T) {
	arr := typ.NewArray(typ.Number)
	result := WidenForInference(arr)

	if result != arr {
		t.Error("array with no literals should be unchanged")
	}
}

func TestWidenForInference_MapUnchanged(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	result := WidenForInference(m)

	if result != m {
		t.Error("map with no literals should be unchanged")
	}
}

func TestWidenForInference_OtherTypes(t *testing.T) {
	fn := typ.Func().Param("x", typ.Number).Returns(typ.String).Build()
	result := WidenForInference(fn)

	if result != fn {
		t.Error("function should be unchanged by WidenForInference")
	}
}

func TestWidenForInference_FunctionNestedLiterals(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.LiteralInt(7)).
		Returns(
			typ.NewRecord().
				Field("label", typ.LiteralString("ok")).
				Build(),
		).
		Build()

	result := WidenForInference(fn)
	resultFn, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}

	if resultFn.Params[0].Type != typ.Integer {
		t.Fatalf("expected widened param type Integer, got %v", resultFn.Params[0].Type)
	}

	retRec, ok := resultFn.Returns[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record return, got %T", resultFn.Returns[0])
	}
	field := retRec.GetField("label")
	if field == nil || field.Type != typ.String {
		t.Fatalf("expected widened record field type String, got %v", field)
	}
}

func TestWidenForInference_InterfaceMethodNestedLiterals(t *testing.T) {
	method := typ.Func().
		Param("self", typ.Any).
		Returns(typ.LiteralString("ok")).
		Build()
	iface := typ.NewInterface("", []typ.Method{
		{Name: "status", Type: method},
	})

	result := WidenForInference(iface)
	resultIface, ok := result.(*typ.Interface)
	if !ok {
		t.Fatalf("expected Interface, got %T", result)
	}
	if len(resultIface.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(resultIface.Methods))
	}
	if resultIface.Methods[0].Type.Returns[0] != typ.String {
		t.Fatalf("expected widened method return String, got %v", resultIface.Methods[0].Type.Returns[0])
	}
}

func TestWiden_UnchangedUnion(t *testing.T) {
	union := typ.NewUnion(typ.Number, typ.String)
	result := Widen(union)

	if result.Kind() != typ.NewUnion(typ.Number, typ.String).Kind() {
		t.Error("union without literals should be unchanged in structure")
	}
}

func TestWiden_UnchangedOptional(t *testing.T) {
	opt := typ.NewOptional(typ.Number)
	result := Widen(opt)

	if result != opt {
		t.Error("optional without literal should be unchanged")
	}
}

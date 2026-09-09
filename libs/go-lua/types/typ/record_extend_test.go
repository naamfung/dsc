package typ

import (
	"testing"
)

func TestExtendRecordWithField_NilBase(t *testing.T) {
	result := ExtendRecordWithField(nil, "foo", String)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("foo"); f == nil || f.Type != String {
		t.Error("expected field foo with string type")
	}
}

func TestExtendRecordWithField_AnyBase(t *testing.T) {
	result := ExtendRecordWithField(Any, "foo", Integer)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("foo"); f == nil || f.Type != Integer {
		t.Error("expected field foo with integer type")
	}
}

func TestExtendRecordWithField_UnknownBase(t *testing.T) {
	result := ExtendRecordWithField(Unknown, "bar", Boolean)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("bar"); f == nil || f.Type != Boolean {
		t.Error("expected field bar with boolean type")
	}
}

func TestExtendRecordWithField_NilBaseType(t *testing.T) {
	result := ExtendRecordWithField(Nil, "x", Number)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("x"); f == nil || f.Type != Number {
		t.Error("expected field x with number type")
	}
}

func TestExtendRecordWithField_AddToRecord(t *testing.T) {
	base := NewRecord().Field("a", String).Build()
	result := ExtendRecordWithField(base, "b", Integer)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("a"); f == nil || f.Type != String {
		t.Error("expected field a preserved")
	}
	if f := rec.GetField("b"); f == nil || f.Type != Integer {
		t.Error("expected field b added")
	}
}

func TestExtendRecordWithField_UpdateExisting(t *testing.T) {
	base := NewRecord().Field("x", String).Build()
	result := ExtendRecordWithField(base, "x", Integer)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("x"); f == nil || f.Type != Integer {
		t.Errorf("expected field x updated to integer, got %v", f)
	}
}

func TestExtendRecordWithField_PreservesOptional(t *testing.T) {
	base := NewRecord().OptField("opt", String).Build()
	result := ExtendRecordWithField(base, "new", Integer)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("opt"); f == nil || !f.Optional {
		t.Error("expected optional field preserved")
	}
}

func TestExtendRecordWithField_PreservesReadonly(t *testing.T) {
	base := NewRecord().ReadonlyField("ro", String).Build()
	result := ExtendRecordWithField(base, "new", Integer)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if f := rec.GetField("ro"); f == nil || !f.Readonly {
		t.Error("expected readonly field preserved")
	}
}

func TestExtendRecordWithField_PreservesMetatable(t *testing.T) {
	meta := NewRecord().Field("__index", Func().Build()).Build()
	base := NewRecord().Field("x", String).Metatable(meta).Build()
	result := ExtendRecordWithField(base, "y", Integer)
	rec, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if rec.Metatable == nil {
		t.Error("expected metatable preserved")
	}
}

func TestExtendRecordWithField_EmptyFieldName(t *testing.T) {
	base := NewRecord().Field("x", String).Build()
	result := ExtendRecordWithField(base, "", Integer)
	if result != base {
		t.Error("expected base returned for empty field name")
	}
}

func TestExtendRecordWithField_NilFieldType(t *testing.T) {
	base := NewRecord().Field("x", String).Build()
	result := ExtendRecordWithField(base, "y", nil)
	if result != base {
		t.Error("expected base returned for nil field type")
	}
}

func TestExtendRecordWithField_NonRecordBase(t *testing.T) {
	result := ExtendRecordWithField(String, "x", Integer)
	if result != String {
		t.Error("expected original type returned for non-record base")
	}
}

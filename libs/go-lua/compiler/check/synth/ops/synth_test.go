package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestFieldDef(t *testing.T) {
	fd := FieldDef{
		Name:     "test",
		Type:     typ.String,
		Optional: true,
	}
	if fd.Name != "test" {
		t.Errorf("expected Name 'test', got %q", fd.Name)
	}
	if fd.Type != typ.String {
		t.Errorf("expected Type String, got %v", fd.Type)
	}
	if !fd.Optional {
		t.Error("expected Optional true")
	}
}

func TestTableConstructor_Empty(t *testing.T) {
	result := tableConstructor(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil type for empty table")
	}
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected *typ.Record, got %T", result)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(rec.Fields))
	}
}

func TestTableConstructor_PureArray(t *testing.T) {
	elements := []typ.Type{typ.Number, typ.String}
	result := tableConstructor(nil, elements)
	if result == nil {
		t.Fatal("expected non-nil type for array")
	}
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected *typ.Array, got %T", result)
	}
	if arr.Element == nil {
		t.Error("expected non-nil element type")
	}
}

func TestTableConstructor_Record(t *testing.T) {
	fields := []FieldDef{
		{Name: "x", Type: typ.Number, Optional: false},
		{Name: "y", Type: typ.String, Optional: true},
	}
	result := tableConstructor(fields, nil)
	if result == nil {
		t.Fatal("expected non-nil type for record")
	}
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected *typ.Record, got %T", result)
	}
	if len(rec.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(rec.Fields))
	}
}

func TestSynthesizeArray_Empty(t *testing.T) {
	result := synthesizeArray(nil)
	if result == nil {
		t.Fatal("expected non-nil type")
	}
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected *typ.Array, got %T", result)
	}
	if arr.Element != typ.Never {
		t.Errorf("expected Never element type, got %v", arr.Element)
	}
}

func TestSynthesizeArray_Single(t *testing.T) {
	result := synthesizeArray([]typ.Type{typ.Number})
	if result == nil {
		t.Fatal("expected non-nil type")
	}
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected *typ.Array, got %T", result)
	}
	if arr.Element != typ.Number {
		t.Errorf("expected Number element type, got %v", arr.Element)
	}
}

func TestSynthesizeArray_Multiple(t *testing.T) {
	result := synthesizeArray([]typ.Type{typ.Number, typ.String})
	if result == nil {
		t.Fatal("expected non-nil type")
	}
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected *typ.Array, got %T", result)
	}
	if arr.Element == nil {
		t.Error("expected non-nil element type (union)")
	}
}

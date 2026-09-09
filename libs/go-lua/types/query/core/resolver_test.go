package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestFuncResolver_NilReceiver(t *testing.T) {
	var r *FuncResolver
	ft, ok := r.Field(typ.String, "x")
	if ok || ft != nil {
		t.Error("expected nil, false for nil receiver")
	}
	it, ok := r.Index(typ.String, typ.Number)
	if ok || it != nil {
		t.Error("expected nil, false for nil receiver")
	}
}

func TestFuncResolver_NilFunctions(t *testing.T) {
	r := &FuncResolver{}
	ft, ok := r.Field(typ.String, "x")
	if ok || ft != nil {
		t.Error("expected nil, false for nil FieldFunc")
	}
	it, ok := r.Index(typ.String, typ.Number)
	if ok || it != nil {
		t.Error("expected nil, false for nil IndexFunc")
	}
}

func TestFuncResolver_WithFieldFunc(t *testing.T) {
	r := &FuncResolver{
		FieldFunc: func(t typ.Type, name string) (typ.Type, bool) {
			if name == "x" {
				return typ.Number, true
			}
			return nil, false
		},
	}

	ft, ok := r.Field(typ.String, "x")
	if !ok || ft != typ.Number {
		t.Errorf("expected Number, true; got %v, %v", ft, ok)
	}

	ft, ok = r.Field(typ.String, "y")
	if ok || ft != nil {
		t.Errorf("expected nil, false; got %v, %v", ft, ok)
	}
}

func TestFuncResolver_WithIndexFunc(t *testing.T) {
	r := &FuncResolver{
		IndexFunc: func(t typ.Type, key typ.Type) (typ.Type, bool) {
			if key == typ.Number {
				return typ.String, true
			}
			return nil, false
		},
	}

	it, ok := r.Index(typ.String, typ.Number)
	if !ok || it != typ.String {
		t.Errorf("expected String, true; got %v, %v", it, ok)
	}

	it, ok = r.Index(typ.String, typ.Boolean)
	if ok || it != nil {
		t.Errorf("expected nil, false; got %v, %v", it, ok)
	}
}

func TestResolver_NotNil(t *testing.T) {
	r := Resolver()
	if r == nil {
		t.Fatal("expected non-nil resolver")
	}
	if r.FieldFunc == nil {
		t.Error("expected non-nil FieldFunc")
	}
	if r.IndexFunc == nil {
		t.Error("expected non-nil IndexFunc")
	}
}

func TestResolver_Field(t *testing.T) {
	r := Resolver()
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	ft, ok := r.Field(rec, "x")
	if !ok {
		t.Error("expected true for existing field")
	}
	if ft != typ.Number {
		t.Errorf("expected Number, got %v", ft)
	}
}

func TestResolver_FieldMissing(t *testing.T) {
	r := Resolver()
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	_, ok := r.Field(rec, "y")
	if ok {
		t.Error("expected false for missing field")
	}
}

package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestFieldAccess(t *testing.T) {
	tp := NewTypeParam("T", nil)
	fa := NewFieldAccess(tp, "name")

	if fa.Kind() != kind.FieldAccess {
		t.Errorf("Kind: got %v, want FieldAccess", fa.Kind())
	}

	if fa.Base != tp {
		t.Error("Base should be type param")
	}

	if fa.Field != "name" {
		t.Errorf("Field: got %q, want %q", fa.Field, "name")
	}

	if fa.String() != "T.name" {
		t.Errorf("String: got %q, want %q", fa.String(), "T.name")
	}
}

func TestFieldAccessEquality(t *testing.T) {
	tp := NewTypeParam("T", nil)
	fa1 := NewFieldAccess(tp, "x")
	fa2 := NewFieldAccess(tp, "x")
	fa3 := NewFieldAccess(tp, "y")

	if !fa1.Equals(fa2) {
		t.Error("T.x should equal T.x")
	}

	if fa1.Equals(fa3) {
		t.Error("T.x should not equal T.y")
	}

	if fa1.Hash() != fa2.Hash() {
		t.Error("equal field accesses should have same hash")
	}
}

func TestFieldAccessDifferentBase(t *testing.T) {
	t1 := NewTypeParam("T", nil)
	t2 := NewTypeParam("U", nil)
	fa1 := NewFieldAccess(t1, "x")
	fa2 := NewFieldAccess(t2, "x")

	if fa1.Equals(fa2) {
		t.Error("T.x should not equal U.x")
	}
}

func TestIndexAccess(t *testing.T) {
	tp := NewTypeParam("T", nil)
	ia := NewIndexAccess(tp, String)

	if ia.Kind() != kind.IndexAccess {
		t.Errorf("Kind: got %v, want IndexAccess", ia.Kind())
	}

	if ia.Base != tp {
		t.Error("Base should be type param")
	}

	if ia.Index != String {
		t.Error("Index should be String")
	}

	if ia.String() != "T[string]" {
		t.Errorf("String: got %q, want %q", ia.String(), "T[string]")
	}
}

func TestIndexAccessEquality(t *testing.T) {
	tp := NewTypeParam("T", nil)
	ia1 := NewIndexAccess(tp, String)
	ia2 := NewIndexAccess(tp, String)
	ia3 := NewIndexAccess(tp, Number)

	if !ia1.Equals(ia2) {
		t.Error("T[string] should equal T[string]")
	}

	if ia1.Equals(ia3) {
		t.Error("T[string] should not equal T[number]")
	}

	if ia1.Hash() != ia2.Hash() {
		t.Error("equal index accesses should have same hash")
	}
}

func TestIndexAccessDifferentBase(t *testing.T) {
	t1 := NewTypeParam("T", nil)
	t2 := NewTypeParam("U", nil)
	ia1 := NewIndexAccess(t1, String)
	ia2 := NewIndexAccess(t2, String)

	if ia1.Equals(ia2) {
		t.Error("T[string] should not equal U[string]")
	}
}

func TestFieldAccessNotEqualToPrimitive(t *testing.T) {
	tp := NewTypeParam("T", nil)

	fa := NewFieldAccess(tp, "x")
	if fa.Equals(Number) {
		t.Error("field access should not equal primitive")
	}
}

func TestIndexAccessNotEqualToPrimitive(t *testing.T) {
	tp := NewTypeParam("T", nil)

	ia := NewIndexAccess(tp, String)
	if ia.Equals(Number) {
		t.Error("index access should not equal primitive")
	}
}

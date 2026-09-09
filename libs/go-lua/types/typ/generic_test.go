package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestTypeParam(t *testing.T) {
	tp := NewTypeParam("T", nil)

	if tp.Kind() != kind.TypeParam {
		t.Errorf("Kind: got %v, want TypeParam", tp.Kind())
	}

	if tp.Name != "T" {
		t.Errorf("Name: got %q, want %q", tp.Name, "T")
	}

	if tp.String() != "T" {
		t.Errorf("String: got %q, want %q", tp.String(), "T")
	}
}

func TestTypeParamWithConstraint(t *testing.T) {
	tp := NewTypeParam("T", Number)

	if tp.Constraint != Number {
		t.Error("Constraint should be Number")
	}

	if tp.String() != "T : number" {
		t.Errorf("String: got %q, want %q", tp.String(), "T : number")
	}
}

func TestTypeParamEquality(t *testing.T) {
	tp1 := NewTypeParam("T", nil)
	tp2 := NewTypeParam("T", nil)
	tp3 := NewTypeParam("U", nil)
	tp4 := NewTypeParam("T", Number)

	if !tp1.Equals(tp2) {
		t.Error("T should equal T")
	}

	if tp1.Equals(tp3) {
		t.Error("T should not equal U")
	}

	if tp1.Equals(tp4) {
		t.Error("T should not equal T : number")
	}
}

func TestGeneric(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, NewArray(tp))

	if g.Kind() != kind.Generic {
		t.Errorf("Kind: got %v, want Generic", g.Kind())
	}

	if g.Name != "List" {
		t.Errorf("Name: got %q, want %q", g.Name, "List")
	}

	if len(g.TypeParams) != 1 {
		t.Errorf("TypeParams: got %d, want 1", len(g.TypeParams))
	}

	if g.String() != "List<T>" {
		t.Errorf("String: got %q, want %q", g.String(), "List<T>")
	}
}

func TestGenericMultipleParams(t *testing.T) {
	k := NewTypeParam("K", nil)
	v := NewTypeParam("V", nil)
	g := NewGeneric("Dict", []*TypeParam{k, v}, NewMap(k, v))

	if g.String() != "Dict<K, V>" {
		t.Errorf("String: got %q, want %q", g.String(), "Dict<K, V>")
	}
}

func TestGenericEquality(t *testing.T) {
	tp1 := NewTypeParam("T", nil)
	tp2 := NewTypeParam("T", nil)
	g1 := NewGeneric("List", []*TypeParam{tp1}, NewArray(tp1))
	g2 := NewGeneric("List", []*TypeParam{tp2}, NewArray(tp2))
	g3 := NewGeneric("Vector", []*TypeParam{tp1}, NewArray(tp1))

	if !g1.Equals(g2) {
		t.Error("List<T> should equal List<T>")
	}

	if g1.Equals(g3) {
		t.Error("List<T> should not equal Vector<T>")
	}
}

func TestInstantiated(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, NewArray(tp))
	i := Instantiate(g, Number)

	if i.Kind() != kind.Instantiated {
		t.Errorf("Kind: got %v, want Instantiated", i.Kind())
	}

	if i.String() != "List<number>" {
		t.Errorf("String: got %q, want %q", i.String(), "List<number>")
	}

	if len(i.TypeArgs) != 1 {
		t.Errorf("TypeArgs: got %d, want 1", len(i.TypeArgs))
	}

	if i.TypeArgs[0] != Number {
		t.Error("TypeArgs[0] should be Number")
	}
}

func TestInstantiatedEquality(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, NewArray(tp))
	i1 := Instantiate(g, Number)
	i2 := Instantiate(g, Number)
	i3 := Instantiate(g, String)

	if !i1.Equals(i2) {
		t.Error("List<number> should equal List<number>")
	}

	if i1.Equals(i3) {
		t.Error("List<number> should not equal List<string>")
	}
}

func TestTypeVar(t *testing.T) {
	tv := NewTypeVar(0)

	if tv.Kind() != kind.TypeVar {
		t.Errorf("Kind: got %v, want TypeVar", tv.Kind())
	}

	if tv.ID != 0 {
		t.Errorf("ID: got %d, want 0", tv.ID)
	}

	if tv.String() != "$a" {
		t.Errorf("String: got %q, want %q", tv.String(), "$a")
	}
}

func TestTypeVarEquality(t *testing.T) {
	tv1 := NewTypeVar(0)
	tv2 := NewTypeVar(0)
	tv3 := NewTypeVar(1)

	if !tv1.Equals(tv2) {
		t.Error("$a should equal $a")
	}

	if tv1.Equals(tv3) {
		t.Error("$a should not equal $b")
	}
}

func TestTypeVarNotEqualToPrimitive(t *testing.T) {
	tv := NewTypeVar(0)
	if tv.Equals(Number) {
		t.Error("type var should not equal primitive")
	}
}

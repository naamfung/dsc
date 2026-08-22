package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestSumEmpty(t *testing.T) {
	s := NewSum("Option", nil)

	if s.Kind() != kind.Sum {
		t.Errorf("Kind: got %v, want Sum", s.Kind())
	}

	if s.Name != "Option" {
		t.Errorf("Name: got %q, want %q", s.Name, "Option")
	}
}

func TestSumWithVariants(t *testing.T) {
	variants := []Variant{
		{Tag: "None", Types: nil},
		{Tag: "Some", Types: []Type{Number}},
	}
	s := NewSum("Option", variants)

	if len(s.Variants) != 2 {
		t.Errorf("Variants: got %d, want 2", len(s.Variants))
	}

	if s.Variants[0].Tag != "None" {
		t.Errorf("first variant tag: got %q, want %q", s.Variants[0].Tag, "None")
	}

	if s.Variants[1].Tag != "Some" {
		t.Errorf("second variant tag: got %q, want %q", s.Variants[1].Tag, "Some")
	}
}

func TestSumString(t *testing.T) {
	variants := []Variant{
		{Tag: "Left", Types: []Type{Number}},
		{Tag: "Right", Types: []Type{String}},
	}
	s := NewSum("Either", variants)

	expected := "enum Either { Left(number) | Right(string) }"
	if s.String() != expected {
		t.Errorf("String: got %q, want %q", s.String(), expected)
	}
}

func TestSumEquality(t *testing.T) {
	v1 := []Variant{{Tag: "A"}, {Tag: "B"}}
	v2 := []Variant{{Tag: "A"}, {Tag: "B"}}
	v3 := []Variant{{Tag: "X"}, {Tag: "Y"}}

	s1 := NewSum("T", v1)
	s2 := NewSum("T", v2)
	s3 := NewSum("T", v3)
	s4 := NewSum("U", v1)

	if !s1.Equals(s2) {
		t.Error("same sum types should be equal")
	}

	if s1.Equals(s3) {
		t.Error("different variants should not be equal")
	}

	if s1.Equals(s4) {
		t.Error("different names should not be equal")
	}
}

func TestInterface(t *testing.T) {
	methods := []Method{
		{Name: "read", Type: Func().Returns(String).Build()},
	}
	i := NewInterface("Reader", methods)

	if i.Kind() != kind.Interface {
		t.Errorf("Kind: got %v, want Interface", i.Kind())
	}

	if i.Name != "Reader" {
		t.Errorf("Name: got %q, want %q", i.Name, "Reader")
	}

	if len(i.Methods) != 1 {
		t.Errorf("Methods: got %d, want 1", len(i.Methods))
	}
}

func TestInterfaceString(t *testing.T) {
	// Named interface renders as just the name
	methods := []Method{
		{Name: "read", Type: Func().Returns(String).Build()},
		{Name: "write", Type: Func().Param("data", String).Build()},
	}
	i := NewInterface("IO", methods)

	s := i.String()
	if s != "IO" {
		t.Errorf("String: got %q, want %q", s, "IO")
	}

	// Anonymous interface expands methods
	anon := NewInterface("", methods)
	anonStr := anon.String()
	if anonStr != "interface { read: fun() -> string; write: fun(data: string) }" {
		t.Errorf("Anonymous String: got %q", anonStr)
	}
}

func TestInterfaceEquality(t *testing.T) {
	m1 := []Method{{Name: "foo", Type: Func().Build()}}
	m2 := []Method{{Name: "foo", Type: Func().Build()}}
	m3 := []Method{{Name: "bar", Type: Func().Build()}}

	i1 := NewInterface("I", m1)
	i2 := NewInterface("I", m2)
	i3 := NewInterface("I", m3)
	i4 := NewInterface("J", m1)

	if !i1.Equals(i2) {
		t.Error("same interfaces should be equal")
	}

	if i1.Equals(i3) {
		t.Error("different methods should not be equal")
	}

	if i1.Equals(i4) {
		t.Error("different names should not be equal")
	}
}

func TestSumNotEqualToPrimitive(t *testing.T) {
	s := NewSum("T", nil)
	if s.Equals(Number) {
		t.Error("sum type should not equal primitive")
	}
}

func TestInterfaceNotEqualToPrimitive(t *testing.T) {
	i := NewInterface("I", nil)
	if i.Equals(Number) {
		t.Error("interface should not equal primitive")
	}
}

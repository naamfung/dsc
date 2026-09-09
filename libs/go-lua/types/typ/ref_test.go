package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestRef(t *testing.T) {
	r := NewRef("math", "Vector")

	if r.Kind() != kind.Ref {
		t.Errorf("Kind: got %v, want Ref", r.Kind())
	}

	if r.Module != "math" {
		t.Errorf("Module: got %q, want %q", r.Module, "math")
	}

	if r.Name != "Vector" {
		t.Errorf("Name: got %q, want %q", r.Name, "Vector")
	}

	if r.String() != "math.Vector" {
		t.Errorf("String: got %q, want %q", r.String(), "math.Vector")
	}
}

func TestRefNoModule(t *testing.T) {
	r := NewRef("", "LocalType")

	if r.String() != "LocalType" {
		t.Errorf("String: got %q, want %q", r.String(), "LocalType")
	}
}

func TestRefEquality(t *testing.T) {
	r1 := NewRef("mod", "T")
	r2 := NewRef("mod", "T")
	r3 := NewRef("mod", "U")
	r4 := NewRef("other", "T")

	if !r1.Equals(r2) {
		t.Error("mod.T should equal mod.T")
	}

	if r1.Equals(r3) {
		t.Error("mod.T should not equal mod.U")
	}

	if r1.Equals(r4) {
		t.Error("mod.T should not equal other.T")
	}

	if r1.Hash() != r2.Hash() {
		t.Error("equal refs should have same hash")
	}
}

func TestAlias(t *testing.T) {
	a := NewAlias("StringList", NewArray(String))

	if a.Kind() != kind.Alias {
		t.Errorf("Kind: got %v, want Alias", a.Kind())
	}

	if a.Name != "StringList" {
		t.Errorf("Name: got %q, want %q", a.Name, "StringList")
	}

	if a.String() != "StringList" {
		t.Errorf("String: got %q, want %q", a.String(), "StringList")
	}
}

func TestAliasEquality(t *testing.T) {
	a1 := NewAlias("T", Number)
	a2 := NewAlias("T", Number)
	a3 := NewAlias("T", String)
	a4 := NewAlias("U", Number)

	if !a1.Equals(a2) {
		t.Error("same aliases should be equal")
	}

	if a1.Equals(a3) {
		t.Error("different targets should not be equal")
	}

	if !a1.Equals(a4) {
		t.Error("aliases with same target should be structurally equal regardless of name")
	}
}

func TestPlatform(t *testing.T) {
	p := NewPlatform("userdata")

	if p.Kind() != kind.Platform {
		t.Errorf("Kind: got %v, want Platform", p.Kind())
	}

	if p.Name != "userdata" {
		t.Errorf("Name: got %q, want %q", p.Name, "userdata")
	}

	if p.String() != "userdata" {
		t.Errorf("String: got %q, want %q", p.String(), "userdata")
	}
}

func TestPlatformEquality(t *testing.T) {
	p1 := NewPlatform("userdata")
	p2 := NewPlatform("userdata")
	p3 := NewPlatform("thread")

	if !p1.Equals(p2) {
		t.Error("same platform types should be equal")
	}

	if p1.Equals(p3) {
		t.Error("different platform types should not be equal")
	}
}

func TestMeta(t *testing.T) {
	m := NewMeta(Number)

	if m.Kind() != kind.Meta {
		t.Errorf("Kind: got %v, want Meta", m.Kind())
	}

	if m.Of != Number {
		t.Error("Of should be Number")
	}

	if m.String() != "typeof(number)" {
		t.Errorf("String: got %q, want %q", m.String(), "typeof(number)")
	}
}

func TestMetaEquality(t *testing.T) {
	m1 := NewMeta(Number)
	m2 := NewMeta(Number)
	m3 := NewMeta(String)

	if !m1.Equals(m2) {
		t.Error("typeof(number) should equal typeof(number)")
	}

	if m1.Equals(m3) {
		t.Error("typeof(number) should not equal typeof(string)")
	}

	if m1.Hash() != m2.Hash() {
		t.Error("equal metas should have same hash")
	}
}

func TestRefNotEqualToPrimitive(t *testing.T) {
	r := NewRef("", "T")
	if r.Equals(Number) {
		t.Error("ref should not equal primitive")
	}
}

func TestAliasStructuralEquality(t *testing.T) {
	a := NewAlias("T", Number)
	if !a.Equals(Number) {
		t.Error("alias should equal its target type structurally")
	}

	if !TypeEquals(Number, a) {
		t.Error("TypeEquals should handle alias comparison symmetrically")
	}
}

func TestAliasTransitiveEquality(t *testing.T) {
	a1 := NewAlias("A", Number)
	a2 := NewAlias("B", Number)

	if !a1.Equals(a2) {
		t.Error("aliases with same target should be equal")
	}
}

func TestPlatformNotEqualToPrimitive(t *testing.T) {
	p := NewPlatform("x")
	if p.Equals(Number) {
		t.Error("platform should not equal primitive")
	}
}

func TestMetaNotEqualToPrimitive(t *testing.T) {
	m := NewMeta(Number)
	if m.Equals(Number) {
		t.Error("meta should not equal primitive")
	}
}

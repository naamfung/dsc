package domain

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestShapeDomain_ApplyConstraint(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	key := constraint.PathKey("sym1")

	fieldResolver := func(ty typ.Type, name string) (typ.Type, bool) {
		if r, ok := ty.(*typ.Record); ok {
			for _, f := range r.Fields {
				if f.Name == name {
					return f.Type, true
				}
			}
		}
		return nil, false
	}

	env := constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return union
			}
			return nil
		},
		Resolver: &mockResolver{fieldFunc: fieldResolver},
	}

	d := NewShapeDomain(env)

	path := constraint.Path{Root: "x", Symbol: 1}
	c := constraint.FieldEquals{Target: path, Field: "tag", Value: typ.LiteralString("a")}

	ok := d.ApplyConstraint(c, key)
	if !ok {
		t.Fatal("ApplyConstraint should succeed")
	}

	result := d.NarrowedTypeAt(key)
	if result == nil {
		t.Fatal("expected narrowed type")
	}

	if !typ.TypeEquals(result, typeA) {
		t.Fatalf("expected typeA, got %v", result)
	}
}

type mockResolver struct {
	fieldFunc func(typ.Type, string) (typ.Type, bool)
}

func (m *mockResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	if m.fieldFunc != nil {
		return m.fieldFunc(t, name)
	}
	return nil, false
}

func (m *mockResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	return nil, false
}

func (m *mockResolver) Call(t typ.Type, args []typ.Type) ([]typ.Type, bool) {
	return nil, false
}

func (m *mockResolver) Method(t typ.Type, name string) (typ.Type, bool) {
	return nil, false
}

func (m *mockResolver) Expand(t typ.Type) typ.Type {
	return t
}

func TestShapeDomain_TypeAt_FallsBackToResolver(t *testing.T) {
	key := constraint.PathKey("sym1")
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return typ.String
			}
			return nil
		},
	}

	d := NewShapeDomain(env)

	result := d.TypeAt(key)
	if result == nil || result.Kind() != typ.String.Kind() {
		t.Fatalf("expected string from resolver, got %v", result)
	}
}

func TestShapeDomain_NarrowedTypeAt_ReturnsNilIfNotNarrowed(t *testing.T) {
	env := constraint.Env{}
	d := NewShapeDomain(env)

	result := d.NarrowedTypeAt("nonexistent")
	if result != nil {
		t.Fatal("expected nil for non-narrowed key")
	}
}

func TestShapeDomain_Clone(t *testing.T) {
	env := constraint.Env{}
	d := NewShapeDomain(env)
	d.Narrowed["x"] = typ.String

	clone := d.Clone().(*ShapeDomain)

	if clone.NarrowedTypeAt("x").Kind() != typ.String.Kind() {
		t.Fatal("clone should have same narrowed type")
	}

	clone.Narrowed["x"] = typ.Number
	if d.NarrowedTypeAt("x").Kind() != typ.String.Kind() {
		t.Fatal("original should be unchanged")
	}
}

func TestShapeDomain_Join(t *testing.T) {
	env := constraint.Env{}

	a := NewShapeDomain(env)
	a.Narrowed["x"] = typ.String

	b := NewShapeDomain(env)
	b.Narrowed["x"] = typ.Number

	joined := a.Join(b).(*ShapeDomain)
	result := joined.NarrowedTypeAt("x")

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(u.Members))
	}
}

func TestShapeDomain_Join_DropsNonCommonKeys(t *testing.T) {
	env := constraint.Env{}

	a := NewShapeDomain(env)
	a.Narrowed["x"] = typ.String

	b := NewShapeDomain(env)

	joined := a.Join(b).(*ShapeDomain)
	if _, hasKey := joined.Narrowed["x"]; hasKey {
		t.Fatal("key should be dropped when not in both sides")
	}
}

func TestShapeDomain_Join_UnsatSideIgnored(t *testing.T) {
	env := constraint.Env{}

	a := NewShapeDomain(env)
	a.Narrowed["x"] = typ.String

	b := NewShapeDomain(env)
	b.Unsat = true

	joined := a.Join(b).(*ShapeDomain)
	if joined.NarrowedTypeAt("x").Kind() != typ.String.Kind() {
		t.Fatal("should use non-unsat side")
	}
}

func TestShapeDomain_IsUnsat(t *testing.T) {
	env := constraint.Env{}
	d := NewShapeDomain(env)

	if d.IsUnsat() {
		t.Fatal("new domain should not be unsat")
	}

	d.Unsat = true
	if !d.IsUnsat() {
		t.Fatal("should be unsat after setting flag")
	}
}

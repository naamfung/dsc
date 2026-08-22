package domain

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func benchEnv(types map[constraint.PathKey]typ.Type) constraint.Env {
	return constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				return constraint.PathKey(p.Root)
			}
			return p.Key()
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if types == nil {
				return nil
			}
			return types[key]
		},
	}
}

func BenchmarkClassifyAtom_HasType(b *testing.B) {
	atom := constraint.Atom{Kind: constraint.AtomKindHasType}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyAtom(atom)
	}
}

func BenchmarkClassifyAtom_EqNil(b *testing.B) {
	atom := constraint.Atom{
		Kind:  constraint.AtomKindEq,
		Right: constraint.TermNil(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyAtom(atom)
	}
}

func BenchmarkClassifyAtom_EqVar(b *testing.B) {
	atom := constraint.Atom{
		Kind:  constraint.AtomKindEq,
		Left:  constraint.TermVar("x"),
		Right: constraint.TermVar("y"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyAtom(atom)
	}
}

func BenchmarkTypeDomain_ApplyAtom_Truthy(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})
	atom := constraint.AtomTruthy(constraint.TermVar(key))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewTypeDomain(env)
		d.ApplyAtom(atom)
	}
}

func BenchmarkTypeDomain_ApplyAtom_HasType(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Number, typ.Boolean, typ.Nil),
	})
	atom := constraint.AtomHasType(constraint.TermVar(key), narrow.BuiltinTypeKey("string"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewTypeDomain(env)
		d.ApplyAtom(atom)
	}
}

func BenchmarkTypeDomain_Clone(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})
	d := NewTypeDomain(env)
	d.ApplyAtom(constraint.AtomTruthy(constraint.TermVar(key)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Clone()
	}
}

func BenchmarkTypeDomain_Join(b *testing.B) {
	env := benchEnv(nil)
	a := NewTypeDomain(env)
	a.Narrowed["x"] = typ.String
	a.Narrowed["y"] = typ.Number

	c := NewTypeDomain(env)
	c.Narrowed["x"] = typ.Number
	c.Narrowed["y"] = typ.Boolean

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Join(c)
	}
}

func BenchmarkTypeDomain_MultipleNarrowings(b *testing.B) {
	keys := make([]constraint.PathKey, 10)
	types := make(map[constraint.PathKey]typ.Type, 10)
	for i := 0; i < 10; i++ {
		key := constraint.PathKey(string('a' + rune(i)))
		keys[i] = key
		types[key] = typ.NewUnion(typ.String, typ.Number, typ.Nil)
	}
	env := benchEnv(types)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewTypeDomain(env)
		for _, key := range keys {
			d.ApplyAtom(constraint.AtomTruthy(constraint.TermVar(key)))
		}
	}
}

func BenchmarkShapeDomain_Clone(b *testing.B) {
	env := benchEnv(nil)
	d := NewShapeDomain(env)
	d.Narrowed["x"] = typ.String
	d.Narrowed["y"] = typ.Number
	d.Narrowed["z"] = typ.Boolean

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Clone()
	}
}

func BenchmarkShapeDomain_Join(b *testing.B) {
	env := benchEnv(nil)
	a := NewShapeDomain(env)
	a.Narrowed["x"] = typ.String
	a.Narrowed["y"] = typ.Number

	c := NewShapeDomain(env)
	c.Narrowed["x"] = typ.Number
	c.Narrowed["y"] = typ.Boolean

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Join(c)
	}
}

func BenchmarkIsChildPath(b *testing.B) {
	parent := "sym1@1"
	child := "sym1@1.field.nested"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsChildPath(parent, child)
	}
}

func BenchmarkIsChildPath_NotChild(b *testing.B) {
	parent := "sym1@1"
	notChild := "sym2@1.field"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsChildPath(parent, notChild)
	}
}

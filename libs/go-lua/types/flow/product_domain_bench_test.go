package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func benchProductEnv(types map[constraint.PathKey]typ.Type) constraint.Env {
	return constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				if p.IsPlaceholder() {
					return constraint.PathKey(p.Root)
				}
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

func BenchmarkProductDomain_ApplyAtom_TypeOnly(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchProductEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})
	atom := constraint.AtomTruthy(constraint.TermVar(key))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyAtom(atom)
	}
}

func BenchmarkProductDomain_ApplyAtom_NumericOnly(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchProductEnv(nil)
	atom := constraint.AtomGe(constraint.TermVar(key), constraint.TermConst(0))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyAtom(atom)
	}
}

func BenchmarkProductDomain_ApplyAtom_Both(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchProductEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Number),
	})
	atomType := constraint.AtomHasType(constraint.TermVar(key), narrow.BuiltinTypeKey("number"))
	atomNumeric := constraint.AtomGe(constraint.TermVar(key), constraint.TermConst(0))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyAtom(atomType)
		d.ApplyAtom(atomNumeric)
	}
}

func BenchmarkProductDomain_ApplyConjunction_Simple(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchProductEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})
	path := constraint.Path{Root: "x"}
	constraints := constraint.NewConjunction(constraint.Truthy{Path: path})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyConjunction(constraints)
	}
}

func BenchmarkProductDomain_ApplyConjunction_Multiple(b *testing.B) {
	types := make(map[constraint.PathKey]typ.Type, 5)
	constraints := make([]constraint.Constraint, 5)
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		key := constraint.PathKey(name)
		types[key] = typ.NewUnion(typ.String, typ.Nil)
		constraints[i] = constraint.Truthy{Path: constraint.Path{Root: name}}
	}
	env := benchProductEnv(types)
	conj := constraint.NewConjunction(constraints...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyConjunction(conj)
	}
}

func BenchmarkProductDomain_ApplyCondition_SingleDisjunct(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchProductEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})
	path := constraint.Path{Root: "x"}
	cond := constraint.FromConstraints(constraint.Truthy{Path: path})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyCondition(cond)
	}
}

func BenchmarkProductDomain_ApplyCondition_TwoDisjuncts(b *testing.B) {
	keyX := constraint.PathKey("x")
	keyY := constraint.PathKey("y")
	env := benchProductEnv(map[constraint.PathKey]typ.Type{
		keyX: typ.NewUnion(typ.String, typ.Nil),
		keyY: typ.NewUnion(typ.Number, typ.Nil),
	})
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}
	cond := constraint.Or(
		constraint.FromConstraints(constraint.Truthy{Path: pathX}),
		constraint.FromConstraints(constraint.Truthy{Path: pathY}),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyCondition(cond)
	}
}

func BenchmarkProductDomain_Clone(b *testing.B) {
	types := make(map[constraint.PathKey]typ.Type, 5)
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		types[constraint.PathKey(name)] = typ.NewUnion(typ.String, typ.Nil)
	}
	env := benchProductEnv(types)
	d := NewProductDomain(env)
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		d.ApplyAtom(constraint.AtomTruthy(constraint.TermVar(constraint.PathKey(name))))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Clone()
	}
}

func BenchmarkProductDomain_Join(b *testing.B) {
	env := benchProductEnv(nil)

	a := NewProductDomain(env)
	a.Type.Narrowed["x"] = typ.String
	a.Type.Narrowed["y"] = typ.Number

	c := NewProductDomain(env)
	c.Type.Narrowed["x"] = typ.Number
	c.Type.Narrowed["y"] = typ.Boolean

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Join(c)
	}
}

func BenchmarkProductDomain_TypeAt(b *testing.B) {
	key := constraint.PathKey("x")
	env := benchProductEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})
	d := NewProductDomain(env)
	d.ApplyAtom(constraint.AtomTruthy(constraint.TermVar(key)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.TypeAt(key)
	}
}

func BenchmarkProductDomain_NarrowedChildPaths(b *testing.B) {
	env := benchProductEnv(nil)
	d := NewProductDomain(env)

	parent := constraint.PathKey("sym1@1")
	for i := 0; i < 10; i++ {
		child := constraint.PathKey("sym1@1." + string(rune('a'+i)))
		d.Type.Narrowed[child] = typ.String
	}
	d.Shape.Narrowed["sym1@1.field"] = typ.Number

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.NarrowedChildPaths(parent)
	}
}

func BenchmarkProductDomain_EqPath_Propagation(b *testing.B) {
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	types := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(typ.String, typ.Number),
		pathY.Key(): typ.NewUnion(typ.String, typ.Number),
	}
	env := benchProductEnv(types)
	constraints := []constraint.Constraint{
		constraint.NewEqPath(pathX, pathY),
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewProductDomain(env)
		d.ApplyConjunction(constraints)
	}
}

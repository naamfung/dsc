package core

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestInstantiateGeneric(t *testing.T) {
	t.Run("nil generic", func(t *testing.T) {
		_, err := InstantiateGeneric(nil, nil)
		if !errors.Is(err, ErrNotGeneric) {
			t.Errorf("expected ErrNotGeneric, got %v", err)
		}
	})

	t.Run("wrong type arg count", func(t *testing.T) {
		g := typ.NewGeneric("G",
			[]*typ.TypeParam{{Name: "T"}},
			typ.NewTypeParam("T", nil),
		)

		_, err := InstantiateGeneric(g, []typ.Type{typ.String, typ.Integer})
		if !errors.Is(err, ErrTypeArgCount) {
			t.Errorf("expected ErrTypeArgCount, got %v", err)
		}
	})

	t.Run("constraint violation", func(t *testing.T) {
		g := typ.NewGeneric("G",
			[]*typ.TypeParam{{Name: "T", Constraint: typ.String}},
			typ.NewTypeParam("T", nil),
		)

		_, err := InstantiateGeneric(g, []typ.Type{typ.Integer})
		if !errors.Is(err, ErrConstraintViolation) {
			t.Errorf("expected ErrConstraintViolation, got %v", err)
		}
	})

	t.Run("valid instantiation", func(t *testing.T) {
		g := typ.NewGeneric("G",
			[]*typ.TypeParam{{Name: "T"}},
			typ.NewArray(typ.NewTypeParam("T", nil)),
		)

		result, err := InstantiateGeneric(g, []typ.Type{typ.String})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
		// Result should be Array of String
		if arr, ok := result.(*typ.Array); ok {
			if arr.Element != typ.String {
				t.Errorf("expected string element, got %v", arr.Element)
			}
		} else {
			t.Errorf("expected Array, got %T", result)
		}
	})
}

func TestSubstitute(t *testing.T) {
	subst := map[string]typ.Type{"T": typ.String}

	t.Run("nil type", func(t *testing.T) {
		result := Substitute(nil, subst)
		if result != nil {
			t.Error("expected nil")
		}
	})

	t.Run("type param replaced", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)

		result := Substitute(tp, subst)
		if result != typ.String {
			t.Errorf("expected String, got %v", result)
		}
	})

	t.Run("type param not in subst", func(t *testing.T) {
		tp := typ.NewTypeParam("U", nil)
		result := Substitute(tp, subst)

		if result != tp {
			t.Errorf("expected same TypeParam, got %v", result)
		}
	})

	t.Run("array element substituted", func(t *testing.T) {
		arr := typ.NewArray(typ.NewTypeParam("T", nil))

		result := Substitute(arr, subst)
		if a, ok := result.(*typ.Array); ok {
			if a.Element != typ.String {
				t.Errorf("expected String element")
			}
		} else {
			t.Errorf("expected Array")
		}
	})

	t.Run("array unchanged", func(t *testing.T) {
		arr := typ.NewArray(typ.Integer)
		result := Substitute(arr, subst)

		if result != arr {
			t.Error("expected same array (no change)")
		}
	})

	t.Run("map substituted", func(t *testing.T) {
		m := typ.NewMap(typ.NewTypeParam("T", nil), typ.NewTypeParam("T", nil))

		result := Substitute(m, subst)
		if mp, ok := result.(*typ.Map); ok {
			if mp.Key != typ.String || mp.Value != typ.String {
				t.Error("expected String key and value")
			}
		} else {
			t.Errorf("expected Map")
		}
	})

	t.Run("map unchanged", func(t *testing.T) {
		m := typ.NewMap(typ.String, typ.Integer)
		result := Substitute(m, subst)

		if result != m {
			t.Error("expected same map")
		}
	})

	t.Run("tuple substituted", func(t *testing.T) {
		tuple := typ.NewTuple(typ.NewTypeParam("T", nil), typ.Integer)

		result := Substitute(tuple, subst)
		if tup, ok := result.(*typ.Tuple); ok {
			if tup.Elements[0] != typ.String {
				t.Error("expected first element substituted")
			}
		} else {
			t.Errorf("expected Tuple")
		}
	})

	t.Run("tuple unchanged", func(t *testing.T) {
		tuple := typ.NewTuple(typ.String, typ.Integer)
		result := Substitute(tuple, subst)

		if result != tuple {
			t.Error("expected same tuple")
		}
	})

	t.Run("optional substituted", func(t *testing.T) {
		opt := typ.NewOptional(typ.NewTypeParam("T", nil))

		result := Substitute(opt, subst)
		if o, ok := result.(*typ.Optional); ok {
			if o.Inner != typ.String {
				t.Error("expected String inner")
			}
		} else {
			t.Errorf("expected Optional")
		}
	})

	t.Run("optional unchanged", func(t *testing.T) {
		opt := typ.NewOptional(typ.Integer)
		result := Substitute(opt, subst)

		if result != opt {
			t.Error("expected same optional")
		}
	})

	t.Run("union substituted", func(t *testing.T) {
		union := typ.NewUnion(typ.NewTypeParam("T", nil), typ.Integer)
		result := Substitute(union, subst)
		// Union might normalize, just check it's not nil
		if result == nil {
			t.Error("expected non-nil union")
		}
	})

	t.Run("union unchanged", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer)
		result := Substitute(union, subst)

		if result != union {
			t.Error("expected same union")
		}
	})

	t.Run("intersection substituted", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("x", typ.NewTypeParam("T", nil)).Build()
		rec2 := typ.NewRecord().Field("y", typ.Integer).Build()
		inter := typ.NewIntersection(rec1, rec2)

		result := Substitute(inter, subst)
		if result == nil {
			t.Error("expected non-nil intersection")
		}
	})

	t.Run("intersection unchanged", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("x", typ.String).Build()
		rec2 := typ.NewRecord().Field("y", typ.Integer).Build()
		inter := typ.NewIntersection(rec1, rec2)
		result := Substitute(inter, subst)

		if result != inter {
			t.Error("expected same intersection")
		}
	})

	t.Run("ref unchanged", func(t *testing.T) {
		ref := typ.NewRef("mod", "Type")
		result := Substitute(ref, subst)

		if result != ref {
			t.Error("expected same ref")
		}
	})

	t.Run("alias substituted", func(t *testing.T) {
		alias := typ.NewAlias("A", typ.NewTypeParam("T", nil))

		result := Substitute(alias, subst)
		if a, ok := result.(*typ.Alias); ok {
			if a.Target != typ.String {
				t.Error("expected String target")
			}
		} else {
			t.Errorf("expected Alias")
		}
	})

	t.Run("alias unchanged", func(t *testing.T) {
		alias := typ.NewAlias("A", typ.Integer)
		result := Substitute(alias, subst)

		if result != alias {
			t.Error("expected same alias")
		}
	})

	t.Run("generic unchanged", func(t *testing.T) {
		g := typ.NewGeneric("G", []*typ.TypeParam{{Name: "U"}}, typ.NewTypeParam("U", nil))
		result := Substitute(g, subst)

		if result != g {
			t.Error("expected same generic")
		}
	})

	t.Run("primitive unchanged", func(t *testing.T) {
		result := Substitute(typ.String, subst)
		if result != typ.String {
			t.Error("expected same primitive")
		}
	})
}

func TestSubstituteFunction(t *testing.T) {
	subst := map[string]typ.Type{"T": typ.String}

	t.Run("function params substituted", func(t *testing.T) {
		fn := typ.Func().
			Param("x", typ.NewTypeParam("T", nil)).
			Returns(typ.Integer).
			Build()

		result := Substitute(fn, subst)
		if f, ok := result.(*typ.Function); ok {
			if f.Params[0].Type != typ.String {
				t.Error("expected String param")
			}
		} else {
			t.Errorf("expected Function")
		}
	})

	t.Run("function returns substituted", func(t *testing.T) {
		fn := typ.Func().
			Param("x", typ.Integer).
			Returns(typ.NewTypeParam("T", nil)).
			Build()

		result := Substitute(fn, subst)
		if f, ok := result.(*typ.Function); ok {
			if f.Returns[0] != typ.String {
				t.Error("expected String return")
			}
		} else {
			t.Errorf("expected Function")
		}
	})

	t.Run("function variadic substituted", func(t *testing.T) {
		fn := typ.Func().
			Variadic(typ.NewTypeParam("T", nil)).
			Returns(typ.Integer).
			Build()

		result := Substitute(fn, subst)
		if f, ok := result.(*typ.Function); ok {
			if f.Variadic != typ.String {
				t.Error("expected String variadic")
			}
		} else {
			t.Errorf("expected Function")
		}
	})

	t.Run("function unchanged", func(t *testing.T) {
		fn := typ.Func().
			Param("x", typ.Integer).
			Returns(typ.Boolean).
			Build()
		result := Substitute(fn, subst)

		if result != fn {
			t.Error("expected same function")
		}
	})
}

func TestSubstituteRecord(t *testing.T) {
	subst := map[string]typ.Type{"T": typ.String}

	t.Run("record field substituted", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.NewTypeParam("T", nil)).Build()

		result := Substitute(rec, subst)
		if r, ok := result.(*typ.Record); ok {
			if r.Fields[0].Type != typ.String {
				t.Error("expected String field type")
			}
		} else {
			t.Errorf("expected Record")
		}
	})

	t.Run("record unchanged", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.Integer).Build()
		result := Substitute(rec, subst)

		if result != rec {
			t.Error("expected same record")
		}
	})

	t.Run("record with optional field", func(t *testing.T) {
		rec := typ.NewRecord().OptField("x", typ.NewTypeParam("T", nil)).Build()

		result := Substitute(rec, subst)
		if r, ok := result.(*typ.Record); ok {
			if !r.Fields[0].Optional {
				t.Error("expected optional field")
			}
		}
	})

	t.Run("record with readonly field", func(t *testing.T) {
		rec := typ.NewRecord().ReadonlyField("x", typ.NewTypeParam("T", nil)).Build()

		result := Substitute(rec, subst)
		if r, ok := result.(*typ.Record); ok {
			if !r.Fields[0].Readonly {
				t.Error("expected readonly field")
			}
		}
	})
}

func TestSubstituteInstantiated(t *testing.T) {
	subst := map[string]typ.Type{"T": typ.String}

	g := typ.NewGeneric("G", []*typ.TypeParam{{Name: "U"}}, typ.NewArray(typ.NewTypeParam("U", nil)))
	inst := typ.Instantiate(g, typ.NewTypeParam("T", nil))

	t.Run("instantiated type args substituted", func(t *testing.T) {
		result := Substitute(inst, subst)
		if i, ok := result.(*typ.Instantiated); ok {
			if i.TypeArgs[0] != typ.String {
				t.Error("expected String type arg")
			}
		} else {
			t.Errorf("expected Instantiated")
		}
	})

	t.Run("instantiated unchanged", func(t *testing.T) {
		inst2 := typ.Instantiate(g, typ.Integer)
		result := Substitute(inst2, subst)

		if result != inst2 {
			t.Error("expected same instantiated")
		}
	})
}

func TestResolveInstantiated(t *testing.T) {
	g := typ.NewGeneric("G",
		[]*typ.TypeParam{{Name: "T"}},
		typ.NewArray(typ.NewTypeParam("T", nil)),
	)
	inst := typ.Instantiate(g, typ.String)

	result, err := ResolveInstantiated(inst)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if arr, ok := result.(*typ.Array); ok {
		if arr.Element != typ.String {
			t.Error("expected String element")
		}
	} else {
		t.Errorf("expected Array, got %T", result)
	}
}

func TestCollectTypeParams(t *testing.T) {
	t.Run("nil type", func(t *testing.T) {
		params := CollectTypeParams(nil)
		if len(params) != 0 {
			t.Error("expected empty")
		}
	})

	t.Run("no type params", func(t *testing.T) {
		params := CollectTypeParams(typ.String)
		if len(params) != 0 {
			t.Error("expected empty")
		}
	})

	t.Run("single type param", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)

		params := CollectTypeParams(tp)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in array", func(t *testing.T) {
		arr := typ.NewArray(typ.NewTypeParam("T", nil))

		params := CollectTypeParams(arr)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in map", func(t *testing.T) {
		m := typ.NewMap(typ.NewTypeParam("K", nil), typ.NewTypeParam("V", nil))

		params := CollectTypeParams(m)
		if len(params) != 2 {
			t.Errorf("expected 2, got %d", len(params))
		}
	})

	t.Run("type param in function", func(t *testing.T) {
		fn := typ.Func().
			Param("x", typ.NewTypeParam("T", nil)).
			Variadic(typ.NewTypeParam("U", nil)).
			Returns(typ.NewTypeParam("V", nil)).
			Build()

		params := CollectTypeParams(fn)
		if len(params) != 3 {
			t.Errorf("expected 3, got %d", len(params))
		}
	})

	t.Run("type param in record", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.NewTypeParam("T", nil)).Build()

		params := CollectTypeParams(rec)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in tuple", func(t *testing.T) {
		tuple := typ.NewTuple(typ.NewTypeParam("T", nil), typ.NewTypeParam("U", nil))

		params := CollectTypeParams(tuple)
		if len(params) != 2 {
			t.Errorf("expected 2, got %d", len(params))
		}
	})

	t.Run("type param in optional", func(t *testing.T) {
		opt := typ.NewOptional(typ.NewTypeParam("T", nil))

		params := CollectTypeParams(opt)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in union", func(t *testing.T) {
		union := typ.NewUnion(typ.NewTypeParam("T", nil), typ.String)

		params := CollectTypeParams(union)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in intersection", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.NewTypeParam("T", nil)).Build()
		inter := typ.NewIntersection(rec, typ.NewRecord().Build())

		params := CollectTypeParams(inter)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in alias", func(t *testing.T) {
		alias := typ.NewAlias("A", typ.NewTypeParam("T", nil))

		params := CollectTypeParams(alias)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in instantiated", func(t *testing.T) {
		g := typ.NewGeneric("G", []*typ.TypeParam{{Name: "U"}}, typ.NewTypeParam("U", nil))
		inst := typ.Instantiate(g, typ.NewTypeParam("T", nil))

		params := CollectTypeParams(inst)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("ref type", func(t *testing.T) {
		ref := typ.NewRef("mod", "Type")

		params := CollectTypeParams(ref)
		if len(params) != 0 {
			t.Error("expected empty for ref")
		}
	})
}

func TestHasTypeParams(t *testing.T) {
	if HasTypeParams(typ.String) {
		t.Error("expected false for primitive")
	}

	if !HasTypeParams(typ.NewTypeParam("T", nil)) {
		t.Error("expected true for type param")
	}

	if !HasTypeParams(typ.NewArray(typ.NewTypeParam("T", nil))) {
		t.Error("expected true for array with type param")
	}
}

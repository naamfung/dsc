package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestComputeReachability(t *testing.T) {
	t.Run("nil record", func(t *testing.T) {
		result := ComputeReachability(nil)
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		if len(result.CanReach) != 0 {
			t.Error("expected empty reach map")
		}
	})

	t.Run("empty record", func(t *testing.T) {
		rec := typ.NewRecord().Build()

		result := ComputeReachability(rec)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("record with primitive fields", func(t *testing.T) {
		rec := typ.NewRecord().
			Field("a", typ.String).
			Field("b", typ.Integer).
			Build()

		result := ComputeReachability(rec)
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		if len(result.CanReach) != 2 {
			t.Errorf("expected 2 entries, got %d", len(result.CanReach))
		}
	})
}

func TestIsAcyclicByReach(t *testing.T) {
	t.Run("nil record", func(t *testing.T) {
		if !IsAcyclicByReach(nil, nil) {
			t.Error("expected true for nil record")
		}
	})

	t.Run("nil reach matrix", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.String).Build()
		if !IsAcyclicByReach(rec, nil) {
			t.Error("expected true for nil reach matrix")
		}
	})

	t.Run("no self-references", func(t *testing.T) {
		rec := typ.NewRecord().
			Field("a", typ.String).
			Field("b", typ.Integer).
			Build()
		reach := ComputeReachability(rec)

		if !IsAcyclicByReach(rec, reach) {
			t.Error("expected true for record with no self-references")
		}
	})

	t.Run("with self-referencing field", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.String).Build()
		reach := &ReachMatrix{
			CanReach: map[string]map[string]bool{
				"x": {"x": true},
			},
		}

		if IsAcyclicByReach(rec, reach) {
			t.Error("expected false for self-referencing field")
		}
	})
}

func TestFieldCanCycle(t *testing.T) {
	t.Run("nil record", func(t *testing.T) {
		if FieldCanCycle(nil, "x") {
			t.Error("expected false for nil record")
		}
	})

	t.Run("missing field", func(t *testing.T) {
		rec := typ.NewRecord().Field("a", typ.String).Build()
		if FieldCanCycle(rec, "missing") {
			t.Error("expected false for missing field")
		}
	})

	t.Run("primitive field", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.String).Build()
		if FieldCanCycle(rec, "x") {
			t.Error("expected false for primitive field")
		}
	})

	t.Run("field with any type", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.Any).Build()
		if !FieldCanCycle(rec, "x") {
			t.Error("expected true for field with any type")
		}
	})
}

func TestCanContain(t *testing.T) {
	target := typ.NewRecord().Field("id", typ.Integer).Build()

	t.Run("nil container", func(t *testing.T) {
		if CanContain(nil, target) {
			t.Error("expected false for nil container")
		}
	})

	t.Run("same type", func(t *testing.T) {
		if !CanContain(target, target) {
			t.Error("expected true for same type")
		}
	})

	t.Run("primitive container", func(t *testing.T) {
		if CanContain(typ.String, target) {
			t.Error("expected false for primitive container")
		}
	})

	t.Run("array containing target", func(t *testing.T) {
		arr := typ.NewArray(target)
		if !CanContain(arr, target) {
			t.Error("expected true for array containing target")
		}
	})

	t.Run("array not containing target", func(t *testing.T) {
		arr := typ.NewArray(typ.String)
		if CanContain(arr, target) {
			t.Error("expected false for array not containing target")
		}
	})

	t.Run("map key containing target", func(t *testing.T) {
		m := typ.NewMap(target, typ.String)
		if !CanContain(m, target) {
			t.Error("expected true for map key containing target")
		}
	})

	t.Run("map value containing target", func(t *testing.T) {
		m := typ.NewMap(typ.String, target)
		if !CanContain(m, target) {
			t.Error("expected true for map value containing target")
		}
	})

	t.Run("optional containing target", func(t *testing.T) {
		opt := typ.NewOptional(target)
		if !CanContain(opt, target) {
			t.Error("expected true for optional containing target")
		}
	})

	t.Run("union containing target", func(t *testing.T) {
		union := typ.NewUnion(typ.String, target)
		if !CanContain(union, target) {
			t.Error("expected true for union containing target")
		}
	})

	t.Run("intersection containing target", func(t *testing.T) {
		inter := typ.NewIntersection(typ.NewRecord().Build(), target)
		if !CanContain(inter, target) {
			t.Error("expected true for intersection containing target")
		}
	})

	t.Run("tuple containing target", func(t *testing.T) {
		tuple := typ.NewTuple(typ.String, target)
		if !CanContain(tuple, target) {
			t.Error("expected true for tuple containing target")
		}
	})

	t.Run("record field containing target", func(t *testing.T) {
		rec := typ.NewRecord().Field("data", target).Build()
		if !CanContain(rec, target) {
			t.Error("expected true for record containing target")
		}
	})

	t.Run("ref type is conservative", func(t *testing.T) {
		ref := typ.NewRef("mod", "Type")
		if !CanContain(ref, target) {
			t.Error("expected true for ref (conservative)")
		}
	})

	t.Run("alias containing target", func(t *testing.T) {
		alias := typ.NewAlias("T", target)
		if !CanContain(alias, target) {
			t.Error("expected true for alias containing target")
		}
	})

	t.Run("any type is conservative", func(t *testing.T) {
		if !CanContain(typ.Any, target) {
			t.Error("expected true for any type")
		}
	})

	t.Run("unknown type is conservative", func(t *testing.T) {
		if !CanContain(typ.Unknown, target) {
			t.Error("expected true for unknown type")
		}
	})

	t.Run("literal does not contain", func(t *testing.T) {
		if CanContain(typ.LiteralInt(5), target) {
			t.Error("expected false for literal")
		}
	})
}

func TestFindReachableFields(t *testing.T) {
	target := typ.NewRecord().
		Field("a", typ.String).
		Field("b", typ.Integer).
		Build()

	t.Run("nil type", func(t *testing.T) {
		result := findReachableFields(nil, target, make(map[typ.Type]bool))
		if len(result) != 0 {
			t.Error("expected empty result")
		}
	})

	t.Run("primitive type", func(t *testing.T) {
		result := findReachableFields(typ.String, target, make(map[typ.Type]bool))
		if len(result) != 0 {
			t.Error("expected empty result for primitive")
		}
	})

	t.Run("same record", func(t *testing.T) {
		result := findReachableFields(target, target, make(map[typ.Type]bool))
		if len(result) != 2 {
			t.Errorf("expected 2 fields, got %d", len(result))
		}
	})

	t.Run("array of target", func(t *testing.T) {
		arr := typ.NewArray(target)

		result := findReachableFields(arr, target, make(map[typ.Type]bool))
		if len(result) != 2 {
			t.Errorf("expected 2 fields from array element, got %d", len(result))
		}
	})

	t.Run("optional of target", func(t *testing.T) {
		opt := typ.NewOptional(target)

		result := findReachableFields(opt, target, make(map[typ.Type]bool))
		if len(result) != 2 {
			t.Errorf("expected 2 fields from optional inner, got %d", len(result))
		}
	})

	t.Run("union with target", func(t *testing.T) {
		union := typ.NewUnion(typ.String, target)

		result := findReachableFields(union, target, make(map[typ.Type]bool))
		if len(result) != 2 {
			t.Errorf("expected 2 fields from union, got %d", len(result))
		}
	})

	t.Run("intersection with target", func(t *testing.T) {
		inter := typ.NewIntersection(typ.NewRecord().Build(), target)

		result := findReachableFields(inter, target, make(map[typ.Type]bool))
		if len(result) != 2 {
			t.Errorf("expected 2 fields from intersection, got %d", len(result))
		}
	})

	t.Run("ref type", func(t *testing.T) {
		ref := typ.NewRef("mod", "Type")

		result := findReachableFields(ref, target, make(map[typ.Type]bool))
		if len(result) != 0 {
			t.Error("expected empty for ref")
		}
	})

	t.Run("alias to target", func(t *testing.T) {
		alias := typ.NewAlias("T", target)

		result := findReachableFields(alias, target, make(map[typ.Type]bool))
		if len(result) != 2 {
			t.Errorf("expected 2 fields from alias, got %d", len(result))
		}
	})

	t.Run("visited cycle prevention", func(t *testing.T) {
		visited := make(map[typ.Type]bool)
		visited[target] = true

		result := findReachableFields(target, target, visited)
		if len(result) != 0 {
			t.Error("expected empty when already visited")
		}
	})
}

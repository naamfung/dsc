package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestTraverseUnion(t *testing.T) {
	t.Run("non-union type", func(t *testing.T) {
		result := TraverseUnion(typ.String, func(_ typ.Type) typ.Type {
			return typ.Integer
		})
		if result != typ.Integer {
			t.Errorf("expected Integer, got %v", result)
		}
	})

	t.Run("union type returns first non-nil", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer, typ.Boolean)
		count := 0

		result := TraverseUnion(union, func(t typ.Type) typ.Type {
			count++

			if t == typ.Integer {
				return typ.Number
			}

			return nil
		})
		if result != typ.Number {
			t.Errorf("expected Number, got %v", result)
		}
	})

	t.Run("nil type", func(t *testing.T) {
		result := TraverseUnion[typ.Type](nil, func(_ typ.Type) typ.Type {
			return typ.String
		})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("all return nil", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer)

		result := TraverseUnion(union, func(t typ.Type) typ.Type {
			return nil
		})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}

func TestTraverseIntersection(t *testing.T) {
	rec1 := typ.NewRecord().Field("a", typ.String).Build()
	rec2 := typ.NewRecord().Field("b", typ.Integer).Build()
	inter := typ.NewIntersection(rec1, rec2)

	t.Run("non-intersection type", func(t *testing.T) {
		result := TraverseIntersection(typ.String, func(t typ.Type) typ.Type {
			return typ.Integer
		})
		if result != typ.Integer {
			t.Errorf("expected Integer, got %v", result)
		}
	})

	t.Run("intersection type returns first non-nil", func(t *testing.T) {
		result := TraverseIntersection(inter, func(t typ.Type) typ.Type {
			if t == rec2 {
				return typ.Number
			}

			return nil
		})
		if result != typ.Number {
			t.Errorf("expected Number, got %v", result)
		}
	})

	t.Run("nil type", func(t *testing.T) {
		result := TraverseIntersection[typ.Type](nil, func(t typ.Type) typ.Type {
			return typ.String
		})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}

func TestTraverseOptional(t *testing.T) {
	t.Run("non-optional type", func(t *testing.T) {
		result := TraverseOptional(typ.String, func(t typ.Type) typ.Type {
			return typ.Integer
		})
		if result != typ.Integer {
			t.Errorf("expected Integer, got %v", result)
		}
	})

	t.Run("optional type", func(t *testing.T) {
		opt := typ.NewOptional(typ.String)

		result := TraverseOptional(opt, func(t typ.Type) typ.Type {
			if t == typ.String {
				return typ.Number
			}

			return nil
		})
		if result != typ.Number {
			t.Errorf("expected Number, got %v", result)
		}
	})

	t.Run("nested optional", func(t *testing.T) {
		innerOpt := typ.NewOptional(typ.Integer)
		// NewOptional flattens, so create manually
		opt := &typ.Optional{Inner: innerOpt}

		result := TraverseOptional(opt, func(t typ.Type) typ.Type {
			if t == typ.Integer {
				return typ.Boolean
			}

			return nil
		})
		if result != typ.Boolean {
			t.Errorf("expected Boolean, got %v", result)
		}
	})

	t.Run("nil type", func(t *testing.T) {
		result := TraverseOptional[typ.Type](nil, func(t typ.Type) typ.Type {
			return typ.String
		})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}

func TestForEachMember(t *testing.T) {
	t.Run("single type", func(t *testing.T) {
		var visited []typ.Type

		result := ForEachMember(typ.String, func(t typ.Type) bool {
			visited = append(visited, t)
			return true
		})
		if !result {
			t.Error("expected true return")
		}

		if len(visited) != 1 || visited[0] != typ.String {
			t.Errorf("expected [String], got %v", visited)
		}
	})

	t.Run("union type", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer, typ.Boolean)

		var visited []typ.Type

		result := ForEachMember(union, func(t typ.Type) bool {
			visited = append(visited, t)
			return true
		})
		if !result {
			t.Error("expected true return")
		}

		if len(visited) != 3 {
			t.Errorf("expected 3 members, got %d", len(visited))
		}
	})

	t.Run("union type with early return", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer, typ.Boolean)
		count := 0

		result := ForEachMember(union, func(t typ.Type) bool {
			count++
			return count < 2
		})
		if result {
			t.Error("expected false return")
		}

		if count != 2 {
			t.Errorf("expected 2 visits, got %d", count)
		}
	})

	t.Run("intersection type", func(t *testing.T) {
		rec1 := typ.NewRecord().Build()
		rec2 := typ.NewRecord().Field("x", typ.Number).Build()
		inter := typ.NewIntersection(rec1, rec2)

		var visited []typ.Type

		result := ForEachMember(inter, func(t typ.Type) bool {
			visited = append(visited, t)
			return true
		})
		if !result {
			t.Error("expected true return")
		}

		if len(visited) != 2 {
			t.Errorf("expected 2 members, got %d", len(visited))
		}
	})

	t.Run("nil type", func(t *testing.T) {
		result := ForEachMember(nil, func(t typ.Type) bool {
			return false
		})
		if !result {
			t.Error("expected true for nil")
		}
	})
}

func TestAllMembers(t *testing.T) {
	t.Run("single type", func(t *testing.T) {
		result := AllMembers(typ.String)
		if len(result) != 1 || result[0] != typ.String {
			t.Errorf("expected [String], got %v", result)
		}
	})

	t.Run("union type", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer, typ.Boolean)

		result := AllMembers(union)
		if len(result) != 3 {
			t.Errorf("expected 3 members, got %d", len(result))
		}
	})

	t.Run("intersection type", func(t *testing.T) {
		rec1 := typ.NewRecord().Build()
		rec2 := typ.NewRecord().Field("x", typ.Number).Build()
		inter := typ.NewIntersection(rec1, rec2)

		result := AllMembers(inter)
		if len(result) != 2 {
			t.Errorf("expected 2 members, got %d", len(result))
		}
	})

	t.Run("nested union", func(t *testing.T) {
		// Nested unions are flattened by NewUnion, but test behavior
		union := typ.NewUnion(typ.String, typ.Integer)
		outer := typ.NewUnion(union, typ.Boolean)

		result := AllMembers(outer)
		if len(result) != 3 {
			t.Errorf("expected 3 members (flattened), got %d", len(result))
		}
	})

	t.Run("nil type", func(t *testing.T) {
		result := AllMembers(nil)
		if len(result) != 0 {
			t.Errorf("expected empty, got %v", result)
		}
	})
}

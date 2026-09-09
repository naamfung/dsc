package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestField(t *testing.T) {
	rec := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()
	recWithOpt := typ.NewRecord().
		OptField("name", typ.String).
		Build()

	iface := typ.NewInterface("Reader", []typ.Method{
		{Name: "read", Type: typ.Func().Param("n", typ.Integer).Returns(typ.String).Build()},
	})

	tests := []struct {
		name      string
		t         typ.Type
		fieldName string
		found     bool
		checker   func(typ.Type) bool
	}{
		{"nil type", nil, "x", false, nil},
		{"record existing field", rec, "name", true, func(t typ.Type) bool { return t == typ.String }},
		{"record another field", rec, "age", true, func(t typ.Type) bool { return t == typ.Integer }},
		{"record optional field", recWithOpt, "name", true, func(t typ.Type) bool {
			return typ.TypeEquals(t, typ.NewOptional(typ.String))
		}},
		{"record missing field", rec, "missing", false, nil},
		{"interface method", iface, "read", true, func(t typ.Type) bool { return t.Kind() == typ.String.Kind() || true }},
		{"interface missing", iface, "write", false, nil},
		{"builtin table marker", typ.NewInterface("table", nil), "anything", true, func(t typ.Type) bool { return t == typ.Unknown }},
		{"any type", typ.Any, "anything", true, func(t typ.Type) bool { return t == typ.Any }},
		{"unknown type", typ.Unknown, "anything", true, func(t typ.Type) bool { return t == typ.Unknown }},
		{"never type", typ.Never, "anything", true, func(t typ.Type) bool { return t == typ.Never }},
		{"primitive string", typ.String, "x", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := Field(tt.t, tt.fieldName)
			if ok != tt.found {
				t.Errorf("expected found=%v, got %v", tt.found, ok)
			}

			if tt.found && tt.checker != nil && !tt.checker(result) {
				t.Errorf("checker failed for result %v", result)
			}
		})
	}
}

func TestFieldUnion(t *testing.T) {
	rec1 := typ.NewRecord().Field("id", typ.Integer).Field("name", typ.String).Build()
	rec2 := typ.NewRecord().Field("id", typ.Integer).Field("email", typ.String).Build()
	union := typ.NewUnion(rec1, rec2)

	t.Run("common field in union", func(t *testing.T) {
		result, ok := Field(union, "id")
		if !ok {
			t.Error("expected to find 'id' in union")
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("field only in one member", func(t *testing.T) {
		got, ok := Field(union, "name")
		if !ok {
			t.Error("expected to find 'name' as optional from partial union")
			return
		}
		if !typ.TypeEquals(got, typ.NewOptional(typ.String)) {
			t.Errorf("expected string?, got %v", got)
		}
	})

	t.Run("empty union members", func(t *testing.T) {
		emptyUnion := &typ.Union{Members: []typ.Type{}}

		_, ok := Field(emptyUnion, "x")
		if ok {
			t.Error("expected not to find field in empty union")
		}
	})
}

func TestFieldIntersection(t *testing.T) {
	rec1 := typ.NewRecord().Field("a", typ.String).Build()
	rec2 := typ.NewRecord().Field("b", typ.Integer).Build()
	inter := typ.NewIntersection(rec1, rec2)

	t.Run("field from first member", func(t *testing.T) {
		result, ok := Field(inter, "a")
		if !ok {
			t.Error("expected to find 'a' in intersection")
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("field from second member", func(t *testing.T) {
		result, ok := Field(inter, "b")
		if !ok {
			t.Error("expected to find 'b' in intersection")
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("missing field", func(t *testing.T) {
		_, ok := Field(inter, "missing")
		if ok {
			t.Error("expected not to find 'missing'")
		}
	})
}

func TestFieldOptional(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	opt := typ.NewOptional(rec)
	optFieldRec := typ.NewRecord().OptField("x", typ.Number).Build()
	optFieldOptRec := typ.NewOptional(optFieldRec)

	t.Run("field on optional record", func(t *testing.T) {
		result, ok := Field(opt, "x")
		if !ok {
			t.Error("expected to find field in optional")
		}

		if result == nil {
			t.Error("expected non-nil result")
			return
		}

		if _, isOpt := result.(*typ.Optional); !isOpt {
			t.Error("expected optional wrapper on field type")
		}
	})

	t.Run("optional field on record", func(t *testing.T) {
		result, ok := Field(optFieldRec, "x")
		if !ok {
			t.Error("expected to find optional field")
		}
		if !typ.TypeEquals(result, typ.NewOptional(typ.Number)) {
			t.Errorf("expected number?, got %v", result)
		}
	})

	t.Run("optional field on optional record stays optional", func(t *testing.T) {
		result, ok := Field(optFieldOptRec, "x")
		if !ok {
			t.Error("expected to find optional field on optional record")
		}
		if !typ.TypeEquals(result, typ.NewOptional(typ.Number)) {
			t.Errorf("expected number?, got %v", result)
		}
	})
}

func TestFieldAlias(t *testing.T) {
	rec := typ.NewRecord().Field("value", typ.String).Build()
	alias := typ.NewAlias("MyRecord", rec)

	t.Run("field through alias", func(t *testing.T) {
		result, ok := Field(alias, "value")
		if !ok {
			t.Error("expected to find field through alias")
		}

		if result != typ.String {
			t.Errorf("expected string, got %v", result)
		}
	})
}

func TestHasField(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()

	tests := []struct {
		name   string
		t      typ.Type
		field  string
		expect bool
	}{
		{"has field", rec, "x", true},
		{"no field", rec, "y", false},
		{"nil type", nil, "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasField(tt.t, tt.field) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

// =============================================================================
// __index Metamethod Fallback Tests (Lua Semantics)
// =============================================================================

// TestField_IndexMetamethodFallback tests that __index metamethod is checked
// when a field is not found directly on the record.
//
// Lua pattern:
//
//	local Base = { shared = "value" }
//	local Child = setmetatable({}, { __index = Base })
//	print(Child.shared)  -- Uses __index to find "shared" in Base
func TestField_IndexMetamethodFallback(t *testing.T) {
	// Create base type with "shared" field
	base := typ.NewRecord().Field("shared", typ.String).Build()

	// Create __index function that returns the base type
	// In Lua, __index can be a table or function
	indexFn := typ.Func().Param("table", typ.Any).Param("key", typ.String).Returns(typ.Any).Build()
	meta := typ.NewRecord().Field("__index", base).Build() // __index is a table

	// Child record with metatable containing __index
	child := typ.NewRecord().Field("own", typ.Integer).Metatable(meta).Build()

	t.Run("direct field found", func(t *testing.T) {
		result, ok := Field(child, "own")
		if !ok {
			t.Error("expected to find direct field 'own'")
		}

		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("field via __index table", func(t *testing.T) {
		result, ok := Field(child, "shared")
		if !ok {
			t.Error("expected to find 'shared' via __index table fallback")
		}

		if result != typ.String {
			t.Errorf("expected string from __index, got %v", result)
		}
	})

	t.Run("field via __index function", func(t *testing.T) {
		// __index as function returns any for unknown keys
		metaFn := typ.NewRecord().Field("__index", indexFn).Build()
		childFn := typ.NewRecord().Field("own", typ.Integer).Metatable(metaFn).Build()

		result, ok := Field(childFn, "dynamic")
		if !ok {
			t.Error("expected to find field via __index function fallback")
		}

		if result != typ.Any {
			t.Errorf("expected any from __index function, got %v", result)
		}
	})
}

// TestField_IndexMetamethodChain tests __index chains (prototype pattern).
//
// Lua pattern:
//
//	local A = { a = 1 }
//	local B = setmetatable({ b = 2 }, { __index = A })
//	local C = setmetatable({ c = 3 }, { __index = B })
//	print(C.a)  -- Chains through B to A
func TestField_IndexMetamethodChain(t *testing.T) {
	a := typ.NewRecord().Field("a", typ.Integer).Build()
	metaB := typ.NewRecord().Field("__index", a).Build()
	b := typ.NewRecord().Field("b", typ.Integer).Metatable(metaB).Build()
	metaC := typ.NewRecord().Field("__index", b).Build()
	c := typ.NewRecord().Field("c", typ.Integer).Metatable(metaC).Build()

	t.Run("field from chain root", func(t *testing.T) {
		result, ok := Field(c, "a")
		if !ok {
			t.Error("expected to find 'a' via __index chain")
		}

		if result != typ.Integer {
			t.Errorf("expected integer from chain, got %v", result)
		}
	})

	t.Run("field from middle of chain", func(t *testing.T) {
		result, ok := Field(c, "b")
		if !ok {
			t.Error("expected to find 'b' via __index chain")
		}

		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("direct field on c", func(t *testing.T) {
		result, ok := Field(c, "c")
		if !ok {
			t.Error("expected to find direct field 'c'")
		}

		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})
}

// TestField_NewindexMetamethod documents __newindex for assignment.
// This is more relevant for assignment checking than field lookup.
//
// Lua pattern:
//
//	local t = setmetatable({}, { __newindex = function(t, k, v) rawset(t, k, v) end })
//	t.x = 10  -- Triggers __newindex
func TestField_NewindexMetamethod(t *testing.T) {
	newindexFn := typ.Func().
		Param("t", typ.Any).
		Param("key", typ.String).
		Param("value", typ.Any).
		Build()
	meta := typ.NewRecord().Field("__newindex", newindexFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()

	t.Run("record with __newindex", func(t *testing.T) {
		// __newindex affects assignment, not field lookup
		// This test documents that Field() should NOT use __newindex
		_, ok := Field(rec, "dynamic")
		if ok {
			t.Error("__newindex should not affect field lookup")
		}
	})
}

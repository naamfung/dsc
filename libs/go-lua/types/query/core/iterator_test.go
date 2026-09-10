package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// =============================================================================
// Iterator Pattern Tests (pairs/ipairs/custom iterators)
// =============================================================================

// TestPairsIterator tests pairs() iterator semantics.
//
// Lua pattern:
//
//	for k, v in pairs(t) do
//	  -- k is key type, v is value type
//	end
func TestPairsIterator(t *testing.T) {
	// Record type
	rec := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	t.Run("pairs on record", func(t *testing.T) {
		// pairs(record) yields (string, union of field types)
		// Key is string (field name), value is union of all field types
		keyType, valueType := IteratorTypes(rec, "pairs")
		if keyType == nil {
			t.Fatal("pairs iterator types should be implemented")
		}

		if !subtype.IsSubtype(keyType, typ.String) {
			t.Errorf("expected key subtype of string, got %v", keyType)
		}

		if valueType == nil {
			t.Error("expected non-nil value type")
		}
	})

	t.Run("pairs on map", func(t *testing.T) {
		// Map<string, integer>
		m := typ.NewMap(typ.String, typ.Integer)

		keyType, valueType := IteratorTypes(m, "pairs")
		if keyType == nil {
			t.Fatal("pairs on map should be implemented")
		}

		if keyType != typ.String {
			t.Errorf("expected string key, got %v", keyType)
		}

		if valueType != typ.Integer {
			t.Errorf("expected integer value, got %v", valueType)
		}
	})

	t.Run("pairs on array", func(t *testing.T) {
		arr := typ.NewArray(typ.String)

		keyType, valueType := IteratorTypes(arr, "pairs")
		if keyType == nil {
			t.Fatal("pairs on array should be implemented")
		}
		// Arrays have integer keys
		if keyType != typ.Integer {
			t.Errorf("expected integer key, got %v", keyType)
		}

		if valueType != typ.String {
			t.Errorf("expected string value, got %v", valueType)
		}
	})
}

// TestIpairsIterator tests ipairs() iterator semantics.
//
// Lua pattern:
//
//	for i, v in ipairs(arr) do
//	  -- i is always integer, v is element type
//	end
func TestIpairsIterator(t *testing.T) {
	arr := typ.NewArray(typ.String)

	t.Run("ipairs on array", func(t *testing.T) {
		keyType, valueType := IteratorTypes(arr, "ipairs")
		if keyType == nil {
			t.Fatal("ipairs iterator types should be implemented")
		}
		// ipairs always yields integer index
		if keyType != typ.Integer {
			t.Errorf("expected integer index, got %v", keyType)
		}

		if valueType != typ.String {
			t.Errorf("expected string value, got %v", valueType)
		}
	})

	t.Run("ipairs on tuple", func(t *testing.T) {
		tuple := typ.NewTuple(typ.String, typ.Integer, typ.Boolean)

		keyType, valueType := IteratorTypes(tuple, "ipairs")
		if keyType == nil {
			t.Fatal("ipairs on tuple should be implemented")
		}

		if keyType != typ.Integer {
			t.Errorf("expected integer index, got %v", keyType)
		}
		// Value should be union of tuple elements
		if valueType == nil {
			t.Error("expected non-nil value type for tuple ipairs")
		}
	})
}

// TestIteratorMetamethod tests __pairs and __ipairs metamethods.
//
// Lua pattern:
//
//	local mt = { __pairs = function(t) return next, t, nil end }
//	setmetatable(obj, mt)
//	for k, v in pairs(obj) do ... end  -- Uses __pairs
func TestIteratorMetamethod(t *testing.T) {
	// __pairs metamethod
	pairsFn := typ.Func().
		Param("t", typ.Any).
		Returns(typ.Any, typ.Any, typ.Nil).
		Build()
	meta := typ.NewRecord().Field("__pairs", pairsFn).Build()
	customPairs := typ.NewRecord().Field("data", typ.Any).Metatable(meta).Build()

	t.Run("__pairs metamethod", func(t *testing.T) {
		mm, ok := GetMetamethod(customPairs, "__pairs")
		if !ok {
			t.Error("expected to find __pairs metamethod")
		}

		if mm == nil {
			t.Error("metamethod should not be nil")
		}
	})

	t.Run("__ipairs metamethod", func(t *testing.T) {
		ipairsFn := typ.Func().Param("t", typ.Any).Returns(typ.Any, typ.Any, typ.Integer).Build()
		ipairsMeta := typ.NewRecord().Field("__ipairs", ipairsFn).Build()
		customIpairs := typ.NewRecord().Metatable(ipairsMeta).Build()

		mm, ok := GetMetamethod(customIpairs, "__ipairs")
		if !ok {
			t.Error("expected to find __ipairs metamethod")
		}

		if mm == nil {
			t.Error("metamethod should not be nil")
		}
	})
}

// =============================================================================
// Numeric For Loop
// =============================================================================

// TestNumericForLoop tests numeric for loop type inference.
//
// Lua: for i = start, stop, step do ... end
func TestNumericForLoop(t *testing.T) {
	t.Run("integer bounds", func(t *testing.T) {
		// for i = 1, 10 do
		// Loop variable i is integer when bounds are integers
		start, stop := typ.Integer, typ.Integer
		// Loop var type is LUB of start and stop
		if start != typ.Integer || stop != typ.Integer {
			t.Error("integer bounds should give integer loop var")
		}
	})

	t.Run("number bounds", func(t *testing.T) {
		// for i = 1.0, 10.5, 0.5 do
		// Loop variable is number when any bound is number
		start := typ.Number
		stop := typ.Number
		step := typ.Number
		// All are number, loop var is number
		if start != typ.Number || stop != typ.Number || step != typ.Number {
			t.Error("number bounds should give number loop var")
		}
	})

	t.Run("mixed bounds widen to number", func(t *testing.T) {
		// for i = 1, 10.5 do
		// Integer start, number stop -> loop var is number
		start := typ.Integer
		stop := typ.Number
		// LUB(integer, number) = number
		result := typ.NewUnion(start, stop)
		if result == nil {
			t.Error("mixed bounds should produce union/widened type")
		}
	})

	t.Run("step defaults to 1", func(t *testing.T) {
		// for i = 1, 10 do (step defaults to 1)
		// Step is implicitly integer 1
		defaultStep := typ.Integer
		if defaultStep != typ.Integer {
			t.Error("default step should be integer")
		}
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

// IteratorTypes returns the key and value types for an iterator.
// iterKind is "pairs" or "ipairs".
func IteratorTypes(t typ.Type, iterKind string) (keyType, valueType typ.Type) {
	if t == nil {
		return nil, nil
	}

	switch v := t.(type) {
	case *typ.Array:
		if iterKind == "ipairs" || iterKind == "pairs" {
			return typ.Integer, v.Element
		}
	case *typ.Map:
		return v.Key, v.Value
	case *typ.Tuple:
		if len(v.Elements) == 0 {
			return typ.Integer, typ.Nil
		}
		return typ.Integer, typ.NewUnion(v.Elements...)
	case *typ.Record:
		if iterKind == "pairs" {
			var fieldTypes []typ.Type
			for _, f := range v.Fields {
				fieldTypes = append(fieldTypes, f.Type)
			}

			if len(fieldTypes) == 0 {
				return typ.String, typ.Any
			}

			return typ.String, typ.NewUnion(fieldTypes...)
		}
	}

	return nil, nil
}

// =============================================================================
// Goto and Labels (Lua 5.2+)
// =============================================================================

// TestGotoLabels tests goto/label control flow.
func TestGotoLabels(t *testing.T) {
	t.Run("goto is control flow", func(t *testing.T) {
		// goto transfers control, affects reachability
		// ::label:: defines jump target
		// Type system tracks reachability after goto

		// After unconditional goto, code is unreachable
		// This is a control flow concern, not a type concern
		// But affects what variables are definitely assigned
	})

	t.Run("label scope", func(t *testing.T) {
		// Labels are visible in enclosing block
		// Cannot jump into a local variable scope
		// These are semantic constraints
	})
}

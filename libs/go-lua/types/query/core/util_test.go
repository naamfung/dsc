package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeNames(t *testing.T) {
	tests := []struct {
		name   string
		input  []typ.Type
		expect []string
	}{
		{"nil slice", nil, nil},
		{"empty slice", []typ.Type{}, []string{}},
		{"single type", []typ.Type{typ.String}, []string{"string"}},
		{"multiple types", []typ.Type{typ.String, typ.Integer, typ.Boolean}, []string{"string", "integer", "boolean"}},
		{"with nil element", []typ.Type{typ.String, nil, typ.Integer}, []string{"string", "<nil>", "integer"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TypeNames(tt.input)
			if !stringSlicesEqual(result, tt.expect) {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestAllFields(t *testing.T) {
	rec := typ.NewRecord().
		Field("a", typ.String).
		Field("b", typ.Integer).
		Build()

	iface := typ.NewInterface("Iface", []typ.Method{
		{Name: "x", Type: typ.Func().Build()},
		{Name: "y", Type: typ.Func().Build()},
	})

	tests := []struct {
		name     string
		input    typ.Type
		expected int
	}{
		{"nil type", nil, 0},
		{"empty record", typ.NewRecord().Build(), 0},
		{"record with fields", rec, 2},
		{"interface", iface, 2},
		{"primitive", typ.String, 0},
		{"alias to record", typ.NewAlias("R", rec), 2},
		{"optional record", typ.NewOptional(rec), 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AllFields(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d fields, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestAllFieldsUnion(t *testing.T) {
	recA := typ.NewRecord().
		Field("a", typ.String).
		Field("b", typ.Integer).
		Build()
	recB := typ.NewRecord().
		Field("a", typ.Number).
		Field("c", typ.Boolean).
		Build()

	t.Run("common fields only", func(t *testing.T) {
		union := typ.NewUnion(recA, recB)
		result := AllFields(union)
		if len(result) != 1 {
			t.Fatalf("expected 1 common field, got %d: %v", len(result), result)
		}
		if result[0] != "a" {
			t.Errorf("expected field a, got %s", result[0])
		}
	})

	t.Run("member with no fields", func(t *testing.T) {
		union := typ.NewUnion(recA, typ.String)
		result := AllFields(union)
		if len(result) != 0 {
			t.Errorf("expected 0 fields, got %d", len(result))
		}
	})

	t.Run("empty union", func(t *testing.T) {
		empty := &typ.Union{Members: []typ.Type{}}
		result := AllFields(empty)
		if len(result) != 0 {
			t.Errorf("expected 0 fields, got %d", len(result))
		}
	})
}

func TestAllFieldsIntersection(t *testing.T) {
	recA := typ.NewRecord().
		Field("a", typ.String).
		Build()
	recB := typ.NewRecord().
		Field("b", typ.Integer).
		Build()

	t.Run("all fields from all members", func(t *testing.T) {
		inter := typ.NewIntersection(recA, recB)
		result := AllFields(inter)
		if len(result) != 2 {
			t.Fatalf("expected 2 fields, got %d: %v", len(result), result)
		}
		seen := map[string]bool{}
		for _, n := range result {
			seen[n] = true
		}
		if !seen["a"] || !seen["b"] {
			t.Errorf("expected fields a and b, got %v", result)
		}
	})

	t.Run("empty intersection", func(t *testing.T) {
		empty := &typ.Intersection{Members: []typ.Type{}}
		result := AllFields(empty)
		if len(result) != 0 {
			t.Errorf("expected 0 fields, got %d", len(result))
		}
	})
}

func TestAllFieldTypes(t *testing.T) {
	rec := typ.NewRecord().
		Field("a", typ.String).
		Field("b", typ.Integer).
		Build()

	iface := typ.NewInterface("Iface", []typ.Method{
		{Name: "x", Type: typ.Func().Build()},
		{Name: "y", Type: typ.Func().Build()},
	})

	recA := typ.NewRecord().
		Field("a", typ.String).
		Field("c", typ.Boolean).
		Build()
	recB := typ.NewRecord().
		Field("a", typ.Integer).
		Field("d", typ.Number).
		Build()

	t.Run("nil type", func(t *testing.T) {
		if result := AllFieldTypes(nil); result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("empty record", func(t *testing.T) {
		if result := AllFieldTypes(typ.NewRecord().Build()); result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("record with fields", func(t *testing.T) {
		result := AllFieldTypes(rec)
		if len(result) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(result))
		}
		if result["a"] != typ.String {
			t.Errorf("expected field a to be string, got %v", result["a"])
		}
		if result["b"] != typ.Integer {
			t.Errorf("expected field b to be integer, got %v", result["b"])
		}
	})

	t.Run("interface", func(t *testing.T) {
		result := AllFieldTypes(iface)
		if len(result) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(result))
		}
		if result["x"] == nil {
			t.Error("expected field x to be non-nil")
		}
		if result["y"] == nil {
			t.Error("expected field y to be non-nil")
		}
	})

	t.Run("alias", func(t *testing.T) {
		result := AllFieldTypes(typ.NewAlias("R", rec))
		if len(result) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(result))
		}
	})

	t.Run("optional", func(t *testing.T) {
		result := AllFieldTypes(typ.NewOptional(rec))
		if len(result) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(result))
		}
	})

	t.Run("union common fields only", func(t *testing.T) {
		union := typ.NewUnion(recA, recB)
		result := AllFieldTypes(union)
		if len(result) != 1 {
			t.Fatalf("expected 1 common field, got %d", len(result))
		}
		if result["a"] == nil {
			t.Fatal("expected field a to be present")
		}
		u, ok := result["a"].(*typ.Union)
		if !ok {
			t.Fatalf("expected union type for field a, got %T", result["a"])
		}
		if len(u.Members) != 2 {
			t.Errorf("expected 2 union members, got %d", len(u.Members))
		}
	})

	t.Run("intersection all fields", func(t *testing.T) {
		inter := typ.NewIntersection(recA, recB)
		result := AllFieldTypes(inter)
		if len(result) != 3 {
			t.Fatalf("expected 3 fields, got %d", len(result))
		}
		if result["c"] != typ.Boolean {
			t.Errorf("expected field c to be boolean, got %v", result["c"])
		}
		if result["d"] != typ.Number {
			t.Errorf("expected field d to be number, got %v", result["d"])
		}
		// field a appears in both members, should be an intersection type
		if _, ok := result["a"].(*typ.Intersection); !ok {
			t.Errorf("expected intersection type for field a, got %T", result["a"])
		}
	})

	t.Run("primitive", func(t *testing.T) {
		if result := AllFieldTypes(typ.String); result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}

func TestAllFieldTypesResolved(t *testing.T) {
	t.Run("record fields unchanged", func(t *testing.T) {
		rec := typ.NewRecord().
			Field("x", typ.String).
			Build()
		result := AllFieldTypesResolved(rec)
		if len(result) != 1 {
			t.Fatalf("expected 1 field, got %d", len(result))
		}
		if result["x"] != typ.String {
			t.Errorf("expected string, got %v", result["x"])
		}
	})

	t.Run("interface substitutes Self", func(t *testing.T) {
		// Interface method takes Self parameter
		selfParam := typ.Func().Param("other", typ.Self).Returns(typ.Boolean).Build()
		iface := typ.NewInterface("Eq", []typ.Method{
			{Name: "equals", Type: selfParam},
		})

		result := AllFieldTypesResolved(iface)
		if len(result) != 1 {
			t.Fatalf("expected 1 field, got %d", len(result))
		}
		fn, ok := result["equals"].(*typ.Function)
		if !ok {
			t.Fatalf("expected function type, got %T", result["equals"])
		}
		// Self should be replaced with the interface type
		if len(fn.Params) == 0 {
			t.Fatal("expected at least one param")
		}
		if fn.Params[0].Type == typ.Self {
			t.Error("Self was not substituted")
		}
	})

	t.Run("nil type", func(t *testing.T) {
		result := AllFieldTypesResolved(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("alias to interface", func(t *testing.T) {
		selfParam := typ.Func().Param("other", typ.Self).Returns(typ.Boolean).Build()
		iface := typ.NewInterface("Eq", []typ.Method{
			{Name: "equals", Type: selfParam},
		})
		alias := typ.NewAlias("MyEq", iface)

		result := AllFieldTypesResolved(alias)
		if len(result) != 1 {
			t.Fatalf("expected 1 field, got %d", len(result))
		}
		fn, ok := result["equals"].(*typ.Function)
		if !ok {
			t.Fatalf("expected function type, got %T", result["equals"])
		}
		if len(fn.Params) == 0 {
			t.Fatal("expected at least one param")
		}
		if fn.Params[0].Type == typ.Self {
			t.Error("Self was not substituted in aliased interface")
		}
	})
}

func TestAllMethods(t *testing.T) {
	methodFn := typ.Func().Build()
	meta := typ.NewRecord().Field("m1", methodFn).Field("m2", methodFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()
	recNoMeta := typ.NewRecord().Build()

	iface := typ.NewInterface("Iface", []typ.Method{
		{Name: "foo", Type: typ.Func().Build()},
	})

	tests := []struct {
		name     string
		input    typ.Type
		expected int
	}{
		{"nil type", nil, 0},
		{"record with metatable", rec, 2},
		{"record without metatable", recNoMeta, 0},
		{"interface", iface, 1},
		{"alias to record", typ.NewAlias("R", rec), 2},
		{"optional record", typ.NewOptional(rec), 2},
		{"primitive", typ.String, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AllMethods(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d methods, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestLength(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect int
	}{
		{"nil type", nil, -1},
		{"tuple", typ.NewTuple(typ.String, typ.Integer), 2},
		{"empty tuple", typ.NewTuple(), 0},
		{"literal string", typ.LiteralString("hello"), 5},
		{"empty literal string", typ.LiteralString(""), 0},
		{"literal int", typ.LiteralInt(42), -1},
		{"array", typ.NewArray(typ.String), -1},
		{"alias to tuple", typ.NewAlias("T", typ.NewTuple(typ.String)), 1},
		{"primitive string", typ.String, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Length(tt.input)
			if result != tt.expect {
				t.Errorf("expected %d, got %d", tt.expect, result)
			}
		})
	}
}

func TestIterable(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect bool
	}{
		{"nil type", nil, false},
		{"array", typ.NewArray(typ.String), true},
		{"map", typ.NewMap(typ.String, typ.Integer), true},
		{"tuple", typ.NewTuple(typ.String), true},
		{"record", typ.NewRecord().Build(), true},
		{"string", typ.String, true},
		{"any", typ.Any, true},
		{"integer", typ.Integer, false},
		{"boolean", typ.Boolean, false},
		{"function", typ.Func().Build(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Iterable(tt.input)
			if result != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestIterableUnion(t *testing.T) {
	t.Run("union of iterables", func(t *testing.T) {
		union := typ.NewUnion(typ.NewArray(typ.String), typ.NewMap(typ.String, typ.Integer), typ.NewRecord().Build())
		if !Iterable(union) {
			t.Error("expected union of iterables to be iterable")
		}
	})

	t.Run("union with non-iterable", func(t *testing.T) {
		union := typ.NewUnion(typ.NewArray(typ.String), typ.Integer)
		if Iterable(union) {
			t.Error("expected union with non-iterable to not be iterable")
		}
	})

	t.Run("empty union", func(t *testing.T) {
		emptyUnion := &typ.Union{Members: []typ.Type{}}
		if Iterable(emptyUnion) {
			t.Error("expected empty union to not be iterable")
		}
	})
}

func TestIterableIntersection(t *testing.T) {
	t.Run("intersection with iterable", func(t *testing.T) {
		inter := typ.NewIntersection(typ.NewArray(typ.String), typ.NewRecord().Build())
		if !Iterable(inter) {
			t.Error("expected intersection with iterable to be iterable")
		}
	})

	t.Run("intersection without iterable", func(t *testing.T) {
		inter := typ.NewIntersection(typ.Integer, typ.Boolean)
		if Iterable(inter) {
			t.Error("expected intersection without iterable to not be iterable")
		}
	})
}

func TestIterableOptionalAndAlias(t *testing.T) {
	t.Run("optional array", func(t *testing.T) {
		opt := typ.NewOptional(typ.NewArray(typ.String))
		if !Iterable(opt) {
			t.Error("expected optional array to be iterable")
		}
	})

	t.Run("alias to array", func(t *testing.T) {
		alias := typ.NewAlias("Arr", typ.NewArray(typ.String))
		if !Iterable(alias) {
			t.Error("expected alias to array to be iterable")
		}
	})
}

func TestComparable(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect bool
	}{
		{"nil type", nil, false},
		{"string", typ.String, true},
		{"integer", typ.Integer, true},
		{"record", typ.NewRecord().Build(), true},
		{"function", typ.Func().Build(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if Comparable(tt.input) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestOrdered(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect bool
	}{
		{"nil type", nil, false},
		{"number", typ.Number, true},
		{"integer", typ.Integer, true},
		{"string", typ.String, true},
		{"any", typ.Any, true},
		{"boolean", typ.Boolean, false},
		{"record", typ.NewRecord().Build(), false},
		{"literal int", typ.LiteralInt(5), true},
		{"literal string", typ.LiteralString("abc"), true},
		{"literal bool", typ.LiteralBool(true), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if Ordered(tt.input) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestOrderedUnion(t *testing.T) {
	t.Run("union of ordered types", func(t *testing.T) {
		union := typ.NewUnion(typ.Integer, typ.Number, typ.String)
		if !Ordered(union) {
			t.Error("expected union of ordered types to be ordered")
		}
	})

	t.Run("union with non-ordered", func(t *testing.T) {
		union := typ.NewUnion(typ.Integer, typ.Boolean)
		if Ordered(union) {
			t.Error("expected union with non-ordered to not be ordered")
		}
	})

	t.Run("empty union", func(t *testing.T) {
		emptyUnion := &typ.Union{Members: []typ.Type{}}
		if Ordered(emptyUnion) {
			t.Error("expected empty union to not be ordered")
		}
	})
}

func TestOrderedAlias(t *testing.T) {
	alias := typ.NewAlias("I", typ.Integer)
	if !Ordered(alias) {
		t.Error("expected alias to integer to be ordered")
	}
}

func TestContainsNil(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect bool
	}{
		{"nil input", nil, true},
		{"nil type", typ.Nil, true},
		{"any type", typ.Any, true},
		{"unknown type", typ.Unknown, true},
		{"optional", typ.NewOptional(typ.String), true},
		{"string", typ.String, false},
		{"integer", typ.Integer, false},
		{"record", typ.NewRecord().Build(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ContainsNil(tt.input) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestContainsNilUnion(t *testing.T) {
	t.Run("union with nil", func(t *testing.T) {
		union := typ.NewUnion(typ.Nil, typ.String, typ.Integer)
		if !ContainsNil(union) {
			t.Error("expected union with nil to contain nil")
		}
	})

	t.Run("union without nil", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer)
		if ContainsNil(union) {
			t.Error("expected union without nil to not contain nil")
		}
	})
}

func TestContainsNilAlias(t *testing.T) {
	alias := typ.NewAlias("Opt", typ.NewOptional(typ.String))
	if !ContainsNil(alias) {
		t.Error("expected alias to optional to contain nil")
	}
}

func TestElementType(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect typ.Type
	}{
		{"nil type", nil, nil},
		{"array", typ.NewArray(typ.String), typ.String},
		{"map", typ.NewMap(typ.String, typ.Integer), typ.Integer},
		{"tuple", typ.NewTuple(typ.String, typ.Integer), typ.NewUnion(typ.String, typ.Integer)},
		{"empty tuple", typ.NewTuple(), nil},
		{"optional array", typ.NewOptional(typ.NewArray(typ.Number)), typ.Number},
		{"alias to array", typ.NewAlias("Arr", typ.NewArray(typ.Boolean)), typ.Boolean},
		{"primitive", typ.String, nil},
		{"record", typ.NewRecord().Build(), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ElementType(tt.input)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if result == nil {
				t.Errorf("expected %v, got nil", tt.expect)
			} else if result.Hash() != tt.expect.Hash() {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestElementTypeUnion(t *testing.T) {
	union := typ.NewUnion(typ.NewArray(typ.String), typ.NewArray(typ.Integer))
	result := ElementType(union)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(u.Members))
	}
}

func TestKeyType(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect typ.Type
	}{
		{"nil type", nil, nil},
		{"map", typ.NewMap(typ.String, typ.Integer), typ.String},
		{"array", typ.NewArray(typ.String), typ.Integer},
		{"tuple", typ.NewTuple(typ.String, typ.Integer), typ.Integer},
		{"record", typ.NewRecord().Field("a", typ.String).Build(), typ.LiteralString("a")},
		{"optional map", typ.NewOptional(typ.NewMap(typ.Number, typ.Boolean)), typ.Number},
		{"alias to map", typ.NewAlias("M", typ.NewMap(typ.Boolean, typ.String)), typ.Boolean},
		{"primitive", typ.String, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := KeyType(tt.input)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if result == nil {
				t.Errorf("expected %v, got nil", tt.expect)
			} else if result.Hash() != tt.expect.Hash() {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestKeyTypeUnion(t *testing.T) {
	union := typ.NewUnion(typ.NewMap(typ.String, typ.Integer), typ.NewMap(typ.Number, typ.Boolean))
	result := KeyType(union)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(u.Members))
	}
}

func TestValueType(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect typ.Type
	}{
		{"nil type", nil, nil},
		{"map", typ.NewMap(typ.String, typ.Integer), typ.Integer},
		{"array", typ.NewArray(typ.String), typ.String},
		{"tuple", typ.NewTuple(typ.String, typ.Integer), typ.NewUnion(typ.String, typ.Integer)},
		{"empty tuple", typ.NewTuple(), nil},
		{"record", typ.NewRecord().Field("a", typ.String).Field("b", typ.Integer).Build(), typ.NewUnion(typ.String, typ.Integer)},
		{"empty record", typ.NewRecord().Build(), nil},
		{"optional map", typ.NewOptional(typ.NewMap(typ.String, typ.Number)), typ.Number},
		{"alias to map", typ.NewAlias("M", typ.NewMap(typ.String, typ.Boolean)), typ.Boolean},
		{"primitive", typ.String, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValueType(tt.input)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if result == nil {
				t.Errorf("expected %v, got nil", tt.expect)
			} else if result.Hash() != tt.expect.Hash() {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestValueTypeUnion(t *testing.T) {
	union := typ.NewUnion(typ.NewMap(typ.String, typ.Integer), typ.NewMap(typ.String, typ.Boolean))
	result := ValueType(union)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(u.Members))
	}
}

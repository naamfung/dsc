package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBinaryOp_Arithmetic(t *testing.T) {
	tests := []struct {
		name   string
		left   typ.Type
		op     string
		right  typ.Type
		expect typ.Type
	}{
		{"int + int", typ.Integer, "+", typ.Integer, typ.Integer},
		{"int + number", typ.Integer, "+", typ.Number, typ.Number},
		{"number + number", typ.Number, "+", typ.Number, typ.Number},
		{"int - int", typ.Integer, "-", typ.Integer, typ.Integer},
		{"int * int", typ.Integer, "*", typ.Integer, typ.Integer},
		{"int / int", typ.Integer, "/", typ.Integer, typ.Integer},
		{"int // int", typ.Integer, "//", typ.Integer, typ.Integer},
		{"number // number", typ.Number, "//", typ.Number, typ.Integer},
		{"string + number", typ.String, "+", typ.Number, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BinaryOp(tt.left, tt.op, tt.right)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if result == nil || !result.Equals(tt.expect) {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestBinaryOp_Concat(t *testing.T) {
	tests := []struct {
		name   string
		left   typ.Type
		right  typ.Type
		expect typ.Type
	}{
		{"string .. string", typ.String, typ.String, typ.String},
		{"string .. number", typ.String, typ.Number, typ.String}, // Lua coerces
		{"number .. string", typ.Number, typ.String, typ.String},
		{"number .. number", typ.Number, typ.Number, typ.String},
		{"string .. boolean", typ.String, typ.Boolean, nil}, // boolean not coercible
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BinaryOp(tt.left, "..", tt.right)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if result == nil || !result.Equals(tt.expect) {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestBinaryOp_Comparison(t *testing.T) {
	tests := []struct {
		name   string
		left   typ.Type
		op     string
		right  typ.Type
		expect typ.Type
	}{
		{"int == int", typ.Integer, "==", typ.Integer, typ.Boolean},
		{"string == string", typ.String, "==", typ.String, typ.Boolean},
		{"int == string", typ.Integer, "==", typ.String, typ.Boolean}, // always valid
		{"int < int", typ.Integer, "<", typ.Integer, typ.Boolean},
		{"string < string", typ.String, "<", typ.String, typ.Boolean},
		{"int < string", typ.Integer, "<", typ.String, nil}, // incompatible
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BinaryOp(tt.left, tt.op, tt.right)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if result == nil || !result.Equals(tt.expect) {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestBinaryOp_Logical(t *testing.T) {
	t.Run("and with truthy left", func(t *testing.T) {
		// string and number -> number (left is truthy, returns right)
		result := BinaryOp(typ.String, "and", typ.Number)
		if result != typ.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	t.Run("and with falsy left", func(t *testing.T) {
		// nil and string -> nil
		result := BinaryOp(typ.Nil, "and", typ.String)
		if result != typ.Nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("or with truthy left", func(t *testing.T) {
		// string or number -> string
		result := BinaryOp(typ.String, "or", typ.Number)
		if result != typ.String {
			t.Errorf("expected string, got %v", result)
		}
	})

	t.Run("or with falsy left", func(t *testing.T) {
		// nil or string -> string
		result := BinaryOp(typ.Nil, "or", typ.String)
		if result != typ.String {
			t.Errorf("expected string, got %v", result)
		}
	})

	t.Run("or with optional", func(t *testing.T) {
		// string? or "default" -> string (common Lua idiom)
		opt := typ.NewOptional(typ.String)
		result := BinaryOp(opt, "or", typ.String)
		// Result could be string (truthy) or string (from right)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})
}

func TestUnaryOp(t *testing.T) {
	t.Run("minus number", func(t *testing.T) {
		result := UnaryOp("-", typ.Number)
		if result != typ.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	t.Run("minus integer", func(t *testing.T) {
		result := UnaryOp("-", typ.Integer)
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("minus integer literal", func(t *testing.T) {
		result := UnaryOp("-", typ.LiteralInt(2))
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("minus string", func(t *testing.T) {
		result := UnaryOp("-", typ.String)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("length of string", func(t *testing.T) {
		result := UnaryOp("#", typ.String)
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("not of any", func(t *testing.T) {
		result := UnaryOp("not", typ.String)
		if result != typ.Boolean {
			t.Errorf("expected boolean, got %v", result)
		}
	})

	t.Run("length of string literal", func(t *testing.T) {
		result := UnaryOp("#", typ.LiteralString("hello"))
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("length of any", func(t *testing.T) {
		result := UnaryOp("#", typ.Any)
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("length of boolean returns nil", func(t *testing.T) {
		result := UnaryOp("#", typ.Boolean)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("length of number returns nil", func(t *testing.T) {
		result := UnaryOp("#", typ.Number)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("length of integer literal returns nil", func(t *testing.T) {
		result := UnaryOp("#", typ.LiteralInt(42))
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}

// =============================================================================
// Metamethod Dispatch Tests (Lua Semantics)
// =============================================================================

// TestBinaryOp_ArithmeticMetamethod tests that __add/__sub/etc metamethods
// are checked when operands are not numeric.
//
// Lua pattern:
//
//	local Vector = {}
//	Vector.__add = function(a, b) return Vector.new(a.x + b.x, a.y + b.y) end
//	local v1, v2 = Vector.new(1, 2), Vector.new(3, 4)
//	local v3 = v1 + v2  -- Uses __add metamethod
func TestBinaryOp_ArithmeticMetamethod(t *testing.T) {
	// Create Vector type with __add metamethod
	addFn := typ.Func().Param("a", typ.Any).Param("b", typ.Any).Returns(typ.Any).Build()
	meta := typ.NewRecord().Field("__add", addFn).Build()
	vector := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Metatable(meta).Build()

	t.Run("record + record with __add", func(t *testing.T) {
		result := BinaryOp(vector, "+", vector)
		if result == nil {
			t.Error("expected non-nil result from __add metamethod")
		}
	})

	t.Run("record * number with __mul", func(t *testing.T) {
		mulFn := typ.Func().Param("v", typ.Any).Param("n", typ.Number).Returns(typ.Any).Build()
		mulMeta := typ.NewRecord().Field("__mul", mulFn).Build()
		scalable := typ.NewRecord().Field("value", typ.Number).Metatable(mulMeta).Build()

		result := BinaryOp(scalable, "*", typ.Number)
		if result == nil {
			t.Error("expected non-nil result from __mul metamethod")
		}
	})

	t.Run("record without metamethod + record", func(t *testing.T) {
		noMeta := typ.NewRecord().Field("x", typ.Number).Build()

		result := BinaryOp(noMeta, "+", noMeta)
		if result != nil {
			t.Error("expected nil for records without __add")
		}
	})
}

// TestBinaryOp_ConcatMetamethod tests __concat metamethod.
//
// Lua pattern:
//
//	local StringBuilder = {}
//	StringBuilder.__concat = function(a, b) return StringBuilder.new(a.value .. b.value) end
func TestBinaryOp_ConcatMetamethod(t *testing.T) {
	concatFn := typ.Func().Param("a", typ.Any).Param("b", typ.Any).Returns(typ.String).Build()
	meta := typ.NewRecord().Field("__concat", concatFn).Build()
	builder := typ.NewRecord().Field("value", typ.String).Metatable(meta).Build()

	t.Run("record .. record with __concat", func(t *testing.T) {
		result := BinaryOp(builder, "..", builder)
		if result == nil {
			t.Error("expected non-nil result from __concat metamethod")
		}

		if result != typ.String {
			t.Errorf("expected string, got %v", result)
		}
	})

	t.Run("record .. string with __concat", func(t *testing.T) {
		result := BinaryOp(builder, "..", typ.String)
		if result == nil {
			t.Error("expected non-nil result from __concat metamethod")
		}
	})

	t.Run("record without __concat", func(t *testing.T) {
		noMeta := typ.NewRecord().Field("value", typ.String).Build()

		result := BinaryOp(noMeta, "..", noMeta)
		if result != nil {
			t.Error("expected nil for records without __concat")
		}
	})
}

// TestUnaryOp_LengthMetamethod tests __len metamethod.
//
// Lua pattern:
//
//	local Set = {}
//	Set.__len = function(self) return self.count end
//	local s = Set.new()
//	print(#s)  -- Uses __len
func TestUnaryOp_LengthMetamethod(t *testing.T) {
	lenFn := typ.Func().Param("self", typ.Any).Returns(typ.Integer).Build()
	meta := typ.NewRecord().Field("__len", lenFn).Build()
	set := typ.NewRecord().Field("count", typ.Integer).Metatable(meta).Build()
	tableTop := typ.NewInterface("table", nil)

	t.Run("# on record with __len", func(t *testing.T) {
		result := UnaryOp("#", set)
		// Current: returns integer (records have default length behavior)
		// This is actually correct for records without __len,
		// but __len should take precedence if defined
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("# on builtin table marker", func(t *testing.T) {
		result := UnaryOp("#", tableTop)
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})
}

// TestUnaryOp_UnmMetamethod tests __unm (unary minus) metamethod.
//
// Lua pattern:
//
//	local Vector = {}
//	Vector.__unm = function(v) return Vector.new(-v.x, -v.y) end
//	local v = -someVector
func TestUnaryOp_UnmMetamethod(t *testing.T) {
	unmFn := typ.Func().Param("v", typ.Any).Returns(typ.Any).Build()
	meta := typ.NewRecord().Field("__unm", unmFn).Build()
	vector := typ.NewRecord().Field("x", typ.Number).Metatable(meta).Build()

	t.Run("- on record with __unm", func(t *testing.T) {
		result := UnaryOp("-", vector)
		if result == nil {
			t.Error("expected non-nil result from __unm metamethod")
		}
	})

	t.Run("- on record without __unm", func(t *testing.T) {
		noMeta := typ.NewRecord().Field("x", typ.Number).Build()

		result := UnaryOp("-", noMeta)
		if result != nil {
			t.Error("expected nil for record without __unm")
		}
	})
}

// TestBinaryOp_ComparisonMetamethod tests __eq/__lt/__le metamethods.
//
// Lua pattern:
//
//	local Point = {}
//	Point.__eq = function(a, b) return a.x == b.x and a.y == b.y end
func TestBinaryOp_ComparisonMetamethod(t *testing.T) {
	eqFn := typ.Func().Param("a", typ.Any).Param("b", typ.Any).Returns(typ.Boolean).Build()
	meta := typ.NewRecord().Field("__eq", eqFn).Build()
	point := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Metatable(meta).Build()

	t.Run("record == record with __eq", func(t *testing.T) {
		result := BinaryOp(point, "==", point)
		// == always returns boolean regardless of metamethod
		if result != typ.Boolean {
			t.Errorf("expected boolean, got %v", result)
		}
	})

	ltFn := typ.Func().Param("a", typ.Any).Param("b", typ.Any).Returns(typ.Boolean).Build()
	ltMeta := typ.NewRecord().Field("__lt", ltFn).Build()
	comparableRec := typ.NewRecord().Field("value", typ.Number).Metatable(ltMeta).Build()

	t.Run("record < record with __lt", func(t *testing.T) {
		result := BinaryOp(comparableRec, "<", comparableRec)
		if result == nil {
			t.Error("expected non-nil result from __lt metamethod")
		}

		if result != typ.Boolean {
			t.Errorf("expected boolean, got %v", result)
		}
	})

	t.Run("record > record uses __lt with swapped args", func(t *testing.T) {
		result := BinaryOp(comparableRec, ">", comparableRec)
		if result == nil {
			t.Error("expected non-nil result from __lt (swapped for >)")
		}

		if result != typ.Boolean {
			t.Errorf("expected boolean, got %v", result)
		}
	})

	t.Run("record without __lt < record", func(t *testing.T) {
		noMeta := typ.NewRecord().Field("value", typ.Number).Build()

		result := BinaryOp(noMeta, "<", noMeta)
		if result != nil {
			t.Error("expected nil for records without __lt")
		}
	})
}

func TestBinaryOp_Union(t *testing.T) {
	t.Run("union + number", func(t *testing.T) {
		union := typ.NewUnion(typ.Integer, typ.Number)

		result := BinaryOp(union, "+", typ.Number)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("union with non-numeric member", func(t *testing.T) {
		union := typ.NewUnion(typ.Integer, typ.String)
		result := BinaryOp(union, "+", typ.Number)
		// String can't be added, so this should fail or return partial
		// Current behavior: returns nil if any member fails
		if result != nil {
			t.Logf("got result: %v", result)
		}
	})
}

func TestBinaryOp_NilInputs(t *testing.T) {
	if BinaryOp(nil, "+", typ.Number) != nil {
		t.Error("nil left should return nil")
	}

	if BinaryOp(typ.Number, "+", nil) != nil {
		t.Error("nil right should return nil")
	}
}

// =============================================================================
// Bitwise Operator Tests (Lua 5.3+)
// =============================================================================

// TestBinaryOp_Bitwise tests bitwise operators.
//
// Lua 5.3+ operators: &, |, ~, <<, >>
func TestBinaryOp_Bitwise(t *testing.T) {
	tests := []struct {
		name   string
		left   typ.Type
		op     string
		right  typ.Type
		expect typ.Type
	}{
		{"int & int", typ.Integer, "&", typ.Integer, typ.Integer},
		{"int | int", typ.Integer, "|", typ.Integer, typ.Integer},
		{"int ~ int", typ.Integer, "~", typ.Integer, typ.Integer},
		{"int << int", typ.Integer, "<<", typ.Integer, typ.Integer},
		{"int >> int", typ.Integer, ">>", typ.Integer, typ.Integer},
		{"number & number", typ.Number, "&", typ.Number, typ.Integer}, // Truncates to int
		{"string & int", typ.String, "&", typ.Integer, nil},           // Invalid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BinaryOp(tt.left, tt.op, tt.right)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if result == nil {
				t.Fatalf("bitwise operator %s should be implemented", tt.op)
			} else if !result.Equals(tt.expect) {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

// TestUnaryOp_BitwiseNot tests bitwise NOT (~) operator.
func TestUnaryOp_BitwiseNot(t *testing.T) {
	t.Run("~integer", func(t *testing.T) {
		result := UnaryOp("~", typ.Integer)
		if result == nil {
			t.Fatal("bitwise ~ should be implemented")
		}

		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("~string", func(t *testing.T) {
		result := UnaryOp("~", typ.String)
		if result != nil {
			t.Errorf("expected nil for ~string, got %v", result)
		}
	})
}

// TestBinaryOp_BitwiseMetamethod tests bitwise metamethods.
//
// Metamethods: __band, __bor, __bxor, __bnot, __shl, __shr
func TestBinaryOp_BitwiseMetamethod(t *testing.T) {
	bandFn := typ.Func().Param("a", typ.Any).Param("b", typ.Any).Returns(typ.Integer).Build()
	meta := typ.NewRecord().Field("__band", bandFn).Build()
	bitset := typ.NewRecord().Field("bits", typ.Integer).Metatable(meta).Build()

	t.Run("record & record with __band", func(t *testing.T) {
		result := BinaryOp(bitset, "&", bitset)
		if result == nil {
			t.Fatal("__band metamethod should be implemented")
		}

		if result != typ.Integer {
			t.Errorf("expected integer from __band, got %v", result)
		}
	})
}

func TestUnaryOp_NilInput(t *testing.T) {
	if UnaryOp("-", nil) != nil {
		t.Error("nil operand should return nil")
	}
}

func TestBinaryOp_TypeAlias(t *testing.T) {
	// Type alias for number
	amountAlias := typ.NewAlias("Amount", typ.Number)

	t.Run("alias + number", func(t *testing.T) {
		result := BinaryOp(amountAlias, "+", typ.Number)
		if result == nil || result.Kind() != kind.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	t.Run("number + alias", func(t *testing.T) {
		result := BinaryOp(typ.Number, "+", amountAlias)
		if result == nil || result.Kind() != kind.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	t.Run("alias + alias", func(t *testing.T) {
		result := BinaryOp(amountAlias, "+", amountAlias)
		if result == nil || result.Kind() != kind.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	t.Run("alias * integer", func(t *testing.T) {
		result := BinaryOp(amountAlias, "*", typ.Integer)
		if result == nil || result.Kind() != kind.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	// Type alias for string
	nameAlias := typ.NewAlias("Name", typ.String)

	t.Run("alias .. string", func(t *testing.T) {
		result := BinaryOp(nameAlias, "..", typ.String)
		if result == nil || result.Kind() != kind.String {
			t.Errorf("expected string, got %v", result)
		}
	})

	// Type alias for integer
	countAlias := typ.NewAlias("Count", typ.Integer)

	t.Run("alias // alias", func(t *testing.T) {
		result := BinaryOp(countAlias, "//", countAlias)
		if result == nil || result.Kind() != kind.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})
}

func TestUnaryOp_TypeAlias(t *testing.T) {
	amountAlias := typ.NewAlias("Amount", typ.Number)

	t.Run("-alias", func(t *testing.T) {
		result := UnaryOp("-", amountAlias)
		if result == nil || result.Kind() != kind.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	countAlias := typ.NewAlias("Count", typ.Integer)

	t.Run("~alias", func(t *testing.T) {
		result := UnaryOp("~", countAlias)
		if result == nil || result.Kind() != kind.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})
}

func TestBinaryOp_Any(t *testing.T) {
	t.Run("any + any", func(t *testing.T) {
		result := BinaryOp(typ.Any, "+", typ.Any)
		if result != typ.Any {
			t.Errorf("expected any, got %v", result)
		}
	})

	t.Run("any .. any", func(t *testing.T) {
		result := BinaryOp(typ.Any, "..", typ.Any)
		if result != typ.Any {
			t.Errorf("expected any, got %v", result)
		}
	})

	t.Run("any == any", func(t *testing.T) {
		result := BinaryOp(typ.Any, "==", typ.Any)
		if result != typ.Boolean {
			t.Errorf("expected boolean, got %v", result)
		}
	})
}

func TestBinaryOp_Unknown(t *testing.T) {
	t.Run("unknown + number keeps numeric type", func(t *testing.T) {
		result := BinaryOp(typ.Unknown, "+", typ.Number)
		if result != typ.Number {
			t.Errorf("expected number, got %v", result)
		}
	})

	t.Run("integer + unknown keeps numeric type", func(t *testing.T) {
		result := BinaryOp(typ.Integer, "+", typ.Unknown)
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("unknown concat string stays unknown", func(t *testing.T) {
		result := BinaryOp(typ.Unknown, "..", typ.String)
		if result != typ.Unknown {
			t.Errorf("expected unknown, got %v", result)
		}
	})
}

func TestUnaryOp_Unknown(t *testing.T) {
	t.Run("length unknown returns integer", func(t *testing.T) {
		result := UnaryOp("#", typ.Unknown)
		if result != typ.Integer {
			t.Errorf("expected integer, got %v", result)
		}
	})

	t.Run("minus unknown returns unknown", func(t *testing.T) {
		result := UnaryOp("-", typ.Unknown)
		if result != typ.Unknown {
			t.Errorf("expected unknown, got %v", result)
		}
	})
}

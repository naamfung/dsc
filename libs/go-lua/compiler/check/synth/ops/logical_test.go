package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestLogicalAndTyped_LeftTruthy(t *testing.T) {
	result := LogicalAndTyped(typ.Integer, typ.String)
	if result != typ.String {
		t.Errorf("truthy and X should return X, got %v", result)
	}
}

func TestLogicalAndTyped_LeftFalsy(t *testing.T) {
	result := LogicalAndTyped(typ.Nil, typ.String)
	if result != typ.Nil {
		t.Errorf("nil and X should return nil, got %v", result)
	}
}

func TestLogicalAndTyped_LeftOptional(t *testing.T) {
	result := LogicalAndTyped(typ.NewOptional(typ.Integer), typ.String)
	if result == nil {
		t.Fatal("should not be nil")
	}
	// Result is nil | string, which may be represented as Optional(string) or Union
	if CanBeFalsy(result) {
		return // correct: contains falsy part
	}
	t.Errorf("expected result to contain falsy part, got %v", result)
}

func TestLogicalAndTyped_LeftFalseLiteral(t *testing.T) {
	result := LogicalAndTyped(typ.LiteralBool(false), typ.String)
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("expected literal, got %T", result)
	}

	if b, ok := lit.Value.(bool); !ok || b {
		t.Error("should return false literal")
	}
}

func TestLogicalAndTyped_Never(t *testing.T) {
	result := LogicalAndTyped(typ.Never, typ.String)
	if result.Kind() != kind.Never {
		t.Errorf("never and X should return never, got %v", result)
	}

	result = LogicalAndTyped(typ.Integer, typ.Never)
	if result.Kind() != kind.Never {
		t.Errorf("X and never should return never, got %v", result)
	}
}

func TestLogicalOrTyped_LeftTruthy(t *testing.T) {
	result := LogicalOrTyped(typ.Integer, typ.String)
	if result != typ.Integer {
		t.Errorf("truthy or X should return truthy, got %v", result)
	}
}

func TestLogicalOrTyped_LeftFalsy(t *testing.T) {
	result := LogicalOrTyped(typ.Nil, typ.String)
	if result != typ.String {
		t.Errorf("nil or X should return X, got %v", result)
	}
}

func TestLogicalOrTyped_LeftOptional(t *testing.T) {
	result := LogicalOrTyped(typ.NewOptional(typ.Integer), typ.String)
	if result == nil {
		t.Fatal("should not be nil")
	}
	// Should be integer | string (truthy part of optional | right)
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}

	if len(u.Members) < 2 {
		t.Error("expected at least 2 union members")
	}
}

func TestLogicalOrTyped_SoftOptionalPrefersRight(t *testing.T) {
	left := typ.NewOptional(typ.NewArray(typ.Any))
	right := typ.NewArray(typ.Number)
	result := LogicalOrTyped(left, right)
	if result == nil || result.String() != "number[]" {
		t.Errorf("expected right to win for soft optional, got %v", result)
	}
}

func TestLogicalOrTyped_Never(t *testing.T) {
	result := LogicalOrTyped(typ.Never, typ.String)
	if result.Kind() != kind.Never {
		t.Errorf("never or X should return never, got %v", result)
	}

	result = LogicalOrTyped(typ.Integer, typ.Never)
	if result.Kind() != kind.Never {
		t.Errorf("X or never should return never, got %v", result)
	}
}

func TestLogicalOrTyped_FalseLiteral(t *testing.T) {
	result := LogicalOrTyped(typ.LiteralBool(false), typ.Integer)
	if result != typ.Integer {
		t.Errorf("false or X should return X, got %v", result)
	}
}

func TestLogicalAndTyped_Boolean(t *testing.T) {
	result := LogicalAndTyped(typ.Boolean, typ.String)
	if result == nil {
		t.Fatal("should not be nil")
	}
}

func TestLogicalAndTyped_DoesNotDropSoftRightToNil(t *testing.T) {
	result := LogicalAndTyped(typ.NewOptional(typ.Number), typ.Any)
	if typ.TypeEquals(result, typ.Nil) {
		t.Fatalf("optional(number) and any collapsed to nil: %v", result)
	}
}

func TestLogicalOrTyped_Boolean(t *testing.T) {
	result := LogicalOrTyped(typ.Boolean, typ.String)
	if result == nil {
		t.Fatal("should not be nil")
	}
}

// Regression tests for nil type handling.
// A nil Go type represents unknown/unresolved type, not definitely truthy.

func TestLogicalAndTyped_NilLeft(t *testing.T) {
	// nil left type should NOT be treated as definitely truthy
	result := LogicalAndTyped(nil, typ.String)
	// With unknown left, result should be unknown
	if result != typ.Unknown {
		t.Errorf("nil left should return unknown, got %v", result)
	}
}

func TestLogicalAndTyped_NilRight(t *testing.T) {
	result := LogicalAndTyped(typ.Integer, nil)
	// truthy and nil -> nil (the right operand)
	if result != nil {
		t.Errorf("integer and nil should return nil, got %v", result)
	}
}

func TestLogicalOrTyped_NilLeft(t *testing.T) {
	// nil left type should NOT be treated as definitely truthy
	result := LogicalOrTyped(nil, typ.String)
	// With unknown left, result should be unknown
	if result != typ.Unknown {
		t.Errorf("nil left should return unknown, got %v", result)
	}
}

func TestLogicalOrTyped_NilRight(t *testing.T) {
	result := LogicalOrTyped(typ.Nil, nil)
	// nil or nil -> nil (the right operand since left is falsy)
	if result != nil {
		t.Errorf("nil or nil should return nil, got %v", result)
	}
}

func TestLogicalOrTyped_UnknownOrNil_DoesNotCollapseToNil(t *testing.T) {
	result := LogicalOrTyped(typ.Unknown, typ.Nil)
	if typ.TypeEquals(result, typ.Nil) {
		t.Fatalf("unknown or nil collapsed to nil: %v", result)
	}
	if _, ok := result.(*typ.Optional); !ok {
		t.Fatalf("unknown or nil should preserve uncertainty as optional, got %T (%v)", result, result)
	}
}

func TestLogicalOrTyped_AnyOrNil_DoesNotCollapseToNil(t *testing.T) {
	result := LogicalOrTyped(typ.Any, typ.Nil)
	if typ.TypeEquals(result, typ.Nil) {
		t.Fatalf("any or nil collapsed to nil: %v", result)
	}
}

func TestLogicalAndTyped_UnknownAndUnknown_DoesNotCollapseToFalsy(t *testing.T) {
	result := LogicalAndTyped(typ.Unknown, typ.Unknown)
	if !typ.TypeEquals(result, typ.Unknown) {
		t.Fatalf("unknown and unknown should stay unknown, got %v", result)
	}
}

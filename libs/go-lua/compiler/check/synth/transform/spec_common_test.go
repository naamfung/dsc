package transform

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestConditionAnyDisjunctMatches(t *testing.T) {
	if !conditionAnyDisjunctMatches(constraint.TrueCondition(), func(constraint.Constraint) bool { return false }) {
		t.Fatal("true condition should always match")
	}
	if conditionAnyDisjunctMatches(constraint.FalseCondition(), func(constraint.Constraint) bool { return true }) {
		t.Fatal("false condition should never match")
	}

	path := constraint.Path{Root: "x"}
	cond := constraint.Or(
		constraint.FromConstraints(constraint.NotNil{Path: path}),
		constraint.FromConstraints(constraint.Truthy{Path: path}),
	)

	matched := conditionAnyDisjunctMatches(cond, func(c constraint.Constraint) bool {
		_, ok := c.(constraint.Truthy)
		return ok
	})
	if !matched {
		t.Fatal("expected match when one disjunct fully matches predicate")
	}
}

func TestPlaceholderArgIndex(t *testing.T) {
	if idx, ok := constraint.PlaceholderArgIndex(constraint.ParamPath(0), 1); !ok || idx != 0 {
		t.Fatalf("PlaceholderArgIndex($0,1) = (%d,%v), want (0,true)", idx, ok)
	}
	if idx, ok := constraint.PlaceholderArgIndex(constraint.ParamPath(1), 1); ok {
		t.Fatalf("PlaceholderArgIndex($1,1) should fail, got (%d,true)", idx)
	}
	if idx, ok := constraint.PlaceholderArgIndex(constraint.Path{Root: "x"}, 1); ok {
		t.Fatalf("PlaceholderArgIndex(non-placeholder,1) should fail, got (%d,true)", idx)
	}
}

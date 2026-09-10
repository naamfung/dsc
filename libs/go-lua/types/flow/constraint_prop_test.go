package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestCondition_And_BothTrue(t *testing.T) {
	a := constraint.TrueCondition()
	b := constraint.TrueCondition()
	result := constraint.And(a, b)
	if !result.IsTrue() {
		t.Fatalf("expected true condition")
	}
}

func TestCondition_And_OneHasConstraints(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	a := constraint.FromConstraints(constraint.Truthy{Path: pathX})
	b := constraint.TrueCondition()

	result := constraint.And(a, b)
	if result.NumDisjuncts() != 1 {
		t.Fatalf("expected 1 disjunct, got %d", result.NumDisjuncts())
	}
	if len(result.DisjunctConstraints(0)) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(result.DisjunctConstraints(0)))
	}
}

func TestCondition_And_BothHaveConstraints(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}
	a := constraint.FromConstraints(constraint.Truthy{Path: pathX})
	b := constraint.FromConstraints(constraint.Truthy{Path: pathY})

	result := constraint.And(a, b)
	if result.NumDisjuncts() != 1 {
		t.Fatalf("expected 1 disjunct, got %d", result.NumDisjuncts())
	}
	if len(result.DisjunctConstraints(0)) != 2 {
		t.Fatalf("expected 2 constraints, got %d", len(result.DisjunctConstraints(0)))
	}
}

func TestCondition_Or_BothFalse(t *testing.T) {
	a := constraint.FalseCondition()
	b := constraint.FalseCondition()
	result := constraint.Or(a, b)
	if !result.IsFalse() {
		t.Fatalf("expected false condition")
	}
}

func TestCondition_Or_OneFalse(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	a := constraint.FromConstraints(constraint.Truthy{Path: pathX})
	b := constraint.FalseCondition()

	result := constraint.Or(a, b)
	if result.NumDisjuncts() != 1 {
		t.Fatalf("expected 1 disjunct, got %d", result.NumDisjuncts())
	}
}

func TestCondition_Or_BothHaveConstraints(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}
	a := constraint.FromConstraints(constraint.Truthy{Path: pathX})
	b := constraint.FromConstraints(constraint.Truthy{Path: pathY})

	result := constraint.Or(a, b)
	if result.NumDisjuncts() != 2 {
		t.Fatalf("expected 2 disjuncts, got %d", result.NumDisjuncts())
	}
}

func TestCondition_Equals(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	a := constraint.FromConstraints(constraint.Truthy{Path: pathX})
	b := constraint.FromConstraints(constraint.Truthy{Path: pathX})

	if !a.Equals(b) {
		t.Fatal("expected equal")
	}

	pathY := constraint.Path{Root: "y"}
	c := constraint.FromConstraints(constraint.Truthy{Path: pathY})
	if a.Equals(c) {
		t.Fatal("expected not equal")
	}
}

func TestCondition_CommonConstraints(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}
	common := constraint.Truthy{Path: pathX}

	a := constraint.FromConstraints(common, constraint.Truthy{Path: pathY})
	b := constraint.FromConstraints(common)

	// OR the two conditions
	result := constraint.Or(a, b)

	// MustConstraints returns what's common to all disjuncts
	must := result.MustConstraints()
	if len(must) != 1 {
		t.Fatalf("expected 1 common constraint, got %d", len(must))
	}
}

func TestCondition_NoCommon(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}

	a := constraint.FromConstraints(constraint.Truthy{Path: pathX})
	b := constraint.FromConstraints(constraint.Truthy{Path: pathY})

	result := constraint.Or(a, b)
	must := result.MustConstraints()
	if len(must) != 0 {
		t.Fatalf("expected 0 common constraints, got %d", len(must))
	}
}

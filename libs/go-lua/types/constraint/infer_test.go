package constraint_test

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferSetBasic(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	cs.AddSubtype(tv, typ.String)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 1 {
		t.Fatalf("expected 1 solution, got %d", len(solution))
	}

	if solution[1] != typ.String {
		t.Errorf("expected string, got %v", solution[1])
	}
}

func TestInferSetArray(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	arrayOfT := typ.NewArray(tv)
	arrayOfString := typ.NewArray(typ.String)

	constraint.MatchContra(arrayOfT, arrayOfString, cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 1 {
		t.Fatalf("expected 1 solution, got %d", len(solution))
	}

	if solution[1] != typ.String {
		t.Errorf("expected string, got %v", solution[1])
	}
}

func TestInferSetMap(t *testing.T) {
	cs := constraint.NewInferSet()
	tvK := typ.NewTypeVar(1)
	tvV := typ.NewTypeVar(2)

	mapKV := typ.NewMap(tvK, tvV)
	mapStringNumber := typ.NewMap(typ.String, typ.Number)

	constraint.MatchContra(mapKV, mapStringNumber, cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 2 {
		t.Fatalf("expected 2 solutions, got %d", len(solution))
	}

	if solution[1] != typ.String {
		t.Errorf("expected key to be string, got %v", solution[1])
	}

	if solution[2] != typ.Number {
		t.Errorf("expected value to be number, got %v", solution[2])
	}
}

func TestInferSetMultipleConstraints(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	cs.AddSubtype(typ.Integer, tv)
	cs.AddSubtype(tv, typ.Number)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 1 {
		t.Fatalf("expected 1 solution, got %d", len(solution))
	}

	if solution[1] != typ.Integer {
		t.Errorf("expected integer (lower bound preferred), got %v", solution[1])
	}
}

func TestInferSetRecord(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	patternRec := typ.NewRecord().Field("value", tv).Build()
	concreteRec := typ.NewRecord().Field("value", typ.String).Build()

	constraint.MatchContra(patternRec, concreteRec, cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if solution[1] != typ.String {
		t.Errorf("expected string, got %v", solution[1])
	}
}

func TestInferSubstitutionApply(t *testing.T) {
	tv := typ.NewTypeVar(1)
	sub := constraint.InferSubstitution{
		1: typ.String,
	}

	result := sub.Apply(typ.NewArray(tv))
	arr, ok := result.(*typ.Array)

	if !ok {
		t.Fatalf("expected array, got %T", result)
	}

	if arr.Element != typ.String {
		t.Errorf("expected array of string, got %v", arr.Element)
	}
}

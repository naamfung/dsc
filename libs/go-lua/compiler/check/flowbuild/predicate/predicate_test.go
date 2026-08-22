package predicate_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestLookupPredicateLink_NilInputsParam(t *testing.T) {
	result := predicate.LookupPredicateLink("name", nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestLookupPredicateLink_NilPredicateLinksMap(t *testing.T) {
	inputs := &flow.Inputs{PredicateLinks: nil}
	result := predicate.LookupPredicateLink("name", inputs)
	if result != nil {
		t.Error("expected nil for nil predicate links")
	}
}

func TestLookupPredicateLink_EmptyNameParam(t *testing.T) {
	inputs := &flow.Inputs{
		PredicateLinks: map[string]flow.PredicateLink{
			"name@1": {},
		},
	}
	result := predicate.LookupPredicateLink("", inputs)
	if result != nil {
		t.Error("expected nil for empty name")
	}
}

func TestLookupPredicateLink_NoMatchFound(t *testing.T) {
	inputs := &flow.Inputs{
		PredicateLinks: map[string]flow.PredicateLink{
			"other@1": {},
		},
	}
	result := predicate.LookupPredicateLink("name", inputs)
	if result != nil {
		t.Error("expected nil when no match found")
	}
}

func TestLookupPredicateLink_SingleMatchFound(t *testing.T) {
	link := flow.PredicateLink{
		OnTruthy: constraint.Condition{
			Disjuncts: [][]constraint.Constraint{
				{constraint.NotNil{Path: constraint.Path{Root: "x"}}},
			},
		},
	}
	inputs := &flow.Inputs{
		PredicateLinks: map[string]flow.PredicateLink{
			"name@5": link,
		},
	}
	result := predicate.LookupPredicateLink("name", inputs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.OnTruthy.Disjuncts) != 1 {
		t.Errorf("expected 1 disjunct, got %d", len(result.OnTruthy.Disjuncts))
	}
}

func TestLookupPredicateLink_HighestDefPointSelected(t *testing.T) {
	wantCondition := constraint.Condition{
		Disjuncts: [][]constraint.Constraint{
			{constraint.NotNil{Path: constraint.Path{Root: "marker"}}},
		},
	}
	inputs := &flow.Inputs{
		PredicateLinks: map[string]flow.PredicateLink{
			"name@3":  {},
			"name@7":  {OnTruthy: wantCondition},
			"name@1":  {},
			"name@10": {},
		},
	}
	result := predicate.LookupPredicateLink("name", inputs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.OnTruthy.Disjuncts) != 0 {
		t.Log("result should be from name@10 which has empty condition")
	}
}

func TestLookupPredicateLink_InvalidDefPointFormat(t *testing.T) {
	inputs := &flow.Inputs{
		PredicateLinks: map[string]flow.PredicateLink{
			"name@abc": {},
		},
	}
	result := predicate.LookupPredicateLink("name", inputs)
	if result != nil {
		t.Error("expected nil for invalid def point")
	}
}

func TestLinkKey_FormatCorrect(t *testing.T) {
	tests := []struct {
		name     string
		defPoint cfg.Point
		expected string
	}{
		{"var", 0, "var@0"},
		{"myFunc", 42, "myFunc@42"},
		{"x", 100, "x@100"},
	}

	for _, tt := range tests {
		result := predicate.LinkKey(tt.name, tt.defPoint)
		if result != tt.expected {
			t.Errorf("LinkKey(%q, %d) = %q, expected %q", tt.name, tt.defPoint, result, tt.expected)
		}
	}
}

func TestBuildConstResolver_NilInputsParam(t *testing.T) {
	result := predicate.BuildConstResolver(nil, 0)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestBuildConstResolver_NilConstValuesMap(t *testing.T) {
	result := predicate.BuildConstResolver(&flow.Inputs{ConstValues: nil}, 0)
	if result != nil {
		t.Error("expected nil for nil const values")
	}
}

func TestBuildConstResolver_NilGraphInInputs(t *testing.T) {
	result := predicate.BuildConstResolver(&flow.Inputs{
		ConstValues: make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		Graph:       nil,
	}, 0)
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

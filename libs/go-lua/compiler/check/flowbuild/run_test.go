package flowbuild

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRun_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	result := Run(fc)
	if result != nil {
		t.Errorf("expected nil for nil graph, got %v", result)
	}
}

func TestCoreDecomposer_ElementType_Unknown(t *testing.T) {
	d := coreDecomposer{}
	result := d.ElementType(typ.Unknown)
	if result != nil {
		t.Errorf("expected nil for Unknown type, got %v", result)
	}
}

func TestCoreDecomposer_ElementType_Array(t *testing.T) {
	d := coreDecomposer{}
	arr := typ.NewArray(typ.String)
	result := d.ElementType(arr)
	if result != typ.String {
		t.Errorf("expected string element type, got %v", result)
	}
}

func TestCoreDecomposer_KeyType_Unknown(t *testing.T) {
	d := coreDecomposer{}
	result := d.KeyType(typ.Unknown)
	if result != nil {
		t.Errorf("expected nil for Unknown type, got %v", result)
	}
}

func TestCoreDecomposer_KeyType_Map(t *testing.T) {
	d := coreDecomposer{}
	m := typ.NewMap(typ.String, typ.Number)
	result := d.KeyType(m)
	if result != typ.String {
		t.Errorf("expected string key type, got %v", result)
	}
}

func TestCoreDecomposer_ValueType_Unknown(t *testing.T) {
	d := coreDecomposer{}
	result := d.ValueType(typ.Unknown)
	if result != nil {
		t.Errorf("expected nil for Unknown type, got %v", result)
	}
}

func TestCoreDecomposer_ValueType_Map(t *testing.T) {
	d := coreDecomposer{}
	m := typ.NewMap(typ.String, typ.Number)
	result := d.ValueType(m)
	if result != typ.Number {
		t.Errorf("expected number value type, got %v", result)
	}
}

func TestMergeCallConstraintsIntoEdges_Nil(t *testing.T) {
	inputs := &flow.Inputs{
		EdgeConditions: nil,
	}
	MergeCallConstraintsIntoEdges(inputs, nil)
	if len(inputs.EdgeConditions) != 0 {
		t.Errorf("expected no edge conditions, got %d", len(inputs.EdgeConditions))
	}
}

func TestMergeCallConstraintsIntoEdges_EmptyMap(t *testing.T) {
	inputs := &flow.Inputs{
		EdgeConditions: nil,
	}
	MergeCallConstraintsIntoEdges(inputs, map[cond.EdgeKey]constraint.Condition{})
	if len(inputs.EdgeConditions) != 0 {
		t.Errorf("expected no edge conditions, got %d", len(inputs.EdgeConditions))
	}
}

func TestMergeCallConstraintsIntoEdges_FalseConditionSkipped(t *testing.T) {
	inputs := &flow.Inputs{
		EdgeConditions: nil,
	}
	constraints := map[cond.EdgeKey]constraint.Condition{
		{From: 1, To: 2}: constraint.FalseCondition(),
	}
	MergeCallConstraintsIntoEdges(inputs, constraints)
	if len(inputs.EdgeConditions) != 0 {
		t.Errorf("expected 0 edge conditions (FalseCondition has no constraints), got %d", len(inputs.EdgeConditions))
	}
}

func TestMergeCallConstraintsIntoEdges_WithConstraints(t *testing.T) {
	inputs := &flow.Inputs{
		EdgeConditions: nil,
	}
	path := constraint.Path{Root: "x", Symbol: 1}
	cond1 := constraint.FromConstraints(constraint.IsNil{Path: path})
	constraints := map[cond.EdgeKey]constraint.Condition{
		{From: 1, To: 2}: cond1,
	}
	MergeCallConstraintsIntoEdges(inputs, constraints)
	if len(inputs.EdgeConditions) != 1 {
		t.Fatalf("expected 1 edge condition, got %d", len(inputs.EdgeConditions))
	}
	if inputs.EdgeConditions[0].From != cfg.Point(1) {
		t.Errorf("expected From=1, got %v", inputs.EdgeConditions[0].From)
	}
	if inputs.EdgeConditions[0].To != cfg.Point(2) {
		t.Errorf("expected To=2, got %v", inputs.EdgeConditions[0].To)
	}
}

func TestMergeCallConstraintsIntoEdges_SortsByFromThenTo(t *testing.T) {
	inputs := &flow.Inputs{
		EdgeConditions: nil,
	}
	path1 := constraint.Path{Root: "a", Symbol: 1}
	path2 := constraint.Path{Root: "b", Symbol: 2}
	path3 := constraint.Path{Root: "c", Symbol: 3}
	cond1 := constraint.FromConstraints(constraint.IsNil{Path: path1})
	cond2 := constraint.FromConstraints(constraint.IsNil{Path: path2})
	cond3 := constraint.FromConstraints(constraint.IsNil{Path: path3})
	constraints := map[cond.EdgeKey]constraint.Condition{
		{From: 3, To: 4}: cond3,
		{From: 1, To: 2}: cond1,
		{From: 1, To: 3}: cond2,
	}
	MergeCallConstraintsIntoEdges(inputs, constraints)
	if len(inputs.EdgeConditions) != 3 {
		t.Fatalf("expected 3 edge conditions, got %d", len(inputs.EdgeConditions))
	}
	if inputs.EdgeConditions[0].From != 1 || inputs.EdgeConditions[0].To != 2 {
		t.Errorf("expected first edge (1,2), got (%v,%v)", inputs.EdgeConditions[0].From, inputs.EdgeConditions[0].To)
	}
	if inputs.EdgeConditions[1].From != 1 || inputs.EdgeConditions[1].To != 3 {
		t.Errorf("expected second edge (1,3), got (%v,%v)", inputs.EdgeConditions[1].From, inputs.EdgeConditions[1].To)
	}
	if inputs.EdgeConditions[2].From != 3 || inputs.EdgeConditions[2].To != 4 {
		t.Errorf("expected third edge (3,4), got (%v,%v)", inputs.EdgeConditions[2].From, inputs.EdgeConditions[2].To)
	}
}

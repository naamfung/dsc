package propagate

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

type mockGraph struct {
	entry cfg.Point
	nodes map[cfg.Point]*cfg.Node
	preds map[cfg.Point][]cfg.Point
	succs map[cfg.Point][]cfg.Point
	rpo   []cfg.Point
}

func (m *mockGraph) Entry() cfg.Point                     { return m.entry }
func (m *mockGraph) RPO() []cfg.Point                     { return m.rpo }
func (m *mockGraph) Node(p cfg.Point) *cfg.Node           { return m.nodes[p] }
func (m *mockGraph) Predecessors(p cfg.Point) []cfg.Point { return m.preds[p] }
func (m *mockGraph) Successors(p cfg.Point) []cfg.Point   { return m.succs[p] }

func TestPropagate_SimpleLinear(t *testing.T) {
	// entry -> block1 -> block2
	g := &mockGraph{
		entry: 1,
		nodes: map[cfg.Point]*cfg.Node{
			1: {Kind: cfg.NodeEntry},
			2: {Kind: cfg.NodeAssign},
			3: {Kind: cfg.NodeAssign},
		},
		preds: map[cfg.Point][]cfg.Point{
			1: {},
			2: {1},
			3: {2},
		},
		succs: map[cfg.Point][]cfg.Point{
			1: {2},
			2: {3},
			3: {},
		},
		rpo: []cfg.Point{1, 2, 3},
	}

	inputs := &Inputs{
		Graph:          g,
		EdgeConditions: make(EdgeConditions),
	}

	result := Propagate(inputs)

	if result.PointConditions[1].IsFalse() {
		t.Error("entry should not be false")
	}
	if result.PointConditions[2].IsFalse() {
		t.Error("block1 should not be false")
	}
	if result.PointConditions[3].IsFalse() {
		t.Error("block2 should not be false")
	}
}

func TestPropagate_WithEdgeCondition(t *testing.T) {
	// entry -> branch -> then
	g := &mockGraph{
		entry: 1,
		nodes: map[cfg.Point]*cfg.Node{
			1: {Kind: cfg.NodeEntry},
			2: {Kind: cfg.NodeBranch},
			3: {Kind: cfg.NodeAssign},
		},
		preds: map[cfg.Point][]cfg.Point{
			1: {},
			2: {1},
			3: {2},
		},
		succs: map[cfg.Point][]cfg.Point{
			1: {2},
			2: {3},
			3: {},
		},
		rpo: []cfg.Point{1, 2, 3},
	}

	path := constraint.Path{Root: "x", Symbol: 100}
	edgeCond := constraint.FromConstraints(constraint.NotNil{Path: path})

	inputs := &Inputs{
		Graph: g,
		EdgeConditions: EdgeConditions{
			{From: 2, To: 3}: edgeCond,
		},
	}

	result := Propagate(inputs)

	// At block3, the condition should include NotNil
	cond := result.PointConditions[3]
	if cond.IsFalse() {
		t.Error("then block should not be false")
	}
	if !cond.HasConstraints() {
		t.Error("then block should have constraints from edge condition")
	}
}

func TestFilterConditionSymbols(t *testing.T) {
	path := constraint.Path{Root: "x", Symbol: 100}
	cond := constraint.FromConstraints(constraint.NotNil{Path: path})

	// Filter out symbol 100
	filtered := FilterConditionSymbols(cond, []cfg.SymbolID{100})
	if !filtered.IsTrue() {
		t.Errorf("expected true after filtering out the only constraint, got %v", filtered)
	}

	// Filter out different symbol - should keep constraint
	filtered2 := FilterConditionSymbols(cond, []cfg.SymbolID{200})
	if !filtered2.HasConstraints() {
		t.Error("expected constraints to remain when filtering different symbol")
	}
}

func TestPathAffectedByAssignment(t *testing.T) {
	tests := []struct {
		name       string
		path       constraint.Path
		assignSym  cfg.SymbolID
		assignSegs []constraint.Segment
		want       bool
	}{
		{
			name:      "same symbol no segments",
			path:      constraint.Path{Symbol: 100},
			assignSym: 100,
			want:      true,
		},
		{
			name:      "different symbol",
			path:      constraint.Path{Symbol: 100},
			assignSym: 200,
			want:      false,
		},
		{
			name: "path is child of assignment",
			path: constraint.Path{
				Symbol: 100,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "x"},
					{Kind: constraint.SegmentField, Name: "y"},
				},
			},
			assignSym: 100,
			assignSegs: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "x"},
			},
			want: true,
		},
		{
			name: "path is sibling of assignment",
			path: constraint.Path{
				Symbol: 100,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "y"},
				},
			},
			assignSym: 100,
			assignSegs: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "x"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PathAffectedByAssignment(tt.path, tt.assignSym, tt.assignSegs)
			if got != tt.want {
				t.Errorf("PathAffectedByAssignment() = %v, want %v", got, tt.want)
			}
		})
	}
}

package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFlow_NumericConstraints_Contradiction(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Integer

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{
		{
			From: branch,
			To:   thenNode,
			Constraints: []constraint.NumericConstraint{
				constraint.LeConst{X: pathX, C: -1}, // x < 0 means x <= -1
				constraint.GeConst{X: pathX, C: 2},  // x > 1 means x >= 2
			},
		},
	}

	s := Solve(inputs, testResolver())

	if !s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("expected edge to be marked unreachable due to numeric contradiction")
	}

	edges := s.UnreachableEdges()
	found := false

	for _, e := range edges {
		if e.From == branch && e.To == thenNode {
			found = true
			break
		}
	}

	if !found {
		t.Error("branch->thenNode edge should be in unreachable edges")
	}
}

func TestFlow_NumericConstraints_Satisfiable(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Integer

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{
		{
			From: branch,
			To:   thenNode,
			Constraints: []constraint.NumericConstraint{
				constraint.GeConst{X: pathX, C: 0},  // x >= 0
				constraint.LeConst{X: pathX, C: 10}, // x <= 10
			},
		},
	}

	s := Solve(inputs, testResolver())

	if s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("edge should NOT be marked unreachable - constraints are satisfiable")
	}

	edges := s.UnreachableEdges()
	if len(edges) != 0 {
		t.Errorf("expected 0 unreachable edges, got %d", len(edges))
	}
}

func TestFlow_NumericConstraints_PathContradiction(t *testing.T) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	target := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, branch2, true)
	c.AddEdge(branch2, target, true)
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(branch1, c.Exit(), false)

	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch1, branch2, target, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Integer

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{
		{
			From:        branch1,
			To:          branch2,
			Constraints: []constraint.NumericConstraint{constraint.GeConst{X: pathX, C: 6}}, // x > 5
		},
		{
			From:        branch2,
			To:          target,
			Constraints: []constraint.NumericConstraint{constraint.LeConst{X: pathX, C: 2}}, // x < 3
		},
	}

	s := Solve(inputs, testResolver())

	if !s.IsEdgeUnreachable(branch2, target) {
		t.Error("expected edge to be marked unreachable due to path contradiction (x >= 6 AND x <= 2)")
	}
}

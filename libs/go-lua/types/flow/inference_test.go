package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConstraintPropagation_SinglePath(t *testing.T) {
	// CFG: entry -> branch -> (true path) -> return
	//                     -> (false path) -> return
	g := cfg.New()
	branch := g.AddBranch(cfg.SymbolID(0), cfg.CondCheck{Kind: cfg.CheckNotNil})
	truePath := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	trueReturn := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")
	falsePath := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	falseReturn := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")

	g.AddEdge(g.Entry(), branch, false)
	g.AddEdge(branch, truePath, true)
	g.AddEdge(truePath, trueReturn, false)
	g.AddEdge(trueReturn, g.Exit(), false)
	g.AddEdge(branch, falsePath, false)
	g.AddEdge(falsePath, falseReturn, false)
	g.AddEdge(falseReturn, g.Exit(), false)

	// Edge constraints: true branch has NotNil(x)
	xPath := constraint.Path{Root: "x"}
	mock := newMockSSAGraph(g)
	allPoints := []cfg.Point{g.Entry(), branch, truePath, trueReturn, falsePath, falseReturn, g.Exit()}
	symX := setupSymbol(mock, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(mock, p, symX, verX)
	}

	inputs := newInputs(mock)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: truePath, Condition: constraint.FromConstraints(constraint.NotNil{Path: xPath})},
		{From: branch, To: falsePath, Condition: constraint.FromConstraints(constraint.IsNil{Path: xPath})},
	}

	solver := Solve(inputs, testResolver())

	// Check constraints at true return
	trueCond := solver.ConditionAt(trueReturn)
	if !trueCond.HasConstraints() {
		t.Errorf("expected constraints at true return")
	}

	found := false
	for _, c := range trueCond.AllConstraints() {
		if _, ok := c.(constraint.NotNil); ok {
			found = true
		}
	}

	if !found {
		t.Error("expected NotNil constraint at true return")
	}

	// Check constraints at false return
	falseCond := solver.ConditionAt(falseReturn)
	if !falseCond.HasConstraints() {
		t.Errorf("expected constraints at false return")
	}

	found = false
	for _, c := range falseCond.AllConstraints() {
		if _, ok := c.(constraint.IsNil); ok {
			found = true
		}
	}

	if !found {
		t.Error("expected IsNil constraint at false return")
	}
}

func TestConstraintPropagation_JoinIntersection(t *testing.T) {
	// CFG: entry -> branch -> path1 -> join -> return
	//                     -> path2 -> join
	// Both paths have NotNil(x), so join should have NotNil(x)
	g := cfg.New()
	branch := g.AddBranch(cfg.SymbolID(0), cfg.CondCheck{Kind: cfg.CheckTruthy})
	path1 := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	path2 := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := g.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")

	g.AddEdge(g.Entry(), branch, false)
	g.AddEdge(branch, path1, true)
	g.AddEdge(branch, path2, false)
	g.AddEdge(path1, join, false)
	g.AddEdge(path2, join, false)
	g.AddEdge(join, ret, false)
	g.AddEdge(ret, g.Exit(), false)

	// Both edges have NotNil(x)
	xPath := constraint.Path{Root: "x"}
	mock := newMockSSAGraph(g)
	allPoints := []cfg.Point{g.Entry(), branch, path1, path2, join, ret, g.Exit()}
	symX := setupSymbol(mock, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(mock, p, symX, verX)
	}

	inputs := newInputs(mock)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: path1, Condition: constraint.FromConstraints(constraint.NotNil{Path: xPath})},
		{From: branch, To: path2, Condition: constraint.FromConstraints(constraint.NotNil{Path: xPath})},
	}

	solver := Solve(inputs, testResolver())

	// At join, NotNil should be present (common to both paths via MustConstraints)
	joinCond := solver.ConditionAt(join)
	must := joinCond.MustConstraints()
	found := false

	for _, c := range must {
		if _, ok := c.(constraint.NotNil); ok {
			found = true
		}
	}

	if !found {
		t.Error("expected NotNil constraint at join (common to both paths)")
	}
}

func TestConstraintPropagation_JoinDifferentConstraints(t *testing.T) {
	// CFG: entry -> branch -> path1 -> join -> return
	//                     -> path2 -> join
	// path1 has NotNil(x), path2 has HasType(x, string)
	// Join should have no common constraints, but condition preserves both disjuncts
	g := cfg.New()
	branch := g.AddBranch(cfg.SymbolID(0), cfg.CondCheck{Kind: cfg.CheckTruthy})
	path1 := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	path2 := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := g.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")

	g.AddEdge(g.Entry(), branch, false)
	g.AddEdge(branch, path1, true)
	g.AddEdge(branch, path2, false)
	g.AddEdge(path1, join, false)
	g.AddEdge(path2, join, false)
	g.AddEdge(join, ret, false)
	g.AddEdge(ret, g.Exit(), false)

	xPath := constraint.Path{Root: "x"}
	mock := newMockSSAGraph(g)
	allPoints := []cfg.Point{g.Entry(), branch, path1, path2, join, ret, g.Exit()}
	symX := setupSymbol(mock, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(mock, p, symX, verX)
	}

	inputs := newInputs(mock)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: path1, Condition: constraint.FromConstraints(constraint.NotNil{Path: xPath})},
		{From: branch, To: path2, Condition: constraint.FromConstraints(constraint.HasType{Path: xPath, Type: narrow.BuiltinTypeKey("string")})},
	}

	solver := Solve(inputs, testResolver())

	// At join, MustConstraints should be empty (no common), but condition has 2 disjuncts
	joinCond := solver.ConditionAt(join)
	must := joinCond.MustConstraints()
	if len(must) != 0 {
		t.Errorf("expected 0 common constraints at join, got %d", len(must))
	}
	// DNF should preserve both paths
	if joinCond.NumDisjuncts() != 2 {
		t.Errorf("expected 2 disjuncts at join (OR of both paths), got %d", joinCond.NumDisjuncts())
	}
}

func TestConstraintPropagation_NestedBranches(t *testing.T) {
	// CFG: entry -> branch1 -> branch2 -> deepPath -> return
	// Constraints accumulate through nested branches
	g := cfg.New()
	branch1 := g.AddBranch(cfg.SymbolID(0), cfg.CondCheck{Kind: cfg.CheckNotNil})
	branch2 := g.AddBranch(cfg.SymbolID(0), cfg.CondCheck{Kind: cfg.CheckNotNil})
	deepPath := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")

	g.AddEdge(g.Entry(), branch1, false)
	g.AddEdge(branch1, branch2, true)
	g.AddEdge(branch2, deepPath, true)
	g.AddEdge(deepPath, ret, false)
	g.AddEdge(ret, g.Exit(), false)

	xPath := constraint.Path{Root: "x"}
	yPath := constraint.Path{Root: "y"}
	mock := newMockSSAGraph(g)
	allPoints := []cfg.Point{g.Entry(), branch1, branch2, deepPath, ret, g.Exit()}
	symX := setupSymbol(mock, "x", allPoints)
	symY := setupSymbol(mock, "y", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(mock, p, symX, verX)
		setVersion(mock, p, symY, verY)
	}

	inputs := newInputs(mock)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.DeclaredTypes[symY] = typ.Any
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch1, To: branch2, Condition: constraint.FromConstraints(constraint.NotNil{Path: xPath})},
		{From: branch2, To: deepPath, Condition: constraint.FromConstraints(constraint.NotNil{Path: yPath})},
	}

	solver := Solve(inputs, testResolver())

	// At return, should have both NotNil(x) and NotNil(y) as common constraints
	retCond := solver.ConditionAt(ret)
	allConstraints := retCond.AllConstraints()
	if len(allConstraints) < 2 {
		t.Errorf("expected at least 2 constraints at return, got %d", len(allConstraints))
	}

	hasNotNilX := false
	hasNotNilY := false

	for _, c := range allConstraints {
		if nn, ok := c.(constraint.NotNil); ok {
			if nn.Path.Root == "x" {
				hasNotNilX = true
			}

			if nn.Path.Root == "y" {
				hasNotNilY = true
			}
		}
	}

	if !hasNotNilX {
		t.Error("expected NotNil(x) at return")
	}

	if !hasNotNilY {
		t.Error("expected NotNil(y) at return")
	}
}

func TestConstraintPropagation_Loop(_ *testing.T) {
	// CFG: entry -> loopHeader -> loopBody -> loopHeader (back edge)
	//                         -> exit -> return
	// Constraints should stabilize via fixed-point
	g := cfg.New()
	header := g.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	body := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	exitBranch := g.AddBranch(cfg.SymbolID(0), cfg.CondCheck{Kind: cfg.CheckTruthy})
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")

	g.AddEdge(g.Entry(), header, false)
	g.AddEdge(header, exitBranch, false)
	g.AddEdge(exitBranch, body, false) // continue loop
	g.AddEdge(body, header, false)     // back edge
	g.AddEdge(exitBranch, ret, true)   // exit loop
	g.AddEdge(ret, g.Exit(), false)

	mock := newMockSSAGraph(g)
	inputs := newInputs(mock)

	// Should not panic or hang
	solver := Solve(inputs, testResolver())
	_ = solver.ConditionAt(ret)
}

func TestInferFunctionRefinement_Empty(t *testing.T) {
	g := cfg.New()
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")
	g.AddEdge(g.Entry(), ret, false)
	g.AddEdge(ret, g.Exit(), false)

	mock := newMockSSAGraph(g)
	inputs := &Inputs{Graph: mock}
	solver := Solve(inputs, testResolver())

	effect := InferFunctionRefinement(solver, g, nil, typ.Boolean)
	if effect != nil {
		t.Error("expected nil effect for empty constraints")
	}
}

func TestInferFunctionRefinement_NonBoolean(t *testing.T) {
	g := cfg.New()
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")
	g.AddEdge(g.Entry(), ret, false)
	g.AddEdge(ret, g.Exit(), false)

	mock := newMockSSAGraph(g)
	inputs := &Inputs{Graph: mock}
	solver := Solve(inputs, testResolver())

	// Non-boolean return type should not produce OnTrue/OnFalse
	effect := InferFunctionRefinement(solver, g, []ParamInfo{{Name: "x", Symbol: 100, Type: typ.Any}}, typ.String)
	if effect != nil {
		t.Error("expected nil effect for non-boolean return type")
	}
}

func TestFilterParamConstraints(t *testing.T) {
	// Use Symbol-based identity for parameter matching
	symX := cfg.SymbolID(100)
	symY := cfg.SymbolID(101)
	symZ := cfg.SymbolID(102)

	xPath := constraint.Path{Root: "x", Symbol: symX}
	yPath := constraint.Path{Root: "y", Symbol: symY}
	zPath := constraint.Path{Root: "z", Symbol: symZ}

	set := constraint.NewConjunction(
		constraint.NotNil{Path: xPath},
		constraint.NotNil{Path: yPath},
		constraint.NotNil{Path: zPath},
	)

	// Only x and y are parameters (Symbol-keyed)
	paramIndex := map[cfg.SymbolID]int{symX: 0, symY: 1}

	filtered := filterParamConstraints(set, paramIndex, nil)
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered constraints, got %d", len(filtered))
	}

	// z should be filtered out (not in paramIndex)
	for _, c := range filtered {
		if nn, ok := c.(constraint.NotNil); ok {
			if nn.Path.Symbol == symZ {
				t.Error("z should have been filtered out")
			}
		}
	}
}

func TestSubstituteToPlaceholders(t *testing.T) {
	// Use Symbol-based identity for parameter matching
	symX := cfg.SymbolID(100)
	symY := cfg.SymbolID(101)

	xPath := constraint.Path{Root: "x", Symbol: symX}
	yPath := constraint.Path{Root: "y", Symbol: symY, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "field"}}}

	set := constraint.NewConjunction(
		constraint.NotNil{Path: xPath},
		constraint.HasType{Path: yPath, Type: narrow.BuiltinTypeKey("string")},
	)

	// Symbol-keyed parameter index
	paramIndex := map[cfg.SymbolID]int{symX: 0, symY: 1}

	substituted := substituteToPlaceholders(set, paramIndex, nil)
	if len(substituted) != 2 {
		t.Errorf("expected 2 substituted constraints, got %d", len(substituted))
	}

	for _, c := range substituted {
		switch v := c.(type) {
		case constraint.NotNil:
			if v.Path.Root != "$0" {
				t.Errorf("expected $0, got %s", v.Path.Root)
			}
			// Placeholder paths have Symbol cleared
			if v.Path.Symbol != 0 {
				t.Errorf("expected Symbol=0 for placeholder, got %d", v.Path.Symbol)
			}
		case constraint.HasType:
			if v.Path.Root != "$1" {
				t.Errorf("expected $1, got %s", v.Path.Root)
			}
			if len(v.Path.Segments) != 1 || v.Path.Segments[0].Name != "field" {
				t.Error("expected .field segment preserved")
			}
			// Placeholder paths have Symbol cleared
			if v.Path.Symbol != 0 {
				t.Errorf("expected Symbol=0 for placeholder, got %d", v.Path.Symbol)
			}
		}
	}
}

func TestSubstitutePathsInConstraint_EqPath(t *testing.T) {
	// Use Symbol-based identity for parameter matching
	symX := cfg.SymbolID(100)
	symY := cfg.SymbolID(101)

	c := constraint.EqPath{
		Left:  constraint.Path{Root: "x", Symbol: symX},
		Right: constraint.Path{Root: "y", Symbol: symY},
	}

	// Symbol-keyed placeholder mapping
	placeholders := map[cfg.SymbolID]string{symX: "$0", symY: "$1"}
	result := substitutePathsInConstraint(c, placeholders, nil)

	eq, ok := result.(constraint.EqPath)
	if !ok {
		t.Fatal("expected EqPath")
	}

	// EqPath is canonicalized, so order might change
	if (eq.Left.Root != "$0" && eq.Left.Root != "$1") || (eq.Right.Root != "$0" && eq.Right.Root != "$1") {
		t.Errorf("expected $0 and $1, got %s and %s", eq.Left.Root, eq.Right.Root)
	}

	// Placeholder paths have Symbol cleared
	if eq.Left.Symbol != 0 || eq.Right.Symbol != 0 {
		t.Error("expected Symbol=0 for placeholder paths")
	}
}

func TestConditionAt_EntryIsTrue(t *testing.T) {
	g := cfg.New()
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")
	g.AddEdge(g.Entry(), ret, false)
	g.AddEdge(ret, g.Exit(), false)

	mock := newMockSSAGraph(g)
	inputs := &Inputs{Graph: mock}
	solver := Solve(inputs, testResolver())

	entryCond := solver.ConditionAt(g.Entry())
	if !entryCond.IsTrue() {
		t.Errorf("expected true condition at entry")
	}
}

func TestConstraintPropagation_ChainedEdges(t *testing.T) {
	// CFG: entry -> a -> b -> c -> return
	// Each edge adds a constraint
	g := cfg.New()
	a := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	b := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c := g.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	ret := g.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")

	g.AddEdge(g.Entry(), a, false)
	g.AddEdge(a, b, false)
	g.AddEdge(b, c, false)
	g.AddEdge(c, ret, false)
	g.AddEdge(ret, g.Exit(), false)

	xPath := constraint.Path{Root: "x"}
	yPath := constraint.Path{Root: "y"}
	zPath := constraint.Path{Root: "z"}

	mock := newMockSSAGraph(g)
	inputs := &Inputs{
		Graph: mock,
		EdgeConditions: []EdgeCondition{
			{From: g.Entry(), To: a, Condition: constraint.FromConstraints(constraint.NotNil{Path: xPath})},
			{From: a, To: b, Condition: constraint.FromConstraints(constraint.NotNil{Path: yPath})},
			{From: b, To: c, Condition: constraint.FromConstraints(constraint.NotNil{Path: zPath})},
		},
	}

	solver := Solve(inputs, testResolver())

	// At return, should have all three constraints (single path, all accumulated)
	retCond := solver.ConditionAt(ret)
	allConstraints := retCond.AllConstraints()
	if len(allConstraints) != 3 {
		t.Errorf("expected 3 constraints at return, got %d", len(allConstraints))
	}
}

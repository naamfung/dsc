package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFlow_MultiDisjunctCondition_Join(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	inputs.DeclaredTypes[symY] = typ.NewUnion(typ.String, typ.Number)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// Then branch: x is string
	// Else branch: y is number
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      branch,
			To:        elseNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathY, Type: narrow.BuiltinTypeKey("number")}),
		},
	}

	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(join)
	if len(cond.Disjuncts) != 2 {
		t.Fatalf("ConditionAt(join) should have 2 disjuncts, got %d", len(cond.Disjuncts))
	}
}

func TestFlow_MultiDisjunctCondition_DisjunctionPropagation(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// (x is string) OR (x is number)
	strCond := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})
	numCond := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")})
	disjunction := constraint.Or(strCond, numCond)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: disjunction,
		},
	}

	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(thenNode)
	if len(cond.Disjuncts) != 2 {
		t.Fatalf("disjunction should have 2 disjuncts at thenNode, got %d", len(cond.Disjuncts))
	}
}

func TestFlow_NarrowedTypeAt_UnionAcrossDisjuncts(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean, typ.Nil)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// (x is string) OR (x is number)
	strCond := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})
	numCond := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")})
	disjunction := constraint.Or(strCond, numCond)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: disjunction,
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}

	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Errorf("NarrowedTypeAt = %v, want %v", got, want)
	}
}

func TestFlow_NarrowedTypeAt_MustConstraintFromMultiDisjunct(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	optional := typ.NewOptional(typ.NewUnion(typ.String, typ.Number))
	inputs.DeclaredTypes[symX] = optional

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// (x is string AND x is notNil) OR (x is number AND x is notNil)
	// MustConstraint should be: x is notNil
	strNotNil := constraint.FromConstraints(
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")},
		constraint.NotNil{Path: pathX},
	)
	numNotNil := constraint.FromConstraints(
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")},
		constraint.NotNil{Path: pathX},
	)
	disjunction := constraint.Or(strNotNil, numNotNil)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: disjunction,
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}

	// Should narrow out nil, leaving string | number
	if u, ok := got.(*typ.Union); ok && u.Contains(typ.Nil) {
		t.Errorf("NarrowedTypeAt should not contain nil, got %v", got)
	} else if got == typ.Nil {
		t.Errorf("NarrowedTypeAt should not be nil, got %v", got)
	}
}

func TestFlow_DisjunctionJoin_ThreeBranches(t *testing.T) {
	// Build a CFG with three branches joining
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	node1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	node2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	node3 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, node1, true)
	c.AddEdge(branch1, branch2, false)
	c.AddEdge(branch2, node2, true)
	c.AddEdge(branch2, node3, false)
	c.AddEdge(node1, join, true)
	c.AddEdge(node2, join, true)
	c.AddEdge(node3, join, true)
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch1, node1, branch2, node2, node3, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch1,
			To:        node1,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      branch2,
			To:        node2,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")}),
		},
		{
			From:      branch2,
			To:        node3,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("boolean")}),
		},
	}

	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(join)
	if len(cond.Disjuncts) != 3 {
		t.Fatalf("join should have 3 disjuncts, got %d", len(cond.Disjuncts))
	}
}

func TestFlow_NestedDNF_AndInsideOr(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)
	inputs.DeclaredTypes[symY] = typ.NewUnion(typ.Boolean, typ.Nil)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// (x is string AND y is notNil) OR (x is number)
	branch1 := constraint.FromConstraints(
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")},
		constraint.NotNil{Path: pathY},
	)
	branch2 := constraint.FromConstraints(
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")},
	)
	disjunction := constraint.Or(branch1, branch2)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: disjunction,
		},
	}

	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(thenNode)
	if len(cond.Disjuncts) != 2 {
		t.Fatalf("condition should have 2 disjuncts, got %d", len(cond.Disjuncts))
	}
}

func TestFlow_DeMorgan_NegatedConditionPropagation(t *testing.T) {
	c, branch, _, elseNode, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, elseNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Then: x is string, Else: NOT(x is string) = x is notHasType string
	thenCond := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})
	elseCond := constraint.Not(thenCond)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        elseNode,
			Condition: elseCond,
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(elseNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}

	// Should narrow to number | boolean (not string)
	want := typ.NewUnion(typ.Number, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		t.Errorf("NarrowedTypeAt = %v, want %v", got, want)
	}
}

func TestFlow_ComplexDNF_FourDisjuncts(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)
	inputs.DeclaredTypes[symY] = typ.NewUnion(typ.Boolean, typ.Nil)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// (x is string OR x is number) AND (y is boolean OR y is nil)
	xStr := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})
	xNum := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")})
	yBool := constraint.FromConstraints(constraint.HasType{Path: pathY, Type: narrow.BuiltinTypeKey("boolean")})
	yNil := constraint.FromConstraints(constraint.IsNil{Path: pathY})

	xOr := constraint.Or(xStr, xNum)
	yOr := constraint.Or(yBool, yNil)
	combined := constraint.And(xOr, yOr)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: combined,
		},
	}

	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(thenNode)
	if len(cond.Disjuncts) != 4 {
		t.Fatalf("(A OR B) AND (C OR D) should produce 4 disjuncts, got %d", len(cond.Disjuncts))
	}
}

func TestFlow_DisjunctUnionType_AtJoin(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      branch,
			To:        elseNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(join, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt at join returned nil")
	}

	// At join, type should be string | number (union of both branches)
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Errorf("NarrowedTypeAt(join) = %v, want %v", got, want)
	}
}

func TestFlow_MustConstraints_PropagatedToNarrowing(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.NewUnion(typ.String, typ.Number))

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Both disjuncts have NotNil, so it's a must constraint
	d1 := constraint.FromConstraints(
		constraint.NotNil{Path: pathX},
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")},
	)
	d2 := constraint.FromConstraints(
		constraint.NotNil{Path: pathX},
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")},
	)
	disjunction := constraint.Or(d1, d2)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: disjunction,
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}

	if u, ok := got.(*typ.Union); ok && u.Contains(typ.Nil) {
		t.Errorf("must constraint NotNil should remove nil, got %v", got)
	} else if got == typ.Nil {
		t.Errorf("must constraint NotNil should remove nil, got %v", got)
	}
}

func TestFlow_LoopWithDNFCondition(t *testing.T) {
	c, header, body := buildLoopCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), header, body, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Loop body condition: (x is string) OR (x is number)
	strCond := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})
	numCond := constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")})
	disjunction := constraint.Or(strCond, numCond)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      header,
			To:        body,
			Condition: disjunction,
		},
	}

	s := Solve(inputs, testResolver())

	iterations := s.DebugIterations()
	maxExpected := c.Size() * 5
	if iterations > maxExpected {
		t.Errorf("too many iterations: got %d, expected <= %d", iterations, maxExpected)
	}

	got := s.NarrowedTypeAt(body, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt(body) returned nil")
	}

	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Errorf("NarrowedTypeAt(body) = %v, want %v", got, want)
	}
}

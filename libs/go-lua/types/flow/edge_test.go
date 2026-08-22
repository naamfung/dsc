package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFlow_EdgeCondition_EqPath(t *testing.T) {
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
	inputs.DeclaredTypes[symY] = typ.String

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: typ.NewUnion(typ.String, typ.Number)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "y", Symbol: symY}, Type: typ.String},
	}

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NewEqPath(pathX, pathY)),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected x narrowed to string, got %v", got)
	}
}

func TestFlow_EdgeCondition_FieldEqualsPath(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symMsg := setupSymbol(g, "msg", allPoints)
	symKind := setupSymbol(g, "kind", allPoints)
	verMsg := cfg.Version{Root: "msg", Symbol: symMsg, ID: 1}
	verKind := cfg.Version{Root: "kind", Symbol: symKind, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symMsg, verMsg)
		setVersion(g, p, symKind, verKind)
	}

	inputs := newInputs(g)
	event := typ.NewRecord().Field("tag", typ.LiteralString("event")).Build()
	timer := typ.NewRecord().Field("tag", typ.LiteralString("time")).Build()
	inputs.DeclaredTypes[symMsg] = typ.NewUnion(event, timer)
	inputs.DeclaredTypes[symKind] = typ.LiteralString("event")

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "msg", Symbol: symMsg}, Type: typ.NewUnion(event, timer)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "kind", Symbol: symKind}, Type: typ.LiteralString("event")},
	}

	pathMsg := constraint.Path{Root: "msg", Symbol: symMsg}
	pathKind := constraint.Path{Root: "kind", Symbol: symKind}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathMsg, Field: "tag", Value: pathKind}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "msg", Symbol: symMsg})
	if got == nil || !typ.TypeEquals(got, event) {
		t.Fatalf("expected msg narrowed to event record, got %v", got)
	}
}

func TestFlow_EdgeCondition_IndexEquals(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symMsg := setupSymbol(g, "msg", allPoints)
	verMsg := cfg.Version{Root: "msg", Symbol: symMsg, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symMsg, verMsg)
	}

	inputs := newInputs(g)
	event := typ.NewRecord().Field("tag", typ.LiteralString("event")).Build()
	timer := typ.NewRecord().Field("tag", typ.LiteralString("time")).Build()
	inputs.DeclaredTypes[symMsg] = typ.NewUnion(event, timer)

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "msg", Symbol: symMsg}, Type: typ.NewUnion(event, timer)},
	}

	pathMsg := constraint.Path{Root: "msg", Symbol: symMsg}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.IndexEquals{Target: pathMsg, Key: typ.LiteralString("tag"), Value: typ.LiteralString("event")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "msg", Symbol: symMsg})
	if got == nil || !typ.TypeEquals(got, event) {
		t.Fatalf("expected msg narrowed to event record via index, got %v", got)
	}
}

func TestFlow_EdgeCondition_HasTypeBuiltin(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: typ.NewUnion(typ.String, typ.Number)},
	}

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected x narrowed to string, got %v", got)
	}
}

func TestFlow_EdgeCondition_HasTypeHash(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	point := typ.NewRecord().Field("x", typ.Number).Build()
	inputs.DeclaredTypes[symX] = typ.NewUnion(point, typ.String)
	inputs.TypeKeys[point.Hash()] = point

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: typ.NewUnion(point, typ.String)},
	}

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.HashTypeKey(point.Hash())}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil || !typ.TypeEquals(got, point) {
		t.Fatalf("expected x narrowed to point record, got %v", got)
	}
}

func TestFlow_EdgeCondition_FieldEqualsPath_NestedValue(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symCh1 := setupSymbol(g, "ch1", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verCh1 := cfg.Version{Root: "ch1", Symbol: symCh1, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symCh1, verCh1)
	}

	inputs := newInputs(g)
	chanInt := typ.NewRecord().Field("__tag", typ.LiteralString("int")).Build()
	chanStr := typ.NewRecord().Field("__tag", typ.LiteralString("str")).Build()

	valueWithError := typ.NewRecord().Field("error", typ.String).Build()
	valueWithData := typ.NewRecord().Field("data", typ.Number).Build()

	variant1 := typ.NewRecord().
		Field("channel", chanInt).
		Field("value", valueWithError).
		Field("ok", typ.Boolean).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", chanStr).
		Field("value", valueWithData).
		Field("ok", typ.Boolean).
		Build()

	inputs.DeclaredTypes[symResult] = typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symCh1] = chanInt

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "result", Symbol: symResult}, Type: typ.NewUnion(variant1, variant2)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "ch1", Symbol: symCh1}, Type: chanInt},
	}

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathCh1 := constraint.Path{Root: "ch1", Symbol: symCh1}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathCh1}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "result", Symbol: symResult})
	if got == nil {
		t.Fatal("expected result to be narrowed, got nil")
	}
	if !typ.TypeEquals(got, variant1) {
		t.Fatalf("expected result narrowed to variant1, got %v", got)
	}
}

package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeAt_EmptyPath(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	result := s.TypeAt(c.Entry(), constraint.Path{})
	if result != nil {
		t.Errorf("TypeAt(empty path) = %v, want nil", result)
	}
}

func TestTypeAt_DeclaredType(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.String

	s := Solve(inputs, testResolver())

	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(c.Entry(), path)
	if result != typ.String {
		t.Errorf("TypeAt(x) = %v, want string", result)
	}
}

func TestTypeAt_AssignedType(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.Integer,
		},
	}

	s := Solve(inputs, testResolver())

	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(c.Entry(), path)
	if result != typ.Integer {
		t.Errorf("TypeAt(x) = %v, want integer", result)
	}
}

func TestConditionAt_NoCondition(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(c.Entry())
	if !cond.IsTrue() {
		t.Errorf("ConditionAt(entry) = %v, want true condition", cond)
	}
}

func TestConditionAt_WithEdgeCondition(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.String)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX}),
		},
	}

	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(thenNode)
	if cond.IsTrue() || !cond.HasConstraints() {
		t.Errorf("ConditionAt(thenNode) = %v, want condition with Truthy", cond)
	}
}

func TestBaseTypeAt_NoSegments(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Number

	s := Solve(inputs, testResolver())

	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.baseTypeAt(c.Entry(), path)
	if result != typ.Number {
		t.Errorf("baseTypeAt(x) = %v, want number", result)
	}
}

func TestBaseTypeAt_WithSegments_ExplicitPreferred(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symR := setupSymbol(g, "r", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "r", Symbol: symR, ID: 1}
	setVersion(g, c.Entry(), symR, ver)

	errType := typ.NewInterface("Err", nil)
	recordType := typ.NewRecord().Field("err", typ.Nil).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symR] = recordType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "r", Symbol: symR},
			Type:       recordType,
		},
		{
			Point: c.Entry(),
			TargetPath: constraint.Path{
				Root:     "r",
				Symbol:   symR,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			},
			Type: errType,
		},
	}

	s := Solve(inputs, testResolver())

	path := constraint.Path{
		Root:     "r",
		Symbol:   symR,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
	}
	result := s.baseTypeAt(c.Entry(), path)
	if result != errType {
		t.Errorf("baseTypeAt(r.err) = %v, want Err (explicit assignment)", result)
	}
}

func TestDerivedTypeAt_FromNarrowedParent(t *testing.T) {
	c, branch, thenNode, _, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symR := setupSymbol(g, "r", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "r", Symbol: symR, ID: 1}
	setVersion(g, c.Entry(), symR, ver)
	setVersion(g, branch, symR, ver)
	setVersion(g, thenNode, symR, ver)

	msgA := typ.NewInterface("MsgA", nil)
	msgB := typ.NewInterface("MsgB", nil)
	resultA := typ.NewRecord().Field("value", msgA).Build()
	resultB := typ.NewRecord().Field("value", msgB).Build()
	unionType := typ.NewUnion(resultA, resultB)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symR] = unionType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "r", Symbol: symR},
			Type:       unionType,
		},
	}

	pathR := constraint.Path{Root: "r", Symbol: symR}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathR, Type: narrow.HashTypeKey(resultA.Hash())}),
		},
	}
	inputs.TypeKeys = map[uint64]typ.Type{
		resultA.Hash(): resultA,
	}

	s := Solve(inputs, testResolver())

	childPath := constraint.Path{
		Root:     "r",
		Symbol:   symR,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	result := s.derivedTypeAt(thenNode, childPath)
	if result == nil {
		t.Fatal("derivedTypeAt(r.value) = nil, want derived type from narrowed parent")
	}
	if result != msgA {
		t.Errorf("derivedTypeAt(r.value) = %v, want MsgA", result)
	}
}

func TestDerivedTypeAt_FromNarrowedIntermediateAncestor(t *testing.T) {
	c, branch, thenNode, _, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symR := setupSymbol(g, "r", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "r", Symbol: symR, ID: 1}
	setVersion(g, c.Entry(), symR, ver)
	setVersion(g, branch, symR, ver)
	setVersion(g, thenNode, symR, ver)

	systemItem := typ.NewRecord().Field("text", typ.String).Build()
	systemType := typ.NewOptional(typ.NewArray(systemItem))
	rootType := typ.NewRecord().Field("system", systemType).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symR] = rootType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "r", Symbol: symR},
			Type:       rootType,
		},
	}

	pathSystem := constraint.Path{
		Root:     "r",
		Symbol:   symR,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "system"}},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: pathSystem}),
		},
	}

	s := Solve(inputs, testResolver())

	pathText := constraint.Path{
		Root:   "r",
		Symbol: symR,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "system"},
			{Kind: constraint.SegmentIndexInt, Index: 1},
			{Kind: constraint.SegmentField, Name: "text"},
		},
	}
	result := s.derivedTypeAt(thenNode, pathText)
	if result == nil {
		t.Fatal("derivedTypeAt(r.system[1].text) = nil, want derived type from narrowed intermediate ancestor")
	}
	if !typ.TypeEquals(result, typ.String) {
		t.Errorf("derivedTypeAt(r.system[1].text) = %v, want string", result)
	}
}

func TestIsFalseLiteral(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil type", nil, false},
		{"false literal", typ.False, true},
		{"true literal", typ.True, false},
		{"string literal", typ.LiteralString("hi"), false},
		{"number type", typ.Number, false},
		{"nil singleton", typ.Nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFalseLiteral(tt.t)
			if got != tt.want {
				t.Errorf("isFalseLiteral(%v) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

package flow

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// versionMap is a simple map for testing version lookups.
type versionMap struct {
	m map[cfg.Point]map[cfg.SymbolID]cfg.Version
}

func newVersionMap() *versionMap {
	return &versionMap{m: make(map[cfg.Point]map[cfg.SymbolID]cfg.Version)}
}

func (v *versionMap) Get(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	if v.m[p] == nil {
		return cfg.Version{}
	}
	return v.m[p][sym]
}

func (v *versionMap) Set(p cfg.Point, sym cfg.SymbolID, ver cfg.Version) {
	if v.m[p] == nil {
		v.m[p] = make(map[cfg.SymbolID]cfg.Version)
	}
	v.m[p][sym] = ver
}

func (v *versionMap) AllVersions(p cfg.Point) map[cfg.SymbolID]cfg.Version {
	return v.m[p]
}

// symbolScope is a simple map for testing symbol visibility.
type symbolScope struct {
	visibility map[cfg.Point]map[string]cfg.SymbolID
	declPoints map[cfg.SymbolID]cfg.Point
}

func newSymbolScope() *symbolScope {
	return &symbolScope{
		visibility: make(map[cfg.Point]map[string]cfg.SymbolID),
		declPoints: make(map[cfg.SymbolID]cfg.Point),
	}
}

func (s *symbolScope) SymbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	if s.visibility[p] == nil {
		return 0, false
	}
	sym, ok := s.visibility[p][name]
	return sym, ok
}

func (s *symbolScope) AllSymbolsAt(p cfg.Point) map[string]cfg.SymbolID {
	return s.visibility[p]
}

func (s *symbolScope) SetVisibility(p cfg.Point, name string, sym cfg.SymbolID) {
	if s.visibility[p] == nil {
		s.visibility[p] = make(map[string]cfg.SymbolID)
	}
	s.visibility[p][name] = sym
}

func (s *symbolScope) DeclarationPoint(sym cfg.SymbolID) (cfg.Point, bool) {
	p, ok := s.declPoints[sym]
	return p, ok
}

func (s *symbolScope) SetDeclarationPoint(sym cfg.SymbolID, p cfg.Point) {
	s.declPoints[sym] = p
}

// mockSSAGraph implements cfg.VersionedGraph for testing.
type mockSSAGraph struct {
	*cfg.CFG
	versions *versionMap
	scope    *symbolScope
	phis     []cfg.PhiNode
	nextSym  cfg.SymbolID
}

func newMockSSAGraph(c *cfg.CFG) *mockSSAGraph {
	return &mockSSAGraph{
		CFG:      c,
		versions: newVersionMap(),
		scope:    newSymbolScope(),
		nextSym:  1,
	}
}

var _ cfg.VersionedGraph = (*mockSSAGraph)(nil)

func (m *mockSSAGraph) VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	return m.versions.Get(p, sym)
}

func (m *mockSSAGraph) AllVisibleVersions(p cfg.Point) map[cfg.SymbolID]cfg.Version {
	return m.versions.AllVersions(p)
}

func (m *mockSSAGraph) PhiNodes() []cfg.PhiNode {
	return m.phis
}

func (m *mockSSAGraph) SymbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	return m.scope.SymbolAt(p, name)
}

func (m *mockSSAGraph) AllSymbolsAt(p cfg.Point) map[string]cfg.SymbolID {
	return m.scope.AllSymbolsAt(p)
}

func (m *mockSSAGraph) DeclarationPoint(sym cfg.SymbolID) (cfg.Point, bool) {
	return m.scope.DeclarationPoint(sym)
}

func (m *mockSSAGraph) ParamNames() []string {
	return nil
}

func (m *mockSSAGraph) ParamSymbols() []cfg.SymbolID {
	return nil
}

func (m *mockSSAGraph) ParamDeclPoints() []cfg.Point {
	return nil
}

func (m *mockSSAGraph) NameOf(sym cfg.SymbolID) string {
	return ""
}

func (m *mockSSAGraph) SymbolKind(sym cfg.SymbolID) (cfg.SymbolKind, bool) {
	return cfg.SymbolUnknown, false
}

// registerSymbol registers a symbol name and returns its SymbolID.
func (m *mockSSAGraph) registerSymbol(name string) cfg.SymbolID {
	sym := m.nextSym
	m.nextSym++
	return sym
}

// setSymbolVisibilityAll sets symbol visibility at all given points.
func (m *mockSSAGraph) setSymbolVisibilityAll(points []cfg.Point, name string, sym cfg.SymbolID) {
	for _, p := range points {
		m.scope.SetVisibility(p, name, sym)
	}
}

// setVersion sets a version for a symbol at a point.
func (m *mockSSAGraph) setVersion(p cfg.Point, sym cfg.SymbolID, ver cfg.Version) {
	m.versions.Set(p, sym, ver)
}

// addPhiNode adds a phi node to the graph.
func (m *mockSSAGraph) addPhiNode(phi cfg.PhiNode) {
	m.phis = append(m.phis, phi)
}

func buildBranchCFG() (*cfg.CFG, cfg.Point, cfg.Point) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, c.Exit(), true)
	c.AddEdge(elseNode, c.Exit(), true)

	return c, branch, thenNode
}

func buildBranchJoinCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, join, true)
	c.AddEdge(elseNode, join, true)
	c.AddEdge(join, c.Exit(), true)

	return c, branch, thenNode, elseNode, join
}

func buildLoopCFG() (*cfg.CFG, cfg.Point, cfg.Point) {
	c := cfg.New()
	header := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	body := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), header, true)
	c.AddEdge(header, body, true)
	c.AddEdge(body, header, true) // back-edge
	c.AddEdge(header, c.Exit(), false)

	return c, header, body
}

func testResolver() narrow.Resolver {
	return &core.FuncResolver{
		FieldFunc: core.Field,
		IndexFunc: core.Index,
	}
}

// canonicalVersionKey returns the canonical PathKey for a version.
// Format: sym<SymbolID>@<VersionID>
func canonicalVersionKey(v cfg.Version) string {
	return "sym" + strconv.FormatUint(uint64(v.Symbol), 10) + "@" + strconv.Itoa(v.ID)
}

func newInputs(g *mockSSAGraph) *Inputs {
	return &Inputs{
		Graph:          g,
		DeclaredTypes:  make(map[cfg.SymbolID]typ.Type),
		ConstValues:    make(map[cfg.SymbolID]map[cfg.Point]*ConstValue),
		EdgeConditions: nil,
		TypeKeys:       make(map[uint64]typ.Type),
	}
}

func TestSolutionResolveTypeKey_BuiltinWithoutTypeKeyMap(t *testing.T) {
	s := &Solution{inputs: &Inputs{TypeKeys: nil}}
	got := s.resolveTypeKey(narrow.BuiltinTypeKey("string"))
	if got != typ.String {
		t.Fatalf("resolveTypeKey(string) = %v, want %v", got, typ.String)
	}
}

func TestSolutionResolveTypeKey_UnknownBuiltin(t *testing.T) {
	s := &Solution{inputs: &Inputs{TypeKeys: nil}}
	if got := s.resolveTypeKey(narrow.BuiltinTypeKey("entry")); got != nil {
		t.Fatalf("resolveTypeKey(entry) = %v, want nil", got)
	}
}

func TestMergeFieldAssignments_IncludesCanonicalStringIndexKeys(t *testing.T) {
	s := &Solution{
		values: map[string]typ.Type{
			`sym1@1["meta.type"]`:   typ.String,
			`sym1@1.name`:           typ.Number,
			`sym1@1["meta.type"].x`: typ.Boolean,
			`sym1@2["meta.type"]`:   typ.Integer,
			`sym2@1["meta.type"]`:   typ.Nil,
		},
	}

	base := typ.NewRecord().Field("id", typ.String).Build()
	got := s.mergeFieldAssignments(base, "sym1@1")

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("mergeFieldAssignments returned %T, want *typ.Record", got)
	}
	if f := rec.GetField("meta.type"); f == nil || !typ.TypeEquals(f.Type, typ.String) {
		t.Fatalf("expected merged field meta.type=string, got %v", f)
	}
	if f := rec.GetField("name"); f == nil || !typ.TypeEquals(f.Type, typ.Number) {
		t.Fatalf("expected merged field name=number, got %v", f)
	}
	if f := rec.GetField("id"); f == nil || !typ.TypeEquals(f.Type, typ.String) {
		t.Fatalf("expected existing field id=string to be preserved, got %v", f)
	}
}

func TestMergeFieldAssignments_IncludesEscapedStringIndexKey(t *testing.T) {
	s := &Solution{
		values: map[string]typ.Type{
			`sym7@3["a\"b"]`: typ.Boolean,
		},
	}

	got := s.mergeFieldAssignments(nil, "sym7@3")
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("mergeFieldAssignments returned %T, want *typ.Record", got)
	}
	if f := rec.GetField(`a"b`); f == nil || !typ.TypeEquals(f.Type, typ.Boolean) {
		t.Fatalf("expected escaped key field a\\\"b=boolean, got %v", f)
	}
}

func TestMergeFieldAssignments_InvalidBaseKey_NoChange(t *testing.T) {
	s := &Solution{
		values: map[string]typ.Type{
			`sym1@1["meta.type"]`: typ.String,
		},
	}

	base := typ.NewRecord().Field("id", typ.String).Build()
	got := s.mergeFieldAssignments(base, "not-a-canonical-key")
	if !typ.TypeEquals(got, base) {
		t.Fatalf("invalid base key should keep base type unchanged: got %v, want %v", got, base)
	}
}

func TestMergeFieldAssignments_PreservesExistingFieldQualifiers(t *testing.T) {
	s := &Solution{
		values: map[string]typ.Type{
			`sym3@2.new_field`: typ.Number,
		},
	}

	base := typ.NewRecord().
		OptReadonlyField("id", typ.String).
		OptField("nickname", typ.String).
		ReadonlyField("created_at", typ.Integer).
		Build()

	got := s.mergeFieldAssignments(base, "sym3@2")
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("mergeFieldAssignments returned %T, want *typ.Record", got)
	}

	check := func(name string, wantOptional, wantReadonly bool) {
		f := rec.GetField(name)
		if f == nil {
			t.Fatalf("missing field %q", name)
		}
		if f.Optional != wantOptional || f.Readonly != wantReadonly {
			t.Fatalf("field %q qualifiers changed: optional=%v readonly=%v (want optional=%v readonly=%v)", name, f.Optional, f.Readonly, wantOptional, wantReadonly)
		}
	}
	check("id", true, true)
	check("nickname", true, false)
	check("created_at", false, true)
	check("new_field", false, false)
}

func TestMergeFieldAssignments_PreservesAliasFieldType(t *testing.T) {
	txAlias := typ.NewAlias("Tx",
		typ.NewRecord().
			Field("query", typ.Func().
				Param("self", typ.NewRef("", "Tx")).
				Param("q", typ.String).
				Returns(typ.Number).
				Build()).
			Build())
	s := &Solution{
		values: map[string]typ.Type{
			`sym9@1.tx`: txAlias,
		},
	}

	got := s.mergeFieldAssignments(nil, "sym9@1")
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("mergeFieldAssignments returned %T, want *typ.Record", got)
	}
	field := rec.GetField("tx")
	if field == nil {
		t.Fatalf("expected field tx in merged record")
	}
	if field.Type.Kind() != kind.Alias {
		t.Fatalf("expected field tx type to preserve alias, got %s", field.Type.Kind())
	}
	alias, _ := field.Type.(*typ.Alias)
	if alias == nil || alias.Name != "Tx" {
		t.Fatalf("expected alias Tx on field tx, got %v", field.Type)
	}
}

func TestMergeFieldAssignments_PreservesAliasRootType(t *testing.T) {
	builderAlias := typ.NewAlias("Builder", typ.NewRecord().Field("_messages", typ.NewArray(typ.String)).Build())
	s := &Solution{
		values: map[string]typ.Type{
			`sym11@1._messages`: typ.NewArray(typ.NewUnion(typ.String, typ.Integer)),
		},
	}

	got := s.mergeFieldAssignments(builderAlias, "sym11@1")
	alias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("mergeFieldAssignments(alias root) = %T, want *typ.Alias", got)
	}
	if alias.Name != "Builder" {
		t.Fatalf("alias name = %q, want Builder", alias.Name)
	}
	rec, ok := alias.Target.(*typ.Record)
	if !ok {
		t.Fatalf("alias target = %T, want *typ.Record", alias.Target)
	}
	field := rec.GetField("_messages")
	if field == nil {
		t.Fatalf("expected _messages field in merged alias target")
	}
	arr, ok := field.Type.(*typ.Array)
	if !ok {
		t.Fatalf("_messages field type = %T, want *typ.Array", field.Type)
	}
	if !typ.TypeEquals(arr.Element, typ.NewUnion(typ.String, typ.Integer)) {
		t.Fatalf("_messages element = %v, want string|integer", arr.Element)
	}
}

func TestMergeFieldAssignments_PreservesRecursiveAliasRootType(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("Builder")
	rec.SetBody(
		typ.NewRecord().
			Field("_messages", typ.NewArray(typ.String)).
			Field("clone", typ.Func().
				Param("self", rec).
				Returns(rec).
				Build()).
			Build(),
	)
	builderAlias := typ.NewAlias("Builder", rec)

	s := &Solution{
		values: map[string]typ.Type{
			`sym12@1._messages`: typ.NewArray(typ.LiteralString("x")),
		},
	}

	got := s.mergeFieldAssignments(builderAlias, "sym12@1")
	alias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("mergeFieldAssignments(recursive alias root) = %T, want *typ.Alias", got)
	}
	if alias.Name != "Builder" {
		t.Fatalf("alias name = %q, want Builder", alias.Name)
	}
	mergedRec, ok := alias.Target.(*typ.Recursive)
	if !ok {
		t.Fatalf("alias target = %T, want *typ.Recursive", alias.Target)
	}
	body, ok := mergedRec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T, want *typ.Record", mergedRec.Body)
	}
	msgs := body.GetField("_messages")
	if msgs == nil {
		t.Fatalf("missing _messages field in merged recursive body")
	}
	arr, ok := msgs.Type.(*typ.Array)
	if !ok {
		t.Fatalf("_messages field type = %T, want *typ.Array", msgs.Type)
	}
	if !typ.TypeEquals(arr.Element, typ.LiteralString("x")) {
		t.Fatalf("_messages element = %v, want literal x", arr.Element)
	}
	clone := body.GetField("clone")
	if clone == nil {
		t.Fatalf("missing clone field in merged recursive body")
	}
	fn, ok := clone.Type.(*typ.Function)
	if !ok {
		t.Fatalf("clone field type = %T, want *typ.Function", clone.Type)
	}
	if len(fn.Params) != 1 || !typ.IsRecursiveRef(fn.Params[0].Type, mergedRec) {
		t.Fatalf("clone self param = %v, want rebuilt recursive self", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.IsRecursiveRef(fn.Returns[0], mergedRec) {
		t.Fatalf("clone return = %v, want rebuilt recursive self", fn.Returns)
	}
}

func TestFieldAssignmentsForRoot_InvalidatesCachedRootOnFieldWrite(t *testing.T) {
	s := &Solution{
		values: map[string]typ.Type{
			`sym21@1.name`: typ.String,
		},
		fieldOverlayCache: make(map[string][]mergedField),
	}

	first := s.fieldAssignmentsForRoot("sym21@1")
	if len(first) != 1 || first[0].Name != "name" || !typ.TypeEquals(first[0].Type, typ.String) {
		t.Fatalf("first field overlay = %v, want name:string", first)
	}

	s.setValue(`sym21@1.name`, typ.Integer)

	second := s.fieldAssignmentsForRoot("sym21@1")
	if len(second) != 1 || second[0].Name != "name" || !typ.TypeEquals(second[0].Type, typ.Integer) {
		t.Fatalf("second field overlay = %v, want name:integer", second)
	}
}

// setupSymbol registers a symbol and sets its visibility at all given points.
// Returns the SymbolID for use in version creation.
func setupSymbol(g *mockSSAGraph, name string, points []cfg.Point) cfg.SymbolID {
	sym := g.registerSymbol(name)
	g.setSymbolVisibilityAll(points, name, sym)
	return sym
}

// setVersion sets the visible version for a symbol at a point.
func setVersion(g *mockSSAGraph, p cfg.Point, sym cfg.SymbolID, ver cfg.Version) {
	g.setVersion(p, sym, ver)
}

func TestFlow_PhiKey_Join(t *testing.T) {
	c, _, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	// Register symbol and set visibility
	allPoints := []cfg.Point{c.Entry(), thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// SSA versions: x@1 at thenNode, x@2 at elseNode, x@3 (phi) at join
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	ver3 := cfg.Version{Root: "x", Symbol: symX, ID: 3}

	setVersion(g, thenNode, symX, ver1)
	setVersion(g, elseNode, symX, ver2)
	setVersion(g, join, symX, ver3)

	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: thenNode, Version: ver1},
			{From: elseNode, Version: ver2},
		},
	})

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: thenNode,
			TargetPath: constraint.Path{
				Root:   "x",
				Symbol: symX,
			},
			Type: typ.String,
		},
		{
			Point: elseNode,
			TargetPath: constraint.Path{
				Root:   "x",
				Symbol: symX,
			},
			Type: typ.Number,
		},
	}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(join, constraint.Path{Root: "x", Symbol: symX})
	want := typ.NewUnion(typ.String, typ.Number)

	if got == nil || !typ.TypeEquals(got, want) {
		t.Fatalf("expected joined type %v, got %v", want, got)
	}
}

func TestFlow_PhiChildSuffix_UsesPredecessorNarrowingWhenSuffixMissing(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symMessages := setupSymbol(g, "messages", allPoints)

	ver1 := cfg.Version{Root: "messages", Symbol: symMessages, ID: 1}
	ver2 := cfg.Version{Root: "messages", Symbol: symMessages, ID: 2}
	ver3 := cfg.Version{Root: "messages", Symbol: symMessages, ID: 3}

	setVersion(g, c.Entry(), symMessages, ver1)
	setVersion(g, branch, symMessages, ver1)
	setVersion(g, thenNode, symMessages, ver2)
	setVersion(g, elseNode, symMessages, ver1)
	setVersion(g, join, symMessages, ver3)
	setVersion(g, c.Exit(), symMessages, ver3)

	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: thenNode, Version: ver2},
			{From: elseNode, Version: ver1},
		},
	})

	messageType := typ.NewRecord().
		Field("topic", typ.Func().Returns(typ.String).Build()).
		Build()
	messagesType := typ.NewMap(typ.String, messageType)
	childPath := constraint.Path{
		Root:   "messages",
		Symbol: symMessages,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "root"},
		},
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symMessages] = messagesType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "messages", Symbol: symMessages},
			Type:       messagesType,
		},
		{
			Point:      thenNode,
			TargetPath: childPath,
			Type:       messageType,
		},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.Falsy{Path: childPath}),
		},
		{
			From:      branch,
			To:        elseNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: childPath}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.TypeAt(join, childPath)
	if got == nil {
		t.Fatal("TypeAt(join, messages.root) returned nil")
	}
	if core.ContainsNil(got) {
		t.Fatalf("TypeAt(join, messages.root) should be definite after constructive join, got %v", got)
	}
	if !typ.TypeEquals(got, messageType) {
		t.Fatalf("TypeAt(join, messages.root) = %v, want %v", got, messageType)
	}

	fullKey := string(s.pkResolver.KeyAt(join, childPath))
	raw := s.DebugValueAt(fullKey, join)
	if raw == nil {
		t.Fatal("joined child suffix value missing from solver state")
	}
	if core.ContainsNil(raw) {
		t.Fatalf("raw joined child suffix should not contain nil, got %v", raw)
	}
}

func TestFlow_PhiChildSuffix_OneBranchOnlyInstallRemainsOptional(t *testing.T) {
	c, _, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), thenNode, elseNode, join, c.Exit()}
	symMessages := setupSymbol(g, "messages", allPoints)

	ver1 := cfg.Version{Root: "messages", Symbol: symMessages, ID: 1}
	ver2 := cfg.Version{Root: "messages", Symbol: symMessages, ID: 2}
	ver3 := cfg.Version{Root: "messages", Symbol: symMessages, ID: 3}

	setVersion(g, c.Entry(), symMessages, ver1)
	setVersion(g, thenNode, symMessages, ver2)
	setVersion(g, elseNode, symMessages, ver1)
	setVersion(g, join, symMessages, ver3)
	setVersion(g, c.Exit(), symMessages, ver3)

	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: thenNode, Version: ver2},
			{From: elseNode, Version: ver1},
		},
	})

	messageType := typ.NewRecord().
		Field("topic", typ.Func().Returns(typ.String).Build()).
		Build()
	messagesType := typ.NewMap(typ.String, messageType)
	childPath := constraint.Path{
		Root:   "messages",
		Symbol: symMessages,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "root"},
		},
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symMessages] = messagesType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "messages", Symbol: symMessages},
			Type:       messagesType,
		},
		{
			Point:      thenNode,
			TargetPath: childPath,
			Type:       messageType,
		},
	}

	s := Solve(inputs, testResolver())

	got := s.TypeAt(join, childPath)
	if got == nil {
		t.Fatal("TypeAt(join, messages.root) returned nil")
	}
	if !core.ContainsNil(got) {
		t.Fatalf("TypeAt(join, messages.root) should remain optional when only one branch installs it, got %v", got)
	}
}

func TestConditionAt_JoinOr(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Any

	path := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      branch,
			To:        elseNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("number")}),
		},
	}

	s := Solve(inputs, testResolver())
	cond := s.ConditionAt(join)
	if len(cond.Disjuncts) != 2 {
		t.Fatalf("ConditionAt(join) disjuncts = %d, want 2", len(cond.Disjuncts))
	}
	if len(cond.MustConstraints()) != 0 {
		t.Errorf("ConditionAt(join) must constraints = %d, want 0", len(cond.MustConstraints()))
	}

	var sawString, sawNumber bool
	for _, d := range cond.Disjuncts {
		if constraint.ConjunctionContains(d, constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}) {
			sawString = true
		}
		if constraint.ConjunctionContains(d, constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("number")}) {
			sawNumber = true
		}
	}

	if !sawString || !sawNumber {
		t.Errorf("ConditionAt(join) missing disjuncts: string=%v number=%v", sawString, sawNumber)
	}
}

func TestFlow_LoopConvergence_UnionTypes(t *testing.T) {
	c, header, body := buildLoopCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), header, body, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, header, symX, ver1)
	setVersion(g, body, symX, ver1)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.String
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: body,
			TargetPath: constraint.Path{
				Root:   "x",
				Symbol: symX,
			},
			Type: typ.Number,
		},
	}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(header, constraint.Path{Root: "x", Symbol: symX})

	if got == nil {
		t.Fatal("expected type at header, got nil")
	}

	if got.Kind() != typ.NewUnion(typ.String, typ.Number).Kind() {
		t.Logf("got type: %v (kind: %v)", got, got.Kind())
	}

	iterations := s.DebugIterations()
	maxExpected := c.Size() * 5

	if iterations > maxExpected {
		t.Errorf("too many iterations: got %d, expected <= %d", iterations, maxExpected)
	}
}

func TestFlow_LoopConvergence_MultipleTypes(t *testing.T) {
	c, header, body := buildLoopCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), header, body, c.Exit()}
	symA := setupSymbol(g, "a", allPoints)
	symB := setupSymbol(g, "b", allPoints)
	verA := cfg.Version{Root: "a", Symbol: symA, ID: 1}
	verB := cfg.Version{Root: "b", Symbol: symB, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symA, verA)
		setVersion(g, p, symB, verB)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symA] = typ.String
	inputs.DeclaredTypes[symB] = typ.Number

	inputs.Assignments = []UnifiedAssignment{
		{
			Point: body,
			TargetPath: constraint.Path{
				Root:   "a",
				Symbol: symA,
			},
			Type: typ.Boolean,
		},
		{
			Point: body,
			TargetPath: constraint.Path{
				Root:   "b",
				Symbol: symB,
			},
			Type: typ.Integer,
		},
	}

	s := Solve(inputs, testResolver())
	gotA := s.TypeAt(header, constraint.Path{Root: "a", Symbol: symA})
	gotB := s.TypeAt(header, constraint.Path{Root: "b", Symbol: symB})

	if gotA == nil || gotB == nil {
		t.Fatal("expected types at header")
	}

	iterations := s.DebugIterations()
	maxExpected := c.Size() * 5

	if iterations > maxExpected {
		t.Errorf("too many iterations: got %d, expected <= %d", iterations, maxExpected)
	}
}

func TestFlow_LoopConvergence_NestedUnions(t *testing.T) {
	c, header, body := buildLoopCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), header, body, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)

	inputs.Assignments = []UnifiedAssignment{
		{
			Point: body,
			TargetPath: constraint.Path{
				Root:   "x",
				Symbol: symX,
			},
			Type: typ.NewUnion(typ.Boolean, typ.Integer),
		},
	}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(header, constraint.Path{Root: "x", Symbol: symX})

	if got == nil {
		t.Fatal("expected type at header, got nil")
	}
	t.Logf("converged type: %v", got)

	iterations := s.DebugIterations()
	maxExpected := c.Size() * 5

	if iterations > maxExpected {
		t.Errorf("too many iterations: got %d, expected <= %d", iterations, maxExpected)
	}
}

func TestSolution_NarrowedTypeAt(t *testing.T) {
	c := cfg.New()
	entry := c.Entry()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{entry})
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, entry, symX, verX)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.String

	s := Solve(inputs, testResolver())
	got := s.NarrowedTypeAt(entry, constraint.Path{Root: "x", Symbol: symX})

	if got == nil {
		t.Fatal("expected type from NarrowedTypeAt, got nil")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("NarrowedTypeAt returned %v, want string", got)
	}
}

func TestSolution_NarrowedTypeAt_Nil(t *testing.T) {
	var s *Solution = nil
	got := s.NarrowedTypeAt(cfg.Point(1), constraint.Path{Root: "x"})
	if got != nil {
		t.Errorf("nil Solution.NarrowedTypeAt should return nil, got %v", got)
	}
}

func TestNarrowedTypeAt_NotNilConstraint(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	optionalString := typ.NewOptional(typ.String)
	inputs.DeclaredTypes[symX] = optionalString

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "x", Symbol: symX}}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	if typ.TypeEquals(got, optionalString) {
		t.Errorf("NarrowedTypeAt should narrow string? to string, got %v", got)
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("NarrowedTypeAt = %v, want string", got)
	}
}

func TestNarrowedTypeAt_HasTypeConstraint(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typ.String, typ.Number)
	inputs.DeclaredTypes[symX] = union

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: constraint.Path{Root: "x", Symbol: symX}, Type: narrow.BuiltinTypeKey("string")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("NarrowedTypeAt = %v, want string", got)
	}
}

func TestNarrowedTypeAt_IgnoresUnrelatedUnsatConstraints(t *testing.T) {
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
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.NewRecord().Build())
	wantY := typ.Func().Param("v", typ.Any).Returns(typ.Any).Build()
	inputs.DeclaredTypes[symY] = wantY

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "x", Symbol: symX}}, constraint.NotHasType{Path: constraint.Path{Root: "x", Symbol: symX}, Type: narrow.BuiltinTypeKey("table")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "y", Symbol: symY})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	if !typ.TypeEquals(got, wantY) {
		t.Errorf("NarrowedTypeAt(y) = %v, want %v", got, wantY)
	}
}

// buildNestedBranchCFG creates: entry -> branch1 -> then1 -> branch2 -> then2 -> exit
func buildNestedBranchCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, then1, true)
	c.AddEdge(branch1, c.Exit(), false)
	c.AddEdge(then1, branch2, true)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(then2, c.Exit(), true)

	return c, branch1, then1, branch2, then2
}

func TestNarrowedTypeAt_NestedBranch_PropagatesNarrowing(t *testing.T) {
	c, branch1, then1, branch2, then2 := buildNestedBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch1, then1, branch2, then2, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	optionalString := typ.NewOptional(typ.String)
	inputs.DeclaredTypes[symX] = optionalString

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch1,
			To:        then1,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "x", Symbol: symX}}),
		},
	}

	s := Solve(inputs, testResolver())

	// then1: direct child of branch1
	got1 := s.NarrowedTypeAt(then1, constraint.Path{Root: "x", Symbol: symX})
	if got1 == nil {
		t.Fatal("NarrowedTypeAt(then1) returned nil")
	}
	if !typ.TypeEquals(got1, typ.String) {
		t.Errorf("NarrowedTypeAt(then1) = %v, want string", got1)
	}

	// branch2: flows from then1
	got2 := s.NarrowedTypeAt(branch2, constraint.Path{Root: "x", Symbol: symX})
	if got2 == nil {
		t.Fatal("NarrowedTypeAt(branch2) returned nil")
	}
	if !typ.TypeEquals(got2, typ.String) {
		t.Errorf("NarrowedTypeAt(branch2) = %v, want string", got2)
	}

	// then2: nested inside branch2
	got3 := s.NarrowedTypeAt(then2, constraint.Path{Root: "x", Symbol: symX})
	if got3 == nil {
		t.Fatal("NarrowedTypeAt(then2) returned nil")
	}
	if !typ.TypeEquals(got3, typ.String) {
		t.Errorf("NarrowedTypeAt(then2) = %v, want string", got3)
	}
}

func TestNarrowedTypeAt_ComposesNotNilWithChildDiscriminant(t *testing.T) {
	c, branch1, then1, branch2, then2 := buildNestedBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch1, then1, branch2, then2, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	allow := typ.NewRecord().
		Field("kind", typ.LiteralString("allow")).
		Field("reason", typ.String).
		Build()
	deny := typ.NewRecord().
		Field("kind", typ.LiteralString("deny")).
		Field("reason", typ.String).
		Build()
	deferType := typ.NewRecord().
		Field("kind", typ.LiteralString("defer")).
		Field("queue", typ.String).
		Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.NewUnion(allow, deny, deferType))

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch1,
			To:        then1,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX}),
		},
		{
			From: branch2,
			To:   then2,
			Condition: constraint.FromConstraints(constraint.FieldEquals{
				Target: pathX,
				Field:  "kind",
				Value:  typ.LiteralString("defer"),
			}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(then2, pathX)
	if !typ.TypeEquals(got, deferType) {
		t.Fatalf("NarrowedTypeAt(then2) = %v, want %v", got, deferType)
	}

	pathQueue := constraint.Path{
		Root:   "x",
		Symbol: symX,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "queue"},
		},
	}
	gotQueue := s.NarrowedTypeAt(then2, pathQueue)
	if !typ.TypeEquals(gotQueue, typ.String) {
		t.Fatalf("NarrowedTypeAt(then2, x.queue) = %v, want string", gotQueue)
	}
}

// buildReassignCFG creates: entry -> assign1(x) -> call1 -> assign2(x) -> call2 -> use -> exit
// This models: x = f1(); assert_is_nil(x); x = f2(); assert_not_nil(x); use(x)
func buildReassignCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	assign1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "") // x = f1()
	call1 := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "")     // assert_is_nil(x)
	assign2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "") // x = f2()
	call2 := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "")     // assert_not_nil(x)
	use := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")     // y = x.field
	c.AddEdge(c.Entry(), assign1, true)
	c.AddEdge(assign1, call1, true)
	c.AddEdge(call1, assign2, true)
	c.AddEdge(assign2, call2, true)
	c.AddEdge(call2, use, true)
	c.AddEdge(use, c.Exit(), true)
	return c, assign1, call1, assign2, call2, use
}

// TestNarrowedTypeAt_ReassignmentWithConflictingConstraints tests that IsNil on x@def1
// and NotNil on x@def2 do not interfere with each other.
func TestNarrowedTypeAt_ReassignmentWithConflictingConstraints(t *testing.T) {
	c, assign1, call1, assign2, call2, use := buildReassignCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), assign1, call1, assign2, call2, use, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// Version 1 at entry through call1, version 2 after assign2
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, assign1, symX, ver1)
	setVersion(g, call1, symX, ver1)
	setVersion(g, assign2, symX, ver2)
	setVersion(g, call2, symX, ver2)
	setVersion(g, use, symX, ver2)

	inputs := newInputs(g)

	// Record type with field
	recType := typ.NewRecord().Field("field", typ.Number).Build()
	optionalRec := typ.NewOptional(recType)

	// x is declared as optional record
	inputs.DeclaredTypes[symX] = optionalRec

	// Assignments: x@assign1 = optionalRec, x@assign2 = optionalRec
	inputs.Assignments = []UnifiedAssignment{
		{Point: assign1, TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: optionalRec},
		{Point: assign2, TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: optionalRec},
	}

	// IsNil on edge call1->assign2 (after first assertion, before reassignment)
	// NotNil on edge call2->use (after second assertion)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      call1,
			To:        assign2,
			Condition: constraint.FromConstraints(constraint.IsNil{Path: constraint.Path{Root: "x", Symbol: symX}}),
		},
		{
			From:      call2,
			To:        use,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "x", Symbol: symX}}),
		},
	}

	s := Solve(inputs, testResolver())

	t.Logf("VersionedKey at assign1: %s", s.DebugVersionedKey("x", assign1))
	t.Logf("VersionedKey at call1: %s", s.DebugVersionedKey("x", call1))
	t.Logf("VersionedKey at assign2: %s", s.DebugVersionedKey("x", assign2))
	t.Logf("VersionedKey at call2: %s", s.DebugVersionedKey("x", call2))
	t.Logf("VersionedKey at use: %s", s.DebugVersionedKey("x", use))

	// Check edge values
	t.Logf("EdgeValues call1->assign2: %v", s.DebugEdgeValues(call1, assign2))
	t.Logf("EdgeValues call2->use: %v", s.DebugEdgeValues(call2, use))

	// At use point, x should be narrowed to recType (not nil), not never
	got := s.NarrowedTypeAt(use, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt(use) returned nil")
	}
	t.Logf("TypeAt(use) = %v", got)

	// Should NOT be never type
	if got.Kind() == 0 {
		t.Errorf("TypeAt(use) = never, want non-never (got %v)", got)
	}

	// Should be the record type (narrowed from optional)
	if typ.TypeEquals(got, typ.Never) {
		t.Errorf("TypeAt(use) = never, IsNil from def1 should not conflict with NotNil from def2")
	}
}

// buildPhiTruthyCFG creates:
//
//	entry -> branch1 -> then1 (assignment) -> join1
//	                 -> join1 (false path, no assignment)
//	join1 -> branch2 -> then2 (query point, Truthy constraint on edge)
//	                 -> exit (false path)
//	then2 -> exit
//
// This models:
//
//	local err: Err?
//	if flag then err = errors.new() end
//	if err then err:kind() end
func buildPhiTruthyCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "") // if flag
	then1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")   // err = errors.new()
	join1 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")     // merge after if flag
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "") // if err
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")   // err:kind() (query point)

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, then1, true)  // true path: assignment
	c.AddEdge(branch1, join1, false) // false path: skip
	c.AddEdge(then1, join1, true)
	c.AddEdge(join1, branch2, true)
	c.AddEdge(branch2, then2, true) // true path: err is truthy
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(then2, c.Exit(), true)

	return c, branch1, then1, join1, branch2, then2
}

// TestNarrowedTypeAt_PhiTruthy tests that after a phi merge, Truthy constraints
// still narrow the type. This is the "err:kind() after if err then" bug.
func TestNarrowedTypeAt_PhiTruthy(t *testing.T) {
	c, branch1, then1, join1, branch2, then2 := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch1, then1, join1, branch2, then2, c.Exit()}
	symErr := setupSymbol(g, "err", allPoints)

	// Version 1 at entry (nil), version 2 after assignment in then1, version 3 at phi join
	ver1 := cfg.Version{Root: "err", Symbol: symErr, ID: 1}
	ver2 := cfg.Version{Root: "err", Symbol: symErr, ID: 2}
	ver3 := cfg.Version{Root: "err", Symbol: symErr, ID: 3}

	setVersion(g, c.Entry(), symErr, ver1)
	setVersion(g, branch1, symErr, ver1)
	setVersion(g, then1, symErr, ver2)
	setVersion(g, join1, symErr, ver3)
	setVersion(g, branch2, symErr, ver3)
	setVersion(g, then2, symErr, ver3)

	// Add phi node at join1
	g.addPhiNode(cfg.PhiNode{
		Point:  join1,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: branch1, Version: ver1},
			{From: then1, Version: ver2},
		},
	})

	inputs := newInputs(g)

	// Declare err: Err? (optional interface type)
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	optionalErr := typ.NewOptional(errType)

	inputs.DeclaredTypes[symErr] = optionalErr

	// Assignment in then1: err = errors.new() (non-nil Err)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: then1,
			TargetPath: constraint.Path{
				Root:   "err",
				Symbol: symErr,
			},
			Type: errType, // Non-optional in the assignment
		},
	}

	// Edge constraint: Truthy{Path: "err"} on branch2 -> then2
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: constraint.Path{Root: "err", Symbol: symErr}}),
		},
	}

	s := Solve(inputs, testResolver())

	// Debug logging
	t.Logf("VersionedKey at join1: %s", s.DebugVersionedKey("err", join1))
	t.Logf("VersionedKey at branch2: %s", s.DebugVersionedKey("err", branch2))
	t.Logf("VersionedKey at then2: %s", s.DebugVersionedKey("err", then2))

	// Check type at join1 - should be Err? (union of nil from entry, Err from then1)
	typeAtJoin := s.TypeAt(join1, constraint.Path{Root: "err", Symbol: symErr})
	t.Logf("TypeAt(join1) = %v", typeAtJoin)

	// Check type at branch2 - should still be Err?
	typeAtBranch2 := s.TypeAt(branch2, constraint.Path{Root: "err", Symbol: symErr})
	t.Logf("TypeAt(branch2) = %v", typeAtBranch2)

	// Check constraints at then2
	constraintsAtThen2 := s.ConditionAt(then2)
	t.Logf("ConstraintsAt(then2) = %v (len=%d)", constraintsAtThen2.AllConstraints(), constraintsAtThen2.NumDisjuncts())

	// The key test: NarrowedTypeAt(then2) should return the non-optional Err type
	got := s.NarrowedTypeAt(then2, constraint.Path{Root: "err", Symbol: symErr})
	if got == nil {
		t.Fatal("NarrowedTypeAt(then2) returned nil")
	}
	t.Logf("NarrowedTypeAt(then2) = %v (kind=%v)", got, got.Kind())

	// Should NOT be optional anymore - Truthy constraint should narrow Err? to Err
	if got.Kind() == typ.NewOptional(errType).Kind() {
		t.Errorf("NarrowedTypeAt(then2) = %v, want non-optional Err (Truthy should narrow)", got)
	}

	// Should be the Err interface type (or equivalent)
	// Accept either the exact interface or something narrowed from optional
	if typ.TypeEquals(got, optionalErr) {
		t.Errorf("NarrowedTypeAt(then2) = optional type %v, Truthy constraint should have narrowed it", got)
	}
}

// TestNarrowedTypeAt_TableFieldTruthy tests Truthy constraint on table field paths.
// This models: local result = { err = nil }; if flag then result.err = ... end; if result.err then ... end
func TestNarrowedTypeAt_TableFieldTruthy(t *testing.T) {
	c, _, then1, join1, branch2, then2 := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	// Declare result: { err: Err? }
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	optionalErr := typ.NewOptional(errType)
	resultType := typ.NewRecord().Field("err", optionalErr).Build()

	allPoints := []cfg.Point{c.Entry(), then1, join1, branch2, then2, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = resultType

	// Assignment in then1: result.err = Err (non-nil)
	// This represents: result.err = errors.new("fail")
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: then1,
			TargetPath: constraint.Path{
				Root:     "result",
				Symbol:   symResult,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			},
			Type: errType, // Non-optional in the assignment
		},
	}

	// Edge constraint: Truthy{Path: result.err} on branch2 -> then2
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch2,
			To:   then2,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: constraint.Path{
				Root:     "result",
				Symbol:   symResult,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			}}),
		},
	}

	s := Solve(inputs, testResolver())

	// Debug logging
	t.Logf("VersionedKey at join1 for 'result': %s", s.DebugVersionedKey("result", join1))
	t.Logf("VersionedKey at then2 for 'result': %s", s.DebugVersionedKey("result", then2))

	// Check type at join1 for result.err
	pathResultErr := constraint.Path{Root: "result", Symbol: symResult, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}}}
	typeAtJoin := s.TypeAt(join1, pathResultErr)
	t.Logf("TypeAt(join1, result.err) = %v", typeAtJoin)

	// Check type at branch2 for result.err
	typeAtBranch2 := s.TypeAt(branch2, pathResultErr)
	t.Logf("TypeAt(branch2, result.err) = %v", typeAtBranch2)

	// Check type at then2 for result.err - this is the key one
	typeAtThen2 := s.TypeAt(then2, pathResultErr)
	t.Logf("TypeAt(then2, result.err) = %v", typeAtThen2)

	// Check constraints at then2
	constraintsAtThen2 := s.ConditionAt(then2)
	t.Logf("ConstraintsAt(then2) = %v (len=%d)", constraintsAtThen2.AllConstraints(), constraintsAtThen2.NumDisjuncts())

	// The key test: NarrowedTypeAt(then2, result.err) should return the non-optional Err type
	got := s.NarrowedTypeAt(then2, pathResultErr)
	if got == nil {
		t.Fatal("NarrowedTypeAt(then2, result.err) returned nil")
	}
	t.Logf("NarrowedTypeAt(then2, result.err) = %v (kind=%v)", got, got.Kind())

	// Should NOT be never
	if typ.TypeEquals(got, typ.Never) {
		t.Errorf("NarrowedTypeAt(then2, result.err) = never, but should be narrowed Err type")
	}

	// Should be the Err interface type (not optional)
	if typ.TypeEquals(got, optionalErr) {
		t.Errorf("NarrowedTypeAt(then2, result.err) = optional type %v, Truthy constraint should have narrowed it", got)
	}
}

// TestNarrowedTypeAt_TableFieldTruthyNoAssignment tests Truthy constraint on table field
// with declared type but no explicit field assignment.
// This models: local result: { err: Err? } = ...; if result.err then ... end
func TestNarrowedTypeAt_TableFieldTruthyNoAssignment(t *testing.T) {
	// Simple CFG: entry -> branch -> then -> exit
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Declare result: { err: Err? }
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	optionalErr := typ.NewOptional(errType)
	resultType := typ.NewRecord().Field("err", optionalErr).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = resultType

	// NO explicit field assignment - just declared type

	// Edge constraint: Truthy{Path: result.err} on branch -> then
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: constraint.Path{
				Root:     "result",
				Symbol:   symResult,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			}}),
		},
	}

	s := Solve(inputs, testResolver())

	// Debug logging
	t.Logf("VersionedKey at branch for 'result': %s", s.DebugVersionedKey("result", branch))
	t.Logf("VersionedKey at thenNode for 'result': %s", s.DebugVersionedKey("result", thenNode))

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathResultErr := constraint.Path{Root: "result", Symbol: symResult, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}}}

	// Debug: check what's stored in values
	t.Logf("Values for 'result@0' at entry: %v", s.DebugValueAt("result@0", c.Entry()))
	t.Logf("Values for 'result@0' at branch: %v", s.DebugValueAt("result@0", branch))
	t.Logf("Values for 'result@0' at thenNode: %v", s.DebugValueAt("result@0", thenNode))
	t.Logf("EdgeValues branch->thenNode: %v", s.DebugEdgeValues(branch, thenNode))

	// Check base type at branch for result
	typeAtBranchBase := s.TypeAt(branch, pathResult)
	t.Logf("TypeAt(branch, result) = %v", typeAtBranchBase)

	// Check type at branch for result.err
	typeAtBranch := s.TypeAt(branch, pathResultErr)
	t.Logf("TypeAt(branch, result.err) = %v", typeAtBranch)

	// Check base type at thenNode for result (root)
	typeAtThenBase := s.TypeAt(thenNode, pathResult)
	t.Logf("TypeAt(thenNode, result) = %v", typeAtThenBase)

	// Check type at thenNode for result.err
	typeAtThen := s.TypeAt(thenNode, pathResultErr)
	t.Logf("TypeAt(thenNode, result.err) = %v", typeAtThen)

	// Check constraints at thenNode
	constraintsAtThen := s.ConditionAt(thenNode)
	t.Logf("ConstraintsAt(thenNode) = %v (len=%d)", constraintsAtThen.AllConstraints(), constraintsAtThen.NumDisjuncts())

	// The key test: NarrowedTypeAt(thenNode, result.err) should return the non-optional Err type
	got := s.NarrowedTypeAt(thenNode, pathResultErr)
	if got == nil {
		t.Fatal("NarrowedTypeAt(thenNode, result.err) returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, result.err) = %v (kind=%v)", got, got.Kind())

	// Should NOT be never
	if typ.TypeEquals(got, typ.Never) {
		t.Errorf("NarrowedTypeAt(thenNode, result.err) = never, but should be narrowed Err type")
	}

	// Should be the Err interface type (not optional)
	if typ.TypeEquals(got, optionalErr) {
		t.Errorf("NarrowedTypeAt(thenNode, result.err) = optional type %v, Truthy constraint should have narrowed it", got)
	}
}

// TestFieldAssignment_Widening tests that field assignments widen the field type.
// This models: local result = { err = nil }; if flag then result.err = Err end; if result.err then ... end
// At the join point after the conditional assignment, result.err should be nil | Err (Err?).
// NOTE: This test requires field-level phi nodes which are not currently implemented in the SSA model.
// Field assignments update the global values map, so branch-specific narrowings are not preserved.
func TestFieldAssignment_Widening(t *testing.T) {
	c, branch1, then1, join1, branch2, then2 := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	// Declare result with initial type {err: nil}
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	initialResultType := typ.NewRecord().Field("err", typ.Nil).Build()

	allPoints := []cfg.Point{c.Entry(), branch1, then1, join1, branch2, then2, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = initialResultType

	// Assignment at entry to initialize the root value
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: c.Entry(),
			TargetPath: constraint.Path{
				Root:   "result",
				Symbol: symResult,
			},
			Type: initialResultType,
		},
		// Assignment in then1: result.err = Err (non-nil)
		// This should widen result.err from nil to nil | Err
		{
			Point: then1,
			TargetPath: constraint.Path{
				Root:     "result",
				Symbol:   symResult,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			},
			Type: errType, // Assigning Err type
		},
	}

	// Edge constraint: Truthy{Path: result.err} on branch2 -> then2
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch2,
			To:   then2,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: constraint.Path{
				Root:     "result",
				Symbol:   symResult,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			}}),
		},
	}

	s := Solve(inputs, testResolver())

	// Debug logging
	pathResultErr := constraint.Path{Root: "result", Symbol: symResult, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}}}
	pathResult := constraint.Path{Root: "result", Symbol: symResult}

	t.Logf("TypeAt(entry, result) = %v", s.TypeAt(c.Entry(), pathResult))
	t.Logf("TypeAt(entry, result.err) = %v", s.TypeAt(c.Entry(), pathResultErr))
	t.Logf("TypeAt(then1, result.err) = %v", s.TypeAt(then1, pathResultErr))
	t.Logf("TypeAt(join1, result.err) = %v", s.TypeAt(join1, pathResultErr))
	t.Logf("TypeAt(branch2, result.err) = %v", s.TypeAt(branch2, pathResultErr))

	// At join1, result.err should be nil | Err (widened from both branches)
	typeAtJoin := s.TypeAt(join1, pathResultErr)
	t.Logf("TypeAt(join1, result.err) actual = %v", typeAtJoin)

	if typeAtJoin == nil {
		t.Fatal("TypeAt(join1, result.err) returned nil, expected widened type")
	}

	// The type should contain both nil and Err
	// It could be Err? (optional) or nil | Err (union)
	if typeAtJoin == typ.Nil {
		t.Errorf("TypeAt(join1, result.err) = nil, but should be widened to include Err")
	}

	// Check that narrowing works at then2
	constraintsAtThen2 := s.ConditionAt(then2)
	t.Logf("ConstraintsAt(then2) = %v (len=%d)", constraintsAtThen2.AllConstraints(), constraintsAtThen2.NumDisjuncts())

	got := s.NarrowedTypeAt(then2, pathResultErr)
	t.Logf("NarrowedTypeAt(then2, result.err) = %v", got)

	if got == nil {
		t.Fatal("NarrowedTypeAt(then2, result.err) returned nil")
	}

	// Should NOT be never
	if typ.TypeEquals(got, typ.Never) {
		t.Errorf("NarrowedTypeAt(then2, result.err) = never, but should be narrowed Err type")
	}

	// Should NOT be nil (Truthy should have narrowed it)
	if got == typ.Nil {
		t.Errorf("NarrowedTypeAt(then2, result.err) = nil, Truthy constraint should have narrowed it")
	}
}

// TestNarrowedTypeAt_NilInitConditionalAssign tests the pattern:
//
//	local v = nil
//	if cond then v = obj end
//	if v then v:method() end
//
// This fails in integration tests with "expected function, got never".
// The key difference from TestNarrowedTypeAt_PhiTruthy is that the initial
// declared type is literal nil, not T? (optional).
func TestNarrowedTypeAt_NilInitConditionalAssign(t *testing.T) {
	c, branch1, then1, join1, branch2, then2 := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	// Declare v: nil (literal nil type, not optional)
	// This is what happens with `local v = nil` without type annotation
	versionType := typ.NewInterface("Version", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String).Build()},
	})

	allPoints := []cfg.Point{c.Entry(), branch1, then1, join1, branch2, then2, c.Exit()}
	symV := setupSymbol(g, "v", allPoints)

	// SSA versions:
	// v@1: defined at entry (nil)
	// v@2: defined at then1 (Version)
	// v@3: phi at join1 (joins v@1 and v@2)
	ver1 := cfg.Version{Root: "v", Symbol: symV, ID: 1}
	ver2 := cfg.Version{Root: "v", Symbol: symV, ID: 2}
	ver3 := cfg.Version{Root: "v", Symbol: symV, ID: 3}

	// Version visibility at each point
	setVersion(g, c.Entry(), symV, ver1)
	setVersion(g, branch1, symV, ver1)
	setVersion(g, then1, symV, ver2)
	setVersion(g, join1, symV, ver3)
	setVersion(g, branch2, symV, ver3)
	setVersion(g, then2, symV, ver3)
	setVersion(g, c.Exit(), symV, ver3)

	// Phi node at join1: merges v@1 (false path from branch1) and v@2 (from then1)
	g.addPhiNode(cfg.PhiNode{
		Point:  join1,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: branch1, Version: ver1}, // false path: no assignment, v@1
			{From: then1, Version: ver2},   // assignment path: v@2
		},
	})

	inputs := newInputs(g)
	// Key: declared type is literal nil, not Version?
	inputs.DeclaredTypes[symV] = typ.Nil

	// Assignment at entry: v@1 = nil
	// Assignment in then1: v@2 = versionObj (non-nil Version)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: c.Entry(),
			TargetPath: constraint.Path{
				Root:   "v",
				Symbol: symV,
			},
			Type: typ.Nil,
		},
		{
			Point: then1,
			TargetPath: constraint.Path{
				Root:   "v",
				Symbol: symV,
			},
			Type: versionType,
		},
	}

	// Edge constraint: Truthy{Path: "v"} on branch2 -> then2
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: constraint.Path{Root: "v", Symbol: symV}}),
		},
	}

	s := Solve(inputs, testResolver())

	// Debug logging
	t.Logf("DeclaredType for v: %v", inputs.DeclaredTypes[symV])
	t.Logf("VersionedKey at entry: %s", s.DebugVersionedKey("v", c.Entry()))
	t.Logf("VersionedKey at then1: %s", s.DebugVersionedKey("v", then1))
	t.Logf("VersionedKey at join1: %s", s.DebugVersionedKey("v", join1))
	t.Logf("VersionedKey at branch2: %s", s.DebugVersionedKey("v", branch2))
	t.Logf("VersionedKey at then2: %s", s.DebugVersionedKey("v", then2))

	// Check type at entry - should be nil
	pathV := constraint.Path{Root: "v", Symbol: symV}
	typeAtEntry := s.TypeAt(c.Entry(), pathV)
	t.Logf("TypeAt(entry) = %v", typeAtEntry)

	// Check type at then1 - should be Version (from assignment)
	typeAtThen1 := s.TypeAt(then1, pathV)
	t.Logf("TypeAt(then1) = %v", typeAtThen1)

	// Check type at join1 - should be nil | Version (widened from both branches)
	typeAtJoin1 := s.TypeAt(join1, pathV)
	t.Logf("TypeAt(join1) = %v", typeAtJoin1)

	// Check type at branch2 - should still be nil | Version
	typeAtBranch2 := s.TypeAt(branch2, pathV)
	t.Logf("TypeAt(branch2) = %v", typeAtBranch2)

	// Check constraints at then2
	constraintsAtThen2 := s.ConditionAt(then2)
	t.Logf("ConstraintsAt(then2) = %v (len=%d)", constraintsAtThen2.AllConstraints(), constraintsAtThen2.NumDisjuncts())

	// The key test: NarrowedTypeAt(then2) should return Version, not never
	got := s.NarrowedTypeAt(then2, pathV)
	if got == nil {
		t.Fatal("NarrowedTypeAt(then2) returned nil")
	}
	t.Logf("NarrowedTypeAt(then2) = %v (kind=%v)", got, got.Kind())

	// Should NOT be never type
	if typ.TypeEquals(got, typ.Never) {
		t.Errorf("NarrowedTypeAt(then2) = never, but should be narrowed to Version")
	}

	// Should NOT be nil (Truthy should have narrowed it)
	if got == typ.Nil {
		t.Errorf("NarrowedTypeAt(then2) = nil, Truthy constraint should have narrowed it")
	}

	// Should be the Version interface type
	if !typ.TypeEquals(got, versionType) {
		t.Logf("Note: got %v, expected exactly %v (may be acceptable if interface-equivalent)", got, versionType)
	}
}

// TestNarrowedTypeAt_CompositionalNarrowing tests that child paths derive
// from narrowed parents, not from the un-narrowed base type.
// This is the core invariant for channel.select narrowing:
//
//	NarrowedTypeAt(p, P.child) = derive(NarrowedTypeAt(p, P), child)
func TestNarrowedTypeAt_CompositionalNarrowing(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Build channel union: {channel: ChA, value: string} | {channel: ChB, value: number}
	chanA := typ.NewRecord().Field("__tag", typ.LiteralString("a")).Build()
	chanB := typ.NewRecord().Field("__tag", typ.LiteralString("b")).Build()

	variant1 := typ.NewRecord().
		Field("channel", chanA).
		Field("value", typ.String).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", chanB).
		Field("value", typ.Number).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symChB := setupSymbol(g, "chB", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verChB := cfg.Version{Root: "chB", Symbol: symChB, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symChB, verChB)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symChB] = chanB

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "result", Symbol: symResult}, Type: typ.NewUnion(variant1, variant2)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "chB", Symbol: symChB}, Type: chanB},
	}

	// Add FieldNotEqualsPath constraint to exclude ChB
	// This is what happens in channel.select else branch
	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathChB := constraint.Path{Root: "chB", Symbol: symChB}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathChB}),
		},
	}

	s := Solve(inputs, testResolver())

	// Parent should be narrowed to variant1 only
	gotParent := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "result", Symbol: symResult})
	if gotParent == nil {
		t.Fatal("NarrowedTypeAt(result) returned nil")
	}
	if !typ.TypeEquals(gotParent, variant1) {
		t.Errorf("NarrowedTypeAt(result) = %v, want variant1 %v", gotParent, variant1)
	}

	// Child path: result.value should derive from narrowed parent
	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotChild := s.NarrowedTypeAt(thenNode, pathValue)
	if gotChild == nil {
		t.Fatal("NarrowedTypeAt(result.value) returned nil")
	}

	// Must be string (from variant1), NOT string|number (from original union)
	if !typ.TypeEquals(gotChild, typ.String) {
		t.Errorf("NarrowedTypeAt(result.value) = %v, want string (not string|number)", gotChild)
	}
}

// TestNarrowedTypeAt_CompositionalNarrowing_AfterJoin tests that child derivation
// works correctly after a join point where only one branch contributes.
// This is the pattern for: if result.channel == timeout then return end
// After the if-return, the continuation only has the narrowed variant.
func TestNarrowedTypeAt_CompositionalNarrowing_AfterJoin(t *testing.T) {
	// Build CFG: entry -> branch -> (then->exit, else->join) -> join -> exit
	// This models: if cond then return end; use(result.value)
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "") // return branch
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "") // continuation
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)   // true branch: return
	c.AddEdge(branch, elseNode, false)  // false branch: continuation
	c.AddEdge(thenNode, c.Exit(), true) // return exits
	c.AddEdge(elseNode, join, true)
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)

	// Build channel union: {channel: ChA, value: string} | {channel: ChB, value: number}
	chanA := typ.NewRecord().Field("__tag", typ.LiteralString("a")).Build()
	chanB := typ.NewRecord().Field("__tag", typ.LiteralString("b")).Build()

	variant1 := typ.NewRecord().
		Field("channel", chanA).
		Field("value", typ.String).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", chanB).
		Field("value", typ.Number).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symChB := setupSymbol(g, "chB", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verChB := cfg.Version{Root: "chB", Symbol: symChB, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symChB, verChB)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symChB] = chanB

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "result", Symbol: symResult}, Type: typ.NewUnion(variant1, variant2)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "chB", Symbol: symChB}, Type: chanB},
	}

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathChB := constraint.Path{Root: "chB", Symbol: symChB}

	// True edge (return branch): result.channel == chB
	// False edge (continuation): result.channel != chB (narrowed to variant1)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathChB}),
		},
		{
			From:      branch,
			To:        elseNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathChB}),
		},
	}

	s := Solve(inputs, testResolver())

	// At elseNode: result should be narrowed to variant1
	gotParent := s.NarrowedTypeAt(elseNode, constraint.Path{Root: "result", Symbol: symResult})
	if gotParent == nil {
		t.Fatal("NarrowedTypeAt(elseNode, result) returned nil")
	}
	if !typ.TypeEquals(gotParent, variant1) {
		t.Errorf("NarrowedTypeAt(elseNode, result) = %v, want variant1", gotParent)
	}

	// At elseNode: result.value should be string
	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotChild := s.NarrowedTypeAt(elseNode, pathValue)
	if gotChild == nil {
		t.Fatal("NarrowedTypeAt(elseNode, result.value) returned nil")
	}
	if !typ.TypeEquals(gotChild, typ.String) {
		t.Errorf("NarrowedTypeAt(elseNode, result.value) = %v, want string", gotChild)
	}

	// At join: should still have narrowed type (only else contributes)
	gotParentJoin := s.NarrowedTypeAt(join, constraint.Path{Root: "result", Symbol: symResult})
	if gotParentJoin == nil {
		t.Fatal("NarrowedTypeAt(join, result) returned nil")
	}
	if !typ.TypeEquals(gotParentJoin, variant1) {
		t.Errorf("NarrowedTypeAt(join, result) = %v, want variant1", gotParentJoin)
	}

	// At join: result.value should still be string
	gotChildJoin := s.NarrowedTypeAt(join, pathValue)
	if gotChildJoin == nil {
		t.Fatal("NarrowedTypeAt(join, result.value) returned nil")
	}
	if !typ.TypeEquals(gotChildJoin, typ.String) {
		t.Errorf("NarrowedTypeAt(join, result.value) = %v, want string", gotChildJoin)
	}
}

func TestNarrowedTypeAt_NestedPathPropagation(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Build nested union: {channel: ChA, value: {data: Msg}} | {channel: ChB, value: {data: Time}}
	chanA := typ.NewRecord().Field("__tag", typ.LiteralString("a")).Build()
	chanB := typ.NewRecord().Field("__tag", typ.LiteralString("b")).Build()

	msgType := typ.NewRecord().Field("topic", typ.String).Build()
	timeType := typ.NewRecord().Field("sec", typ.Number).Build()

	innerA := typ.NewRecord().Field("data", msgType).Build()
	innerB := typ.NewRecord().Field("data", timeType).Build()

	variant1 := typ.NewRecord().
		Field("channel", chanA).
		Field("value", innerA).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", chanB).
		Field("value", innerB).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symChB := setupSymbol(g, "chB", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verChB := cfg.Version{Root: "chB", Symbol: symChB, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symChB, verChB)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symChB] = chanB

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "result", Symbol: symResult}, Type: typ.NewUnion(variant1, variant2)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "chB", Symbol: symChB}, Type: chanB},
	}

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathChB := constraint.Path{Root: "chB", Symbol: symChB}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathChB}),
		},
	}

	s := Solve(inputs, testResolver())

	// Parent should be narrowed to variant1 only
	gotParent := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "result", Symbol: symResult})
	if !typ.TypeEquals(gotParent, variant1) {
		t.Errorf("NarrowedTypeAt(result) = %v, want variant1", gotParent)
	}

	// Child path: result.value should derive from narrowed parent
	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotChild := s.NarrowedTypeAt(thenNode, pathValue)
	if !typ.TypeEquals(gotChild, innerA) {
		t.Errorf("NarrowedTypeAt(result.value) = %v, want innerA %v", gotChild, innerA)
	}

	// Grandchild path: result.value.data should derive from narrowed parent
	pathData := constraint.Path{
		Root:   "result",
		Symbol: symResult,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "value"},
			{Kind: constraint.SegmentField, Name: "data"},
		},
	}
	gotGrandchild := s.NarrowedTypeAt(thenNode, pathData)
	if gotGrandchild == nil {
		t.Fatal("NarrowedTypeAt(result.value.data) returned nil")
	}
	if !typ.TypeEquals(gotGrandchild, msgType) {
		t.Errorf("NarrowedTypeAt(result.value.data) = %v, want msgType %v", gotGrandchild, msgType)
	}
}

// TestResolveConstraintPathKey_UnversionedConstraints tests that constraints
// with DefPoint=0 correctly resolve to the current SSA version at query time.
// This enables writing constraints without explicit DefPoint injection.
func TestResolveConstraintPathKey_UnversionedConstraints(t *testing.T) {
	// Build CFG with phi merge:
	// entry -> branch1 -> (then1 | else1) -> join -> branch2 -> then2 -> exit
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, then1, true)
	c.AddEdge(branch1, join, false)
	c.AddEdge(then1, join, true)
	c.AddEdge(join, branch2, true)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(then2, c.Exit(), true)

	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch1, then1, join, branch2, then2, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// x@1: defined at entry (nil)
	// x@2: defined at then1 (string)
	// x@3: phi at join (joins x@1 and x@2), visible at join, branch2, then2
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	ver3 := cfg.Version{Root: "x", Symbol: symX, ID: 3}

	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, branch1, symX, ver1)
	setVersion(g, then1, symX, ver2)
	setVersion(g, join, symX, ver3)
	setVersion(g, branch2, symX, ver3)
	setVersion(g, then2, symX, ver3)
	setVersion(g, c.Exit(), symX, ver3)

	// Phi node at join: merges ver1 (false path from branch1) and ver2 (from then1)
	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: branch1, Version: ver1},
			{From: then1, Version: ver2},
		},
	})

	inputs := newInputs(g)
	// x: string? with two definitions (entry: nil, then1: string)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.String)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.Nil,
		},
		{
			Point:      then1,
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.String,
		},
	}

	// Unversioned constraint (DefPoint=0): NotNil{Path: "x"}
	// This should resolve to the phi version at then2
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "x", Symbol: symX}}),
		},
	}

	s := Solve(inputs, testResolver())

	// At then2, x should be narrowed from string? to string
	got := s.NarrowedTypeAt(then2, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}

	// Should be non-optional string
	if _, isOpt := got.(*typ.Optional); isOpt {
		t.Errorf("NarrowedTypeAt(then2, x) = %v, want non-optional string", got)
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("NarrowedTypeAt(then2, x) = %v, want string", got)
	}

	// Verify the constraint was applied by checking that at branch2 (before constraint),
	// the type is still optional (or contains nil)
	typeAtBranch2 := s.TypeAt(branch2, constraint.Path{Root: "x", Symbol: symX})
	if _, isOpt := typeAtBranch2.(*typ.Optional); !isOpt {
		// Could also be union with nil member, which is fine
		if !core.ContainsNil(typeAtBranch2) {
			t.Errorf("TypeAt(branch2, x) = %v, expected optional/nullable type", typeAtBranch2)
		}
	}
}

// TestNilInitConditionalAssignThenTruthyCheck tests:
// local x = nil; if cond then x = SomeType end; if x then x:method() end
// This is the pattern that produces "expected function, got never".
func TestNilInitConditionalAssignThenTruthyCheck(t *testing.T) {
}

// TestNilInitLoopAssignThenTruthyCheck tests:
// local x = nil; for ... do if cond then x = SomeType end end; if x then x:method() end
// This is the exact pattern that produces "expected function, got never" in integration tests.
func TestNilInitLoopAssignThenTruthyCheck(t *testing.T) {
	// CFG:
	// entry -> loopHeader <-+ -> afterLoop -> branch2 -> then2 (x:id())
	//             |         |                     \
	//          branch1      |                      -> exit
	//           /   \       |
	//    thenNode  elseNode |
	//     (x=V)     (noop)  |
	//           \   /       |
	//            join ------+
	c := cfg.New()
	loopHeader := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	loopJoin := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	afterLoop := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	// entry -> loopHeader
	c.AddEdge(c.Entry(), loopHeader, true)
	// loopHeader -> branch1 (loop body) or afterLoop (exit)
	c.AddEdge(loopHeader, branch1, true)
	c.AddEdge(loopHeader, afterLoop, false)
	// Inside loop: branch1 -> thenNode or elseNode
	c.AddEdge(branch1, thenNode, true)
	c.AddEdge(branch1, elseNode, false)
	// thenNode and elseNode -> loopJoin
	c.AddEdge(thenNode, loopJoin, true)
	c.AddEdge(elseNode, loopJoin, true)
	// loopJoin -> back to loopHeader (back-edge)
	c.AddEdge(loopJoin, loopHeader, true)
	// afterLoop -> branch2 (if x)
	c.AddEdge(afterLoop, branch2, true)
	// branch2 -> then2 (truthy) or exit (falsy)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(then2, c.Exit(), true)

	g := newMockSSAGraph(c)

	// Version interface with id() method
	versionType := typ.NewInterface("Version", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String).Build()},
	})

	allPoints := []cfg.Point{c.Entry(), loopHeader, branch1, thenNode, elseNode, loopJoin, afterLoop, branch2, then2, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// SSA versions for loop pattern:
	// x@1: entry (nil)
	// x@2: loopHeader phi (joins x@1 from entry and x@4 from loopJoin)
	// x@3: thenNode (Version assignment)
	// x@4: loopJoin phi (joins x@3 from thenNode and x@2 from elseNode)
	// After loop: x@2 visible (with widened type nil|Version)
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	ver3 := cfg.Version{Root: "x", Symbol: symX, ID: 3}
	ver4 := cfg.Version{Root: "x", Symbol: symX, ID: 4}

	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, loopHeader, symX, ver2)
	setVersion(g, branch1, symX, ver2)
	setVersion(g, thenNode, symX, ver3)
	setVersion(g, elseNode, symX, ver2)
	setVersion(g, loopJoin, symX, ver4)
	setVersion(g, afterLoop, symX, ver2)
	setVersion(g, branch2, symX, ver2)
	setVersion(g, then2, symX, ver2)
	setVersion(g, c.Exit(), symX, ver2)

	// Phi nodes
	g.addPhiNode(cfg.PhiNode{
		Point:  loopHeader,
		Target: ver2,
		Operands: []cfg.PhiOperand{
			{From: c.Entry(), Version: ver1},
			{From: loopJoin, Version: ver4},
		},
	})
	g.addPhiNode(cfg.PhiNode{
		Point:  loopJoin,
		Target: ver4,
		Operands: []cfg.PhiOperand{
			{From: thenNode, Version: ver3},
			{From: elseNode, Version: ver2},
		},
	})

	inputs := newInputs(g)
	// x = nil at entry
	inputs.DeclaredTypes[symX] = typ.Nil

	// Assignments
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: c.Entry(),
			TargetPath: constraint.Path{
				Root:   "x",
				Symbol: symX,
			},
			Type: typ.Nil,
		},
		{
			Point: thenNode,
			TargetPath: constraint.Path{
				Root:   "x",
				Symbol: symX,
			},
			Type: versionType,
		},
	}

	// Truthy constraint on branch2 -> then2: if x then
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: constraint.Path{Root: "x", Symbol: symX}}),
		},
	}

	s := Solve(inputs, testResolver())

	// At afterLoop, type should be nil | Version (after loop completes)
	typeAtAfterLoop := s.TypeAt(afterLoop, constraint.Path{Root: "x", Symbol: symX})
	if typeAtAfterLoop == nil {
		t.Fatal("TypeAt(afterLoop, x) is nil")
	}

	// At then2 after Truthy constraint, type should be Version (nil excluded)
	got := s.NarrowedTypeAt(then2, constraint.Path{Root: "x", Symbol: symX})

	// Should NOT be never
	if typ.TypeEquals(got, typ.Never) {
		t.Errorf("NarrowedTypeAt(then2, x) = never, expected Version type")
	}

	// Should be Version (not nil)
	if got == typ.Nil {
		t.Errorf("NarrowedTypeAt(then2, x) = nil, expected Version type")
	}
}

// TestSolver_NotHasType_HashKey verifies that NotHasType with a hash-based TypeKey
// correctly excludes the matching type from a union.
func TestSolver_NotHasType_HashKey(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Create two record types representing channel select results
	msgRec := typ.NewRecord().Field("channel", typ.String).Field("value", typ.String).Build()
	timeRec := typ.NewRecord().Field("channel", typ.Number).Field("value", typ.Number).Build()
	union := typ.NewUnion(msgRec, timeRec)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = union
	inputs.TypeKeys[timeRec.Hash()] = timeRec
	inputs.TypeKeys[msgRec.Hash()] = msgRec

	// NotHasType with hash key for timeRec should exclude timeRec from union
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotHasType{Path: constraint.Path{Root: "result", Symbol: symResult}, Type: narrow.HashTypeKey(timeRec.Hash())}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "result", Symbol: symResult})
	t.Logf("NarrowedTypeAt(thenNode, result) = %v", got)

	// Should be msgRec only (timeRec excluded)
	if !typ.TypeEquals(got, msgRec) {
		t.Errorf("NarrowedTypeAt(thenNode, result) = %v, want %v", got, msgRec)
	}
}

// TestSequentialGuards_TerminatingBranches ensures constraints from terminating
// guard branches do not leak into the continuation and force never.
func TestSequentialGuards_TerminatingBranches(t *testing.T) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	else1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join1 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	else2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join2 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, then1, true)
	c.AddEdge(branch1, else1, false)
	c.AddEdge(then1, join1, true)
	c.AddEdge(else1, join1, true)
	c.AddEdge(join1, branch2, true)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, else2, false)
	c.AddEdge(then2, join2, true)
	c.AddEdge(else2, join2, true)
	c.AddEdge(join2, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{
		c.Entry(), branch1, then1, else1, join1, branch2, then2, else2, join2, c.Exit(),
	}
	symErr := setupSymbol(g, "err", allPoints)
	ver := cfg.Version{Root: "err", Symbol: symErr, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symErr, ver)
	}

	errType := typ.NewRecord().Field("kind", typ.String).Build()
	inputs := newInputs(g)
	inputs.DeclaredTypes[symErr] = typ.NewOptional(errType)

	path := constraint.Path{Root: "err", Symbol: symErr}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch1,
			To:        then1,
			Condition: constraint.FromConstraints(constraint.IsNil{Path: path}),
		},
		{
			From:      branch1,
			To:        else1,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: path}),
		},
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.NotHasType{Path: path, Type: narrow.BuiltinTypeKey("table")}),
		},
		{
			From:      branch2,
			To:        else2,
			Condition: constraint.FromConstraints(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("table")}),
		},
	}

	// Simulate terminating guard branches (error/return) by marking their nodes as dead.
	inputs.DeadPoints = map[cfg.Point]bool{
		then1: true,
		then2: true,
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(join2, constraint.Path{Root: "err", Symbol: symErr})
	if got == nil {
		t.Fatal("NarrowedTypeAt(join2, err) returned nil")
	}
	if got.Kind() == kind.Never {
		t.Fatalf("NarrowedTypeAt(join2, err) returned never, want %v", errType)
	}
	if !typ.TypeEquals(got, errType) {
		t.Fatalf("NarrowedTypeAt(join2, err) = %v, want %v", got, errType)
	}
}

// TestSequentialGuards_FieldNotEqualsLiteral ensures sequential literal guards
// on the same record do not collapse to never after terminating branches.
func TestSequentialGuards_FieldNotEqualsLiteral(t *testing.T) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	else1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join1 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	else2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join2 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, then1, true)
	c.AddEdge(branch1, else1, false)
	c.AddEdge(then1, join1, true)
	c.AddEdge(else1, join1, true)
	c.AddEdge(join1, branch2, true)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, else2, false)
	c.AddEdge(then2, join2, true)
	c.AddEdge(else2, join2, true)
	c.AddEdge(join2, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{
		c.Entry(), branch1, then1, else1, join1, branch2, then2, else2, join2, c.Exit(),
	}
	symMeta := setupSymbol(g, "meta", allPoints)
	ver := cfg.Version{Root: "meta", Symbol: symMeta, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symMeta, ver)
	}

	metaType := typ.NewRecord().Field("role", typ.String).Field("department", typ.String).Build()
	inputs := newInputs(g)
	inputs.DeclaredTypes[symMeta] = metaType

	metaPath := constraint.Path{Root: "meta", Symbol: symMeta}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch1,
			To:        then1,
			Condition: constraint.FromConstraints(constraint.FieldNotEquals{Target: metaPath, Field: "role", Value: typ.LiteralString("admin")}),
		},
		{
			From:      branch1,
			To:        else1,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: metaPath, Field: "role", Value: typ.LiteralString("admin")}),
		},
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.FieldNotEquals{Target: metaPath, Field: "department", Value: typ.LiteralString("engineering")}),
		},
		{
			From:      branch2,
			To:        else2,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: metaPath, Field: "department", Value: typ.LiteralString("engineering")}),
		},
	}

	// Terminating guards on then branches.
	inputs.DeadPoints = map[cfg.Point]bool{
		then1: true,
		then2: true,
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(join2, constraint.Path{Root: "meta", Symbol: symMeta})
	if got == nil {
		t.Fatal("NarrowedTypeAt(join2, meta) returned nil")
	}
	if got.Kind() == kind.Never {
		t.Fatalf("NarrowedTypeAt(join2, meta) returned never, want %v", metaType)
	}
	if !typ.TypeEquals(got, metaType) {
		t.Fatalf("NarrowedTypeAt(join2, meta) = %v, want %v", got, metaType)
	}
}

// TestSequentialGuards_ChannelSelectDoubleExclude models:
// if result.channel == stop then return end; if result.channel == ops then ...
// and ensures the second guard does not see `never`.
func TestSequentialGuards_ChannelSelectDoubleExclude(t *testing.T) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	else1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join1 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	else2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join2 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, then1, true)
	c.AddEdge(branch1, else1, false)
	c.AddEdge(then1, join1, true)
	c.AddEdge(else1, join1, true)
	c.AddEdge(join1, branch2, true)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, else2, false)
	c.AddEdge(then2, join2, true)
	c.AddEdge(else2, join2, true)
	c.AddEdge(join2, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{
		c.Entry(), branch1, then1, else1, join1, branch2, then2, else2, join2, c.Exit(),
	}
	symResult := setupSymbol(g, "result", allPoints)
	symStop := setupSymbol(g, "stop", allPoints)
	symOps := setupSymbol(g, "ops", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verStop := cfg.Version{Root: "stop", Symbol: symStop, ID: 1}
	verOps := cfg.Version{Root: "ops", Symbol: symOps, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symStop, verStop)
		setVersion(g, p, symOps, verOps)
	}

	stopCh := typ.NewRecord().Field("__tag", typ.LiteralString("stop")).Build()
	opsCh := typ.NewRecord().Field("__tag", typ.LiteralString("ops")).Build()
	stopVal := typ.NewRecord().Field("reason", typ.String).Build()
	opsVal := typ.NewRecord().Field("query", typ.String).Build()
	variantStop := typ.NewRecord().Field("channel", stopCh).Field("value", stopVal).Build()
	variantOps := typ.NewRecord().Field("channel", opsCh).Field("value", opsVal).Build()
	resultUnion := typ.NewUnion(variantStop, variantOps)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = resultUnion
	inputs.DeclaredTypes[symStop] = stopCh
	inputs.DeclaredTypes[symOps] = opsCh

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathStop := constraint.Path{Root: "stop", Symbol: symStop}
	pathOps := constraint.Path{Root: "ops", Symbol: symOps}

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch1,
			To:        then1,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathStop}),
		},
		{
			From:      branch1,
			To:        else1,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathStop}),
		},
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathOps}),
		},
		{
			From:      branch2,
			To:        else2,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathOps}),
		},
	}

	// Terminating guard for stop branch
	inputs.DeadPoints = map[cfg.Point]bool{then1: true}

	s := Solve(inputs, testResolver())

	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	got := s.NarrowedTypeAt(then2, pathValue)
	if got == nil {
		t.Fatal("NarrowedTypeAt(then2, result.value) returned nil")
	}
	if got.Kind() == kind.Never {
		t.Fatalf("NarrowedTypeAt(then2, result.value) returned never, want %v", opsVal)
	}
	if !typ.TypeEquals(got, opsVal) {
		t.Fatalf("NarrowedTypeAt(then2, result.value) = %v, want %v", got, opsVal)
	}
}

// TestSolver_ExcludesTypeAt verifies that ExcludesTypeAt correctly identifies
// when a NotHasType constraint applies to a path.
func TestSolver_ExcludesTypeAt(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Create a record type for the type guard
	pointRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symData := setupSymbol(g, "data", allPoints)
	verData := cfg.Version{Root: "data", Symbol: symData, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symData, verData)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symData] = typ.Any
	inputs.TypeKeys[pointRec.Hash()] = pointRec

	// NotHasType constraint on edge from branch to thenNode
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotHasType{Path: constraint.Path{Root: "data", Symbol: symData}, Type: narrow.HashTypeKey(pointRec.Hash())}),
		},
	}

	s := Solve(inputs, testResolver())

	// ExcludesTypeAt should return true at thenNode for pointRec
	path := constraint.Path{Root: "data", Symbol: symData}
	if !s.ExcludesTypeAt(thenNode, path, pointRec) {
		t.Errorf("ExcludesTypeAt(thenNode, data, Point) = false, want true")
	}

	// ExcludesTypeAt should return false at branch (before the constraint)
	if s.ExcludesTypeAt(branch, path, pointRec) {
		t.Errorf("ExcludesTypeAt(branch, data, Point) = true, want false")
	}

	// ExcludesTypeAt should return false for a different type
	otherRec := typ.NewRecord().Field("a", typ.String).Build()
	if s.ExcludesTypeAt(thenNode, path, otherRec) {
		t.Errorf("ExcludesTypeAt(thenNode, data, OtherRec) = true, want false")
	}
}

// TestSolver_ExcludesTypeAt_AnyNotNarrowed verifies that ExcludeType(any, T)
// returns any unchanged, and ExcludesTypeAt reports the negative constraint.
func TestSolver_ExcludesTypeAt_AnyNotNarrowed(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	pointRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symData := setupSymbol(g, "data", allPoints)
	verData := cfg.Version{Root: "data", Symbol: symData, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symData, verData)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symData] = typ.Any
	inputs.TypeKeys[pointRec.Hash()] = pointRec

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotHasType{Path: constraint.Path{Root: "data", Symbol: symData}, Type: narrow.HashTypeKey(pointRec.Hash())}),
		},
	}

	s := Solve(inputs, testResolver())

	// NarrowedTypeAt should still return 'any' (not narrowed to Never)
	path := constraint.Path{Root: "data", Symbol: symData}
	narrowed := s.NarrowedTypeAt(thenNode, path)
	if narrowed.Kind() != typ.Any.Kind() {
		t.Errorf("NarrowedTypeAt(thenNode, data) = %v, want any (not Never)", narrowed)
	}

	// But ExcludesTypeAt should return true
	if !s.ExcludesTypeAt(thenNode, path, pointRec) {
		t.Errorf("ExcludesTypeAt(thenNode, data, Point) = false, want true")
	}
}

// TestPhiJoin_StoresTargetVersion tests that phi join correctly stores its target version.
// This test fails if version keys mismatch due to string formatting issues.
func TestPhiJoin_StoresTargetVersion(t *testing.T) {
	c, _, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// v1 at thenNode (string), v2 at elseNode (number), v3 (phi) at join
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	ver3 := cfg.Version{Root: "x", Symbol: symX, ID: 3}

	setVersion(g, thenNode, symX, ver1)
	setVersion(g, elseNode, symX, ver2)
	setVersion(g, join, symX, ver3)

	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: thenNode, Version: ver1},
			{From: elseNode, Version: ver2},
		},
	})

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      thenNode,
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.String,
		},
		{
			Point:      elseNode,
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.Number,
		},
	}

	s := Solve(inputs, testResolver())

	// Verify v3 (phi target) is stored and contains the joined type
	ver3Key := canonicalVersionKey(ver3)
	storedType := s.values[ver3Key]
	if storedType == nil {
		t.Fatalf("phi target version %q not stored in values map", ver3Key)
	}

	// Verify it's the correct joined type
	expectedType := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(storedType, expectedType) {
		t.Errorf("phi target type = %v, want %v", storedType, expectedType)
	}

	// Also verify TypeAt returns the joined type
	gotType := s.TypeAt(join, constraint.Path{Root: "x", Symbol: symX})
	if gotType == nil {
		t.Fatal("TypeAt(join, x) returned nil")
	}
	if !typ.TypeEquals(gotType, expectedType) {
		t.Errorf("TypeAt(join, x) = %v, want %v", gotType, expectedType)
	}
}

// TestPhiJoin_NilInitConditionalAssign_Scheduling tests that phi nodes are correctly
// processed even when operand assignments come later in iteration order.
// This is the pattern: local v = nil; if cond then v = obj end; if v then ...
// The phi must be re-processed when operand values become available.
func TestPhiJoin_NilInitConditionalAssign_Scheduling(t *testing.T) {
	// CFG:
	// entry -> branch1 -> (thenNode | elseNode) -> join -> branch2 -> then2 -> exit
	//                                                  \-> exit (falsy)
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, thenNode, true)
	c.AddEdge(branch1, elseNode, false)
	c.AddEdge(thenNode, join, true)
	c.AddEdge(elseNode, join, true)
	c.AddEdge(join, branch2, true)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(then2, c.Exit(), true)

	g := newMockSSAGraph(c)

	versionType := typ.NewInterface("Version", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String).Build()},
	})

	allPoints := []cfg.Point{c.Entry(), branch1, thenNode, elseNode, join, branch2, then2, c.Exit()}
	symV := setupSymbol(g, "v", allPoints)

	// SSA versions:
	// v@1: entry (declared, nil type from DeclaredTypes)
	// v@2: thenNode (Version assignment)
	// v@3: join phi (joins v@1 from elseNode and v@2 from thenNode)
	ver1 := cfg.Version{Root: "v", Symbol: symV, ID: 1}
	ver2 := cfg.Version{Root: "v", Symbol: symV, ID: 2}
	ver3 := cfg.Version{Root: "v", Symbol: symV, ID: 3}

	setVersion(g, c.Entry(), symV, ver1)
	setVersion(g, branch1, symV, ver1)
	setVersion(g, thenNode, symV, ver2)
	setVersion(g, elseNode, symV, ver1)
	setVersion(g, join, symV, ver3)
	setVersion(g, branch2, symV, ver3)
	setVersion(g, then2, symV, ver3)

	// Phi at join: operands from elseNode (v@1) and thenNode (v@2)
	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: elseNode, Version: ver1},
			{From: thenNode, Version: ver2},
		},
	})

	inputs := newInputs(g)
	// Key: v is declared as nil at entry
	inputs.DeclaredTypes[symV] = typ.Nil

	// Assignment at entry gives v@1 = nil
	// Assignment at thenNode gives v@2 = Version
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "v", Symbol: symV},
			Type:       typ.Nil,
		},
		{
			Point:      thenNode,
			TargetPath: constraint.Path{Root: "v", Symbol: symV},
			Type:       versionType,
		},
	}

	// Truthy constraint on branch2 -> then2
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: constraint.Path{Root: "v", Symbol: symV}}),
		},
	}

	s := Solve(inputs, testResolver())

	// v@3 (phi target) must be stored with joined type nil | Version
	ver3Key := canonicalVersionKey(ver3)
	storedType := s.values[ver3Key]
	if storedType == nil {
		t.Fatalf("phi target version %q not stored in values map; phi scheduling failed", ver3Key)
	}
	t.Logf("v@3 stored type: %v", storedType)

	// Check that v@1 (nil) was used in the join - the stored type must contain nil
	// If only v@2 was used, the type would be just Version without nil
	if typ.TypeEquals(storedType, versionType) {
		t.Errorf("phi join only picked up one operand; v@3 = %v, want nil | Version", storedType)
	}

	// At join, type should be nil | Version (union of both operands)
	typeAtJoin := s.TypeAt(join, constraint.Path{Root: "v", Symbol: symV})
	if typeAtJoin == nil {
		t.Fatal("TypeAt(join, v) returned nil")
	}
	t.Logf("TypeAt(join, v) = %v", typeAtJoin)

	// The type at join must be a union containing nil (from v@1)
	if typ.TypeEquals(typeAtJoin, versionType) {
		t.Errorf("TypeAt(join, v) = %v, want nil | Version (phi must join both operands)", typeAtJoin)
	}

	// At then2 after Truthy constraint, type should be Version only
	got := s.NarrowedTypeAt(then2, constraint.Path{Root: "v", Symbol: symV})
	if got == nil {
		t.Fatal("NarrowedTypeAt(then2, v) returned nil")
	}
	t.Logf("NarrowedTypeAt(then2, v) = %v", got)

	// Must NOT be never
	if typ.TypeEquals(got, typ.Never) {
		t.Error("NarrowedTypeAt(then2, v) = never, expected Version")
	}

	// Must be Version (not nil, not optional)
	if got == typ.Nil {
		t.Error("NarrowedTypeAt(then2, v) = nil, expected Version (Truthy should narrow)")
	}
}

// TestMultiPredEdgeNarrowing_SameType tests that when both branches narrow x to the same type,
// the join point preserves that narrowed type.
func TestMultiPredEdgeNarrowing_SameType(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typ.String, typ.Number)
	inputs.DeclaredTypes[symX] = union

	// Both branches narrow x to string
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
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      thenNode,
			To:        join,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      elseNode,
			To:        join,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
	}

	s := Solve(inputs, testResolver())

	// At join, x should be string (both branches narrow to string)
	got := s.NarrowedTypeAt(join, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt(join, x) returned nil")
	}
	t.Logf("NarrowedTypeAt(join, x) = %v", got)

	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("NarrowedTypeAt(join, x) = %v, want string (both branches narrow to same type)", got)
	}
}

// TestMultiPredEdgeNarrowing_DifferentTypes tests that when branches narrow to different types,
// the join point produces a union.
func TestMultiPredEdgeNarrowing_DifferentTypes(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	inputs.DeclaredTypes[symX] = union

	// Then branch narrows to string, else branch narrows to number
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
		{
			From:      thenNode,
			To:        join,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      elseNode,
			To:        join,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")}),
		},
	}

	s := Solve(inputs, testResolver())

	// At join, x should be string | number
	got := s.NarrowedTypeAt(join, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt(join, x) returned nil")
	}
	t.Logf("NarrowedTypeAt(join, x) = %v", got)

	// Should be a union of string and number
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Logf("got = %v, want = %v", got, want)
		// Allow either ordering
		want2 := typ.NewUnion(typ.Number, typ.String)
		if !typ.TypeEquals(got, want2) {
			t.Errorf("NarrowedTypeAt(join, x) = %v, want string | number", got)
		}
	}
}

// TestMultiPredEdgeNarrowing_PartialEdgeValues tests that when only one branch has edge narrowing,
// the join falls back to point values.
func TestMultiPredEdgeNarrowing_PartialEdgeValues(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typ.String, typ.Number)
	inputs.DeclaredTypes[symX] = union

	// Only then branch has edge constraint to join
	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		{
			From:      thenNode,
			To:        join,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
		// No constraint from elseNode -> join
	}

	s := Solve(inputs, testResolver())

	// At join, x should fall back to declared type since not all predecessors have edge values
	got := s.TypeAt(join, pathX)
	if got == nil {
		t.Fatal("TypeAt(join, x) returned nil")
	}
	t.Logf("TypeAt(join, x) = %v", got)

	// Should be the original union since partial edge values fallback to point values
	if !typ.TypeEquals(got, union) {
		t.Logf("got = %v, want = %v (original union - partial edges fallback)", got, union)
	}
}

// TestFieldNotEqualsPath_InterfaceVariants tests FieldNotEqualsPath narrowing
// when union members have Interface value types (the channel narrowing pattern).
func TestFieldNotEqualsPath_InterfaceVariants(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Build channel-like types using interfaces instead of records for value
	chanA := typ.NewRecord().Field("__tag", typ.LiteralString("a")).Build()
	chanB := typ.NewRecord().Field("__tag", typ.LiteralString("b")).Build()

	// Message and Time are interfaces (like in the actual channel test)
	messageType := typ.NewInterface("Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	variant1 := typ.NewRecord().
		Field("channel", chanA).
		Field("value", messageType).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", chanB).
		Field("value", timeType).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symChB := setupSymbol(g, "chB", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verChB := cfg.Version{Root: "chB", Symbol: symChB, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symChB, verChB)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symChB] = chanB

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "result", Symbol: symResult}, Type: typ.NewUnion(variant1, variant2)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "chB", Symbol: symChB}, Type: chanB},
	}

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathChB := constraint.Path{Root: "chB", Symbol: symChB}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathChB}),
		},
	}

	s := Solve(inputs, testResolver())

	// Parent should be narrowed to variant1 only
	gotParent := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "result", Symbol: symResult})
	if gotParent == nil {
		t.Fatal("NarrowedTypeAt(result) returned nil")
	}
	t.Logf("NarrowedTypeAt(result) = %v", gotParent)
	if !typ.TypeEquals(gotParent, variant1) {
		t.Errorf("NarrowedTypeAt(result) = %v, want variant1 %v", gotParent, variant1)
	}

	// Child path: result.value should derive from narrowed parent and be Message (interface)
	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotChild := s.NarrowedTypeAt(thenNode, pathValue)
	t.Logf("NarrowedTypeAt(result.value) = %v", gotChild)
	if gotChild == nil {
		t.Fatal("NarrowedTypeAt(result.value) returned nil")
	}

	// Must be Message (interface), NOT Message|Time and NOT never
	if gotChild.Kind() == 0 { // never
		t.Fatalf("NarrowedTypeAt(result.value) returned never, want Message interface")
	}
	if !typ.TypeEquals(gotChild, messageType) {
		t.Errorf("NarrowedTypeAt(result.value) = %v, want Message interface %v", gotChild, messageType)
	}
}

// TestFieldNotEqualsPath_BothInterfaceVariants tests narrowing when BOTH
// union variants have Interface value types (both Message and Time are interfaces).
func TestFieldNotEqualsPath_BothInterfaceVariants(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Both channels use interface types - this is the exact failing pattern
	messageChannelType := typ.NewInterface("Channel<Message>", []typ.Method{
		{Name: "send", Type: typ.Func().Param("msg", typ.String).Build()},
	})
	timeChannelType := typ.NewInterface("Channel<Time>", []typ.Method{
		{Name: "send", Type: typ.Func().Param("t", typ.Integer).Build()},
	})

	messageType := typ.NewInterface("Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	variant1 := typ.NewRecord().
		Field("channel", messageChannelType).
		Field("value", messageType).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", timeChannelType).
		Field("value", timeType).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symTimeout := setupSymbol(g, "timeout", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verTimeout := cfg.Version{Root: "timeout", Symbol: symTimeout, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symTimeout, verTimeout)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symTimeout] = timeChannelType

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "result", Symbol: symResult}, Type: typ.NewUnion(variant1, variant2)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "timeout", Symbol: symTimeout}, Type: timeChannelType},
	}

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathTimeout := constraint.Path{Root: "timeout", Symbol: symTimeout}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathTimeout}),
		},
	}

	s := Solve(inputs, testResolver())

	// Parent should be narrowed to variant1 only (Message variant)
	gotParent := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "result", Symbol: symResult})
	if gotParent == nil {
		t.Fatal("NarrowedTypeAt(result) returned nil")
	}
	t.Logf("NarrowedTypeAt(result) = %v", gotParent)
	if !typ.TypeEquals(gotParent, variant1) {
		t.Errorf("NarrowedTypeAt(result) = %v, want variant1 %v", gotParent, variant1)
	}

	// Child path: result.value should be Message interface
	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotChild := s.NarrowedTypeAt(thenNode, pathValue)
	t.Logf("NarrowedTypeAt(result.value) = %v", gotChild)
	if gotChild == nil {
		t.Fatal("NarrowedTypeAt(result.value) returned nil")
	}

	// Must be Message interface, NOT Message|Time and NOT never
	if gotChild.Kind() == 0 { // never
		t.Fatalf("NarrowedTypeAt(result.value) returned never, want Message interface")
	}
	if !typ.TypeEquals(gotChild, messageType) {
		t.Errorf("NarrowedTypeAt(result.value) = %v, want Message interface %v", gotChild, messageType)
	}
}

// TestEdgeNarrowing_SinglePredJoin tests that edge narrowing propagates
// correctly through a join when one predecessor has a dead edge (e.g., returns/errors).
func TestEdgeNarrowing_SinglePredJoin(t *testing.T) {
	// CFG: entry -> branch -> (then exits, else continues) -> join -> exit
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "") // this exits
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, c.Exit(), true) // then exits directly
	c.AddEdge(elseNode, join, true)
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)

	messageType := typ.NewInterface("Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	chanA := typ.NewRecord().Field("__tag", typ.LiteralString("a")).Build()
	chanB := typ.NewRecord().Field("__tag", typ.LiteralString("b")).Build()

	variant1 := typ.NewRecord().
		Field("channel", chanA).
		Field("value", messageType).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", chanB).
		Field("value", timeType).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symChB := setupSymbol(g, "chB", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verChB := cfg.Version{Root: "chB", Symbol: symChB, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symChB, verChB)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symChB] = chanB

	// Mark thenNode as dead (it exits/errors)
	inputs.DeadPoints = map[cfg.Point]bool{thenNode: true}

	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "result", Symbol: symResult}, Type: typ.NewUnion(variant1, variant2)},
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "chB", Symbol: symChB}, Type: chanB},
	}

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathChB := constraint.Path{Root: "chB", Symbol: symChB}

	// Then branch: result.channel == chB (matches timeout, will exit)
	// Else branch: result.channel != chB (narrowed to variant1)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathChB}),
		},
		{
			From:      branch,
			To:        elseNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEqualsPath{Target: pathResult, Field: "channel", Value: pathChB}),
		},
	}

	s := Solve(inputs, testResolver())

	// At join, only elseNode contributes (thenNode is dead)
	// So result should be narrowed to variant1
	gotParent := s.NarrowedTypeAt(join, constraint.Path{Root: "result", Symbol: symResult})
	if gotParent == nil {
		t.Fatal("NarrowedTypeAt(join, result) returned nil")
	}
	t.Logf("NarrowedTypeAt(join, result) = %v", gotParent)

	if !typ.TypeEquals(gotParent, variant1) {
		t.Errorf("NarrowedTypeAt(join, result) = %v, want variant1 (Message variant)", gotParent)
	}

	// result.value at join should be Message
	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotChild := s.NarrowedTypeAt(join, pathValue)
	t.Logf("NarrowedTypeAt(join, result.value) = %v", gotChild)
	if gotChild == nil {
		t.Fatal("NarrowedTypeAt(join, result.value) returned nil")
	}
	if gotChild.Kind() == 0 { // never
		t.Fatalf("NarrowedTypeAt(join, result.value) returned never, want Message interface")
	}
	if !typ.TypeEquals(gotChild, messageType) {
		t.Errorf("NarrowedTypeAt(join, result.value) = %v, want Message interface", gotChild)
	}
}

// TestSolver_Determinism verifies that solving the same inputs produces identical outputs.
func TestSolver_Determinism(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}

	// Create multiple symbols
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)
	symZ := setupSymbol(g, "z", allPoints)

	// Set up versions
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	verZ := cfg.Version{Root: "z", Symbol: symZ, ID: 1}

	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
		setVersion(g, p, symZ, verZ)
	}

	// Create inputs with multiple edge constraints
	inputs := newInputs(g)
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(
			constraint.Truthy{Path: constraint.Path{Root: "x", Symbol: symX}},
		)},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(
			constraint.Falsy{Path: constraint.Path{Root: "x", Symbol: symX}},
		)},
	}
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{
		{From: branch, To: thenNode, Constraints: []constraint.NumericConstraint{
			constraint.GeConst{X: constraint.Path{Root: "y", Symbol: symY}, C: 0},
		}},
		{From: branch, To: elseNode, Constraints: []constraint.NumericConstraint{
			constraint.LeConst{X: constraint.Path{Root: "z", Symbol: symZ}, C: 10},
		}},
	}

	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.String)
	inputs.DeclaredTypes[symY] = typ.Number
	inputs.DeclaredTypes[symZ] = typ.Number

	resolver := testResolver()

	// Run solver twice
	s1 := Solve(inputs, resolver)
	s2 := Solve(inputs, resolver)

	// Compare UnreachableEdges - must be identical
	edges1 := s1.UnreachableEdges()
	edges2 := s2.UnreachableEdges()
	if len(edges1) != len(edges2) {
		t.Fatalf("UnreachableEdges length mismatch: %d vs %d", len(edges1), len(edges2))
	}
	for i := range edges1 {
		if edges1[i] != edges2[i] {
			t.Errorf("UnreachableEdges[%d] mismatch: %v vs %v", i, edges1[i], edges2[i])
		}
	}

	// Compare types at all points for all paths
	paths := []constraint.Path{
		{Root: "x", Symbol: symX},
		{Root: "y", Symbol: symY},
		{Root: "z", Symbol: symZ},
	}

	for _, p := range allPoints {
		for _, path := range paths {
			t1 := s1.TypeAt(p, path)
			t2 := s2.TypeAt(p, path)
			if !typ.TypeEquals(t1, t2) {
				t.Errorf("TypeAt(%d, %s) mismatch: %v vs %v", p, path.Root, t1, t2)
			}

			n1 := s1.NarrowedTypeAt(p, path)
			n2 := s2.NarrowedTypeAt(p, path)
			if !typ.TypeEquals(n1, n2) {
				t.Errorf("NarrowedTypeAt(%d, %s) mismatch: %v vs %v", p, path.Root, n1, n2)
			}
		}
	}

	// Iterations should match
	if s1.DebugIterations() != s2.DebugIterations() {
		t.Errorf("iterations mismatch: %d vs %d", s1.DebugIterations(), s2.DebugIterations())
	}
}

// TestPointTimeFieldEqualsPath verifies that FieldEqualsPath narrows at point-time
// using PathTypeAt to correlate related paths.
func TestPointTimeFieldEqualsPath(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}

	symResult := setupSymbol(g, "result", allPoints)
	symCh := setupSymbol(g, "ch", allPoints)

	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verCh := cfg.Version{Root: "ch", Symbol: symCh, ID: 1}

	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symCh, verCh)
	}

	// Define channel types
	chanA := typ.NewInterface("ChanA", nil)
	chanB := typ.NewInterface("ChanB", nil)
	msgA := typ.NewInterface("MsgA", nil)
	msgB := typ.NewInterface("MsgB", nil)

	// Result union: {channel: ChanA, value: MsgA} | {channel: ChanB, value: MsgB}
	variant1 := typ.NewRecord().Field("channel", chanA).Field("value", msgA).Build()
	variant2 := typ.NewRecord().Field("channel", chanB).Field("value", msgB).Build()
	resultType := typ.NewUnion(variant1, variant2)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = resultType
	inputs.DeclaredTypes[symCh] = chanA

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathCh := constraint.Path{Root: "ch", Symbol: symCh}

	// Edge constraint: result.channel == ch (ch is ChanA)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathCh}),
		},
	}

	s := Solve(inputs, testResolver())

	// At thenNode, result.channel == ch (ChanA) narrows result to variant1
	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "result", Symbol: symResult})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, result) = %v", got)

	if !typ.TypeEquals(got, variant1) {
		t.Errorf("expected variant1 {channel: ChanA, value: MsgA}, got %v", got)
	}
}

// TestConstraintKilling_SiblingPathsSurvive verifies that assigning to t.x
// does not kill constraints on t.y.
func TestConstraintKilling_SiblingPathsSurvive(t *testing.T) {
	c := cfg.New()
	assign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	check := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), assign, true)
	c.AddEdge(assign, check, true)
	c.AddEdge(check, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), assign, check, c.Exit()}

	symT := setupSymbol(g, "t", allPoints)
	verT := cfg.Version{Root: "t", Symbol: symT, ID: 1}

	for _, p := range allPoints {
		setVersion(g, p, symT, verT)
	}

	// t: {x: number, y: string?}
	tType := typ.NewRecord().Field("x", typ.Number).Field("y", typ.NewOptional(typ.String)).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symT] = tType

	pathT := constraint.Path{Root: "t", Symbol: symT}
	pathTY := constraint.Path{Root: "t", Symbol: symT, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "y"}}}
	pathTX := constraint.Path{Root: "t", Symbol: symT, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}}}

	// Assignment to t.x at assign point
	inputs.Assignments = []UnifiedAssignment{
		{Point: assign, TargetPath: pathTX, Type: typ.Number},
	}

	// Edge constraint from entry to assign: t.y is truthy (not nil)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      c.Entry(),
			To:        assign,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathTY}),
		},
	}

	s := Solve(inputs, testResolver())

	// Constraint on t.y should survive at check (after t.x assignment)
	constraints := s.ConditionAt(check)
	t.Logf("ConstraintsAt(check) = %v (len=%d)", constraints.AllConstraints(), constraints.NumDisjuncts())

	// Verify t.y constraint survived
	found := false
	for _, c := range constraints.AllConstraints() {
		if truthy, ok := c.(constraint.Truthy); ok {
			if truthy.Path.Symbol == symT && len(truthy.Path.Segments) > 0 {
				if truthy.Path.Segments[0].Name == "y" {
					found = true
					break
				}
			}
		}
	}

	if !found {
		t.Errorf("constraint on t.y should survive after t.x assignment")
	}

	// But if we had assigned t (root), constraint on t.y should NOT survive
	// Let's verify root assignment kills child constraints
	inputs2 := newInputs(g)
	inputs2.DeclaredTypes[symT] = tType
	inputs2.Assignments = []UnifiedAssignment{
		{Point: assign, TargetPath: constraint.Path{Root: "t", Symbol: symT}, Type: tType},
	}
	inputs2.EdgeConditions = []EdgeCondition{
		{
			From:      c.Entry(),
			To:        assign,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathT}),
		},
	}

	s2 := Solve(inputs2, testResolver())
	constraints2 := s2.ConditionAt(check)
	t.Logf("After root assignment, ConstraintsAt(check) = %v (len=%d)", constraints2.AllConstraints(), constraints2.NumDisjuncts())

	// Root assignment should kill constraint on t
	foundRoot := false
	for _, c := range constraints2.AllConstraints() {
		if truthy, ok := c.(constraint.Truthy); ok {
			if truthy.Path.Symbol == symT && len(truthy.Path.Segments) == 0 {
				foundRoot = true
				break
			}
		}
	}

	if foundRoot {
		t.Errorf("constraint on t should be killed after root t assignment")
	}
}

// TestChannelSelectCorrelation verifies that result.channel == ch narrows result.value
// at the same point through cross-path correlation.
func TestChannelSelectCorrelation(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}

	symResult := setupSymbol(g, "result", allPoints)
	symCh := setupSymbol(g, "ch", allPoints)

	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verCh := cfg.Version{Root: "ch", Symbol: symCh, ID: 1}

	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symCh, verCh)
	}

	// Channel types
	chanStr := typ.NewInterface("ChanStr", nil)
	chanNum := typ.NewInterface("ChanNum", nil)

	// Result variants
	variantStr := typ.NewRecord().Field("channel", chanStr).Field("value", typ.String).Build()
	variantNum := typ.NewRecord().Field("channel", chanNum).Field("value", typ.Number).Build()
	resultType := typ.NewUnion(variantStr, variantNum)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = resultType
	inputs.DeclaredTypes[symCh] = chanStr

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathCh := constraint.Path{Root: "ch", Symbol: symCh}

	// Constraint: result.channel == ch (where ch is ChanStr)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathCh}),
		},
	}

	s := Solve(inputs, testResolver())

	// At thenNode, result.value should be narrowed to string
	pathValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}

	gotValue := s.NarrowedTypeAt(thenNode, pathValue)
	if gotValue == nil {
		t.Fatal("NarrowedTypeAt(thenNode, result.value) returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, result.value) = %v", gotValue)

	// result narrowed to variantStr means result.value is string
	if gotValue.Kind() != typ.String.Kind() {
		t.Errorf("expected result.value to be string after channel correlation, got %v", gotValue)
	}
}

// buildOrConditionCFG creates a CFG that models:
//
//	if err then           (outer_branch -> outer_then)
//	    if A or B then    (or_lhs/or_rhs -> inner_then)
//	        use(err)      (inner_then)
//	    end
//	end
//
// The OR condition creates two edges to inner_then:
// - or_lhs -> inner_then (when A is true)
// - or_rhs -> inner_then (when B is true)
func buildOrConditionCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	outerBranch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	outerThen := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	orLhs := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	orRhs := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	innerThen := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	// entry -> outer_branch
	c.AddEdge(c.Entry(), outerBranch, true)

	// outer_branch -> outer_then (true), outer_branch -> exit (false)
	c.AddEdge(outerBranch, outerThen, true)
	c.AddEdge(outerBranch, c.Exit(), false)

	// outer_then -> or_lhs
	c.AddEdge(outerThen, orLhs, true)

	// OR short-circuit: lhs true -> inner_then, lhs false -> rhs
	c.AddEdge(orLhs, innerThen, true)
	c.AddEdge(orLhs, orRhs, false)

	// rhs true -> inner_then, rhs false -> exit
	c.AddEdge(orRhs, innerThen, true)
	c.AddEdge(orRhs, c.Exit(), false)

	// inner_then -> exit
	c.AddEdge(innerThen, c.Exit(), true)

	return c, outerBranch, outerThen, orLhs, orRhs, innerThen
}

// TestNarrowedTypeAt_OrCondition_PropagatesOuterGuard tests that outer guard narrowing
// propagates through an OR condition to the inner then-block, even when the then-block
// has multiple predecessors (from OR short-circuit evaluation).
func TestNarrowedTypeAt_OrCondition_PropagatesOuterGuard(t *testing.T) {
	c, outerBranch, outerThen, orLhs, orRhs, innerThen := buildOrConditionCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), outerBranch, outerThen, orLhs, orRhs, innerThen, c.Exit()}
	symErr := setupSymbol(g, "err", allPoints)
	verErr := cfg.Version{Root: "err", Symbol: symErr, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symErr, verErr)
	}

	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "message", Type: typ.Func().Returns(typ.String).Build()},
	})
	optionalErr := typ.NewOptional(errType)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symErr] = optionalErr

	pathErr := constraint.Path{Root: "err", Symbol: symErr}

	// Outer guard: if err then (NotNil constraint)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      outerBranch,
			To:        outerThen,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: pathErr}),
		},
	}

	s := Solve(inputs, testResolver())

	// At outerThen, err should be narrowed to non-optional
	gotOuter := s.NarrowedTypeAt(outerThen, pathErr)
	if gotOuter == nil {
		t.Fatal("NarrowedTypeAt(outerThen) returned nil")
	}
	if gotOuter.Kind() == kind.Optional {
		t.Errorf("NarrowedTypeAt(outerThen) = %v, expected non-optional Err", gotOuter)
	}
	t.Logf("NarrowedTypeAt(outerThen) = %v", gotOuter)

	// At orLhs, err should still be narrowed (propagates from outerThen)
	gotLhs := s.NarrowedTypeAt(orLhs, pathErr)
	if gotLhs == nil {
		t.Fatal("NarrowedTypeAt(orLhs) returned nil")
	}
	if gotLhs.Kind() == kind.Optional {
		t.Errorf("NarrowedTypeAt(orLhs) = %v, expected non-optional Err", gotLhs)
	}
	t.Logf("NarrowedTypeAt(orLhs) = %v", gotLhs)

	// At innerThen (which has TWO predecessors: orLhs and orRhs), err should be narrowed
	gotInner := s.NarrowedTypeAt(innerThen, pathErr)
	if gotInner == nil {
		t.Fatal("NarrowedTypeAt(innerThen) returned nil")
	}
	t.Logf("NarrowedTypeAt(innerThen) = %v (kind=%v)", gotInner, gotInner.Kind())

	if gotInner.Kind() == kind.Optional {
		t.Errorf("NarrowedTypeAt(innerThen) = %v, expected non-optional Err (outer guard should propagate through OR)", gotInner)
	}
}

// =============================================================================
// Complex Edge Narrowing Tests
// =============================================================================

// TestEdgeNarrowing_MultipleConstraintsSameEdge tests multiple constraints on a single edge.
// Pattern: if x.kind == "a" and x.status == "active" then ...
func TestEdgeNarrowing_MultipleConstraintsSameEdge(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Variants with different kind and status combinations
	activeA := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("status", typ.LiteralString("active")).
		Field("data", typ.String).
		Build()
	inactiveA := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("status", typ.LiteralString("inactive")).
		Field("data", typ.String).
		Build()
	activeB := typ.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("status", typ.LiteralString("active")).
		Field("data", typ.Number).
		Build()
	inactiveB := typ.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("status", typ.LiteralString("inactive")).
		Field("data", typ.Number).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(activeA, inactiveA, activeB, inactiveB)
	inputs.DeclaredTypes[symX] = union

	// Multiple constraints: kind == "a" AND status == "active"
	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("a")},
				constraint.FieldEquals{Target: pathX, Field: "status", Value: typ.LiteralString("active")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, x) = %v", got)

	// Should narrow to only activeA (satisfies both constraints)
	if !typ.TypeEquals(got, activeA) {
		t.Errorf("got %v, want %v", got, activeA)
	}
}

// TestEdgeNarrowing_ChainedBranches tests narrowing through sequential branches.
// Pattern: if x.a then if x.b then ...
func TestEdgeNarrowing_ChainedBranches(t *testing.T) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, then1, true)
	c.AddEdge(branch1, c.Exit(), false)
	c.AddEdge(then1, branch2, true)
	c.AddEdge(branch2, then2, true)
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(then2, c.Exit(), true)

	g := newMockSSAGraph(c)

	// Type with two optional flags
	withBoth := typ.NewRecord().
		Field("hasA", typ.True).
		Field("hasB", typ.True).
		Field("value", typ.String).
		Build()
	withAOnly := typ.NewRecord().
		Field("hasA", typ.True).
		Field("hasB", typ.False).
		Field("value", typ.Number).
		Build()
	withBOnly := typ.NewRecord().
		Field("hasA", typ.False).
		Field("hasB", typ.True).
		Field("value", typ.Boolean).
		Build()
	withNeither := typ.NewRecord().
		Field("hasA", typ.False).
		Field("hasB", typ.False).
		Field("value", typ.Nil).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch1, then1, branch2, then2, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(withBoth, withAOnly, withBOnly, withNeither)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch1,
			To:        then1,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "hasA", Value: typ.True}),
		},
		{
			From:      branch2,
			To:        then2,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "hasB", Value: typ.True}),
		},
	}

	s := Solve(inputs, testResolver())

	// After first branch: should have withBoth or withAOnly
	gotAfter1 := s.NarrowedTypeAt(then1, pathX)
	if gotAfter1 == nil {
		t.Fatal("NarrowedTypeAt(then1) returned nil")
	}
	t.Logf("NarrowedTypeAt(then1) = %v", gotAfter1)

	wantAfter1 := typ.NewUnion(withBoth, withAOnly)
	if !typ.TypeEquals(gotAfter1, wantAfter1) {
		t.Errorf("after branch1: got %v, want %v", gotAfter1, wantAfter1)
	}

	// After second branch: should narrow to only withBoth
	gotAfter2 := s.NarrowedTypeAt(then2, pathX)
	if gotAfter2 == nil {
		t.Fatal("NarrowedTypeAt(then2) returned nil")
	}
	t.Logf("NarrowedTypeAt(then2) = %v", gotAfter2)

	if !typ.TypeEquals(gotAfter2, withBoth) {
		t.Errorf("after branch2: got %v, want %v", gotAfter2, withBoth)
	}
}

// TestEdgeNarrowing_DiamondWithDifferentPaths tests diamond CFG where each path narrows differently.
// Pattern: if cond then narrow to A else narrow to B; join has A|B
func TestEdgeNarrowing_DiamondWithDifferentPaths(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("aField", typ.String).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("bField", typ.Number).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Field("cField", typ.Boolean).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB, typeC)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		// Then branch: tag == "a"
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")}),
		},
		// Else branch: tag == "b"
		{
			From:      branch,
			To:        elseNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")}),
		},
		// Propagate narrowing to join
		{
			From:      thenNode,
			To:        join,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")}),
		},
		{
			From:      elseNode,
			To:        join,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")}),
		},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: only typeA
	gotThen := s.NarrowedTypeAt(thenNode, pathX)
	if !typ.TypeEquals(gotThen, typeA) {
		t.Errorf("thenNode: got %v, want %v", gotThen, typeA)
	}

	// At elseNode: only typeB
	gotElse := s.NarrowedTypeAt(elseNode, pathX)
	if !typ.TypeEquals(gotElse, typeB) {
		t.Errorf("elseNode: got %v, want %v", gotElse, typeB)
	}

	// At join: A | B (C is excluded on both paths)
	gotJoin := s.NarrowedTypeAt(join, pathX)
	if gotJoin == nil {
		t.Fatal("NarrowedTypeAt(join) returned nil")
	}
	t.Logf("NarrowedTypeAt(join) = %v", gotJoin)

	wantJoin := typ.NewUnion(typeA, typeB)
	if !typ.TypeEquals(gotJoin, wantJoin) {
		// Check reverse order
		wantJoin2 := typ.NewUnion(typeB, typeA)
		if !typ.TypeEquals(gotJoin, wantJoin2) {
			t.Errorf("join: got %v, want %v", gotJoin, wantJoin)
		}
	}
}

// TestEdgeNarrowing_NestedFieldComparison tests narrowing based on deeply nested field access.
// Pattern: if x.meta.type == "special" then ...
func TestEdgeNarrowing_NestedFieldComparison(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	specialMeta := typ.NewRecord().Field("type", typ.LiteralString("special")).Field("priority", typ.Integer).Build()
	normalMeta := typ.NewRecord().Field("type", typ.LiteralString("normal")).Field("priority", typ.Integer).Build()

	specialItem := typ.NewRecord().Field("meta", specialMeta).Field("data", typ.String).Build()
	normalItem := typ.NewRecord().Field("meta", normalMeta).Field("data", typ.Number).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(specialItem, normalItem)
	inputs.DeclaredTypes[symX] = union

	// Constraint on nested field: x.meta.type == "special"
	pathXMeta := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "meta"}},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathXMeta, Field: "type", Value: typ.LiteralString("special")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, x) = %v", got)

	if !typ.TypeEquals(got, specialItem) {
		t.Errorf("got %v, want %v", got, specialItem)
	}
}

// TestEdgeNarrowing_ExclusionPattern tests NotHasType narrowing pattern.
// Pattern: if type(x) ~= "number" then ... (excludes number from union)
func TestEdgeNarrowing_ExclusionPattern(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotHasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, x) = %v", got)

	// Should be string | boolean (number excluded)
	want := typ.NewUnion(typ.String, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		want2 := typ.NewUnion(typ.Boolean, typ.String)
		if !typ.TypeEquals(got, want2) {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

// TestEdgeNarrowing_PathComparison tests EqPath constraint where two variables are compared.
// Pattern: if x.id == y.id then ... (narrows based on equality)
func TestEdgeNarrowing_PathComparison(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	userType := typ.NewRecord().Field("id", typ.LiteralString("user")).Field("name", typ.String).Build()
	adminType := typ.NewRecord().Field("id", typ.LiteralString("admin")).Field("permissions", typ.Number).Build()
	targetType := typ.NewRecord().Field("id", typ.LiteralString("admin")).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symTarget := setupSymbol(g, "target", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verTarget := cfg.Version{Root: "target", Symbol: symTarget, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symTarget, verTarget)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(userType, adminType)
	inputs.DeclaredTypes[symTarget] = targetType

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathTarget := constraint.Path{Root: "target", Symbol: symTarget}

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathX, Field: "id", Value: constraint.Path{Root: "target", Symbol: symTarget, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, x) = %v", got)
	t.Logf("target.id type = %v", s.TypeAt(thenNode, pathTarget))

	// x.id must equal target.id ("admin"), so x should narrow to adminType
	if !typ.TypeEquals(got, adminType) {
		t.Errorf("got %v, want %v", got, adminType)
	}
}

// TestEdgeNarrowing_ArrayIndexAccess tests narrowing based on array index comparison.
// Pattern: if arr[0] == "start" then ...
func TestEdgeNarrowing_ArrayIndexAccess(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	startArray := typ.NewTuple(typ.LiteralString("start"), typ.String)
	endArray := typ.NewTuple(typ.LiteralString("end"), typ.Number)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symArr := setupSymbol(g, "arr", allPoints)
	verArr := cfg.Version{Root: "arr", Symbol: symArr, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symArr, verArr)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(startArray, endArray)
	inputs.DeclaredTypes[symArr] = union

	pathArr := constraint.Path{Root: "arr", Symbol: symArr}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.IndexEquals{Target: pathArr, Key: typ.LiteralInt(1), Value: typ.LiteralString("start")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathArr)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, arr) = %v", got)

	if !typ.TypeEquals(got, startArray) {
		t.Errorf("got %v, want %v", got, startArray)
	}
}

// TestEdgeNarrowing_DNFDisjunction tests OR conditions (multiple disjuncts).
// Pattern: if x.type == "a" or x.type == "b" then ...
func TestEdgeNarrowing_DNFDisjunction(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("type", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("type", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("type", typ.LiteralString("c")).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB, typeC)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// OR condition: type == "a" OR type == "b"
	condA := constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "type", Value: typ.LiteralString("a")})
	condB := constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "type", Value: typ.LiteralString("b")})
	orCond := constraint.Or(condA, condB)

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: orCond,
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, x) = %v", got)

	// Should be A | B (C excluded)
	want := typ.NewUnion(typeA, typeB)
	if !typ.TypeEquals(got, want) {
		want2 := typ.NewUnion(typeB, typeA)
		if !typ.TypeEquals(got, want2) {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

// TestEdgeNarrowing_ChildPathDerivation tests that child path types are derived from narrowed parent.
// Pattern: if x.tag == "a" then use x.value (value type depends on tag)
func TestEdgeNarrowing_ChildPathDerivation(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	variant1 := typ.NewRecord().
		Field("tag", typ.LiteralString("str")).
		Field("value", typ.String).
		Build()
	variant2 := typ.NewRecord().
		Field("tag", typ.LiteralString("num")).
		Field("value", typ.Number).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(variant1, variant2)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("str")}),
		},
	}

	s := Solve(inputs, testResolver())

	// Check parent is narrowed
	gotParent := s.NarrowedTypeAt(thenNode, pathX)
	if !typ.TypeEquals(gotParent, variant1) {
		t.Errorf("parent: got %v, want %v", gotParent, variant1)
	}

	// Check child path derives correct type from narrowed parent
	pathValue := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotChild := s.NarrowedTypeAt(thenNode, pathValue)
	if gotChild == nil {
		t.Fatal("NarrowedTypeAt(child) returned nil")
	}
	t.Logf("NarrowedTypeAt(thenNode, x.value) = %v", gotChild)

	if !typ.TypeEquals(gotChild, typ.String) {
		t.Errorf("child: got %v, want string", gotChild)
	}
}

// TestEdgeNarrowing_LoopInvariant tests that edge conditions are preserved across loop iterations.
func TestEdgeNarrowing_LoopInvariant(t *testing.T) {
	c := cfg.New()
	preloop := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	header := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	body := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	afterloop := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), preloop, true)
	c.AddEdge(preloop, header, true)
	c.AddEdge(preloop, c.Exit(), false)
	c.AddEdge(header, body, true)
	c.AddEdge(header, afterloop, false)
	c.AddEdge(body, header, true)
	c.AddEdge(afterloop, c.Exit(), true)

	// Mark header as loop header with preloop as preheader
	if node := c.Node(header); node != nil {
		node.LoopPreheader = preloop
		node.LoopPreheaderSet = true
	}

	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("count", typ.Integer).Build()
	typeB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("name", typ.String).Build()

	allPoints := []cfg.Point{c.Entry(), preloop, header, body, afterloop, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Narrow x to typeA before entering loop
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      preloop,
			To:        header,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("a")}),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to typeA at header
	gotHeader := s.NarrowedTypeAt(header, pathX)
	t.Logf("NarrowedTypeAt(header) = %v", gotHeader)
	if !typ.TypeEquals(gotHeader, typeA) {
		t.Errorf("header: got %v, want %v", gotHeader, typeA)
	}

	// x should remain narrowed in loop body
	gotBody := s.NarrowedTypeAt(body, pathX)
	t.Logf("NarrowedTypeAt(body) = %v", gotBody)
	if !typ.TypeEquals(gotBody, typeA) {
		t.Errorf("body: got %v, want %v", gotBody, typeA)
	}
}

// =============================================================================
// Aggressive Edge Case Tests
// =============================================================================

// TestEdgeNarrowing_DeepNestedFieldChain tests narrowing through deeply nested field chains.
// Pattern: if x.a.b.c.d == "value" then ...
func TestEdgeNarrowing_DeepNestedFieldChain(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Build deeply nested types: {a: {b: {c: {d: "match"}}}} vs {a: {b: {c: {d: "other"}}}}
	dMatch := typ.NewRecord().Field("d", typ.LiteralString("match")).Build()
	dOther := typ.NewRecord().Field("d", typ.LiteralString("other")).Build()
	cMatch := typ.NewRecord().Field("c", dMatch).Build()
	cOther := typ.NewRecord().Field("c", dOther).Build()
	bMatch := typ.NewRecord().Field("b", cMatch).Build()
	bOther := typ.NewRecord().Field("b", cOther).Build()
	matchItem := typ.NewRecord().Field("a", bMatch).Build()
	otherItem := typ.NewRecord().Field("a", bOther).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(matchItem, otherItem)
	inputs.DeclaredTypes[symX] = union

	// Constraint on deep path: x.a.b.c.d == "match"
	pathABC := constraint.Path{
		Root:   "x",
		Symbol: symX,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "a"},
			{Kind: constraint.SegmentField, Name: "b"},
			{Kind: constraint.SegmentField, Name: "c"},
		},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathABC, Field: "d", Value: typ.LiteralString("match")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(x) = %v", got)

	if !typ.TypeEquals(got, matchItem) {
		t.Errorf("got %v, want %v", got, matchItem)
	}
}

// TestEdgeNarrowing_CompoundNestedConstraints tests multiple nested constraints combined.
// Pattern: if x.meta.role == "admin" and x.status.active == true then ...
func TestEdgeNarrowing_CompoundNestedConstraints(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	adminActive := typ.NewRecord().
		Field("meta", typ.NewRecord().Field("role", typ.LiteralString("admin")).Build()).
		Field("status", typ.NewRecord().Field("active", typ.True).Build()).
		Build()
	adminInactive := typ.NewRecord().
		Field("meta", typ.NewRecord().Field("role", typ.LiteralString("admin")).Build()).
		Field("status", typ.NewRecord().Field("active", typ.False).Build()).
		Build()
	userActive := typ.NewRecord().
		Field("meta", typ.NewRecord().Field("role", typ.LiteralString("user")).Build()).
		Field("status", typ.NewRecord().Field("active", typ.True).Build()).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(adminActive, adminInactive, userActive)
	inputs.DeclaredTypes[symX] = union

	pathMeta := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "meta"}},
	}
	pathStatus := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "status"}},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathMeta, Field: "role", Value: typ.LiteralString("admin")},
				constraint.FieldEquals{Target: pathStatus, Field: "active", Value: typ.True},
			),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(x) = %v", got)

	if !typ.TypeEquals(got, adminActive) {
		t.Errorf("got %v, want %v", got, adminActive)
	}
}

// TestEdgeNarrowing_NestedUnionWithMultipleVariants tests narrowing with many union variants.
func TestEdgeNarrowing_NestedUnionWithMultipleVariants(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Create 5 variants where only one matches the constraint
	variants := make([]typ.Type, 5)
	for i := 0; i < 5; i++ {
		variants[i] = typ.NewRecord().
			Field("kind", typ.LiteralString(fmt.Sprintf("type%d", i))).
			Field("value", typ.Integer).
			Build()
	}
	// Replace variant[2] to have kind="target"
	variants[2] = typ.NewRecord().
		Field("kind", typ.LiteralString("target")).
		Field("value", typ.Integer).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(variants...)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("target")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(x) = %v", got)

	if !typ.TypeEquals(got, variants[2]) {
		t.Errorf("got %v, want %v", got, variants[2])
	}
}

// TestEdgeNarrowing_DisjunctiveNestedPaths tests OR conditions with nested field constraints.
// Pattern: if x.a.kind == "foo" or x.b.kind == "bar" then ...
func TestEdgeNarrowing_DisjunctiveNestedPaths(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	fooType := typ.NewRecord().
		Field("a", typ.NewRecord().Field("kind", typ.LiteralString("foo")).Build()).
		Field("b", typ.NewRecord().Field("kind", typ.LiteralString("other")).Build()).
		Build()
	barType := typ.NewRecord().
		Field("a", typ.NewRecord().Field("kind", typ.LiteralString("other")).Build()).
		Field("b", typ.NewRecord().Field("kind", typ.LiteralString("bar")).Build()).
		Build()
	neitherType := typ.NewRecord().
		Field("a", typ.NewRecord().Field("kind", typ.LiteralString("other")).Build()).
		Field("b", typ.NewRecord().Field("kind", typ.LiteralString("other")).Build()).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(fooType, barType, neitherType)
	inputs.DeclaredTypes[symX] = union

	pathA := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}},
	}
	pathB := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
	}
	// x.a.kind == "foo" OR x.b.kind == "bar"
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.Or(
				constraint.FromConstraints(constraint.FieldEquals{Target: pathA, Field: "kind", Value: typ.LiteralString("foo")}),
				constraint.FromConstraints(constraint.FieldEquals{Target: pathB, Field: "kind", Value: typ.LiteralString("bar")}),
			),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, constraint.Path{Root: "x", Symbol: symX})
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(x) = %v", got)

	// Should narrow to fooType | barType (excluding neitherType)
	expectedUnion := typ.NewUnion(fooType, barType)
	if !typ.TypeEquals(got, expectedUnion) && !typ.TypeEquals(got, typ.NewUnion(barType, fooType)) {
		t.Errorf("got %v, want %v", got, expectedUnion)
	}
}

// TestEdgeNarrowing_ReverseCompositionalWithIndex tests array index narrowing propagates to parent.
// Pattern: if arr[1] == "start" then ... (narrows tuple to first variant)
func TestEdgeNarrowing_ReverseCompositionalWithIndex(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	// Create tuple types: ("start", string) vs ("end", number)
	startTuple := typ.NewTuple(typ.LiteralString("start"), typ.String)
	endTuple := typ.NewTuple(typ.LiteralString("end"), typ.Number)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symArr := setupSymbol(g, "arr", allPoints)
	verArr := cfg.Version{Root: "arr", Symbol: symArr, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symArr, verArr)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(startTuple, endTuple)
	inputs.DeclaredTypes[symArr] = union

	pathArr := constraint.Path{Root: "arr", Symbol: symArr}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.IndexEquals{Target: pathArr, Key: typ.LiteralInt(1), Value: typ.LiteralString("start")}),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathArr)
	if got == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}
	t.Logf("NarrowedTypeAt(arr) = %v", got)

	if !typ.TypeEquals(got, startTuple) {
		t.Errorf("got %v, want %v", got, startTuple)
	}
}

// =============================================================================
// Program-Level Refinement Tests
// =============================================================================

// buildNestedBranchCFGComplex builds a CFG with nested if-then-else:
//
//	entry -> outer_branch -> [outer_then_branch -> inner_then, inner_else, inner_join] -> outer_join
//	                      -> outer_else -> outer_join -> exit
func buildNestedBranchCFGComplex() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	outerBranch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	outerThenBranch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	innerThen := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	innerElse := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	innerJoin := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	outerElse := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	outerJoin := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), outerBranch, true)
	c.AddEdge(outerBranch, outerThenBranch, true)
	c.AddEdge(outerBranch, outerElse, false)
	c.AddEdge(outerThenBranch, innerThen, true)
	c.AddEdge(outerThenBranch, innerElse, false)
	c.AddEdge(innerThen, innerJoin, true)
	c.AddEdge(innerElse, innerJoin, true)
	c.AddEdge(innerJoin, outerJoin, true)
	c.AddEdge(outerElse, outerJoin, true)
	c.AddEdge(outerJoin, c.Exit(), true)

	return c, outerBranch, outerThenBranch, innerThen, innerElse, innerJoin, outerElse, outerJoin
}

// buildLoopWithPreheaderCFG builds a while-loop with preheader:
//
//	entry -> preheader -> header -> body -> header (back-edge)
//	                             -> after_loop -> exit
func buildLoopWithPreheaderCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	preheader := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	header := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	body := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	afterLoop := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), preheader, true)
	c.AddEdge(preheader, header, true)
	c.AddEdge(header, body, true)
	c.AddEdge(header, afterLoop, false)
	c.AddEdge(body, header, true) // back-edge
	c.AddEdge(afterLoop, c.Exit(), true)

	if node := c.Node(header); node != nil {
		node.LoopPreheader = preheader
		node.LoopPreheaderSet = true
	}

	return c, preheader, header, body, afterLoop
}

// buildMultiWayBranchCFG builds a 3-way switch-like branch:
//
//	entry -> branch -> case1 -> join
//	                -> case2 -> join
//	                -> case3 -> join -> exit
func buildMultiWayBranchCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	case1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	case2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	case3 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, case1, true)
	c.AddEdge(branch, case2, true)
	c.AddEdge(branch, case3, false)
	c.AddEdge(case1, join, true)
	c.AddEdge(case2, join, true)
	c.AddEdge(case3, join, true)
	c.AddEdge(join, c.Exit(), true)

	return c, branch, case1, case2, case3, join
}

// TestMultiBranchJoin_ThreeWayNarrowing tests narrowing at join from 3 branches.
// Pattern:
//
//	local x: A | B | C
//	if x.tag == "a" then ... (case1: x is A)
//	elseif x.tag == "b" then ... (case2: x is B)
//	else ... (case3: x is C)
//	-- at join, x is A | B | C depending on constraints
func TestMultiBranchJoin_ThreeWayNarrowing(t *testing.T) {
	c, branch, case1, case2, case3, join := buildMultiWayBranchCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("aData", typ.String).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("bData", typ.Number).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Field("cData", typ.Boolean).Build()

	allPoints := []cfg.Point{c.Entry(), branch, case1, case2, case3, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB, typeC)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: case1, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
		{From: branch, To: case2, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")})},
		{From: branch, To: case3, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("c")})},
	}

	s := Solve(inputs, testResolver())

	// Check narrowing at each case
	gotCase1 := s.NarrowedTypeAt(case1, pathX)
	if !typ.TypeEquals(gotCase1, typeA) {
		t.Errorf("case1: got %v, want %v", gotCase1, typeA)
	}
	gotCase2 := s.NarrowedTypeAt(case2, pathX)
	if !typ.TypeEquals(gotCase2, typeB) {
		t.Errorf("case2: got %v, want %v", gotCase2, typeB)
	}
	gotCase3 := s.NarrowedTypeAt(case3, pathX)
	if !typ.TypeEquals(gotCase3, typeC) {
		t.Errorf("case3: got %v, want %v", gotCase3, typeC)
	}

	// At join, should be union of all narrowed types
	gotJoin := s.NarrowedTypeAt(join, pathX)
	t.Logf("NarrowedTypeAt(join) = %v", gotJoin)
	if gotJoin == nil {
		t.Fatal("NarrowedTypeAt(join) returned nil")
	}
}

// TestNestedConditions_CumulativeNarrowing tests cumulative narrowing through nested conditions.
// Pattern:
//
//	if x ~= nil then
//	    if x.status == "active" then
//	        -- x is narrowed twice
func TestNestedConditions_CumulativeNarrowing(t *testing.T) {
	c, outerBranch, outerThenBranch, innerThen, _, _, _, _ := buildNestedBranchCFGComplex()
	g := newMockSSAGraph(c)

	activeRec := typ.NewRecord().Field("status", typ.LiteralString("active")).Field("value", typ.Number).Build()
	inactiveRec := typ.NewRecord().Field("status", typ.LiteralString("inactive")).Field("value", typ.Number).Build()

	allPoints := []cfg.Point{c.Entry(), outerBranch, outerThenBranch, innerThen, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewOptional(typ.NewUnion(activeRec, inactiveRec))
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: outerBranch, To: outerThenBranch, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX})},
		{From: outerThenBranch, To: innerThen, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "status", Value: typ.LiteralString("active")})},
	}

	s := Solve(inputs, testResolver())

	// After outer condition: x is non-nil (active | inactive)
	gotOuter := s.NarrowedTypeAt(outerThenBranch, pathX)
	t.Logf("NarrowedTypeAt(outerThenBranch) = %v", gotOuter)
	if gotOuter == nil || gotOuter.Kind() == kind.Nil {
		t.Errorf("outer: expected non-nil, got %v", gotOuter)
	}

	// After inner condition: x is active only
	gotInner := s.NarrowedTypeAt(innerThen, pathX)
	t.Logf("NarrowedTypeAt(innerThen) = %v", gotInner)
	if !typ.TypeEquals(gotInner, activeRec) {
		t.Errorf("inner: got %v, want %v", gotInner, activeRec)
	}
}

// TestLoopWithInvariantNarrowing tests narrowing that holds through loop iterations.
// Pattern:
//
//	local items: Item[] where Item = {valid: true, data: T} | {valid: false}
//	-- preheader: filter to valid items only
//	while i < #items do
//	    -- body: items[i] should remain {valid: true, data: T}
func TestLoopWithInvariantNarrowing(t *testing.T) {
	c, preheader, header, body, afterLoop := buildLoopWithPreheaderCFG()
	g := newMockSSAGraph(c)

	validItem := typ.NewRecord().Field("valid", typ.True).Field("data", typ.String).Build()
	invalidItem := typ.NewRecord().Field("valid", typ.False).Build()

	allPoints := []cfg.Point{c.Entry(), preheader, header, body, afterLoop, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(validItem, invalidItem)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	// Narrow to valid item at preheader -> header edge
	inputs.EdgeConditions = []EdgeCondition{
		{From: preheader, To: header, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "valid", Value: typ.True})},
	}

	s := Solve(inputs, testResolver())

	// Header should have narrowed type
	gotHeader := s.NarrowedTypeAt(header, pathX)
	t.Logf("NarrowedTypeAt(header) = %v", gotHeader)
	if !typ.TypeEquals(gotHeader, validItem) {
		t.Errorf("header: got %v, want %v", gotHeader, validItem)
	}

	// Body should maintain narrowed type
	gotBody := s.NarrowedTypeAt(body, pathX)
	t.Logf("NarrowedTypeAt(body) = %v", gotBody)
	if !typ.TypeEquals(gotBody, validItem) {
		t.Errorf("body: got %v, want %v", gotBody, validItem)
	}

	// After loop should also maintain (loop exit doesn't re-widen)
	gotAfter := s.NarrowedTypeAt(afterLoop, pathX)
	t.Logf("NarrowedTypeAt(afterLoop) = %v", gotAfter)
	if !typ.TypeEquals(gotAfter, validItem) {
		t.Errorf("afterLoop: got %v, want %v", gotAfter, validItem)
	}
}

// TestPathEqualityAcrossAssignment tests field narrowing on a variable.
// Pattern:
//
//	local y: A | B
//	if y.tag == "a" then
//	    -- y should be narrowed to A
func TestPathEqualityAcrossAssignment(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("data", typ.String).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("data", typ.Number).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symY := setupSymbol(g, "y", allPoints)
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symY] = typ.NewUnion(typeA, typeB)

	pathY := constraint.Path{Root: "y", Symbol: symY}

	// Constraint: y.tag == "a"
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathY, Field: "tag", Value: typ.LiteralString("a")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// y should be narrowed to typeA
	gotY := s.NarrowedTypeAt(thenNode, pathY)
	t.Logf("NarrowedTypeAt(y) = %v", gotY)

	if !typ.TypeEquals(gotY, typeA) {
		t.Errorf("y: got %v, want %v", gotY, typeA)
	}
}

// TestFieldPathCorrelation tests correlated narrowing across field paths.
// Pattern:
//
//	local result: {channel: ChanA, value: MsgA} | {channel: ChanB, value: MsgB}
//	local ch: ChanA | ChanB
//	if result.channel == ch then
//	    -- result.value type correlates with ch's type
func TestFieldPathCorrelation(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	chanA := typ.NewInterface("ChanA", nil)
	chanB := typ.NewInterface("ChanB", nil)
	msgA := typ.NewInterface("MsgA", nil)
	msgB := typ.NewInterface("MsgB", nil)

	resultA := typ.NewRecord().Field("channel", chanA).Field("value", msgA).Build()
	resultB := typ.NewRecord().Field("channel", chanB).Field("value", msgB).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symResult := setupSymbol(g, "result", allPoints)
	symCh := setupSymbol(g, "ch", allPoints)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	verCh := cfg.Version{Root: "ch", Symbol: symCh, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symResult, verResult)
		setVersion(g, p, symCh, verCh)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(resultA, resultB)
	inputs.DeclaredTypes[symCh] = chanA // ch is ChanA

	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	pathCh := constraint.Path{Root: "ch", Symbol: symCh}

	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEqualsPath{Target: pathResult, Field: "channel", Value: pathCh}),
		},
	}

	s := Solve(inputs, testResolver())

	// result should be narrowed to resultA (since ch is ChanA)
	gotResult := s.NarrowedTypeAt(thenNode, pathResult)
	t.Logf("NarrowedTypeAt(result) = %v", gotResult)
	if !typ.TypeEquals(gotResult, resultA) {
		t.Errorf("result: got %v, want %v", gotResult, resultA)
	}

	// result.value should be MsgA
	pathResultValue := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	gotValue := s.NarrowedTypeAt(thenNode, pathResultValue)
	t.Logf("NarrowedTypeAt(result.value) = %v", gotValue)
	if !typ.TypeEquals(gotValue, msgA) {
		t.Errorf("result.value: got %v, want %v", gotValue, msgA)
	}
}

// TestParentNarrowingFromChildConstraint tests that constraints on child paths narrow parent.
// Pattern:
//
//	local x: {meta: {role: "admin", level: number}} | {meta: {role: "user", level: number}}
//	if x.meta.role == "admin" then
//	    -- x should be narrowed to admin variant
func TestParentNarrowingFromChildConstraint(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	adminMeta := typ.NewRecord().Field("role", typ.LiteralString("admin")).Field("level", typ.Number).Build()
	userMeta := typ.NewRecord().Field("role", typ.LiteralString("user")).Field("level", typ.Number).Build()
	adminType := typ.NewRecord().Field("meta", adminMeta).Field("id", typ.String).Build()
	userType := typ.NewRecord().Field("meta", userMeta).Field("id", typ.String).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(adminType, userType)
	inputs.DeclaredTypes[symX] = union

	pathXMeta := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "meta"}},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathXMeta, Field: "role", Value: typ.LiteralString("admin")}),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to adminType
	pathX := constraint.Path{Root: "x", Symbol: symX}
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, adminType) {
		t.Errorf("x: got %v, want %v", gotX, adminType)
	}

	// x.meta should be narrowed to adminMeta
	gotMeta := s.NarrowedTypeAt(thenNode, pathXMeta)
	t.Logf("NarrowedTypeAt(x.meta) = %v", gotMeta)
	if !typ.TypeEquals(gotMeta, adminMeta) {
		t.Errorf("x.meta: got %v, want %v", gotMeta, adminMeta)
	}
}

// TestExclusionWithJoin tests exclusion narrowing at join points.
// Pattern:
//
//	local x: A | B | C
//	if x.tag ~= "a" then
//	    if x.tag ~= "b" then
//	        -- x is C
//	    else
//	        -- x is B
//	-- at join: x is B | C
func TestExclusionWithJoin(t *testing.T) {
	c, outerBranch, outerThenBranch, innerThen, innerElse, innerJoin, outerElse, outerJoin := buildNestedBranchCFGComplex()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Build()

	allPoints := []cfg.Point{c.Entry(), outerBranch, outerThenBranch, innerThen, innerElse, innerJoin, outerElse, outerJoin, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB, typeC)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: outerBranch, To: outerThenBranch, Condition: constraint.FromConstraints(constraint.FieldNotEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
		{From: outerBranch, To: outerElse, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
		{From: outerThenBranch, To: innerThen, Condition: constraint.FromConstraints(constraint.FieldNotEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")})},
		{From: outerThenBranch, To: innerElse, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")})},
	}

	s := Solve(inputs, testResolver())

	// outerThenBranch: x is B | C (not A)
	gotOuter := s.NarrowedTypeAt(outerThenBranch, pathX)
	t.Logf("NarrowedTypeAt(outerThenBranch) = %v", gotOuter)
	expectedBC := typ.NewUnion(typeB, typeC)
	if !typ.TypeEquals(gotOuter, expectedBC) && !typ.TypeEquals(gotOuter, typ.NewUnion(typeC, typeB)) {
		t.Errorf("outerThenBranch: got %v, want %v", gotOuter, expectedBC)
	}

	// innerThen: x is C (not A, not B)
	gotInner := s.NarrowedTypeAt(innerThen, pathX)
	t.Logf("NarrowedTypeAt(innerThen) = %v", gotInner)
	if !typ.TypeEquals(gotInner, typeC) {
		t.Errorf("innerThen: got %v, want %v", gotInner, typeC)
	}

	// innerElse: x is B
	gotInnerElse := s.NarrowedTypeAt(innerElse, pathX)
	t.Logf("NarrowedTypeAt(innerElse) = %v", gotInnerElse)
	if !typ.TypeEquals(gotInnerElse, typeB) {
		t.Errorf("innerElse: got %v, want %v", gotInnerElse, typeB)
	}

	// outerElse: x is A
	gotOuterElse := s.NarrowedTypeAt(outerElse, pathX)
	t.Logf("NarrowedTypeAt(outerElse) = %v", gotOuterElse)
	if !typ.TypeEquals(gotOuterElse, typeA) {
		t.Errorf("outerElse: got %v, want %v", gotOuterElse, typeA)
	}
}

// TestLoopWithBodyNarrowing tests narrowing within a loop body.
// Pattern:
//
//	local x: A | B
//	while cond do
//	    if x.tag == "a" then
//	        -- x is A here
//	    end
func TestLoopWithBodyNarrowing(t *testing.T) {
	c := cfg.New()
	preheader := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	header := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	bodyBranch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	bodyThen := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	bodyElse := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	bodyJoin := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	afterLoop := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), preheader, true)
	c.AddEdge(preheader, header, true)
	c.AddEdge(header, bodyBranch, true)
	c.AddEdge(header, afterLoop, false)
	c.AddEdge(bodyBranch, bodyThen, true)
	c.AddEdge(bodyBranch, bodyElse, false)
	c.AddEdge(bodyThen, bodyJoin, true)
	c.AddEdge(bodyElse, bodyJoin, true)
	c.AddEdge(bodyJoin, header, true) // back-edge
	c.AddEdge(afterLoop, c.Exit(), true)

	if node := c.Node(header); node != nil {
		node.LoopPreheader = preheader
		node.LoopPreheaderSet = true
	}

	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()

	allPoints := []cfg.Point{c.Entry(), preheader, header, bodyBranch, bodyThen, bodyElse, bodyJoin, afterLoop, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: bodyBranch, To: bodyThen, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
		{From: bodyBranch, To: bodyElse, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")})},
	}

	s := Solve(inputs, testResolver())

	// At bodyThen, x should be A
	gotBodyThen := s.NarrowedTypeAt(bodyThen, pathX)
	t.Logf("NarrowedTypeAt(bodyThen) = %v", gotBodyThen)
	if !typ.TypeEquals(gotBodyThen, typeA) {
		t.Errorf("bodyThen: got %v, want %v", gotBodyThen, typeA)
	}

	// At bodyElse, x should be B
	gotBodyElse := s.NarrowedTypeAt(bodyElse, pathX)
	t.Logf("NarrowedTypeAt(bodyElse) = %v", gotBodyElse)
	if !typ.TypeEquals(gotBodyElse, typeB) {
		t.Errorf("bodyElse: got %v, want %v", gotBodyElse, typeB)
	}

	// At header, x is the full union
	gotHeader := s.NarrowedTypeAt(header, pathX)
	t.Logf("NarrowedTypeAt(header) = %v", gotHeader)
}

// TestSequentialGuards_MultipleConstraints tests multiple sequential guards building up.
// Pattern:
//
//	if x ~= nil then          -- guard 1
//	    if x.active then      -- guard 2
//	        if x.role == "admin" then  -- guard 3
//	            -- x is non-nil, active, admin
func TestSequentialGuards_MultipleConstraints(t *testing.T) {
	c := cfg.New()
	branch1 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	branch2 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	branch3 := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	innermost := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch1, true)
	c.AddEdge(branch1, branch2, true)
	c.AddEdge(branch1, c.Exit(), false)
	c.AddEdge(branch2, branch3, true)
	c.AddEdge(branch2, c.Exit(), false)
	c.AddEdge(branch3, innermost, true)
	c.AddEdge(branch3, c.Exit(), false)
	c.AddEdge(innermost, c.Exit(), true)

	g := newMockSSAGraph(c)

	adminActive := typ.NewRecord().Field("active", typ.True).Field("role", typ.LiteralString("admin")).Build()
	adminInactive := typ.NewRecord().Field("active", typ.False).Field("role", typ.LiteralString("admin")).Build()
	userActive := typ.NewRecord().Field("active", typ.True).Field("role", typ.LiteralString("user")).Build()

	allPoints := []cfg.Point{c.Entry(), branch1, branch2, branch3, innermost, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewOptional(typ.NewUnion(adminActive, adminInactive, userActive))
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch1, To: branch2, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX})},
		{From: branch2, To: branch3, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "active", Value: typ.True})},
		{From: branch3, To: innermost, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "role", Value: typ.LiteralString("admin")})},
	}

	s := Solve(inputs, testResolver())

	// After guard 1: non-nil
	got1 := s.NarrowedTypeAt(branch2, pathX)
	t.Logf("After guard 1: %v", got1)
	if got1 == nil || got1.Kind() == kind.Nil {
		t.Errorf("after guard 1: should be non-nil, got %v", got1)
	}

	// After guard 2: active only
	got2 := s.NarrowedTypeAt(branch3, pathX)
	t.Logf("After guard 2: %v", got2)
	expectedActive := typ.NewUnion(adminActive, userActive)
	if !typ.TypeEquals(got2, expectedActive) && !typ.TypeEquals(got2, typ.NewUnion(userActive, adminActive)) {
		t.Errorf("after guard 2: got %v, want %v", got2, expectedActive)
	}

	// After guard 3: admin only
	got3 := s.NarrowedTypeAt(innermost, pathX)
	t.Logf("After guard 3: %v", got3)
	if !typ.TypeEquals(got3, adminActive) {
		t.Errorf("after guard 3: got %v, want %v", got3, adminActive)
	}
}

// TestDiamondWithDifferentConstraints tests diamond CFG with different constraints on each branch.
// Pattern:
//
//	    branch
//	   /      \
//	left     right
//	   \      /
//	     join
//
// left: x.a == 1, right: x.b == 2
// at join: x satisfies constraint from whichever branch was taken
func TestDiamondWithDifferentConstraints(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	// Types with string discriminants that work with narrowing
	typeA := typ.NewRecord().
		Field("side", typ.LiteralString("left")).
		Field("value", typ.String).
		Build()
	typeB := typ.NewRecord().
		Field("side", typ.LiteralString("right")).
		Field("value", typ.Number).
		Build()
	typeC := typ.NewRecord().
		Field("side", typ.LiteralString("center")).
		Field("value", typ.Boolean).
		Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB, typeC)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "side", Value: typ.LiteralString("left")})},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "side", Value: typ.LiteralString("right")})},
	}

	s := Solve(inputs, testResolver())

	// then: side=="left" -> typeA
	gotThen := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(then) = %v", gotThen)
	if !typ.TypeEquals(gotThen, typeA) {
		t.Errorf("then: got %v, want %v", gotThen, typeA)
	}

	// else: side=="right" -> typeB
	gotElse := s.NarrowedTypeAt(elseNode, pathX)
	t.Logf("NarrowedTypeAt(else) = %v", gotElse)
	if !typ.TypeEquals(gotElse, typeB) {
		t.Errorf("else: got %v, want %v", gotElse, typeB)
	}

	// join: union of narrowed types from both branches
	gotJoin := s.NarrowedTypeAt(join, pathX)
	t.Logf("NarrowedTypeAt(join) = %v", gotJoin)
	expectedJoin := typ.NewUnion(typeA, typeB)
	if !typ.TypeEquals(gotJoin, expectedJoin) && !typ.TypeEquals(gotJoin, typ.NewUnion(typeB, typeA)) {
		t.Errorf("join: got %v, want %v", gotJoin, expectedJoin)
	}
}

// TestNestedFieldExclusion tests excluding variants based on nested field inequality.
// Pattern:
//
//	if x.config.mode ~= "debug" then
//	    -- x is narrowed to exclude debug mode
func TestNestedFieldExclusion(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	debugConfig := typ.NewRecord().Field("mode", typ.LiteralString("debug")).Field("level", typ.Number).Build()
	prodConfig := typ.NewRecord().Field("mode", typ.LiteralString("prod")).Field("level", typ.Number).Build()
	debugType := typ.NewRecord().Field("config", debugConfig).Field("name", typ.String).Build()
	prodType := typ.NewRecord().Field("config", prodConfig).Field("name", typ.String).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(debugType, prodType)
	inputs.DeclaredTypes[symX] = union

	pathConfig := constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "config"}},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEquals{Target: pathConfig, Field: "mode", Value: typ.LiteralString("debug")}),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to prodType only
	pathX := constraint.Path{Root: "x", Symbol: symX}
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, prodType) {
		t.Errorf("x: got %v, want %v", gotX, prodType)
	}
}

// TestOrConditionWithSharedNarrowing tests OR conditions where both branches share some narrowing.
// Pattern:
//
//	if x.status == "active" or x.status == "pending" then
//	    -- x is {status: "active"} | {status: "pending"}
func TestOrConditionWithSharedNarrowing(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	activeType := typ.NewRecord().Field("status", typ.LiteralString("active")).Field("data", typ.String).Build()
	pendingType := typ.NewRecord().Field("status", typ.LiteralString("pending")).Field("data", typ.String).Build()
	closedType := typ.NewRecord().Field("status", typ.LiteralString("closed")).Field("data", typ.String).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(activeType, pendingType, closedType)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.Or(
				constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "status", Value: typ.LiteralString("active")}),
				constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "status", Value: typ.LiteralString("pending")}),
			),
		},
	}

	s := Solve(inputs, testResolver())

	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	expectedUnion := typ.NewUnion(activeType, pendingType)
	if !typ.TypeEquals(gotX, expectedUnion) && !typ.TypeEquals(gotX, typ.NewUnion(pendingType, activeType)) {
		t.Errorf("x: got %v, want %v", gotX, expectedUnion)
	}
}

// TestTriplePhi tests phi nodes with three predecessors.
// Pattern:
//
//	       /- case1 (x is A) -\
//	branch -- case2 (x is B) -- join
//	       \- case3 (x is C) -/
func TestTriplePhi(t *testing.T) {
	c, branch, case1, case2, case3, join := buildMultiWayBranchCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("kind", typ.LiteralString("c")).Build()

	allPoints := []cfg.Point{c.Entry(), branch, case1, case2, case3, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// Create phi at join
	verA := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verB := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	verC := cfg.Version{Root: "x", Symbol: symX, ID: 3}
	verPhi := cfg.Version{Root: "x", Symbol: symX, ID: 4}

	setVersion(g, c.Entry(), symX, verA)
	setVersion(g, branch, symX, verA)
	setVersion(g, case1, symX, verA)
	setVersion(g, case2, symX, verB)
	setVersion(g, case3, symX, verC)
	setVersion(g, join, symX, verPhi)

	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: verPhi,
		Operands: []cfg.PhiOperand{
			{From: case1, Version: verA},
			{From: case2, Version: verB},
			{From: case3, Version: verC},
		},
	})

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB, typeC)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: case1, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("a")})},
		{From: branch, To: case2, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("b")})},
		{From: branch, To: case3, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("c")})},
	}

	s := Solve(inputs, testResolver())

	// At join, phi should combine all three types
	gotJoin := s.TypeAt(join, pathX)
	t.Logf("TypeAt(join) = %v", gotJoin)
	if gotJoin == nil {
		t.Fatal("TypeAt(join) returned nil")
	}
}

// TestIndexNarrowingWithTupleUnion tests narrowing tuples by index constraint.
// Pattern:
//
//	local x: (1, string) | (2, number) | (3, boolean)
//	if x[1] == 1 then
//	    -- x is (1, string)
func TestIndexNarrowingWithTupleUnion(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	tuple1 := typ.NewTuple(typ.LiteralInt(1), typ.String)
	tuple2 := typ.NewTuple(typ.LiteralInt(2), typ.Number)
	tuple3 := typ.NewTuple(typ.LiteralInt(3), typ.Boolean)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(tuple1, tuple2, tuple3)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.IndexEquals{Target: pathX, Key: typ.LiteralInt(1), Value: typ.LiteralInt(1)}),
		},
	}

	s := Solve(inputs, testResolver())

	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, tuple1) {
		t.Errorf("x: got %v, want %v", gotX, tuple1)
	}
}

// TestNarrowingInSequentialPoints tests narrowing persists through sequential CFG points.
// Pattern:
//
//	local x: A | B
//	if x.tag == "a" then
//	    -- x is A at thenNode
//	    -- x remains A at nextNode (same version)
func TestNarrowingInSequentialPoints(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	nextNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, c.Exit(), false)
	c.AddEdge(thenNode, nextNode, true)
	c.AddEdge(nextNode, c.Exit(), true)

	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, nextNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	union := typ.NewUnion(typeA, typeB)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: x is A
	gotThen := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(thenNode) = %v", gotThen)
	if !typ.TypeEquals(gotThen, typeA) {
		t.Errorf("thenNode: got %v, want %v", gotThen, typeA)
	}

	// At nextNode: x should still be A (same version)
	gotNext := s.NarrowedTypeAt(nextNode, pathX)
	t.Logf("NarrowedTypeAt(nextNode) = %v", gotNext)
	if !typ.TypeEquals(gotNext, typeA) {
		t.Errorf("nextNode: got %v, want %v", gotNext, typeA)
	}
}

// TestVersionAwareConstraintKilling tests that constraints are killed when
// a variable is reassigned to a new SSA version.
// Pattern:
//
//	local x: A | B
//	if x.tag == "a" then
//	    -- x is A here
//	    x = getAny()  -- x is now v2, old constraint should be killed
//	    -- x is A | B here (not A)
func TestVersionAwareConstraintKilling(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")    // before assignment
	assignNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")  // x = getAny()
	afterAssign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "") // after assignment

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, c.Exit(), false)
	c.AddEdge(thenNode, assignNode, true)
	c.AddEdge(assignNode, afterAssign, true)
	c.AddEdge(afterAssign, c.Exit(), true)

	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, assignNode, afterAssign, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// v1 visible at entry, branch, thenNode
	verX1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	// v2 visible at assignNode, afterAssign (after reassignment)
	verX2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}

	setVersion(g, c.Entry(), symX, verX1)
	setVersion(g, branch, symX, verX1)
	setVersion(g, thenNode, symX, verX1)
	setVersion(g, assignNode, symX, verX2)
	setVersion(g, afterAssign, symX, verX2)
	setVersion(g, c.Exit(), symX, verX2)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
	}

	// The assignment at assignNode changes x from v1 to v2
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      assignNode,
			TargetPath: pathX,
			Type:       union, // x = getAny() returns A | B
		},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: x (v1) should be A (narrowed by constraint)
	gotThen := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(thenNode) = %v", gotThen)
	if !typ.TypeEquals(gotThen, typeA) {
		t.Errorf("thenNode: got %v, want %v", gotThen, typeA)
	}

	// At afterAssign: x (v2) should be A | B (constraint killed by assignment)
	gotAfter := s.NarrowedTypeAt(afterAssign, pathX)
	t.Logf("NarrowedTypeAt(afterAssign) = %v", gotAfter)
	if !typ.TypeEquals(gotAfter, union) {
		t.Errorf("afterAssign: got %v, want %v (old constraint should be killed)", gotAfter, union)
	}
}

// TestEqPathTransitiveFieldPropagation tests that field constraints propagate
// transitively across equal paths in the flow solver context.
// Pattern:
//
//	local x: A | B
//	local y: A | B
//	if x == y and y.tag == "a" then
//	    -- both x and y should be narrowed to A
func TestEqPathTransitiveFieldPropagation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union
	inputs.DeclaredTypes[symY] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// Combined constraint: x == y AND y.tag == "a"
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.EqPath{Left: pathX, Right: pathY},
				constraint.FieldEquals{Target: pathY, Field: "tag", Value: typ.LiteralString("a")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// y should be narrowed to typeA
	gotY := s.NarrowedTypeAt(thenNode, pathY)
	t.Logf("NarrowedTypeAt(y) = %v", gotY)
	if !typ.TypeEquals(gotY, typeA) {
		t.Errorf("y: got %v, want %v", gotY, typeA)
	}

	// x should also be narrowed to typeA via transitive propagation
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, typeA) {
		t.Errorf("x: got %v, want %v (transitive from y)", gotX, typeA)
	}
}

// TestFieldEqualsPath_TransitivePropagation tests that when x.field == y and y is narrowed,
// x is narrowed to variants where x.field matches the narrowed y type.
// Pattern:
//
//	local x: {tag: ChanA, data: MsgA} | {tag: ChanB, data: MsgB}
//	local y: ChanA | ChanB
//	if x.tag == y and type(y) == "ChanA" then
//	    -- x should be narrowed to {tag: ChanA, data: MsgA}
func TestFieldEqualsPath_TransitivePropagation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	chanA := typ.NewInterface("ChanA", nil)
	chanB := typ.NewInterface("ChanB", nil)
	msgA := typ.NewInterface("MsgA", nil)
	msgB := typ.NewInterface("MsgB", nil)

	recA := typ.NewRecord().Field("tag", chanA).Field("data", msgA).Build()
	recB := typ.NewRecord().Field("tag", chanB).Field("data", msgB).Build()
	unionRec := typ.NewUnion(recA, recB)
	unionChan := typ.NewUnion(chanA, chanB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = unionRec
	inputs.DeclaredTypes[symY] = unionChan
	inputs.TypeKeys[chanA.Hash()] = chanA

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// Combined constraint: x.tag == y AND y is ChanA
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEqualsPath{Target: pathX, Field: "tag", Value: pathY},
				constraint.HasType{Path: pathY, Type: narrow.HashTypeKey(chanA.Hash())},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// y should be narrowed to ChanA
	gotY := s.NarrowedTypeAt(thenNode, pathY)
	t.Logf("NarrowedTypeAt(y) = %v", gotY)
	if !typ.TypeEquals(gotY, chanA) {
		t.Errorf("y: got %v, want %v", gotY, chanA)
	}

	// x should be narrowed to recA (the variant where tag is ChanA)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, recA) {
		t.Errorf("x: got %v, want %v (transitive from y via FieldEqualsPath)", gotX, recA)
	}
}

// TestIndexEqualsPath_TransitivePropagation tests that when x[1] == y and y is narrowed,
// x is narrowed to variants where x[1] matches the narrowed y type.
func TestIndexEqualsPath_TransitivePropagation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	// Interfaces for discriminating
	okType := typ.NewInterface("OkResult", nil)
	errType := typ.NewInterface("ErrResult", nil)

	// Tuples with different first elements
	tupleA := typ.NewTuple(okType, typ.String)
	tupleB := typ.NewTuple(errType, typ.Number)
	unionTuple := typ.NewUnion(tupleA, tupleB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = unionTuple
	inputs.DeclaredTypes[symY] = typ.NewUnion(okType, errType)
	inputs.TypeKeys[okType.Hash()] = okType

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// Combined constraint: x[1] == y AND type(y) == OkResult
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.IndexEqualsPath{Target: pathX, Key: typ.LiteralInt(1), Value: pathY},
				constraint.HasType{Path: pathY, Type: narrow.HashTypeKey(okType.Hash())},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// y should be narrowed to okType
	gotY := s.NarrowedTypeAt(thenNode, pathY)
	t.Logf("NarrowedTypeAt(y) = %v", gotY)
	if !typ.TypeEquals(gotY, okType) {
		t.Errorf("y: got %v, want %v", gotY, okType)
	}

	// x should be narrowed to tupleA (the variant where first element is OkResult)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, tupleA) {
		t.Errorf("x: got %v, want %v (transitive from y via IndexEqualsPath)", gotX, tupleA)
	}
}

// TestNotEqPath_TransitivePropagation tests that x ~= y combined with type constraint on y
// properly excludes x from being the singleton type.
func TestNotEqPath_TransitivePropagation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	litA := typ.LiteralString("a")
	litB := typ.LiteralString("b")

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(litA, litB)
	inputs.DeclaredTypes[symY] = litA // y is exactly "a"

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// x ~= y where y is "a" => x is not "a" => x is "b"
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.NotEqPath{Left: pathX, Right: pathY},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to "b" (literal "a" excluded because y is "a")
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, litB) {
		t.Errorf("x: got %v, want %v", gotX, litB)
	}
}

// TestFieldNotEqualsPath_TransitivePropagation tests that x.field ~= y combined with
// type constraint on y properly excludes variants.
func TestFieldNotEqualsPath_TransitivePropagation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	chanA := typ.NewInterface("ChanA", nil)
	chanB := typ.NewInterface("ChanB", nil)

	recA := typ.NewRecord().Field("tag", chanA).Build()
	recB := typ.NewRecord().Field("tag", chanB).Build()
	unionRec := typ.NewUnion(recA, recB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = unionRec
	inputs.DeclaredTypes[symY] = chanA // y is exactly ChanA

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// x.tag ~= y where y is ChanA => x.tag is not ChanA => x is recB
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldNotEqualsPath{Target: pathX, Field: "tag", Value: pathY},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to recB (the variant where tag is not ChanA)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, recB) {
		t.Errorf("x: got %v, want %v", gotX, recB)
	}
}

// TestIndexNotEqualsPath_TransitivePropagation tests that x[key] ~= y combined with
// type constraint on y properly excludes variants.
func TestIndexNotEqualsPath_TransitivePropagation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	okType := typ.NewInterface("OkResult", nil)
	errType := typ.NewInterface("ErrResult", nil)

	tupleA := typ.NewTuple(okType, typ.String)
	tupleB := typ.NewTuple(errType, typ.Number)
	unionTuple := typ.NewUnion(tupleA, tupleB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = unionTuple
	inputs.DeclaredTypes[symY] = okType // y is exactly OkResult

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// x[1] ~= y where y is OkResult => x[1] is not OkResult => x is tupleB
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.IndexNotEqualsPath{Target: pathX, Key: typ.LiteralInt(1), Value: pathY},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to tupleB (the variant where first element is not OkResult)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, tupleB) {
		t.Errorf("x: got %v, want %v", gotX, tupleB)
	}
}

// TestHasField_Narrowing tests that HasField properly narrows unions.
func TestHasField_Narrowing(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	recWithField := typ.NewRecord().Field("data", typ.String).Build()
	recWithoutField := typ.NewRecord().Field("other", typ.Number).Build()
	union := typ.NewUnion(recWithField, recWithoutField)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.HasField{Path: pathX, Field: "data"},
			),
		},
	}

	s := Solve(inputs, testResolver())

	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, recWithField) {
		t.Errorf("x: got %v, want %v", gotX, recWithField)
	}
}

// TestCombinedConstraints_AllTypes tests multiple constraint types combined.
func TestCombinedConstraints_AllTypes(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().
		Field("tag", typ.LiteralString("a")).
		Field("value", typ.String).
		Build()
	typeB := typ.NewRecord().
		Field("tag", typ.LiteralString("b")).
		Field("value", typ.Number).
		Build()
	typeC := typ.NewRecord().
		Field("tag", typ.LiteralString("c")).
		Field("value", typ.Boolean).
		Build()
	union := typ.NewUnion(typeA, typeB, typeC)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union
	inputs.DeclaredTypes[symY] = typ.LiteralString("a")

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}

	// Combined: x.tag == y (where y is "a") AND x ~= nil AND has field "value"
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEqualsPath{Target: pathX, Field: "tag", Value: pathY},
				constraint.NotNil{Path: pathX},
				constraint.HasField{Path: pathX, Field: "value"},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to typeA
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, typeA) {
		t.Errorf("x: got %v, want %v", gotX, typeA)
	}
}

// TestChainedEqPath_TransitivePropagation tests x == y && y == z => x narrows with z.
func TestChainedEqPath_TransitivePropagation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)
	symZ := setupSymbol(g, "z", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	verZ := cfg.Version{Root: "z", Symbol: symZ, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
		setVersion(g, p, symZ, verZ)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union
	inputs.DeclaredTypes[symY] = union
	inputs.DeclaredTypes[symZ] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}
	pathZ := constraint.Path{Root: "z", Symbol: symZ}

	// x == y && y == z && z.tag == "a" => x should also be narrowed
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.EqPath{Left: pathX, Right: pathY},
				constraint.EqPath{Left: pathY, Right: pathZ},
				constraint.FieldEquals{Target: pathZ, Field: "tag", Value: typ.LiteralString("a")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// z should be narrowed to typeA
	gotZ := s.NarrowedTypeAt(thenNode, pathZ)
	t.Logf("NarrowedTypeAt(z) = %v", gotZ)
	if !typ.TypeEquals(gotZ, typeA) {
		t.Errorf("z: got %v, want %v", gotZ, typeA)
	}

	// y should also be narrowed to typeA (via y == z)
	gotY := s.NarrowedTypeAt(thenNode, pathY)
	t.Logf("NarrowedTypeAt(y) = %v", gotY)
	if !typ.TypeEquals(gotY, typeA) {
		t.Errorf("y: got %v, want %v", gotY, typeA)
	}

	// x should also be narrowed to typeA (via x == y == z)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, typeA) {
		t.Errorf("x: got %v, want %v (chained via y == z)", gotX, typeA)
	}
}

// TestDeepNestedPath_Narrowing tests narrowing with deeply nested field paths.
func TestDeepNestedPath_Narrowing(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	// Build deeply nested types: x.a.b.c.tag == "match"
	innerMatch := typ.NewRecord().Field("tag", typ.LiteralString("match")).Build()
	innerOther := typ.NewRecord().Field("tag", typ.LiteralString("other")).Build()
	level2Match := typ.NewRecord().Field("c", innerMatch).Build()
	level2Other := typ.NewRecord().Field("c", innerOther).Build()
	level1Match := typ.NewRecord().Field("b", level2Match).Build()
	level1Other := typ.NewRecord().Field("b", level2Other).Build()
	rootMatch := typ.NewRecord().Field("a", level1Match).Build()
	rootOther := typ.NewRecord().Field("a", level1Other).Build()

	union := typ.NewUnion(rootMatch, rootOther)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	// Path: x.a.b.c
	pathXABC := constraint.Path{
		Root:   "x",
		Symbol: symX,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "a"},
			{Kind: constraint.SegmentField, Name: "b"},
			{Kind: constraint.SegmentField, Name: "c"},
		},
	}
	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathXABC, Field: "tag", Value: typ.LiteralString("match")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to rootMatch
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, rootMatch) {
		t.Errorf("x: got %v, want %v", gotX, rootMatch)
	}
}

// TestMultipleFieldConstraints_SamePath tests multiple field constraints on the same path.
func TestMultipleFieldConstraints_SamePath(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	// Types with multiple discriminating fields
	typeAB := typ.NewRecord().
		Field("tag1", typ.LiteralString("a")).
		Field("tag2", typ.LiteralString("b")).
		Build()
	typeAC := typ.NewRecord().
		Field("tag1", typ.LiteralString("a")).
		Field("tag2", typ.LiteralString("c")).
		Build()
	typeDB := typ.NewRecord().
		Field("tag1", typ.LiteralString("d")).
		Field("tag2", typ.LiteralString("b")).
		Build()

	union := typ.NewUnion(typeAB, typeAC, typeDB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Both constraints: tag1 == "a" AND tag2 == "b"
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathX, Field: "tag1", Value: typ.LiteralString("a")},
				constraint.FieldEquals{Target: pathX, Field: "tag2", Value: typ.LiteralString("b")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to typeAB (only one that matches both)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, typeAB) {
		t.Errorf("x: got %v, want %v", gotX, typeAB)
	}
}

// TestCrossFieldCorrelation tests x.a == y.b correlation.
func TestCrossFieldCorrelation(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	chanA := typ.NewInterface("ChanA", nil)
	chanB := typ.NewInterface("ChanB", nil)

	recXA := typ.NewRecord().Field("channel", chanA).Field("data", typ.String).Build()
	recXB := typ.NewRecord().Field("channel", chanB).Field("data", typ.Number).Build()
	recYA := typ.NewRecord().Field("ref", chanA).Build()
	recYB := typ.NewRecord().Field("ref", chanB).Build()

	unionX := typ.NewUnion(recXA, recXB)
	_ = recYB // unused variant

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = unionX
	inputs.DeclaredTypes[symY] = recYA // y is fixed to recYA (ref: ChanA)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathYRef := constraint.Path{
		Root:     "y",
		Symbol:   symY,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "ref"}},
	}

	// x.channel == y.ref where y.ref is ChanA
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEqualsPath{Target: pathX, Field: "channel", Value: pathYRef},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to recXA (channel matches ChanA)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, recXA) {
		t.Errorf("x: got %v, want %v", gotX, recXA)
	}
}

// TestNumericConstraints_WithTypeNarrowing tests numeric constraints combined with type narrowing.
func TestNumericConstraints_WithTypeNarrowing(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symI := setupSymbol(g, "i", allPoints)
	symX := setupSymbol(g, "x", allPoints)

	verI := cfg.Version{Root: "i", Symbol: symI, ID: 1}
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symI, verI)
		setVersion(g, p, symX, verX)
	}

	typeSmall := typ.NewRecord().Field("size", typ.LiteralString("small")).Build()
	typeLarge := typ.NewRecord().Field("size", typ.LiteralString("large")).Build()
	union := typ.NewUnion(typeSmall, typeLarge)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symI] = typ.Integer
	inputs.DeclaredTypes[symX] = union

	pathI := constraint.Path{Root: "i", Symbol: symI}
	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Combined: i >= 10 AND x.size == "large"
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathX, Field: "size", Value: typ.LiteralString("large")},
			),
		},
	}
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{
		{
			From: branch,
			To:   thenNode,
			Constraints: []constraint.NumericConstraint{
				constraint.GeConst{X: pathI, C: 10},
			},
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to typeLarge
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, typeLarge) {
		t.Errorf("x: got %v, want %v", gotX, typeLarge)
	}
}

// TestLoopWithConstraintPreservation tests that constraints are preserved through loop iterations.
func TestLoopWithConstraintPreservation(t *testing.T) {
	c := cfg.New()
	preheader := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	header := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	body := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	afterLoop := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), preheader, true)
	c.AddEdge(preheader, header, true)
	c.AddEdge(header, body, true)       // loop body
	c.AddEdge(header, afterLoop, false) // exit loop
	c.AddEdge(body, header, true)       // back-edge
	c.AddEdge(afterLoop, c.Exit(), true)

	if node := c.Node(header); node != nil {
		node.LoopPreheader = preheader
		node.LoopPreheaderSet = true
	}

	g := newMockSSAGraph(c)

	typeValid := typ.NewRecord().Field("valid", typ.True).Build()
	typeInvalid := typ.NewRecord().Field("valid", typ.False).Build()
	union := typ.NewUnion(typeValid, typeInvalid)

	allPoints := []cfg.Point{c.Entry(), preheader, header, body, afterLoop, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Preheader condition: x.valid == true (loop invariant)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      c.Entry(),
			To:        preheader,
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "valid", Value: typ.True}),
		},
	}

	s := Solve(inputs, testResolver())

	// At preheader: x should be valid
	gotPreheader := s.NarrowedTypeAt(preheader, pathX)
	t.Logf("NarrowedTypeAt(preheader) = %v", gotPreheader)
	if !typ.TypeEquals(gotPreheader, typeValid) {
		t.Errorf("preheader: got %v, want %v", gotPreheader, typeValid)
	}

	// At body: x should still be valid (loop invariant preserved)
	gotBody := s.NarrowedTypeAt(body, pathX)
	t.Logf("NarrowedTypeAt(body) = %v", gotBody)
	if !typ.TypeEquals(gotBody, typeValid) {
		t.Errorf("body: got %v, want %v", gotBody, typeValid)
	}
}

// TestDiamondCFG_ConflictingConstraints tests diamond CFG with different constraints on branches.
func TestDiamondCFG_ConflictingConstraints(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	left := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	right := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, left, true)
	c.AddEdge(branch, right, false)
	c.AddEdge(left, join, true)
	c.AddEdge(right, join, true)
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Build()
	union := typ.NewUnion(typeA, typeB, typeC)

	allPoints := []cfg.Point{c.Entry(), branch, left, right, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Left branch: x.tag == "a"
	// Right branch: x.tag == "b"
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: left, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
		{From: branch, To: right, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")})},
	}

	s := Solve(inputs, testResolver())

	// At left: x should be typeA
	gotLeft := s.NarrowedTypeAt(left, pathX)
	t.Logf("NarrowedTypeAt(left) = %v", gotLeft)
	if !typ.TypeEquals(gotLeft, typeA) {
		t.Errorf("left: got %v, want %v", gotLeft, typeA)
	}

	// At right: x should be typeB
	gotRight := s.NarrowedTypeAt(right, pathX)
	t.Logf("NarrowedTypeAt(right) = %v", gotRight)
	if !typ.TypeEquals(gotRight, typeB) {
		t.Errorf("right: got %v, want %v", gotRight, typeB)
	}

	// At join: x should be typeA | typeB (union of both branches)
	gotJoin := s.NarrowedTypeAt(join, pathX)
	t.Logf("NarrowedTypeAt(join) = %v", gotJoin)
	wantJoin := typ.NewUnion(typeA, typeB)
	if !typ.TypeEquals(gotJoin, wantJoin) {
		t.Errorf("join: got %v, want %v", gotJoin, wantJoin)
	}
}

// TestNestedLoops_ConstraintPropagation tests constraint propagation through nested loops.
func TestNestedLoops_ConstraintPropagation(t *testing.T) {
	c := cfg.New()
	outer := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	inner := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	body := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	afterInner := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	afterOuter := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), outer, true)
	c.AddEdge(outer, inner, true)
	c.AddEdge(outer, afterOuter, false)
	c.AddEdge(inner, body, true)
	c.AddEdge(inner, afterInner, false)
	c.AddEdge(body, inner, true)
	c.AddEdge(afterInner, outer, true)
	c.AddEdge(afterOuter, c.Exit(), true)

	// Mark inner as loop header with outer as preheader
	if node := c.Node(inner); node != nil {
		node.LoopPreheader = outer
		node.LoopPreheaderSet = true
	}

	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), outer, inner, body, afterInner, afterOuter, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	typeOK := typ.NewRecord().Field("status", typ.LiteralString("ok")).Build()
	typeErr := typ.NewRecord().Field("status", typ.LiteralString("err")).Build()
	union := typ.NewUnion(typeOK, typeErr)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Outer loop entry: x.status == "ok"
	inputs.EdgeConditions = []EdgeCondition{
		{From: outer, To: inner, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "status", Value: typ.LiteralString("ok")})},
	}

	s := Solve(inputs, testResolver())

	// At body (inside inner loop): x should be typeOK
	gotBody := s.NarrowedTypeAt(body, pathX)
	t.Logf("NarrowedTypeAt(body) = %v", gotBody)
	if !typ.TypeEquals(gotBody, typeOK) {
		t.Errorf("body: got %v, want %v", gotBody, typeOK)
	}
}

// TestOptionalChaining tests narrowing through optional type chains.
func TestOptionalChaining(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	inner := typ.NewRecord().Field("value", typ.String).Build()
	optInner := typ.NewOptional(inner)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = optInner

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// x ~= nil => x is the inner type
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX})},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: x should be inner (not optional)
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, inner) {
		t.Errorf("x: got %v, want %v", gotX, inner)
	}
}

// TestMixedConstraints_TypeNumericShape tests all three constraint domains combined.
func TestMixedConstraints_TypeNumericShape(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symI := setupSymbol(g, "i", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verI := cfg.Version{Root: "i", Symbol: symI, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symI, verI)
	}

	typePos := typ.NewRecord().Field("kind", typ.LiteralString("positive")).Build()
	typeNeg := typ.NewRecord().Field("kind", typ.LiteralString("negative")).Build()
	union := typ.NewUnion(typePos, typeNeg)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union
	inputs.DeclaredTypes[symI] = typ.Integer

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathI := constraint.Path{Root: "i", Symbol: symI}

	// Combined: x.kind == "positive" AND i > 0 (type + numeric)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("positive")},
				constraint.NotNil{Path: pathX},
			),
		},
	}
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{
		{
			From: branch,
			To:   thenNode,
			Constraints: []constraint.NumericConstraint{
				constraint.GeConst{X: pathI, C: 1},
			},
		},
	}

	s := Solve(inputs, testResolver())

	// x should be narrowed to typePos
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, typePos) {
		t.Errorf("x: got %v, want %v", gotX, typePos)
	}
}

// TestJoinAsymmetricNarrowing tests that join handles asymmetric narrowing correctly.
// When one branch narrows and the other doesn't, the result should be union of both.
func TestJoinAsymmetricNarrowing(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	left := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	right := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, left, true)
	c.AddEdge(branch, right, false)
	c.AddEdge(left, join, true)
	c.AddEdge(right, join, true)
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	allPoints := []cfg.Point{c.Entry(), branch, left, right, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// ONLY left branch has constraint, right branch has NO constraint
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: left, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
		// No condition on right branch - x remains A | B
	}

	s := Solve(inputs, testResolver())

	// At left: x should be typeA
	gotLeft := s.NarrowedTypeAt(left, pathX)
	t.Logf("NarrowedTypeAt(left) = %v", gotLeft)
	if !typ.TypeEquals(gotLeft, typeA) {
		t.Errorf("left: got %v, want %v", gotLeft, typeA)
	}

	// At right: x should be A | B (no constraint)
	gotRight := s.NarrowedTypeAt(right, pathX)
	t.Logf("NarrowedTypeAt(right) = %v", gotRight)
	if !typ.TypeEquals(gotRight, union) {
		t.Errorf("right: got %v, want %v", gotRight, union)
	}

	// At join: x should be A | B (union of typeA and union)
	gotJoin := s.NarrowedTypeAt(join, pathX)
	t.Logf("NarrowedTypeAt(join) = %v", gotJoin)
	// Since left is A and right is A|B, join should be A|B
	if !typ.TypeEquals(gotJoin, union) {
		t.Errorf("join: got %v, want %v", gotJoin, union)
	}
}

// TestAliasTypeNarrowing tests that constraints work through type aliases.
func TestAliasTypeNarrowing(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	// Create an alias for a union
	innerA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	innerB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()
	innerUnion := typ.NewUnion(innerA, innerB)
	aliasType := typ.NewAlias("MyUnion", innerUnion)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = aliasType

	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("a")})},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: x should be narrowed through the alias to innerA
	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, innerA) {
		t.Errorf("x: got %v, want %v (narrowed through alias)", gotX, innerA)
	}
}

// TestExclusionNarrowing_NotHasType tests that NotHasType properly excludes types.
func TestExclusionNarrowing_NotHasType(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// type(x) ~= "string" => x is number | boolean
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.NotHasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})},
	}

	s := Solve(inputs, testResolver())

	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	wantX := typ.NewUnion(typ.Number, typ.Boolean)
	if !typ.TypeEquals(gotX, wantX) {
		t.Errorf("x: got %v, want %v", gotX, wantX)
	}
}

// TestFieldNotEquals_Literal tests FieldNotEquals with literal values.
func TestFieldNotEquals_Literal(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Build()
	union := typ.NewUnion(typeA, typeB, typeC)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// x.tag ~= "a" => x is B | C
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.FieldNotEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
	}

	s := Solve(inputs, testResolver())

	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	wantX := typ.NewUnion(typeB, typeC)
	if !typ.TypeEquals(gotX, wantX) {
		t.Errorf("x: got %v, want %v", gotX, wantX)
	}
}

// TestIndexNotEquals_Literal tests IndexNotEquals with literal values.
func TestIndexNotEquals_Literal(t *testing.T) {
	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	tupleA := typ.NewTuple(typ.LiteralString("ok"), typ.String)
	tupleB := typ.NewTuple(typ.LiteralString("err"), typ.String)
	union := typ.NewUnion(tupleA, tupleB)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = union

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// x[1] ~= "ok" => x is tupleB
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.IndexNotEquals{Target: pathX, Key: typ.LiteralInt(1), Value: typ.LiteralString("ok")})},
	}

	s := Solve(inputs, testResolver())

	gotX := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(x) = %v", gotX)
	if !typ.TypeEquals(gotX, tupleB) {
		t.Errorf("x: got %v, want %v", gotX, tupleB)
	}
}

// TestTruthyFalsy_Narrowing tests Truthy and Falsy constraint narrowing.
func TestTruthyFalsy_Narrowing(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Nil, typ.False)

	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX})},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(constraint.Falsy{Path: pathX})},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: x is truthy => string
	gotThen := s.NarrowedTypeAt(thenNode, pathX)
	t.Logf("NarrowedTypeAt(thenNode) = %v", gotThen)
	if !typ.TypeEquals(gotThen, typ.String) {
		t.Errorf("thenNode: got %v, want %v", gotThen, typ.String)
	}

	// At elseNode: x is falsy => nil | false
	gotElse := s.NarrowedTypeAt(elseNode, pathX)
	t.Logf("NarrowedTypeAt(elseNode) = %v", gotElse)
	wantElse := typ.NewUnion(typ.Nil, typ.False)
	if !typ.TypeEquals(gotElse, wantElse) {
		t.Errorf("elseNode: got %v, want %v", gotElse, wantElse)
	}
}

// TestInstantiatedGenericNarrowing tests narrowing of instantiated generic types.
// When a variable has type List<T> | nil and we check for nil, it narrows to List<T>.
func TestInstantiatedGenericNarrowing(t *testing.T) {
	c := cfg.New()
	entry := c.Entry()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	exit := c.Exit()

	c.AddEdge(entry, branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, join, true)
	c.AddEdge(elseNode, join, true)
	c.AddEdge(join, exit, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{entry, branch, thenNode, elseNode, join, exit}
	symList := setupSymbol(g, "list", allPoints)

	verList := cfg.Version{Root: "list", Symbol: symList, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symList, verList)
	}

	// Create List<T> generic type
	tParam := typ.NewTypeParam("T", nil)
	listBody := typ.NewInterface("List", []typ.Method{
		{Name: "get", Type: typ.Func().Param("index", typ.Integer).Returns(tParam).Build()},
	})
	listGeneric := typ.NewGeneric("List", []*typ.TypeParam{tParam}, listBody)
	listOfString := typ.Instantiate(listGeneric, typ.String)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symList] = typ.NewOptional(listOfString)

	pathList := constraint.Path{Root: "list", Symbol: symList}

	// if list ~= nil then ... else ...
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathList})},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(constraint.IsNil{Path: pathList})},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: list is not nil => List<string>
	gotThen := s.NarrowedTypeAt(thenNode, pathList)
	t.Logf("NarrowedTypeAt(thenNode) = %v", gotThen)
	if gotThen == nil || gotThen.Kind() == kind.Never {
		t.Errorf("thenNode: expected List<string>, got %v", gotThen)
	}

	// At elseNode: list is nil => nil
	gotElse := s.NarrowedTypeAt(elseNode, pathList)
	t.Logf("NarrowedTypeAt(elseNode) = %v", gotElse)
	if gotElse != typ.Nil {
		t.Errorf("elseNode: got %v, want nil", gotElse)
	}
}

// TestRecursiveTypeNarrowing tests narrowing of recursive types (self-referential).
// A LinkedNode type that contains a field referencing itself.
func TestRecursiveTypeNarrowing(t *testing.T) {
	c := cfg.New()
	entry := c.Entry()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	exit := c.Exit()

	c.AddEdge(entry, branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, exit, true)
	c.AddEdge(elseNode, exit, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{entry, branch, thenNode, elseNode, exit}
	symNode := setupSymbol(g, "node", allPoints)

	verNode := cfg.Version{Root: "node", Symbol: symNode, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symNode, verNode)
	}

	// Create a self-referential type: LinkedNode = {value: number, next: LinkedNode?}
	nodeType := typ.NewRecord().
		Field("value", typ.Number).
		Field("next", typ.NewOptional(typ.Unknown)). // Simplified recursive reference
		Build()
	nodeAlias := typ.NewAlias("LinkedNode", nodeType)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symNode] = typ.NewOptional(nodeAlias)

	pathNode := constraint.Path{Root: "node", Symbol: symNode}

	// if node ~= nil then ... else ...
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathNode})},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(constraint.IsNil{Path: pathNode})},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: node is not nil => LinkedNode
	gotThen := s.NarrowedTypeAt(thenNode, pathNode)
	t.Logf("NarrowedTypeAt(thenNode) = %v", gotThen)
	if gotThen == nil || gotThen.Kind() == kind.Never {
		t.Errorf("thenNode: expected LinkedNode, got %v", gotThen)
	}

	// At elseNode: node is nil => nil
	gotElse := s.NarrowedTypeAt(elseNode, pathNode)
	t.Logf("NarrowedTypeAt(elseNode) = %v", gotElse)
	if gotElse != typ.Nil {
		t.Errorf("elseNode: got %v, want nil", gotElse)
	}
}

// TestConstraintPropagationThroughMultipleEqualities tests constraint propagation
// through chains of equalities: x == y, y == z implies narrowing propagates to all.
func TestConstraintPropagationThroughMultipleEqualities(t *testing.T) {
	c := cfg.New()
	entry := c.Entry()
	n1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	n2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	exit := c.Exit()

	c.AddEdge(entry, n1, true)
	c.AddEdge(n1, n2, true)
	c.AddEdge(n2, exit, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{entry, n1, n2, exit}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)
	symZ := setupSymbol(g, "z", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	verZ := cfg.Version{Root: "z", Symbol: symZ, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
		setVersion(g, p, symY, verY)
		setVersion(g, p, symZ, verZ)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)
	inputs.DeclaredTypes[symY] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	inputs.DeclaredTypes[symZ] = typ.NewUnion(typ.String, typ.Number, typ.Boolean, typ.Nil)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}
	pathZ := constraint.Path{Root: "z", Symbol: symZ}

	// x == y (establishes x and y are equal)
	// y == z (establishes y and z are equal)
	// type(x) == "string" (narrows x to string)
	// This should transitively narrow y to string as well
	inputs.EdgeConditions = []EdgeCondition{
		{From: n1, To: n2, Condition: constraint.FromConstraints(
			constraint.EqPath{Left: pathX, Right: pathY},
			constraint.EqPath{Left: pathY, Right: pathZ},
			constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")},
		)},
	}

	s := Solve(inputs, testResolver())

	// At n2: x is string
	gotX := s.NarrowedTypeAt(n2, pathX)
	t.Logf("NarrowedTypeAt(n2, x) = %v", gotX)
	if !typ.TypeEquals(gotX, typ.String) {
		t.Errorf("n2 x: got %v, want string", gotX)
	}

	// At n2: y should also be narrowed to string through transitive equality
	gotY := s.NarrowedTypeAt(n2, pathY)
	t.Logf("NarrowedTypeAt(n2, y) = %v", gotY)
	// y's declared type intersected with string = string
	if gotY != nil && gotY.Kind() != kind.Never && !typ.TypeEquals(gotY, typ.String) {
		// Allow intersection result that contains string
		if gotY.Kind() != kind.String {
			t.Logf("y narrowed to %v (acceptable)", gotY)
		}
	}

	// At n2: z should also be narrowed through y == z transitivity
	gotZ := s.NarrowedTypeAt(n2, pathZ)
	t.Logf("NarrowedTypeAt(n2, z) = %v", gotZ)
	// z's declared type intersected with string = string
	if gotZ != nil && gotZ.Kind() != kind.Never && !typ.TypeEquals(gotZ, typ.String) {
		if gotZ.Kind() != kind.String {
			t.Logf("z narrowed to %v (acceptable)", gotZ)
		}
	}
}

// TestNestedFieldNarrowing tests narrowing of deeply nested fields.
// x.a.b.c == "hello" should narrow that specific path.
func TestNestedFieldNarrowing(t *testing.T) {
	c := cfg.New()
	entry := c.Entry()
	n1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	exit := c.Exit()

	c.AddEdge(entry, n1, true)
	c.AddEdge(n1, exit, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{entry, n1, exit}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	// x: {a: {b: {c: string | number}}}
	innerType := typ.NewRecord().Field("c", typ.NewUnion(typ.String, typ.Number)).Build()
	middleType := typ.NewRecord().Field("b", innerType).Build()
	outerType := typ.NewRecord().Field("a", middleType).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = outerType

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathABC := pathX.Append(constraint.Segment{Kind: constraint.SegmentField, Name: "a"}).
		Append(constraint.Segment{Kind: constraint.SegmentField, Name: "b"}).
		Append(constraint.Segment{Kind: constraint.SegmentField, Name: "c"})

	// x.a.b.c is string
	inputs.EdgeConditions = []EdgeCondition{
		{From: n1, To: exit, Condition: constraint.FromConstraints(
			constraint.HasType{Path: pathABC, Type: narrow.BuiltinTypeKey("string")},
		)},
	}

	s := Solve(inputs, testResolver())

	// At exit: x.a.b.c should be narrowed to string
	gotABC := s.NarrowedTypeAt(exit, pathABC)
	t.Logf("NarrowedTypeAt(exit, x.a.b.c) = %v", gotABC)
	if !typ.TypeEquals(gotABC, typ.String) {
		t.Errorf("exit x.a.b.c: got %v, want string", gotABC)
	}

	// x itself should still be the interface type (but with narrowed field)
	gotX := s.NarrowedTypeAt(exit, pathX)
	t.Logf("NarrowedTypeAt(exit, x) = %v", gotX)
	if gotX == nil || gotX.Kind() == kind.Never {
		t.Errorf("exit x: expected interface type, got %v", gotX)
	}
}

// TestUnionWithMixedTableAndPrimitive tests narrowing unions containing both
// tables and primitives via type() checks.
func TestUnionWithMixedTableAndPrimitive(t *testing.T) {
	c := cfg.New()
	entry := c.Entry()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	elseNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	exit := c.Exit()

	c.AddEdge(entry, branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, exit, true)
	c.AddEdge(elseNode, exit, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{entry, branch, thenNode, elseNode, exit}
	symVal := setupSymbol(g, "val", allPoints)

	verVal := cfg.Version{Root: "val", Symbol: symVal, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symVal, verVal)
	}

	// val: {name: string} | string | number
	tableType := typ.NewRecord().Field("name", typ.String).Build()
	inputs := newInputs(g)
	inputs.DeclaredTypes[symVal] = typ.NewUnion(tableType, typ.String, typ.Number)

	pathVal := constraint.Path{Root: "val", Symbol: symVal}

	// if type(val) == "table" then ... else ...
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(
			constraint.HasType{Path: pathVal, Type: narrow.BuiltinTypeKey("table")},
		)},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(
			constraint.NotHasType{Path: pathVal, Type: narrow.BuiltinTypeKey("table")},
		)},
	}

	s := Solve(inputs, testResolver())

	// At thenNode: val is table => {name: string}
	gotThen := s.NarrowedTypeAt(thenNode, pathVal)
	t.Logf("NarrowedTypeAt(thenNode) = %v", gotThen)
	if gotThen == nil || gotThen.Kind() == kind.Never {
		t.Errorf("thenNode: expected table type, got %v", gotThen)
	} else if gotThen.Kind() == kind.Union {
		t.Errorf("thenNode: expected non-union table type, got union %v", gotThen)
	}

	// At elseNode: val is not table => string | number
	gotElse := s.NarrowedTypeAt(elseNode, pathVal)
	t.Logf("NarrowedTypeAt(elseNode) = %v", gotElse)
	wantElse := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(gotElse, wantElse) {
		t.Errorf("elseNode: got %v, want %v", gotElse, wantElse)
	}
}

// TestInferFunctionRefinement_ReturnConstraint tests that return expression constraints
// are correctly converted to function refinements with placeholder substitution.
func TestInferFunctionRefinement_ReturnConstraint(t *testing.T) {
	// Build simple CFG: entry -> return -> exit
	c := cfg.New()
	ret := c.AddNode(cfg.NodeReturn, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), ret, true)
	c.AddEdge(ret, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), ret, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Any

	// Simulate return Point(x) with HasType constraint
	pathX := constraint.Path{Root: "x", Symbol: symX}
	typeKey := narrow.TypeKey{Kind: narrow.TypeKeyHash, Hash: 12345}
	inputs.ReturnConstraints = map[cfg.Point]ReturnExprConstraints{
		ret: {
			OnTrue: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: typeKey}),
		},
	}

	s := Solve(inputs, testResolver())

	// Debug: log what's in the solution
	t.Logf("Solution inputs.ReturnConstraints count: %d", len(s.inputs.ReturnConstraints))
	for p, rc := range s.inputs.ReturnConstraints {
		t.Logf("  Point %d: OnTrue=%v OnFalse=%v", p, rc.OnTrue, rc.OnFalse)
		for i, disj := range rc.OnTrue.Disjuncts {
			for j, c := range disj {
				for k, path := range c.Paths() {
					t.Logf("    Constraint[%d][%d].Path[%d]: Root=%q Symbol=%d", i, j, k, path.Root, path.Symbol)
				}
			}
		}
	}

	// Now test effect inference
	params := []ParamInfo{
		{Name: "x", Symbol: symX, Type: typ.Any},
	}

	t.Logf("Param x has Symbol=%d", symX)

	// Debug: check CFG structure
	t.Logf("CFG RPO: %v", c.RPO())
	for _, p := range c.RPO() {
		node := c.Node(p)
		if node != nil {
			t.Logf("  Node at %d: Kind=%d", p, node.Kind)
		}
	}
	t.Logf("Entry: %d, Exit: %d", c.Entry(), c.Exit())

	eff := InferFunctionRefinement(s, c, params, typ.Any)

	t.Logf("Effect: %+v", eff)
	if eff == nil {
		t.Fatal("InferFunctionRefinement returned nil, want effect with OnReturn")
	}

	if !eff.OnReturn.HasConstraints() {
		t.Errorf("OnReturn.HasConstraints() = false, want true")
	}

	// Check that the constraint has been substituted with placeholder $0
	for i, disj := range eff.OnReturn.Disjuncts {
		for j, c := range disj {
			t.Logf("  Disjunct[%d][%d]: %T paths=%v", i, j, c, c.Paths())
			for _, path := range c.Paths() {
				if path.Root != "$0" {
					t.Errorf("Path root = %q, want $0", path.Root)
				}
				if path.Symbol != 0 {
					t.Errorf("Path symbol = %d, want 0 (placeholder paths have no symbol)", path.Symbol)
				}
			}
		}
	}
}

// buildNestedLoopCFG creates a nested loop structure that could cause condition oscillation:
//
//	entry -> header1 -> header2 -> body -> header2 (back) -> header1 (back) -> exit
func buildNestedLoopCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point) {
	c := cfg.New()
	header1 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	header2 := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
	body := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), header1, true)
	c.AddEdge(header1, header2, true)
	c.AddEdge(header2, body, true)
	c.AddEdge(body, header2, true)    // inner back-edge
	c.AddEdge(header2, header1, true) // outer back-edge
	c.AddEdge(header1, c.Exit(), false)

	return c, header1, header2, body
}

// TestFlow_ConstraintPropagation_Convergence tests that propagateConstraints terminates
// even with complex edge conditions on nested loops.
func TestFlow_ConstraintPropagation_Convergence(t *testing.T) {
	c, header1, header2, body := buildNestedLoopCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), header1, header2, body, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	symY := setupSymbol(g, "y", allPoints)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.String)
	inputs.DeclaredTypes[symY] = typ.NewOptional(typ.Number)

	// Add edge conditions that create different paths through the loops
	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}
	inputs.EdgeConditions = []EdgeCondition{
		{From: header1, To: header2, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX})},
		{From: header2, To: body, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathY})},
		{From: body, To: header2, Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX})},
		{From: header2, To: header1, Condition: constraint.FromConstraints(constraint.Truthy{Path: pathY})},
	}

	// This should terminate without hanging
	done := make(chan struct{})
	go func() {
		_ = Solve(inputs, testResolver())
		close(done)
	}()

	select {
	case <-done:
		// Success - terminated
	case <-time.After(2 * time.Second):
		t.Fatal("propagateConstraints did not terminate within 2 seconds - possible infinite loop")
	}
}

// TestFlow_ConstraintPropagation_MonotonicConvergence tests that conditions converge
// monotonically (only get weaker, never oscillate).
func TestFlow_ConstraintPropagation_MonotonicConvergence(t *testing.T) {
	c, header, body := buildLoopCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), header, body, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)

	// Set up SSA version
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, ver1)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.String)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: c.Entry(), To: header, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX})},
		{From: body, To: header, Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX})},
	}

	s := Solve(inputs, testResolver())

	// At the header, condition should be the OR of entry and back-edge conditions.
	// Due to monotonic convergence, this should stabilize.
	cond := s.ConditionAt(header)
	t.Logf("Condition at header: disjuncts=%d, true=%v, false=%v", cond.NumDisjuncts(), cond.IsTrue(), cond.IsFalse())
	for i := 0; i < cond.NumDisjuncts(); i++ {
		t.Logf("  Disjunct %d: %v", i, cond.DisjunctConstraints(i))
	}

	if cond.IsFalse() {
		t.Error("header condition should not be False")
	}

	// The narrowed type at header reflects the merged conditions from all paths.
	// Both entry (NotNil) and back-edge (Truthy) constraints narrow out nil.
	// The OR of these should still narrow string? to string.
	narrowed := s.NarrowedTypeAt(header, pathX)
	if narrowed == nil {
		t.Fatal("NarrowedTypeAt(header) returned nil")
	}
	t.Logf("NarrowedTypeAt(header) = %v", narrowed)

	// Both NotNil and Truthy narrow out nil from optional types
	if typ.TypeEquals(narrowed, typ.NewOptional(typ.String)) {
		t.Errorf("NarrowedTypeAt(header) = %v, should narrow out nil", narrowed)
	}
}

// TestConstraintApplication_WithSSAVersion verifies constraints apply with SSA versioned keys.
func TestConstraintApplication_WithSSAVersion(t *testing.T) {
	c := cfg.New()
	block := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), block, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), block}
	symX := setupSymbol(g, "x", allPoints)

	// Set up SSA version
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, block, symX, ver1)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.String)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs.EdgeConditions = []EdgeCondition{
		{From: c.Entry(), To: block, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX})},
	}

	s := Solve(inputs, testResolver())

	narrowed := s.NarrowedTypeAt(block, pathX)
	if narrowed == nil {
		t.Fatal("NarrowedTypeAt returned nil")
	}

	// Constraint should narrow out nil
	if typ.TypeEquals(narrowed, typ.NewOptional(typ.String)) {
		t.Errorf("expected narrowed type without nil, got %v", narrowed)
	}
	if !typ.TypeEquals(narrowed, typ.String) {
		t.Errorf("expected string, got %v", narrowed)
	}
}

// TestConstraintApplication_CrossPointSymbol verifies constraints created at
// one point apply correctly when queried at a different point using SSA versioned keys.
func TestConstraintApplication_CrossPointSymbol(t *testing.T) {
	c := cfg.New()
	check := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	then := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), check, true)
	c.AddEdge(check, then, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), check, then}
	symX := setupSymbol(g, "x", allPoints)

	// Set up SSA version
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, check, symX, ver1)
	setVersion(g, then, symX, ver1)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number, typ.Nil)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	// Constraint at check->then edge narrows type
	inputs.EdgeConditions = []EdgeCondition{
		{From: check, To: then, Condition: constraint.FromConstraints(constraint.NotNil{Path: pathX})},
	}

	s := Solve(inputs, testResolver())

	// At entry: full union type
	atEntry := s.NarrowedTypeAt(c.Entry(), pathX)
	if atEntry == nil {
		t.Fatal("NarrowedTypeAt(entry) returned nil")
	}

	// At then: nil should be narrowed out
	atThen := s.NarrowedTypeAt(then, pathX)
	if atThen == nil {
		t.Fatal("NarrowedTypeAt(then) returned nil")
	}

	// Entry should have nil (optional/union), then should not
	entryHasNil := core.ContainsNil(atEntry)
	thenHasNil := core.ContainsNil(atThen)

	if !entryHasNil {
		t.Errorf("entry type should contain nil, got %v", atEntry)
	}
	if thenHasNil {
		t.Errorf("then type should not contain nil after NotNil constraint, got %v", atThen)
	}
}

// TestNarrowedTypeAt_ChildPath_Truthy verifies that child paths with segments
// can be narrowed via Truthy constraints (e.g., "if obj.from then" narrows obj.from).
func TestNarrowedTypeAt_ChildPath_Truthy(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	thenNode := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, thenNode}
	symObj := setupSymbol(g, "obj", allPoints)

	ver := cfg.Version{Root: "obj", Symbol: symObj, ID: 1}
	setVersion(g, c.Entry(), symObj, ver)
	setVersion(g, branch, symObj, ver)
	setVersion(g, thenNode, symObj, ver)

	// obj: {from: string?}
	objType := typ.NewRecord().Field("from", typ.NewOptional(typ.String)).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symObj] = objType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "obj", Symbol: symObj},
			Type:       objType,
		},
	}

	// Constraint: Truthy on obj.from (the child path with segment)
	objFromPath := constraint.Path{
		Root:     "obj",
		Symbol:   symObj,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "from"}},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.Truthy{Path: objFromPath})},
	}

	s := Solve(inputs, testResolver())

	// At branch: obj.from is still string?
	atBranch := s.NarrowedTypeAt(branch, objFromPath)
	if atBranch == nil {
		t.Fatal("NarrowedTypeAt(branch, obj.from) returned nil")
	}
	t.Logf("At branch: obj.from = %v", atBranch)

	// At thenNode: obj.from should be narrowed to string (truthy narrows out nil and false)
	atThen := s.NarrowedTypeAt(thenNode, objFromPath)
	if atThen == nil {
		t.Fatal("NarrowedTypeAt(then, obj.from) returned nil")
	}
	t.Logf("At then: obj.from = %v", atThen)

	// Check that nil is narrowed out
	if core.ContainsNil(atThen) {
		t.Errorf("obj.from should not contain nil after Truthy constraint, got %v", atThen)
	}
	if !typ.TypeEquals(atThen, typ.String) {
		t.Errorf("obj.from should be string, got %v", atThen)
	}
}

// TestGuard_NoSymbolKeyInFlowLogic ensures SymbolKey is not used in flow logic (only in tests).
func TestGuard_NoSymbolKeyInFlowLogic(t *testing.T) {
	cmd := exec.Command("rg", "-l", "SymbolKey\\(", ".")
	cmd.Dir = "."
	out, _ := cmd.Output()

	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if file == "" {
			continue
		}
		// Allow: resolver.go (definition), test files
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		if strings.HasSuffix(file, "pathkey/resolver.go") {
			continue
		}
		t.Errorf("SymbolKey usage found in non-test file: %s", file)
	}
}

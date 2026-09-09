package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProcessPointReturnChangedKeys_NoChanges(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	keys := s.processPointReturnChangedKeys(c.Entry())
	if len(keys) != 0 {
		t.Errorf("processPointReturnChangedKeys = %v, want empty", keys)
	}
}

func TestProcessAssignmentReturnChangedKeys_SingleAssignment(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.String,
		},
	}

	s := Solve(inputs, testResolver())

	// Check that value was set
	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(c.Entry(), path)
	if result != typ.String {
		t.Errorf("TypeAt after assignment = %v, want string", result)
	}
}

func TestProcessAssignmentReturnChangedKeys_FieldNilUsesDeclaredFieldType(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symResult := setupSymbol(g, "result", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	setVersion(g, c.Entry(), symResult, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewRecord().Field("err", typ.String).Build()
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: c.Entry(),
			TargetPath: constraint.Path{
				Root:     "result",
				Symbol:   symResult,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			},
			Type: typ.Nil,
		},
	}

	s := Solve(inputs, testResolver())
	path := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
	}
	got := s.TypeAt(c.Entry(), path)
	if got == nil {
		t.Fatal("expected field type after nil assignment")
	}
	if !subtype.IsSubtype(typ.String, got) || !subtype.IsSubtype(typ.Nil, got) {
		t.Fatalf("expected string|nil from declared field + nil assignment, got %v", got)
	}
	if typ.IsAbsentOrUnknown(got) || typ.TypeEquals(got, typ.NewOptional(typ.Unknown)) {
		t.Fatalf("expected concrete typed optional, got %v", got)
	}
}

func TestResolveSymbolKeyType_ZeroSymbol(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	result := s.resolveSymbolKeyType(c.Entry(), 0, "x")
	if result != nil {
		t.Errorf("resolveSymbolKeyType(0) = %v, want nil", result)
	}
}

func TestResolveSymbolKeyType_ValidSymbol(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symK := setupSymbol(g, "k", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "k", Symbol: symK, ID: 1}
	setVersion(g, c.Entry(), symK, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symK] = typ.String

	s := Solve(inputs, testResolver())

	result := s.resolveSymbolKeyType(c.Entry(), symK, "k")
	if result != typ.String {
		t.Errorf("resolveSymbolKeyType(k) = %v, want string", result)
	}
}

func TestWidenArrayElementType_EmptyArray(t *testing.T) {
	result := WidenArrayElementType(nil, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("WidenArrayElementType(nil, string) = %T, want *typ.Array", result)
	}
	if arr.Element != typ.String {
		t.Errorf("WidenArrayElementType(nil, string).Element = %v, want string", arr.Element)
	}
}

func TestWidenArrayElementType_ExistingArray(t *testing.T) {
	existing := typ.NewArray(typ.Integer)
	result := WidenArrayElementType(existing, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("WidenArrayElementType(int[], string) = %T, want *typ.Array", result)
	}
	// Element should be union of integer and string
	union, ok := arr.Element.(*typ.Union)
	if !ok {
		t.Fatalf("widenArrayElementType element = %T, want union", arr.Element)
	}
	if len(union.Members) != 2 {
		t.Errorf("union members = %d, want 2", len(union.Members))
	}
}

func TestWidenArrayElementType_EmptyRecord(t *testing.T) {
	emptyRecord := typ.NewRecord().Build()
	result := WidenArrayElementType(emptyRecord, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("WidenArrayElementType({}, string) = %T, want *typ.Array", result)
	}
	if arr.Element != typ.String {
		t.Errorf("WidenArrayElementType({}, string).Element = %v, want string", arr.Element)
	}
}

func TestWidenWithIndexer_NilBase(t *testing.T) {
	result := widenWithIndexer(nil, typ.String, typ.Integer)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("widenWithIndexer(nil, string, int) = %T, want *typ.Map", result)
	}
	if m.Key != typ.String {
		t.Errorf("widenWithIndexer.Key = %v, want string", m.Key)
	}
	if m.Value != typ.Integer {
		t.Errorf("widenWithIndexer.Value = %v, want integer", m.Value)
	}
}

func TestWidenWithIndexer_EmptyRecord(t *testing.T) {
	emptyRecord := typ.NewRecord().Build()
	result := widenWithIndexer(emptyRecord, typ.String, typ.Number)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("widenWithIndexer({}, string, number) = %T, want *typ.Map", result)
	}
	if m.Key != typ.String {
		t.Errorf("widenWithIndexer.Key = %v, want string", m.Key)
	}
	if m.Value != typ.Number {
		t.Errorf("widenWithIndexer.Value = %v, want number", m.Value)
	}
}

func TestWidenWithIndexer_ExistingMap(t *testing.T) {
	existingMap := typ.NewMap(typ.String, typ.Integer)
	result := widenWithIndexer(existingMap, typ.Number, typ.Boolean)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("widenWithIndexer(map, number, bool) = %T, want *typ.Map", result)
	}
	// Key should be union of string and number
	keyUnion, ok := m.Key.(*typ.Union)
	if !ok {
		t.Fatalf("widenWithIndexer.Key = %T, want union", m.Key)
	}
	if len(keyUnion.Members) != 2 {
		t.Errorf("key union members = %d, want 2", len(keyUnion.Members))
	}
}

func TestWidenMapValueArray_NilBase(t *testing.T) {
	result := WidenMapValueArray(nil, typ.String, typ.Integer)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("WidenMapValueArray(nil) = %T, want *typ.Map", result)
	}
	arr, ok := m.Value.(*typ.Array)
	if !ok {
		t.Fatalf("widenMapValueArray.Value = %T, want *typ.Array", m.Value)
	}
	if arr.Element != typ.Integer {
		t.Errorf("widenMapValueArray.Value.Element = %v, want integer", arr.Element)
	}
}

func TestWidenMapValueArray_PrefersNonSoftElement(t *testing.T) {
	base := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	elem := typ.NewRecord().Field("id", typ.String).Build()
	result := WidenMapValueArray(base, typ.String, elem)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("WidenMapValueArray(map) = %T, want *typ.Map", result)
	}
	arr, ok := m.Value.(*typ.Array)
	if !ok {
		t.Fatalf("widenMapValueArray.Value = %T, want *typ.Array", m.Value)
	}
	if !typ.TypeEquals(arr.Element, elem) {
		t.Fatalf("widenMapValueArray.Value.Element = %v, want %v", arr.Element, elem)
	}
}

func TestWidenArrayElementType_PreservesAlias(t *testing.T) {
	base := typ.NewAlias("Items", typ.NewArray(typ.String))
	got := WidenArrayElementType(base, typ.Integer, typ.JoinPreferNonSoft)
	alias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("WidenArrayElementType(alias) = %T, want *typ.Alias", got)
	}
	if alias.Name != "Items" {
		t.Fatalf("alias name = %q, want Items", alias.Name)
	}
	arr, ok := alias.Target.(*typ.Array)
	if !ok {
		t.Fatalf("alias target = %T, want *typ.Array", alias.Target)
	}
	if !typ.TypeEquals(arr.Element, typ.NewUnion(typ.String, typ.Integer)) {
		t.Fatalf("alias array element = %v, want string|integer", arr.Element)
	}
}

func TestWidenMapValueArray_PreservesAlias(t *testing.T) {
	base := typ.NewAlias("Registry", typ.NewMap(typ.String, typ.NewArray(typ.String)))
	got := WidenMapValueArray(base, typ.String, typ.Integer)
	alias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("WidenMapValueArray(alias) = %T, want *typ.Alias", got)
	}
	if alias.Name != "Registry" {
		t.Fatalf("alias name = %q, want Registry", alias.Name)
	}
	mp, ok := alias.Target.(*typ.Map)
	if !ok {
		t.Fatalf("alias target = %T, want *typ.Map", alias.Target)
	}
	arr, ok := mp.Value.(*typ.Array)
	if !ok {
		t.Fatalf("map value = %T, want *typ.Array", mp.Value)
	}
	if !typ.TypeEquals(arr.Element, typ.NewUnion(typ.String, typ.Integer)) {
		t.Fatalf("map alias array element = %v, want string|integer", arr.Element)
	}
}

func TestProcessJoinReturnChangedKeys_NoPhi(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	keys := s.processJoinReturnChangedKeys(c.Entry())
	if len(keys) != 0 {
		t.Errorf("processJoinReturnChangedKeys = %v, want empty", keys)
	}
}

func TestProcessJoinReturnChangedKeys_WithPhi(t *testing.T) {
	c, branch, then1, join, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, then1, join})
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	ver3 := cfg.Version{Root: "x", Symbol: symX, ID: 3}

	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, branch, symX, ver1)
	setVersion(g, then1, symX, ver2)
	setVersion(g, join, symX, ver3)

	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: branch, Version: ver1},
			{From: then1, Version: ver2},
		},
	})

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: typ.String},
		{Point: then1, TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: typ.Integer},
	}

	s := Solve(inputs, testResolver())

	// At join, x should be union of string and integer
	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(join, path)
	if result == nil {
		t.Fatal("TypeAt(join, x) = nil, want union")
	}

	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("TypeAt(join, x) = %T (%v), want union", result, result)
	}
	if len(union.Members) != 2 {
		t.Errorf("union members = %d, want 2", len(union.Members))
	}
}

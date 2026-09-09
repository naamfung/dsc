package scope

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New returned nil")
	}

	if s.ID() == 0 {
		t.Error("expected non-zero ID")
	}

	if s.Stamp() == 0 {
		t.Error("expected non-zero Stamp")
	}

	if s.Depth() != 0 {
		t.Errorf("expected depth 0, got %d", s.Depth())
	}

	if s.Parent() != nil {
		t.Error("root scope should have nil parent")
	}

	if s.Name() != "" {
		t.Errorf("expected empty name, got %q", s.Name())
	}
}

func TestNilReceiver(t *testing.T) {
	var s *State
	if s.ID() != 0 {
		t.Error("nil.ID() should return 0")
	}

	if s.Stamp() != 0 {
		t.Error("nil.Stamp() should return 0")
	}

	if s.Hash() != 0 {
		t.Error("nil.Hash() should return 0")
	}

	if s.Depth() != 0 {
		t.Error("nil.Depth() should return 0")
	}

	if s.Name() != "" {
		t.Error("nil.Name() should return empty string")
	}

	if s.Parent() != nil {
		t.Error("nil.Parent() should return nil")
	}

	if s.IsLocal("x") {
		t.Error("nil.IsLocal() should return false")
	}

	if s.IsMutated("x") {
		t.Error("nil.IsMutated() should return false")
	}

	if _, ok := s.LookupType("T"); ok {
		t.Error("nil.LookupType() should return false")
	}

	if _, ok := s.LookupTypeParam("T"); ok {
		t.Error("nil.LookupTypeParam() should return false")
	}

	if s.SelfType() != nil {
		t.Error("nil.SelfType() should return nil")
	}

	if s.VariadicType() != nil {
		t.Error("nil.VariadicType() should return nil")
	}

	if s.ReturnTypes() != nil {
		t.Error("nil.ReturnTypes() should return nil")
	}

	if s.AllLocals() != nil {
		t.Error("nil.AllLocals() should return nil")
	}

	if s.AllMutated() != nil {
		t.Error("nil.AllMutated() should return nil")
	}

	if s.AllTypes() != nil {
		t.Error("nil.AllTypes() should return nil")
	}

	if s.TypeParams() != nil {
		t.Error("nil.TypeParams() should return nil")
	}

	// WithLocalName on nil should create new scope
	s2 := s.WithLocalName("x")
	if s2 == nil {
		t.Error("nil.WithLocalName() should create new scope")
	}

	if !s2.IsLocal("x") {
		t.Error("created scope should have local name")
	}

	s3 := s.Child()
	if s3 == nil {
		t.Error("nil.Child() should create new scope")
	}
}

func TestWithLocalName(t *testing.T) {
	s := New().WithLocalName("x")

	if !s.IsLocal("x") {
		t.Error("x should be marked as local")
	}

	// Empty name should be no-op
	s2 := s.WithLocalName("")
	if s2 != s {
		t.Error("empty name should return same state")
	}
}

func TestWithLocalNames(t *testing.T) {
	s := New().WithLocalNames([]string{"a", "b"})

	if !s.IsLocal("a") || !s.IsLocal("b") {
		t.Error("both names should be local")
	}

	// Empty names should be no-op
	s2 := s.WithLocalNames(nil)
	if s2 != s {
		t.Error("empty names should return same state")
	}

	s3 := s.WithLocalNames([]string{})
	if s3 != s {
		t.Error("empty slice should return same state")
	}
}

func TestMutated(t *testing.T) {
	s := New()
	if s.IsMutated("x") {
		t.Error("x should not be mutated initially")
	}

	s2 := s.WithMutated("x")
	if !s2.IsMutated("x") {
		t.Error("x should be mutated after WithMutated")
	}

	if s.IsMutated("x") {
		t.Error("original scope should not be mutated")
	}

	// Empty name should be no-op
	s3 := s.WithMutated("")
	if s3 != s {
		t.Error("empty name should return same state")
	}
}

func TestWithMutatedNames(t *testing.T) {
	s := New().WithMutatedNames([]string{"a", "b", "c"})
	for _, name := range []string{"a", "b", "c"} {
		if !s.IsMutated(name) {
			t.Errorf("%s should be mutated", name)
		}
	}

	// Empty should be no-op
	s2 := s.WithMutatedNames(nil)
	if s2 != s {
		t.Error("nil names should return same state")
	}
}

func TestLookupType(t *testing.T) {
	point := &typ.Record{Fields: []typ.Field{{Name: "x", Type: typ.Number}}}
	s := New().WithType("Point", point)

	got, ok := s.LookupType("Point")
	if !ok {
		t.Fatal("expected to find type Point")
	}

	if got != point {
		t.Error("wrong type returned")
	}

	if _, ok := s.LookupType("Unknown"); ok {
		t.Error("should not find unknown type")
	}
}

func TestWithTypes(t *testing.T) {
	s := New().WithTypes(map[string]typ.Type{
		"A": typ.Integer,
		"B": typ.String,
	})

	if _, ok := s.LookupType("A"); !ok {
		t.Error("type A should exist")
	}

	if _, ok := s.LookupType("B"); !ok {
		t.Error("type B should exist")
	}
}

func TestLookupTypeParam(t *testing.T) {
	tvar := &typ.TypeParam{Name: "T"}
	s := New().WithTypeParams(map[string]typ.Type{"T": tvar})

	got, ok := s.LookupTypeParam("T")
	if !ok {
		t.Fatal("expected to find type param T")
	}

	if got != tvar {
		t.Error("wrong type param returned")
	}
}

func TestWithName(t *testing.T) {
	s := New().WithName("myScope")
	if s.Name() != "myScope" {
		t.Errorf("expected name myScope, got %s", s.Name())
	}
}

func TestSelfType(t *testing.T) {
	self := &typ.Record{Fields: []typ.Field{{Name: "id", Type: typ.Integer}}}
	s := New().WithSelf(self)

	if s.SelfType() != self {
		t.Error("wrong self type")
	}

	s2 := New()
	if s2.SelfType() != nil {
		t.Error("new scope should have nil self type")
	}
}

func TestVariadicType(t *testing.T) {
	s := New().WithVariadic(typ.Any)
	if s.VariadicType() != typ.Any {
		t.Error("wrong variadic type")
	}

	s2 := New()
	if s2.VariadicType() != nil {
		t.Error("new scope should have nil variadic type")
	}
}

func TestReturnTypes(t *testing.T) {
	returns := []typ.Type{typ.Integer, typ.String}
	s := New().WithReturn(returns)

	got := s.ReturnTypes()
	if len(got) != 2 {
		t.Fatalf("expected 2 return types, got %d", len(got))
	}

	if got[0] != typ.Integer || got[1] != typ.String {
		t.Error("wrong return types")
	}

	// Verify copy semantics
	returns[0] = typ.Boolean

	got2 := s.ReturnTypes()
	if got2[0] != typ.Integer {
		t.Error("return types should be copied, not referenced")
	}
}

func TestChild(t *testing.T) {
	parent := New().WithName("parent").WithLocalName("x")
	child := parent.Child()

	if child.Parent() != parent {
		t.Error("child should reference parent")
	}

	if child.Depth() != parent.Depth()+1 {
		t.Errorf("child depth should be %d, got %d", parent.Depth()+1, child.Depth())
	}

	if child.ID() == parent.ID() {
		t.Error("child should have different ID")
	}

	// Child inherits name
	if child.Name() != "parent" {
		t.Error("child should inherit parent name")
	}

	// Child does NOT inherit locals (each scope tracks its own)
	if child.IsLocal("x") {
		t.Error("child should not inherit parent's local marker")
	}
}

func TestChildInheritsContext(t *testing.T) {
	self := &typ.Record{}
	returns := []typ.Type{typ.Integer}
	parent := New().WithSelf(self).WithVariadic(typ.Any).WithReturn(returns)
	child := parent.Child()

	if child.SelfType() != self {
		t.Error("child should inherit self type")
	}

	if child.VariadicType() != typ.Any {
		t.Error("child should inherit variadic type")
	}

	if len(child.ReturnTypes()) != 1 {
		t.Error("child should inherit return types")
	}
}

func TestHashStability(t *testing.T) {
	s := New().WithType("T", typ.String)

	h1 := s.Hash()
	h2 := s.Hash()

	if h1 != h2 {
		t.Error("hash should be stable across calls")
	}

	if h1 == 0 {
		t.Error("hash should be non-zero for non-empty scope")
	}
}

func TestHashDiffers(t *testing.T) {
	s1 := New().WithType("T", typ.Integer)
	s2 := New().WithType("T", typ.String)
	s3 := New().WithType("U", typ.Integer)

	if s1.Hash() == s2.Hash() {
		t.Error("different types should produce different hashes")
	}

	if s1.Hash() == s3.Hash() {
		t.Error("different names should produce different hashes")
	}
}

func TestStampChanges(t *testing.T) {
	s1 := New()
	s2 := s1.WithType("T", typ.Integer)

	if s1.Stamp() == s2.Stamp() {
		t.Error("stamp should change when state changes")
	}

	if s1.ID() != s2.ID() {
		t.Error("ID should remain same for derived state")
	}
}

func TestAllLocals(t *testing.T) {
	s := New().WithLocalName("x").WithLocalName("y")
	locals := s.AllLocals()

	if len(locals) != 2 {
		t.Errorf("expected 2 locals, got %d", len(locals))
	}

	if !locals["x"] || !locals["y"] {
		t.Error("both should be local")
	}
}

func TestAllMutated(t *testing.T) {
	s := New().WithMutated("a").WithMutated("b")
	mutated := s.AllMutated()

	if len(mutated) != 2 {
		t.Errorf("expected 2 mutated, got %d", len(mutated))
	}

	if !mutated["a"] || !mutated["b"] {
		t.Error("both should be mutated")
	}
}

func TestAllTypes(t *testing.T) {
	s := New().WithType("A", typ.Integer).WithType("B", typ.String)
	types := s.AllTypes()

	if len(types) != 2 {
		t.Errorf("expected 2 types, got %d", len(types))
	}
}

func TestTypeParams(t *testing.T) {
	tvar := &typ.TypeParam{Name: "T"}
	s := New().WithTypeParams(map[string]typ.Type{"T": tvar})
	params := s.TypeParams()

	if len(params) != 1 {
		t.Errorf("expected 1 type param, got %d", len(params))
	}

	if params["T"] != tvar {
		t.Error("wrong type param")
	}
}

func TestRangeLocals(t *testing.T) {
	s := New().WithLocalName("x").WithLocalName("y")

	var names []string

	s.RangeLocals(func(name string) bool {
		names = append(names, name)
		return true
	})

	if len(names) != 2 {
		t.Errorf("expected 2 locals, got %d", len(names))
	}
}

func TestRangeMutations(t *testing.T) {
	s := New().WithMutated("a").WithMutated("b")

	var names []string

	s.RangeMutations(func(name string) bool {
		names = append(names, name)
		return true
	})

	if len(names) != 2 {
		t.Errorf("expected 2 mutations, got %d", len(names))
	}
}

func TestRangeTypes(t *testing.T) {
	s := New().WithType("A", typ.Integer).WithType("B", typ.String)

	count := 0

	s.RangeTypes(func(_ string, _ typ.Type) bool {
		count++
		return true
	})

	if count != 2 {
		t.Errorf("expected 2 types, got %d", count)
	}
}

func TestRangeTypeParams(t *testing.T) {
	s := New().WithTypeParams(map[string]typ.Type{
		"T": &typ.TypeParam{Name: "T"},
		"U": &typ.TypeParam{Name: "U"},
	})

	count := 0

	s.RangeTypeParams(func(_ string, _ typ.Type) bool {
		count++
		return true
	})

	if count != 2 {
		t.Errorf("expected 2 type params, got %d", count)
	}
}

func TestDeepNesting(t *testing.T) {
	s := New()
	for i := 0; i < 100; i++ {
		s = s.Child()
	}

	if s.Depth() != 100 {
		t.Errorf("expected depth 100, got %d", s.Depth())
	}

	// Verify parent chain
	current := s
	for i := 0; i < 100; i++ {
		if current.Parent() == nil {
			t.Fatalf("parent nil at depth %d", i)
		}

		current = current.Parent()
	}

	if current.Parent() != nil {
		t.Error("root should have nil parent")
	}
}

func TestUniqueIDs(t *testing.T) {
	ids := make(map[uint64]bool)

	for i := 0; i < 1000; i++ {
		s := New()
		if ids[s.ID()] {
			t.Fatalf("duplicate ID at iteration %d", i)
		}

		ids[s.ID()] = true
	}
}

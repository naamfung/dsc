package theory

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func makePath(name string, sym cfg.SymbolID, segments ...string) constraint.Path {
	path := constraint.Path{Root: name, Symbol: sym}
	for _, s := range segments {
		path.Segments = append(path.Segments, constraint.Segment{
			Kind: constraint.SegmentField,
			Name: s,
		})
	}
	return path
}

func TestEGraph_BasicEquality(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)

	eg.Register(x)
	eg.Register(y)

	if eg.AreEqual(x.Key(), y.Key()) {
		t.Error("x and y should not be equal initially")
	}

	eg.AddEquality(x, y)

	if !eg.AreEqual(x.Key(), y.Key()) {
		t.Error("x and y should be equal after AddEquality")
	}
}

func TestEGraph_Transitivity(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)
	z := makePath("z", 3)

	eg.Register(x)
	eg.Register(y)
	eg.Register(z)

	eg.AddEquality(x, y)
	eg.AddEquality(y, z)

	if !eg.AreEqual(x.Key(), z.Key()) {
		t.Error("x and z should be equal via transitivity")
	}
}

func TestEGraph_FieldCongruence(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)
	xField := makePath("x", 1, "field")
	yField := makePath("y", 2, "field")

	eg.Register(x)
	eg.Register(y)
	eg.Register(xField)
	eg.Register(yField)

	// Before: x.field and y.field are different
	if eg.AreEqual(xField.Key(), yField.Key()) {
		t.Error("x.field and y.field should not be equal initially")
	}

	// Add x == y
	eg.AddEquality(x, y)

	// After: x.field == y.field via congruence
	if !eg.AreEqual(xField.Key(), yField.Key()) {
		t.Error("x.field and y.field should be equal via congruence (x == y)")
	}
}

func TestEGraph_NestedFieldCongruence(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)
	xA := makePath("x", 1, "a")
	yA := makePath("y", 2, "a")
	xAB := makePath("x", 1, "a", "b")
	yAB := makePath("y", 2, "a", "b")

	eg.Register(x)
	eg.Register(y)
	eg.Register(xA)
	eg.Register(yA)
	eg.Register(xAB)
	eg.Register(yAB)

	// x == y should imply x.a == y.a and x.a.b == y.a.b
	eg.AddEquality(x, y)

	if !eg.AreEqual(xA.Key(), yA.Key()) {
		t.Error("x.a and y.a should be equal via congruence")
	}

	if !eg.AreEqual(xAB.Key(), yAB.Key()) {
		t.Error("x.a.b and y.a.b should be equal via nested congruence")
	}
}

func TestEGraph_TransitiveWithFields(t *testing.T) {
	eg := NewEGraph()

	// x == y, y == z, all have .tag field
	x := makePath("x", 1)
	y := makePath("y", 2)
	z := makePath("z", 3)
	xTag := makePath("x", 1, "tag")
	yTag := makePath("y", 2, "tag")
	zTag := makePath("z", 3, "tag")

	eg.Register(x)
	eg.Register(y)
	eg.Register(z)
	eg.Register(xTag)
	eg.Register(yTag)
	eg.Register(zTag)

	eg.AddEquality(x, y)
	eg.AddEquality(y, z)

	// All should be equivalent
	if !eg.AreEqual(x.Key(), z.Key()) {
		t.Error("x and z should be equal via transitivity")
	}

	if !eg.AreEqual(xTag.Key(), yTag.Key()) {
		t.Error("x.tag and y.tag should be equal")
	}

	if !eg.AreEqual(yTag.Key(), zTag.Key()) {
		t.Error("y.tag and z.tag should be equal")
	}

	if !eg.AreEqual(xTag.Key(), zTag.Key()) {
		t.Error("x.tag and z.tag should be equal via transitivity")
	}
}

func TestEGraph_Clone(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)

	eg.Register(x)
	eg.Register(y)
	eg.AddEquality(x, y)

	clone := eg.Clone()

	// Clone should preserve equality
	if !clone.AreEqual(x.Key(), y.Key()) {
		t.Error("clone should preserve x == y")
	}

	// Adding to original shouldn't affect clone
	z := makePath("z", 3)
	eg.Register(z)
	eg.AddEquality(y, z)

	if clone.AreEqual(x.Key(), z.Key()) {
		t.Error("clone should be independent of original")
	}
}

func TestEGraph_GetEquivalenceClass(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)
	z := makePath("z", 3)
	w := makePath("w", 4)

	eg.Register(x)
	eg.Register(y)
	eg.Register(z)
	eg.Register(w)

	eg.AddEquality(x, y)
	eg.AddEquality(y, z)
	// w is separate

	class := eg.GetEquivalenceClass(x.Key())
	if len(class) != 3 {
		t.Errorf("expected 3 members in x's class, got %d", len(class))
	}

	wClass := eg.GetEquivalenceClass(w.Key())
	if len(wClass) != 1 {
		t.Errorf("expected 1 member in w's class, got %d", len(wClass))
	}
}

func TestEGraph_IndexCongruence(t *testing.T) {
	eg := NewEGraph()

	arr1 := makePath("arr1", 1)
	arr2 := makePath("arr2", 2)

	// Create index paths manually
	arr1Idx := constraint.Path{
		Root:   "arr1",
		Symbol: 1,
		Segments: []constraint.Segment{{
			Kind: constraint.SegmentIndexString,
			Name: "0",
		}},
	}
	arr2Idx := constraint.Path{
		Root:   "arr2",
		Symbol: 2,
		Segments: []constraint.Segment{{
			Kind: constraint.SegmentIndexString,
			Name: "0",
		}},
	}

	eg.Register(arr1)
	eg.Register(arr2)
	eg.Register(arr1Idx)
	eg.Register(arr2Idx)

	eg.AddEquality(arr1, arr2)

	if !eg.AreEqual(arr1Idx.Key(), arr2Idx.Key()) {
		t.Error("arr1[0] and arr2[0] should be equal via congruence")
	}
}

func TestEGraph_PartialCongruence(t *testing.T) {
	// Test case where only one side has a field
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)
	xField := makePath("x", 1, "field")
	// y.field is NOT registered

	eg.Register(x)
	eg.Register(y)
	eg.Register(xField)

	eg.AddEquality(x, y)

	// Should not crash, x.field has no counterpart
	class := eg.GetEquivalenceClass(xField.Key())
	if len(class) != 1 {
		t.Errorf("x.field should be alone (no y.field registered), got %d members", len(class))
	}
}

func TestEGraph_IndexStringAndIndexInt_DoNotAliasInCongruence(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)

	xStringIndex := constraint.Path{
		Root:   "x",
		Symbol: 1,
		Segments: []constraint.Segment{{
			Kind: constraint.SegmentIndexString,
			Name: "1",
		}},
	}
	yIntIndex := constraint.Path{
		Root:   "y",
		Symbol: 2,
		Segments: []constraint.Segment{{
			Kind:  constraint.SegmentIndexInt,
			Index: 1,
		}},
	}

	eg.Register(x)
	eg.Register(y)
	eg.Register(xStringIndex)
	eg.Register(yIntIndex)

	eg.AddEquality(x, y)

	if eg.AreEqual(xStringIndex.Key(), yIntIndex.Key()) {
		t.Fatal("x[\"1\"] and y[1] must not become equal via congruence")
	}
}

func TestEGraph_AllPaths(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)
	z := makePath("z", 3)

	eg.Register(x)
	eg.Register(y)
	eg.Register(z)

	paths := eg.AllPaths()
	if len(paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(paths))
	}

	pathMap := make(map[constraint.PathKey]bool)
	for _, p := range paths {
		pathMap[p] = true
	}

	if !pathMap[x.Key()] {
		t.Error("AllPaths should contain x")
	}
	if !pathMap[y.Key()] {
		t.Error("AllPaths should contain y")
	}
	if !pathMap[z.Key()] {
		t.Error("AllPaths should contain z")
	}
}

func TestEGraph_ClassRepresentatives(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)
	z := makePath("z", 3)
	w := makePath("w", 4)

	eg.Register(x)
	eg.Register(y)
	eg.Register(z)
	eg.Register(w)

	eg.AddEquality(x, y)
	eg.AddEquality(y, z)
	// w is separate

	reps := eg.ClassRepresentatives()

	if len(reps) != 2 {
		t.Errorf("expected 2 equivalence classes, got %d", len(reps))
	}
}

func TestEGraph_RegisterKey(t *testing.T) {
	eg := NewEGraph()

	key := constraint.PathKey("test.key")
	eg.RegisterKey(key)

	paths := eg.AllPaths()
	if len(paths) != 1 {
		t.Errorf("expected 1 path after RegisterKey, got %d", len(paths))
	}
	if paths[0] != key {
		t.Errorf("expected key %q, got %q", key, paths[0])
	}
}

func TestEGraph_UnionWithUnknownPaths(t *testing.T) {
	eg := NewEGraph()

	x := makePath("x", 1)
	y := makePath("y", 2)

	// Don't register, try to union
	eg.Union(x.Key(), y.Key())

	// Should not crash; implementation may or may not auto-register.
	_ = eg.AreEqual(x.Key(), y.Key())
}

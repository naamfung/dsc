package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

// TestRecursiveBasic tests basic recursive type creation and properties.
func TestRecursiveBasic(t *testing.T) {
	// type Node = { next: Node? }
	// This is a self-referential type
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	if rec.Kind() != kind.Recursive {
		t.Errorf("Kind: got %v, want Recursive", rec.Kind())
	}

	if rec.Name != "Node" {
		t.Errorf("Name: got %q, want %q", rec.Name, "Node")
	}

	// Body should be a record
	body := rec.Body
	if body == nil {
		t.Fatal("Body should not be nil")
	}

	if body.Kind() != kind.Record {
		t.Errorf("Body kind: got %v, want Record", body.Kind())
	}
}

// TestRecursiveEqualsSelf tests that a recursive type equals itself (no infinite loop).
func TestRecursiveEqualsSelf(t *testing.T) {
	// type Node = { next: Node? }
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// Should equal itself without stack overflow
	if !TypeEquals(rec, rec) {
		t.Error("recursive type should equal itself")
	}

	if !rec.Equals(rec) {
		t.Error("recursive type Equals should return true for itself")
	}
}

// TestRecursiveEqualsEquivalent tests equality of two equivalent recursive types.
func TestRecursiveEqualsEquivalent(t *testing.T) {
	// Create two structurally identical recursive types
	rec1 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// They should be structurally equal
	if !TypeEquals(rec1, rec2) {
		t.Error("structurally equivalent recursive types should be equal")
	}
}

// TestRecursiveNotEqualsNonRecursive tests that recursive types don't equal non-recursive.
func TestRecursiveNotEqualsNonRecursive(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// A non-recursive record
	plain := NewRecord().OptField("next", Number).Build()

	if TypeEquals(rec, plain) {
		t.Error("recursive type should not equal non-recursive type")
	}
}

// TestRecursiveHashConsistency tests that same recursive structure produces same hash.
func TestRecursiveHashConsistency(t *testing.T) {
	rec1 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	if rec1.Hash() != rec2.Hash() {
		t.Error("structurally equal recursive types should have same hash")
	}
}

// TestRecursiveHashNoPanic tests that hashing a recursive type doesn't cause infinite recursion.
func TestRecursiveHashNoPanic(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// Should not panic
	_ = rec.Hash()
}

// TestRecursiveString tests string representation of recursive types.
func TestRecursiveString(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	s := rec.String()
	if s == "" {
		t.Error("String() should not be empty")
	}

	// Should contain the name and ID for stable ordering
	if len(s) < 5 { // "X#1" minimum
		t.Errorf("String format unexpected: %s", s)
	}
}

// TestRecursiveStringNoPanic tests that String() on recursive type doesn't infinite loop.
func TestRecursiveStringNoPanic(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// Should not panic or hang
	_ = rec.String()
	_ = TypeString(rec)
}

// TestRecursiveMutualRecursion tests two mutually recursive types.
func TestRecursiveMutualRecursion(t *testing.T) {
	// type A = { b: B? }
	// type B = { a: A? }
	// Use placeholders for mutual recursion
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")

	// Now set the bodies
	recA.SetBody(NewRecord().OptField("b", recB).Build())
	recB.SetBody(NewRecord().OptField("a", recA).Build())

	// Neither should cause infinite loops
	if !TypeEquals(recA, recA) {
		t.Error("recA should equal itself")
	}

	if !TypeEquals(recB, recB) {
		t.Error("recB should equal itself")
	}

	// A and B should not be equal
	if TypeEquals(recA, recB) {
		t.Error("A should not equal B")
	}
}

// TestRecursiveMutualHashOrderIndependence tests that mutual recursion hash
// is consistent regardless of SetBody call order.
func TestRecursiveMutualHashOrderIndependence(t *testing.T) {
	// Setup 1: A first, then B
	recA1 := NewRecursivePlaceholder("X")
	recB1 := NewRecursivePlaceholder("Y")
	recA1.SetBody(NewRecord().OptField("ref", recB1).Build())
	recB1.SetBody(NewRecord().OptField("ref", recA1).Build())

	// Setup 2: B first, then A (reversed order)
	recA2 := NewRecursivePlaceholder("X")
	recB2 := NewRecursivePlaceholder("Y")
	recB2.SetBody(NewRecord().OptField("ref", recA2).Build())
	recA2.SetBody(NewRecord().OptField("ref", recB2).Build())

	// Hashes should match regardless of setup order
	if recA1.Hash() != recA2.Hash() {
		t.Errorf("X hash order-dependent: %d vs %d", recA1.Hash(), recA2.Hash())
	}
	if recB1.Hash() != recB2.Hash() {
		t.Errorf("Y hash order-dependent: %d vs %d", recB1.Hash(), recB2.Hash())
	}

	// Types should be equal
	if !TypeEquals(recA1, recA2) {
		t.Error("X types should be structurally equal")
	}
	if !TypeEquals(recB1, recB2) {
		t.Error("Y types should be structurally equal")
	}
}

// TestRecursiveInUnion tests recursive type as union member.
func TestRecursiveInUnion(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	union := NewUnion(rec, Nil)

	if union == nil {
		t.Fatal("union should not be nil")
	}

	// Should not panic
	_ = union.String()
	_ = union.Hash()
}

// TestRecursiveAsAliasTarget tests recursive type wrapped in alias.
func TestRecursiveAsAliasTarget(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	alias := NewAlias("MyNode", rec)

	if !TypeEquals(alias, rec) {
		t.Error("alias to recursive type should equal the recursive type")
	}

	// Should not panic
	_ = alias.String()
	_ = alias.Hash()
}

// TestRecursiveListType tests a classic recursive list type.
func TestRecursiveListType(t *testing.T) {
	// type List<T> = nil | { head: T, tail: List<T> }
	// Simplified: type List = { head: number, tail: List? }
	rec := NewRecursive("List", func(self Type) Type {
		return NewRecord().
			Field("head", Number).
			OptField("tail", self).
			Build()
	})

	// Should handle equality
	if !TypeEquals(rec, rec) {
		t.Error("list type should equal itself")
	}

	// Hash should be stable
	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("hash should be stable")
	}
}

// TestRecursiveTreeType tests a recursive tree structure.
func TestRecursiveTreeType(t *testing.T) {
	// type Tree = { value: number, left: Tree?, right: Tree? }
	rec := NewRecursive("Tree", func(self Type) Type {
		return NewRecord().
			Field("value", Number).
			OptField("left", self).
			OptField("right", self).
			Build()
	})

	if !TypeEquals(rec, rec) {
		t.Error("tree type should equal itself")
	}
}

// TestRecursiveDifferentStructuresNotEqual tests that different recursive structures are not equal.
func TestRecursiveDifferentStructuresNotEqual(t *testing.T) {
	// type A = { next: A? }
	recA := NewRecursive("A", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// type B = { child: B?, value: number }
	recB := NewRecursive("B", func(self Type) Type {
		return NewRecord().
			OptField("child", self).
			Field("value", Number).
			Build()
	})

	if TypeEquals(recA, recB) {
		t.Error("different recursive structures should not be equal")
	}
}

// TestIsRecursiveRef tests the IsRecursiveRef utility function.
func TestIsRecursiveRef(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// Same pointer should match
	if !IsRecursiveRef(rec, rec) {
		t.Error("IsRecursiveRef should return true for same pointer")
	}

	// Different recursive type with same ID should match
	rec2 := &Recursive{ID: rec.ID, Name: "Node"}
	if !IsRecursiveRef(rec2, rec) {
		t.Error("IsRecursiveRef should return true for same ID")
	}

	// Different ID should not match
	rec3 := NewRecursive("Other", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})
	if IsRecursiveRef(rec3, rec) {
		t.Error("IsRecursiveRef should return false for different IDs")
	}

	// Non-recursive type should not match
	if IsRecursiveRef(Number, rec) {
		t.Error("IsRecursiveRef should return false for non-recursive type")
	}

	// Nil should not match
	if IsRecursiveRef(nil, rec) {
		t.Error("IsRecursiveRef should return false for nil")
	}
}

// TestRecursiveInArray tests recursive type as array element.
func TestRecursiveInArray(t *testing.T) {
	// type Node = { children: Node[] }
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("children", NewArray(self)).Build()
	})

	// Hash should be stable
	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in array should have stable hash")
	}

	// Should equal itself
	if !TypeEquals(rec, rec) {
		t.Error("recursive type in array should equal itself")
	}

	// Equivalent structure should be equal
	rec2 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("children", NewArray(self)).Build()
	})
	if !TypeEquals(rec, rec2) {
		t.Error("equivalent recursive types in arrays should be equal")
	}
}

// TestRecursiveInMap tests recursive type in map key and value.
func TestRecursiveInMap(t *testing.T) {
	// type Node = { lookup: Map<string, Node> }
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("lookup", NewMap(String, self)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in map value should have stable hash")
	}

	if !TypeEquals(rec, rec) {
		t.Error("recursive type in map should equal itself")
	}
}

// TestRecursiveInTuple tests recursive type in tuple elements.
func TestRecursiveInTuple(t *testing.T) {
	// type Node = { pair: (Node, number) }
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("pair", NewTuple(self, Number)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in tuple should have stable hash")
	}

	if !TypeEquals(rec, rec) {
		t.Error("recursive type in tuple should equal itself")
	}
}

// TestRecursiveInFunction tests recursive type in function parameters and returns.
func TestRecursiveInFunction(t *testing.T) {
	// type Handler = (self: Handler, input: number) -> Handler
	rec := NewRecursive("Handler", func(self Type) Type {
		return Func().Param("self", self).Param("input", Number).Returns(self).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in function should have stable hash")
	}

	if !TypeEquals(rec, rec) {
		t.Error("recursive type in function should equal itself")
	}
}

// TestRecursiveNestedRecords tests deeply nested recursive structures.
func TestRecursiveNestedRecords(t *testing.T) {
	// type Node = { a: { b: { c: Node? } } }
	rec := NewRecursive("Node", func(self Type) Type {
		inner := NewRecord().OptField("c", self).Build()
		middle := NewRecord().Field("b", inner).Build()
		return NewRecord().Field("a", middle).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("deeply nested recursive type should have stable hash")
	}

	if !TypeEquals(rec, rec) {
		t.Error("deeply nested recursive type should equal itself")
	}

	// Equivalent structure
	rec2 := NewRecursive("Node", func(self Type) Type {
		inner := NewRecord().OptField("c", self).Build()
		middle := NewRecord().Field("b", inner).Build()
		return NewRecord().Field("a", middle).Build()
	})
	if !TypeEquals(rec, rec2) {
		t.Error("equivalent deeply nested recursive types should be equal")
	}
}

// TestRecursiveTripleMutual tests three mutually recursive types.
func TestRecursiveTripleMutual(t *testing.T) {
	// type A = { b: B? }
	// type B = { c: C? }
	// type C = { a: A? }
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recC := NewRecursivePlaceholder("C")

	recA.SetBody(NewRecord().OptField("b", recB).Build())
	recB.SetBody(NewRecord().OptField("c", recC).Build())
	recC.SetBody(NewRecord().OptField("a", recA).Build())

	// All should equal themselves
	if !TypeEquals(recA, recA) {
		t.Error("recA should equal itself")
	}
	if !TypeEquals(recB, recB) {
		t.Error("recB should equal itself")
	}
	if !TypeEquals(recC, recC) {
		t.Error("recC should equal itself")
	}

	// Hash should be stable
	hA1, hA2 := recA.Hash(), recA.Hash()
	if hA1 != hA2 {
		t.Error("triple mutual recursion A hash should be stable")
	}

	// None should equal each other
	if TypeEquals(recA, recB) || TypeEquals(recB, recC) || TypeEquals(recA, recC) {
		t.Error("different mutually recursive types should not be equal")
	}
}

// TestRecursivePlaceholderNilBody tests placeholder with nil body.
func TestRecursivePlaceholderNilBody(t *testing.T) {
	rec := NewRecursivePlaceholder("Empty")

	// Should not panic
	h := rec.Hash()
	if h == 0 {
		t.Error("placeholder hash should be non-zero")
	}

	s := rec.String()
	if s == "" {
		t.Error("placeholder string should not be empty")
	}
}

// TestRecursiveHashDeterminism tests that hash is deterministic across calls.
func TestRecursiveHashDeterminism(t *testing.T) {
	for i := 0; i < 100; i++ {
		rec := NewRecursive("Node", func(self Type) Type {
			return NewRecord().OptField("next", self).Build()
		})
		h1 := rec.Hash()
		h2 := rec.Hash()
		h3 := rec.Hash()
		if h1 != h2 || h2 != h3 {
			t.Fatalf("hash not deterministic on iteration %d: %d, %d, %d", i, h1, h2, h3)
		}
	}
}

// TestRecursiveInOptional tests recursive type wrapped in optional.
func TestRecursiveInOptional(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("next", NewOptional(self)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive in optional should have stable hash")
	}

	if !TypeEquals(rec, rec) {
		t.Error("recursive in optional should equal itself")
	}
}

// TestRecursiveInUnionMultiple tests recursive type in union with multiple members.
func TestRecursiveInUnionMultiple(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("value", NewUnion(self, Number, String)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive in multi-member union should have stable hash")
	}

	if !TypeEquals(rec, rec) {
		t.Error("recursive in multi-member union should equal itself")
	}
}

// TestRecursiveEqualsDifferentNames tests that name affects equality.
func TestRecursiveEqualsDifferentNames(t *testing.T) {
	rec1 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Item", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// Different names means different types
	if TypeEquals(rec1, rec2) {
		t.Error("recursive types with different names should not be equal")
	}

	// Hashes should differ
	if rec1.Hash() == rec2.Hash() {
		t.Error("recursive types with different names should have different hashes")
	}
}

// TestUnionDedupMutualRecursion tests that union deduplicates structurally equal
// mutual recursive types built in different orders.
func TestUnionDedupMutualRecursion(t *testing.T) {
	// Build mutual recursion: A <-> B, first order
	recA1 := NewRecursivePlaceholder("A")
	recB1 := NewRecursivePlaceholder("B")
	recA1.SetBody(NewRecord().OptField("b", recB1).Build())
	recB1.SetBody(NewRecord().OptField("a", recA1).Build())

	// Build equivalent mutual recursion, second order
	recA2 := NewRecursivePlaceholder("A")
	recB2 := NewRecursivePlaceholder("B")
	recB2.SetBody(NewRecord().OptField("a", recA2).Build())
	recA2.SetBody(NewRecord().OptField("b", recB2).Build())

	// Union should deduplicate equivalent types
	union := NewUnion(recA1, recA2, Number)

	u, ok := union.(*Union)
	if !ok {
		// Could be simplified to just recA1 | number if deduped perfectly
		if opt, ok := union.(*Optional); ok {
			t.Logf("Union simplified to optional: %v", opt)
			return
		}
		t.Fatalf("expected union, got %T", union)
	}

	// Should have 2 members (recA and number), not 3
	if len(u.Members) > 2 {
		t.Errorf("expected union to dedupe recursive types, got %d members: %v", len(u.Members), u.Members)
	}
}

// TestRecursiveMutualHashConsistency tests that mutual recursion produces
// consistent hashes when accessed multiple times.
func TestRecursiveMutualHashConsistency(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recA.SetBody(NewRecord().OptField("b", recB).Build())
	recB.SetBody(NewRecord().OptField("a", recA).Build())

	// Access hashes multiple times
	hashes := make([]uint64, 10)
	for i := 0; i < 10; i++ {
		hashes[i] = recA.Hash()
	}

	// All hashes should be identical
	for i := 1; i < 10; i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("hash inconsistent at iteration %d: %d vs %d", i, hashes[i], hashes[0])
		}
	}

	// Same for B
	hashesB := make([]uint64, 10)
	for i := 0; i < 10; i++ {
		hashesB[i] = recB.Hash()
	}
	for i := 1; i < 10; i++ {
		if hashesB[i] != hashesB[0] {
			t.Errorf("hash B inconsistent at iteration %d: %d vs %d", i, hashesB[i], hashesB[0])
		}
	}
}

// TestRecursiveHashOptionalReadonlyFlags tests that optional/readonly flags affect recursive hash.
func TestRecursiveHashOptionalReadonlyFlags(t *testing.T) {
	// Required field
	rec1 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("next", self).Build()
	})

	// Optional field (different)
	rec2 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// Readonly field (different)
	rec3 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().ReadonlyField("next", self).Build()
	})

	// All should have different hashes
	h1 := rec1.Hash()
	h2 := rec2.Hash()
	h3 := rec3.Hash()

	if h1 == h2 {
		t.Error("required and optional field should have different hashes")
	}
	if h1 == h3 {
		t.Error("required and readonly field should have different hashes")
	}
	if h2 == h3 {
		t.Error("optional and readonly field should have different hashes")
	}
}

// TestRecursiveHashFunctionVariadic tests that variadic changes recursive hash.
func TestRecursiveHashFunctionVariadic(t *testing.T) {
	// Function with param only
	rec1 := NewRecursive("Handler", func(self Type) Type {
		return NewRecord().
			Field("process", Func().Param("x", Number).Returns(self).Build()).
			Build()
	})

	// Function with param AND variadic (different - more args)
	rec2 := NewRecursive("Handler", func(self Type) Type {
		return NewRecord().
			Field("process", Func().Param("x", Number).Variadic(String).Returns(self).Build()).
			Build()
	})

	h1 := rec1.Hash()
	h2 := rec2.Hash()

	if h1 == h2 {
		t.Error("function with variadic should have different hash than without")
	}
}

// TestRecursiveHashMetatable tests that metatable changes recursive hash.
func TestRecursiveHashMetatable(t *testing.T) {
	metaType := NewRecord().
		Field("__index", Func().Param("key", String).Returns(Any).Build()).
		Build()

	// Without metatable
	rec1 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("value", Number).OptField("next", self).Build()
	})

	// With metatable (different)
	rec2 := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("value", Number).OptField("next", self).Metatable(metaType).Build()
	})

	h1 := rec1.Hash()
	h2 := rec2.Hash()

	if h1 == h2 {
		t.Error("records with and without metatable should have different hashes")
	}
}

// TestRecursiveHashIntersection tests recursive types with intersections.
func TestRecursiveHashIntersection(t *testing.T) {
	// Recursive type using intersection
	rec := NewRecursive("Combined", func(self Type) Type {
		part1 := NewRecord().Field("a", Number).Build()
		part2 := NewRecord().Field("b", String).OptField("next", self).Build()
		return NewIntersection(part1, part2)
	})

	h1 := rec.Hash()
	h2 := rec.Hash()

	if h1 != h2 {
		t.Error("recursive intersection hash should be stable")
	}

	if !TypeEquals(rec, rec) {
		t.Error("recursive intersection should equal itself")
	}
}

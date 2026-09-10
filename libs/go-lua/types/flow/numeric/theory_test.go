package numeric

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

// TestTheorySolver_TransitiveBounds tests that difference logic derives
// transitive bounds: given i < n and n <= 10, theory infers i <= 9.
func TestTheorySolver_TransitiveBounds(t *testing.T) {
	ts := NewTheorySolver()

	// i < n  =>  i - n <= -1
	ts.AddDifferenceConstraint("i", "n", -1)
	// n <= 10
	ts.AddBounds("n", 0, 10)

	if !ts.CheckSatisfiability() {
		t.Fatal("expected satisfiable")
	}

	// Theory should infer i <= 9 (since i < n and n <= 10)
	lower, upper, ok := ts.InferBounds("i")
	if !ok {
		t.Fatal("expected bounds for i")
	}
	if upper > 9 {
		t.Errorf("expected upper bound <= 9, got %d", upper)
	}
	t.Logf("inferred bounds for i: [%d, %d]", lower, upper)
}

// TestTheorySolver_ChainedRelations tests chained difference constraints:
// a < b < c < d with d <= 100 should infer a <= 97.
func TestTheorySolver_ChainedRelations(t *testing.T) {
	ts := NewTheorySolver()

	ts.AddDifferenceConstraint("a", "b", -1) // a < b
	ts.AddDifferenceConstraint("b", "c", -1) // b < c
	ts.AddDifferenceConstraint("c", "d", -1) // c < d
	ts.AddBounds("d", 0, 100)

	if !ts.CheckSatisfiability() {
		t.Fatal("expected satisfiable")
	}

	lower, upper, ok := ts.InferBounds("a")
	if !ok {
		t.Fatal("expected bounds for a")
	}
	if upper > 97 {
		t.Errorf("expected upper bound <= 97 (d-3), got %d", upper)
	}
	t.Logf("inferred bounds for a: [%d, %d]", lower, upper)
}

// TestTheorySolver_DetectsContradiction tests that contradictory constraints
// are detected: i < j and j < i.
func TestTheorySolver_DetectsContradiction(t *testing.T) {
	ts := NewTheorySolver()

	ts.AddDifferenceConstraint("i", "j", -1) // i < j
	ts.AddDifferenceConstraint("j", "i", -1) // j < i (contradiction)

	if ts.CheckSatisfiability() {
		t.Error("expected unsatisfiable due to i < j and j < i")
	}
}

// TestTheorySolver_ModularConsistency tests modular arithmetic reasoning.
func TestTheorySolver_ModularConsistency(t *testing.T) {
	ts := NewTheorySolver()

	// x is known to be 6
	ts.AddBounds("x", 6, 6)

	// x % 2 == 0 should be consistent (6 is even)
	if !ts.CheckModularConsistency("x", 2, 0) {
		t.Error("expected x % 2 == 0 to be consistent with x = 6")
	}

	// x % 2 == 1 should be inconsistent (6 is even)
	if ts.CheckModularConsistency("x", 2, 1) {
		t.Error("expected x % 2 == 1 to be inconsistent with x = 6")
	}
}

// TestTheorySolver_ModularCount tests counting values in range with modular filter.
func TestTheorySolver_ModularCount(t *testing.T) {
	ts := NewTheorySolver()

	// x in [0, 10]
	ts.AddBounds("x", 0, 10)

	// Count even numbers in [0, 10]: 0, 2, 4, 6, 8, 10 = 6
	count := ts.CountModularInRange("x", 2, 0)
	if count != 6 {
		t.Errorf("expected 6 even numbers in [0,10], got %d", count)
	}

	// Count odd numbers in [0, 10]: 1, 3, 5, 7, 9 = 5
	count = ts.CountModularInRange("x", 2, 1)
	if count != 5 {
		t.Errorf("expected 5 odd numbers in [0,10], got %d", count)
	}
}

// TestState_TightenWithTheory tests that theory-based tightening
// produces more precise bounds than basic interval arithmetic.
func TestState_TightenWithTheory(t *testing.T) {
	state := NewState()

	// Set up: i < n, n <= 10
	state.ApplyLt("i", "n")     // i - n <= -1
	state.ApplyLeConst("n", 10) // n <= 10
	state.ApplyGeConst("i", 0)  // i >= 0

	// Basic bounds: i in [0, MaxInt64], n in [-inf, 10]
	lower, upper, ok := state.BoundsFor("i")
	if !ok {
		t.Fatal("expected bounds for i")
	}
	t.Logf("basic bounds for i: [%d, %d]", lower, upper)

	// Tighten with theory: should infer i <= 9
	tightened := TightenWithTheory(state)
	if tightened.IsUnsat() {
		t.Fatal("expected tightened state to be satisfiable")
	}

	lower, upper, ok = tightened.BoundsFor("i")
	if !ok {
		t.Fatal("expected bounds for i after tightening")
	}
	if upper > 9 {
		t.Errorf("expected upper bound <= 9 after theory tightening, got %d", upper)
	}
	t.Logf("theory-tightened bounds for i: [%d, %d]", lower, upper)
}

// TestState_BoundsForWithTheory tests transitive bound inference.
func TestState_BoundsForWithTheory(t *testing.T) {
	state := NewState()

	// a < b < c, c <= 50
	state.ApplyLt("a", "b")
	state.ApplyLt("b", "c")
	state.ApplyLeConst("c", 50)

	// BoundsFor should not give bounds for a (no direct constraint)
	_, _, ok := state.BoundsFor("a")
	if ok {
		t.Log("direct bounds exist for a")
	}

	// BoundsForWithTheory should infer a <= 48 (c-2)
	lower, upper, ok := BoundsForWithTheory(state, "a")
	if !ok {
		t.Fatal("expected theory-inferred bounds for a")
	}
	if upper > 48 {
		t.Errorf("expected upper bound <= 48, got %d", upper)
	}
	t.Logf("theory-inferred bounds for a: [%d, %d]", lower, upper)
}

// TestState_LoopBoundsInference tests typical loop pattern bounds.
func TestState_LoopBoundsInference(t *testing.T) {
	state := NewState()

	// Typical for-loop pattern: for i = 0, i < n, i++
	// At loop body: 0 <= i < n, n = 100

	iKey := constraint.PathKey("i#1")
	nKey := constraint.PathKey("n#1")

	state.ApplyGeConst(iKey, 0)            // i >= 0
	state.applyLeWithConst(iKey, nKey, -1) // i < n (i - n <= -1)
	state.ApplyEqConst(nKey, 100)          // n == 100

	tightened := TightenWithTheory(state)
	if tightened.IsUnsat() {
		t.Fatal("expected satisfiable")
	}

	lower, upper, ok := tightened.BoundsFor(iKey)
	if !ok {
		t.Fatal("expected bounds for i")
	}
	if lower != 0 {
		t.Errorf("expected lower bound 0, got %d", lower)
	}
	if upper != 99 {
		t.Errorf("expected upper bound 99 (n-1), got %d", upper)
	}
	t.Logf("loop index bounds: [%d, %d]", lower, upper)
}

// TestState_FilterLengthModular tests filter length reasoning with modular arithmetic.
func TestState_FilterLengthModular(t *testing.T) {
	state := NewState()

	// Array with known length: len(arr) = 10, indices 0..9
	// Filter: keep even indices -> indices 0, 2, 4, 6, 8 = 5 elements

	idxKey := constraint.PathKey("idx#1")
	state.ApplyGeConst(idxKey, 0) // idx >= 0
	state.ApplyLeConst(idxKey, 9) // idx <= 9

	// How many values in [0,9] satisfy idx % 2 == 0?
	count := CountModularValues(state, idxKey, 2, 0)
	if count != 5 {
		t.Errorf("expected 5 even indices in [0,9], got %d", count)
	}

	// How many satisfy idx % 3 == 0? (0, 3, 6, 9 = 4)
	count = CountModularValues(state, idxKey, 3, 0)
	if count != 4 {
		t.Errorf("expected 4 indices divisible by 3 in [0,9], got %d", count)
	}
}

// TestState_InferRelationalBound tests deriving x - y bounds.
func TestState_InferRelationalBound(t *testing.T) {
	state := NewState()

	// x < y + 5 (x - y <= 4)
	// y < z + 3 (y - z <= 2)
	// Therefore x - z <= 6 (transitively)

	xKey := constraint.PathKey("sym1")
	yKey := constraint.PathKey("sym2")
	zKey := constraint.PathKey("z#1")

	state.applyLeWithConst(xKey, yKey, 4) // x - y <= 4
	state.applyLeWithConst(yKey, zKey, 2) // y - z <= 2

	bound, ok := InferRelationalBound(state, xKey, zKey)
	if !ok {
		t.Fatal("expected relational bound x - z")
	}
	if bound != 6 {
		t.Errorf("expected x - z <= 6, got %d", bound)
	}
	t.Logf("inferred x - z <= %d", bound)
}

// TestTheorySolver_ArrayBoundsPattern tests array bounds checking pattern.
func TestTheorySolver_ArrayBoundsPattern(t *testing.T) {
	// Pattern: arr[i] where 0 <= i < len(arr), len(arr) = n
	// If n = 5, then i in [0, 4]

	ts := NewTheorySolver()

	// i >= 0
	ts.AddBounds("i", 0, maxWeight)
	// i < n (i - n <= -1)
	ts.AddDifferenceConstraint("i", "n", -1)
	// n == 5
	ts.AddBounds("n", 5, 5)

	if !ts.CheckSatisfiability() {
		t.Fatal("expected satisfiable")
	}

	lower, upper, ok := ts.InferBounds("i")
	if !ok {
		t.Fatal("expected bounds for i")
	}
	if lower != 0 {
		t.Errorf("expected lower bound 0, got %d", lower)
	}
	if upper != 4 {
		t.Errorf("expected upper bound 4 (n-1), got %d", upper)
	}
}

// TestTheorySolver_OffByOneDetection tests detecting off-by-one errors.
func TestTheorySolver_OffByOneDetection(t *testing.T) {
	ts := NewTheorySolver()

	// Constraint: i <= n (off-by-one: should be i < n for array access)
	// With n = len(arr), this allows i = n which is out of bounds

	ts.AddDifferenceConstraint("i", "n", 0) // i <= n (i - n <= 0)
	ts.AddBounds("n", 5, 5)                 // n = 5
	ts.AddBounds("i", 0, maxWeight)         // i >= 0

	if !ts.CheckSatisfiability() {
		t.Fatal("expected satisfiable")
	}

	lower, upper, ok := ts.InferBounds("i")
	if !ok {
		t.Fatal("expected bounds for i")
	}
	// With i <= n and n = 5, upper bound is 5 (allowing the bug)
	if upper != 5 {
		t.Errorf("expected upper bound 5 (allows off-by-one), got %d", upper)
	}
	t.Logf("off-by-one pattern bounds: [%d, %d]", lower, upper)
}

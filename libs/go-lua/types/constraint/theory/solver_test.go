package theory

import "testing"

// Complex transitivity chains
func TestDifferenceGraph_LongChain(t *testing.T) {
	// a < b < c < d < e < f, f == 10
	// Chain: a-b≤-1, b-c≤-1, c-d≤-1, d-e≤-1, e-f≤-1 → a-f≤-5
	// f==10 → a-0 ≤ -5+10 = 5, so a ≤ 5
	g := NewDifferenceGraph()
	g.AddLT("a", "b")
	g.AddLT("b", "c")
	g.AddLT("c", "d")
	g.AddLT("d", "e")
	g.AddLT("e", "f")
	g.AddConst("f", 10)

	if g.HasNegativeCycle() {
		t.Fatal("unexpected contradiction")
	}

	upper, ok := g.GetUpperBound("a")
	if !ok {
		t.Fatal("expected upper bound for a")
	}

	if upper > 5 {
		t.Errorf("expected a <= 5, got %d", upper)
	}
}

func TestDifferenceGraph_DiamondPattern(t *testing.T) {
	// x < y, x < z, y < w, z < w
	// Should derive x < w with margin 2
	g := NewDifferenceGraph()
	g.AddLT("x", "y")
	g.AddLT("x", "z")
	g.AddLT("y", "w")
	g.AddLT("z", "w")

	if g.HasNegativeCycle() {
		t.Fatal("unexpected contradiction")
	}

	bound, ok := g.GetBound("x", "w")
	if !ok {
		t.Fatal("expected bound for x-w")
	}

	if bound > -2 {
		t.Errorf("expected x-w <= -2, got %d", bound)
	}
}

func TestDifferenceGraph_MixedConstraints(t *testing.T) {
	// i >= 0, i < n, j >= 0, j < n, i + j < n, n == 10
	// This is common in 2D array access patterns
	g := NewDifferenceGraph()

	g.AddLowerBound("i", 0)
	g.AddLT("i", "n")
	g.AddLowerBound("j", 0)
	g.AddLT("j", "n")
	g.AddConst("n", 10)

	if g.HasNegativeCycle() {
		t.Fatal("unexpected contradiction")
	}

	iUpper, _ := g.GetUpperBound("i")
	jUpper, _ := g.GetUpperBound("j")

	if iUpper > 9 || jUpper > 9 {
		t.Errorf("expected i,j <= 9, got i<=%d, j<=%d", iUpper, jUpper)
	}
}

func TestDifferenceGraph_StringLengthPattern(t *testing.T) {
	// Common pattern: s.len >= 1, i < s.len ⊢ i <= s.len - 1
	g := NewDifferenceGraph()

	g.AddLowerBound("s.len", 1)
	g.AddLT("i", "s.len")
	g.AddLowerBound("i", 0)

	if g.HasNegativeCycle() {
		t.Fatal("unexpected contradiction")
	}

	// i < s.len means i - s.len <= -1
	bound, ok := g.GetBound("i", "s.len")
	if !ok || bound > -1 {
		t.Errorf("expected i - s.len <= -1, got %d", bound)
	}
}

func TestDifferenceGraph_LoopInvariant(t *testing.T) {
	// Loop: for i = 0, n-1 do
	// Invariant: 0 <= i < n
	g := NewDifferenceGraph()

	g.AddConst("n", 100)
	g.AddLowerBound("i", 0)
	g.AddLT("i", "n")

	if g.HasNegativeCycle() {
		t.Fatal("unexpected contradiction")
	}

	lower, lok := g.GetLowerBound("i")
	upper, uok := g.GetUpperBound("i")

	if !lok || lower != 0 {
		t.Errorf("expected lower bound 0, got %d", lower)
	}

	if !uok || upper > 99 {
		t.Errorf("expected upper bound <= 99, got %d", upper)
	}
}

func TestDifferenceGraph_OffByOneDetection(t *testing.T) {
	// Common bug: i <= n instead of i < n when accessing arr[i]
	// If n == len(arr), then i could equal len, which is out of bounds
	g := NewDifferenceGraph()
	g.AddConst("len", 5)
	g.AddLowerBound("i", 0)
	g.AddLE("i", "len", 0) // i <= len (incorrect for array bounds)

	upper, _ := g.GetUpperBound("i")
	if upper != 5 {
		t.Errorf("expected i <= 5 with <= constraint, got %d", upper)
	}

	// Now with correct constraint
	g2 := NewDifferenceGraph()
	g2.AddConst("len", 5)
	g2.AddLowerBound("i", 0)
	g2.AddLT("i", "len") // i < len (correct)

	upper2, _ := g2.GetUpperBound("i")
	if upper2 != 4 {
		t.Errorf("expected i <= 4 (correct), got %d", upper2)
	}
}

func TestDifferenceGraph_NegativeIndices(t *testing.T) {
	// Some languages allow negative indices: arr[-1] is last element
	// i >= -n, i < n, n == 5
	g := NewDifferenceGraph()

	g.AddConst("n", 5)
	g.AddLowerBound("i", -5)
	g.AddLT("i", "n")

	if g.HasNegativeCycle() {
		t.Fatal("unexpected contradiction")
	}

	lower, _ := g.GetLowerBound("i")
	upper, _ := g.GetUpperBound("i")

	if lower != -5 {
		t.Errorf("expected lower -5, got %d", lower)
	}

	if upper != 4 {
		t.Errorf("expected upper 4, got %d", upper)
	}
}

func TestDifferenceGraph_SlicePattern(t *testing.T) {
	// slice(arr, start, end): 0 <= start <= end <= len(arr)
	g := NewDifferenceGraph()

	g.AddConst("len", 10)
	g.AddLowerBound("start", 0)
	g.AddLE("start", "end", 0)
	g.AddLE("end", "len", 0)

	if g.HasNegativeCycle() {
		t.Fatal("unexpected contradiction")
	}

	// Invalid: start > end
	g2 := NewDifferenceGraph()
	g2.AddConst("len", 10)
	g2.AddConst("start", 5)
	g2.AddConst("end", 3) // end < start

	g2.AddLE("start", "end", 0) // requires start <= end

	if !g2.HasNegativeCycle() {
		t.Error("expected contradiction: start > end")
	}
}

// Modular arithmetic tests
func TestModularSolver_FilterEvenOdd(t *testing.T) {
	solver := NewModularSolver()

	// Array of 10 elements, indices 0-9
	solver.AddRange("idx", 0, 9)

	evenCount := solver.CountInRange("idx", 2, 0) // 0,2,4,6,8
	oddCount := solver.CountInRange("idx", 2, 1)  // 1,3,5,7,9

	if evenCount != 5 {
		t.Errorf("expected 5 even indices, got %d", evenCount)
	}

	if oddCount != 5 {
		t.Errorf("expected 5 odd indices, got %d", oddCount)
	}
}

func TestModularSolver_DivisibleBy3(t *testing.T) {
	solver := NewModularSolver()
	solver.AddRange("x", 1, 100)

	div3 := solver.CountInRange("x", 3, 0) // 3,6,9,...,99

	// From 1 to 100: 3,6,9,...,99 = 33 numbers
	if div3 != 33 {
		t.Errorf("expected 33 numbers divisible by 3 in [1,100], got %d", div3)
	}
}

func TestModularSolver_ComplexPredicate(t *testing.T) {
	// x % 6 == 0 means divisible by both 2 and 3
	solver := NewModularSolver()
	solver.AddRange("x", 1, 30)

	div6 := solver.CountInRange("x", 6, 0) // 6,12,18,24,30

	if div6 != 5 {
		t.Errorf("expected 5 numbers divisible by 6 in [1,30], got %d", div6)
	}
}

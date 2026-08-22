package theory

import (
	"math"
	"testing"
)

func TestDifferenceGraph_BasicTransitivity(t *testing.T) {
	// x < y, y < z ⊢ x < z
	g := NewDifferenceGraph()
	g.AddLT("x", "y")
	g.AddLT("y", "z")

	bound, ok := g.GetBound("x", "z")
	if !ok {
		t.Fatal("expected bound for x-z")
	}

	if bound > -2 {
		t.Errorf("expected x-z <= -2, got %d", bound)
	}
}

func TestDifferenceGraph_ArrayBounds(t *testing.T) {
	// i >= 1, i < len(arr), len(arr) == 10 ⊢ i <= 9
	g := NewDifferenceGraph()

	g.AddLowerBound("i", 1)
	g.AddLT("i", "len_arr")
	g.AddConst("len_arr", 10)

	if g.HasNegativeCycle() {
		t.Fatal("unexpected negative cycle")
	}

	upper, ok := g.GetUpperBound("i")
	if !ok {
		t.Fatal("expected upper bound for i")
	}

	if upper > 9 {
		t.Errorf("expected i <= 9, got %d", upper)
	}
}

func TestDifferenceGraph_DetectsContradiction(t *testing.T) {
	// x < y, y < x is unsatisfiable
	g := NewDifferenceGraph()
	g.AddLT("x", "y")
	g.AddLT("y", "x")

	if !g.HasNegativeCycle() {
		t.Error("expected negative cycle (contradiction)")
	}
}

func TestDifferenceGraph_EqualityConstraints(t *testing.T) {
	g := NewDifferenceGraph()
	g.AddEQ("x", "y")

	bound1, ok1 := g.GetBound("x", "y")
	bound2, ok2 := g.GetBound("y", "x")

	if !ok1 || !ok2 {
		t.Fatal("expected bounds for equality")
	}

	if bound1 != 0 || bound2 != 0 {
		t.Errorf("expected x-y=0 and y-x=0, got %d and %d", bound1, bound2)
	}
}

func TestDifferenceGraph_ChainedInequalities(t *testing.T) {
	// a <= b, b <= c, c <= d ⊢ a <= d
	g := NewDifferenceGraph()
	g.AddLE("a", "b", 0)
	g.AddLE("b", "c", 0)
	g.AddLE("c", "d", 0)

	bound, ok := g.GetBound("a", "d")
	if !ok {
		t.Fatal("expected bound for a-d")
	}

	if bound != 0 {
		t.Errorf("expected a-d <= 0, got %d", bound)
	}
}

func TestDifferenceGraph_OffsetConstraints(t *testing.T) {
	// x <= y + 5
	g := NewDifferenceGraph()
	g.AddLE("x", "y", 5)

	bound, ok := g.GetBound("x", "y")
	if !ok {
		t.Fatal("expected bound")
	}

	if bound != 5 {
		t.Errorf("expected x-y <= 5, got %d", bound)
	}
}

func TestDifferenceGraph_ConstantBounds(t *testing.T) {
	g := NewDifferenceGraph()
	g.AddConst("x", 10)

	upper, ok := g.GetUpperBound("x")
	if !ok || upper != 10 {
		t.Errorf("expected upper bound 10, got %d", upper)
	}

	lower, ok := g.GetLowerBound("x")
	if !ok || lower != 10 {
		t.Errorf("expected lower bound 10, got %d", lower)
	}
}

func TestDifferenceGraph_RangeBounds(t *testing.T) {
	// 1 <= x <= 10
	g := NewDifferenceGraph()
	g.AddLowerBound("x", 1)
	g.AddUpperBound("x", 10)

	lower, lok := g.GetLowerBound("x")
	upper, uok := g.GetUpperBound("x")

	if !lok || lower != 1 {
		t.Errorf("expected lower bound 1, got %d", lower)
	}

	if !uok || upper != 10 {
		t.Errorf("expected upper bound 10, got %d", upper)
	}
}

func TestDifferenceGraph_ImpossibleRange(t *testing.T) {
	// x >= 10, x <= 5 is unsatisfiable
	g := NewDifferenceGraph()
	g.AddLowerBound("x", 10)
	g.AddUpperBound("x", 5)

	if !g.HasNegativeCycle() {
		t.Error("expected negative cycle for impossible range")
	}
}

func TestDifferenceGraph_Clone(t *testing.T) {
	g := NewDifferenceGraph()
	g.AddLT("x", "y")

	clone := g.Clone()

	clone.AddLT("y", "x")

	if !clone.HasNegativeCycle() {
		t.Error("clone should have negative cycle")
	}

	if g.HasNegativeCycle() {
		t.Error("original should not have negative cycle")
	}
}

func TestDifferenceGraph_MultipleVariables(t *testing.T) {
	// i >= 0, i < n, j >= 0, j < m, n == m == 10
	g := NewDifferenceGraph()

	g.AddLowerBound("i", 0)
	g.AddLT("i", "n")
	g.AddLowerBound("j", 0)
	g.AddLT("j", "m")
	g.AddConst("n", 10)
	g.AddConst("m", 10)

	if g.HasNegativeCycle() {
		t.Fatal("unexpected negative cycle")
	}

	iUpper, ok := g.GetUpperBound("i")
	if !ok || iUpper > 9 {
		t.Errorf("expected i <= 9, got %d", iUpper)
	}

	jUpper, ok := g.GetUpperBound("j")
	if !ok || jUpper > 9 {
		t.Errorf("expected j <= 9, got %d", jUpper)
	}
}

func TestDifferenceGraph_TransitiveEquality(t *testing.T) {
	// x == y, y == z ⊢ x == z
	g := NewDifferenceGraph()
	g.AddEQ("x", "y")
	g.AddEQ("y", "z")

	bound1, _ := g.GetBound("x", "z")
	bound2, _ := g.GetBound("z", "x")

	if bound1 != 0 || bound2 != 0 {
		t.Errorf("expected x == z from transitivity, got bounds %d, %d", bound1, bound2)
	}
}

func TestDifferenceGraph_IndexPlusOne(t *testing.T) {
	// i >= 0, i < n, n == 5 ⊢ i+1 <= 5
	// Modeled as: new variable i1 where i1 == i + 1, so i1 - i == 1
	g := NewDifferenceGraph()

	g.AddLowerBound("i", 0)
	g.AddLT("i", "n")
	g.AddConst("n", 5)

	g.AddLE("i1", "i", 1)
	g.AddLE("i", "i1", -1)

	upper, ok := g.GetUpperBound("i1")
	if !ok {
		t.Fatal("expected bound for i1")
	}

	if upper > 5 {
		t.Errorf("expected i+1 <= 5, got %d", upper)
	}
}

func TestDifferenceGraph_AddGT(t *testing.T) {
	g := NewDifferenceGraph()
	g.AddGT("x", "y")

	bound, ok := g.GetBound("y", "x")
	if !ok {
		t.Fatal("expected bound for y-x")
	}

	if bound >= 0 {
		t.Errorf("expected y-x < 0 (meaning x > y), got %d", bound)
	}
}

func TestDifferenceGraph_AddGE(t *testing.T) {
	g := NewDifferenceGraph()
	g.AddGE("x", "y")

	bound, ok := g.GetBound("y", "x")
	if !ok {
		t.Fatal("expected bound for y-x")
	}

	if bound > 0 {
		t.Errorf("expected y-x <= 0 (meaning x >= y), got %d", bound)
	}
}

func TestDifferenceGraph_GetBoundUnknownVariable(t *testing.T) {
	g := NewDifferenceGraph()
	g.AddLT("x", "y")

	_, ok := g.GetBound("unknown", "y")
	if ok {
		t.Error("GetBound should return false for unknown variable")
	}

	_, ok = g.GetBound("x", "unknown")
	if ok {
		t.Error("GetBound should return false for unknown variable")
	}
}

func TestDifferenceGraph_GetLowerBoundUnknown(t *testing.T) {
	g := NewDifferenceGraph()

	_, ok := g.GetLowerBound("unknown")
	if ok {
		t.Error("GetLowerBound should return false for unknown variable")
	}
}

func TestDifferenceGraph_SafeAddInt64(t *testing.T) {
	a := safeAddInt64(math.MaxInt64, 1)
	if a != maxWeight {
		t.Errorf("expected saturation to maxWeight, got %d", a)
	}

	b := safeAddInt64(math.MinInt64, -1)
	if b != -maxWeight {
		t.Errorf("expected saturation to -maxWeight, got %d", b)
	}

	c := safeAddInt64(100, 200)
	if c != 300 {
		t.Errorf("expected 300, got %d", c)
	}
}

func TestDifferenceGraph_CloneEmpty(t *testing.T) {
	g := NewDifferenceGraph()
	clone := g.Clone()

	clone.AddLT("x", "y")

	if g.HasNegativeCycle() {
		t.Error("original should not be affected by clone")
	}

	_, ok := g.GetBound("x", "y")
	if ok {
		t.Error("original should not have bound after clone modification")
	}
}

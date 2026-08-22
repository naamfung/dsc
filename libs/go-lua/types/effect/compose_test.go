package effect

import "testing"

func TestUnionEmpty(t *testing.T) {
	result := Union(Empty, Empty)
	if !result.Pure() {
		t.Error("union of empty rows should be empty")
	}
}

func TestUnionWithEmpty(t *testing.T) {
	r := Empty.With(IO{})
	result := Union(r, Empty)

	if len(result.Labels) != 1 {
		t.Errorf("expected 1 label, got %d", len(result.Labels))
	}
}

func TestUnionCombinesLabels(t *testing.T) {
	r1 := Empty.With(IO{})
	r2 := Empty.With(Throw{})
	result := Union(r1, r2)

	if len(result.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(result.Labels))
	}
}

func TestUnionDeduplicates(t *testing.T) {
	r1 := Empty.With(IO{})
	r2 := Empty.With(IO{})
	result := Union(r1, r2)

	if len(result.Labels) != 1 {
		t.Errorf("expected 1 label (deduplicated), got %d", len(result.Labels))
	}
}

func TestUnionWithUnknown(t *testing.T) {
	r := Empty.With(IO{})
	result := Union(r, Unknown)

	if !result.IsUnknown() {
		t.Error("union with Unknown should be Unknown")
	}
}

func TestUnionPreservesTail(t *testing.T) {
	r1 := Open("e1", IO{})
	r2 := Empty.With(Throw{})
	result := Union(r1, r2)

	if !result.IsOpen() {
		t.Error("union should preserve open tail")
	}
}

func TestIntersectEmpty(t *testing.T) {
	r := Empty.With(IO{})
	result := Intersect(r, Empty)

	if !result.Pure() {
		t.Error("intersect with empty should be empty")
	}
}

func TestIntersectCommonLabels(t *testing.T) {
	r1 := Row{Labels: []Label{IO{}, Throw{}}}
	r2 := Row{Labels: []Label{IO{}, Diverge{}}}
	result := Intersect(r1, r2)

	if len(result.Labels) != 1 {
		t.Errorf("expected 1 common label, got %d", len(result.Labels))
	}

	if _, ok := result.Labels[0].(IO); !ok {
		t.Error("common label should be IO")
	}
}

func TestIntersectNoCommon(t *testing.T) {
	r1 := Empty.With(IO{})
	r2 := Empty.With(Throw{})
	result := Intersect(r1, r2)

	if len(result.Labels) != 0 {
		t.Errorf("expected no common labels, got %d", len(result.Labels))
	}
}

func TestSubsetEmpty(t *testing.T) {
	if !Subset(Empty, Empty.With(IO{})) {
		t.Error("empty should be subset of any row")
	}
}

func TestSubsetSame(t *testing.T) {
	r := Empty.With(IO{})
	if !Subset(r, r) {
		t.Error("row should be subset of itself")
	}
}

func TestSubsetSmaller(t *testing.T) {
	r1 := Empty.With(IO{})
	r2 := Row{Labels: []Label{IO{}, Throw{}}}

	if !Subset(r1, r2) {
		t.Error("smaller row should be subset of larger")
	}
}

func TestSubsetLargerNotSubset(t *testing.T) {
	r1 := Row{Labels: []Label{IO{}, Throw{}}}
	r2 := Empty.With(IO{})

	if Subset(r1, r2) {
		t.Error("larger row should not be subset of smaller closed row")
	}
}

func TestSubsetUnknown(t *testing.T) {
	r := Empty.With(IO{})
	if !Subset(r, Unknown) {
		t.Error("any row should be subset of Unknown")
	}

	if Subset(Unknown, r) {
		t.Error("Unknown should not be subset of closed row")
	}
}

func TestOpen(t *testing.T) {
	r := Open("e", IO{})

	if !r.IsOpen() {
		t.Error("should be open")
	}

	if r.Tail.Name != "e" {
		t.Errorf("tail name: got %q, want %q", r.Tail.Name, "e")
	}

	if len(r.Labels) != 1 {
		t.Errorf("labels: got %d, want 1", len(r.Labels))
	}
}

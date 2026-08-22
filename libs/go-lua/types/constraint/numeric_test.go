package constraint

import "testing"

func TestLe(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}
	c := Le{X: x, Y: y, C: 5}

	if c.NumKind() != NumLe {
		t.Errorf("expected NumLe, got %v", c.NumKind())
	}

	paths := c.Paths()
	if len(paths) != 2 || !paths[0].Equal(x) || !paths[1].Equal(y) {
		t.Error("unexpected paths")
	}

	if c.Hash() == 0 {
		t.Error("hash should be non-zero")
	}

	c2 := Le{X: x, Y: y, C: 5}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := Le{X: x, Y: y, C: 10}
	if c.Equals(c3) {
		t.Error("different C should not be equal")
	}
}

func TestLt(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}
	c := Lt{X: x, Y: y}

	if c.NumKind() != NumLt {
		t.Errorf("expected NumLt, got %v", c.NumKind())
	}

	if len(c.Paths()) != 2 {
		t.Error("expected 2 paths")
	}

	c2 := Lt{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := Lt{X: y, Y: x}
	if c.Equals(c3) {
		t.Error("swapped paths should not be equal")
	}
}

func TestGe(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}
	c := Ge{X: x, Y: y}

	if c.NumKind() != NumGe {
		t.Errorf("expected NumGe, got %v", c.NumKind())
	}

	c2 := Ge{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestGt(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}
	c := Gt{X: x, Y: y}

	if c.NumKind() != NumGt {
		t.Errorf("expected NumGt, got %v", c.NumKind())
	}

	c2 := Gt{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestEq(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}
	c := Eq{X: x, Y: y}

	if c.NumKind() != NumEq {
		t.Errorf("expected NumEq, got %v", c.NumKind())
	}

	c2 := Eq{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestEqConst(t *testing.T) {
	x := Path{Root: "x"}
	c := EqConst{X: x, C: 42}

	if c.NumKind() != NumEqConst {
		t.Errorf("expected NumEqConst, got %v", c.NumKind())
	}

	if len(c.Paths()) != 1 {
		t.Error("expected 1 path")
	}

	c2 := EqConst{X: x, C: 42}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := EqConst{X: x, C: 100}
	if c.Equals(c3) {
		t.Error("different C should not be equal")
	}
}

func TestLeConst(t *testing.T) {
	x := Path{Root: "x"}
	c := LeConst{X: x, C: 10}

	if c.NumKind() != NumLeConst {
		t.Errorf("expected NumLeConst, got %v", c.NumKind())
	}

	c2 := LeConst{X: x, C: 10}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestGeConst(t *testing.T) {
	x := Path{Root: "x"}
	c := GeConst{X: x, C: 0}

	if c.NumKind() != NumGeConst {
		t.Errorf("expected NumGeConst, got %v", c.NumKind())
	}

	c2 := GeConst{X: x, C: 0}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestModEq(t *testing.T) {
	x := Path{Root: "x"}
	c := ModEq{X: x, M: 3, R: 1}

	if c.NumKind() != NumModEq {
		t.Errorf("expected NumModEq, got %v", c.NumKind())
	}

	if len(c.Paths()) != 1 {
		t.Error("expected 1 path")
	}

	c2 := ModEq{X: x, M: 3, R: 1}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := ModEq{X: x, M: 3, R: 2}
	if c.Equals(c3) {
		t.Error("different R should not be equal")
	}

	c4 := ModEq{X: x, M: 5, R: 1}
	if c.Equals(c4) {
		t.Error("different M should not be equal")
	}
}

func TestNumericConstraintHashUniqueness(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}

	constraints := []NumericConstraint{
		Le{X: x, Y: y, C: 5},
		Lt{X: x, Y: y},
		Ge{X: x, Y: y},
		Gt{X: x, Y: y},
		Eq{X: x, Y: y},
		EqConst{X: x, C: 5},
		LeConst{X: x, C: 5},
		GeConst{X: x, C: 5},
		ModEq{X: x, M: 3, R: 1},
	}

	hashes := make(map[uint64]int)

	for i, c := range constraints {
		h := c.Hash()
		if prev, exists := hashes[h]; exists {
			t.Errorf("hash collision between constraint %d and %d", prev, i)
		}

		hashes[h] = i
	}
}

func TestNumericConstraintEqualsWrongType(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}

	le := Le{X: x, Y: y, C: 5}
	lt := Lt{X: x, Y: y}

	if le.Equals(lt) {
		t.Error("Le should not equal Lt")
	}

	if lt.Equals(le) {
		t.Error("Lt should not equal Le")
	}
}

func TestNumKindValues(t *testing.T) {
	if NumInvalid != 0 {
		t.Error("NumInvalid should be 0")
	}

	kinds := []NumKind{NumLe, NumLt, NumGe, NumGt, NumEq, NumEqConst, NumLeConst, NumGeConst, NumModEq, NumLeLenOf}
	seen := make(map[NumKind]bool)

	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate NumKind value: %d", k)
		}

		seen[k] = true

		if k == NumInvalid {
			t.Errorf("kind should not be NumInvalid")
		}
	}
}

func TestLeLenOf(t *testing.T) {
	x := Path{Root: "x"}
	arr := Path{Root: "arr"}
	c := LeLenOf{X: x, Array: arr}

	if c.NumKind() != NumLeLenOf {
		t.Errorf("expected NumLeLenOf, got %v", c.NumKind())
	}

	paths := c.Paths()
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
	if !paths[0].Equal(x) || !paths[1].Equal(arr) {
		t.Error("unexpected paths")
	}

	if c.Hash() == 0 {
		t.Error("hash should be non-zero")
	}

	c2 := LeLenOf{X: x, Array: arr}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := LeLenOf{X: x, Array: Path{Root: "other"}}
	if c.Equals(c3) {
		t.Error("different array should not be equal")
	}

	c4 := LeLenOf{X: Path{Root: "other"}, Array: arr}
	if c.Equals(c4) {
		t.Error("different X should not be equal")
	}
}

func TestNumericPathsMethod(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}

	tests := []struct {
		name    string
		c       NumericConstraint
		wantLen int
	}{
		{"Le", Le{X: x, Y: y, C: 5}, 2},
		{"Lt", Lt{X: x, Y: y}, 2},
		{"Ge", Ge{X: x, Y: y}, 2},
		{"Gt", Gt{X: x, Y: y}, 2},
		{"Eq", Eq{X: x, Y: y}, 2},
		{"EqConst", EqConst{X: x, C: 42}, 1},
		{"LeConst", LeConst{X: x, C: 10}, 1},
		{"GeConst", GeConst{X: x, C: 0}, 1},
		{"ModEq", ModEq{X: x, M: 3, R: 1}, 1},
		{"LeLenOf", LeLenOf{X: x, Array: y}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths := tc.c.Paths()
			if len(paths) != tc.wantLen {
				t.Errorf("Paths() returned %d paths, want %d", len(paths), tc.wantLen)
			}
		})
	}
}

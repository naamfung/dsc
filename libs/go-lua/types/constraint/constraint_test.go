package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEqPathCanonicalization(t *testing.T) {
	a := Path{Root: "b"}
	b := Path{Root: "a"}

	eq := NewEqPath(a, b)
	if eq.Left.Root != "a" || eq.Right.Root != "b" {
		t.Fatalf("expected canonical order, got %v %v", eq.Left, eq.Right)
	}
}

func TestHasTypeHashAndEquals(t *testing.T) {
	p := Path{Root: "x"}
	k := narrow.BuiltinTypeKey("string")
	a := HasType{Path: p, Type: k}
	b := HasType{Path: p, Type: k}

	if a.Hash() != b.Hash() {
		t.Fatalf("expected same hash")
	}

	if !a.Equals(b) {
		t.Fatalf("expected equality")
	}
}

func TestFieldEqualsLiteral(t *testing.T) {
	p := Path{Root: "x"}
	lit := typ.LiteralString("event")

	c := FieldEquals{Target: p, Field: "kind", Value: lit}
	if c.Hash() == 0 {
		t.Fatalf("expected non-zero hash")
	}

	other := FieldEquals{Target: p, Field: "kind", Value: typ.LiteralString("event")}
	if !c.Equals(other) {
		t.Fatalf("expected equals for same literal")
	}
}

func TestIndexEqualsLiteral(t *testing.T) {
	p := Path{Root: "x"}
	key := typ.LiteralString("kind")
	lit := typ.LiteralString("event")

	c := IndexEquals{Target: p, Key: key, Value: lit}
	if c.Hash() == 0 {
		t.Fatalf("expected non-zero hash")
	}

	other := IndexEquals{Target: p, Key: typ.LiteralString("kind"), Value: typ.LiteralString("event")}
	if !c.Equals(other) {
		t.Fatalf("expected equals for same literal")
	}
}

func TestConstraintPathsMethod(t *testing.T) {
	p := NewPath(1, "x")
	pf := p.Field("y")

	tests := []struct {
		name       string
		constraint Constraint
		wantLen    int
	}{
		{"Truthy simple", Truthy{Path: p}, 1},
		{"Truthy with field", Truthy{Path: pf}, 2},
		{"Falsy simple", Falsy{Path: p}, 1},
		{"Falsy with field", Falsy{Path: pf}, 2},
		{"IsNil", IsNil{Path: p}, 1},
		{"NotNil", NotNil{Path: p}, 1},
		{"HasType", HasType{Path: p, Type: narrow.BuiltinTypeKey("string")}, 1},
		{"NotHasType", NotHasType{Path: p, Type: narrow.BuiltinTypeKey("string")}, 1},
		{"HasField", HasField{Path: p, Field: "f"}, 1},
		{"FieldEquals", FieldEquals{Target: p, Field: "f"}, 1},
		{"FieldEquals with parent", FieldEquals{Target: pf, Field: "f"}, 2},
		{"FieldNotEquals", FieldNotEquals{Target: p, Field: "f"}, 1},
		{"IndexEquals", IndexEquals{Target: p}, 1},
		{"IndexNotEquals", IndexNotEquals{Target: p}, 1},
		{"EqPath", EqPath{Left: p, Right: NewPath(2, "y")}, 2},
		{"NotEqPath", NotEqPath{Left: p, Right: NewPath(2, "y")}, 2},
		{"FieldEqualsPath", FieldEqualsPath{Target: p, Field: "f", Value: NewPath(2, "y")}, 2},
		{"FieldNotEqualsPath", FieldNotEqualsPath{Target: p, Field: "f", Value: NewPath(2, "y")}, 2},
		{"IndexEqualsPath", IndexEqualsPath{Target: p, Value: NewPath(2, "y")}, 2},
		{"IndexNotEqualsPath", IndexNotEqualsPath{Target: p, Value: NewPath(2, "y")}, 2},
		{"KeyOf", KeyOf{Table: p, Key: NewPath(2, "k")}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths := tc.constraint.Paths()
			if len(paths) != tc.wantLen {
				t.Errorf("Paths() returned %d paths, want %d", len(paths), tc.wantLen)
			}
		})
	}
}

func TestConstraintStringMethod(t *testing.T) {
	p := NewPath(1, "x")

	tests := []struct {
		name         string
		constraint   interface{ String() string }
		wantNonEmpty bool
	}{
		{"Truthy", Truthy{Path: p}, true},
		{"Falsy", Falsy{Path: p}, true},
		{"IsNil", IsNil{Path: p}, true},
		{"NotNil", NotNil{Path: p}, true},
		{"HasType", HasType{Path: p, Type: narrow.BuiltinTypeKey("string")}, true},
		{"NotHasType", NotHasType{Path: p, Type: narrow.BuiltinTypeKey("string")}, true},
		{"HasField", HasField{Path: p, Field: "f"}, true},
		{"FieldEquals", FieldEquals{Target: p, Field: "f"}, true},
		{"FieldNotEquals", FieldNotEquals{Target: p, Field: "f"}, true},
		{"IndexEquals nil key", IndexEquals{Target: p, Key: nil}, true},
		{"IndexEquals with key", IndexEquals{Target: p, Key: typ.LiteralString("k")}, true},
		{"IndexNotEquals nil key", IndexNotEquals{Target: p, Key: nil}, true},
		{"IndexNotEquals with key", IndexNotEquals{Target: p, Key: typ.LiteralString("k")}, true},
		{"EqPath", EqPath{Left: p, Right: NewPath(2, "y")}, true},
		{"NotEqPath", NotEqPath{Left: p, Right: NewPath(2, "y")}, true},
		{"FieldEqualsPath", FieldEqualsPath{Target: p, Field: "f", Value: NewPath(2, "y")}, true},
		{"FieldNotEqualsPath", FieldNotEqualsPath{Target: p, Field: "f", Value: NewPath(2, "y")}, true},
		{"IndexEqualsPath nil key", IndexEqualsPath{Target: p, Value: NewPath(2, "y")}, true},
		{"IndexEqualsPath with key", IndexEqualsPath{Target: p, Key: typ.LiteralString("k"), Value: NewPath(2, "y")}, true},
		{"IndexNotEqualsPath nil key", IndexNotEqualsPath{Target: p, Value: NewPath(2, "y")}, true},
		{"IndexNotEqualsPath with key", IndexNotEqualsPath{Target: p, Key: typ.LiteralString("k"), Value: NewPath(2, "y")}, true},
		{"KeyOf", KeyOf{Table: p, Key: NewPath(2, "k")}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.constraint.String()
			if tc.wantNonEmpty && s == "" {
				t.Error("String() returned empty, want non-empty")
			}
		})
	}
}

func TestConstraintSubstituteMethod(t *testing.T) {
	placeholder := NewPlaceholder(0)
	arg := NewPath(1, "actual")
	args := []Path{arg}

	tests := []struct {
		name       string
		constraint Constraint
	}{
		{"Truthy", Truthy{Path: placeholder}},
		{"Falsy", Falsy{Path: placeholder}},
		{"IsNil", IsNil{Path: placeholder}},
		{"NotNil", NotNil{Path: placeholder}},
		{"HasType", HasType{Path: placeholder, Type: narrow.BuiltinTypeKey("string")}},
		{"NotHasType", NotHasType{Path: placeholder, Type: narrow.BuiltinTypeKey("string")}},
		{"HasField", HasField{Path: placeholder, Field: "f"}},
		{"FieldEquals", FieldEquals{Target: placeholder, Field: "f"}},
		{"FieldNotEquals", FieldNotEquals{Target: placeholder, Field: "f"}},
		{"IndexEquals", IndexEquals{Target: placeholder}},
		{"IndexNotEquals", IndexNotEquals{Target: placeholder}},
		{"EqPath", EqPath{Left: placeholder, Right: placeholder}},
		{"NotEqPath", NotEqPath{Left: placeholder, Right: placeholder}},
		{"FieldEqualsPath", FieldEqualsPath{Target: placeholder, Field: "f", Value: placeholder}},
		{"FieldNotEqualsPath", FieldNotEqualsPath{Target: placeholder, Field: "f", Value: placeholder}},
		{"IndexEqualsPath", IndexEqualsPath{Target: placeholder, Value: placeholder}},
		{"IndexNotEqualsPath", IndexNotEqualsPath{Target: placeholder, Value: placeholder}},
		{"KeyOf", KeyOf{Table: placeholder, Key: placeholder}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := tc.constraint.Substitute(args)
			if !ok {
				t.Fatal("Substitute() returned false")
			}
			if result == nil {
				t.Fatal("Substitute() returned nil")
			}
		})
	}
}

func TestConstraintSubstituteFailure(t *testing.T) {
	placeholder := NewPlaceholder(5)
	args := []Path{NewPath(1, "x")}

	tests := []struct {
		name       string
		constraint Constraint
	}{
		{"Truthy", Truthy{Path: placeholder}},
		{"Falsy", Falsy{Path: placeholder}},
		{"IsNil", IsNil{Path: placeholder}},
		{"NotNil", NotNil{Path: placeholder}},
		{"HasType", HasType{Path: placeholder, Type: narrow.BuiltinTypeKey("string")}},
		{"NotHasType", NotHasType{Path: placeholder, Type: narrow.BuiltinTypeKey("string")}},
		{"HasField", HasField{Path: placeholder, Field: "f"}},
		{"FieldEquals", FieldEquals{Target: placeholder, Field: "f"}},
		{"FieldNotEquals", FieldNotEquals{Target: placeholder, Field: "f"}},
		{"IndexEquals", IndexEquals{Target: placeholder}},
		{"IndexNotEquals", IndexNotEquals{Target: placeholder}},
		{"EqPath", EqPath{Left: placeholder, Right: placeholder}},
		{"NotEqPath", NotEqPath{Left: placeholder, Right: placeholder}},
		{"FieldEqualsPath", FieldEqualsPath{Target: placeholder, Field: "f", Value: placeholder}},
		{"FieldNotEqualsPath", FieldNotEqualsPath{Target: placeholder, Field: "f", Value: placeholder}},
		{"IndexEqualsPath", IndexEqualsPath{Target: placeholder, Value: placeholder}},
		{"IndexNotEqualsPath", IndexNotEqualsPath{Target: placeholder, Value: placeholder}},
		{"KeyOf", KeyOf{Table: placeholder, Key: placeholder}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := tc.constraint.Substitute(args)
			if ok {
				t.Error("Substitute() should return false for out-of-bounds placeholder")
			}
		})
	}
}

func TestConstraintEqualsTypeMismatch(t *testing.T) {
	p := NewPath(1, "x")

	constraints := []Constraint{
		Truthy{Path: p},
		Falsy{Path: p},
		IsNil{Path: p},
		NotNil{Path: p},
		HasType{Path: p, Type: narrow.BuiltinTypeKey("string")},
		NotHasType{Path: p, Type: narrow.BuiltinTypeKey("string")},
		HasField{Path: p, Field: "f"},
		FieldEquals{Target: p, Field: "f"},
		FieldNotEquals{Target: p, Field: "f"},
		IndexEquals{Target: p},
		IndexNotEquals{Target: p},
		EqPath{Left: p, Right: p},
		NotEqPath{Left: p, Right: p},
		FieldEqualsPath{Target: p, Field: "f", Value: p},
		FieldNotEqualsPath{Target: p, Field: "f", Value: p},
		IndexEqualsPath{Target: p, Value: p},
		IndexNotEqualsPath{Target: p, Value: p},
		KeyOf{Table: p, Key: p},
	}

	for i, c1 := range constraints {
		for j, c2 := range constraints {
			if i == j {
				continue
			}
			if c1.Equals(c2) {
				t.Errorf("Different constraint types should not be equal: %T vs %T", c1, c2)
			}
		}
	}
}

func TestFieldEqualsEquality(t *testing.T) {
	p := NewPath(1, "x")
	lit := typ.LiteralString("v")

	c1 := FieldEquals{Target: p, Field: "f", Value: lit}
	c2 := FieldEquals{Target: p, Field: "f", Value: lit}
	c3 := FieldEquals{Target: p, Field: "g", Value: lit}
	c4 := FieldEquals{Target: p, Field: "f", Value: nil}
	c5 := FieldEquals{Target: p, Field: "f", Value: nil}

	if !c1.Equals(c2) {
		t.Error("Same FieldEquals should be equal")
	}
	if c1.Equals(c3) {
		t.Error("Different field names should not be equal")
	}
	if c1.Equals(c4) {
		t.Error("Different nil value should not be equal")
	}
	if !c4.Equals(c5) {
		t.Error("Both nil values should be equal")
	}
}

func TestFieldNotEqualsEquality(t *testing.T) {
	p := NewPath(1, "x")
	lit := typ.LiteralString("v")

	c1 := FieldNotEquals{Target: p, Field: "f", Value: lit}
	c2 := FieldNotEquals{Target: p, Field: "f", Value: lit}
	c3 := FieldNotEquals{Target: p, Field: "g", Value: lit}
	c4 := FieldNotEquals{Target: p, Field: "f", Value: nil}
	c5 := FieldNotEquals{Target: p, Field: "f", Value: nil}

	if !c1.Equals(c2) {
		t.Error("Same FieldNotEquals should be equal")
	}
	if c1.Equals(c3) {
		t.Error("Different field names should not be equal")
	}
	if c1.Equals(c4) {
		t.Error("Different nil value should not be equal")
	}
	if !c4.Equals(c5) {
		t.Error("Both nil values should be equal")
	}
}

func TestIndexEqualsEquality(t *testing.T) {
	p := NewPath(1, "x")
	key := typ.LiteralString("k")
	lit := typ.LiteralString("v")

	c1 := IndexEquals{Target: p, Key: key, Value: lit}
	c2 := IndexEquals{Target: p, Key: key, Value: lit}
	c3 := IndexEquals{Target: p, Key: nil, Value: lit}
	c4 := IndexEquals{Target: p, Key: nil, Value: lit}
	c5 := IndexEquals{Target: p, Key: typ.LiteralString("other"), Value: lit}

	if !c1.Equals(c2) {
		t.Error("Same IndexEquals should be equal")
	}
	if c1.Equals(c3) {
		t.Error("Different key (nil vs non-nil) should not be equal")
	}
	if !c3.Equals(c4) {
		t.Error("Both nil keys should be equal")
	}
	if c1.Equals(c5) {
		t.Error("Different keys should not be equal")
	}
}

func TestIndexNotEqualsEquality(t *testing.T) {
	p := NewPath(1, "x")
	key := typ.LiteralString("k")
	lit := typ.LiteralString("v")

	c1 := IndexNotEquals{Target: p, Key: key, Value: lit}
	c2 := IndexNotEquals{Target: p, Key: key, Value: lit}
	c3 := IndexNotEquals{Target: p, Key: nil, Value: lit}
	c4 := IndexNotEquals{Target: p, Key: nil, Value: lit}

	if !c1.Equals(c2) {
		t.Error("Same IndexNotEquals should be equal")
	}
	if c1.Equals(c3) {
		t.Error("Different key (nil vs non-nil) should not be equal")
	}
	if !c3.Equals(c4) {
		t.Error("Both nil keys should be equal")
	}
}

func TestIndexEqualsPathEquality(t *testing.T) {
	p := NewPath(1, "x")
	v := NewPath(2, "y")
	key := typ.LiteralString("k")

	c1 := IndexEqualsPath{Target: p, Key: key, Value: v}
	c2 := IndexEqualsPath{Target: p, Key: key, Value: v}
	c3 := IndexEqualsPath{Target: p, Key: nil, Value: v}
	c4 := IndexEqualsPath{Target: p, Key: nil, Value: v}
	c5 := IndexEqualsPath{Target: p, Key: key, Value: NewPath(3, "z")}

	if !c1.Equals(c2) {
		t.Error("Same IndexEqualsPath should be equal")
	}
	if c1.Equals(c3) {
		t.Error("Different key (nil vs non-nil) should not be equal")
	}
	if !c3.Equals(c4) {
		t.Error("Both nil keys should be equal")
	}
	if c1.Equals(c5) {
		t.Error("Different value paths should not be equal")
	}
}

func TestIndexNotEqualsPathEquality(t *testing.T) {
	p := NewPath(1, "x")
	v := NewPath(2, "y")
	key := typ.LiteralString("k")

	c1 := IndexNotEqualsPath{Target: p, Key: key, Value: v}
	c2 := IndexNotEqualsPath{Target: p, Key: key, Value: v}
	c3 := IndexNotEqualsPath{Target: p, Key: nil, Value: v}
	c4 := IndexNotEqualsPath{Target: p, Key: nil, Value: v}
	c5 := IndexNotEqualsPath{Target: p, Key: key, Value: NewPath(3, "z")}

	if !c1.Equals(c2) {
		t.Error("Same IndexNotEqualsPath should be equal")
	}
	if c1.Equals(c3) {
		t.Error("Different key (nil vs non-nil) should not be equal")
	}
	if !c3.Equals(c4) {
		t.Error("Both nil keys should be equal")
	}
	if c1.Equals(c5) {
		t.Error("Different value paths should not be equal")
	}
}

func TestNewNotEqPathCanonicalization(t *testing.T) {
	a := Path{Root: "b"}
	b := Path{Root: "a"}

	neq := NewNotEqPath(a, b)
	if neq.Left.Root != "a" || neq.Right.Root != "b" {
		t.Errorf("expected canonical order, got %v %v", neq.Left, neq.Right)
	}

	neq2 := NewNotEqPath(b, a)
	if neq2.Left.Root != "a" || neq2.Right.Root != "b" {
		t.Errorf("expected canonical order, got %v %v", neq2.Left, neq2.Right)
	}
}

func TestConstraintKinds(t *testing.T) {
	p := NewPath(1, "x")
	tests := []struct {
		constraint Constraint
		wantKind   Kind
	}{
		{Truthy{Path: p}, KindTruthy},
		{Falsy{Path: p}, KindFalsy},
		{IsNil{Path: p}, KindIsNil},
		{NotNil{Path: p}, KindNotNil},
		{HasType{Path: p, Type: narrow.BuiltinTypeKey("string")}, KindHasType},
		{NotHasType{Path: p, Type: narrow.BuiltinTypeKey("string")}, KindNotHasType},
		{HasField{Path: p, Field: "f"}, KindHasField},
		{FieldEquals{Target: p, Field: "f"}, KindFieldEquals},
		{FieldNotEquals{Target: p, Field: "f"}, KindFieldNotEquals},
		{IndexEquals{Target: p}, KindIndexEquals},
		{IndexNotEquals{Target: p}, KindIndexNotEquals},
		{EqPath{Left: p, Right: p}, KindEqPath},
		{NotEqPath{Left: p, Right: p}, KindNotEqPath},
		{FieldEqualsPath{Target: p, Field: "f", Value: p}, KindFieldEqualsPath},
		{FieldNotEqualsPath{Target: p, Field: "f", Value: p}, KindFieldNotEqualsPath},
		{IndexEqualsPath{Target: p, Value: p}, KindIndexEqualsPath},
		{IndexNotEqualsPath{Target: p, Value: p}, KindIndexNotEqualsPath},
		{KeyOf{Table: p, Key: p}, KindKeyOf},
	}

	for _, tc := range tests {
		if got := tc.constraint.Kind(); got != tc.wantKind {
			t.Errorf("%T.Kind() = %v, want %v", tc.constraint, got, tc.wantKind)
		}
	}
}

func TestConstraintHashMethod(t *testing.T) {
	p := NewPath(1, "x")
	p2 := NewPath(2, "y")
	lit := typ.LiteralString("val")
	tk := narrow.BuiltinTypeKey("string")

	constraints := []Constraint{
		Truthy{Path: p},
		Falsy{Path: p},
		IsNil{Path: p},
		NotNil{Path: p},
		HasType{Path: p, Type: tk},
		NotHasType{Path: p, Type: tk},
		HasField{Path: p, Field: "f"},
		FieldEquals{Target: p, Field: "f", Value: lit},
		FieldNotEquals{Target: p, Field: "f", Value: lit},
		IndexEquals{Target: p, Key: lit, Value: lit},
		IndexNotEquals{Target: p, Key: lit, Value: lit},
		NewEqPath(p, p2),
		NewNotEqPath(p, p2),
		FieldEqualsPath{Target: p, Field: "f", Value: p2},
		FieldNotEqualsPath{Target: p, Field: "f", Value: p2},
		IndexEqualsPath{Target: p, Key: lit, Value: p2},
		IndexNotEqualsPath{Target: p, Key: lit, Value: p2},
		KeyOf{Table: p, Key: p2},
	}

	hashes := make(map[uint64]int)
	for i, c := range constraints {
		h := c.Hash()
		if h == 0 {
			t.Errorf("%T.Hash() returned 0", c)
		}
		if prev, exists := hashes[h]; exists {
			t.Errorf("Hash collision between constraint %d (%T) and %d (%T)", prev, constraints[prev], i, c)
		}
		hashes[h] = i
	}
}

func TestConstraintHashNilValues(t *testing.T) {
	p := NewPath(1, "x")

	c1 := FieldEquals{Target: p, Field: "f", Value: nil}
	c2 := FieldNotEquals{Target: p, Field: "f", Value: nil}
	c3 := IndexEquals{Target: p, Key: nil, Value: nil}
	c4 := IndexNotEquals{Target: p, Key: nil, Value: nil}
	c5 := IndexEqualsPath{Target: p, Key: nil, Value: p}
	c6 := IndexNotEqualsPath{Target: p, Key: nil, Value: p}

	for _, c := range []Constraint{c1, c2, c3, c4, c5, c6} {
		if c.Hash() == 0 {
			t.Errorf("%T.Hash() with nil values should not be 0", c)
		}
	}
}

func TestConstraintWithParentPaths(t *testing.T) {
	p := NewPath(1, "x").Field("nested")
	p2 := NewPath(2, "y")

	tests := []struct {
		name       string
		constraint Constraint
		wantLen    int
	}{
		{"FieldEqualsPath with nested", FieldEqualsPath{Target: p, Field: "f", Value: p2}, 3},
		{"FieldNotEqualsPath with nested", FieldNotEqualsPath{Target: p, Field: "f", Value: p2}, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths := tc.constraint.Paths()
			if len(paths) != tc.wantLen {
				t.Errorf("Paths() returned %d paths, want %d", len(paths), tc.wantLen)
			}
		})
	}
}

func TestConstraintPathsNoSliceAliasing(t *testing.T) {
	// Create paths with extra capacity to trigger aliasing if buggy
	segments := make([]Segment, 2, 4)
	segments[0] = Segment{Kind: SegmentField, Name: "a"}
	segments[1] = Segment{Kind: SegmentField, Name: "b"}

	p := Path{Root: "x", Symbol: 1, Segments: segments}

	constraints := []Constraint{
		Truthy{Path: p},
		Falsy{Path: p},
		FieldEquals{Target: p, Field: "f"},
		FieldNotEquals{Target: p, Field: "f"},
		FieldEqualsPath{Target: p, Field: "f", Value: NewPath(2, "y")},
		FieldNotEqualsPath{Target: p, Field: "f", Value: NewPath(2, "y")},
	}

	for _, c := range constraints {
		paths := c.Paths()
		if len(paths) < 2 {
			continue
		}

		parent := paths[len(paths)-1]
		if cap(parent.Segments) > len(parent.Segments) {
			extended := append(parent.Segments, Segment{Kind: SegmentField, Name: "CORRUPTED"})
			_ = extended

			if len(p.Segments) >= 2 && p.Segments[1].Name != "b" {
				t.Errorf("%T.Paths() has slice aliasing bug: original path corrupted", c)
			}
		}
	}
}

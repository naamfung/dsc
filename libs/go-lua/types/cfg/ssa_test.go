package cfg

import (
	"testing"
)

// =============================================================================
// Version Tests
// =============================================================================

func TestVersion_IsZero(t *testing.T) {
	tests := []struct {
		name    string
		version Version
		want    bool
	}{
		{
			name:    "zero version",
			version: Version{},
			want:    true,
		},
		{
			name:    "zero ID with root",
			version: Version{Root: "x", ID: 0},
			want:    true,
		},
		{
			name:    "non-zero version",
			version: Version{Root: "x", ID: 1},
			want:    false,
		},
		{
			name:    "high version number",
			version: Version{Root: "counter", ID: 100},
			want:    false,
		},
		{
			name:    "with symbol, zero ID",
			version: Version{Root: "x", Symbol: 42, ID: 0},
			want:    true,
		},
		{
			name:    "with symbol, non-zero ID",
			version: Version{Root: "x", Symbol: 42, ID: 1},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.version.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersion_Key(t *testing.T) {
	tests := []struct {
		name    string
		version Version
		want    string
	}{
		{
			name:    "simple version",
			version: Version{Root: "x", ID: 1},
			want:    "x@1",
		},
		{
			name:    "zero version",
			version: Version{Root: "y", ID: 0},
			want:    "y@0",
		},
		{
			name:    "high version number",
			version: Version{Root: "counter", ID: 42},
			want:    "counter@42",
		},
		{
			name:    "with symbol",
			version: Version{Root: "x", Symbol: 123, ID: 1},
			want:    "x#123@1",
		},
		{
			name:    "with large symbol",
			version: Version{Root: "var", Symbol: 9999, ID: 5},
			want:    "var#9999@5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.version.Key(); got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersion_String(t *testing.T) {
	v := Version{Root: "x", Symbol: 42, ID: 3}
	s := v.String()
	if s != "x@3" {
		t.Errorf("String() = %q, want %q", s, "x@3")
	}
}

func TestVersion_KeyDeterminism(t *testing.T) {
	v := Version{Root: "x", Symbol: 42, ID: 5}
	first := v.Key()

	for i := 0; i < 100; i++ {
		if got := v.Key(); got != first {
			t.Errorf("Iteration %d: Key() = %q, want %q", i, got, first)
		}
	}
}

func TestVersion_Equality(t *testing.T) {
	v1 := Version{Root: "x", Symbol: 1, ID: 1}
	v2 := Version{Root: "x", Symbol: 1, ID: 1}
	v3 := Version{Root: "x", Symbol: 1, ID: 2}
	v4 := Version{Root: "x", Symbol: 2, ID: 1}
	v5 := Version{Root: "y", Symbol: 1, ID: 1}

	if v1 != v2 {
		t.Error("Equal versions should be equal")
	}
	if v1 == v3 {
		t.Error("Different ID should not be equal")
	}
	if v1 == v4 {
		t.Error("Different Symbol should not be equal")
	}
	if v1 == v5 {
		t.Error("Different Root should not be equal")
	}
}

func TestVersion_DistinguishesShadowedVariables(t *testing.T) {
	// Outer x (symbol 1)
	outerX := Version{Root: "x", Symbol: 1, ID: 1}
	// Inner x shadows outer (symbol 2)
	innerX := Version{Root: "x", Symbol: 2, ID: 1}

	// Same name, same version number, but different symbols
	if outerX == innerX {
		t.Error("Shadowed variables with different symbols should not be equal")
	}
	if outerX.Key() == innerX.Key() {
		t.Error("Shadowed variables should have different keys")
	}
}

// =============================================================================
// SymbolID Tests
// =============================================================================

func TestSymbolID_Zero(t *testing.T) {
	var sym SymbolID
	if sym != 0 {
		t.Error("Zero value should be 0")
	}

	sym = SymbolID(0)
	if sym != 0 {
		t.Error("Explicit zero should be 0")
	}
}

func TestSymbolID_NonZero(t *testing.T) {
	sym := SymbolID(42)
	if sym == 0 {
		t.Error("Non-zero symbol should not equal 0")
	}
}

func TestSymbolID_Uniqueness(t *testing.T) {
	sym1 := SymbolID(1)
	sym2 := SymbolID(2)
	sym3 := SymbolID(1)

	if sym1 == sym2 {
		t.Error("Different IDs should not be equal")
	}
	if sym1 != sym3 {
		t.Error("Same IDs should be equal")
	}
}

// =============================================================================
// PhiOperand Tests
// =============================================================================

func TestPhiOperand(t *testing.T) {
	op := PhiOperand{
		From:    Point(5),
		Version: Version{Root: "x", ID: 1},
	}

	if op.From != 5 {
		t.Errorf("From = %d, want 5", op.From)
	}
	if op.Version.Root != "x" {
		t.Error("Version.Root should be x")
	}
	if op.Version.ID != 1 {
		t.Error("Version.ID should be 1")
	}
}

// =============================================================================
// PhiNode Tests
// =============================================================================

func TestPhiNode_Basic(t *testing.T) {
	phi := PhiNode{
		Point:  Point(10),
		Target: Version{Root: "x", ID: 3},
		Operands: []PhiOperand{
			{From: Point(5), Version: Version{Root: "x", ID: 1}},
			{From: Point(7), Version: Version{Root: "x", ID: 2}},
		},
	}

	if phi.Point != 10 {
		t.Errorf("Point = %d, want 10", phi.Point)
	}
	if phi.Target.ID != 3 {
		t.Error("Target version should be 3")
	}
	if len(phi.Operands) != 2 {
		t.Errorf("Operands count = %d, want 2", len(phi.Operands))
	}
}

func TestPhiNode_TargetDistinctFromOperands(t *testing.T) {
	phi := PhiNode{
		Point:  Point(10),
		Target: Version{Root: "x", ID: 3},
		Operands: []PhiOperand{
			{From: Point(5), Version: Version{Root: "x", ID: 1}},
			{From: Point(7), Version: Version{Root: "x", ID: 2}},
		},
	}

	for i, op := range phi.Operands {
		if op.Version == phi.Target {
			t.Errorf("Operand %d should be distinct from target", i)
		}
	}
}

func TestPhiNode_OperandOrderPreserved(t *testing.T) {
	phi := PhiNode{
		Point:  Point(10),
		Target: Version{Root: "x", ID: 4},
		Operands: []PhiOperand{
			{From: Point(3), Version: Version{Root: "x", ID: 1}},
			{From: Point(5), Version: Version{Root: "x", ID: 2}},
			{From: Point(7), Version: Version{Root: "x", ID: 3}},
		},
	}

	expectedFroms := []Point{3, 5, 7}
	expectedIDs := []int{1, 2, 3}

	for i, op := range phi.Operands {
		if op.From != expectedFroms[i] {
			t.Errorf("Operand %d: From = %d, want %d", i, op.From, expectedFroms[i])
		}
		if op.Version.ID != expectedIDs[i] {
			t.Errorf("Operand %d: ID = %d, want %d", i, op.Version.ID, expectedIDs[i])
		}
	}
}

func TestPhiNode_MultipleVariables(t *testing.T) {
	phiX := PhiNode{
		Point:  Point(10),
		Target: Version{Root: "x", ID: 2},
		Operands: []PhiOperand{
			{From: Point(5), Version: Version{Root: "x", ID: 1}},
		},
	}

	phiY := PhiNode{
		Point:  Point(10),
		Target: Version{Root: "y", ID: 2},
		Operands: []PhiOperand{
			{From: Point(5), Version: Version{Root: "y", ID: 1}},
		},
	}

	if phiX.Target.Root == phiY.Target.Root {
		t.Error("Different phi nodes should have different roots")
	}
}

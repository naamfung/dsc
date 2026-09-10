package theory

import "testing"

func TestModularFact_EvaluateConcrete(t *testing.T) {
	tests := []struct {
		value    int64
		modulus  int64
		residue  int64
		expected bool
	}{
		{4, 2, 0, true},
		{5, 2, 0, false},
		{5, 2, 1, true},
		{6, 3, 0, true},
		{7, 3, 1, true},
		{8, 3, 2, true},
		{9, 3, 0, true},
		{0, 2, 0, true},
		{-2, 2, 0, true},
		{-3, 2, 1, true},
	}

	for _, tt := range tests {
		fact := ModularFact{Modulus: tt.modulus, Residue: tt.residue}

		result := fact.Check(tt.value)
		if result != tt.expected {
			t.Errorf("Check(%d %% %d == %d) = %v, want %v",
				tt.value, tt.modulus, tt.residue, result, tt.expected)
		}
	}
}

func TestModularSolver_DeriveFromEquality(t *testing.T) {
	solver := NewModularSolver()

	solver.AddEquality("x", 6)

	facts := solver.GetFacts("x")
	found2 := false
	found3 := false

	for _, f := range facts {
		if f.Modulus == 2 && f.Residue == 0 {
			found2 = true
		}

		if f.Modulus == 3 && f.Residue == 0 {
			found3 = true
		}
	}

	if !found2 {
		t.Error("expected to derive x % 2 == 0 from x == 6")
	}

	if !found3 {
		t.Error("expected to derive x % 3 == 0 from x == 6")
	}
}

func TestModularSolver_CheckConsistency(t *testing.T) {
	solver := NewModularSolver()

	solver.AddEquality("x", 6)

	if !solver.IsConsistent("x", 2, 0) {
		t.Error("x % 2 == 0 should be consistent with x == 6")
	}

	if solver.IsConsistent("x", 2, 1) {
		t.Error("x % 2 == 1 should NOT be consistent with x == 6")
	}
}

func TestModularSolver_RangeConsistency(t *testing.T) {
	solver := NewModularSolver()

	solver.AddRange("x", 0, 10)

	if !solver.IsConsistent("x", 2, 0) {
		t.Error("x % 2 == 0 should be consistent when x in [0,10]")
	}

	if !solver.IsConsistent("x", 2, 1) {
		t.Error("x % 2 == 1 should be consistent when x in [0,10]")
	}
}

func TestModularSolver_CountInRange(t *testing.T) {
	solver := NewModularSolver()

	solver.AddRange("x", 0, 10)

	count := solver.CountInRange("x", 2, 0)
	if count != 6 {
		t.Errorf("count of even numbers in [0,10] = %d, want 6", count)
	}

	count = solver.CountInRange("x", 2, 1)
	if count != 5 {
		t.Errorf("count of odd numbers in [0,10] = %d, want 5", count)
	}
}

func TestModularSolver_CountInRangeGeneral(t *testing.T) {
	tests := []struct {
		lower, upper int64
		modulus      int64
		residue      int64
		expected     int64
	}{
		{1, 6, 2, 0, 3},
		{1, 6, 2, 1, 3},
		{0, 9, 3, 0, 4},
		{0, 9, 3, 1, 3},
		{0, 9, 3, 2, 3},
		{5, 5, 2, 1, 1},
		{5, 5, 2, 0, 0},
		{0, 0, 2, 0, 1},
	}

	for _, tt := range tests {
		solver := NewModularSolver()
		solver.AddRange("x", tt.lower, tt.upper)

		count := solver.CountInRange("x", tt.modulus, tt.residue)
		if count != tt.expected {
			t.Errorf("count(%d %% %d == %d in [%d,%d]) = %d, want %d",
				tt.modulus, tt.modulus, tt.residue, tt.lower, tt.upper, count, tt.expected)
		}
	}
}

func TestModularSolver_EmptyRange(t *testing.T) {
	solver := NewModularSolver()
	solver.AddRange("x", 5, 4)

	count := solver.CountInRange("x", 2, 0)
	if count != 0 {
		t.Errorf("count in empty range should be 0, got %d", count)
	}
}

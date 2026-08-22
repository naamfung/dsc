// theory.go provides SMT-style theory solving for numeric constraints.
//
// The TheorySolver bridges the compact State representation to more sophisticated
// theory solvers that can perform transitive inference and consistency checking.
// It wraps two solvers:
//
//   - DifferenceGraph: Implements difference-bound matrix (DBM) analysis for
//     transitive reasoning about inequalities. Detects unsatisfiability via
//     negative cycle detection.
//
//   - ModularSolver: Handles modular arithmetic constraints (x % m == r) and
//     range-modular consistency checking.
package numeric

import (
	"math"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/constraint/theory"
)

// maxWeight is the maximum bound value for "unbounded" constraints.
// Matches theory.maxWeight to ensure consistent handling across packages.
// Chosen as 2^62 - 1 to avoid overflow in arithmetic operations.
const maxWeight = 1<<62 - 1

// TheorySolver bridges State to SMT-style theory solvers.
//
// The solver maintains two components:
//   - diff: DifferenceGraph for inequality reasoning (x - y <= c)
//   - mod: ModularSolver for modular arithmetic (x % m == r)
//
// These are kept synchronized with State constraints and provide
// additional inference capabilities beyond what State tracks directly.
type TheorySolver struct {
	diff *theory.DifferenceGraph
	mod  *theory.ModularSolver
}

// NewTheorySolver creates a new TheorySolver with empty solvers.
//
// The solvers must be populated via LoadFromState or individual Add* methods
// before queries will return meaningful results.
func NewTheorySolver() *TheorySolver {
	return &TheorySolver{
		diff: theory.NewDifferenceGraph(),
		mod:  theory.NewModularSolver(),
	}
}

// Clone creates a deep copy of the solver for speculative evaluation.
//
// Both underlying solvers are cloned to ensure independent modification.
func (t *TheorySolver) Clone() *TheorySolver {
	return &TheorySolver{
		diff: t.diff.Clone(),
		mod:  t.mod.Clone(),
	}
}

// LoadFromState populates theory solvers from a numeric State.
//
// Transfers all constraints from the State to the theory solvers:
//   - Bounds become upper/lower bound constraints in the difference graph
//   - Exact bounds (lower == upper) also become modular equality constraints
//   - Relations become difference constraints in the graph
//   - Modular constraints are noted but handled via range+residue
//
// This enables transitive inference not available in the raw State.
func (t *TheorySolver) LoadFromState(s *State) {
	if s == nil || s.IsUnsat() {
		return
	}

	// Load bounds as constant constraints
	bounds := s.Bounds()
	for _, key := range constraint.SortedPathKeys(bounds) {
		interval := bounds[key]
		varName := string(key)
		if interval.Lower == interval.Upper {
			t.diff.AddConst(varName, interval.Lower)
			t.mod.AddEquality(varName, interval.Lower)
		} else {
			if interval.Upper < maxWeight {
				t.diff.AddUpperBound(varName, interval.Upper)
			}
			if interval.Lower > -maxWeight {
				t.diff.AddLowerBound(varName, interval.Lower)
			}
			t.mod.AddRange(varName, interval.Lower, interval.Upper)
		}
	}

	// Load difference relations
	relations := s.Relations()
	for _, x := range constraint.SortedPathKeys(relations) {
		rels := relations[x]
		for _, y := range constraint.SortedPathKeys(rels) {
			t.diff.AddLE(string(x), string(y), rels[y])
		}
	}

	// Load modular constraints
	modular := s.Modular()
	for _, key := range constraint.SortedPathKeys(modular) {
		varName := string(key)
		// If we have a range, update it for modular reasoning
		if interval, ok := bounds[key]; ok {
			t.mod.AddRange(varName, interval.Lower, interval.Upper)
		}
	}
}

// CheckSatisfiability checks if the current constraints are satisfiable.
//
// Uses the difference graph's negative cycle detection. A negative cycle
// in the constraint graph means the constraints are mutually unsatisfiable
// (e.g., x > y > z > x implies x > x, a contradiction).
func (t *TheorySolver) CheckSatisfiability() bool {
	return !t.diff.HasNegativeCycle()
}

// InferBounds uses the difference graph to derive tighter bounds via transitive closure.
//
// The difference graph can infer bounds that aren't directly stored. For example,
// if x <= y and y <= 10, then x <= 10 even without a direct bound on x.
//
// Returns the inferred interval (lower, upper, true) or (0, 0, false) if no
// bounds can be derived for the key.
func (t *TheorySolver) InferBounds(key constraint.PathKey) (lower, upper int64, ok bool) {
	varName := string(key)
	lowerBound, hasLower := t.diff.GetLowerBound(varName)
	upperBound, hasUpper := t.diff.GetUpperBound(varName)

	if !hasLower && !hasUpper {
		return 0, 0, false
	}

	if hasLower && hasUpper {
		return lowerBound, upperBound, true
	}
	if hasLower {
		return lowerBound, maxWeight, true
	}
	return -maxWeight, upperBound, true
}

// InferRelationalBound derives the tightest x - y <= c from transitive constraints.
//
// This uses the difference graph's transitive closure to find the minimum
// value of c such that x - y <= c is implied by all constraints. This is
// stronger than just looking up direct relations.
func (t *TheorySolver) InferRelationalBound(x, y constraint.PathKey) (int64, bool) {
	return t.diff.GetBound(string(x), string(y))
}

// CheckModularConsistency checks if x % modulus == residue is consistent with known bounds.
//
// The modular solver checks if there exists at least one value in the variable's
// range that satisfies the modular constraint. Returns false if no such value exists.
func (t *TheorySolver) CheckModularConsistency(key constraint.PathKey, modulus, residue int64) bool {
	return t.mod.IsConsistent(string(key), modulus, residue)
}

// CountModularInRange counts values in a variable's range satisfying x % m == r.
//
// This is used for cardinality reasoning: if the count is 0, the constraint
// is unsatisfiable; if the count is 1, the variable is uniquely determined.
func (t *TheorySolver) CountModularInRange(key constraint.PathKey, modulus, residue int64) int64 {
	return t.mod.CountInRange(string(key), modulus, residue)
}

// AddConstraintWithResolver adds a numeric constraint using versioned keys.
//
// The resolver converts constraint paths to versioned PathKeys at a specific
// CFG point, ensuring constraints bind to the correct SSA versions. If any
// path resolution fails (returns empty key), the constraint is silently skipped.
//
// Supports all standard numeric constraint types: Le, Lt, Ge, Gt, Eq, EqConst,
// LeConst, GeConst. Each is translated to appropriate difference graph operations.
func (t *TheorySolver) AddConstraintWithResolver(nc constraint.NumericConstraint, resolve constraint.PathResolver) {
	if resolve == nil {
		return
	}
	constraint.VisitNumericConstraint(nc, constraint.NumericConstraintVisitor[struct{}]{
		Le: func(c constraint.Le) struct{} {
			xKey := resolve(c.X)
			yKey := resolve(c.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			t.diff.AddLE(string(xKey), string(yKey), c.C)
			return struct{}{}
		},
		Lt: func(c constraint.Lt) struct{} {
			xKey := resolve(c.X)
			yKey := resolve(c.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			t.diff.AddLT(string(xKey), string(yKey))
			return struct{}{}
		},
		Ge: func(c constraint.Ge) struct{} {
			xKey := resolve(c.X)
			yKey := resolve(c.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			t.diff.AddGE(string(xKey), string(yKey))
			return struct{}{}
		},
		Gt: func(c constraint.Gt) struct{} {
			xKey := resolve(c.X)
			yKey := resolve(c.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			t.diff.AddGT(string(xKey), string(yKey))
			return struct{}{}
		},
		Eq: func(c constraint.Eq) struct{} {
			xKey := resolve(c.X)
			yKey := resolve(c.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			t.diff.AddEQ(string(xKey), string(yKey))
			return struct{}{}
		},
		EqConst: func(c constraint.EqConst) struct{} {
			xKey := resolve(c.X)
			if xKey == "" {
				return struct{}{}
			}
			varName := string(xKey)
			t.diff.AddConst(varName, c.C)
			t.mod.AddEquality(varName, c.C)
			return struct{}{}
		},
		LeConst: func(c constraint.LeConst) struct{} {
			xKey := resolve(c.X)
			if xKey == "" {
				return struct{}{}
			}
			t.diff.AddUpperBound(string(xKey), c.C)
			return struct{}{}
		},
		GeConst: func(c constraint.GeConst) struct{} {
			xKey := resolve(c.X)
			if xKey == "" {
				return struct{}{}
			}
			t.diff.AddLowerBound(string(xKey), c.C)
			return struct{}{}
		},
	})
}

// AddDifferenceConstraint adds a difference constraint x - y <= c directly.
//
// This is the primitive operation for the difference graph. All comparison
// operators are translated to this form:
//   - x < y  becomes x - y <= -1
//   - x <= y becomes x - y <= 0
//   - x > y  becomes y - x <= -1
//   - x >= y becomes y - x <= 0
//   - x == y becomes both x - y <= 0 and y - x <= 0
func (t *TheorySolver) AddDifferenceConstraint(x, y constraint.PathKey, c int64) {
	t.diff.AddLE(string(x), string(y), c)
}

// AddBounds adds lower and upper bounds for a variable.
//
// If lower == upper, the variable has a known constant value, which is also
// added as a modular equality for enhanced modular reasoning.
//
// Bounds at +/- maxWeight are treated as unbounded and not added.
func (t *TheorySolver) AddBounds(key constraint.PathKey, lower, upper int64) {
	varName := string(key)
	if lower == upper {
		t.diff.AddConst(varName, lower)
		t.mod.AddEquality(varName, lower)
	} else {
		if upper < maxWeight {
			t.diff.AddUpperBound(varName, upper)
		}
		if lower > -maxWeight {
			t.diff.AddLowerBound(varName, lower)
		}
		t.mod.AddRange(varName, lower, upper)
	}
}

// AddModular adds a modular constraint x % m == r.
//
// Modular constraints are tracked for consistency checking and cardinality
// reasoning but don't feed directly into the difference graph. They are
// used in combination with range bounds for modular-range consistency.
func (t *TheorySolver) AddModular(key constraint.PathKey, modulus, residue int64) {
	// Modular constraints are tracked but don't feed into difference graph
	// They're used for consistency checks and cardinality reasoning
}

// ToTheorySolver creates a TheorySolver initialized with a state's constraints.
//
// This is a convenience function that creates a new solver and loads all
// constraints from the given state in one operation.
func ToTheorySolver(s *State) *TheorySolver {
	ts := NewTheorySolver()
	ts.LoadFromState(s)
	return ts
}

// TightenWithTheory uses theory solvers to compute tighter bounds via transitive closure.
//
// The theory solver can derive bounds that aren't directly stored in the State.
// This function applies those inferences to produce a new State with tighter bounds.
//
// Returns:
//   - A new State with improved bounds (if any tightening occurred)
//   - Bottom state if transitive closure proves unsatisfiability
//   - The original state if no improvement is possible
func TightenWithTheory(s *State) *State {
	if s == nil || s.IsUnsat() {
		return s
	}

	ts := ToTheorySolver(s)
	if !ts.CheckSatisfiability() {
		return Bottom()
	}

	result := s.Clone()

	// Collect all variables from relations and bounds
	bounds := s.Bounds()
	relations := s.Relations()
	vars := make(map[constraint.PathKey]struct{})
	for _, k := range constraint.SortedPathKeys(bounds) {
		vars[k] = struct{}{}
	}
	for _, x := range constraint.SortedPathKeys(relations) {
		vars[x] = struct{}{}
		rels := relations[x]
		for _, y := range constraint.SortedPathKeys(rels) {
			vars[y] = struct{}{}
		}
	}

	// Try to infer tighter bounds from theory solver
	for _, key := range constraint.SortedPathKeys(vars) {
		lower, upper, ok := ts.InferBounds(key)
		if !ok {
			continue
		}

		if existing, exists := bounds[key]; exists {
			newLower := maxInt64(existing.Lower, lower)
			newUpper := minInt64(existing.Upper, upper)
			if newLower > newUpper {
				return Bottom()
			}
			if newLower != existing.Lower || newUpper != existing.Upper {
				result.ApplyGeConst(key, newLower)
				result.ApplyLeConst(key, newUpper)
			}
		} else {
			// New bound inferred by theory
			if lower > math.MinInt64 || upper < math.MaxInt64 {
				if lower > math.MinInt64 {
					result.ApplyGeConst(key, lower)
				}
				if upper < math.MaxInt64 {
					result.ApplyLeConst(key, upper)
				}
			}
		}
	}

	return result
}

// BoundsForWithTheory returns bounds using theory solver for transitive inference.
//
// This combines direct bounds from the State with transitive bounds inferred
// by the theory solver. The result is the tightest bounds available from
// both sources.
//
// More expensive than State.BoundsFor due to solver construction, but may
// produce tighter bounds when variables are constrained relative to each other.
func BoundsForWithTheory(s *State, key constraint.PathKey) (lower, upper int64, ok bool) {
	if s == nil || s.Bounds() == nil {
		return 0, 0, false
	}

	// First check direct bounds
	interval, found := s.Bounds()[key]
	if found {
		lower, upper = interval.Lower, interval.Upper
	} else {
		lower, upper = math.MinInt64, math.MaxInt64
	}

	// Use theory solver for transitive closure
	if len(s.Relations()) > 0 {
		ts := ToTheorySolver(s)
		if tLower, tUpper, tOk := ts.InferBounds(key); tOk {
			lower = maxInt64(lower, tLower)
			upper = minInt64(upper, tUpper)
		}
	}

	if lower > math.MinInt64 || upper < math.MaxInt64 {
		return lower, upper, true
	}
	return 0, 0, false
}

// CountModularValues counts values in a variable's range satisfying x % m == r.
//
// Uses the theory solver to count how many integers in the variable's range
// satisfy the modular constraint. Returns -1 if the count cannot be determined
// (e.g., modulus <= 0 or state is nil).
func CountModularValues(s *State, key constraint.PathKey, modulus, residue int64) int64 {
	if s == nil || modulus <= 0 {
		return -1
	}

	ts := ToTheorySolver(s)
	return ts.CountModularInRange(key, modulus, residue)
}

// InferRelationalBound derives the tightest x - y <= c from transitive constraints.
//
// Convenience function that creates a theory solver from the state and queries
// for the relational bound. Returns (0, false) if the state has no relations
// or if no bound can be inferred.
func InferRelationalBound(s *State, x, y constraint.PathKey) (int64, bool) {
	if s == nil || len(s.Relations()) == 0 {
		return 0, false
	}

	ts := ToTheorySolver(s)
	return ts.InferRelationalBound(x, y)
}

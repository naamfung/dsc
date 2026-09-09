package theory

// ModularFact represents the constraint x ≡ residue (mod modulus).
//
// This encodes that a variable x, when divided by modulus, leaves remainder
// equal to residue. For example:
//
//	x % 2 == 0  →  ModularFact{Modulus: 2, Residue: 0}  (x is even)
//	x % 2 == 1  →  ModularFact{Modulus: 2, Residue: 1}  (x is odd)
//	x % 3 == 2  →  ModularFact{Modulus: 3, Residue: 2}
//
// Residue is always normalized to [0, Modulus).
type ModularFact struct {
	// Modulus is the divisor (must be positive).
	Modulus int64

	// Residue is the remainder, normalized to [0, Modulus).
	Residue int64
}

// Check returns true if value satisfies this modular constraint.
//
// Handles negative values correctly using Go's modulo semantics
// with adjustment to ensure non-negative residue.
func (f ModularFact) Check(value int64) bool {
	if f.Modulus <= 0 {
		return false
	}

	r := value % f.Modulus
	if r < 0 {
		r += f.Modulus
	}

	return r == f.Residue
}

// ModularSolver tracks modular arithmetic constraints for variables.
//
// The solver maintains two types of information:
//  1. Known concrete values (from equalities like x == 6)
//  2. Known ranges (from bounds like 0 ≤ x ≤ 10)
//
// From concrete values, we can derive exact modular facts.
// From ranges, we can count how many values satisfy a modular predicate.
type ModularSolver struct {
	// equalities maps variable names to their known concrete values.
	equalities map[string]int64

	// ranges maps variable names to their known [lower, upper] bounds.
	ranges map[string][2]int64
}

// NewModularSolver creates a new modular arithmetic solver.
func NewModularSolver() *ModularSolver {
	return &ModularSolver{
		equalities: make(map[string]int64),
		ranges:     make(map[string][2]int64),
	}
}

// AddEquality records that variable x equals a concrete value.
//
// This allows deriving all modular facts about x. For example,
// if x == 6, we can derive x % 2 == 0, x % 3 == 0, x % 6 == 0, etc.
func (s *ModularSolver) AddEquality(variable string, value int64) {
	s.equalities[variable] = value
}

// AddRange records that variable x is in the range [lower, upper].
//
// This enables counting how many values in the range satisfy a
// modular predicate, used for filter length reasoning.
func (s *ModularSolver) AddRange(variable string, lower, upper int64) {
	s.ranges[variable] = [2]int64{lower, upper}
}

// GetFacts returns all derived modular facts for a variable.
//
// If the variable has a known concrete value, returns modular facts
// for common moduli (2, 3, 4, 5, 6, 8, 10). Otherwise returns nil.
func (s *ModularSolver) GetFacts(variable string) []ModularFact {
	value, ok := s.equalities[variable]
	if !ok {
		return nil
	}

	moduli := []int64{2, 3, 4, 5, 6, 8, 10}
	facts := make([]ModularFact, 0, len(moduli))

	for _, m := range moduli {
		r := value % m
		if r < 0 {
			r += m
		}

		facts = append(facts, ModularFact{Modulus: m, Residue: r})
	}

	return facts
}

// IsConsistent checks if a modular constraint is consistent with known facts.
//
// Returns true if x % modulus == residue could possibly be true given
// what we know about x. Returns false only if we can prove inconsistency.
func (s *ModularSolver) IsConsistent(variable string, modulus, residue int64) bool {
	if modulus <= 0 {
		return false
	}

	r := residue % modulus
	if r < 0 {
		r += modulus
	}

	if value, ok := s.equalities[variable]; ok {
		fact := ModularFact{Modulus: modulus, Residue: r}
		return fact.Check(value)
	}

	if bounds, ok := s.ranges[variable]; ok {
		return s.countInRangeImpl(bounds[0], bounds[1], modulus, r) > 0
	}

	return true
}

// CountInRange returns the count of values in the variable's range
// that satisfy x % modulus == residue.
//
// If the variable has no known range, returns -1 (unknown).
// Used for determining filter output lengths.
func (s *ModularSolver) CountInRange(variable string, modulus, residue int64) int64 {
	bounds, ok := s.ranges[variable]
	if !ok {
		return -1
	}

	return s.countInRangeImpl(bounds[0], bounds[1], modulus, residue)
}

// countInRangeImpl counts integers in [lower, upper] where x % modulus == residue.
//
// The formula is based on counting how many complete "cycles" of the modulus
// fit in the range, plus checking the partial cycles at the ends.
func (s *ModularSolver) countInRangeImpl(lower, upper, modulus, residue int64) int64 {
	if upper < lower || modulus <= 0 {
		return 0
	}

	r := residue % modulus
	if r < 0 {
		r += modulus
	}

	first := lower

	rem := first % modulus
	if rem < 0 {
		rem += modulus
	}

	if rem <= r {
		first = lower + (r - rem)
	} else {
		first = lower + (modulus - rem + r)
	}

	if first > upper {
		return 0
	}

	return (upper-first)/modulus + 1
}

// Clone creates a deep copy of the modular solver.
func (s *ModularSolver) Clone() *ModularSolver {
	newS := &ModularSolver{
		equalities: make(map[string]int64, len(s.equalities)),
		ranges:     make(map[string][2]int64, len(s.ranges)),
	}

	for k, v := range s.equalities {
		newS.equalities[k] = v
	}

	for k, v := range s.ranges {
		newS.ranges[k] = v
	}

	return newS
}

package theory

// Result represents the outcome of a satisfiability or entailment check.
//
// Theories use three-valued logic because constraint solving is often
// incomplete - a theory may not have enough information to determine
// satisfiability definitively.
//
// The three values form a lattice:
//
//	    Unknown
//	    /     \
//	Valid    Invalid
//
// When combining results from multiple theories:
//   - If any theory returns Invalid, the combined result is Invalid
//   - If all theories return Valid, the combined result is Valid
//   - Otherwise, the combined result is Unknown
type Result int

const (
	// Valid indicates the constraint is definitely satisfiable.
	// There exists at least one assignment of values to variables
	// that makes all constraints true.
	Valid Result = iota

	// Invalid indicates the constraint is definitely unsatisfiable.
	// No assignment of values to variables can make all constraints true.
	// The theory has found a proof of unsatisfiability
	// (e.g., a negative cycle in difference logic).
	Invalid

	// Unknown indicates the theory cannot determine satisfiability.
	// This is NOT an error - it simply means the theory's decision
	// procedure is incomplete for this particular constraint.
	// Another theory or solver may still decide it.
	Unknown
)

// String returns a human-readable representation of the result.
func (r Result) String() string {
	switch r {
	case Valid:
		return "valid"
	case Invalid:
		return "invalid"
	case Unknown:
		return "unknown"
	default:
		return "?"
	}
}

// Combine merges two results following the lattice rules.
// Invalid dominates (if either is Invalid, result is Invalid).
// Valid requires both to be Valid.
// Otherwise Unknown.
func (r Result) Combine(other Result) Result {
	if r == Invalid || other == Invalid {
		return Invalid
	}

	if r == Valid && other == Valid {
		return Valid
	}

	return Unknown
}

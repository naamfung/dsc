// domain.go implements the numeric subdomain for the flow solver's ProductDomain.
//
// The numeric Domain tracks integer constraints (bounds, orderings, modular residues)
// and integrates State (compact storage) with TheorySolver (transitive inference).
// It satisfies the domain.Domain interface for use in the constraint solving framework.
package numeric

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/domain"
)

// Domain implements the numeric subdomain for ProductDomain.
//
// Domain tracks numeric constraints and bounds for variables. It combines two
// complementary analysis components:
//
//   - State: Tracks bounds (lower/upper), orderings (x < y), and modular
//     constraints (x ≡ r mod m) for each path key.
//
//   - TheorySolver: Implements difference-bound matrix (DBM) analysis for
//     transitive reasoning about inequalities. Detects unsatisfiability
//     when constraints form negative cycles.
//
// Supported constraints:
//   - Lt (x < y): Strict ordering, translated to difference constraint x - y ≤ -1
//   - Le (x ≤ y): Non-strict ordering, translated to x - y ≤ 0
//   - Gt (x > y): Strict ordering, translated to y - x ≤ -1
//   - Ge (x ≥ y): Non-strict ordering, translated to y - x ≤ 0
//   - Eq (x = c): Exact value, sets both lower and upper bounds to c
//   - ModEq (x ≡ r mod m): Modular arithmetic constraint
//
// When combined with type narrowing, numeric domain enables patterns like:
//
//	if x >= 0 and x <= 10 then
//	    -- x is known to be in range [0, 10]
//	end
//
// The domain maintains soundness by eagerly checking the theory solver after
// each constraint application. If constraints are mutually unsatisfiable
// (e.g., x > 10 and x < 5), the domain becomes UNSAT.
type Domain struct {
	state  *State
	theory *TheorySolver
	env    constraint.Env
}

// NewDomain creates a new numeric Domain with empty state.
//
// Both State and TheorySolver are initialized empty. Constraints are added
// via ApplyAtom calls.
func NewDomain(env constraint.Env) *Domain {
	return &Domain{
		state:  NewState(),
		theory: NewTheorySolver(),
		env:    env,
	}
}

// ApplyAtom applies a numeric constraint atom to the domain.
//
// The atom is applied to both State (for bounds tracking) and TheorySolver
// (for transitive reasoning). After application, satisfiability is checked.
//
// Supported atoms:
//   - AtomKindLt: x < y (variable-variable comparison)
//   - AtomKindLe: x ≤ y or x ≤ c (variable-variable or variable-constant)
//   - AtomKindGe: x ≥ y or x ≥ c (variable-variable or variable-constant)
//   - AtomKindGt: x > y (variable-variable comparison)
//   - AtomKindEq: x = c or x = y (equality constraint)
//   - AtomKindModEq: x ≡ r (mod m) (modular arithmetic)
//
// Returns false if the constraint makes the domain unsatisfiable.
func (d *Domain) ApplyAtom(atom constraint.Atom) bool {
	if d.state == nil || d.state.IsUnsat() {
		return false
	}

	switch atom.Kind {
	case constraint.AtomKindLt:
		if atom.Left.IsVar() && atom.Right.IsVar() {
			d.state.ApplyLt(atom.Left.Path, atom.Right.Path)
			d.theory.AddDifferenceConstraint(atom.Left.Path, atom.Right.Path, -1)
		}
	case constraint.AtomKindLe:
		if atom.Left.IsVar() && atom.Right.IsConst() {
			d.state.ApplyLeConst(atom.Left.Path, atom.Right.Const)
			d.theory.AddBounds(atom.Left.Path, -maxWeight, atom.Right.Const)
		} else if atom.Left.IsVar() && atom.Right.IsLen() {
			d.state.ApplyLeLenOf(atom.Left.Path, atom.Right.Path)
		} else if atom.Left.IsVar() && atom.Right.IsVar() {
			d.state.ApplyLe(atom.Left.Path, atom.Right.Path)
			d.theory.AddDifferenceConstraint(atom.Left.Path, atom.Right.Path, 0)
		}
	case constraint.AtomKindGe:
		if atom.Left.IsVar() && atom.Right.IsConst() {
			d.state.ApplyGeConst(atom.Left.Path, atom.Right.Const)
			d.theory.AddBounds(atom.Left.Path, atom.Right.Const, maxWeight)
		} else if atom.Left.IsVar() && atom.Right.IsVar() {
			d.state.ApplyGe(atom.Left.Path, atom.Right.Path)
			d.theory.AddDifferenceConstraint(atom.Right.Path, atom.Left.Path, 0)
		}
	case constraint.AtomKindGt:
		if atom.Left.IsVar() && atom.Right.IsVar() {
			d.state.ApplyGt(atom.Left.Path, atom.Right.Path)
			d.theory.AddDifferenceConstraint(atom.Right.Path, atom.Left.Path, -1)
		}
	case constraint.AtomKindEq:
		if atom.Left.IsVar() && atom.Right.IsConst() {
			d.state.ApplyEqConst(atom.Left.Path, atom.Right.Const)
			d.theory.AddBounds(atom.Left.Path, atom.Right.Const, atom.Right.Const)
		} else if atom.Left.IsVar() && atom.Right.IsVar() {
			d.state.ApplyEq(atom.Left.Path, atom.Right.Path)
			d.theory.AddDifferenceConstraint(atom.Left.Path, atom.Right.Path, 0)
			d.theory.AddDifferenceConstraint(atom.Right.Path, atom.Left.Path, 0)
		}
	case constraint.AtomKindModEq:
		if atom.Left.IsVar() {
			d.state.ApplyModEq(atom.Left.Path, atom.Mod, atom.Rem)
			d.theory.AddModular(atom.Left.Path, atom.Mod, atom.Rem)
		}
	}

	// Check theory solver for early UNSAT detection
	if !d.theory.CheckSatisfiability() {
		d.state.SetUnsat()
		return false
	}

	return !d.state.IsUnsat()
}

// IsUnsat returns true if numeric constraints are unsatisfiable.
func (d *Domain) IsUnsat() bool {
	return d.state != nil && d.state.IsUnsat()
}

// Clone creates a deep copy of the Domain for speculative evaluation.
//
// Both State and TheorySolver are cloned to ensure independent modification.
func (d *Domain) Clone() domain.Domain {
	return &Domain{
		state:  d.state.Clone(),
		theory: d.theory.Clone(),
		env:    d.env,
	}
}

// Join computes the least upper bound of two numeric domains.
//
// Join semantics for numeric constraints:
//   - Bounds are widened: lower = min(d.lower, o.lower), upper = max(d.upper, o.upper)
//   - Only orderings present in BOTH domains are preserved
//   - Modular constraints are intersected
//
// The TheorySolver is rebuilt from the joined State to ensure consistency.
func (d *Domain) Join(other domain.Domain) domain.Domain {
	o := other.(*Domain)
	// Join State (takes intersection of facts)
	joinedState := Join(d.state, o.state)
	// Rebuild theory solver from joined state
	result := &Domain{
		state:  joinedState,
		theory: NewTheorySolver(),
		env:    d.env,
	}
	if joinedState != nil && !joinedState.IsUnsat() {
		result.theory.LoadFromState(joinedState)
	}
	return result
}

// State returns the underlying State for direct inspection.
//
// Provides access to the compact constraint storage for debugging, testing,
// or advanced queries. The returned State should not be modified directly;
// use Domain methods for constraint application.
func (d *Domain) State() *State {
	return d.state
}

// Theory returns the underlying TheorySolver for direct inspection.
//
// Provides access to the theory solver for debugging, testing, or advanced
// queries. The returned solver should not be modified directly outside of
// Domain method calls.
func (d *Domain) Theory() *TheorySolver {
	return d.theory
}

// BoundsForWithTheory returns bounds after transitive inference via the theory solver.
//
// This method uses the difference graph to derive tighter bounds from ordering
// constraints. For example, if x <= y and y <= 10, the method infers x <= 10
// even without a direct bound on x.
//
// Returns (lower, upper, true) if bounds exist (direct or inferred), or
// (0, 0, false) if no bounds are known or can be inferred.
func (d *Domain) BoundsForWithTheory(key constraint.PathKey) (lower, upper int64, ok bool) {
	return BoundsForWithTheory(d.state, key)
}

// TightenWithTheory returns a new Domain with bounds tightened via transitive closure.
//
// Applies the theory solver's transitive inference to compute tighter bounds
// for all variables. The original domain is not modified.
//
// This is useful for producing a "final" domain with maximum precision before
// querying bounds. Note that this can be expensive for large constraint sets.
func (d *Domain) TightenWithTheory() *Domain {
	tightened := TightenWithTheory(d.state)
	result := &Domain{
		state:  tightened,
		theory: NewTheorySolver(),
		env:    d.env,
	}
	if tightened != nil && !tightened.IsUnsat() {
		result.theory.LoadFromState(tightened)
	}
	return result
}

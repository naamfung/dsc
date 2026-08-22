package constraint

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// RefinementLookupBySym retrieves a function's inferred refinement by symbol ID.
//
// Used during call site analysis to determine what type refinements a
// function call produces. Returns nil if the function has no recorded refinement.
type RefinementLookupBySym func(sym cfg.SymbolID) *FunctionRefinement

// FunctionRefinement describes the type refinements a function produces.
//
// Refinements encode how a function call narrows types based on its return value.
// Three categories are supported:
//   - OnReturn: constraints that hold when the function returns normally
//     (used for assert-style functions that error() on failure)
//   - OnTrue: constraints that hold when the function returns truthy
//     (used for type predicate functions like isString(x))
//   - OnFalse: constraints that hold when the function returns falsy
//
// Placeholder roots ($0, $1, ...) reference parameters by position.
// At call sites, placeholders are substituted with actual argument paths.
type FunctionRefinement struct {
	// Row is the effect label set (IO, Mutate, Throw, etc.).
	// Stored as typ.EffectInfo to avoid circular import with effect package.
	// The concrete type is effect.Row.
	Row typ.EffectInfo

	// OnReturn: constraints that hold when function returns normally.
	// Used for assert-style functions that error() on failure.
	OnReturn Condition

	// OnTrue: constraints that hold when function returns truthy.
	// Used for type predicate functions like isString(x).
	OnTrue Condition

	// OnFalse: constraints that hold when function returns falsy.
	OnFalse Condition

	// Terminates indicates the function never returns normally.
	// Used for functions that always call error() or similar.
	Terminates bool
}

// NewRefinement creates a FunctionRefinement from constraint slices.
func NewRefinement(onReturn, onTrue, onFalse []Constraint) *FunctionRefinement {
	return &FunctionRefinement{
		OnReturn: FromConstraints(onReturn...),
		OnTrue:   FromConstraints(onTrue...),
		OnFalse:  FromConstraints(onFalse...),
	}
}

// IsEmpty returns true if the effect has no constraints, no row, and doesn't terminate.
func (e *FunctionRefinement) IsEmpty() bool {
	if e == nil {
		return true
	}

	return e.Row == nil && !e.OnReturn.HasConstraints() && !e.OnTrue.HasConstraints() && !e.OnFalse.HasConstraints() && !e.Terminates
}

// HasAssertSemantics returns true if function has assert-style semantics.
func (e *FunctionRefinement) HasAssertSemantics() bool {
	return e != nil && e.OnReturn.HasConstraints()
}

// HasPredicateSemantics returns true if function has predicate semantics.
func (e *FunctionRefinement) HasPredicateSemantics() bool {
	return e != nil && (e.OnTrue.HasConstraints() || e.OnFalse.HasConstraints())
}

// Equals returns true if two function refinements are structurally equal.
// Implements internal.Equaler interface for use in typ.Function.
func (e *FunctionRefinement) Equals(other any) bool {
	if other == nil {
		return e == nil
	}

	o, ok := other.(*FunctionRefinement)

	if !ok {
		return false
	}

	if e == nil && o == nil {
		return true
	}

	if e == nil || o == nil {
		return false
	}

	if !effectRowEquals(e.Row, o.Row) {
		return false
	}

	return e.OnReturn.Equals(o.OnReturn) &&
		e.OnTrue.Equals(o.OnTrue) &&
		e.OnFalse.Equals(o.OnFalse) &&
		e.Terminates == o.Terminates
}

// effectRowEquals compares two effect rows stored as typ.EffectInfo.
func effectRowEquals(a, b typ.EffectInfo) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
}

// IsRefinementInfo implements typ.RefinementInfo.
func (e *FunctionRefinement) IsRefinementInfo() {}

// Substitute returns a new FunctionRefinement with placeholder paths replaced.
//
// At a call site, parameter placeholders ($0, $1, ...) are replaced with
// the actual argument paths, producing concrete constraints that can be
// applied to narrow types at the call location.
func (e *FunctionRefinement) Substitute(args []Path) *FunctionRefinement {
	if e == nil || e.IsEmpty() {
		return nil
	}

	result := &FunctionRefinement{
		OnReturn: e.OnReturn.Substitute(args),
		OnTrue:   e.OnTrue.Substitute(args),
		OnFalse:  e.OnFalse.Substitute(args),
	}
	if result.IsEmpty() {
		return nil
	}

	return result
}

// KeysCollectorInfo reports KeyOf-based keys-collector behavior.
//
// Detects patterns like `for k in pairs(t)` where the function returns keys
// from a table parameter.
//
// Detection looks for KeyOf constraints in OnReturn where the table is a
// parameter placeholder ($N) and the key is a return path.
func (e *FunctionRefinement) KeysCollectorInfo() (paramIndex int, returnIndex int, ok bool) {
	if e == nil || !e.OnReturn.HasConstraints() {
		return 0, 0, false
	}

	var candidateParamIdx int
	var candidateReturnIdx int
	found := false

	for _, disj := range e.OnReturn.Disjuncts {
		for _, c := range disj {
			if keyOf, ok := c.(KeyOf); ok {
				if !keyOf.Table.IsPlaceholder() || !IsReturnPath(keyOf.Key) {
					continue
				}
				paramIdx := keyOf.Table.PlaceholderIndex()
				returnIdx := ReturnIndexFromString(keyOf.Key.Root)
				if paramIdx >= 0 && returnIdx >= 0 {
					if !found {
						candidateParamIdx = paramIdx
						candidateReturnIdx = returnIdx
						found = true
						continue
					}
					if candidateParamIdx != paramIdx || candidateReturnIdx != returnIdx {
						return 0, 0, false
					}
				}
			}
		}
	}
	if !found {
		return 0, 0, false
	}
	return candidateParamIdx, candidateReturnIdx, true
}

// KeysCollectorParamIndex checks if the function returns keys of a parameter.
//
// Returns the parameter index (0-based) if found, or -1 otherwise.
func (e *FunctionRefinement) KeysCollectorParamIndex() int {
	paramIdx, _, ok := e.KeysCollectorInfo()
	if !ok {
		return -1
	}
	return paramIdx
}

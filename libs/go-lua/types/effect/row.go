// Package effect provides row-polymorphic effect tracking for Lua type checking.
//
// Effect rows describe the observable behaviors of functions beyond their type
// signatures. The system is inspired by Koka's row-polymorphic effect types,
// adapted for gradual typing in Lua.
//
// # Row Polymorphism
//
// An effect row is a set of effect labels with an optional tail variable:
//
//   - Closed row {throw, io}: exactly these effects, no polymorphism
//   - Open row {throw | e}: throw plus whatever effects e contains
//   - Unknown row {?}: gradual typing for untyped Lua (assumes any effect)
//   - Empty row {}: pure function with no effects
//
// The tail variable enables effect polymorphism: a function can accept callbacks
// with arbitrary additional effects and propagate those effects to its result.
//
// # Effect Labels
//
// Labels describe specific function behaviors:
//
// Control effects track exceptional control flow:
//   - Throw: function may raise an error via error()
//   - Diverge: function may not terminate (infinite loops)
//   - IO: function performs I/O operations
//
// Mutation effects track type-level state changes:
//   - Mutate: modifies a parameter's type (e.g., table.insert widens array element type)
//   - LengthChange: modifies array length (+1 for insert, -1 for remove)
//   - TableMutator: specialized mutation for table operations
//
// Ownership effects track value lifecycle:
//   - Borrow: temporary read access, value can be released after call
//   - Store: persistent storage, value escapes the call
//   - Send: cross-actor transfer, value becomes frozen
//   - Freeze: immutability marker for shared values
//
// Return effects track type derivations:
//   - Return: describes how return type derives from parameters
//   - ErrorReturn: encodes the Lua (value, error) return pattern
//   - ReturnLength: relates return array length to parameter lengths
//   - CorrelatedReturn: marks return positions that are nil/non-nil together
//
// Iterator effects describe iteration semantics:
//   - Iterator: marks ipairs/pairs style iteration with source and kind
//
// Flow effects track value identity:
//   - PassThrough: parameter flows unchanged to return position
//   - FlowInto: parameter flows into a field of the returned value
//
// Semantic effects enable special handling:
//   - ModuleLoad: require-like module loading
//   - TypePredicate: type()-like type inspection
//   - VariadicTransform: select-like variadic manipulation
//   - CallableType: TypeName(x) constructor pattern
//
// # Row Operations
//
// The package provides set-theoretic operations on effect rows:
//
//   - Union: sequential composition, combines effects from both rows
//   - Intersect: common effects, keeps only labels in both rows
//   - Subset: containment check for effect compatibility
//
// # Serialization
//
// Effect labels support binary serialization via the codec registry for cross-module
// type manifest storage. Each label type registers its codec in init().
//
// # Usage
//
// Effects are attached to function types via the builder pattern:
//
//	fn := typ.Func().
//	    Param("t", typ.NewArray(typ.String)).
//	    Param("val", typ.String).
//	    Returns(typ.Nil).
//	    Effects(effect.Row{Labels: []effect.Label{
//	        effect.Mutate{Target: effect.ParamRef{Index: 0}, Transform: effect.ElementUnion{Source: effect.ParamRef{Index: 1}}},
//	        effect.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1},
//	    }}).
//	    Build()
//
// During type checking, effect rows are unified and propagated through call sites
// to verify effect compatibility and derive precise types.
package effect

import (
	"fmt"
	"strings"
)

// Var represents an effect variable for row polymorphism.
//
// Effect variables enable functions to be polymorphic over their effects,
// similar to how type parameters enable polymorphism over types. A function
// with effect row {throw | e} can be called with any callback that throws,
// and the variable e captures additional effects from the callback.
//
// The special variable name "?" represents the unknown effect for gradual
// typing, indicating the function may have any effect.
//
// During effect unification, variables with the same name are considered equal.
// Different variables create merged tails with names like "e1∪e2".
type Var struct {
	Name string
}

// String returns the variable name.
func (v *Var) String() string {
	if v == nil {
		return ""
	}

	return v.Name
}

// Row represents an effect row: a set of labels with optional tail variable.
//
// A Row is the fundamental unit of effect tracking. It contains zero or more
// concrete effect labels describing known behaviors, plus an optional tail
// variable enabling effect polymorphism.
//
// Row semantics:
//
//   - Empty row {} (Pure returns true): The function is pure with no observable
//     effects. This is the strongest guarantee and enables optimizations.
//
//   - Closed row {throw, io} (IsClosed returns true, Tail is nil): The function
//     has exactly these effects and no others. Used for complete specifications.
//
//   - Open row {throw | e} (IsOpen returns true, Tail is non-nil): The function
//     has at least throw, plus whatever additional effects are bound to e.
//     Used for polymorphic functions that propagate callee effects.
//
//   - Unknown row {?} (IsUnknown returns true): The function may have any effect.
//     Used for untyped Lua code during gradual typing migration.
//
// Rows support set operations via Union, Intersect, and Subset. The With and
// Without methods create modified copies without mutating the original.
//
// The Labels slice uses semantic equality via Equals, not pointer equality,
// so two rows with structurally identical labels compare equal.
type Row struct {
	Labels []Label // Concrete effects in this row
	Tail   *Var    // Effect variable for polymorphism (nil = closed row)
}

// Empty is the empty effect row (pure function).
var Empty = Row{}

// Unknown is the unknown effect row (gradual typing for untyped Lua).
// Functions with unknown effects are assumed to potentially have any effect.
var Unknown = Row{Tail: &Var{Name: "?"}}

// Pure returns true if this row has no effects.
func (r Row) Pure() bool {
	return len(r.Labels) == 0 && r.Tail == nil
}

// IsClosed returns true if this row has no tail variable.
func (r Row) IsClosed() bool {
	return r.Tail == nil
}

// IsOpen returns true if this row has a tail variable.
func (r Row) IsOpen() bool {
	return r.Tail != nil
}

// IsUnknown returns true if this is the unknown effect row.
func (r Row) IsUnknown() bool {
	return r.Tail != nil && r.Tail.Name == "?"
}

// Has checks if the row contains a specific label type.
func (r Row) Has(check func(Label) bool) bool {
	for _, l := range r.Labels {
		if check(l) {
			return true
		}
	}

	return false
}

// HasThrow returns true if throw is in the row.
func (r Row) HasThrow() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Throw); return ok })
}

// HasIO returns true if io is in the row.
func (r Row) HasIO() bool {
	return r.Has(func(l Label) bool { _, ok := l.(IO); return ok })
}

// HasDiverge returns true if diverge is in the row.
func (r Row) HasDiverge() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Diverge); return ok })
}

// HasMutate returns true if any mutation is in the row.
func (r Row) HasMutate() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Mutate); return ok })
}

// HasIterator returns true if any iterator effect is in the row.
func (r Row) HasIterator() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Iterator); return ok })
}

// HasTableMutator returns true if any table mutator effect is in the row.
func (r Row) HasTableMutator() bool {
	return r.Has(func(l Label) bool { _, ok := l.(TableMutator); return ok })
}

// GetMutate returns the mutation effect for a specific parameter, if any.
//
// Returns nil if the row contains no Mutate label targeting the given parameter.
// The paramIdx is 0-based, with -1 indicating the last variadic argument.
func (r Row) GetMutate(paramIdx int) *Mutate {
	for _, l := range r.Labels {
		if m, ok := l.(Mutate); ok && m.Target.Index == paramIdx {
			return &m
		}
	}

	return nil
}

// GetReturn returns the return type derivation effect for a specific return position.
//
// Used by the type checker to derive precise return types based on parameter types.
// For example, table.remove(t) returns the element type of t, encoded as:
//
//	Return{ReturnIndex: 0, Transform: OptionalElementOf{Source: ParamRef{Index: 0}}}
//
// Returns nil if no Return label exists for the given return index.
func (r Row) GetReturn(retIdx int) *Return {
	for _, l := range r.Labels {
		if ret, ok := l.(Return); ok && ret.ReturnIndex == retIdx {
			return &ret
		}
	}

	return nil
}

// GetErrorReturn returns the error-return correlation for a specific value position.
//
// Encodes the Lua pattern where a function returns (value, nil) on success or
// (nil, error) on failure. The type checker uses this to narrow types after
// checking the error return position.
//
// Returns nil if no ErrorReturn label exists for the given value index.
func (r Row) GetErrorReturn(valueIdx int) *ErrorReturn {
	for _, l := range r.Labels {
		if er, ok := l.(ErrorReturn); ok && er.ValueIndex == valueIdx {
			return &er
		}
	}

	return nil
}

// GetCorrelatedReturn returns the correlated return effect that includes the given index.
func (r Row) GetCorrelatedReturn(idx int) *CorrelatedReturn {
	for _, l := range r.Labels {
		if cr, ok := l.(CorrelatedReturn); ok {
			for _, i := range cr.Indices {
				if i == idx {
					return &cr
				}
			}
		}
	}
	return nil
}

// GetReturnLength returns the return length effect for a specific return index.
func (r Row) GetReturnLength(retIdx int) *ReturnLength {
	for _, l := range r.Labels {
		if ret, ok := l.(ReturnLength); ok && ret.ReturnIndex == retIdx {
			return &ret
		}
	}

	return nil
}

// GetIterator returns the first iterator effect in the row.
func (r Row) GetIterator() *Iterator {
	for _, l := range r.Labels {
		if iter, ok := l.(Iterator); ok {
			return &iter
		}
	}

	return nil
}

// GetTableMutator returns the first table mutator effect in the row.
func (r Row) GetTableMutator() *TableMutator {
	for _, l := range r.Labels {
		if mut, ok := l.(TableMutator); ok {
			return &mut
		}
	}

	return nil
}

// IsIndexedIterator returns true if the row has an indexed iterator (ipairs-style).
func (r Row) IsIndexedIterator() bool {
	iter := r.GetIterator()
	return iter != nil && iter.Kind == IterateIndexed
}

// IsKeyedIterator returns true if the row has a keyed iterator (pairs-style).
func (r Row) IsKeyedIterator() bool {
	iter := r.GetIterator()
	return iter != nil && iter.Kind == IterateKeyed
}

// String formats the effect row for display.
func (r Row) String() string {
	if r.Pure() {
		return "{}"
	}

	parts := make([]string, 0, len(r.Labels))
	for _, l := range r.Labels {
		parts = append(parts, l.String())
	}

	if r.Tail != nil {
		if len(parts) == 0 {
			return fmt.Sprintf("{%s}", r.Tail.Name)
		}

		return fmt.Sprintf("{%s | %s}", strings.Join(parts, ", "), r.Tail.Name)
	}

	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}

// With returns a new row with the given labels added.
//
// Labels are deduplicated using semantic equality (Equals), so adding a label
// already present in the row has no effect. The original row is not modified.
//
// Example:
//
//	pure := effect.Empty
//	throws := pure.With(effect.Throw{})
//	throwsAndIO := throws.With(effect.IO{})
func (r Row) With(labels ...Label) Row {
	newLabels := make([]Label, 0, len(r.Labels)+len(labels))
	newLabels = append(newLabels, r.Labels...)

	// Build a temporary row to track what we've added
	result := Row{Labels: newLabels, Tail: r.Tail}
	for _, l := range labels {
		if !containsLabelEquals(result.Labels, l) {
			result.Labels = append(result.Labels, l)
		}
	}

	return result
}

// Without returns a new row excluding labels that match the predicate.
//
// Used to filter out specific effect types, for example when a function
// handles an effect internally and should not propagate it to callers.
//
// Example:
//
//	// Remove all throw effects after wrapping in pcall
//	handled := row.Without(func(l Label) bool { _, ok := l.(Throw); return ok })
func (r Row) Without(match func(Label) bool) Row {
	var newLabels []Label

	for _, l := range r.Labels {
		if !match(l) {
			newLabels = append(newLabels, l)
		}
	}

	return Row{Labels: newLabels, Tail: r.Tail}
}

// Equals checks if two rows are equal (implements internal.Equaler interface).
func (r Row) Equals(other any) bool {
	otherRow, ok := other.(Row)
	if !ok {
		return false
	}

	return r.equalsRow(otherRow)
}

// IsEffectInfo implements typ.EffectInfo.
func (r Row) IsEffectInfo() {}

// equalsRow checks if two rows are equal.
func (r Row) equalsRow(other Row) bool {
	if len(r.Labels) != len(other.Labels) {
		return false
	}

	// Check all labels match
	for _, l := range r.Labels {
		if !containsLabelEquals(other.Labels, l) {
			return false
		}
	}

	// Check tail variables
	if r.Tail == nil && other.Tail == nil {
		return true
	}

	if r.Tail == nil || other.Tail == nil {
		return false
	}

	return r.Tail.Name == other.Tail.Name
}

// Constructors for common effect rows

// Throws creates a row with just the throw effect.
func Throws() Row {
	return Row{Labels: []Label{Throw{}}}
}

// Mutates creates a row with a mutation effect.
func Mutates(paramIdx int, transform TypeTransform) Row {
	return Row{Labels: []Label{Mutate{
		Target:    ParamRef{Index: paramIdx},
		Transform: transform,
	}}}
}

// Returns creates a row with a return type effect.
func Returns(retIdx int, derive ReturnType) Row {
	return Row{Labels: []Label{Return{ReturnIndex: retIdx, Transform: derive}}}
}

// WithIO creates an I/O effect row.
func WithIO() Row {
	return Row{Labels: []Label{IO{}}}
}

// MayDiverge creates a divergence effect row.
func MayDiverge() Row {
	return Row{Labels: []Label{Diverge{}}}
}

// BorrowsOnly creates a row indicating function only borrows all params.
func BorrowsOnly() Row {
	return Row{Labels: []Label{BorrowAll{}}}
}

// StoresParam creates a row indicating function stores a param.
func StoresParam(paramIdx int, intoIdx int) Row {
	return Row{Labels: []Label{Store{
		Param: ParamRef{Index: paramIdx},
		Into:  ParamRef{Index: intoIdx},
	}}}
}

// HasBorrow returns true if the row has any borrow effect.
func (r Row) HasBorrow() bool {
	return r.Has(func(l Label) bool {
		_, ok := l.(Borrow)
		if ok {
			return true
		}

		_, ok = l.(BorrowAll)

		return ok
	})
}

// HasStore returns true if the row has any store effect.
func (r Row) HasStore() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Store); return ok })
}

// OnlyBorrows returns true if function only borrows params (no store).
func (r Row) OnlyBorrows() bool {
	return r.HasBorrow() && !r.HasStore() && !r.HasMutate()
}

// GetBorrow returns borrow effect for a specific parameter, if any.
func (r Row) GetBorrow(paramIdx int) *Borrow {
	for _, l := range r.Labels {
		if b, ok := l.(Borrow); ok && b.Param.Index == paramIdx {
			return &b
		}
	}

	return nil
}

// GetStore returns store effect for a specific parameter, if any.
func (r Row) GetStore(paramIdx int) *Store {
	for _, l := range r.Labels {
		if s, ok := l.(Store); ok && s.Param.Index == paramIdx {
			return &s
		}
	}

	return nil
}

// BorrowsAllParams returns true if function has BorrowAll effect.
func (r Row) BorrowsAllParams() bool {
	return r.Has(func(l Label) bool { _, ok := l.(BorrowAll); return ok })
}

// HasModuleLoad returns true if the row has a module load effect.
func (r Row) HasModuleLoad() bool {
	return r.Has(func(l Label) bool { _, ok := l.(ModuleLoad); return ok })
}

// HasVariadicTransform returns true if the row has a variadic transform effect.
func (r Row) HasVariadicTransform() bool {
	return r.Has(func(l Label) bool { _, ok := l.(VariadicTransform); return ok })
}

// HasTypePredicate returns true if the row has a type predicate effect.
func (r Row) HasTypePredicate() bool {
	return r.Has(func(l Label) bool { _, ok := l.(TypePredicate); return ok })
}

// HasTypeValueMethod returns true if the row has a type value method effect.
func (r Row) HasTypeValueMethod() bool {
	return r.Has(func(l Label) bool { _, ok := l.(TypeValueMethod); return ok })
}

// HasCallableType returns true if the row has a callable type effect.
func (r Row) HasCallableType() bool {
	return r.Has(func(l Label) bool { _, ok := l.(CallableType); return ok })
}

// WithModuleLoad creates a row with the module load effect.
func WithModuleLoad() Row {
	return Row{Labels: []Label{ModuleLoad{}}}
}

// WithVariadicTransform creates a row with the variadic transform effect.
func WithVariadicTransform() Row {
	return Row{Labels: []Label{VariadicTransform{}}}
}

// WithTypePredicate creates a row with the type predicate effect.
func WithTypePredicate() Row {
	return Row{Labels: []Label{TypePredicate{}}}
}

// WithTypeValueMethod creates a row with the type value method effect.
func WithTypeValueMethod() Row {
	return Row{Labels: []Label{TypeValueMethod{}}}
}

// WithCallableType creates a row with the callable type effect.
func WithCallableType() Row {
	return Row{Labels: []Label{CallableType{}}}
}

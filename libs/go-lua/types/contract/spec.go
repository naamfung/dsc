// Package contract provides function behavior specifications using Hoare-style contracts.
//
// A contract (Spec) specifies the complete behavioral interface of a function:
//
// Preconditions (Requires):
// Constraints that must hold at the call site. The type checker verifies these
// at each call and reports errors when preconditions cannot be proven.
//
//	-- Precondition: first argument must be non-nil
//	function assert_not_nil(x)
//	    if x == nil then error("nil!") end
//	    return x
//	end
//
// Postconditions (Ensures):
// Facts that become true after the call returns. The type checker adds these
// to the type environment for subsequent code, enabling narrowing.
//
//	-- Postcondition: return value is non-nil
//	local y = assert_not_nil(x)  -- y is narrowed to non-nil
//
// Effects:
// Type-level side effects from types/effect, including mutations, return type
// derivations, and control effects like throw/diverge.
//
// Callbacks:
// Specifications for callback parameters, describing how they are invoked
// (cardinality), what they return (boolean for predicates), and whether
// they are pure.
//
// Return specifications:
// Conditional return types based on argument conditions, enabling precise
// return types that depend on runtime values.
//
// Usage:
//
//	spec := contract.NewSpec().
//	    WithRequires(constraint.NotNil{Path: constraint.Path{Root: "p0"}}).
//	    WithEnsures(constraint.NotNil{Path: constraint.Path{Root: "r0"}}).
//	    WithEffects(effect.Throw{})
//
//	fn := typ.Func().Param("x", typ.Any).Returns(typ.Any).Spec(spec).Build()
package contract

import (
	"fmt"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExtractSpec unwraps a function type and returns its contract spec, or nil.
func ExtractSpec(t typ.Type) *Spec {
	fn := unwrap.Function(t)
	if fn == nil || fn.Spec == nil {
		return nil
	}
	spec, ok := fn.Spec.(*Spec)
	if !ok {
		return nil
	}
	return spec
}

// Spec is the complete specification of a function's behavior.
//
// A Spec captures everything the type checker needs to know about a function's
// contract with its callers. This information enables:
//   - Precondition checking at call sites
//   - Type narrowing from postconditions
//   - Effect propagation through call chains
//   - Precise return type derivation
//   - Higher-order function analysis via callback specs
type Spec struct {
	// Requires holds preconditions that must be satisfied at call site.
	// Checked by the type checker before the call; errors if not provable.
	Requires constraint.Condition
	// Ensures holds postconditions that become facts after the call returns.
	// Added to the type environment for code following the call.
	Ensures constraint.Condition
	// ExprRequires holds arithmetic preconditions on parameters.
	// Used for length/index bounds checking.
	ExprRequires []constraint.ExprCompare
	// ExprEnsures holds arithmetic postconditions on return values.
	// Used for return length guarantees.
	ExprEnsures []constraint.ExprCompare
	// Effects describes type-level side effects (mutations, returns).
	// See types/effect for the effect label vocabulary.
	Effects effect.Row
	// Callbacks maps parameter indices to their callback specifications.
	// Key is the 0-based parameter index of the callback.
	Callbacks map[int]*CallbackSpec
	// Return describes conditional return type refinements.
	// Enables different return types based on argument conditions.
	Return *ReturnSpec
}

// NewSpec creates an empty spec.
func NewSpec() *Spec {
	return &Spec{
		Requires:  constraint.TrueCondition(),
		Ensures:   constraint.TrueCondition(),
		Effects:   effect.Empty,
		Callbacks: make(map[int]*CallbackSpec),
	}
}

// WithRequires adds preconditions.
func (s *Spec) WithRequires(constraints ...constraint.Constraint) *Spec {
	if len(constraints) == 0 {
		return s
	}

	s.Requires = constraint.And(s.Requires, constraint.FromConstraints(constraints...))
	return s
}

// WithEnsures adds postconditions.
func (s *Spec) WithEnsures(constraints ...constraint.Constraint) *Spec {
	if len(constraints) == 0 {
		return s
	}

	s.Ensures = constraint.And(s.Ensures, constraint.FromConstraints(constraints...))
	return s
}

// WithExprRequires adds expression preconditions.
func (s *Spec) WithExprRequires(constraints ...constraint.ExprCompare) *Spec {
	s.ExprRequires = append(s.ExprRequires, constraints...)
	return s
}

// WithExprEnsures adds expression postconditions.
func (s *Spec) WithExprEnsures(constraints ...constraint.ExprCompare) *Spec {
	s.ExprEnsures = append(s.ExprEnsures, constraints...)
	return s
}

// WithEffects adds type effects.
func (s *Spec) WithEffects(labels ...effect.Label) *Spec {
	s.Effects = s.Effects.With(labels...)
	return s
}

// WithEffectRow sets the effect row directly.
func (s *Spec) WithEffectRow(row effect.Row) *Spec {
	s.Effects = effect.Union(s.Effects, row)
	return s
}

// WithCallback adds a callback specification.
func (s *Spec) WithCallback(paramIdx int, spec *CallbackSpec) *Spec {
	s.Callbacks[paramIdx] = spec
	return s
}

// ReturnCase describes a conditional return type.
//
// A ReturnCase pairs a condition with a return type. When the condition
// can be proven from the call context, the associated type is used instead
// of the default return type.
type ReturnCase struct {
	// When is the condition under which this return type applies.
	When constraint.Condition
	// Type is the refined return type when the condition holds.
	Type typ.Type
}

// Equals returns true if two ReturnCase values are structurally equal.
func (r ReturnCase) Equals(other ReturnCase) bool {
	if !r.When.Equals(other.When) {
		return false
	}

	if r.Type == nil && other.Type == nil {
		return true
	}

	if r.Type == nil || other.Type == nil {
		return false
	}

	return r.Type.Equals(other.Type)
}

// ReturnSpec describes conditional return types.
//
// ReturnSpec enables overload-like behavior where the return type depends
// on the argument types or values. Cases are checked in order; the first
// matching condition determines the return type.
//
// Example for tonumber(s):
//
//	ReturnSpec{
//	    Cases: []ReturnCase{
//	        {When: HasType{p0, integer}, Type: integer},  // integer input
//	        {When: HasType{p0, number}, Type: number},    // number input
//	    },
//	    Default: typ.NewOptional(typ.Number),  // string input may fail
//	}
type ReturnSpec struct {
	// Cases holds condition-dependent return type refinements.
	Cases []ReturnCase
	// Default is the return type when no case condition matches.
	Default typ.Type
}

// Equals returns true if two ReturnSpec values are structurally equal.
func (r *ReturnSpec) Equals(other *ReturnSpec) bool {
	if r == nil && other == nil {
		return true
	}

	if r == nil || other == nil {
		return false
	}

	if len(r.Cases) != len(other.Cases) {
		return false
	}

	for i := range r.Cases {
		if !r.Cases[i].Equals(other.Cases[i]) {
			return false
		}
	}

	if r.Default == nil && other.Default == nil {
		return true
	}

	if r.Default == nil || other.Default == nil {
		return false
	}

	return r.Default.Equals(other.Default)
}

// WithReturnCase adds a conditional return case.
func (s *Spec) WithReturnCase(cond constraint.Condition, t typ.Type) *Spec {
	if s.Return == nil {
		s.Return = &ReturnSpec{}
	}

	s.Return.Cases = append(s.Return.Cases, ReturnCase{When: cond, Type: t})

	return s
}

// WithDefaultReturn sets the default return when no case matches.
func (s *Spec) WithDefaultReturn(t typ.Type) *Spec {
	if s.Return == nil {
		s.Return = &ReturnSpec{}
	}

	s.Return.Default = t

	return s
}

// GetReturnCases returns conditional return cases.
func (s *Spec) GetReturnCases() []ReturnCase {
	if s == nil || s.Return == nil || len(s.Return.Cases) == 0 {
		return nil
	}

	result := make([]ReturnCase, len(s.Return.Cases))
	copy(result, s.Return.Cases)

	return result
}

// GetReturnDefault returns the default return type if set.
func (s *Spec) GetReturnDefault() typ.Type {
	if s == nil || s.Return == nil {
		return nil
	}

	return s.Return.Default
}

// Equals returns true if two Specs are structurally equal.
// Implements internal.Equaler interface for use in typ.Function.
func (s *Spec) Equals(other any) bool {
	if other == nil {
		return s == nil
	}

	o, ok := other.(*Spec)

	if !ok {
		return false
	}

	if s == nil && o == nil {
		return true
	}

	if s == nil || o == nil {
		return false
	}

	if !s.Requires.Equals(o.Requires) {
		return false
	}

	if !s.Ensures.Equals(o.Ensures) {
		return false
	}

	if !exprComparesEqual(s.ExprRequires, o.ExprRequires) {
		return false
	}

	if !exprComparesEqual(s.ExprEnsures, o.ExprEnsures) {
		return false
	}

	if !s.Effects.Equals(o.Effects) {
		return false
	}

	if !callbacksEqual(s.Callbacks, o.Callbacks) {
		return false
	}

	if !returnSpecEquals(s.Return, o.Return) {
		return false
	}

	return true
}

// IsSpecInfo implements typ.SpecInfo.
func (s *Spec) IsSpecInfo() {}

func exprComparesEqual(a, b []constraint.ExprCompare) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !a[i].Equals(b[i]) {
			return false
		}
	}

	return true
}

func callbacksEqual(a, b map[int]*CallbackSpec) bool {
	if len(a) != len(b) {
		return false
	}

	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}

		if !va.Equals(vb) {
			return false
		}
	}

	return true
}

func returnSpecEquals(a, b *ReturnSpec) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return a.Equals(b)
}

// String returns a string representation.
func (s *Spec) String() string {
	if s == nil {
		return "Spec{}"
	}

	returnCases := 0

	if s.Return != nil {
		returnCases = len(s.Return.Cases)
	}

	return fmt.Sprintf("Spec{requires=%d, ensures=%d, effects=%s, callbacks=%d, returns=%d}",
		len(s.Requires.AllConstraints()), len(s.Ensures.AllConstraints()), s.Effects, len(s.Callbacks), returnCases)
}

// EffectRow returns the effect row.
func (s *Spec) EffectRow() effect.Row {
	if s == nil {
		return effect.Empty
	}

	return s.Effects
}

// CallbackSpec describes a callback parameter's expected behavior.
//
// CallbackSpec enables precise typing of higher-order functions by describing
// how a callback is used:
//
// For table.filter(t, predicate):
//
//	CallbackSpec{
//	    InputSource:    ParamRef{Index: 0},  // callback receives elements from t
//	    ReturnsBoolean: true,                // callback is a predicate
//	    Cardinality:    CardAtMostOncePerElement,
//	    Pure:           true,
//	}
//
// This enables the type checker to:
//   - Infer callback parameter types from InputSource
//   - Verify callback return type matches ReturnsBoolean
//   - Track cardinality for escape analysis
//   - Inject EnvOverlay bindings into callback scope
type CallbackSpec struct {
	// InputSource references the parameter providing input to the callback.
	InputSource effect.ParamRef
	// ReturnsBoolean indicates the callback returns a boolean (predicate).
	ReturnsBoolean bool
	// Cardinality describes how many times the callback is invoked.
	Cardinality Cardinality
	// Pure indicates the callback has no side effects.
	Pure bool
	// EnvOverlay describes callback-scoped globals injected into the callback environment.
	EnvOverlay map[string]typ.Type
}

// Equals returns true if two CallbackSpec values are structurally equal.
func (c *CallbackSpec) Equals(other *CallbackSpec) bool {
	if c == nil && other == nil {
		return true
	}

	if c == nil || other == nil {
		return false
	}

	if c.InputSource.Index != other.InputSource.Index ||
		c.ReturnsBoolean != other.ReturnsBoolean ||
		c.Cardinality != other.Cardinality ||
		c.Pure != other.Pure {
		return false
	}

	return envOverlayEqual(c.EnvOverlay, other.EnvOverlay)
}

// WithEnvOverlay sets callback-scoped globals.
func (c *CallbackSpec) WithEnvOverlay(env map[string]typ.Type) *CallbackSpec {
	c.EnvOverlay = env
	return c
}

// Clone returns a deep copy of the CallbackSpec.
func (c *CallbackSpec) Clone() *CallbackSpec {
	if c == nil {
		return nil
	}

	clone := &CallbackSpec{
		InputSource:    c.InputSource,
		ReturnsBoolean: c.ReturnsBoolean,
		Cardinality:    c.Cardinality,
		Pure:           c.Pure,
	}

	if len(c.EnvOverlay) > 0 {
		clone.EnvOverlay = make(map[string]typ.Type, len(c.EnvOverlay))
		for k, v := range c.EnvOverlay {
			clone.EnvOverlay[k] = v
		}
	}

	return clone
}

func envOverlayEqual(a, b map[string]typ.Type) bool {
	if len(a) != len(b) {
		return false
	}

	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}

		if !va.Equals(vb) {
			return false
		}
	}

	return true
}

// Cardinality describes invocation count for callbacks.
//
// Cardinality is used for escape analysis and optimization. A callback
// that is only called once per element may have different optimization
// opportunities than one called an unknown number of times.
type Cardinality int

const (
	// CardOncePerElement: callback is called exactly once per input element.
	// Used by table.map, table.foreach.
	CardOncePerElement Cardinality = iota
	// CardAtMostOncePerElement: callback may be called 0 or 1 times per element.
	// Used by table.filter (stops on first match in some variants).
	CardAtMostOncePerElement
	// CardExactlyOnce: callback is called exactly once total.
	// Used by pcall, xpcall.
	CardExactlyOnce
	// CardAtMostOnce: callback may be called 0 or 1 times total.
	// Used by table.find (calls predicate until match).
	CardAtMostOnce
	// CardUnknown: invocation count is not known.
	// Conservative default for user-defined higher-order functions.
	CardUnknown
)

func (c Cardinality) String() string {
	switch c {
	case CardOncePerElement:
		return "once_per_element"
	case CardAtMostOncePerElement:
		return "at_most_once_per_element"
	case CardExactlyOnce:
		return "exactly_once"
	case CardAtMostOnce:
		return "at_most_once"
	case CardUnknown:
		return "unknown"
	}

	return "unknown"
}

// PredicateSpec creates a callback spec for predicate functions.
func PredicateSpec(inputParam int) *CallbackSpec {
	return &CallbackSpec{
		InputSource:    effect.ParamRef{Index: inputParam},
		ReturnsBoolean: true,
		Cardinality:    CardAtMostOncePerElement,
		Pure:           true,
	}
}

// MapperSpec creates a callback spec for mapping functions.
func MapperSpec(inputParam int) *CallbackSpec {
	return &CallbackSpec{
		InputSource:    effect.ParamRef{Index: inputParam},
		ReturnsBoolean: false,
		Cardinality:    CardOncePerElement,
		Pure:           true,
	}
}

// HasMutation returns true if the spec mutates any parameter.
func (s *Spec) HasMutation() bool {
	if s == nil {
		return false
	}

	return s.Effects.HasMutate()
}

// GetMutation returns the first mutation effect.
func (s *Spec) GetMutation() *effect.Mutate {
	if s == nil {
		return nil
	}

	return s.Effects.GetMutate(0)
}

// GetMutationAt returns the mutation effect for a specific parameter.
func (s *Spec) GetMutationAt(paramIdx int) *effect.Mutate {
	if s == nil {
		return nil
	}

	return s.Effects.GetMutate(paramIdx)
}

// GetReturnLength returns the return length effect.
func (s *Spec) GetReturnLength(retIdx int) *effect.ReturnLength {
	if s == nil {
		return nil
	}

	return s.Effects.GetReturnLength(retIdx)
}

// GetReturnType returns the return type effect.
func (s *Spec) GetReturnType(retIdx int) *effect.Return {
	if s == nil {
		return nil
	}

	return s.Effects.GetReturn(retIdx)
}

// GetCallback returns the callback spec for a parameter.
func (s *Spec) GetCallback(paramIdx int) *CallbackSpec {
	if s == nil || s.Callbacks == nil {
		return nil
	}

	return s.Callbacks[paramIdx]
}

// IsFilter returns true if this is a filter-like behavior.
func (s *Spec) IsFilter() bool {
	if s == nil {
		return false
	}

	for _, spec := range s.Callbacks {
		if spec.ReturnsBoolean {
			return true
		}
	}

	return false
}

// GetIterator returns the iterator effect.
func (s *Spec) GetIterator() *effect.Iterator {
	if s == nil {
		return nil
	}

	return s.Effects.GetIterator()
}

// GetTableMutator returns the table mutator effect.
func (s *Spec) GetTableMutator() *effect.TableMutator {
	if s == nil {
		return nil
	}

	return s.Effects.GetTableMutator()
}

// IsIndexedIterator returns true if this spec has an indexed iterator effect.
func (s *Spec) IsIndexedIterator() bool {
	if s == nil {
		return false
	}

	return s.Effects.IsIndexedIterator()
}

// IsKeyedIterator returns true if this spec has a keyed iterator effect.
func (s *Spec) IsKeyedIterator() bool {
	if s == nil {
		return false
	}

	return s.Effects.IsKeyedIterator()
}

// IsTableMutator returns true if this spec has a table mutator effect.
func (s *Spec) IsTableMutator() bool {
	if s == nil {
		return false
	}

	return s.Effects.HasTableMutator()
}

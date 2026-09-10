// Package subtype provides subtype checking with seen-map cycle detection.
//
// Subtype checking determines whether a value of type A can be safely used
// where type B is expected. This is fundamental to type safety: assignments,
// function calls, and return statements all require subtype checks.
//
// # Subtype Rules
//
// Basic rules:
//   - T <: T (reflexivity)
//   - Never <: T (bottom is subtype of all)
//   - T <: Any (all are subtypes of top)
//   - T <: Unknown (unknown accepts all for gradual typing)
//   - Integer <: Number (numeric hierarchy)
//
// Composite rules:
//   - Union: (A|B) <: C iff A <: C and B <: C
//   - Optional: T <: T? always; T? <: U iff T <: U and nil <: U
//   - Function: contravariant params, covariant returns
//   - Record: width subtyping (more fields OK), depth subtyping (field types)
//
// # Cycle Detection
//
// Recursive types can form infinite subtype derivations. The checker uses
// interface-identity seen pairs for coinductive cycle detection: if a pair
// (A, B) is encountered again, the check succeeds (coinductive assumption).
//
// # Type Normalization
//
// The package also provides union and intersection normalization:
//   - [NormalizeUnion] removes redundant members via subtype subsumption
//   - [NormalizeIntersection] distributes over unions and detects incompatible primitives
//
// # Type Widening
//
// Widening converts specific types to more general forms:
//   - [Widen] converts literal types to their base types (shallow)
//   - [WidenForInference] recursively widens nested types (deep)
//
// # Variance
//
// Variance utilities for generic type parameter analysis:
//   - [Variance] represents covariant, contravariant, invariant, or bivariant positions
//   - [FlipVariance] inverts variance when entering contravariant positions
//   - [CombineVariance] composes variances through nested type constructors
package subtype

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// isSubtype is the internal implementation of IsSubtype.
func isSubtype(sub, super typ.Type) bool {
	if sub != nil {
		sub = typ.PruneSoftUnionMembers(sub)
	}
	if super != nil {
		super = typ.PruneSoftUnionMembers(super)
	}
	c := &checker{}
	return c.check(sub, super, 0)
}

// IsSubtype reports whether sub is a subtype of super according to structural
// subtyping rules. This is the primary entry point for subtype checking.
//
// The check is pure (no side effects) and non-memoized. For repeated checks
// on the same types, consider caching results externally.
//
// Returns false if either type is nil.
//
// Examples:
//
//	IsSubtype(typ.Integer, typ.Number)     // true: integer <: number
//	IsSubtype(typ.Never, typ.String)       // true: never <: any type
//	IsSubtype(typ.String, typ.Any)         // true: any type <: any
//	IsSubtype(typ.String, typ.Number)      // false: incompatible primitives
func IsSubtype(sub, super typ.Type) bool {
	return isSubtype(sub, super)
}

// checker holds mutable state for a single subtype derivation.
// It tracks seen type pairs to handle recursive types via coinduction.
type checker struct {
	seen map[typePair]bool
}

// check performs the recursive subtype check with depth tracking.
// Depth is bounded by typ.DefaultRecursionDepth to prevent stack overflow
// on pathological recursive types.
func (c *checker) check(sub, super typ.Type, depth int) bool {
	if stopDepthPair(sub, super, depth) {
		return false
	}

	// Reflexivity: T <: T
	if sub == super {
		return true
	}

	// Hash equality fast path
	if sub.Hash() == super.Hash() && sub.Equals(super) {
		return true
	}

	// Local refs match aliases by name.
	if ref, ok := sub.(*typ.Ref); ok && ref.Module == "" {
		if a, ok := super.(*typ.Alias); ok && a.Name == ref.Name {
			return true
		}

		if r, ok := super.(*typ.Ref); ok && r.Module == "" && r.Name == ref.Name {
			return true
		}
	}

	if ref, ok := super.(*typ.Ref); ok && ref.Module == "" {
		if a, ok := sub.(*typ.Alias); ok && a.Name == ref.Name {
			return true
		}

		if r, ok := sub.(*typ.Ref); ok && r.Module == "" && r.Name == ref.Name {
			return true
		}
	}

	// Cycle detection using interface identity (non-commutative, co-inductive).
	if needsCycleGuard(sub.Kind()) && needsCycleGuard(super.Kind()) {
		pair := typePair{sub: sub, super: super}
		if c.seen == nil {
			c.seen = make(map[typePair]bool)
		}
		if c.seen[pair] {
			return true // coinductive assumption
		}
		c.seen[pair] = true
	}

	// Unwrap aliases
	if aa, ok := sub.(*typ.Alias); ok {
		return c.check(aa.UnaliasedTarget(), super, depth+1)
	}

	if aa, ok := super.(*typ.Alias); ok {
		return c.check(sub, aa.UnaliasedTarget(), depth+1)
	}

	if rr, ok := sub.(*typ.Recursive); ok && super.Kind() != kind.Recursive && rr.Body != nil && rr.Body != rr {
		return c.check(rr.Body, super, depth+1)
	}

	if rr, ok := super.(*typ.Recursive); ok && sub.Kind() != kind.Recursive && rr.Body != nil && rr.Body != rr {
		return c.check(sub, rr.Body, depth+1)
	}

	// Handle instantiated generics
	subInst, subIsInst := sub.(*typ.Instantiated)
	superInst, superIsInst := super.(*typ.Instantiated)

	// When both are Instantiated with the same generic, compare type args directly
	if subIsInst && superIsInst && subInst.Generic != nil && superInst.Generic != nil {
		if subInst.Generic.Equals(superInst.Generic) {
			return c.checkInstantiated(subInst, superInst, depth)
		}
	}

	// Expand instantiated to body for cross-kind comparisons.
	// Only recurse if expansion actually changes the type to avoid
	// coinductive cycles when the generic body is unavailable.
	if subIsInst {
		expanded := subst.ExpandInstantiated(subInst)
		if expanded != nil && expanded != sub {
			return c.check(expanded, super, depth+1)
		}
	}

	if superIsInst {
		expanded := subst.ExpandInstantiated(superInst)
		if expanded != nil && expanded != super {
			return c.check(sub, expanded, depth+1)
		}
	}

	// Never <: T (bottom type)
	if typ.IsNever(sub) {
		return true
	}
	// T </: Never (except Never itself, handled above)
	if typ.IsNever(super) {
		return false
	}

	// T <: Any (top type)
	if typ.IsAny(super) {
		return true
	}
	// Unknown acts as a top type for unresolved values.
	// T <: Unknown is true for all T, including Any.
	if typ.IsUnknown(super) {
		return true
	}
	// Any is NOT assignable to specific types; only to Any itself (Unknown handled above).
	if typ.IsAny(sub) {
		// Builtin table-top marker is a dynamic table boundary; explicit `any`
		// values are permitted to flow through it.
		if unwrap.IsBuiltinTableTop(super) {
			return true
		}
		return false
	}

	// Unknown acts as a top type for unresolved values, but not bottom.
	// Unknown <: T is false (except T = Any/Unknown handled above), while T <: Unknown is true.
	if typ.IsUnknown(sub) {
		return false
	}

	// Sub union: all members must be subtypes
	if u, ok := sub.(*typ.Union); ok {
		for _, m := range u.Members {
			if !c.check(m, super, depth+1) {
				return false
			}
		}

		return true
	}

	// Super union: sub must be subtype of some member
	if u, ok := super.(*typ.Union); ok {
		// Optional(T) is equivalent to (T | nil). It is a subtype of a union
		// only if both T and nil are subtypes of that union.
		if o, ok := sub.(*typ.Optional); ok {
			return c.check(o.Inner, super, depth+1) && c.checkNil(super, depth+1)
		}
		for _, m := range u.Members {
			if c.check(sub, m, depth+1) {
				return true
			}
		}

		return false
	}

	// Sub intersection: some member must be subtype
	if i, ok := sub.(*typ.Intersection); ok {
		for _, m := range i.Members {
			if c.check(m, super, depth+1) {
				return true
			}
		}

		return false
	}

	// Super intersection: sub must be subtype of all members
	if i, ok := super.(*typ.Intersection); ok {
		for _, m := range i.Members {
			if !c.check(sub, m, depth+1) {
				return false
			}
		}

		return true
	}

	// Optional rules
	if o, ok := super.(*typ.Optional); ok {
		if subOpt, ok := sub.(*typ.Optional); ok {
			return c.check(subOpt.Inner, o.Inner, depth+1)
		}

		if sub.Kind() == kind.Nil {
			return true
		}
		// T <: T?
		return c.check(sub, o.Inner, depth+1)
	}

	if o, ok := sub.(*typ.Optional); ok {
		// T? <: U only if T <: U and nil <: U
		return c.checkNil(super, depth+1) && c.check(o.Inner, super, depth+1)
	}

	// Builtin `table` annotation is modeled as a marker interface.
	// Accept only Lua table-like structural shapes.
	if unwrap.IsBuiltinTableTop(super) {
		return isTableLikeType(sub)
	}

	// Empty record can satisfy array/map shapes, but should still flow through
	// regular record subtyping for record supers (e.g. all-optional records).
	if r, ok := sub.(*typ.Record); ok && len(r.Fields) == 0 {
		if super.Kind() == kind.Array || super.Kind() == kind.Map {
			return true
		}
	}

	if r, ok := sub.(*typ.Record); ok {
		if m, ok := super.(*typ.Map); ok {
			return c.checkRecordToMap(r, m, depth+1)
		}
	}

	// Array <: Map with integer key
	if arr, ok := sub.(*typ.Array); ok {
		if m, ok := super.(*typ.Map); ok {
			return c.checkArrayToMap(arr, m, depth+1)
		}
	}

	// Tuple <: Array when all elements are subtypes
	if tup, ok := sub.(*typ.Tuple); ok {
		if arr, ok := super.(*typ.Array); ok {
			return c.checkTupleToArray(tup, arr, depth+1)
		}
		if m, ok := super.(*typ.Map); ok {
			return c.checkTupleToMap(tup, m, depth+1)
		}
	}

	// Record <: Interface (structural subtyping)
	if rec, ok := sub.(*typ.Record); ok {
		if iface, ok := super.(*typ.Interface); ok {
			return c.checkRecordToInterface(rec, iface, depth+1)
		}
	}

	// TypeParam <: Constraint
	if tp, ok := sub.(*typ.TypeParam); ok {
		if sp, ok := super.(*typ.TypeParam); ok {
			return tp.Equals(sp)
		}

		if tp.Constraint != nil {
			return c.check(tp.Constraint, super, depth+1)
		}

		return typ.IsAny(super)
	}
	// Type <: TypeParam (parameter position)
	if tp, ok := super.(*typ.TypeParam); ok {
		if tp.Constraint != nil {
			return c.check(sub, tp.Constraint, depth+1)
		}

		return true
	}

	// Literal <: Base
	if l, ok := sub.(*typ.Literal); ok {
		switch l.Base {
		case kind.Boolean:
			return super.Kind() == kind.Boolean
		case kind.Integer:
			return super.Kind() == kind.Integer || super.Kind() == kind.Number
		case kind.Number:
			return super.Kind() == kind.Number
		case kind.String:
			return super.Kind() == kind.String
		}
	}

	// integer <: number
	if sub.Kind() == kind.Integer && super.Kind() == kind.Number {
		return true
	}

	// Same kind dispatch
	if sub.Kind() != super.Kind() {
		return false
	}

	switch sub.Kind() {
	case kind.Function:
		return c.checkFunction(sub.(*typ.Function), super.(*typ.Function), depth)
	case kind.Record:
		return c.checkRecord(sub.(*typ.Record), super.(*typ.Record), depth)
	case kind.Array:
		return c.checkArray(sub.(*typ.Array), super.(*typ.Array), depth)
	case kind.Map:
		return c.checkMap(sub.(*typ.Map), super.(*typ.Map), depth)
	case kind.Tuple:
		return c.checkTuple(sub.(*typ.Tuple), super.(*typ.Tuple), depth)
	case kind.Interface:
		return c.checkInterface(sub.(*typ.Interface), super.(*typ.Interface), depth)
	case kind.Instantiated:
		return c.checkInstantiated(sub.(*typ.Instantiated), super.(*typ.Instantiated), depth)
	case kind.Meta:
		return c.check(sub.(*typ.Meta).Of, super.(*typ.Meta).Of, depth+1)
	default:
		return sub.Equals(super)
	}
}

// checkNil reports whether nil is a subtype of super.
// Used when checking optional types: T? <: U requires nil <: U.
func (c *checker) checkNil(super typ.Type, depth int) bool {
	return c.check(typ.Nil, super, depth)
}

// checkFunction implements function subtyping with standard variance rules.
//
// For f_sub <: f_super:
//   - Parameters are contravariant: each super param type must be a subtype of
//     the corresponding sub param type (sub accepts wider input)
//   - Returns are covariant: each sub return type must be a subtype of the
//     corresponding super return type (sub provides narrower output)
//   - Arity: sub must accept at least as many arguments as super requires,
//     and sub must return at least as many values as super promises
//   - Variadic: checked contravariantly like regular parameters
func (c *checker) checkFunction(sub, super *typ.Function, depth int) bool {
	subReq := typ.MinRequiredArgs(sub)
	superReq := typ.MinRequiredArgs(super)

	// sub must accept at least as many args as super requires
	// If sub requires more params than super accepts, sub is more restrictive = not subtype
	if subReq > superReq || (super.Variadic == nil && subReq > len(super.Params)) {
		// sub requires more than super can provide
		return false
	}

	// If super can call with more args than sub accepts, sub is more restrictive
	if sub.Variadic == nil && len(super.Params) > len(sub.Params) {
		return false
	}

	// Check param types (contravariant)
	maxParams := len(sub.Params)
	if len(super.Params) > maxParams {
		maxParams = len(super.Params)
	}

	for i := 0; i < maxParams; i++ {
		var subT, superT typ.Type

		if i < len(sub.Params) {
			subT = sub.Params[i].Type
		} else if sub.Variadic != nil {
			subT = sub.Variadic
		}

		if i < len(super.Params) {
			superT = super.Params[i].Type
		} else if super.Variadic != nil {
			superT = super.Variadic
		}

		if subT == nil || superT == nil {
			continue
		}

		// Contravariant: super param <: sub param
		if !c.check(superT, subT, depth+1) {
			return false
		}
	}

	// Check variadic compatibility
	if sub.Variadic != nil && super.Variadic != nil {
		if !c.check(super.Variadic, sub.Variadic, depth+1) {
			return false
		}
	}

	// Check returns: covariant
	// sub must provide at least as many returns as super promises
	if len(sub.Returns) < len(super.Returns) {
		return false
	}

	for i := 0; i < len(super.Returns); i++ {
		if !c.check(sub.Returns[i], super.Returns[i], depth+1) {
			return false
		}
	}

	return true
}

// checkRecord implements structural record subtyping.
//
// Width subtyping: sub may have additional fields not present in super.
// Depth subtyping: for each field in super, sub must have a compatible field.
//
// Field variance depends on mutability:
//   - Readonly fields: covariant (sub field type <: super field type)
//   - Mutable fields: quasi-invariant with widening allowance
//
// The widening allowance permits assignments like {x: 1} to {x: number} for
// fresh record literals, while preventing unsound aliasing scenarios.
//
// Optional field rules:
//   - Required in super, optional in sub: not a subtype
//   - Optional in super: sub may have required or optional
//   - Missing field in sub: allowed only if super field is optional or accepts nil
func (c *checker) checkRecord(sub, super *typ.Record, depth int) bool {
	// For each field in super, sub must have compatible field
	for _, sf := range super.Fields {
		subField := sub.GetField(sf.Name)
		if subField == nil {
			// Allow missing field if field is optional or type accepts nil
			if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) {
				return false // required field missing and type doesn't accept nil
			}

			continue
		}

		if sf.Readonly {
			// Readonly in super: covariant check is sound (no writes through supertype)
			if !c.check(subField.Type, sf.Type, depth+1) {
				return false
			}
		} else {
			// Mutable in super: sub field must also be mutable
			if subField.Readonly {
				return false
			}

			// Forward check: sub field type must be subtype of super field type
			if !c.check(subField.Type, sf.Type, depth+1) {
				return false
			}
			// Reverse check with widening: allow literal/refinement types to widen
			// This is sound for fresh record literals where no narrower-typed alias exists
			if !c.check(sf.Type, subField.Type, depth+1) && !canWidenTo(subField.Type, sf.Type) {
				return false
			}
		}

		// Optional compatibility:
		// A super field that syntactically looks required can still admit nil
		// via its type (e.g. `x: string?`). In that case an optional sub field
		// remains compatible.
		if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) && subField.Optional {
			return false
		}
	}

	// Compare map components
	if super.HasMapComponent() {
		if !sub.HasMapComponent() {
			return false
		}
		if !c.check(sub.MapKey, super.MapKey, depth+1) {
			return false
		}
		if !c.check(sub.MapValue, super.MapValue, depth+1) {
			return false
		}
	}

	return true
}

// canWidenTo reports whether narrow can safely widen to wide in a mutable context.
//
// This enables practical assignments where a literal or specific type flows to
// a location expecting a more general type:
//
//   - nil -> optional types or unions containing nil
//   - integer -> number
//   - literal types -> their base types (e.g., "hello" -> string)
//   - nested records -> recursively check field widening
//   - functions -> widen return types while requiring equivalent params
//
// This is sound because it only applies to fresh values where no narrower-typed
// alias can exist to observe the widening.
func canWidenTo(narrow, wide typ.Type) bool {
	// Unwrap aliases to get the underlying types
	wide = unwrap.Alias(wide)
	narrow = unwrap.Alias(narrow)

	// Any type accepts everything
	if typ.IsAny(wide) {
		return true
	}

	// Nil can widen to optional types
	if narrow.Kind() == kind.Nil {
		if _, ok := wide.(*typ.Optional); ok {
			return true
		}
		// Nil can also widen to unions containing nil
		if u, ok := wide.(*typ.Union); ok {
			for _, m := range u.Members {
				if m.Kind() == kind.Nil {
					return true
				}
			}
		}
	}

	// Allow widening into optional types when narrow fits the inner type.
	if opt, ok := wide.(*typ.Optional); ok {
		if isSubtype(narrow, opt.Inner) {
			return true
		}
	}

	// Allow widening into unions when narrow fits at least one member.
	if u, ok := wide.(*typ.Union); ok {
		for _, m := range u.Members {
			// Keep literal-tag unions invariant for mutable fields; only allow
			// widening through non-literal branch members (for example number|string).
			if m.Kind() == kind.Literal {
				continue
			}
			if isSubtype(narrow, m) || canWidenTo(narrow, m) {
				return true
			}
		}
		return false
	}

	// Literal unions can widen to a primitive supertype when each branch widens.
	// Example: 0|8000 can widen to integer for mutable record fields.
	if u, ok := narrow.(*typ.Union); ok {
		if len(u.Members) == 0 {
			return false
		}
		for _, m := range u.Members {
			if isSubtype(m, wide) || canWidenTo(m, wide) {
				continue
			}
			return false
		}
		return true
	}

	// Integer can widen to number
	if narrow.Kind() == kind.Integer && wide.Kind() == kind.Number {
		return true
	}

	// Literal types can widen to their base types or optional base types
	if lit, ok := narrow.(*typ.Literal); ok {
		// Check if wide is optional, unwrap if so
		wideInner := wide
		if opt, ok := wide.(*typ.Optional); ok {
			wideInner = unwrap.Alias(opt.Inner)
		}
		switch lit.Base {
		case kind.Boolean:
			return wideInner.Kind() == kind.Boolean
		case kind.String:
			return wideInner.Kind() == kind.String
		case kind.Integer:
			return wideInner.Kind() == kind.Integer || wideInner.Kind() == kind.Number
		case kind.Number:
			return wideInner.Kind() == kind.Number
		}
	}

	// Nested records: check if all fields can widen
	if subRec, ok := narrow.(*typ.Record); ok {
		if supRec, ok := wide.(*typ.Record); ok {
			return canWidenRecordTo(subRec, supRec)
		}
	}

	// Tuples: allow element-wise widening for fresh tuple literals.
	if subTuple, ok := narrow.(*typ.Tuple); ok {
		if supTuple, ok := wide.(*typ.Tuple); ok {
			if len(subTuple.Elements) != len(supTuple.Elements) {
				return false
			}
			for i := range subTuple.Elements {
				if isSubtype(subTuple.Elements[i], supTuple.Elements[i]) ||
					canWidenTo(subTuple.Elements[i], supTuple.Elements[i]) {
					continue
				}
				return false
			}
			return true
		}
	}

	// Functions: allow widening when params are equivalent and returns can widen.
	if subFn, ok := narrow.(*typ.Function); ok {
		if supFn, ok := wide.(*typ.Function); ok {
			if !functionParamsEquivalent(subFn, supFn) {
				return false
			}
			if len(subFn.Returns) < len(supFn.Returns) {
				return false
			}
			for i := 0; i < len(supFn.Returns); i++ {
				if isSubtype(subFn.Returns[i], supFn.Returns[i]) || canWidenTo(subFn.Returns[i], supFn.Returns[i]) {
					continue
				}
				return false
			}
			return true
		}
	}

	return false
}

// functionParamsEquivalent reports whether two functions have equivalent parameter
// signatures. Used by canWidenTo to allow function widening only when parameters
// match exactly (no contravariance in widening context).
func functionParamsEquivalent(a, b *typ.Function) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a.Params) != len(b.Params) {
		return false
	}
	for i := 0; i < len(a.Params); i++ {
		ap := a.Params[i]
		bp := b.Params[i]
		if ap.Optional != bp.Optional {
			return false
		}
		if !isSubtype(ap.Type, bp.Type) || !isSubtype(bp.Type, ap.Type) {
			return false
		}
	}
	if a.Variadic == nil && b.Variadic == nil {
		return true
	}
	if a.Variadic == nil || b.Variadic == nil {
		return false
	}
	return isSubtype(a.Variadic, b.Variadic) && isSubtype(b.Variadic, a.Variadic)
}

// canWidenRecordTo reports whether all fields in narrow can widen to their
// corresponding fields in wide. This is the recursive helper for canWidenTo
// when both types are records.
func canWidenRecordTo(narrow, wide *typ.Record) bool {
	for _, wf := range wide.Fields {
		nf := narrow.GetField(wf.Name)
		if nf == nil {
			continue
		}
		// Forward direction must hold (already checked by main subtype check)
		// Check if reverse direction can be satisfied by widening
		if !isSubtype(wf.Type, nf.Type) && !canWidenTo(nf.Type, wf.Type) {
			return false
		}
	}
	return true
}

// checkArray implements array subtyping with covariant elements.
//
// Array<A> <: Array<B> iff A <: B
//
// Arrays are covariant because Lua tables are dynamically typed and we optimize
// for practical usability over strict soundness. For sound mutable array handling,
// use invariant map types explicitly.
func (c *checker) checkArray(sub, super *typ.Array, depth int) bool {
	return c.check(sub.Element, super.Element, depth+1)
}

// checkMap implements map subtyping with invariant key and value types.
//
// Map<K1, V1> <: Map<K2, V2> iff K1 = K2 and V1 = V2 (bidirectional subtype)
//
// Maps are invariant because they are mutable: a write through the supertype
// could violate the subtype's constraints, and a read through the supertype
// could return an unexpected type.
func (c *checker) checkMap(sub, super *typ.Map, depth int) bool {
	// Keys must be equal (invariant)
	if !c.check(sub.Key, super.Key, depth+1) || !c.check(super.Key, sub.Key, depth+1) {
		return false
	}
	// Values must be equal (invariant)
	return c.check(sub.Value, super.Value, depth+1) &&
		c.check(super.Value, sub.Value, depth+1)
}

// checkTuple implements tuple subtyping with covariant elements.
//
// Tuple<A1, A2, ...> <: Tuple<B1, B2, ...> iff length matches and Ai <: Bi for all i
//
// Tuples are covariant for practical gradual typing. Strict soundness for mutable
// tuples would require invariance, but this is rarely needed in practice.
func (c *checker) checkTuple(sub, super *typ.Tuple, depth int) bool {
	if len(sub.Elements) != len(super.Elements) {
		return false
	}

	for i, e := range sub.Elements {
		if !c.check(e, super.Elements[i], depth+1) {
			return false
		}
	}

	return true
}

// checkInterface implements structural interface subtyping.
//
// Interface A <: Interface B iff for every method M in B, A has a method M
// with a compatible (subtype) signature.
//
// Special case: marker interfaces (interfaces with no methods) use nominal
// equality by name. This prevents all empty interfaces from being subtypes
// of each other, which is important for channel types like Channel<T>.
func (c *checker) checkInterface(sub, super *typ.Interface, depth int) bool {
	if sub == nil || super == nil {
		return false
	}

	// Marker interfaces (no methods) require name equality to prevent
	// all empty interfaces from being subtypes of each other.
	// This gives nominal semantics to channel types like Channel<T>.
	if len(super.Methods) == 0 && len(sub.Methods) == 0 {
		return sub.Name == super.Name
	}

	// For each method in super, sub must have a compatible method
	for _, superMethod := range super.Methods {
		found := false
		for _, subMethod := range sub.Methods {
			if subMethod.Name == superMethod.Name {
				// Method types must be compatible (function subtyping)
				if !c.check(subMethod.Type, superMethod.Type, depth+1) {
					return false
				}
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// checkRecordToMap checks if a record is a subtype of a map.
//
// A record {f1: T1, f2: T2, ...} <: Map<K, V> iff:
//   - Each field name (as a literal string) is a subtype of K
//   - Each field type Ti is a subtype of V
//   - If the record has a map component, its key/value types are also checked
//
// This enables records to flow to map-typed parameters when appropriate.
func (c *checker) checkRecordToMap(sub *typ.Record, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}

	for _, f := range sub.Fields {
		keyType := typ.LiteralString(f.Name)
		if !c.check(keyType, super.Key, depth+1) {
			return false
		}

		if !c.check(f.Type, super.Value, depth+1) {
			return false
		}
	}

	// If record has a map component, check its key/value against the super map
	if sub.HasMapComponent() {
		if !c.check(sub.MapKey, super.Key, depth+1) {
			return false
		}
		if !c.check(sub.MapValue, super.Value, depth+1) {
			return false
		}
	}

	return true
}

// checkArrayToMap checks if an array is a subtype of a map.
//
// Array<E> <: Map<K, V> iff:
//   - integer <: K (arrays have integer keys)
//   - E <: V (element type flows to value type)
//
// This allows arrays to flow to map-typed parameters expecting integer keys.
func (c *checker) checkArrayToMap(sub *typ.Array, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}

	// Array is a map with integer keys
	if !c.check(typ.Integer, super.Key, depth+1) {
		return false
	}

	return c.check(sub.Element, super.Value, depth+1)
}

// checkTupleToArray checks if a tuple is a subtype of an array.
//
// Tuple<E1, E2, ...> <: Array<A> iff Ei <: A for all elements.
//
// This allows tuples to flow to array-typed parameters when all elements
// are compatible with the array's element type.
func (c *checker) checkTupleToArray(sub *typ.Tuple, super *typ.Array, depth int) bool {
	if sub == nil || super == nil {
		return false
	}

	for _, elem := range sub.Elements {
		if !c.check(elem, super.Element, depth+1) {
			return false
		}
	}

	return true
}

// checkTupleToMap checks if a tuple is a subtype of a map.
//
// Tuple<E1, E2, ...> <: Map<K, V> iff:
//   - integer <: K (tuples have integer indices)
//   - Ei <: V for all elements
//
// This allows tuples to flow to map-typed parameters expecting integer keys.
func (c *checker) checkTupleToMap(sub *typ.Tuple, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}

	// Tuple can be viewed as map with integer keys
	if !c.check(typ.Integer, super.Key, depth+1) {
		return false
	}

	for _, elem := range sub.Elements {
		if !c.check(elem, super.Value, depth+1) {
			return false
		}
	}

	return true
}

// checkRecordToInterface checks if a record implements an interface.
//
// Record <: Interface iff for each method M in the interface, the record has
// a field with name M and a type that is a subtype of M's type.
//
// Self type in method signatures is substituted with the record type to enable
// proper structural matching of methods that reference the receiver type.
func (c *checker) checkRecordToInterface(sub *typ.Record, super *typ.Interface, depth int) bool {
	if sub == nil || super == nil {
		return false
	}

	for _, method := range super.Methods {
		field := sub.GetField(method.Name)
		if field == nil {
			return false
		}

		// The field type must be a subtype of the method type.
		// For Self type in method signature, we substitute with the record.
		methodType := subst.Self(method.Type, sub)
		if !c.check(field.Type, methodType, depth+1) {
			return false
		}
	}

	return true
}

// checkInstantiated implements subtyping for instantiated generic types.
//
// Instantiated<G, A1, A2, ...> <: Instantiated<G, B1, B2, ...> iff:
//   - Both share the same generic definition G
//   - Type arguments are invariant: Ai = Bi (bidirectional subtype) for all i
//
// Type arguments are invariant by default because generic types may be used
// in both covariant and contravariant positions internally.
func (c *checker) checkInstantiated(sub, super *typ.Instantiated, depth int) bool {
	if !sub.Generic.Equals(super.Generic) {
		return false
	}

	if len(sub.TypeArgs) != len(super.TypeArgs) {
		return false
	}

	for i, a := range sub.TypeArgs {
		// Type arguments are invariant by default
		if !c.check(a, super.TypeArgs[i], depth+1) || !c.check(super.TypeArgs[i], a, depth+1) {
			return false
		}
	}

	return true
}

func isTableLikeType(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return isTableLikeType(v.UnaliasedTarget())
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && isTableLikeType(v.Body)
	case *typ.Record, *typ.Map, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Intersection:
		return true
	default:
		return false
	}
}

// typePair stores a non-commutative pair of types for cycle detection.
// Interface identity (pointer equality for compound types) serves as key,
// matching the seen-map pattern in typ/equals.go.
type typePair struct {
	sub   typ.Type
	super typ.Type
}

// needsCycleGuard returns true for types that are pointer-backed and could
// participate in recursive cycles. Primitive singletons use value semantics
// and cannot form cycles.
func needsCycleGuard(k kind.Kind) bool {
	switch k {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Self:
		return false
	default:
		return true
	}
}

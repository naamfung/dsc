package subtype

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// NormalizeUnion creates a normalized union type with subtype-based simplification.
//
// Normalization rules applied in order:
//  1. Flatten nested unions: (A | B) | C becomes A | B | C
//  2. Expand optionals: T? becomes T | nil
//  3. Absorb by any: A | any = any
//  4. Remove never: A | never = A
//  5. Remove subsumed types: if A <: B, then A | B simplifies to B
//
// The result is a minimal union where no member is a subtype of another.
// Returns Never for empty input, the single type for single input.
//
// Examples:
//
//	NormalizeUnion(string, never)        // string
//	NormalizeUnion(integer, number)      // number (integer subsumed)
//	NormalizeUnion(string, any, number)  // any (absorbs all)
func NormalizeUnion(types ...typ.Type) typ.Type {
	if len(types) == 0 {
		return typ.Never
	}

	flat := flattenUnion(nil, types)

	// Any absorbs all
	for _, t := range flat {
		if typ.IsAny(t) {
			return typ.Any
		}
	}

	// Remove Never and subsumed types
	result := make([]typ.Type, 0, len(flat))

	for _, t := range flat {
		if typ.IsNever(t) {
			continue
		}

		// Check if t is subsumed by any existing member
		subsumed := false

		for _, existing := range result {
			if t.Equals(existing) || IsSubtype(t, existing) {
				subsumed = true
				break
			}
		}

		if subsumed {
			continue
		}

		// Remove existing members that are subsumed by t
		writeIdx := 0

		for _, existing := range result {
			if !IsSubtype(existing, t) {
				result[writeIdx] = existing
				writeIdx++
			}
		}

		result = result[:writeIdx]
		result = append(result, t)
	}

	if len(result) == 0 {
		return typ.Never
	}

	if len(result) == 1 {
		return result[0]
	}

	return typ.NewUnion(result...)
}

// NormalizeIntersection creates a normalized intersection type with simplification.
//
// Normalization rules:
//  1. Flatten nested intersections: (A & B) & C becomes A & B & C
//  2. Never absorbs all: A & never = never
//  3. Remove any/unknown: A & any = A, A & unknown = A
//  4. Detect incompatible primitives: string & number = never
//  5. Distribute over unions: (A | B) & C = (A & C) | (B & C)
//
// Distribution is bounded by internal.MaxDistributionProduct to prevent
// exponential blowup when intersecting large unions.
//
// Returns Any for empty input, the single type for single input.
//
// Examples:
//
//	NormalizeIntersection(string, any)     // string
//	NormalizeIntersection(string, number)  // never (incompatible)
//	NormalizeIntersection(string, never)   // never (absorbs)
func NormalizeIntersection(types ...typ.Type) typ.Type {
	return normalizeIntersectionDepth(types, 0)
}

// normalizeIntersectionDepth is the recursive implementation with depth tracking
// to prevent infinite recursion on pathological type structures.
func normalizeIntersectionDepth(types []typ.Type, depth int) typ.Type {
	if len(types) == 0 {
		return typ.Any
	}

	// Use the canonical typ recursion budget to avoid behavior cliffs between
	// shallow normalization and other type operations (which use typ.NewGuard()).
	if depth > typ.DefaultRecursionDepth {
		return typ.NewIntersection(types...)
	}

	flat := flattenIntersection(nil, types)

	// Never absorbs all
	for _, t := range flat {
		if typ.IsNever(t) {
			return typ.Never
		}
	}

	// Separate unions from non-unions
	var unions []*typ.Union

	nonUnions := make([]typ.Type, 0, len(flat))

	for _, t := range flat {
		if t.Kind().IsPlaceholder() {
			continue
		}

		if u, ok := t.(*typ.Union); ok {
			unions = append(unions, u)
		} else if !containsEquivalent(nonUnions, t) {
			nonUnions = append(nonUnions, t)
		}
	}

	if len(unions) == 0 && len(nonUnions) == 0 {
		return typ.Any
	}

	if len(unions) == 0 {
		if len(nonUnions) == 1 {
			return nonUnions[0]
		}

		if incompatiblePrimitives(nonUnions) {
			return typ.Never
		}

		return typ.NewIntersection(nonUnions...)
	}

	// Estimate cartesian product size before distributing
	productSize := 1
	for _, u := range unions {
		productSize *= len(u.Members)
		if productSize > internal.MaxDistributionProduct {
			all := make([]typ.Type, 0, len(nonUnions)+len(unions))
			all = append(all, nonUnions...)
			for _, u := range unions {
				all = append(all, u)
			}
			return typ.NewIntersection(all...)
		}
	}

	// Distribute intersection over unions
	distributed := make([][]typ.Type, len(unions[0].Members))
	for i, member := range unions[0].Members {
		distributed[i] = append(append([]typ.Type{}, nonUnions...), member)
	}

	for _, union := range unions[1:] {
		newDistributed := make([][]typ.Type, 0, len(distributed)*len(union.Members))

		for _, existing := range distributed {
			for _, member := range union.Members {
				combo := make([]typ.Type, len(existing)+1)
				copy(combo, existing)
				combo[len(existing)] = member
				newDistributed = append(newDistributed, combo)
			}
		}

		distributed = newDistributed
	}

	// Normalize each combination
	unionMembers := make([]typ.Type, 0, len(distributed))

	for _, combo := range distributed {
		normalized := normalizeIntersectionDepth(combo, depth+1)
		if !typ.IsNever(normalized) {
			unionMembers = append(unionMembers, normalized)
		}
	}

	if len(unionMembers) == 0 {
		return typ.Never
	}

	if len(unionMembers) == 1 {
		return unionMembers[0]
	}

	return NormalizeUnion(unionMembers...)
}

// flattenUnion recursively flattens nested unions and expands optional types
// into their union representation (T? becomes T | nil).
func flattenUnion(acc []typ.Type, types []typ.Type) []typ.Type {
	for _, t := range types {
		if t == nil {
			continue
		}

		unwrapped := typ.UnwrapAnnotated(t)

		switch unwrapped.Kind() {
		case kind.Union:
			acc = flattenUnion(acc, unwrapped.(*typ.Union).Members)
		case kind.Optional:
			acc = append(acc, typ.Nil)
			acc = flattenUnion(acc, []typ.Type{unwrapped.(*typ.Optional).Inner})
		default:
			acc = append(acc, t)
		}
	}

	return acc
}

// flattenIntersection recursively flattens nested intersection types into
// a flat list of members.
func flattenIntersection(acc []typ.Type, types []typ.Type) []typ.Type {
	for _, t := range types {
		if t == nil {
			continue
		}

		if i, ok := typ.UnwrapAnnotated(t).(*typ.Intersection); ok {
			acc = flattenIntersection(acc, i.Members)
		} else {
			acc = append(acc, t)
		}
	}

	return acc
}

// containsEquivalent reports whether list contains a type structurally equal to t.
func containsEquivalent(list []typ.Type, t typ.Type) bool {
	for _, existing := range list {
		if typ.TypeEquals(t, existing) {
			return true
		}
	}

	return false
}

// incompatiblePrimitives reports whether the list contains two or more types
// from different primitive categories that cannot inhabit the same value.
//
// Categories: boolean, numeric (number/integer), string, nil, function, table
//
// For example, string and number are incompatible (no value is both), but
// integer and number are compatible (integer is a subset of number).
func incompatiblePrimitives(list []typ.Type) bool {
	var hasBool, hasNumber, hasInteger, hasString, hasNil, hasFunction, hasTable bool

	for _, t := range list {
		switch t.Kind() {
		case kind.Boolean:
			hasBool = true
		case kind.Number:
			hasNumber = true
		case kind.Integer:
			hasInteger = true
		case kind.String:
			hasString = true
		case kind.Nil:
			hasNil = true
		case kind.Function:
			hasFunction = true
		case kind.Record, kind.Map, kind.Array, kind.Tuple, kind.Interface:
			hasTable = true
		}
	}

	count := 0
	if hasBool {
		count++
	}

	if hasNumber || hasInteger {
		count++
	}

	if hasString {
		count++
	}

	if hasNil {
		count++
	}

	if hasFunction {
		count++
	}

	if hasTable {
		count++
	}

	return count > 1
}

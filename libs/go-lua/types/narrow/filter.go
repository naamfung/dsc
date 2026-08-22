package narrow

import (
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TypeMatcher is a predicate function that determines whether a type satisfies
// a specific criterion for narrowing purposes.
//
// Type matchers enable flexible narrowing by abstracting the matching logic.
// They are used with [FilterByMatch] to selectively keep or exclude union
// members based on arbitrary type properties such as:
//   - Kind matching (type(x) == "number")
//   - Discriminant checks (x.kind == "a")
//   - Structural properties (has certain fields)
//
// The matcher receives individual union members (not the union itself) and
// returns true if the member should be kept (for narrowing) or removed
// (for exclusion).
//
// # Example
//
//	matcher := func(t typ.Type) bool {
//	    return t.Kind() == kind.Number
//	}
//	result := FilterByMatch(union, matcher, false)  // keep numbers
//	result := FilterByMatch(union, matcher, true)   // exclude numbers
type TypeMatcher func(t typ.Type) bool

// ByFieldTruthy narrows a type to members where a field can be truthy.
func ByFieldTruthy(t typ.Type, field string, resolver Resolver) typ.Type {
	if t == nil || field == "" || resolver == nil {
		return t
	}
	return FilterByMatch(t, func(m typ.Type) bool {
		return fieldCanBeTruthy(m, field, resolver)
	}, false)
}

// ByFieldFalsy narrows a type to members where a field can be falsy.
func ByFieldFalsy(t typ.Type, field string, resolver Resolver) typ.Type {
	if t == nil || field == "" || resolver == nil {
		return t
	}
	return FilterByMatch(t, func(m typ.Type) bool {
		return fieldCanBeFalsy(m, field, resolver)
	}, false)
}

// FilterByMatch filters a type by applying a matcher predicate with configurable
// polarity (narrow vs exclude).
//
// This is the core filtering function used by field-based and type-key-based
// narrowing. It provides a flexible way to narrow types based on arbitrary
// predicates.
//
// # Narrowing Mode (exclude=false)
//
// Keeps only type members where the matcher returns true:
//   - Union: keeps members where matcher(member) == true.
//   - Optional: returns inner type if matcher(inner) == true, else Never.
//   - Intersection: returns type if matcher(intersection) == true, else Never.
//   - Other: returns type if matcher(type) == true, else Never.
//
// # Exclusion Mode (exclude=true)
//
// Removes type members where the matcher returns true:
//   - Union: keeps members where matcher(member) == false.
//   - Optional: returns Nil if matcher(inner) == true, else unchanged.
//   - Intersection: returns Never if matcher(intersection) == true, else unchanged.
//   - Other: returns Never if matcher(type) == true, else unchanged.
//
// # Type Unwrapping
//
// Aliases and instantiated generics are unwrapped before matching. The matcher
// receives the underlying concrete type, not wrapper types. This ensures
// consistent behavior regardless of how types are aliased or instantiated.
//
// # Intersection Handling
//
// Intersections are treated atomically: the matcher is applied to the whole
// intersection, not distributed over members. This is because intersection
// members cannot be independently filtered without changing the type's meaning.
//
// # Examples
//
//	// Keep only string members from a union.
//	result := FilterByMatch(union, func(t typ.Type) bool {
//	    return t == typ.String
//	}, false)
//
//	// Exclude number members from a union.
//	result := FilterByMatch(union, func(t typ.Type) bool {
//	    return t.Kind() == kind.Number
//	}, true)
func FilterByMatch(t typ.Type, matches TypeMatcher, exclude bool) typ.Type {
	if t == nil {
		return t
	}

	if a, ok := t.(*typ.Alias); ok {
		return FilterByMatch(a.Target, matches, exclude)
	}

	if expanded := unwrap.Instantiated(t); expanded != t {
		return FilterByMatch(expanded, matches, exclude)
	}

	if exclude {
		return filterExclude(t, matches)
	}

	return filterNarrow(t, matches)
}

func fieldCanBeTruthy(t typ.Type, field string, resolver Resolver) bool {
	if t == nil {
		return false
	}
	if t.Kind().IsPlaceholder() {
		return true
	}
	if rec, ok := t.(*typ.Record); ok {
		if f := rec.GetField(field); f != nil {
			if f.Type == nil {
				return true
			}
			return !ToTruthy(f.Type).Kind().IsNever()
		}
		if rec.Open || rec.HasMapComponent() {
			return true
		}
		return false
	}
	if m, ok := t.(*typ.Map); ok {
		if m.Value == nil {
			return true
		}
		return !ToTruthy(m.Value).Kind().IsNever()
	}
	if ft, ok := resolver.Field(t, field); ok && ft != nil {
		return !ToTruthy(ft).Kind().IsNever()
	}
	return true
}

func fieldCanBeFalsy(t typ.Type, field string, resolver Resolver) bool {
	if t == nil {
		return false
	}
	if t.Kind().IsPlaceholder() {
		return true
	}
	if rec, ok := t.(*typ.Record); ok {
		if f := rec.GetField(field); f != nil {
			if f.Type == nil {
				return true
			}
			return !ToFalsy(f.Type).Kind().IsNever()
		}
		return true
	}
	if m, ok := t.(*typ.Map); ok {
		if m.Value == nil {
			return true
		}
		return !ToFalsy(m.Value).Kind().IsNever()
	}
	if ft, ok := resolver.Field(t, field); ok && ft != nil {
		return !ToFalsy(ft).Kind().IsNever()
	}
	return true
}

// filterNarrow implements narrowing mode filtering, keeping only matching members.
//
// This function handles the positive case where we want to keep types that
// satisfy the matcher. It processes each type variant appropriately:
//   - Intersections are matched atomically.
//   - Optionals unwrap to their inner type when matched.
//   - Unions filter to only matching members.
//   - Other types return themselves if matched, Never otherwise.
func filterNarrow(t typ.Type, matches TypeMatcher) typ.Type {
	if inter, ok := t.(*typ.Intersection); ok {
		if matches(inter) {
			return t
		}
		return typ.Never
	}

	if opt, ok := t.(*typ.Optional); ok {
		if matches(opt.Inner) {
			return opt.Inner
		}

		return typ.Never
	}

	if u, ok := t.(*typ.Union); ok {
		var kept []typ.Type

		for _, m := range u.Members {
			if opt, ok := m.(*typ.Optional); ok {
				if matches(opt.Inner) {
					kept = append(kept, opt.Inner)
				}

				continue
			}

			if matches(m) {
				kept = append(kept, m)
			}
		}

		if len(kept) == 0 {
			return typ.Never
		}

		return typ.NewUnion(kept...)
	}

	if matches(t) {
		return t
	}

	return typ.Never
}

// filterExclude implements exclusion mode filtering, removing matching members.
//
// This function handles the negative case where we want to remove types that
// satisfy the matcher. It processes each type variant appropriately:
//   - Intersections are matched atomically; if matched, returns Never.
//   - Optionals preserve nil when inner matches (only inner is excluded).
//   - Unions filter out matching members.
//   - Other types return Never if matched, themselves otherwise.
func filterExclude(t typ.Type, matches TypeMatcher) typ.Type {
	if inter, ok := t.(*typ.Intersection); ok {
		if matches(inter) {
			return typ.Never
		}
		return t
	}

	if opt, ok := t.(*typ.Optional); ok {
		if matches(opt.Inner) {
			return typ.Nil
		}
		return t
	}

	u, ok := t.(*typ.Union)
	if !ok {
		if matches(t) {
			return typ.Never
		}

		return t
	}

	var kept []typ.Type

	for _, m := range u.Members {
		if opt, ok := m.(*typ.Optional); ok {
			if !matches(opt.Inner) {
				kept = append(kept, m)
			}

			continue
		}

		if !matches(m) {
			kept = append(kept, m)
		}
	}

	if len(kept) == 0 {
		return typ.Never
	}

	return typ.NewUnion(kept...)
}

// ByFieldLiteral narrows a type to members where a field equals a literal value.
//
// This function is the primary mechanism for discriminated union narrowing in
// Lua. When a guard checks a specific field value (e.g., "if x.kind == 'a'"),
// this function narrows the type to only those union variants where the field
// matches the literal.
//
// # Discriminated Unions
//
// A discriminated union uses a common field (discriminant) to distinguish
// between variants. For example:
//
//	type Event = {kind: "click", x: number, y: number}
//	           | {kind: "key", code: string}
//
// After checking "event.kind == 'click'", the type narrows to just the
// click variant, enabling type-safe access to x and y.
//
// # Parameters
//
//   - t: The type to narrow (typically a union of records).
//   - field: The discriminant field name.
//   - lit: The literal value to match against.
//   - resolver: Provides field type lookup for the type.
//
// # Returns
//
// Returns the narrowed type containing only variants where the field can
// match the literal. Returns the original type unchanged if any parameter
// is nil/empty or if the resolver is not provided.
//
// # Example
//
//	recA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
//	recB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()
//	union := typ.NewUnion(recA, recB)
//	narrowed := ByFieldLiteral(union, "kind", typ.LiteralString("a"), resolver)
//	// narrowed is recA
func ByFieldLiteral(t typ.Type, field string, lit *typ.Literal, resolver Resolver) typ.Type {
	if t == nil || field == "" || lit == nil || resolver == nil {
		return t
	}
	if t.Kind().IsPlaceholder() || unwrap.IsBuiltinTableTop(t) {
		// Refining `table` by a field literal should materialize a structural
		// shape so downstream assignment/subtyping can use the discriminant.
		// This also makes narrowing order-independent when placeholder and
		// table-type constraints are both present.
		return typ.NewRecord().Field(field, lit).SetOpen(true).Build()
	}

	return FilterByMatch(t, func(m typ.Type) bool {
		return FieldMatchesLiteral(m, field, lit, resolver)
	}, false)
}

// ExcludeByFieldLiteral excludes union members where a field exactly equals a literal.
//
// This function handles negative discriminant checks (field ~= value). Unlike
// [ByFieldLiteral], it only excludes types where the field IS exactly the
// literal, not types where the field might contain it.
//
// # Exact vs Contains Semantics
//
// The distinction between exact and contains matching is crucial for soundness:
//   - Exact: field type is the singleton literal (e.g., kind: "a").
//   - Contains: field type includes the literal (e.g., kind: string).
//
// For negative checks (x.kind ~= "a"), we can only exclude variants where
// kind is exactly "a". A variant where kind is string cannot be excluded
// because it might hold any string value, not just "a".
//
// # Parameters
//
//   - t: The type to narrow.
//   - field: The discriminant field name.
//   - lit: The literal value to exclude.
//   - resolver: Provides field type lookup.
//
// # Returns
//
// Returns the type with variants removed where the field is exactly the
// literal. Variants with broader field types (like string) are preserved.
//
// # Example
//
//	recA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
//	recB := typ.NewRecord().Field("kind", typ.String).Build()
//	union := typ.NewUnion(recA, recB)
//	narrowed := ExcludeByFieldLiteral(union, "kind", typ.LiteralString("a"), resolver)
//	// narrowed is recB (recA excluded, recB kept because kind: string is broader)
func ExcludeByFieldLiteral(t typ.Type, field string, lit *typ.Literal, resolver Resolver) typ.Type {
	if t == nil || field == "" || lit == nil || resolver == nil {
		return t
	}

	return FilterByMatch(t, func(m typ.Type) bool {
		return FieldIsExactlyLiteral(m, field, lit, resolver)
	}, true)
}

// FieldIsExactlyLiteral returns true only if the field type is exactly the literal.
//
// This function checks whether a type's field has a singleton literal type
// that matches the given literal exactly. It returns false for broader types
// that contain the literal as a possible value.
//
// # Use Case
//
// This is used by [ExcludeByFieldLiteral] to implement sound negative
// narrowing. Only types where the field is provably the excluded literal
// can be removed; types where the field might be something else must be kept.
//
// # Parameters
//
//   - t: The type to check (typically a record).
//   - field: The field name to look up.
//   - lit: The literal to compare against.
//   - resolver: Provides field type lookup.
//
// # Returns
//
// Returns true if:
//  1. The field exists on the type.
//  2. The field's type is exactly the literal (via [TypeIsExactlyLiteral]).
//
// Returns false if any parameter is nil, the field doesn't exist, or the
// field type is broader than the literal.
func FieldIsExactlyLiteral(t typ.Type, field string, lit *typ.Literal, resolver Resolver) bool {
	if resolver == nil || t == nil || lit == nil {
		return false
	}

	fieldType, ok := resolver.Field(t, field)
	if !ok || fieldType == nil {
		return false
	}

	return TypeIsExactlyLiteral(fieldType, lit)
}

// TypeIsExactlyLiteral returns true only if the type is exactly the given literal.
//
// This function distinguishes between a singleton literal type and broader
// types that contain the literal as a member. It is fundamental to sound
// negative narrowing.
//
// # Behavior by Type
//
//   - Literal: Returns true if the literal values are equal.
//   - Alias: Unwraps and checks the target type.
//   - Union: Returns true only if ALL members are exactly the literal.
//   - Optional: Returns true if the inner type is exactly the literal.
//   - Other: Returns false (primitives, records, etc. are broader).
//
// # Examples
//
//	TypeIsExactlyLiteral(typ.LiteralString("a"), typ.LiteralString("a")) // true
//	TypeIsExactlyLiteral(typ.String, typ.LiteralString("a"))             // false
//	TypeIsExactlyLiteral(typ.NewOptional(typ.LiteralString("a")), ...)   // true
//
// # Union Semantics
//
// For unions, all members must be the exact literal. This handles the case
// of duplicate literal types in unions. A union like "a" | "b" would return
// false for either literal because not all members match.
func TypeIsExactlyLiteral(t typ.Type, lit *typ.Literal) bool {
	if t == nil || lit == nil {
		return false
	}

	if a, ok := t.(*typ.Alias); ok {
		return TypeIsExactlyLiteral(a.Target, lit)
	}

	if l, ok := t.(*typ.Literal); ok {
		return typ.LiteralEquals(l, lit)
	}

	// For unions, check if ALL members are the exact literal.
	if u, ok := t.(*typ.Union); ok {
		if len(u.Members) == 0 {
			return false
		}
		for _, m := range u.Members {
			if !TypeIsExactlyLiteral(m, lit) {
				return false
			}
		}
		return true
	}

	// For optional types, check if the inner type is exactly the literal.
	if opt, ok := t.(*typ.Optional); ok {
		return TypeIsExactlyLiteral(opt.Inner, lit)
	}

	return false
}

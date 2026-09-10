package core

import "github.com/wippyai/go-lua/types/typ"

// TraverseUnion applies a function to each union member recursively.
//
// If t is a union, the function is applied to each member (recursively for
// nested unions). Returns the first non-nil/non-zero result, or zero value
// if all applications returned zero.
//
// If t is not a union, the function is applied directly to t.
//
// This is useful for finding a specific member type or checking a property
// that should hold for at least one member.
func TraverseUnion[T any](t typ.Type, f func(typ.Type) T) T {
	return traverseUnionDepth(t, f, 0)
}

// traverseUnionDepth recursively traverses unions with depth limiting.
func traverseUnionDepth[T any](t typ.Type, f func(typ.Type) T, depth int) T {
	var zero T
	if stopDepth(t, depth) {
		return zero
	}

	if u, ok := t.(*typ.Union); ok {
		for _, m := range u.Members {
			if result := traverseUnionDepth[T](m, f, depth+1); any(result) != nil {
				return result
			}
		}

		return zero
	}

	return f(t)
}

// TraverseIntersection applies a function to each intersection member recursively.
//
// Similar to TraverseUnion but for intersection types. If t is an intersection,
// applies f to each member recursively. Returns the first non-zero result.
//
// If t is not an intersection, applies f directly to t.
func TraverseIntersection[T any](t typ.Type, f func(typ.Type) T) T {
	return traverseIntersectionDepth(t, f, 0)
}

// traverseIntersectionDepth recursively traverses intersections with depth limiting.
func traverseIntersectionDepth[T any](t typ.Type, f func(typ.Type) T, depth int) T {
	var zero T
	if stopDepth(t, depth) {
		return zero
	}

	if i, ok := t.(*typ.Intersection); ok {
		for _, m := range i.Members {
			if result := traverseIntersectionDepth[T](m, f, depth+1); any(result) != nil {
				return result
			}
		}

		return zero
	}

	return f(t)
}

// TraverseOptional unwraps optional types and applies a function to the inner type.
//
// If t is Optional, recursively unwraps until a non-optional type is found,
// then applies f. This is useful when you need to operate on the "real" type
// inside nested optionals.
//
// If t is not optional, applies f directly.
func TraverseOptional[T any](t typ.Type, f func(typ.Type) T) T {
	return traverseOptionalDepth(t, f, 0)
}

// traverseOptionalDepth recursively unwraps optionals with depth limiting.
func traverseOptionalDepth[T any](t typ.Type, f func(typ.Type) T, depth int) T {
	var zero T
	if stopDepth(t, depth) {
		return zero
	}

	if o, ok := t.(*typ.Optional); ok {
		return traverseOptionalDepth[T](o.Inner, f, depth+1)
	}

	return f(t)
}

// ForEachMember calls f for each leaf member type in a composite type.
//
// For unions and intersections, recursively visits all member types.
// For other types, calls f with the type itself.
//
// The callback returns false to stop iteration early.
// ForEachMember returns true if all callbacks returned true.
//
// This is useful for checking properties that must hold for all members.
func ForEachMember(t typ.Type, f func(typ.Type) bool) bool {
	return forEachMemberDepth(t, f, 0)
}

// forEachMemberDepth recursively visits members with depth limiting.
func forEachMemberDepth(t typ.Type, f func(typ.Type) bool, depth int) bool {
	if stopDepth(t, depth) {
		return true
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if !forEachMemberDepth(m, f, depth+1) {
					return false
				}
			}

			return true
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if !forEachMemberDepth(m, f, depth+1) {
					return false
				}
			}

			return true
		},
		Default: func(t typ.Type) bool {
			return f(t)
		},
	})
}

// AllMembers returns all leaf types from a union or intersection.
//
// Flattens nested unions and intersections into a single slice of member types.
// For a non-composite type, returns a slice containing just that type.
//
// Example: (A | (B | C)) & D returns [A, B, C, D]
func AllMembers(t typ.Type) []typ.Type {
	var result []typ.Type

	ForEachMember(t, func(m typ.Type) bool {
		result = append(result, m)
		return true
	})

	return result
}

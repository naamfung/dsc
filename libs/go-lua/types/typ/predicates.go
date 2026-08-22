package typ

import "github.com/wippyai/go-lua/types/kind"

// IsUnknown reports whether t is explicitly the unknown type.
func IsUnknown(t Type) bool {
	return t != nil && t.Kind() == kind.Unknown
}

// IsAny reports whether t is explicitly the any type.
func IsAny(t Type) bool {
	return t != nil && t.Kind() == kind.Any
}

// IsNever reports whether t is explicitly the never type.
func IsNever(t Type) bool {
	return t != nil && t.Kind() == kind.Never
}

// IsAbsentOrUnknown reports whether t is missing (nil) or unknown.
//
// This intentionally does not treat the explicit nil type as unknown.
func IsAbsentOrUnknown(t Type) bool {
	return t == nil || IsUnknown(t)
}

// IsUnknownOrNil reports whether t is missing (nil), unknown, or explicit nil type.
func IsUnknownOrNil(t Type) bool {
	return IsAbsentOrUnknown(t) || (t != nil && t.Kind() == kind.Nil)
}

// HasKnownType reports whether the slice contains at least one concrete type.
// nil entries and unknown entries are treated as unresolved.
func HasKnownType(types []Type) bool {
	for _, t := range types {
		if !IsAbsentOrUnknown(t) {
			return true
		}
	}
	return false
}

// IsUnknownOnlyOrEmpty reports whether the slice has no concrete types.
func IsUnknownOnlyOrEmpty(types []Type) bool {
	return !HasKnownType(types)
}

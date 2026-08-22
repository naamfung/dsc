package narrow

import (
	"github.com/wippyai/go-lua/types/typ"
)

// ByTypeKey narrows a type to match a specific TypeKey identity.
//
// This function is the primary entry point for type-based narrowing after
// typeof checks in Lua. It handles both built-in type checks (type(x) == "number")
// and user-defined type checks (via hash-based keys).
//
// # Builtin Keys
//
// For built-in type keys (TypeKeyBuiltin), the function:
//  1. Converts the key name to a kind via kind.FromString.
//  2. Delegates to [FilterByKind] to narrow the type.
//
// This handles Lua's typeof semantics where "table" matches multiple kinds
// (Record, Map, Array, etc.) and "number" matches both Number and Integer.
//
// # Hash Keys
//
// For hash-based type keys (TypeKeyHash), the function:
//  1. Uses the resolver to recover the full type.
//  2. Computes the intersection of the base type and resolved type.
//
// This enables narrowing to specific record or interface types.
//
// # Edge Cases
//
//   - Nil input: Returns nil.
//   - Zero key: Returns original type unchanged.
//   - Unknown builtin kind: Returns original type unchanged.
//   - Nil resolver for hash key: Returns original type unchanged.
//   - Nil resolution: Returns original type unchanged.
//
// # Examples
//
//	// Narrow union to only number members.
//	key := BuiltinTypeKey("number")
//	narrowed := ByTypeKey(typ.NewUnion(typ.String, typ.Number), key, nil)
//	// narrowed is typ.Number
//
//	// Narrow to a specific record type.
//	key := HashTypeKey(myRecord.Hash())
//	narrowed := ByTypeKey(union, key, resolver)
func ByTypeKey(t typ.Type, key TypeKey, resolve TypeResolver) typ.Type {
	if t == nil {
		return nil
	}

	if key.IsZero() {
		return t
	}
	if key.Kind == TypeKeyBuiltin {
		targetKind, ok := key.BuiltinKind()
		if !ok {
			return t
		}
		return FilterByKind(t, targetKind)
	}
	if resolve == nil {
		return t
	}

	exact := resolve(key)
	if exact == nil {
		return t
	}

	return Intersect(t, exact)
}

// ExcludeByTypeKey removes types matching a TypeKey from the input type.
//
// This function is the negative counterpart to [ByTypeKey]. It handles
// exclusion after negative typeof checks (type(x) ~= "number").
//
// # Builtin Keys
//
// For built-in type keys, delegates to [ExcludeKind] to remove matching members.
//
// # Hash Keys
//
// For hash-based keys, uses the resolver to recover the full type and
// excludes it via [ExcludeType].
//
// # Never Preservation
//
// If exclusion would result in Never (all members excluded), returns the
// original type unchanged. This prevents over-narrowing when the type check
// cannot definitively exclude all possibilities.
//
// This is important for soundness: if we have type "any" and check
// "type(x) ~= 'number'", we cannot narrow to anything useful because
// "any" could be anything, and excluding number from any still leaves
// everything except number (which we cannot represent precisely).
//
// # Edge Cases
//
//   - Nil input: Returns nil.
//   - Zero key: Returns original type unchanged.
//   - Unknown builtin kind: Returns original type unchanged.
//   - Nil resolver for hash key: Returns original type unchanged.
//   - Nil resolution: Returns original type unchanged.
//   - Exclusion produces Never: Returns original type (preservation).
//
// # Examples
//
//	// Exclude string from a union.
//	key := BuiltinTypeKey("string")
//	narrowed := ExcludeByTypeKey(typ.NewUnion(typ.String, typ.Number), key, nil)
//	// narrowed is typ.Number
//
//	// Excluding the only member preserves original.
//	narrowed := ExcludeByTypeKey(typ.String, key, nil)
//	// narrowed is typ.String (not Never)
func ExcludeByTypeKey(t typ.Type, key TypeKey, resolve TypeResolver) typ.Type {
	if t == nil {
		return nil
	}
	if key.IsZero() {
		return t
	}
	if key.Kind == TypeKeyBuiltin {
		targetKind, ok := key.BuiltinKind()
		if !ok {
			return t
		}
		narrowed := ExcludeKind(t, targetKind)
		if typ.IsNever(narrowed) {
			return t
		}
		return narrowed
	}
	if key.Kind == TypeKeyHash && resolve != nil {
		exact := resolve(key)
		if exact == nil {
			return t
		}
		narrowed := ExcludeType(t, exact)
		if typ.IsNever(narrowed) {
			return t
		}
		return narrowed
	}
	return t
}

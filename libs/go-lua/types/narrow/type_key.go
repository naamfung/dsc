package narrow

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeKey identifies a type for narrowing operations using either a built-in
// type name or a content hash.
//
// Type keys enable efficient type discrimination without carrying full type
// information. They are used by the constraint solver to represent type
// checks (HasType, NotHasType) in a compact form that can be stored in
// constraints and compared quickly.
//
// # Key Kinds
//
// There are two kinds of type keys:
//
//   - Builtin ([TypeKeyBuiltin]): Identifies primitive types by name.
//     Used for typeof checks like "type(x) == 'number'".
//     The Name field contains the Lua typeof string.
//
//   - Hash ([TypeKeyHash]): Identifies user-defined types by structural hash.
//     Used for interface/record type checks.
//     The Hash field contains the type's content hash.
//
// # Thread Safety
//
// TypeKey is a value type and safe to copy and use concurrently.
//
// # Examples
//
//	BuiltinTypeKey("number")   // identifies the number type
//	HashTypeKey(record.Hash()) // identifies a specific record type
type TypeKey struct {
	Kind TypeKeyKind // Discriminant for key type.
	Name string      // Type name for built-in types (e.g., "number", "string").
	Hash uint64      // Content hash for user-defined types.
}

// TypeKeyKind distinguishes between type key variants.
type TypeKeyKind uint8

const (
	// TypeKeyInvalid represents an uninitialized or invalid type key.
	// This is the zero value and indicates no type.
	TypeKeyInvalid TypeKeyKind = iota

	// TypeKeyBuiltin identifies a built-in Lua type by name.
	// The Name field contains the typeof string (e.g., "number", "string").
	TypeKeyBuiltin

	// TypeKeyHash identifies a user-defined type by structural hash.
	// The Hash field contains the type's content hash for equality comparison.
	TypeKeyHash
)

// BuiltinTypeKey creates a TypeKey for a built-in Lua type name.
//
// Built-in type keys are used for typeof-based narrowing. The name should
// be a valid Lua typeof result: "nil", "boolean", "number", "string",
// "function", "table", "thread", or "userdata".
//
// Returns the zero TypeKey if name is empty.
func BuiltinTypeKey(name string) TypeKey {
	if name == "" {
		return TypeKey{}
	}

	return TypeKey{Kind: TypeKeyBuiltin, Name: name}
}

// KnownBuiltinTypeKey creates a TypeKey for a recognized Lua type() result.
//
// Returns false when name is not a supported built-in type string.
func KnownBuiltinTypeKey(name string) (TypeKey, bool) {
	if kind.FromString(name) == kind.Unknown {
		return TypeKey{}, false
	}
	return BuiltinTypeKey(name), true
}

// HashTypeKey creates a TypeKey for a hash-based type identity.
//
// Hash-based type keys are used for user-defined types (records, interfaces)
// where typeof would just return "table". The hash uniquely identifies the
// type's structure.
//
// Returns the zero TypeKey if hash is 0.
func HashTypeKey(hash uint64) TypeKey {
	if hash == 0 {
		return TypeKey{}
	}

	return TypeKey{Kind: TypeKeyHash, Hash: hash}
}

// IsZero reports whether the key is the zero value (invalid/unset).
//
// A zero key represents no type and should not be used for narrowing.
func (k TypeKey) IsZero() bool { return k.Kind == TypeKeyInvalid }

// BuiltinKind resolves a built-in key name to a Kind.
//
// Returns false when k is not a built-in key or when its name is unknown.
func (k TypeKey) BuiltinKind() (kind.Kind, bool) {
	if k.Kind != TypeKeyBuiltin {
		return kind.Unknown, false
	}
	resolved := kind.FromString(k.Name)
	if resolved == kind.Unknown {
		return kind.Unknown, false
	}
	return resolved, true
}

// Hash64 computes a 64-bit hash for the type key suitable for use in hash tables.
//
// The hash combines the key kind with either the name hash (for builtins)
// or the type hash (for user types). This enables efficient storage in
// hash-based data structures.
//
// Returns 0 for invalid/zero keys.
func (k TypeKey) Hash64() uint64 {
	switch k.Kind {
	case TypeKeyBuiltin:
		return internal.HashCombine(uint64(TypeKeyBuiltin), internal.FnvString(k.Name))
	case TypeKeyHash:
		return internal.HashCombine(uint64(TypeKeyHash), k.Hash)
	default:
		return 0
	}
}

// Equal reports whether two type keys are identical.
//
// Keys must match in kind, name (for builtins), and hash (for user types).
// Two zero keys are considered equal.
func (k TypeKey) Equal(other TypeKey) bool {
	return k.Kind == other.Kind && k.Name == other.Name && k.Hash == other.Hash
}

// TypeResolver resolves a concrete type from a type key.
//
// Used during constraint application to recover full type information from
// the compact key representation. The resolver maps:
//   - Builtin keys to their corresponding primitive types.
//   - Hash keys to their registered user-defined types.
//
// The resolver is typically provided by the type environment that tracks
// all defined types in the program.
//
// Returns nil if the key cannot be resolved (unknown type).
type TypeResolver func(TypeKey) typ.Type

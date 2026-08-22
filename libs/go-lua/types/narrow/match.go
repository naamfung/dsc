package narrow

import (
	"github.com/wippyai/go-lua/types/typ"
)

// FieldMatchesLiteral checks if a type's field can match a literal value.
//
// This function is the core predicate for discriminated union narrowing.
// It determines whether a specific union variant should be kept when
// narrowing based on a field value check.
//
// # Matching Semantics
//
// A field "matches" a literal if the literal is a valid value for that field.
// This is determined by [typ.TypeMatchesLiteral], which checks if the literal
// is a subtype of the field type or vice versa.
//
// For example:
//   - Field type "a" matches literal "a" (exact match).
//   - Field type string matches literal "a" (literal is subtype).
//   - Field type "a" | "b" matches literal "a" (literal in union).
//   - Field type number does NOT match literal "a" (incompatible).
//
// # Use Case
//
// This function is used by [ByFieldLiteral] to filter union variants.
// For a check like "x.kind == 'a'", we want to keep variants where the
// kind field can possibly equal "a".
//
// # Parameters
//
//   - t: The type to check (typically a record in a discriminated union).
//   - field: The name of the discriminant field.
//   - lit: The literal value being checked.
//   - resolver: Provides field type lookup for the type.
//
// # Returns
//
// Returns true if:
//  1. All parameters are non-nil.
//  2. The field exists on the type.
//  3. The field's type is compatible with the literal.
//
// Returns false if any parameter is nil, the field doesn't exist,
// or the field type is incompatible with the literal.
//
// # Example
//
//	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
//	lit := typ.LiteralString("a")
//	matches := FieldMatchesLiteral(rec, "kind", lit, resolver) // true
//
//	rec := typ.NewRecord().Field("kind", typ.Number).Build()
//	matches := FieldMatchesLiteral(rec, "kind", lit, resolver) // false
func FieldMatchesLiteral(t typ.Type, field string, lit *typ.Literal, resolver Resolver) bool {
	if resolver == nil || t == nil || lit == nil {
		return false
	}

	fieldType, ok := resolver.Field(t, field)
	if !ok || fieldType == nil {
		return false
	}

	return typ.TypeMatchesLiteral(fieldType, lit)
}

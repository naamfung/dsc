// Package narrow provides type narrowing operations for control flow refinement.
//
// Type narrowing reduces types based on runtime checks, enabling sound type
// refinement after conditionals. For example, "if x ~= nil" narrows x's type
// by removing nil, giving a more precise type in the then-branch.
//
// # Lua Truthiness Model
//
// Lua has a simple truthiness model: only nil and false are falsy; all other
// values (including 0, "", empty tables, and userdata) are truthy. This package
// implements Lua's truthiness semantics for type narrowing, which differs from
// many other languages where 0 and "" might be falsy.
//
// # Architecture
//
// The package is organized into several functional areas:
//
//   - Core narrowing engine (narrow.go): The [narrowConfig] pattern provides
//     a recursive traversal framework for implementing narrowing operations.
//     Each operation defines handlers for optionals, unions, and leaf types.
//
//   - Truthiness operations (narrow.go): [ToTruthy], [ToFalsy], [RemoveNil],
//     and [RemoveFalse] implement Lua's truthiness-based narrowing.
//
//   - Kind-based narrowing (narrow.go): [FilterByKind] and [ExcludeKind]
//     narrow types based on Lua's typeof results.
//
//   - Type-based narrowing (narrow.go): [ExcludeType] and [Intersect]
//     narrow using type relationships.
//
//   - TypeKey narrowing (type_key.go, key_narrow.go): Compact type identifiers
//     enable efficient storage and comparison of type constraints.
//
//   - Filter-based narrowing (filter.go): The [TypeMatcher] pattern enables
//     flexible predicate-based filtering.
//
//   - Field-based narrowing (filter.go, match.go): [ByFieldLiteral] and
//     [ExcludeByFieldLiteral] implement discriminated union narrowing.
//
//   - Resolver interface (resolver.go): Abstracts field and index lookup
//     to decouple narrowing from the full type checker.
//
// # Integration with Type System
//
// The narrow package is used by the constraint solver during control flow
// analysis. When the solver encounters conditionals, it generates narrowing
// constraints that are applied using these operations:
//
//   - if x then: Apply [ToTruthy] to x in the then-branch.
//   - if not x then: Apply [ToFalsy] to x in the then-branch.
//   - if type(x) == "number" then: Apply [FilterByKind] with kind.Number.
//   - if x.kind == "a" then: Apply [ByFieldLiteral] with the literal.
//
// # Soundness Guarantees
//
// All narrowing operations maintain type system soundness:
//
//   - Never over-narrow: If exclusion would produce Never, preserve original.
//   - Respect subtyping: [TypesOverlap] uses bidirectional subtype checks.
//   - Handle placeholders: Any/Unknown types are handled conservatively.
//   - Distinguish exact vs contains: Negative field checks use exact matching.
//
// # Example Usage
//
//	// Narrow optional to non-nil after guard.
//	optStr := typ.NewOptional(typ.String)
//	nonNil := narrow.RemoveNil(optStr)  // typ.String
//
//	// Narrow union by typeof check.
//	union := typ.NewUnion(typ.String, typ.Number, typ.Nil)
//	nums := narrow.FilterByKind(union, kind.Number)  // typ.Number
//
//	// Narrow discriminated union by field check.
//	narrowed := narrow.ByFieldLiteral(eventUnion, "kind", typ.LiteralString("click"), resolver)
//
// # Performance Considerations
//
// Narrowing operations are designed for efficiency:
//
//   - TypeKey uses hashing for fast type identity comparison.
//   - Narrowing short-circuits when types are unchanged.
//   - Union filtering is linear in the number of members.
//   - No allocations when narrowing produces the same type.
package narrow

// Package predicate builds type predicates from comparison expressions.
//
// This package analyzes comparison operations to extract type predicates
// that constrain variable types. Predicates are the building blocks of
// type guards used for narrowing.
//
// # Predicate Types
//
// The package recognizes patterns like:
//
//	x == nil        -> IsNil predicate
//	x ~= nil        -> NotNil predicate
//	type(x) == "string"  -> IsType("string") predicate
//	x:is(Foo)       -> InstanceOf(Foo) predicate
//
// # Predicate Composition
//
// Predicates combine through logical operators:
//
//	x ~= nil and type(x) == "string"  -> And(NotNil, IsType("string"))
//	x == nil or x == false            -> Or(IsNil, IsFalse)
//
// # Negation
//
// The package handles negation correctly:
//
//	not (x == nil)  -> NotNil
//	not (type(x) == "string")  -> NotType("string")
//
// # Integration
//
// Predicates become part of [flow.Guard] records that the constraint
// solver uses to narrow types at branch points.
package predicate

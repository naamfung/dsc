// Package ops implements type operations for expression synthesis.
//
// This package provides the core type operations used during expression
// type synthesis: function calls, field access, indexing, and operators.
//
// # Call Operations
//
// [Call] handles function call type computation:
//   - Match arguments to parameters
//   - Instantiate generic type parameters
//   - Compute return types
//   - Validate argument compatibility
//
// # Field Access
//
// Field access on records and interfaces:
//
//	obj.field  -- lookup field type in obj's type
//
// # Index Operations
//
// Index operations on arrays and maps:
//
//	arr[1]     -- array element type
//	map[key]   -- map value type
//
// # Operators
//
// Binary and unary operators:
//
//	a + b      -- numeric addition
//	a .. b     -- string concatenation
//	not x      -- boolean negation
//
// # Type Checking
//
// [Check] validates that an expression type is compatible with
// an expected type, producing diagnostics for mismatches.
//
// # Generic Instantiation
//
// [Generic] handles generic function instantiation, binding type
// parameters to concrete types based on argument types.
package ops

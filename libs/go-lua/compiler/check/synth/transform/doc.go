// Package transform provides type transformation operations.
//
// This package implements type transformations applied during synthesis,
// including spec-based return type computation and type matching.
//
// # Spec Return Transformation
//
// For functions with type specs (generics, overloads):
//
//	---@generic T
//	---@param arr T[]
//	---@return T
//	function first(arr) ... end
//
// The package computes the concrete return type by:
//  1. Matching argument types to parameter specs
//  2. Binding type variables
//  3. Substituting in the return type spec
//
// # Spec Matching
//
// The specmatch subpackage handles pattern matching against type specs,
// extracting type variable bindings from argument types.
//
// # Integration
//
// Transformations are applied during call synthesis to compute
// concrete return types from generic function specs.
package transform

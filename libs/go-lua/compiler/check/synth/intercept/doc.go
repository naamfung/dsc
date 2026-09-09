// Package intercept handles special-case type synthesis interceptions.
//
// This package intercepts specific expression patterns that require
// special type handling beyond normal synthesis. This includes type
// casts, type guards, and spec-based return type computation.
//
// # Type Casts
//
// Explicit type casts via annotation:
//
//	local x = value --[[@as string]]
//
// The package intercepts the annotated expression and applies the cast.
//
// # Type Guards
//
// Type guard functions that narrow types:
//
//	if x:is(Foo) then
//	    -- x is Foo here
//	end
//
// The package recognizes guard patterns and produces appropriate
// narrowing predicates.
//
// # Spec Returns
//
// Functions with spec annotations that determine return types:
//
//	---@generic T
//	---@param t T[]
//	---@return T
//	function first(t) ... end
//
// The package uses the spec to compute the concrete return type
// based on argument types.
//
// # Integration
//
// Interceptions are checked before normal synthesis. If an interception
// applies, it produces the final type; otherwise, normal synthesis proceeds.
package intercept

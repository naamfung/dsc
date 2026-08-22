// Package synth implements type synthesis for Lua expressions.
//
// This package provides the core expression type inference engine. It
// synthesizes types for expressions by combining static type information
// with flow-sensitive type facts.
//
// # Synthesis Engine
//
// [Engine] is the main type synthesizer, providing:
//   - Expression type synthesis (TypeOf, Synth)
//   - Expected type propagation (SynthWithExpected)
//   - Type annotation resolution (ResolveType)
//
// # Bidirectional Type Checking
//
// The synthesizer supports bidirectional type checking:
//   - Synthesis: infer type from expression structure
//   - Checking: validate expression against expected type
//
// For table literals, expected types guide field type inference:
//
//	local p: Point = { x = 1, y = 2 }  -- fields typed as Point fields
//
// # Flow Integration
//
// The synthesizer queries flow facts to get narrowed types:
//
//	if x ~= nil then
//	    print(x)  -- synthesizes non-nil type for x
//	end
//
// # Subpackages
//
// The synth package has several subpackages:
//   - ops: Type operations (call, index, field access)
//   - intercept: Special-case handling (type casts, type guards)
//   - phase/extract: Expression extraction from CFG
//   - phase/resolve: Final type resolution
//   - transform: Type transformations (spec returns, etc.)
package synth

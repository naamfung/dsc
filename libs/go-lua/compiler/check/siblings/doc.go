// Package siblings manages sibling function type coordination.
//
// This package handles the coordination of function types for sibling
// functions - local functions defined in the same scope that may
// reference each other.
//
// # Sibling Functions
//
// Functions in the same scope can call each other:
//
//	local function isEven(n)
//	    if n == 0 then return true end
//	    return isOdd(n - 1)
//	end
//	local function isOdd(n)
//	    if n == 0 then return false end
//	    return isEven(n - 1)
//	end
//
// The package ensures both functions see each other's types during analysis.
//
// # Overlay System
//
// [Overlay] provides a view that combines:
//   - Stable sibling types from previous iterations
//   - Pending updates from current iteration
//
// This supports fixpoint iteration for mutually recursive siblings.
//
// # Integration
//
// Sibling coordination runs as part of nested function analysis,
// ensuring consistent types across sibling definitions.
package siblings

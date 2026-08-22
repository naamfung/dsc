// Package sibling handles sibling function analysis in nested scopes.
//
// This package identifies and processes sibling functions - functions defined
// in the same scope that can reference each other. This is important for
// mutually recursive local functions.
//
// # Sibling Detection
//
// Sibling functions share a common parent scope:
//
//	local function foo()
//	    bar()  -- calls sibling
//	end
//	local function bar()
//	    foo()  -- calls sibling
//	end
//
// # Type Propagation
//
// The package ensures that sibling function types are available for:
//   - Call site type checking
//   - Return type inference
//   - Mutual recursion analysis
//
// # Integration
//
// Sibling analysis runs early in the flow build process to establish
// function types before analyzing call sites within the shared scope.
package sibling

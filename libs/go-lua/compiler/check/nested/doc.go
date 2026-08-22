// Package nested handles nested function analysis coordination.
//
// This package manages the analysis of nested function definitions,
// coordinating between parent and child scopes to propagate types
// and ensure consistent analysis ordering.
//
// # Nested Function Processing
//
// When a function contains nested definitions:
//
//	function outer()
//	    local x = 1
//	    local function inner()
//	        return x  -- captures x
//	    end
//	end
//
// The package ensures:
//   - Parent scope is analyzed first
//   - Captured variable types flow to nested functions
//   - Nested function results propagate back to parent
//
// # Constructor Analysis
//
// For functions that construct objects with methods:
//
//	function newCounter()
//	    local count = 0
//	    return {
//	        inc = function() count = count + 1 end
//	    }
//	end
//
// The package coordinates method analysis with constructor return
// type inference.
//
// # Sibling Enrichment
//
// Nested functions in the same scope (siblings) may reference each
// other. The package ensures mutual visibility of sibling types.
package nested

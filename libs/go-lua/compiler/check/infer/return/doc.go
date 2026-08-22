// Package infer infers function return types from body analysis.
//
// This package analyzes function bodies to determine return types when
// they are not explicitly annotated. It examines all return paths and
// synthesizes a unified return type.
//
// # Return Collection
//
// The package collects types from all return statements:
//
//	function f(x)
//	    if x > 0 then
//	        return x        -- number
//	    else
//	        return "neg"    -- string
//	    end
//	end
//
// Result: returns (number | string)
//
// # Implicit Returns
//
// Functions without explicit returns have implicit nil returns:
//
//	function f()
//	    print("hi")
//	    -- implicit: return nil
//	end
//
// # Recursive Functions
//
// For recursive functions, return type inference participates in
// the fixpoint iteration handled by the returns package.
//
// # Integration
//
// Return type inference feeds into signature construction and the
// interprocedural analysis that resolves cross-function calls.
package infer

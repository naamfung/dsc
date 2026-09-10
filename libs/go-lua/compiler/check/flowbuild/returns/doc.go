// Package returns extracts return type information from function bodies.
//
// This package analyzes return statements in the CFG to determine function
// return types. It handles multiple returns, conditional returns, and
// early exits.
//
// # Return Analysis
//
// For each return statement:
//
//	return x, y     -- captures types of x and y
//	return          -- captures empty return (void/nil)
//	error("msg")    -- captures never-returning calls
//
// # Conditional Returns
//
// The package tracks which branches lead to returns:
//
//	if cond then
//	    return 1
//	else
//	    return "a"
//	end
//
// This produces a union type (number | string) for the return.
//
// # Early Exits
//
// Functions that don't always return are detected:
//
//	function f()
//	    if cond then return 1 end
//	    -- implicit nil return
//	end
//
// # Integration
//
// Return information feeds into signature inference and the interprocedural
// return type analysis in the returns package.
package returns

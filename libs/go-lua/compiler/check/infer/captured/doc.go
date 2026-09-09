// Package captured computes captured variable types for nested functions.
//
// This package determines the types of variables captured by nested function
// definitions at their definition points. Captured types flow from parent
// scope analysis into nested function analysis.
//
// # Capture Analysis
//
// When a nested function references a variable from an outer scope:
//
//	local x = getValue()
//	local function inner()
//	    print(x)  -- x is captured
//	end
//
// The package computes x's type at the point where inner is defined,
// using flow facts from the parent function's analysis.
//
// # Flow Integration
//
// Captured types are computed from parent [flow.TypeFacts]:
//   - The parent's flow solution provides types at each CFG point
//   - At the nested function's definition point, captured variable
//     types are extracted and passed to the nested analysis
//
// # Usage
//
// [FromParentFacts] is the main entry point, producing a map from
// captured symbol IDs to their types at the capture point.
package captured

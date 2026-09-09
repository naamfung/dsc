// Package constprop performs constant propagation for flow analysis.
//
// This package tracks constant values through the CFG to enable more precise
// type narrowing. When a variable is known to hold a specific constant value,
// that information propagates to downstream uses.
//
// # Tracked Constants
//
// The package tracks:
//   - Numeric literals (integers, floats)
//   - String literals
//   - Boolean literals (true, false)
//   - nil
//
// # Usage
//
// Constant propagation enables narrowing in patterns like:
//
//	local tag = "foo"
//	if tag == "foo" then  -- always true, branch is taken
//	    ...
//	end
//
// And discriminated unions:
//
//	if obj.kind == "circle" then
//	    -- obj is narrowed to circle variant
//	end
//
// # Integration
//
// Constant values are stored in [flow.Inputs] and used by the constraint
// solver to evaluate guards and narrow union types.
package constprop

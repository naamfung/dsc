// Package numconst handles numeric constant analysis for type inference.
//
// This package determines precise numeric types for constant expressions,
// distinguishing between integers and floats where relevant.
//
// # Numeric Precision
//
// The package tracks:
//   - Integer constants: 1, 42, -100
//   - Float constants: 1.5, 3.14, -0.001
//   - Integer-valued floats: 1.0, 42.0
//
// # Type Refinement
//
// When a numeric literal appears in a context expecting a specific type,
// the package validates compatibility:
//
//	local x: integer = 42    -- valid
//	local y: integer = 1.5   -- error: float assigned to integer
//
// # Integration
//
// Numeric constant analysis supports the constraint solver in making
// precise decisions about numeric type compatibility.
package numconst

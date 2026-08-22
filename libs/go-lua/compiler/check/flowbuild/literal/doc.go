// Package literal handles literal expression type synthesis for flow analysis.
//
// This package determines types for literal expressions (numbers, strings,
// tables, functions) in the context of expected types, enabling bidirectional
// type inference.
//
// # Literal Types
//
// Basic literals have straightforward types:
//   - 123 -> number
//   - "foo" -> string
//   - true -> boolean
//   - nil -> nil
//
// # Table Literals
//
// Table literals are more complex and may be inferred as:
//   - Records: { name = "foo" } -> { name: string }
//   - Arrays: { 1, 2, 3 } -> number[]
//   - Maps: { ["a"] = 1 } -> { [string]: number }
//
// When an expected type is available, the package uses it to guide inference:
//
//	local x: Point = { x = 1, y = 2 }  -- inferred as Point
//
// # Function Literals
//
// Anonymous functions are typed based on their signature and body analysis:
//
//	local f = function(x: number): string ... end
//
// # Integration
//
// Literal types feed into assignment analysis and expression synthesis,
// providing base types that flow analysis refines through the CFG.
package literal

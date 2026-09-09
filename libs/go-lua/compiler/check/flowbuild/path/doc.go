// Package path constructs constraint paths from AST expressions.
//
// This package converts AST expressions into [constraint.Path] values that
// identify variables and field access chains for flow analysis. Paths are
// the primary way the flow solver tracks types through the program.
//
// # Path Structure
//
// A path consists of:
//   - Symbol: The base variable's symbol ID
//   - Fields: Optional field access chain (x.foo.bar -> ["foo", "bar"])
//
// # Expression Conversion
//
// The package converts expressions to paths:
//
//	x          -> Path{Symbol: sym(x)}
//	x.foo      -> Path{Symbol: sym(x), Fields: ["foo"]}
//	x.foo.bar  -> Path{Symbol: sym(x), Fields: ["foo", "bar"]}
//	x["key"]   -> Path{Symbol: sym(x), Fields: ["key"]} (for string keys)
//
// # Usage
//
// Paths are used to:
//   - Track type facts for variables and fields
//   - Apply type guards to specific access paths
//   - Correlate assignments with their targets
//
// # Binding Integration
//
// The package uses binding tables to resolve identifiers to symbol IDs,
// ensuring consistent symbol identity across the analysis.
package path

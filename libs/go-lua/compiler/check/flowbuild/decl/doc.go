// Package decl extracts type declarations from function parameters and locals.
//
// This package processes type annotations on function parameters and local
// declarations to establish the initial type environment for flow analysis.
//
// # Parameter Declarations
//
// For typed function parameters:
//
//	function foo(x: string, y: number?)
//
// The package emits declarations that bind parameter symbols to their
// annotated types at the function entry point.
//
// # Local Declarations
//
// For typed local variables:
//
//	local x: string = getValue()
//
// The package emits the declared type as a constraint that the assigned
// value must satisfy.
//
// # Self Parameter
//
// Method definitions with self parameters receive special handling:
//
//	function Obj:method()  -- self has type Obj
//
// # Integration
//
// Declarations become [flow.Assignment] records with the declared type
// as the assigned value, establishing type facts at declaration points.
package decl

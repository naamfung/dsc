// Package assign provides flow-sensitive assignment analysis for the type checker.
//
// This package extracts type information from assignments in the CFG and produces
// flow inputs for the constraint solver. It handles local declarations, field
// assignments, table literal inference, and type annotation validation.
//
// # Core Functions
//
// [CollectFieldAssignments] scans the graph for field assignments (x.foo = val)
// and groups them by base symbol, producing a map of field types per variable.
//
// [EmitAssignments] produces flow.Assignment records for each assignment target,
// converting CFG assignment info into solver-ready format.
//
// [InferTableType] performs bidirectional type inference for table literals,
// using expected type context to resolve ambiguous field types.
//
// # Type Annotation Handling
//
// When a local declaration has a type annotation:
//
//	local x: string = getValue()
//
// The package validates that the assigned value is compatible with the declared
// type and emits the annotation as the authoritative type for flow analysis.
//
// # Field Assignment Collection
//
// Field assignments are collected to support struct/record type inference:
//
//	local obj = {}
//	obj.name = "foo"  -- collected as {name: string}
//	obj.count = 1     -- collected as {count: number}
//
// The collector produces a map[SymbolID]map[string]Type that later phases use
// to infer table types from their field assignments.
package assign

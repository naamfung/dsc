// Package interproc handles interprocedural analysis after flow solving.
//
// This package processes analysis results after the flow solver converges,
// extracting interprocedural facts that propagate between functions.
//
// # Post-Flow Processing
//
// After flow analysis completes for a function, this package:
//   - Extracts return type summaries
//   - Computes parameter type hints from call sites
//   - Identifies captured variable assignments
//   - Propagates effect information
//
// # Fact Propagation
//
// Interprocedural facts enable:
//   - Return type inference across call boundaries
//   - Parameter type inference from argument types
//   - Captured variable type tracking in nested functions
//
// # Integration
//
// This package bridges per-function flow analysis with the global
// fixpoint iteration that resolves cross-function dependencies.
package interproc

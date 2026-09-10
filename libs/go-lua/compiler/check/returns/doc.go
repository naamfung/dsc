// Package returns provides interprocedural return type analysis.
//
// This package implements the fixpoint iteration for return type inference
// across mutually recursive function groups. It computes strongly connected
// components in the call graph and processes them in dependency order.
//
// # SCC-Based Analysis
//
// Functions are grouped into strongly connected components (SCCs):
//   - Non-recursive functions: analyzed once
//   - Mutually recursive groups: iterated until types stabilize
//
// [ComputeSymbolSCCs] uses Tarjan's algorithm to find SCCs in reverse
// topological order, ensuring callees are analyzed before callers.
//
// # Return Type Computation
//
// For each function:
//   - Collect return expressions from all return statements
//   - Synthesize types for return expressions
//   - Join multiple return types into a union
//   - Apply widening for recursive convergence
//
// # Type Widening
//
// To ensure termination in recursive cases, types are widened:
//   - After N iterations, recursive types are approximated
//   - Widening preserves soundness while ensuring convergence
//
// # Overlay System
//
// [Overlay] provides a mutable view over stable return summaries:
//   - Stable summaries from previous iterations
//   - Pending updates from current iteration
//   - Atomic commit when iteration converges
//
// # Call Graph Construction
//
// [BuildCallGraph] extracts the local function call graph from CFGs,
// identifying which functions call which others within the module.
//
// # Signature Inference
//
// [InferSignature] combines parameter hints and return types to produce
// complete function signatures for functions without annotations.
package returns

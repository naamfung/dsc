// Package effectops provides operations for function effect propagation.
//
// This package handles effect analysis for functions, computing the combined
// effects of a function by propagating effects through call chains. Effects
// include termination guarantees, IO markers, and type predicates.
//
// # Effect Propagation
//
// [Propagate] computes the complete effect for a function:
//   - Starts with the function's local effects
//   - Examines all call sites in the function
//   - Merges callee effects into the function's effect
//
// This enables effects like "may throw" or "performs IO" to propagate
// from callees to callers.
//
// # Termination Analysis
//
// [TerminatesFromReachability] determines if a function never returns:
//   - Checks if all return points are unreachable
//   - Uses flow analysis to determine reachability
//
// Functions that always call error() or similar are marked as terminating.
//
// # Effect Lookup
//
// [LookupRefinementBySym] resolves effects for called functions:
//   - First checks the effect store for analyzed functions
//   - Falls back to global type information for builtins
//   - Extracts effects from function type annotations
//
// # Export Enrichment
//
// [EnrichExportWithEffects] attaches effect information to exported types:
//   - Adds termination annotations to function types
//   - Preserves type guard effects on methods
//   - Ensures effects are visible to importers
package effects

// Package constraint provides multi-path narrowing constraints for type refinement.
//
// Constraints represent facts learned from conditionals and type guards that
// refine types along control flow paths. Unlike simple single-variable narrowing,
// constraints can relate multiple paths (e.g., "if x == y") enabling richer
// type-level reasoning.
//
// # Core Types
//
// [Path]: An SSA-stable identifier for a value (variable.field.index). Paths track
// identity through control flow using CFG symbol IDs, enabling precise narrowing
// even when the same variable name refers to different values at different points.
//
// [Constraint]: A narrowing term that refines types when satisfied. Constraints are
// AST-free and deterministic, making them suitable for serialization and caching.
//
// [Condition]: A DNF (disjunctive normal form) formula of constraint conjunctions,
// representing the complete narrowing information from a conditional expression.
//
// [Solver]: Applies constraints to type environments to produce narrowed types.
//
// [FunctionRefinement]: Describes type refinements a function produces on its parameters.
//
// [Interner]: Provides constraint interning to reduce allocations for common patterns.
//
// # Constraint Kinds
//
// Single-path constraints:
//   - [Truthy]/[Falsy]: Path evaluates to truthy/falsy value
//   - [IsNil]/[NotNil]: Path is/is not nil
//   - [HasType]/[NotHasType]: Path has/lacks a specific type
//   - [HasField]: Path has a specific field (narrows unions)
//
// Literal comparison constraints:
//   - [FieldEquals]/[FieldNotEquals]: path.field equals/not-equals a literal
//   - [IndexEquals]/[IndexNotEquals]: path[key] equals/not-equals a literal
//
// Multi-path constraints:
//   - [EqPath]/[NotEqPath]: Two paths are equal/not-equal
//   - [FieldEqualsPath]/[FieldNotEqualsPath]: path.field equals/not-equals another path
//   - [IndexEqualsPath]/[IndexNotEqualsPath]: path[key] equals/not-equals another path
//   - [KeyOf]: Key path is a known key of table path
//
// # Numeric Constraints
//
// The package provides numeric constraints for arithmetic reasoning via the
// [NumericConstraint] interface:
//   - [Le], [Lt], [Ge], [Gt]: Comparison constraints between paths
//   - [EqConst], [LeConst], [GeConst]: Path compared to constant
//   - [ModEq]: Modular equality (x % m == r)
//   - [LeLenOf]: Path bounded by array length
//
// # Generic Inference
//
// [InferSet] provides constraint-based type variable inference using bounds
// tracking and SCC-based unification (Tarjan's algorithm) for cyclic dependencies.
//
// # Example
//
// Creating and applying constraints:
//
//	// Create paths for variables
//	xPath := constraint.NewPath(xSym, "x")
//	yPath := constraint.NewPath(ySym, "y")
//
//	// Build a condition: x is not nil AND x.kind == "success"
//	cond := constraint.FromConstraints(
//	    constraint.NotNil{Path: xPath},
//	    constraint.FieldEquals{Target: xPath, Field: "kind", Value: typ.LiteralString("success")},
//	)
//
//	// Apply constraints to narrow types
//	solver := constraint.Solver{Env: env}
//	narrowed := solver.Apply(cond.AllConstraints(), baseTypes)
//
// # Thread Safety
//
// Constraint values are immutable and safe to use concurrently. The [Solver]
// is pure and deterministic. The [Interner] is thread-safe for concurrent use.
//
// # Sub-packages
//
// The [theory] sub-package provides modular SMT-style theory solvers:
//   - Difference Logic: x - y ≤ c constraints using Bellman-Ford
//   - Modular Arithmetic: x % n == k constraints
//   - E-Graph: Equality reasoning with congruence closure
package constraint

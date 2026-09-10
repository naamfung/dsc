// Package theory provides modular constraint solving theories for SMT-style reasoning.
//
// This package implements decision procedures for specific constraint domains
// (theories) that can be composed together. The architecture follows the DPLL(T)
// paradigm where a SAT solver coordinates with theory-specific solvers.
//
// # Theories Provided
//
// Difference Logic (difference.go):
// Solves constraints of the form x - y ≤ c where x, y are variables and c is
// a constant. Uses a weighted directed graph and Bellman-Ford shortest paths
// to detect unsatisfiability (negative cycles) and derive implied bounds.
// Used for array bounds checking and index arithmetic.
//
// Modular Arithmetic (modular.go):
// Handles constraints involving modulo operations like x % n == k.
// Used for filter predicates (e.g., "x % 2 == 0" for even numbers).
//
// E-Graph (egraph.go):
// Implements equality reasoning with congruence closure for path equality.
// Tracks equivalence classes and propagates type narrowings across equal paths.
//
// # Design Principles
//
//  1. Self-contained: This package has no dependencies on the parent predicate
//     package. The predicate package imports theory and translates its types.
//
//  2. Immutable results: Theory operations return new values rather than
//     mutating state where practical.
//
//  3. Sound but incomplete: Theories return Unknown when they cannot determine
//     satisfiability, never giving incorrect Valid/Invalid results.
//
//  4. Composable: Theories can share derived facts through a common Context.
package theory

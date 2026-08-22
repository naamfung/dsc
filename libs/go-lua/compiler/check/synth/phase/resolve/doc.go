// Package resolve performs final type resolution after synthesis.
//
// This package runs after flow analysis and narrowing to resolve
// final types for all expressions. It combines synthesized types
// with flow facts to produce the most precise types.
//
// # Resolution Process
//
// For each expression:
//  1. Get the synthesized type from the synth engine
//  2. Apply flow-sensitive narrowing if available
//  3. Resolve any remaining type variables
//  4. Store the final resolved type
//
// # Type Variable Resolution
//
// Unresolved type variables are resolved to their bounds:
//   - Upper bounds for covariant positions
//   - Lower bounds for contravariant positions
//   - Joined bounds for invariant positions
//
// # Integration
//
// Resolution is the final synthesis phase before diagnostic passes
// examine expression types for errors.
package resolve

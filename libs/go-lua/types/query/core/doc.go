// Package core provides pure type inspection, query, and lookup operations for
// the Lua type system. It serves as the foundational layer for type analysis,
// enabling field resolution, method lookup, operator type inference, and
// structural type traversal.
//
// # Architecture
//
// The package is organized around several concerns:
//
//   - Query Engine: Memoized type operations via [Engine] for performance-critical
//     paths. The engine caches results of expensive computations like field lookup
//     and subtype checks.
//
//   - Structural Operations: Pure functions for field ([Field]), index ([Index]),
//     and method ([Method]) resolution that work directly on type structures.
//
//   - Operator Resolution: Type inference for unary ([UnaryOp]) and binary
//     ([BinaryOp]) operators, including Lua metamethod fallback.
//
//   - Generic Instantiation: Substitution of type parameters in generic types
//     via [InstantiateGeneric] and [Substitute].
//
//   - Cycle Analysis: Detection of potentially cyclic types for memory management
//     optimization via [CanFormCycle] and [GetObjectClass].
//
//   - Code Generation Hints: Dispatch strategy ([GetDispatch]) and memory layout
//     ([GetLayout]) analysis for compiler optimization.
//
// # Engine vs Pure Functions
//
// Most operations exist in two forms:
//
//  1. Engine methods (e.g., Engine.Field) provide memoization and should be used
//     in performance-critical paths like type checking loops.
//
//  2. Package-level functions (e.g., Field) are pure and stateless, suitable for
//     one-off queries or contexts where an engine is unavailable.
//
// # Type Traversal
//
// The package handles Lua's complex type system including:
//   - Union types (A | B): field must exist in ALL members
//   - Intersection types (A & B): field from ANY member
//   - Optional types (T?): propagates optionality to results
//   - Alias types: transparent wrappers resolved automatically
//   - Instantiated generics: resolved on demand
//
// # Lua Semantics
//
// Operations respect Lua's runtime semantics:
//   - Map/record field access returns Optional since keys may be absent
//   - Metamethods (__index, __call, __add, etc.) are checked as fallbacks
//   - String methods are resolved through stdlib method providers
//   - Logical operators (and/or) return operand types, not boolean
//
// # Thread Safety
//
// Engine instances are safe for concurrent use. Pure functions have no shared
// state. Query results are immutable types.
package core

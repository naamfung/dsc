// Package phase implements individual analysis phases for type checking.
//
// This package contains the core phases that process a function during
// type checking. Each phase has specific responsibilities and runs in
// a defined order.
//
// # Phase Order
//
// Phases execute in sequence:
//  1. Scope: Build lexical scope information
//  2. Types: Resolve type annotations to concrete types
//  3. Flow: Run dataflow analysis for type narrowing
//  4. Narrow: Apply narrowed types to expressions
//  5. Resolve: Final type resolution for all expressions
//
// # Phase Interface
//
// Each phase receives:
//   - The function being analyzed
//   - Results from previous phases
//   - The analysis session for queries and storage
//
// Phases produce results that subsequent phases consume.
//
// # Flow Phase
//
// The flow phase is the most complex, running the constraint solver
// to propagate types through the CFG. It produces type facts for
// each program point.
//
// # Integration
//
// Phases are orchestrated by the driver package, which manages
// dependencies and iteration for interprocedural analysis.
package phase

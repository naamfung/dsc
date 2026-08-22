// Package scope provides lexical scope state for type checking.
//
// This package maintains the scope state at each CFG point, tracking
// which variables are visible and their symbol IDs. Scope states are
// used by the type synthesizer to resolve identifiers.
//
// # Scope State
//
// [State] represents the scope at a specific CFG point:
//   - Parent scope reference (for nested functions)
//   - Visible symbols and their types
//   - Global symbol access
//
// # Scope Construction
//
// Scopes are built during CFG construction and refined during
// type checking. The scope at each point reflects:
//   - Function parameters
//   - Local variable declarations up to that point
//   - Captured variables from enclosing scopes
//
// # Symbol Resolution
//
// The package provides methods to:
//   - Look up symbols by name
//   - Determine if a symbol is local or global
//   - Access parent scope for captured variables
//
// # Integration
//
// Scope states are stored per CFG point and accessed by the
// type synthesizer when evaluating expressions.
package scope

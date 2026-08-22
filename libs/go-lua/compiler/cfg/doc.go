// Package cfg constructs control flow graphs from Lua AST nodes.
//
// This package transforms parsed Lua code into a graph representation suitable
// for dataflow analysis. The CFG captures control flow through branches, loops,
// and function calls while preserving variable scope and SSA versioning.
//
// # Core Types
//
// [Graph] is the primary type, holding:
//   - The underlying CFG structure with entry/exit points
//   - NodeInfo for each CFG point (assignments, calls, branches, returns)
//   - SSA versioning for all variables
//   - Scope visibility maps for lexical scoping
//   - Nested function references for hierarchical analysis
//
// [Builder] constructs graphs incrementally from AST traversal. It handles:
//   - Creating CFG nodes and edges
//   - Tracking scope entry/exit
//   - Computing SSA versions via dominance frontiers
//   - Extracting nested function definitions
//
// [ScopeTracker] maintains lexical scope during construction:
//   - Symbol registration at declaration points
//   - Visibility snapshots at each CFG point
//   - Global vs local symbol distinction
//
// # Node Information Types
//
// Each CFG node carries semantic information via the [NodeInfo] interface:
//
//   - [AssignInfo]: Local/global assignments with targets and sources
//   - [CallInfo]: Function calls with callee, arguments, and receiver
//   - [ReturnInfo]: Return statements with expressions
//   - [BranchInfo]: Conditional branches with condition analysis
//   - [FuncDefInfo]: Function definitions (global, field, method)
//   - [TypeDefInfo]: Type alias definitions
//
// # SSA Versioning
//
// The package computes SSA (Static Single Assignment) form using the
// Cytron et al. dominance-frontier algorithm:
//
//  1. Collect all assigned symbols across the function
//  2. Compute dominance frontiers via immediate dominators
//  3. Place phi nodes at iterated dominance frontiers
//  4. Rename variables during dominator-tree traversal
//
// SSA versioning enables precise dataflow tracking through control flow joins.
// The [Version] type identifies a specific definition of a symbol, and
// [PhiInfo] represents join points where multiple definitions merge.
//
// # Symbol Resolution
//
// Symbols are bound during a separate binder pass that runs before CFG
// construction. The [Builder] receives a pre-populated [bind.BindingTable]
// mapping AST identifiers to unique SymbolIDs. This ensures consistent
// symbol identity across the analysis.
//
// # Usage
//
// Typical usage involves calling [Build] or [BuildWithBindings]:
//
//	// Simple case: build with automatic binding
//	graph := cfg.Build(funcExpr, "print", "error")
//
//	// Module case: share bindings across functions
//	bindings := bind.Bind(moduleFunc, globalNames...)
//	graph := cfg.BuildWithBindings(funcExpr, bindings)
//
// The resulting graph provides iteration methods for processing nodes:
//
//	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
//	    // Process assignment at point p
//	})
//
//	graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
//	    // Process call statements at point p
//	})
//
//	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
//	    // Process all callsites, including assignment/return call expressions
//	})
//
// # Nested Functions
//
// Nested function definitions are collected during CFG construction and
// accessible via [Graph.NestedFunctions]. Each [NestedFunc] contains
// the definition point, AST node, and assigned symbol. Callers typically
// build separate graphs for nested functions and link them via symbol IDs.
package cfg

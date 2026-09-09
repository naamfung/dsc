// Package api defines interfaces and types for the Lua type checker.
//
// This package provides the contract between different phases of type analysis.
// It defines store interfaces for accessing analysis results, session interfaces
// for managing analysis state, and data types for interprocedural facts.
//
// # Store Interfaces
//
// The package defines a hierarchy of store interfaces with increasing capability:
//
//   - [ModuleStore]: Module-level bindings and alias maps
//   - [GraphStore]: Access to built CFGs by ID
//   - [ParentScopes]: Parent scope lookup for nested functions
//   - [SnapshotStore]: Stable interprocedural fact snapshots
//   - [StoreView]: Read-only view combining all above
//   - [IterationStore]: Adds mutation for fixpoint iteration
//
// These interfaces allow different phases to declare their dependencies and
// enable testing with mock implementations.
//
// # Analysis Session
//
// [AnalysisSession] is the minimal interface for the fixpoint driver. It
// provides access to the query context, CFG construction, result storage,
// and diagnostic collection. The concrete implementation lives in the
// check package.
//
// # Interprocedural Facts
//
// The [Facts] type bundles interprocedural analysis results for a single
// function graph:
//
//   - [FunctionFacts]: Canonical per-function return/signature facts
//   - [ReturnSummaries]: Inferred return types by function symbol
//   - [NarrowReturnSummaries]: Post-narrowing return types
//   - [ParamHints]: Parameter types inferred from call sites
//   - [FuncTypes]: Canonical types for local function symbols
//   - [LiteralSigs]: Signatures for anonymous function literals
//   - [CapturedFieldAssigns]: Field assignments to captured variables
//
// Facts are computed incrementally and stored per (graph, parent-scope) pair.
// The [GraphKey] type provides the canonical key for this lookup.
//
// # Function References
//
// [FunctionRef] maps function symbols to their defining context:
//
//   - Symbol ID for identity
//   - Graph ID for CFG lookup
//   - Parent graph ID for nested functions
//   - Definition point within the parent
//
// The [FunctionRefs] interface provides bidirectional lookup between symbols,
// AST nodes, and CFG graphs.
//
// # Analysis Phases
//
// The [Phase] type identifies the current analysis phase, enabling phase-aware
// queries and diagnostics. Phases progress from AST construction through
// flow analysis and back-propagation.
//
// # Synthesis Interface
//
// [BaseSynth] provides the type synthesis interface for expression analysis.
// It is implemented by the synth.Engine and used by phase runners to
// evaluate expressions in context.
package api

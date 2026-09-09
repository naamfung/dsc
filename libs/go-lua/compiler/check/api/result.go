package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// FuncResult holds the complete analysis output for a single function.
// It is pure data with no back-references to Session or other FuncResults,
// enabling safe memoization and independent storage.
//
// The result captures outputs from all analysis phases:
//   - Phase A (Resolve): Type annotations resolved into Scopes
//   - Phase B (Scope/Extract): BaseScope, Scopes, FlowInputs
//   - Phase C (Solve): FlowSolution
//   - Phase D (Narrow): Facts, FnRefinement, NarrowSynth
type FuncResult struct {
	// Graph is the function's control flow graph containing CFG nodes,
	// binding information, and iteration metadata.
	Graph *cfg.Graph

	// ModuleBindings is the module-level binding table used as fallback when
	// graph-local bindings are insufficient for canonical symbol resolution.
	ModuleBindings *bind.BindingTable

	// BaseScope is the function's entry scope containing parameters,
	// type definitions, and inherited context from the parent scope.
	BaseScope *scope.State

	// Scopes maps each CFG point to its effective scope state.
	// Includes declared types, narrowed types, and lexical bindings.
	Scopes map[cfg.Point]*scope.State

	// Facts provides effective type information for symbols at each CFG point.
	// This is the primary interface for querying narrowed types during synthesis.
	Facts flow.TypeFacts

	// FlowInputs contains the extracted flow constraints from Phase B.
	// Includes assignments, conditions, type guards, and dead point markers.
	FlowInputs *flow.Inputs

	// FlowSolution contains the solved constraint system from Phase C.
	// Provides reachability conditions and exclusion facts for narrowing.
	FlowSolution *flow.Solution

	// FnRefinement captures the function's inferred refinement summary.
	// It includes propagated effect rows and branch-specific narrowing facts.
	FnRefinement *constraint.FunctionRefinement

	// NarrowSynth is the narrowed-phase synthesis engine for this function.
	// Use TypeOf to query expression types with flow-based narrowing applied.
	NarrowSynth Synth
	// LiteralSignatures holds synthesized signatures for function literals in this graph.
	LiteralSignatures map[*ast.FunctionExpr]*typ.Function

	// Extras stores results from registered ComputePass cores.
	// Keyed by ComputePass.Name().
	Extras map[string]any

	// DepthLimitExceeded indicates scope depth limit was hit during scope phase.
	DepthLimitExceeded bool
}

// EffectiveTypeAt returns the narrowed type for a symbol at a specific CFG point.
// The effective type reflects flow-based narrowing (type guards, nil checks, etc.)
// applied to the symbol's declared type based on control flow leading to point p.
//
// Returns TypedValue with:
//   - Type: The narrowed type at point p
//   - State: StateResolved if type is known, StateUnknown otherwise
//
// Delegates to Facts.EffectiveTypeAt when available, otherwise returns unknown.
func (r *FuncResult) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if r == nil || r.Facts == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return r.Facts.EffectiveTypeAt(p, sym)
}

// NarrowedTypeAt returns the precise path-sensitive narrowed type at a CFG point.
func (r *FuncResult) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if r == nil || r.FlowSolution == nil {
		return nil
	}
	return r.FlowSolution.NarrowedTypeAt(p, path)
}

// ExcludesTypeAt checks if the flow solution proves a type is excluded at a CFG point.
// Used for type narrowing when control flow eliminates certain type possibilities.
//
// For example, after "if x ~= nil then", the nil type is excluded from x's type
// in the then-branch. Similarly, "if type(x) == 'string'" excludes non-string types.
//
// Parameters:
//   - p: CFG point where exclusion is checked
//   - path: Symbol path (may include field accesses)
//   - declared: The declared type being queried
//
// Returns true if flow analysis proves the declared type cannot occur at point p.
func (r *FuncResult) ExcludesTypeAt(p cfg.Point, path constraint.Path, declared typ.Type) bool {
	if r == nil || r.FlowSolution == nil {
		return false
	}
	return r.FlowSolution.ExcludesTypeAt(p, path, declared)
}

// FuncResultView is the minimal view of a function analysis result
// required by nested processing and interprocedural helpers.
type FuncResultView struct {
	Graph        *cfg.Graph
	Scopes       map[cfg.Point]*scope.State
	Facts        flow.TypeFacts
	FlowSolution *flow.Solution
	NarrowSynth  Synth
}

// ViewFromResult constructs a minimal view from a full function result.
func ViewFromResult(r *FuncResult) *FuncResultView {
	if r == nil {
		return nil
	}
	return &FuncResultView{
		Graph:        r.Graph,
		Scopes:       r.Scopes,
		Facts:        r.Facts,
		FlowSolution: r.FlowSolution,
		NarrowSynth:  r.NarrowSynth,
	}
}

// Package returns provides return type inference for nested Lua functions.
//
// This package implements the return type inference pass that computes return
// types for locally-defined functions. Since Lua allows omitting return type
// annotations, the type checker must infer return types from the function body.
// This is complicated by mutual recursion: functions in the same scope can call
// each other, creating circular type dependencies.
//
// # SCC-Based Inference
//
// Return type inference uses Strongly Connected Component (SCC) decomposition.
// Functions are grouped into SCCs based on their call relationships. Within
// an SCC, functions are processed together using fixpoint iteration until
// return types stabilize.
//
// # Return Summaries
//
// A return summary is a vector of types representing the types returned by
// a function. For `return a, b, c`, the summary would be [typeof(a), typeof(b),
// typeof(c)]. Summaries are accumulated across all return statements in a
// function body and joined to produce the final return type.
//
// # Canonical vs Seed Summaries
//
// Two summary stores are maintained:
//   - Canonical: Fully computed return types from completed analysis
//   - Seed: Provisional return types from the current iteration
//
// During analysis, seed summaries are used for functions in the current SCC
// (to avoid circular dependence), while canonical summaries are used for
// functions outside the SCC (whose types are already known).
//
// # Parameter Hint Propagation
//
// For unannotated parameters, the system propagates type hints from call sites.
// If function `f` is called as `f(42)`, the first parameter of `f` is hinted
// as `number`. Hints are joined across all call sites and propagated through
// the call graph until fixpoint.
package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// LocalFuncInfo holds information about a local function for SCC computation.
//
// Each LocalFuncInfo represents a function that may participate in mutual
// recursion with other local functions. The info includes the function's
// AST, CFG, definition context, and any parameter hints inferred from
// call sites.
type LocalFuncInfo struct {
	Sym      cfg.SymbolID
	Fn       *ast.FunctionExpr
	DefScope *scope.State
	Graph    *cfg.Graph
	// ParentGraph is the graph where this local function is defined.
	// Used for parent-scope callsite hint propagation.
	ParentGraph *cfg.Graph
	ParentFn    *ast.FunctionExpr
	DefPoint    cfg.Point
	// ParamHints holds inferred parameter types from call sites in the parent graph.
	// Index corresponds to parameter position.
	ParamHints []typ.Type
}

// MaxReturnSummaryIterations limits fixpoint iterations for ReturnSummaries.
// Exceeding this indicates a bug (non-monotonic merge) or pathological recursion.
const MaxReturnSummaryIterations = 10

// Package check implements a multi-phase, fixpoint-iterative type checking system for Lua.
//
// # ARCHITECTURE OVERVIEW
//
// The checker performs interprocedural type analysis through a fixpoint iteration loop
// that processes all functions until inter-function information stabilizes. Interproc
// facts are produced during analysis and captured in a stable snapshot at iteration
// boundaries. Effect facts are accumulated during analysis and swapped into a stable
// snapshot at the boundary alongside constructor field facts.
//
// # PHASE PIPELINE
//
// Each function is analyzed through a five-phase pipeline:
//
//	Phase A (Resolve): Resolves type annotations from AST nodes into concrete types.
//	This processes @type, @param, @return annotations and user-defined type aliases.
//
//	Phase B (Scope): Builds lexical scope states for each CFG point, populating
//	declared types from annotations and synthesizing function literal signatures.
//	Also extracts flow constraints from control flow and assignments.
//
//	Phase C (Solve): Solves the extracted flow constraint system to compute
//	reachability conditions and type narrowing facts at each CFG point.
//
//	Phase D (Narrow): Applies flow solution to narrow declared types, computing
//	final effective types for all expressions and generating function refinements.
//
// # INTERPROCEDURAL ANALYSIS
//
// The checker supports interprocedural analysis through a unified interproc snapshot:
//
//   - ReturnSummaries: Inferred return types for local functions
//   - ParamHints: Inferred parameter types from call sites
//   - FuncTypes: Canonical local function types for sibling lookups
//   - LiteralSigs: Synthesized signatures for function literals
//   - Refinements: Function refinement summaries, stored per symbol
//
// # DETERMINISTIC ORDERING
//
// All analysis is performed in deterministic order:
//   - Root function first, then nested functions in CFG point order
//   - Scope groups sorted by earliest definition point
//   - Functions within scope groups sorted by source position
//
// # MEMOIZATION
//
// Function analysis results are memoized by (GraphID, ParentHash, StoreRevision).
// The memoization cache is cleared at each iteration boundary to force recomputation
// with updated inter-function summaries.
//
// # CONVERGENCE
//
// The fixpoint loop terminates when interproc facts and effect/constructor snapshots
// stabilize. Maximum iteration count is bounded to detect non-convergent analysis.
package check

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/pipeline"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Pass is a diagnostic generation function that runs after the fixpoint converges.
// Passes inspect the fully analyzed function results and emit diagnostics for type
// errors, unused variables, unreachable code, or other semantic issues.
//
// Passes are registered via WithPass and execute in registration order. Each pass
// receives the analysis session, the function AST node, and the complete analysis
// result including narrowed types, flow facts, and effects.
//
// Passes run only after all iterations complete, so they see the final converged
// type information. They should not modify session state.
type Pass func(sess *Session, fn *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic

// Deps contains required dependencies injected into the Checker at construction time.
// These dependencies provide external type information and resolution capabilities
// that the checker cannot derive from the source code alone.
type Deps struct {
	// Types provides type construction and manipulation operations. Used for
	// building function types, records, unions, and other composite types
	// during type synthesis and constraint solving.
	Types core.TypeOps

	// Stdlib provides the type namespace for standard library type aliases.
	// This scope contains type definitions like "table", "string", "number"
	// that are available in all Lua programs without explicit import.
	// Note: This is type namespace only, not the runtime value namespace.
	Stdlib *scope.State

	// GlobalTypes provides the value namespace for built-in global functions
	// like print, pairs, ipairs, type, assert, error, etc. These are runtime
	// values with known types, not type definitions.
	GlobalTypes map[string]typ.Type

	// Resolver provides constraint resolution for the flow solver (Phase C).
	// It implements type narrowing operations like type guards, nil checks,
	// and assertion-based refinements.
	Resolver narrow.Resolver
}

// Option configures optional behaviors of a Checker.
type Option func(*Checker)

// WithPass registers an analysis pass.
func WithPass(p Pass) Option {
	return func(c *Checker) { c.passes = append(c.passes, p) }
}

// WithComputePass registers a compute pass for additional analysis.
func WithComputePass(p api.ComputePass) Option {
	return func(c *Checker) { c.computePasses = append(c.computePasses, p) }
}

// Checker orchestrates the multi-phase type analysis pipeline for Lua modules.
// It is the main entry point for type checking and coordinates the fixpoint
// iteration loop that processes all functions until types stabilize.
//
// DESIGN PRINCIPLE: Checker is stateless with respect to individual analysis runs.
// All per-run state (function results, inter-function channels, diagnostics) lives
// in Session and SessionStore. This allows a single Checker instance to be reused
// for analyzing multiple files in sequence or parallel.
//
// MEMOIZATION: Function analysis is memoized through funcResultQ keyed by FuncKey.
// The cache is cleared at each fixpoint iteration boundary to ensure fresh
// computation with updated inter-function summaries.
//
// EXTENSION POINTS: Checker supports two extension mechanisms:
//   - Pass: Diagnostic generators that run after fixpoint convergence
//   - ComputePass: Analysis phases that run during iteration and store results in Extras
type Checker struct {
	db                        *db.DB
	deps                      Deps
	passes                    []Pass
	computePasses             []api.ComputePass
	maxIterations             int
	maxScopeDepth             int
	emitScopeDepthDiagnostics bool
}

// NewChecker creates a new Checker instance with the given database, dependencies, and options.
//
// The database provides query infrastructure for memoization and type caching. Dependencies
// supply external type information (stdlib types, global functions, type operations).
// Options configure analysis passes and other behaviors.
//
// The returned Checker is safe to reuse for multiple Check calls. Each call creates
// an independent Session with its own fixpoint iteration state.
//
// Example:
//
//	db := db.NewDB()
//	deps := check.Deps{
//	    Types:       typeOps,
//	    Stdlib:      stdlibScope,
//	    GlobalTypes: builtinTypes,
//	    Resolver:    narrowResolver,
//	}
//	checker := check.NewChecker(db, deps, check.WithPass(fieldCheckPass))
//	sess := checker.Check(source, "module.lua")
func NewChecker(database *db.DB, deps Deps, opts ...Option) *Checker {
	c := &Checker{
		db:            database,
		deps:          deps,
		maxIterations: 10,
		maxScopeDepth: 0,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Checker) newPipeline() *pipeline.Driver {
	runner := pipeline.NewRunner(pipeline.RunnerConfig{
		Types:         c.deps.Types,
		GlobalTypes:   c.deps.GlobalTypes,
		Stdlib:        c.deps.Stdlib,
		Manifests:     c.db,
		Resolver:      c.deps.Resolver,
		MaxScopeDepth: c.maxScopeDepth,
		ComputePasses: c.computePasses,
	})
	funcResultQ := db.NewQuery("FuncResult", runner.Run, funcResultEqual)
	return pipeline.New(pipeline.Config{
		Types:         c.deps.Types,
		GlobalTypes:   c.deps.GlobalTypes,
		Stdlib:        c.deps.Stdlib,
		Manifests:     c.db,
		MaxIterations: c.maxIterations,
		MaxScopeDepth: c.maxScopeDepth,
		EmitScopeDiag: c.emitScopeDepthDiagnostics,
		FuncResultQ:   funcResultQ,
	})
}

// WithMaxIterations configures the maximum number of fixpoint iterations.
// Values less than 1 are clamped to 1.
func WithMaxIterations(n int) Option {
	return func(c *Checker) {
		if n < 1 {
			n = 1
		}
		c.maxIterations = n
	}
}

// WithMaxScopeDepth configures a maximum lexical scope nesting depth.
// A value <= 0 disables the limit.
func WithMaxScopeDepth(n int) Option {
	return func(c *Checker) {
		if n <= 0 {
			n = 0
		}
		c.maxScopeDepth = n
	}
}

// WithScopeDepthDiagnostics enables diagnostics when the scope depth limit is exceeded.
func WithScopeDepthDiagnostics(enabled bool) Option {
	return func(c *Checker) {
		c.emitScopeDepthDiagnostics = enabled
	}
}

// Check parses and analyzes Lua source code, returning a Session containing all results.
//
// The method performs the complete analysis pipeline:
//  1. Parse source into AST
//  2. Build module bindings and collect module aliases
//  3. Run fixpoint loop until inter-function types stabilize
//  4. Execute diagnostic passes
//  5. Sort and deduplicate diagnostics
//
// Parse errors are captured as diagnostics in the returned Session. Even with parse
// errors, the Session is valid and contains the error information.
//
// The name parameter identifies the source file for diagnostic reporting and
// module path resolution.
//
// The returned Session contains:
//   - Results: Per-function analysis results (types, flow facts, effects)
//   - Diagnostics: Type errors, warnings, and suggestions
//   - Store: Inter-function channel data for advanced introspection
func (c *Checker) Check(source, name string) *Session {
	ctx := db.NewQueryContext(c.db)
	sess := New(ctx, name)
	// Ensure each top-level Check starts from clean inter-function channel state.
	// These are iteration-stable caches and must not persist across separate runs.
	if sess.Store != nil {
		sess.Store.ClearIterationChannels()
	}

	chunk, err := parse.ParseString(source, name)
	if err != nil {
		pos := diag.Position{}
		span := diag.Span{}
		if perr, ok := err.(*parse.Error); ok && perr != nil {
			file := name
			if perr.Pos.Source != "" {
				file = perr.Pos.Source
			}
			pos = diag.Position{File: file, Line: perr.Pos.Line, Column: perr.Pos.Column}
			endLine := perr.Pos.EndLine
			endCol := perr.Pos.EndColumn
			if endLine == 0 {
				endLine = perr.Pos.Line
			}
			if endCol == 0 {
				endCol = perr.Pos.Column
			}
			span = diag.Span{StartLine: perr.Pos.Line, StartCol: perr.Pos.Column, EndLine: endLine, EndCol: endCol}
		}
		sess.Diagnostics = append(sess.Diagnostics, diag.Diagnostic{
			Position: pos,
			Span:     span,
			Message:  err.Error(),
		})
		return sess
	}

	c.checkChunk(sess, chunk)
	return sess
}

// CheckChunk analyzes a pre-parsed AST chunk, returning a Session with results.
// Use this method when the AST is already available (e.g., from a custom parser
// or AST transformation pipeline).
//
// The chunk is wrapped in a synthetic FunctionExpr representing the module body.
// Analysis proceeds identically to Check: binding, CFG construction, fixpoint
// iteration, and diagnostic generation.
func (c *Checker) CheckChunk(chunk []ast.Stmt, name string) *Session {
	ctx := db.NewQueryContext(c.db)
	sess := New(ctx, name)
	// Attach store accessor and compute context for interproc queries
	if sess.Store != nil {
		sess.Store.ClearIterationChannels()
	}
	c.checkChunk(sess, chunk)
	return sess
}

// checkChunk is the internal implementation for Check and CheckChunk.
// It wraps the chunk in a FunctionExpr, runs the fixpoint loop, and executes passes.
func (c *Checker) checkChunk(sess *Session, chunk []ast.Stmt) {
	if p := c.newPipeline(); p != nil {
		p.Run(sess, chunk)
	}

	// Run passes after fixpoint converges
	c.runPasses(sess)
	pipeline.SortDiagnostics(sess.Diagnostics)
}

// runPasses executes registered passes after fixpoint converges.
func (c *Checker) runPasses(sess *Session) {
	for _, fn := range pipeline.SortedResultFunctions(sess.Results) {
		result := sess.Results[fn]
		for _, p := range c.passes {
			diags := p(sess, fn, result)
			sess.Diagnostics = append(sess.Diagnostics, diags...)
		}

		// Emit widening diagnostics for preflow inference precision loss
		sess.Diagnostics = append(sess.Diagnostics, pipeline.WideningDiagnostics(sess.SourceName, fn, result)...)
	}
}

func funcResultEqual(a, b *api.FuncResult) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Graph != nil && b.Graph != nil {
		return a.Graph.ID() == b.Graph.ID()
	}
	return false
}

// ClearCache removes all memoized function analysis results from the query cache.
//
// Call this between Check calls when analyzing unrelated files to prevent stale
// cache entries and reduce memory usage. The cache is automatically cleared at
// each fixpoint iteration boundary, so this is primarily useful for batch
// processing scenarios where the checker analyzes many independent files.
func (c *Checker) ClearCache() {
	// Function-result memoization is session-local and discarded at the end of Check.
	// Kept for API compatibility.
}

// Database returns the checker's type database for connecting external manifests.
func (c *Checker) Database() *db.DB {
	return c.db
}

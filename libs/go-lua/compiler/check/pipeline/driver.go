// Package driver provides the fixpoint iteration loop for type analysis.
//
// The driver orchestrates the multi-phase type checking process:
//
//  1. Initialize module bindings and CFG hierarchy
//  2. Run pre-flow return type inference for local functions
//  3. Execute the memoized function analysis pipeline
//  4. Propagate effects and interprocedural facts
//  5. Process nested functions recursively
//  6. Repeat until fixpoint (no channel changes) or max iterations
//
// The driver coordinates several inference subsystems:
//   - Return inference: Computes return types for local functions
//   - Nested inference: Processes nested function bodies with parent context
//   - Effect propagation: Tracks side effects through call chains
//   - Interproc facts: Shares type information across function boundaries
package pipeline

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/effects"
	interprocinfer "github.com/wippyai/go-lua/compiler/check/infer/interproc"
	nestedinfer "github.com/wippyai/go-lua/compiler/check/infer/nested"
	returninfer "github.com/wippyai/go-lua/compiler/check/infer/return"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Config supplies dependencies for the fixpoint driver.
type Config struct {
	Types         core.TypeOps
	GlobalTypes   map[string]typ.Type
	Stdlib        *scope.State
	Manifests     *db.DB
	MaxIterations int
	MaxScopeDepth int
	EmitScopeDiag bool
	FuncResultQ   *db.Query[api.FuncKey, *api.FuncResult]
}

// Driver executes the fixpoint loop and function analysis.
type Driver struct {
	cfg Config
}

// New creates a driver with the provided configuration.
func New(cfg Config) *Driver {
	return &Driver{cfg: cfg}
}

// Run analyzes a module chunk using the fixpoint loop.
func (d *Driver) Run(sess api.AnalysisSession, chunk []ast.Stmt) {
	if sess == nil {
		return
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}
	sess.SetRootFuncNode(fn)

	store := sess.StoreHandle()
	if store != nil {
		globals := collectGlobalNames(d.cfg.GlobalTypes)
		store.SetModuleBindings(bind.Bind(fn, globals))
	}

	chunkGraph := sess.GetOrBuildCFG(fn)
	if chunkGraph != nil {
		sess.RegisterGraphHierarchy(chunkGraph)
		if store != nil {
			store.SetModuleAliases(modules.CollectAliases(chunkGraph))
			if d.cfg.Stdlib != nil {
				store.SetGraphParentHash(chunkGraph.ID(), d.cfg.Stdlib.Hash())
			}
		}
	}

	d.runFixpoint(sess, fn, d.cfg.Stdlib)
}

func (d *Driver) runFixpoint(sess api.AnalysisSession, fn *ast.FunctionExpr, parent *scope.State) {
	maxIterations := d.cfg.MaxIterations
	if maxIterations < 1 {
		maxIterations = 1
	}

	converged := false
	for iter := 0; iter < maxIterations; iter++ {
		d.prepareIterationState(sess)
		d.checkFunctionFixpoint(sess, fn, parent)
		if d.advanceFixpoint(sess.StoreHandle()) {
			converged = true
			break
		}
	}

	if !converged {
		store := sess.StoreHandle()
		diffs := []string(nil)
		if store != nil {
			diffs = store.FixpointChannelDiffs()
		}
		msg := "inter-function fixpoint did not converge"
		if len(diffs) > 0 {
			msg += "; unstable channels: " + fmt.Sprintf("%v", diffs)
		}
		sess.AppendDiagnostics(diag.Diagnostic{
			Position: diag.Position{File: sess.Source()},
			Severity: diag.SeverityWarning,
			Message:  msg,
		})
	}
}

func (d *Driver) prepareIterationState(sess api.AnalysisSession) {
	if d.cfg.FuncResultQ != nil {
		d.cfg.FuncResultQ.Clear()
	}
	sess.ResetDiagnostics()

	scopeState := sess.ScopeDepthDiagState()
	for k := range scopeState {
		delete(scopeState, k)
	}
}

func (d *Driver) advanceFixpoint(store api.IterationStore) bool {
	if store == nil {
		return true
	}
	if !store.FixpointSwap() {
		return true
	}
	store.BumpRevision()
	return false
}

func (d *Driver) checkFunctionFixpoint(sess api.AnalysisSession, fn *ast.FunctionExpr, parent *scope.State) {
	graph := sess.GetOrBuildCFG(fn)
	if graph == nil {
		return
	}

	store := sess.StoreHandle()
	parentHash := d.registerParentScope(store, graph.ID(), parent)

	d.runReturnInference(sess, graph, parent, store)

	result := d.loadFunctionResult(sess, graph.ID(), parentHash, store)
	if result == nil {
		return
	}

	results := sess.ResultsMap()
	d.recordFunctionResult(sess, fn, result, results)
	d.emitScopeDepthDiagnostic(sess, fn, result)

	funcSym := cfg.SymbolID(0)
	if store != nil {
		if sym, ok := store.SymbolForFunc(fn); ok {
			funcSym = sym
		}
	}
	d.storeFunctionRefinement(store, result, funcSym)
	interprocinfer.StoreFactsFromResult(store, fn, result, parent)
	d.processNestedFunctions(sess, store, graph, results, result)
}

func (d *Driver) processNestedFunctions(
	sess api.AnalysisSession,
	store api.IterationStore,
	graph *cfg.Graph,
	results map[*ast.FunctionExpr]*api.FuncResult,
	result *api.FuncResult,
) {
	nestedProc := nestedinfer.New(nestedinfer.Config{
		Stdlib: d.cfg.Stdlib,
		Store:  store,
		Graphs: sess,
		Check: func(fn *ast.FunctionExpr, parent *scope.State) {
			d.checkFunctionFixpoint(sess, fn, parent)
		},
		ResultForFunc: func(fn *ast.FunctionExpr) *api.FuncResultView {
			if results == nil {
				return nil
			}
			return api.ViewFromResult(results[fn])
		},
		RootResult: api.ViewFromResult(sess.RootResultValue()),
	})
	nestedProc.ProcessNestedFunctions(graph, api.ViewFromResult(result))
}

func (d *Driver) registerParentScope(store api.IterationStore, graphID uint64, parent *scope.State) uint64 {
	parentHash := api.ParentHashForGraph(store, graphID, parent)
	if store != nil && parentHash != 0 {
		store.SetParentScope(parentHash, parent)
	}
	return parentHash
}

func (d *Driver) runReturnInference(
	sess api.AnalysisSession,
	graph *cfg.Graph,
	parent *scope.State,
	store api.IterationStore,
) {
	if store == nil || graph == nil {
		return
	}

	inferencer := returninfer.New(returninfer.Config{
		Types:         d.cfg.Types,
		GlobalTypes:   d.cfg.GlobalTypes,
		Manifests:     d.cfg.Manifests,
		Stdlib:        d.cfg.Stdlib,
		Store:         store,
		Graphs:        sess,
		SourceName:    sess.Source(),
		MaxIterations: returns.MaxReturnSummaryIterations,
	})

	var refinementLookup constraint.RefinementLookupBySym
	if es := store.RefinementStore(); es != nil {
		refinementLookup = es.LookupRefinementBySym
	}

	summaries, funcTypes, diags := inferencer.ComputeForGraph(returninfer.RunContext{
		Ctx:          sess.Context(),
		ParentFacts:  d.parentFactsForGraph(sess, store, graph.ID()),
		EffectLookup: refinementLookup,
	}, graph, parent)
	if len(diags) > 0 {
		sess.AppendDiagnostics(diags...)
	}
	if len(summaries) == 0 {
		return
	}
	if key, ok := store.GraphKeyFor(graph, parent); ok {
		store.UpdateInterprocFactsNext(key, func(facts *api.Facts) {
			returns.MergeFunctionFactsIntoFacts(facts, summaries, nil, funcTypes)
		})
	}
}

func (d *Driver) parentFactsForGraph(
	sess api.AnalysisSession,
	store api.IterationStore,
	graphID uint64,
) flow.TypeFacts {
	if store == nil || graphID == 0 {
		return nil
	}
	meta, ok := store.NestedMetaFor(graphID)
	if !ok || meta.ParentGraphID == 0 {
		return nil
	}
	results := sess.ResultsMap()
	if results == nil {
		return nil
	}
	parentGraph := store.Graphs()[meta.ParentGraphID]
	if parentGraph == nil {
		return nil
	}
	parentFn := store.FuncForGraph(parentGraph)
	if parentFn == nil {
		return nil
	}
	parentResult := results[parentFn]
	if parentResult == nil {
		return nil
	}
	return parentResult.Facts
}

func (d *Driver) loadFunctionResult(
	sess api.AnalysisSession,
	graphID, parentHash uint64,
	store api.IterationStore,
) *api.FuncResult {
	if d.cfg.FuncResultQ == nil {
		return nil
	}
	revision := uint64(0)
	if store != nil {
		revision = store.Revision()
	}
	return d.cfg.FuncResultQ.Get(sess.Context(), api.FuncKey{
		GraphID:       graphID,
		ParentHash:    parentHash,
		StoreRevision: revision,
	})
}

func (d *Driver) recordFunctionResult(
	sess api.AnalysisSession,
	fn *ast.FunctionExpr,
	result *api.FuncResult,
	results map[*ast.FunctionExpr]*api.FuncResult,
) {
	if result == nil {
		return
	}
	if results != nil {
		results[fn] = result
	}
	if fn == sess.RootFuncNode() {
		sess.SetRootResultValue(result)
	}
}

func (d *Driver) emitScopeDepthDiagnostic(sess api.AnalysisSession, fn *ast.FunctionExpr, result *api.FuncResult) {
	if result == nil || !d.cfg.EmitScopeDiag || d.cfg.MaxScopeDepth <= 0 || !result.DepthLimitExceeded {
		return
	}
	scopeState := sess.ScopeDepthDiagState()
	if scopeState[fn] {
		return
	}
	pos := diag.Position{File: sess.Source()}
	span := diag.Span{}
	if fn != nil && fn.Line() > 0 {
		pos.Line = fn.Line()
		pos.Column = fn.Column()
		span.StartLine = fn.Line()
		span.StartCol = fn.Column()
		span.EndLine = fn.LastLine()
		span.EndCol = fn.LastColumn()
	}
	sess.AppendDiagnostics(diag.Diagnostic{
		Position: pos,
		Span:     span,
		Severity: diag.SeverityWarning,
		Message:  fmt.Sprintf("scope depth limit exceeded (max=%d); analysis may be incomplete", d.cfg.MaxScopeDepth),
	})
	scopeState[fn] = true
}

func (d *Driver) storeFunctionRefinement(store api.IterationStore, result *api.FuncResult, funcSym cfg.SymbolID) {
	if result == nil || store == nil || funcSym == 0 {
		return
	}
	lookup := func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		return effects.LookupRefinementBySym(store.RefinementStore(), store.ModuleBindings(), d.cfg.GlobalTypes, sym)
	}
	fnEffect := effects.Propagate(result, lookup)
	if fnEffect == nil {
		return
	}
	store.StoreFunctionRefinement(funcSym, fnEffect)
}

func collectGlobalNames(globalTypes map[string]typ.Type) []string {
	if globalTypes == nil {
		return nil
	}
	all := cfg.SortedFieldNames(globalTypes)
	names := make([]string, 0, len(all))
	for _, name := range all {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

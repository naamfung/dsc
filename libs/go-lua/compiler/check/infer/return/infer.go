// infer.go implements return type inference for local functions.
// This runs as a pre-phase before the main analysis pipeline to ensure
// return summaries are available when the parent function is analyzed.
//
// # RETURN TYPE INFERENCE
//
// Local functions (defined via "local function f()" or "local f = function()")
// require return type inference to support interprocedural analysis. The inference
// uses a two-level fixpoint:
//
//  1. SCC-level: Functions are grouped into strongly connected components (SCCs)
//     based on their call graph. Non-recursive functions have single-element SCCs.
//
//  2. Per-SCC iteration: Functions within an SCC iterate until return types
//     stabilize. Each iteration uses monotone union to combine new results
//     with previous results, ensuring convergence.
//
// # MONOTONE UNION
//
// Return type inference uses monotone union for convergence:
//   - New return types are joined with previous return types
//   - Types can only grow (become more general), never shrink
//   - Bounded iteration with widening to unknown on non-convergence
//
// # PARAM HINTS
//
// Parameter type hints are collected from call sites:
//   - When a() calls b(10), b's first param gets hint "number"
//   - Hints from multiple call sites are joined
//   - Hints propagate through the call graph (if a() calls b(), b() calls c())
//
// # SEED PROPAGATION
//
// Return summaries are seeded from the previous fixpoint iteration:
//   - Seeds provide initial return type estimates
//   - Iteration refines seeds using actual function body analysis
//   - Convergence occurs when seeds stabilize across iterations
package infer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Config holds dependencies for return inference.
type Config struct {
	Types         core.TypeOps
	GlobalTypes   map[string]typ.Type
	Manifests     io.ManifestQuerier
	Stdlib        *scope.State
	Store         api.StoreView
	Graphs        api.GraphProvider
	SourceName    string
	MaxIterations int
}

// Inferencer computes pre-flow return summaries for local functions.
type Inferencer struct {
	types         core.TypeOps
	globalTypes   map[string]typ.Type
	manifests     io.ManifestQuerier
	stdlib        *scope.State
	store         api.StoreView
	graphs        api.GraphProvider
	sourceName    string
	maxIterations int
}

// New creates a configured return inferencer.
func New(cfg Config) *Inferencer {
	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}
	return &Inferencer{
		types:         cfg.Types,
		globalTypes:   cfg.GlobalTypes,
		manifests:     cfg.Manifests,
		stdlib:        cfg.Stdlib,
		store:         cfg.Store,
		graphs:        cfg.Graphs,
		sourceName:    cfg.SourceName,
		maxIterations: maxIter,
	}
}

// RunContext carries per-run inputs for return inference.
type RunContext struct {
	Ctx          *db.QueryContext
	ParentFacts  flow.TypeFacts
	EffectLookup constraint.RefinementLookupBySym
}

// collectLocalFunctions gathers local function definitions from assignments and FuncDef nodes.
func (i *Inferencer) collectLocalFunctions(
	graph *cfg.Graph,
	pointScopes map[cfg.Point]*scope.State,
	parentFn *ast.FunctionExpr,
) map[cfg.SymbolID]*returns.LocalFuncInfo {
	localFuncs := make(map[cfg.SymbolID]*returns.LocalFuncInfo)

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal || len(info.Targets) == 0 {
			return
		}
		info.EachTargetSource(func(idx int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			fnExpr, ok := source.(*ast.FunctionExpr)
			if !ok {
				return
			}

			fnGraph := (*cfg.Graph)(nil)
			if i.graphs != nil {
				fnGraph = i.graphs.GetOrBuildCFG(fnExpr)
			}
			localFuncs[target.Symbol] = &returns.LocalFuncInfo{
				Sym:         target.Symbol,
				Fn:          fnExpr,
				DefScope:    pointScopes[p],
				Graph:       fnGraph,
				ParentGraph: graph,
				ParentFn:    parentFn,
				DefPoint:    p,
			}
		})
	})

	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil || info.Symbol == 0 || info.FuncExpr == nil {
			return
		}
		if _, exists := localFuncs[info.Symbol]; exists {
			return
		}
		fnGraph := (*cfg.Graph)(nil)
		if i.graphs != nil {
			fnGraph = i.graphs.GetOrBuildCFG(info.FuncExpr)
		}
		localFuncs[info.Symbol] = &returns.LocalFuncInfo{
			Sym:         info.Symbol,
			Fn:          info.FuncExpr,
			DefScope:    pointScopes[p],
			Graph:       fnGraph,
			ParentGraph: graph,
			ParentFn:    parentFn,
			DefPoint:    p,
		}
	})

	return localFuncs
}

// newReturnInferenceEngine creates a synthesis engine configured for return type
// inference within the pre-flow return summary computation phase.
//
// The engine operates in PhaseScopeCompute mode with:
//   - Declared types from the overlay (params, siblings, captured variables)
//   - Global types for built-in function resolution
//   - Module aliases for require() resolution
//   - Return summaries from previous iteration for recursive call resolution
//
// Unlike the main analysis engine, this engine does not have access to flow
// solution or narrowed types, producing "declared-phase" type estimates.
func (i *Inferencer) newReturnInferenceEngine(
	run RunContext,
	scopes map[cfg.Point]*scope.State,
	ctx api.DeclaredEnv,
) *synth.Engine {
	return synth.New(synth.Config{
		Ctx:            run.Ctx,
		Types:          i.types,
		Scopes:         scopes,
		Manifests:      i.manifests,
		Env:            ctx,
		Phase:          api.PhaseScopeCompute,
		ModuleBindings: i.store.ModuleBindings(),
		ModuleAliases:  i.store.ModuleAliases(),
	})
}

// computeReturnSummariesForGraph computes return summaries for local functions in a graph
// and stores them into the interproc facts for the current iteration.
func (i *Inferencer) ComputeForGraph(
	run RunContext,
	graph *cfg.Graph,
	parent *scope.State,
) (api.ReturnSummaries, api.FuncTypes, []diag.Diagnostic) {
	if i == nil || i.store == nil || graph == nil || parent == nil {
		return nil, nil, nil
	}

	parentScope := api.ParentScopeForGraph(i.store, graph.ID(), parent)

	engine := phase.CreateTypeResolutionEngine(run.Ctx, graph, i.globalTypes, nil, parentScope, i.types, i.manifests)
	pointScopes := scope.BuildTypeDefScopes(graph, parentScope, engine.ResolveTypeDef)
	localFuncs := i.collectLocalFunctions(graph, pointScopes, graph.Func())
	if len(localFuncs) == 0 {
		return nil, nil, nil
	}

	// Apply param hints from the stable snapshot (deterministic order).
	if hints := i.store.GetParamHintsSnapshot(graph, parentScope); len(hints) > 0 {
		for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
			info := localFuncs[sym]
			if info == nil {
				continue
			}
			if hintVec, ok := hints[sym]; ok && len(hintVec) > 0 {
				info.ParamHints = hintVec
			}
		}
	}

	seed := i.store.GetReturnSummariesSnapshot(graph, parentScope)
	summaries, diags := i.computeReturnSummariesForGroup(run, parentScope.GroupHash(), localFuncs, seed)
	funcTypes := i.buildLocalFuncTypes(localFuncs, summaries, engine, parentScope)
	return summaries, funcTypes, diags
}

func (i *Inferencer) buildLocalFuncTypes(
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
	engine *synth.Engine,
	parentScope *scope.State,
) api.FuncTypes {
	if len(localFuncs) == 0 {
		return nil
	}
	out := make(api.FuncTypes, len(localFuncs))
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		info := localFuncs[sym]
		if info == nil || info.Fn == nil {
			continue
		}
		resolveScope := info.DefScope
		if resolveScope == nil {
			resolveScope = parentScope
		}
		if resolveScope == nil {
			resolveScope = scope.New()
		}
		var bindings interface {
			ParamSymbols(*ast.FunctionExpr) []cfg.SymbolID
			Name(cfg.SymbolID) string
		}
		if info.Graph != nil {
			bindings = info.Graph.Bindings()
		}
		seed := returns.BuildSeedFunctionTypeWithBindings(info.Fn, engine, resolveScope, bindings)
		fnType := unwrap.Function(seed)
		if fnType == nil {
			continue
		}
		if len(info.ParamHints) > 0 {
			if merged := paramhints.MergeIntoSignature(info.Fn, info.ParamHints, fnType); merged != nil {
				fnType = merged
			}
		}
		if summary := summaries[sym]; len(summary) > 0 {
			if withSummary := returns.WithSummaryOrUnknown(fnType, summary); withSummary != nil {
				fnType = withSummary
			}
		}
		out[sym] = fnType
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// computeReturnSummariesForGroup computes return type summaries for a scope group
// using strongly connected component (SCC) based fixpoint iteration.
//
// SCC ORDERING: Functions are partitioned into SCCs by their call graph. SCCs are
// processed in topological order so that callees are resolved before callers.
// This minimizes iteration count for non-recursive function groups.
//
// PER-SCC ITERATION: Within each SCC (which may contain mutually recursive functions),
// a fixpoint loop iterates until return types stabilize:
//   - Each iteration computes new return types from function bodies
//   - New types are joined with previous types via monotone union
//   - Iteration stops when no type changes
//
// WIDENING: If SCC iteration exceeds MaxReturnSummaryIterations, types are widened
// to unknown to guarantee termination. A diagnostic is emitted for the non-convergence.
//
// SEEDING: Initial return type estimates come from the seed map (previous fixpoint
// iteration). This accelerates convergence for iteratively-refined modules.
func (i *Inferencer) computeReturnSummariesForGroup(
	run RunContext,
	groupHash uint64,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	seed map[cfg.SymbolID][]typ.Type,
) (map[cfg.SymbolID][]typ.Type, []diag.Diagnostic) {
	_ = groupHash
	if len(localFuncs) == 0 {
		return nil, nil
	}

	sccs := i.planLocalFunctionSCCs(localFuncs)
	if len(sccs) == 0 {
		return nil, nil
	}

	summaries := seedSummariesFromSeed(localFuncs, seed)
	return summaries, i.processSCCSummaries(run, sccs, localFuncs, summaries)
}

// returnInferenceContext holds shared state for return type inference phases.
type returnInferenceContext struct {
	run           RunContext
	info          *returns.LocalFuncInfo
	summaries     map[cfg.SymbolID][]typ.Type
	localFuncs    map[cfg.SymbolID]*returns.LocalFuncInfo
	engine        *synth.Engine
	resolveScope  *scope.State
	moduleAliases map[cfg.SymbolID]string
	bindings      *bind.BindingTable
	parentFacts   flow.TypeFacts
}

// buildParameterOverlay creates the initial type overlay with parameter types.
// Parameters are typed from annotations, hints, or default to unknown.
func collectReturnTypes(
	fnGraph *cfg.Graph,
	synthEngine api.Synth,
	deadPoints map[cfg.Point]bool,
) []typ.Type {
	if fnGraph == nil || synthEngine == nil {
		return nil
	}
	var returnTypes []typ.Type
	seenReturn := false

	fnGraph.EachReturn(func(p cfg.Point, retInfo *cfg.ReturnInfo) {
		if retInfo == nil {
			return
		}
		_ = deadPoints

		types := synthesizeReturnExprs(synthEngine, retInfo, p)
		if !seenReturn {
			seenReturn = true
			returnTypes = types
			return
		}

		returnTypes = joinReturnTypes(returnTypes, types)
	})

	return returns.NormalizeReturnVector(returnTypes)
}

// synthesizeReturnExprs computes types for a single return statement's expressions.
func synthesizeReturnExprs(
	synthEngine api.Synth,
	retInfo *cfg.ReturnInfo,
	p cfg.Point,
) []typ.Type {
	if len(retInfo.Exprs) == 0 {
		return nil
	}

	var types []typ.Type
	for i, expr := range retInfo.Exprs {
		if i == len(retInfo.Exprs)-1 {
			multi := synthEngine.MultiTypeOf(expr, p)
			if len(multi) == 0 {
				multi = []typ.Type{typ.Unknown}
			} else {
				for j, mt := range multi {
					if mt == nil {
						multi[j] = typ.Unknown
					}
				}
			}
			types = append(types, multi...)
		} else {
			t := synthEngine.TypeOf(expr, p)
			if t == nil {
				t = typ.Unknown
			}
			types = append(types, t)
		}
	}
	return types
}

// joinReturnTypes merges two return type vectors using union semantics.
func joinReturnTypes(existing, incoming []typ.Type) []typ.Type {
	for len(existing) < len(incoming) {
		existing = append(existing, typ.Nil)
	}
	for i := range existing {
		var t typ.Type
		if i < len(incoming) {
			t = incoming[i]
		} else {
			t = typ.Nil
		}
		existing[i] = typ.JoinReturnSlot(existing[i], t)
	}
	return existing
}

// inferReturnTypesFromBody runs phase 2 synthesis to compute return types.
func (i *Inferencer) inferReturnTypesFromBody(
	ctx *returnInferenceContext,
	finalOverlay map[cfg.SymbolID]typ.Type,
) []typ.Type {
	state := i.runPhase2FlowNarrowing(ctx, finalOverlay)
	narrowed := collectReturnTypes(ctx.info.Graph, state.synth, state.deadPoints)

	fnGraph := ctx.info.Graph
	if fnGraph == nil {
		return narrowed
	}
	phaseReturnSummaries := summarizeWithoutCurrent(ctx.summaries, ctx.info)
	declCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:           fnGraph,
		Bindings:        ctx.bindings,
		BaseScope:       ctx.resolveScope,
		DeclaredTypes:   finalOverlay,
		GlobalTypes:     i.globalTypes,
		ModuleAliases:   ctx.moduleAliases,
		ReturnSummaries: phaseReturnSummaries,
	})
	declSynth := i.newReturnInferenceEngine(
		ctx.run,
		uniformFunctionScopes(fnGraph, ctx.resolveScope),
		declCheckCtx,
	)
	declared := collectReturnTypes(fnGraph, declSynth, nil)

	return returns.MergeReturnSummary(declared, narrowed)
}

// inferReturnWithSummary infers return types for a single function using available summaries.
// This is the core inference logic called by computeReturnSummariesForGroup for each function.
//
// TWO-PHASE INFERENCE:
//
// Phase 1 (Preliminary): Collect inferred types for local variables within the function.
// This uses a preliminary synthesis engine with:
//   - Parameter types (from annotations or param hints)
//   - Sibling function types (from summaries)
//   - Captured variable types (from parent function result)
//
// Phase 2 (Final): Compute return types using enriched overlay containing:
//   - All Phase 1 inferred types
//   - Field assignments merged into table types
//   - Indexer assignments merged into map components
//   - Table mutation effects (table.insert, etc.)
//
// EXPLICIT ANNOTATIONS: If the function has explicit @return annotations, those are
// used directly without body inference.
//
// MULTI-RETURN: Functions may return multiple values. The inference handles multi-return
// by expanding the last expression (which may be a call or vararg) and joining position-wise.
func (i *Inferencer) inferReturnWithSummary(
	run RunContext,
	info *returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
) []typ.Type {
	if info == nil || info.Fn == nil || info.Graph == nil {
		return nil
	}

	fn := info.Fn
	fnGraph := info.Graph
	parentScope := info.DefScope
	moduleAliases := modules.MergeAliases(i.store.ModuleAliases(), modules.CollectAliases(fnGraph))

	engine := phase.CreateTypeResolutionEngine(run.Ctx, fnGraph, i.globalTypes, nil, parentScope, i.types, i.manifests)

	resolveScope := parentScope
	if len(fn.TypeParams) > 0 {
		typeParams := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constr typ.Type
			if tp.Constraint != nil {
				constr = engine.ResolveType(tp.Constraint, resolveScope)
			}
			typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constr)
		}
		resolveScope = resolveScope.WithTypeParams(typeParams)
	}

	// Check for explicit return type annotations.
	if len(fn.ReturnTypes) > 0 {
		rets := engine.ResolveReturnTypes(fn.ReturnTypes, resolveScope)
		if len(rets) > 0 {
			return rets
		}
	}

	// Resolve bindings for this function.
	bindings := fnGraph.Bindings()
	if bindings == nil && i.store != nil {
		bindings = i.store.ModuleBindings()
	}

	// Build inference context shared across all phases.
	ctx := &returnInferenceContext{
		run:           run,
		info:          info,
		summaries:     summaries,
		localFuncs:    localFuncs,
		engine:        engine,
		resolveScope:  resolveScope,
		moduleAliases: moduleAliases,
		bindings:      bindings,
		parentFacts:   run.ParentFacts,
	}

	// Build type overlay with parameter types.
	overlay := i.buildParameterOverlay(ctx)

	// Add sibling function types from summaries.
	i.enrichOverlayWithSiblings(ctx, overlay)

	// Collect all return summaries and add local function types.
	allSummaries := i.collectAllReturnSummaries(ctx)
	i.enrichOverlayWithLocalFunctions(ctx, overlay, allSummaries)

	// Add captured variable types from parent.
	i.enrichOverlayWithCaptured(ctx, overlay)

	// Add local declared types (annotations, loop variables) as overlay hints.
	i.enrichOverlayWithLocalDeclarations(ctx, overlay)

	// Phase 1: Infer local variable types.
	inferred, _, synthAdapter := i.inferLocalVariableTypes(ctx, overlay)

	// Collect field/indexer assignments and apply mutations.
	finalOverlay := i.collectAndApplyMutations(ctx, overlay, inferred, synthAdapter)

	// Phase 2: Infer return types from body.
	return i.inferReturnTypesFromBody(ctx, finalOverlay)
}

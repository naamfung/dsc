// runner.go implements the multi-phase analysis pipeline for functions.
//
// The pipeline executes four phases for each function:
//
//	Phase A (Resolve): Resolves type annotations into concrete types
//	Phase B (Scope): Builds lexical scope states and extracts flow constraints
//	Phase C (Solve): Solves the flow constraint system
//	Phase D (Narrow): Applies narrowing to compute effective types
//
// The [Runner] is used as the compute function for memoized FuncResult queries.
// It receives a FuncKey (graph ID + parent hash + revision) and produces a
// complete [api.FuncResult] containing all phase outputs.
//
// The pipeline integrates several subsystems:
//   - Synthesis engine for expression type computation
//   - Flow solver for control flow analysis
//   - Effect propagation for side effect tracking
//   - Parameter hint inference from call sites
package pipeline

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// RunnerConfig configures a pipeline runner.
type RunnerConfig struct {
	Types         core.TypeOps
	GlobalTypes   map[string]typ.Type
	Stdlib        *scope.State
	Manifests     io.ManifestQuerier
	Resolver      narrow.Resolver
	MaxScopeDepth int

	ComputePasses []api.ComputePass
}

// Runner executes the phase pipeline for a single function.
// It is used as the compute function for FuncResult queries.
type Runner struct {
	types         core.TypeOps
	globalTypes   map[string]typ.Type
	stdlib        *scope.State
	manifests     io.ManifestQuerier
	resolver      narrow.Resolver
	maxScopeDepth int
	computePasses []api.ComputePass
}

// NewRunner returns a configured pipeline runner.
func NewRunner(cfg RunnerConfig) *Runner {
	return &Runner{
		types:         cfg.Types,
		globalTypes:   cfg.GlobalTypes,
		stdlib:        cfg.Stdlib,
		manifests:     cfg.Manifests,
		resolver:      cfg.Resolver,
		maxScopeDepth: cfg.MaxScopeDepth,
		computePasses: cfg.ComputePasses,
	}
}

// Run executes the phase pipeline and returns the full function analysis result.
func (r *Runner) Run(ctx *db.QueryContext, key api.FuncKey) *api.FuncResult {
	store := api.StoreFrom(ctx)
	if store == nil {
		return nil
	}
	withPhase := func(_ api.Phase, fn func()) { fn() }
	if phaser, ok := store.(interface{ WithPhase(api.Phase, func()) }); ok {
		withPhase = phaser.WithPhase
	}
	graphs := store.Graphs()
	parents := store.Parents()
	if graphs == nil || parents == nil {
		return nil
	}
	graph := graphs[key.GraphID]
	if graph == nil {
		return nil
	}
	fn := graph.Func()
	if fn == nil {
		return nil
	}
	parent := parents[key.ParentHash]
	if setter, ok := store.(interface {
		SetGraphParentHash(graphID, parentHash uint64)
	}); ok {
		setter.SetGraphParentHash(graph.ID(), key.ParentHash)
	}

	paramHintSigs := paramhints.BuildParamHintSigView(store, graph, parent, r.stdlib)
	synthSig := r.resolveSynthesizedSignature(ctx, store, graph, fn, parent, paramHintSigs)

	// Canonical local function types for this graph (stable snapshot).
	siblingTypes := store.GetLocalFuncTypesSnapshot(graph, parent)
	// Return summaries include captured field assignments (stable snapshot).
	returnSummaries := store.GetReturnSummariesSnapshot(graph, parent)
	var narrowReturnSummaries map[cfg.SymbolID][]typ.Type
	withPhase(api.PhaseNarrowing, func() {
		narrowReturnSummaries = store.GetNarrowReturnSummariesSnapshot(graph, parent)
	})

	// Build shared phase environment once.
	localAliases := modules.CollectAliases(graph)
	mergedAliases := modules.MergeAliases(store.ModuleAliases(), localAliases)
	env := phase.PhaseEnv{
		Ctx:             ctx,
		Graph:           graph,
		Fn:              fn,
		Types:           r.types,
		Manifests:       r.manifests,
		GlobalTypes:     r.globalTypes,
		ModuleAliases:   mergedAliases,
		ModuleBindings:  store.ModuleBindings(),
		RefinementStore: effectStoreFrom(store),
	}

	// Phase A: Resolve type annotations.
	resolveOut := phase.RunResolve(phase.ResolveInput{
		PhaseEnv:  env,
		Bindings:  graph.Bindings(),
		BaseScope: parent,
	})

	// Literal signatures are provided by the stable snapshot.
	literalSigs := r.literalSigProvider(store, graph, parent)

	// Phase B: Build scopes and extract declared types.
	scopeOut := phase.RunScope(phase.ScopeInput{
		PhaseEnv:                  env,
		Parent:                    parent,
		MaxScopeDepth:             r.maxScopeDepth,
		Resolve:                   resolveOut,
		SynthesizedFunctionSig:    synthSig,
		FunctionLiteralSignatures: literalSigs,
		ParamHintSignatures:       paramHintSigs,
		SiblingTypes:              siblingTypes,
		ReturnSummaries:           returnSummaries,
	})
	// Declared is the default phase for scope/extract and interproc reads.

	if capturedTypes := store.GetCapturedTypesSnapshot(graph, parent); len(capturedTypes) > 0 {
		scopeOut.DeclaredTypes = captured.MergeCapturedTypes(scopeOut.DeclaredTypes, capturedTypes)
	}
	r.mergeCapturedParentFuncTypes(store, graph, fn, &scopeOut)

	// Populate scopes in env for later phases.
	env.Scopes = scopeOut.Scopes

	// Phase B (continued): Synthesize function literal types.
	literalOut := phase.RunLiteral(phase.LiteralInput{
		PhaseEnv:        env,
		Scope:           scopeOut,
		SiblingTypes:    scopeOut.SiblingTypes,
		ReturnSummaries: returnSummaries,
	})
	// Ensure literal function types use canonical local function types.
	if len(siblingTypes) > 0 {
		if literalOut.LiteralTypes == nil {
			literalOut.LiteralTypes = make(flow.DeclaredTypes, len(siblingTypes))
		}
		for sym, fnType := range siblingTypes {
			if fnType == nil {
				continue
			}
			literalOut.LiteralTypes[sym] = fnType
		}
	}

	// Phase B (continued): Extract flow constraints.
	extractOut := phase.RunExtract(phase.FlowExtractInput{
		PhaseEnv:        env,
		Resolve:         resolveOut,
		Scope:           scopeOut,
		SiblingTypes:    scopeOut.SiblingTypes,
		LiteralTypes:    literalOut.LiteralTypes,
		ReturnSummaries: returnSummaries,
	})
	r.appendCapturedMutatorAssignments(store, graph, parent, env, scopeOut, literalOut, returnSummaries, &extractOut)

	// Phase C: Solve flow system.
	solveOut := phase.RunSolve(phase.FlowSolveInput{
		PhaseEnv: env,
		Extract:  extractOut,
		Resolver: r.resolver,
	})
	// Phase D: Narrowing and effect inference.
	var narrowOut phase.NarrowOutput
	withPhase(api.PhaseNarrowing, func() {
		narrowOut = phase.RunNarrow(phase.NarrowInput{
			PhaseEnv:              env,
			Scope:                 scopeOut,
			Extract:               extractOut,
			Solve:                 solveOut,
			SiblingTypes:          scopeOut.SiblingTypes,
			LiteralTypes:          literalOut.LiteralTypes,
			NarrowReturnSummaries: narrowReturnSummaries,
		})
	})

	extras := r.runComputePasses(graph, scopeOut.Scopes)

	return &api.FuncResult{
		Graph:              graph,
		ModuleBindings:     env.ModuleBindings,
		BaseScope:          scopeOut.BaseScope,
		Scopes:             scopeOut.Scopes,
		Facts:              narrowOut.Facts,
		FlowInputs:         extractOut.Inputs,
		FlowSolution:       solveOut.Solution,
		FnRefinement:       narrowOut.Refinement,
		NarrowSynth:        narrowOut.Synth,
		LiteralSignatures:  literalOut.Signatures,
		Extras:             extras,
		DepthLimitExceeded: scopeOut.DepthLimitExceeded,
	}
}

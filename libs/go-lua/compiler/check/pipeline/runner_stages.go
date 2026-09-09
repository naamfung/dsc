package pipeline

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func (r *Runner) resolveSynthesizedSignature(
	ctx *db.QueryContext,
	store api.StoreView,
	graph *cfg.Graph,
	fn *ast.FunctionExpr,
	parent *scope.State,
	paramHintSigs map[*ast.FunctionExpr][]typ.Type,
) *typ.Function {
	if graph == nil || fn == nil {
		return nil
	}

	// Prefer literal signature from parent graph for nested functions.
	synthSig := r.literalSignatureForFunction(store, graph, fn)
	if paramHintSigs == nil {
		return synthSig
	}
	hints := paramHintSigs[fn]
	if len(hints) == 0 {
		return synthSig
	}
	if synthSig == nil {
		engine := synth.New(synth.Config{
			Ctx:       ctx,
			Types:     r.types,
			Manifests: r.manifests,
			Phase:     api.PhaseTypeResolution,
		})
		if sig := engine.ResolveFunctionSignature(fn, parent); sig != nil {
			synthSig = sig
		} else if seedFn, ok := returns.BuildSeedFunctionTypeWithBindings(fn, engine, parent, graph.Bindings()).(*typ.Function); ok {
			synthSig = seedFn
		}
	}
	if synthSig == nil {
		return nil
	}
	return paramhints.MergeIntoSignature(fn, hints, synthSig)
}

func (r *Runner) appendCapturedMutatorAssignments(
	store api.StoreView,
	graph *cfg.Graph,
	parent *scope.State,
	env phase.PhaseEnv,
	scopeOut phase.ScopeOutput,
	literalOut phase.LiteralOutput,
	returnSummaries map[cfg.SymbolID][]typ.Type,
	extractOut *phase.FlowExtractOutput,
) {
	if store == nil || graph == nil || extractOut == nil || extractOut.Inputs == nil {
		return
	}

	capturedContainers := store.GetCapturedContainerMutationsSnapshot(graph, parent)
	if len(capturedContainers) == 0 {
		return
	}

	bindings := graph.Bindings()
	if bindings == nil {
		bindings = store.ModuleBindings()
	}

	declaredEnv := phase.NewContextBuilder(env).
		WithScope(scopeOut).
		WithSiblingTypes(scopeOut.SiblingTypes).
		WithLiteralTypes(literalOut.LiteralTypes).
		WithReturnSummaries(returnSummaries).
		BuildDeclared()

	synthEngine := synth.New(synth.Config{
		Ctx:            env.Ctx,
		Types:          env.Types,
		Scopes:         scopeOut.Scopes,
		Manifests:      env.Manifests,
		Env:            declaredEnv,
		Phase:          api.PhaseScopeCompute,
		ModuleBindings: env.ModuleBindings,
		ModuleAliases:  env.ModuleAliases,
	})

	symResolver := resolve.BuildInputSymbolResolver(declaredEnv, extractOut.Inputs)
	assignmentTypes := resolve.BuildAssignmentTypeResolver(extractOut.Inputs)
	calleeTypeResolver := func(info *cfg.CallInfo, p cfg.Point) typ.Type {
		return resolve.CalleeType(info, p, synthEngine.TypeOf, symResolver, assignmentTypes, graph, bindings, env.ModuleBindings)
	}

	extra := returns.CollectCalledNestedContainerMutatorAssignments(graph, bindings, capturedContainers, calleeTypeResolver)
	if len(extra) == 0 {
		return
	}
	extractOut.Inputs.ContainerMutatorAssignments = append(extractOut.Inputs.ContainerMutatorAssignments, extra...)
}

func (r *Runner) runComputePasses(graph *cfg.Graph, scopes map[cfg.Point]*scope.State) map[string]any {
	if graph == nil || len(r.computePasses) == 0 {
		return nil
	}
	extras := make(map[string]any, len(r.computePasses))
	for _, pass := range r.computePasses {
		extras[pass.Name()] = pass.Run(graph, scopes)
	}
	return extras
}

func (r *Runner) literalSignatureForFunction(store api.StoreView, graph *cfg.Graph, fn *ast.FunctionExpr) *typ.Function {
	if store == nil || graph == nil || fn == nil {
		return nil
	}
	meta, ok := store.NestedMetaFor(graph.ID())
	if !ok {
		return nil
	}
	parentGraph := store.Graphs()[meta.ParentGraphID]
	if parentGraph == nil {
		return nil
	}

	if sigs := scratchLiteralSigs(store, parentGraph.ID()); len(sigs) > 0 {
		if sig := sigs[fn]; sig != nil {
			return sig
		}
	}

	parentScope := r.parentScopeForGraph(store, parentGraph)
	if parentScope == nil {
		return nil
	}
	if sigs := store.GetLiteralSigsSnapshot(parentGraph, parentScope); len(sigs) > 0 {
		if sig := sigs[fn]; sig != nil {
			return sig
		}
	}
	return nil
}

func (r *Runner) literalSigProvider(store api.StoreView, graph *cfg.Graph, parent *scope.State) phase.LiteralSigsProvider {
	if store == nil || graph == nil || parent == nil {
		return nil
	}
	var literalSigMap map[*ast.FunctionExpr]*typ.Function
	if sigs := store.GetLiteralSigsSnapshot(graph, parent); len(sigs) > 0 {
		literalSigMap = mergeLiteralSignatures(nil, sigs, true)
	}
	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			parentScope := r.parentScopeForGraph(store, parentGraph)
			if parentScope != nil {
				if sigs := store.GetLiteralSigsSnapshot(parentGraph, parentScope); len(sigs) > 0 {
					literalSigMap = mergeLiteralSignatures(literalSigMap, sigs, false)
				}
			}
			if sigs := scratchLiteralSigs(store, parentGraph.ID()); len(sigs) > 0 {
				literalSigMap = mergeLiteralSignatures(literalSigMap, sigs, false)
			}
		}
	}
	if len(literalSigMap) > 0 {
		return phase.LiteralSigsMap(literalSigMap)
	}
	return nil
}

type effectStoreProvider interface {
	RefinementStore() api.RefinementStore
}

func effectStoreFrom(store api.StoreView) api.RefinementStore {
	if store == nil {
		return nil
	}
	if provider, ok := store.(effectStoreProvider); ok {
		return provider.RefinementStore()
	}
	return nil
}

type scratchLiteralStore interface {
	ScratchLiteralSigs(graphID uint64) map[*ast.FunctionExpr]*typ.Function
}

func scratchLiteralSigs(store api.StoreView, graphID uint64) map[*ast.FunctionExpr]*typ.Function {
	if store == nil {
		return nil
	}
	if provider, ok := store.(scratchLiteralStore); ok {
		return provider.ScratchLiteralSigs(graphID)
	}
	return nil
}

func (r *Runner) parentScopeForGraph(store api.StoreView, graph *cfg.Graph) *scope.State {
	if store == nil || graph == nil {
		return nil
	}
	if parentScope := api.ParentScopeForGraph(store, graph.ID(), nil); parentScope != nil {
		return parentScope
	}
	if _, ok := store.NestedMetaFor(graph.ID()); !ok {
		return r.stdlib
	}
	return nil
}

func (r *Runner) mergeCapturedParentFuncTypes(
	store api.StoreView,
	graph *cfg.Graph,
	fn *ast.FunctionExpr,
	scopeOut *phase.ScopeOutput,
) {
	if store == nil || graph == nil || fn == nil || scopeOut == nil {
		return
	}
	meta, ok := store.NestedMetaFor(graph.ID())
	if !ok || meta.ParentGraphID == 0 || graph.Bindings() == nil {
		return
	}
	parentGraph := store.Graphs()[meta.ParentGraphID]
	if parentGraph == nil {
		return
	}
	parentScope := r.parentScopeForGraph(store, parentGraph)
	if parentScope == nil {
		return
	}
	parentFuncTypes := store.GetLocalFuncTypesSnapshot(parentGraph, parentScope)
	if len(parentFuncTypes) == 0 {
		return
	}
	for _, sym := range graph.Bindings().CapturedSymbols(fn) {
		ft := parentFuncTypes[sym]
		if sym == 0 || ft == nil {
			continue
		}
		if scopeOut.DeclaredTypes == nil {
			scopeOut.DeclaredTypes = make(flow.DeclaredTypes)
		}
		scopeOut.DeclaredTypes[sym] = ft
	}
}

func mergeLiteralSignatures(
	dst map[*ast.FunctionExpr]*typ.Function,
	src map[*ast.FunctionExpr]*typ.Function,
	overwrite bool,
) map[*ast.FunctionExpr]*typ.Function {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[*ast.FunctionExpr]*typ.Function, len(src))
	}
	for fnExpr, sig := range src {
		if fnExpr == nil || sig == nil {
			continue
		}
		if _, exists := dst[fnExpr]; exists && !overwrite {
			continue
		}
		dst[fnExpr] = sig
	}
	return dst
}

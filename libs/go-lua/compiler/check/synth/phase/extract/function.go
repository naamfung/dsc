// function.go implements function type synthesis and return type inference.
//
// # FUNCTION TYPE SYNTHESIS
//
// SynthFunctionType builds a complete function type from a FunctionExpr by:
//  1. Resolving type parameters (if generic)
//  2. Extracting parameter types from annotations or expected types
//  3. Building a CFG for the function body
//  4. Inferring callback overlay specs (for callback-accepting higher-order functions)
//  5. Inferring return types from body analysis or using expected/declared types
//
// CONTEXTUAL TYPING (EXPECTED TYPES)
//
// When an expected function type is available (e.g., from callback parameter context),
// it provides default types for unannotated parameters and fallback return types.
// This enables idioms like:
//
//	items:filter(function(x) return x > 0 end)  -- x inferred from filter's param
//
// # RETURN TYPE INFERENCE
//
// Return types are inferred by analyzing all return statements in the function body.
// The algorithm:
//  1. Check ReturnSummaries for pre-computed results (from prior iterations)
//  2. Build CFG and create type overlay with parameter types
//  3. Create a temporary synthesizer environment
//  4. Visit each return statement, synthesizing expression types
//  5. Merge return types position-wise across all return paths
//
// # CALLBACK OVERLAY INFERENCE
//
// For higher-order functions that accept callbacks (e.g., transaction wrappers),
// this detects the "setup -> call param -> cleanup" pattern and builds a
// contract.Spec with EnvOverlay describing what types are available inside
// the callback scope.
package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// FunctionType synthesizes a complete function type from a function expression.
//
// Combines declared type annotations with inferred information to build the
// function signature. Delegates to SynthFunctionTypeWithExpected with no expected type.
func (s *Synthesizer) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return s.SynthFunctionTypeWithExpected(fn, sc, nil)
}

// SynthFunctionTypeWithExpected synthesizes a function type with contextual typing.
//
// When an expected function type is provided, it guides inference for:
//   - Unannotated parameter types (uses expected parameter types)
//   - Unannotated return types (uses expected return types as fallback)
//   - Self parameter in methods (infers from expected first param)
//
// Processing order:
// 1. Resolve type parameters and create scoped type param map
// 2. Apply parameter list (annotations + expected types)
// 3. Build CFG for body analysis
// 4. Infer callback env overlays (for callback-accepting functions)
// 5. Infer return types from body or use expected/declared
//
// If fn is nil, returns nil. If scope is nil, returns an empty function type.
func (s *Synthesizer) SynthFunctionTypeWithExpected(fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function) *typ.Function {
	return s.synthFunctionTypeWithCapturePoint(fn, sc, expected, 0, nil)
}

func (s *Synthesizer) getOrBuildFunctionGraph(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil {
		return nil
	}
	if s.deps.CheckCtx != nil {
		if g, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && g != nil && g.Func() == fn {
			return g
		}
	}
	if s.deps.Graphs != nil {
		if g := s.deps.Graphs.GetOrBuildCFG(fn); g != nil {
			return g
		}
	}
	if s.deps.ModuleBindings != nil {
		return cfg.BuildWithBindings(fn, s.deps.ModuleBindings)
	}
	return cfg.Build(fn)
}

func (s *Synthesizer) synthFunctionTypeWithCapturePoint(
	fn *ast.FunctionExpr,
	sc *scope.State,
	expected *typ.Function,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
) *typ.Function {
	if fn == nil {
		return nil
	}
	if s.deps.FunctionTypeInProgress == nil {
		s.deps.FunctionTypeInProgress = make(map[functionTypeProgressKey]bool)
	}
	progressKey := functionTypeProgressKey{Func: fn, CapturePoint: capturePoint}
	if s.deps.FunctionTypeInProgress[progressKey] {
		return s.buildFunctionTypeSummaryFallback(fn, sc, expected)
	}
	s.deps.FunctionTypeInProgress[progressKey] = true
	defer delete(s.deps.FunctionTypeInProgress, progressKey)

	builder := typ.Func()

	resolveScope := sc
	if resolveScope == nil {
		return builder.Build()
	}

	if len(fn.TypeParams) > 0 {
		typeParams := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constraint typ.Type
			if tp.Constraint != nil {
				constraint = s.ResolveType(tp.Constraint, resolveScope)
			}
			typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constraint)
			builder = builder.TypeParam(tp.Name, constraint)
		}
		resolveScope = resolveScope.WithTypeParams(typeParams)
	}

	implicitSelf := core.HasImplicitSelfParam(fn, s.deps.ModuleBindings)
	var implicitSelfType typ.Type
	if implicitSelf {
		if expected != nil && len(expected.Params) > 0 && expected.Params[0].Name == "self" && expected.Params[0].Type != nil {
			implicitSelfType = expected.Params[0].Type
		}
		if implicitSelfType == nil && resolveScope != nil && resolveScope.SelfType() != nil {
			implicitSelfType = resolveScope.SelfType()
		}
	}

	core.ApplyParamList(builder, fn, core.ParamListConfig{
		ResolveType:      s.ResolveType,
		ResolveScope:     resolveScope,
		Expected:         expected,
		ImplicitSelf:     implicitSelf,
		ImplicitSelfType: implicitSelfType,
	})

	// Build CFG once, shared between overlay inference and return inference.
	var fnGraph *cfg.Graph
	if len(fn.Stmts) > 0 {
		fnGraph = s.getOrBuildFunctionGraph(fn)
	}

	// Infer callback env overlays (runs before return types).
	if overlaySpec := s.inferCallbackOverlaySpec(fn, resolveScope, expected, fnGraph); overlaySpec != nil {
		builder = builder.Spec(overlaySpec)
	}

	inferredErrorReturn := false
	if len(fn.ReturnTypes) > 0 {
		returns := s.ResolveReturnTypes(fn.ReturnTypes, resolveScope)
		builder = builder.Returns(returns...)
	} else {
		if bodyReturns, hasErrorReturn := s.inferReturnTypesFromBody(fn, resolveScope, expected, fnGraph, capturePoint, captureTypes); len(bodyReturns) > 0 {
			inferredErrorReturn = hasErrorReturn
			if expected != nil && len(expected.Returns) > 0 {
				if typ.IsUnknownOnlyOrEmpty(bodyReturns) {
					bodyReturns = expected.Returns
				}
			}
			builder = builder.Returns(bodyReturns...)
		} else if expected != nil && len(expected.Returns) > 0 {
			builder = builder.Returns(expected.Returns...)
		}
	}

	fnType := builder.Build()
	if inferredErrorReturn {
		fnType = erreffect.AttachErrorReturnSpec(fnType, 0, 1)
	}
	return fnType
}

// inferReturnTypesFromBody infers return types from the function body.
// If fnGraph is non-nil, it reuses the pre-built CFG instead of building a new one.
func (s *Synthesizer) inferReturnTypesFromBody(
	fn *ast.FunctionExpr,
	parentScope *scope.State,
	expected *typ.Function,
	fnGraph *cfg.Graph,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
) ([]typ.Type, bool) {
	if len(fn.Stmts) == 0 {
		return nil, false
	}

	var returnSummaries map[cfg.SymbolID][]typ.Type
	if s.deps.CheckCtx != nil {
		if s.IsNarrowing() {
			if ctx, ok := s.deps.CheckCtx.(api.NarrowEnv); ok {
				returnSummaries = ctx.NarrowReturnSummaries()
			}
		} else {
			if ctx, ok := s.deps.CheckCtx.(api.DeclaredEnv); ok {
				returnSummaries = ctx.ReturnSummaries()
			}
		}
	}

	var fnSym cfg.SymbolID
	if s.deps.CheckCtx != nil {
		if pg, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && pg != nil {
			fnSym = localFunctionSymbol(pg, fn)
		}
	}

	// If a return summary exists for this function symbol, declared phase can
	// use it directly. Narrowing phase must still infer from the body so flow
	// predicates can remove stale union members from pre-flow summaries.
	var summaryFallback []typ.Type
	if len(returnSummaries) > 0 && fnSym != 0 {
		if rt := returnSummaries[fnSym]; len(rt) > 0 {
			if typ.HasKnownType(rt) {
				summaryFallback = rt
				if !s.IsNarrowing() && capturePoint == 0 && len(captureTypes) == 0 {
					return rt, false
				}
			}
		}
	}

	if fnGraph == nil {
		fnGraph = s.getOrBuildFunctionGraph(fn)
	}
	if fnGraph == nil {
		return nil, false
	}

	resolveScope := parentScope
	if len(fn.TypeParams) > 0 {
		typeParams := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constr typ.Type
			if tp.Constraint != nil {
				constr = s.ResolveType(tp.Constraint, resolveScope)
			}
			typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constr)
		}
		resolveScope = resolveScope.WithTypeParams(typeParams)
	}

	overlay := s.buildParamOverlay(fnGraph, resolveScope, expected)

	// Collect local function types from assignments using return summaries.
	// Uses annotations for params and looks up return types from summaries.
	// returnSummaries resolved above (pre-flow or post-flow depending on phase).

	fnGraph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal || len(info.Targets) == 0 {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			if fnExpr, ok := source.(*ast.FunctionExpr); ok {
				fnType := s.buildFunctionTypeWithSummary(fnExpr, resolveScope, target.Symbol, returnSummaries)
				if fnType != nil {
					overlay[target.Symbol] = fnType
				}
			}
		})
	})

	// Include captured symbol types from the parent context.
	// This allows nested local functions to call sibling locals defined in the parent scope.
	if s.deps.CheckCtx != nil {
		if types := s.deps.CheckCtx.Types(); types != nil {
			p := capturePoint
			if g := s.deps.CheckCtx.Graph(); g != nil {
				if p == 0 {
					p = g.Entry()
				}
			}
			if bindings := fnGraph.Bindings(); bindings != nil {
				for _, sym := range bindings.CapturedSymbols(fn) {
					if sym == 0 {
						continue
					}
					if _, ok := overlay[sym]; ok {
						continue
					}
					if t := captureTypes[sym]; t != nil {
						overlay[sym] = t
						continue
					}
					if solution := s.deps.CheckCtx.Consts(); solution != nil {
						if t := solution.TypeAt(p, constraint.Path{Symbol: sym}); t != nil {
							overlay[sym] = t
							continue
						}
					}
					if tv := types.EffectiveTypeAt(p, sym); tv.State == flow.StateResolved && tv.Type != nil {
						overlay[sym] = tv.Type
					}
				}
			}
		}
	}

	// Include local function types from the parent graph that are visible at this function's definition point.
	// Uses return summaries for return types instead of recursive inference.
	if s.deps.CheckCtx != nil {
		if pg, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && pg != nil {
			var defPoint cfg.Point
			for _, nf := range pg.NestedFunctions() {
				if nf.Func == fn {
					defPoint = nf.Point
					break
				}
			}
			if defPoint != 0 {
				visible := pg.AllSymbolsAt(defPoint)
				if len(visible) > 0 {
					visibleSyms := make(map[cfg.SymbolID]struct{}, len(visible))
					for _, sym := range visible {
						if sym != 0 {
							visibleSyms[sym] = struct{}{}
						}
					}
					pg.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
						if info == nil || !info.IsLocal || len(info.Targets) == 0 {
							return
						}
						info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
							if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
								return
							}
							if _, ok := visibleSyms[target.Symbol]; !ok {
								return
							}
							if _, ok := overlay[target.Symbol]; ok {
								return
							}
							if fnExpr, ok := source.(*ast.FunctionExpr); ok {
								if fnExpr == fn {
									return
								}
								fnType := s.buildFunctionTypeWithSummary(fnExpr, parentScope, target.Symbol, returnSummaries)
								if fnType != nil {
									overlay[target.Symbol] = fnType
								}
							}
						})
					})
				}
			}
		}
	}

	// Infer basic ordered-comparison hints (x > 0, name <= "zz") so unannotated
	// params don't stay unknown when return typing depends on guarded branches.
	enrichOverlayWithOrderedComparisonHints(fnGraph, overlay)

	var globalTypes map[string]typ.Type
	var moduleAliases map[cfg.SymbolID]string
	if s.deps.CheckCtx != nil {
		globalTypes = s.deps.CheckCtx.GlobalTypes()
		if moduleAliases == nil {
			moduleAliases = s.deps.CheckCtx.ModuleAliases()
		}
	}
	if moduleAliases == nil {
		moduleAliases = s.deps.ModuleAliases
	}
	// Phase 1: infer local assignment types using a preliminary context.
	// Build the preliminary synthesizer lazily; many functions never need it.
	var prelimSynth *Synthesizer
	ensurePrelimSynth := func() *Synthesizer {
		if prelimSynth != nil {
			return prelimSynth
		}
		prelimCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
			Graph:         fnGraph,
			Bindings:      fnGraph.Bindings(),
			BaseScope:     resolveScope,
			DeclaredTypes: overlay,
			GlobalTypes:   globalTypes,
			ModuleAliases: moduleAliases,
		})

		prelimDeps := &Deps{
			Ctx:                    s.deps.Ctx,
			Types:                  s.deps.Types,
			DefaultScope:           resolveScope,
			Manifests:              s.deps.Manifests,
			CheckCtx:               prelimCtx,
			Graphs:                 s.deps.Graphs,
			PreCache:               make(api.Cache),
			NarrowCache:            make(api.Cache),
			FunctionTypeInProgress: s.deps.FunctionTypeInProgress,
			ModuleBindings:         s.deps.ModuleBindings,
			ModuleAliases:          moduleAliases,
			Paths:                  s.deps.Paths,
		}
		prelimSynth = NewSynthesizer(prelimDeps, s.phase)
		return prelimSynth
	}

	// Single-pass local inference from assignments (best-effort).
	var localInferred map[cfg.SymbolID]typ.Type
	fnGraph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal || len(info.Targets) == 0 {
			return
		}
		needsInference := false
		for _, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			if _, exists := overlay[target.Symbol]; !exists {
				needsInference = true
				break
			}
		}
		if !needsInference {
			return
		}
		if len(info.Targets) == 1 && len(info.Sources) == 1 {
			target := info.Targets[0]
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				if _, exists := overlay[target.Symbol]; !exists {
					src := info.Sources[0]
					switch src.(type) {
					case *ast.FuncCallExpr, *ast.Comma3Expr:
					default:
						var t typ.Type
						switch lit := src.(type) {
						case *ast.NilExpr:
							t = typ.Nil
						case *ast.TrueExpr:
							t = typ.True
						case *ast.FalseExpr:
							t = typ.False
						case *ast.StringExpr:
							t = typ.LiteralString(lit.Value)
						}
						if t == nil && len(info.SourceSymbols) > 0 {
							if sym := info.SourceSymbols[0]; sym != 0 {
								if inferred, ok := overlay[sym]; ok && inferred != nil {
									t = inferred
								}
							}
						}
						if t == nil {
							t = ensurePrelimSynth().SynthExpr(src, p, nil)
						}
						if t != nil {
							if localInferred == nil {
								localInferred = make(map[cfg.SymbolID]typ.Type)
							}
							localInferred[target.Symbol] = t
						}
						return
					}
				}
			}
		}
		values := ensurePrelimSynth().ExpandValues(info.Sources, len(info.Targets), p)
		info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			if _, exists := overlay[target.Symbol]; exists {
				return
			}
			if i < len(values) && values[i] != nil {
				if localInferred == nil {
					localInferred = make(map[cfg.SymbolID]typ.Type)
				}
				localInferred[target.Symbol] = values[i]
			}
		})
	})
	for sym, t := range localInferred {
		if _, exists := overlay[sym]; !exists {
			overlay[sym] = t
		}
	}

	// Apply the same direct mutation enrichment law used by return inference:
	// returned locals must reflect visible field/index/direct container writes
	// before return expressions are synthesized.
	mutationBindings := fnGraph.Bindings()
	if mutationBindings == nil {
		mutationBindings = s.deps.ModuleBindings
	}
	if mutationBindings != nil {
		enrichedSynth := func(expr ast.Expr, p cfg.Point) typ.Type {
			if ident, ok := expr.(*ast.IdentExpr); ok {
				if sym, found := mutationBindings.SymbolOf(ident); found && sym != 0 {
					if t := overlay[sym]; t != nil {
						return t
					}
				}
			}
			return ensurePrelimSynth().SynthExpr(expr, p, nil)
		}

		fieldAssignments := overlaymut.CollectFieldAssignments(fnGraph, enrichedSynth, nil)
		overlaymut.ApplyFieldMergeToOverlay(overlay, fieldAssignments)

		indexerAssignments := overlaymut.CollectIndexerAssignments(fnGraph, enrichedSynth, mutationBindings, nil)
		tableMutations := mutator.CollectTableInsertMutations(fnGraph, enrichedSynth, mutationBindings)
		mutator.MergeIndexerMutations(indexerAssignments, tableMutations)
		overlaymut.ApplyIndexerMergeToOverlay(overlay, indexerAssignments)

		directMutations := mutator.CollectTableInsertOnDirect(fnGraph, enrichedSynth, mutationBindings)
		overlaymut.ApplyDirectMutationsToOverlay(overlay, directMutations)
	}

	// Phase 2: build final context with enriched overlay for return inference.
	fnCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:         fnGraph,
		Bindings:      fnGraph.Bindings(),
		BaseScope:     resolveScope,
		DeclaredTypes: overlay,
		GlobalTypes:   globalTypes,
		ModuleAliases: moduleAliases,
	})

	tempDeps := &Deps{
		Ctx:                    s.deps.Ctx,
		Types:                  s.deps.Types,
		DefaultScope:           resolveScope,
		Manifests:              s.deps.Manifests,
		CheckCtx:               fnCheckCtx,
		Graphs:                 s.deps.Graphs,
		PreCache:               make(api.Cache),
		NarrowCache:            make(api.Cache),
		FunctionTypeInProgress: s.deps.FunctionTypeInProgress,
		ModuleBindings:         s.deps.ModuleBindings,
		ModuleAliases:          moduleAliases,
		Paths:                  s.deps.Paths,
	}
	if s.IsNarrowing() && s.deps.Flow != nil && s.deps.CheckCtx != nil {
		if currentGraph, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && currentGraph == fnGraph {
			tempDeps.Flow = s.deps.Flow
		}
	}
	tempSynth := NewSynthesizer(tempDeps, s.phase)

	var returnTypes []typ.Type
	seenReturn := false
	fnGraph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		if tempDeps.Flow != nil {
			if tempDeps.Flow.IsPointDead(p) {
				return
			}
		}
		types := tempSynth.inferReturnExprTypes(info.Exprs, p)

		if !seenReturn {
			seenReturn = true
			returnTypes = types
			return
		}

		// Extend returnTypes for new positions: previous returns contributed nil there.
		for len(returnTypes) < len(types) {
			returnTypes = append(returnTypes, typ.Nil)
		}

		// Merge position-wise, padding current return with nil for missing positions.
		for i := range returnTypes {
			var t typ.Type
			if i < len(types) {
				t = types[i]
			} else {
				t = typ.Nil
			}
			returnTypes[i] = typ.JoinReturnSlot(returnTypes[i], t)
		}
	})

	// Normalize nil elements to typ.Unknown so downstream builders never see nil.
	for i, t := range returnTypes {
		if t == nil {
			returnTypes[i] = typ.Unknown
		}
	}

	if typ.IsUnknownOnlyOrEmpty(returnTypes) && len(summaryFallback) > 0 {
		return summaryFallback, false
	}

	return returnTypes, erreffect.HasStrictInverseReturnPattern(fnGraph, nil, tempSynth, 0, 1)
}

func enrichOverlayWithOrderedComparisonHints(fnGraph *cfg.Graph, overlay map[cfg.SymbolID]typ.Type) {
	if fnGraph == nil || len(overlay) == 0 {
		return
	}
	bindings := fnGraph.Bindings()
	if bindings == nil {
		return
	}

	applyHint := func(expr ast.Expr, hinted typ.Type) {
		if hinted == nil || expr == nil {
			return
		}
		ident, ok := expr.(*ast.IdentExpr)
		if !ok || ident == nil {
			return
		}
		sym, ok := bindings.SymbolOf(ident)
		if !ok || sym == 0 {
			return
		}
		existing := overlay[sym]
		if existing == nil {
			overlay[sym] = hinted
			return
		}
		overlay[sym] = typ.JoinPreferNonSoft(existing, hinted)
	}

	var visit func(ast.Expr)
	visit = func(expr ast.Expr) {
		switch e := expr.(type) {
		case *ast.LogicalOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
		case *ast.RelationalOpExpr:
			switch e.Operator {
			case "<", "<=", ">", ">=":
				applyHint(e.Lhs, orderedLiteralType(e.Rhs))
				applyHint(e.Rhs, orderedLiteralType(e.Lhs))
			}
		}
	}

	fnGraph.EachBranch(func(_ cfg.Point, info *cfg.BranchInfo) {
		if info == nil || info.Condition == nil {
			return
		}
		visit(info.Condition)
	})
}

func orderedLiteralType(expr ast.Expr) typ.Type {
	switch expr.(type) {
	case *ast.NumberExpr:
		return typ.Number
	case *ast.StringExpr:
		return typ.String
	default:
		return nil
	}
}

func localFunctionSymbol(graph *cfg.Graph, fn *ast.FunctionExpr) cfg.SymbolID {
	if graph == nil || fn == nil {
		return 0
	}
	if bindings := graph.Bindings(); bindings != nil {
		if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
			if graph.NameOf(sym) != "" {
				return sym
			}
		}
	}
	var fnSym cfg.SymbolID
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if fnSym != 0 || info == nil || !info.IsLocal || len(info.Targets) == 0 {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			if source == fn {
				fnSym = target.Symbol
			}
		})
	})
	if fnSym != 0 {
		return fnSym
	}
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if fnSym != 0 || info == nil || info.Symbol == 0 {
			return
		}
		if info.FuncExpr == fn {
			fnSym = info.Symbol
		}
	})
	return fnSym
}

// inferReturnExprTypes synthesizes types from return expressions using CFG point.
// The last expression is expanded via MultiTypeOf to support multi-return calls.
func (s *Synthesizer) inferReturnExprTypes(exprs []ast.Expr, p cfg.Point) []typ.Type {
	if len(exprs) == 0 {
		return nil
	}
	var narrower api.FlowOps
	if s.IsNarrowing() && s.deps.Flow != nil {
		narrower = s.deps.Flow
	}
	var result []typ.Type
	for i, expr := range exprs {
		if i == len(exprs)-1 {
			multi := s.multiTypeOf(expr, p, narrower)
			if len(multi) == 0 {
				multi = []typ.Type{typ.Unknown}
			} else {
				for j, mt := range multi {
					if mt == nil {
						multi[j] = typ.Unknown
					}
				}
			}
			result = append(result, multi...)
		} else {
			t := s.SynthExpr(expr, p, narrower)
			if t == nil {
				t = typ.Unknown
			}
			result = append(result, t)
		}
	}
	return result
}

// buildFunctionTypeWithSummary builds a function type using annotations for parameters
// and ReturnSummaries for return types. Does not recursively infer return types.
func (s *Synthesizer) buildFunctionTypeWithSummary(
	fn *ast.FunctionExpr,
	sc *scope.State,
	sym cfg.SymbolID,
	returnSummaries map[cfg.SymbolID][]typ.Type,
) *typ.Function {
	if fn == nil {
		return nil
	}

	// Get signature from annotations only (no return inference)
	sig := s.ResolveFunctionSignature(fn, sc)
	if sig == nil {
		return nil
	}

	// If function has explicit return types, use them
	if len(fn.ReturnTypes) > 0 {
		return sig
	}

	// Look up return types from summaries
	var returnTypes []typ.Type
	if returnSummaries != nil && sym != 0 {
		returnTypes = returnSummaries[sym]
	}

	return join.WithReturnsOrUnknown(sig, returnTypes)
}

func (s *Synthesizer) buildFunctionTypeSummaryFallback(
	fn *ast.FunctionExpr,
	sc *scope.State,
	expected *typ.Function,
) *typ.Function {
	if fn == nil {
		return nil
	}
	sig := s.ResolveFunctionSignature(fn, sc)
	if sig == nil {
		return nil
	}
	if expected != nil && len(sig.Returns) == 0 && len(expected.Returns) > 0 {
		sig = join.WithReturns(sig, expected.Returns)
	}
	var summaries map[cfg.SymbolID][]typ.Type
	if s.deps.CheckCtx != nil {
		if s.IsNarrowing() {
			if ctx, ok := s.deps.CheckCtx.(api.NarrowEnv); ok {
				summaries = ctx.NarrowReturnSummaries()
			}
		} else if ctx, ok := s.deps.CheckCtx.(api.DeclaredEnv); ok {
			summaries = ctx.ReturnSummaries()
		}
	}
	var fnSym cfg.SymbolID
	if s.deps.CheckCtx != nil {
		if pg, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && pg != nil {
			fnSym = localFunctionSymbol(pg, fn)
		}
	}
	if fnSym != 0 {
		return join.WithReturnsOrUnknown(sig, summaries[fnSym])
	}
	return join.WithReturnsOrUnknown(sig, nil)
}

func (s *Synthesizer) buildParamOverlay(fnGraph *cfg.Graph, sc *scope.State, expected *typ.Function) map[cfg.SymbolID]typ.Type {
	paramSlots := fnGraph.ParamSlotsReadOnly()
	overlay := make(map[cfg.SymbolID]typ.Type, len(paramSlots))
	for _, slot := range paramSlots {
		if slot.Symbol == 0 {
			continue
		}

		srcIdx, hasSource := slot.SourceParamIndex()
		if !hasSource {
			if selfType := sc.SelfType(); selfType != nil {
				overlay[slot.Symbol] = selfType
			} else {
				overlay[slot.Symbol] = typ.Unknown
			}
			continue
		}

		i := srcIdx
		paramType := typ.Unknown
		if slot.TypeAnnotation != nil {
			paramType = s.ResolveType(slot.TypeAnnotation, sc)
		} else if expected != nil && i < len(expected.Params) {
			paramType = expected.Params[i].Type
		} else if slot.Name == "self" && sc != nil && sc.SelfType() != nil {
			paramType = sc.SelfType()
		}
		overlay[slot.Symbol] = paramType
	}
	return overlay
}

// inferCallbackOverlaySpec detects the "setup -> param call -> cleanup" pattern
// and builds a contract.Spec with EnvOverlay for each callback parameter.
func (s *Synthesizer) inferCallbackOverlaySpec(
	fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function, fnGraph *cfg.Graph,
) *contract.Spec {
	if fnGraph == nil || fn.ParList == nil || len(fn.ParList.Names) == 0 {
		return nil
	}

	paramSlots := fnGraph.ParamSlotsReadOnly()
	if len(paramSlots) == 0 {
		return nil
	}

	var tempSynth *Synthesizer
	synthExpr := func(expr ast.Expr, p cfg.Point) typ.Type {
		if tempSynth == nil {
			overlay := s.buildParamOverlay(fnGraph, sc, expected)

			var globalTypes map[string]typ.Type
			var moduleAliases map[cfg.SymbolID]string
			if s.deps.CheckCtx != nil {
				globalTypes = s.deps.CheckCtx.GlobalTypes()
				moduleAliases = s.deps.CheckCtx.ModuleAliases()
			}
			if moduleAliases == nil {
				moduleAliases = s.deps.ModuleAliases
			}

			fnCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
				Graph:         fnGraph,
				Bindings:      fnGraph.Bindings(),
				BaseScope:     sc,
				DeclaredTypes: overlay,
				GlobalTypes:   globalTypes,
				ModuleAliases: moduleAliases,
			})
			tempDeps := &Deps{
				Ctx:            s.deps.Ctx,
				Types:          s.deps.Types,
				DefaultScope:   sc,
				Manifests:      s.deps.Manifests,
				CheckCtx:       fnCheckCtx,
				Graphs:         s.deps.Graphs,
				PreCache:       make(api.Cache),
				NarrowCache:    make(api.Cache),
				ModuleBindings: s.deps.ModuleBindings,
				ModuleAliases:  moduleAliases,
			}
			tempSynth = NewSynthesizer(tempDeps, api.PhaseTypeResolution)
		}
		return tempSynth.SynthExpr(expr, p, nil)
	}

	overlays := inferCallbackEnvOverlays(fnGraph, paramSlots, synthExpr, s.deps.ModuleBindings)
	if len(overlays) == 0 {
		return nil
	}

	spec := contract.NewSpec()
	for paramIdx, ov := range overlays {
		spec.WithCallback(paramIdx, &contract.CallbackSpec{
			Cardinality: contract.CardExactlyOnce,
			EnvOverlay:  ov,
		})
	}
	return spec
}

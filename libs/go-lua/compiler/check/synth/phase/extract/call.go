package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CallQuery adapts the Synthesizer's type operations into the core.TypeOps interface.
//
// This allows the ops package (which handles generic call synthesis) to query
// methods, fields, and perform subtype checks without directly depending on
// the synthesizer. Instantiated type parameters are automatically substituted
// when looking up methods/fields.
type CallQuery struct {
	s *Synthesizer
}

func (q CallQuery) Method(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if q.s == nil || q.s.deps.Types == nil {
		return nil, false
	}
	mt, ok := q.s.deps.Types.Method(ctx, t, name)
	if !ok || mt == nil {
		return nil, false
	}
	recv := unwrap.Optional(unwrap.Alias(t))
	if inst, ok := recv.(*typ.Instantiated); ok && inst.Generic != nil {
		mt = subst.Params(mt, inst.Generic.TypeParams, inst.TypeArgs)
	}
	return mt, true
}

func (q CallQuery) Field(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if q.s == nil || q.s.deps.Types == nil {
		return nil, false
	}
	return q.s.deps.Types.Field(ctx, t, name)
}

func (q CallQuery) Index(ctx *db.QueryContext, t typ.Type, key typ.Type) (typ.Type, bool) {
	if q.s == nil || q.s.deps.Types == nil {
		return nil, false
	}
	return q.s.deps.Types.Index(ctx, t, key)
}

func (q CallQuery) BinaryOp(ctx *db.QueryContext, left typ.Type, op string, right typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return typ.Unknown
	}
	return q.s.deps.Types.BinaryOp(ctx, left, op, right)
}

func (q CallQuery) UnaryOp(ctx *db.QueryContext, op string, operand typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return typ.Unknown
	}
	return q.s.deps.Types.UnaryOp(ctx, op, operand)
}

func (q CallQuery) IsSubtype(ctx *db.QueryContext, sub, super typ.Type) bool {
	if q.s == nil || q.s.deps.Types == nil {
		return subtype.IsSubtype(sub, super)
	}
	return q.s.deps.Types.IsSubtype(ctx, sub, super)
}

func (q CallQuery) ExpandInstantiated(ctx *db.QueryContext, t typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return subst.ExpandInstantiated(t)
	}
	return q.s.deps.Types.ExpandInstantiated(ctx, t)
}

func (q CallQuery) Widen(ctx *db.QueryContext, t typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return subtype.Widen(t)
	}
	return q.s.deps.Types.Widen(ctx, t)
}

func (q CallQuery) WidenForInference(ctx *db.QueryContext, t typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return subtype.WidenForInference(t)
	}
	return q.s.deps.Types.WidenForInference(ctx, t)
}

// GetCallQuery returns a core.TypeOps implementation.
func (s *Synthesizer) GetCallQuery() core.TypeOps {
	return CallQuery{s: s}
}

// SynthCallCore synthesizes return types from a function call expression.
//
// Call synthesis proceeds through several stages:
// 1. Intercept check: Special calls (require, select, type casts) may be handled
// 2. Argument synthesis: Compute types for all argument expressions
// 3. Call pipeline: Infer type args, re-synthesize callbacks, finish call
// 4. Post-call transforms: Apply spec-based return type overrides and effects
//
// For method calls (obj:method()), dispatches to synthMethodCallCoreWithExpected.
func (s *Synthesizer) SynthCallCore(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) []typ.Type {
	return s.synthCallCoreWithCaptureTypes(ex, p, sc, narrower, recurse, nil, nil)
}

// synthCallCoreWithNarrower synthesizes call with narrower context preserved.
func (s *Synthesizer) synthCallCoreWithNarrower(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth, expected typ.Type) []typ.Type {
	return s.synthCallCoreWithCaptureTypes(ex, p, sc, narrower, recurse, expected, nil)
}

func (s *Synthesizer) synthCallCoreWithCaptureTypes(
	ex *ast.FuncCallExpr,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
	recurse ExprSynth,
	expected typ.Type,
	captureTypes map[cfg.SymbolID]typ.Type,
) []typ.Type {
	if callsite.IsMethodLikeExpr(ex) {
		return s.synthMethodCallCoreWithExpected(ex, p, sc, recurse, expected)
	}

	env := intercept.CallEnv{
		Scope:      sc,
		Recurse:    intercept.ExprSynth(recurse),
		TypeLookup: s.declaredTypeLookup(sc),
	}

	chain := s.buildInterceptChain(sc)
	if result := chain.InterceptCall(ex, env); result.Skip {
		return result.Types
	}

	calleeType := recurse(ex.Func)
	if specialized := s.specializedLocalFunctionCalleeType(ex, p, sc, calleeType, captureTypes); specialized != nil {
		calleeType = specialized
	}
	args := synthArgs(ex.Args, recurse)
	typeArgs := s.resolveTypeArgs(ex.TypeArgs, sc)

	def := ops.CallDef{
		Callee:         calleeType,
		Args:           args,
		TypeArgs:       typeArgs,
		Query:          s.GetCallQuery(),
		ExpectedReturn: expected,
	}

	pipeline := NewCallPipeline(s.deps.Ctx, def, ex.Args).
		WithReSynth(s.callbackAwareReSynth(calleeType, sc))

	if expected != nil {
		pipeline = pipeline.WithExpected(expected)
	}

	result := pipeline.Run()

	returns := unwrapCallResult(result)
	returns = s.applyPostCallTransforms(calleeType, args, returns)

	specOverride := s.specReturnOverride(calleeType, ex.Args, args)
	return intercept.ApplyOverride(returns, specOverride)
}

func (s *Synthesizer) specializedLocalFunctionCalleeType(
	ex *ast.FuncCallExpr,
	p cfg.Point,
	sc *scope.State,
	current typ.Type,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	if s == nil || ex == nil || s.deps.CheckCtx == nil {
		return nil
	}
	if specialized := s.stableLocalFunctionValueType(ex.Func, p, sc, current, captureTypes); specialized != nil {
		return specialized
	}
	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return nil
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	if bindings == nil {
		return nil
	}
	info := graph.CallSiteAt(p, ex)
	if info == nil {
		return nil
	}
	for _, sym := range callsite.CallableCalleeSymbolCandidates(info, graph, bindings, nil) {
		fn := callsite.FunctionLiteralForGraphSymbol(graph, sym)
		if fn == nil {
			continue
		}
		expectedFn, _ := unwrap.Optional(unwrap.Alias(current)).(*typ.Function)
		if fnType := s.synthFunctionTypeWithCapturePoint(fn, sc, expectedFn, p, captureTypes); fnType != nil {
			return fnType
		}
	}
	return nil
}

// SynthCallCoreWithExpected synthesizes call with optional expected return type for generic inference.
func (s *Synthesizer) SynthCallCoreWithExpected(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type) []typ.Type {
	return s.synthCallCoreWithNarrower(ex, p, sc, nil, recurse, expected)
}

// synthMethodCallCoreWithExpected synthesizes method call with optional expected return type.
func (s *Synthesizer) synthMethodCallCoreWithExpected(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type) []typ.Type {
	env := intercept.CallEnv{
		Scope:      sc,
		Recurse:    intercept.ExprSynth(recurse),
		TypeLookup: s.declaredTypeLookup(sc),
	}

	chain := s.buildInterceptChain(sc)
	if result := chain.InterceptMethodCall(ex, env); result.Skip {
		return result.Types
	}

	recvType := recurse(ex.Receiver)
	args := synthArgs(ex.Args, recurse)
	calleeType := s.resolveMethodCallee(recvType, ex.Method)

	def := ops.CallDef{
		IsMethod:            true,
		Receiver:            recvType,
		MethodName:          ex.Method,
		Args:                args,
		Query:               s.GetCallQuery(),
		ExpectedReturn:      expected,
		ForceMethodReceiver: s.forceMethodReceiverAtPoint(p, ex),
	}

	pipeline := NewCallPipeline(s.deps.Ctx, def, ex.Args).
		WithReSynth(s.callbackAwareReSynth(calleeType, sc))

	if expected != nil {
		pipeline = pipeline.WithExpected(expected)
	}

	result := pipeline.Run()
	returns := unwrapCallResult(result)
	returns = s.applyPostCallTransforms(calleeType, args, returns)

	specOverride := s.specReturnOverride(calleeType, ex.Args, args)
	return intercept.ApplyOverride(returns, specOverride)
}

// SynthCallWithReceiverType synthesizes method call with an explicit receiver type.
func (s *Synthesizer) SynthCallWithReceiverType(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, recvType typ.Type, recurse ExprSynth) []typ.Type {
	env := intercept.CallEnv{
		Scope:      sc,
		Recurse:    intercept.ExprSynth(recurse),
		TypeLookup: s.declaredTypeLookup(sc),
	}

	chain := s.buildInterceptChain(sc)
	if result := chain.InterceptMethodCall(ex, env); result.Skip {
		return result.Types
	}

	args := synthArgs(ex.Args, recurse)
	calleeType := s.resolveMethodCallee(recvType, ex.Method)

	def := ops.CallDef{
		IsMethod:            true,
		Receiver:            recvType,
		MethodName:          ex.Method,
		Args:                args,
		Query:               s.GetCallQuery(),
		ForceMethodReceiver: s.forceMethodReceiverAtPoint(p, ex),
	}

	pipeline := NewCallPipeline(s.deps.Ctx, def, ex.Args).
		WithReSynth(s.callbackAwareReSynth(calleeType, sc))

	result := pipeline.Run()
	returns := unwrapCallResult(result)
	returns = s.applyPostCallTransforms(calleeType, args, returns)

	specOverride := s.specReturnOverride(calleeType, ex.Args, args)
	return intercept.ApplyOverride(returns, specOverride)
}

// declaredTypeLookup returns a function that resolves identifier names to their
// declared function types. Used by intercepts for effect-based dispatch.
func (s *Synthesizer) declaredTypeLookup(sc *scope.State) func(string) typ.Type {
	return func(name string) typ.Type {
		// Check global types first (require, select, type, etc.)
		if s.deps.CheckCtx != nil {
			if globalTypes := s.deps.CheckCtx.GlobalTypes(); globalTypes != nil {
				if t, ok := globalTypes[name]; ok {
					return t
				}
			}
		}
		// Check scope metadata for type names (Number, Point, etc.)
		if sc != nil {
			if meta := sc.MetaForName(name); meta != nil {
				return typ.Func().
					Param("value", typ.Any).
					Returns(meta.Of).
					Effects(effect.WithCallableType()).
					Build()
			}
		}
		return nil
	}
}

// buildInterceptChain creates the intercept chain for call synthesis.
func (s *Synthesizer) buildInterceptChain(sc *scope.State) *intercept.Chain {
	builder := intercept.NewChainBuilder()

	if s.deps.Manifests != nil {
		builder.WithManifests(s.deps.Manifests)
	}

	if sc != nil {
		builder.WithVariadicResolver(sc)
	}

	return builder.Build()
}

// synthArgs synthesizes types for argument expressions.
func synthArgs(exprs []ast.Expr, recurse ExprSynth) []typ.Type {
	args := make([]typ.Type, len(exprs))
	for i, arg := range exprs {
		args[i] = recurse(arg)
	}
	return args
}

// resolveTypeArgs resolves explicit type arguments.
func (s *Synthesizer) resolveTypeArgs(typeExprs []ast.TypeExpr, sc *scope.State) []typ.Type {
	if len(typeExprs) == 0 || sc == nil {
		return nil
	}
	typeArgs := make([]typ.Type, len(typeExprs))
	for i, ta := range typeExprs {
		typeArgs[i] = s.ResolveType(ta, sc)
	}
	return typeArgs
}

// resolveMethodCallee resolves the callee type for a method call.
func (s *Synthesizer) resolveMethodCallee(recvType typ.Type, method string) typ.Type {
	if recvType == nil {
		return nil
	}
	if mt, ok := s.Method(recvType, method); ok {
		return mt
	}
	if ft, ok := s.Field(recvType, method); ok {
		return ft
	}
	return nil
}

func (s *Synthesizer) forceMethodReceiverAtPoint(p cfg.Point, ex *ast.FuncCallExpr) bool {
	bindings := s.deps.ModuleBindings
	if bindings == nil && s.deps.CheckCtx != nil {
		bindings = s.deps.CheckCtx.Bindings()
	}
	var graph *compcfg.Graph
	if s.deps.CheckCtx != nil {
		graph, _ = s.deps.CheckCtx.Graph().(*compcfg.Graph)
	}
	return callsite.ForceMethodReceiverAtPoint(bindings, graph, p, ex)
}

// Method looks up a method type on a receiver type.
func (s *Synthesizer) Method(t typ.Type, name string) (typ.Type, bool) {
	if s.deps.Types == nil {
		return nil, false
	}
	mt, ok := s.deps.Types.Method(s.deps.Ctx, t, name)
	if !ok || mt == nil {
		return nil, false
	}
	recv := unwrap.Optional(unwrap.Alias(t))
	if inst, ok := recv.(*typ.Instantiated); ok && inst.Generic != nil {
		mt = subst.Params(mt, inst.Generic.TypeParams, inst.TypeArgs)
	}
	return mt, true
}

// Field looks up a field type on a receiver type.
func (s *Synthesizer) Field(t typ.Type, name string) (typ.Type, bool) {
	if s.deps.Types == nil {
		return nil, false
	}
	ft, ok := s.deps.Types.Field(s.deps.Ctx, t, name)
	if !ok || ft == nil {
		return nil, false
	}
	recv := unwrap.Optional(unwrap.Alias(t))
	if inst, ok := recv.(*typ.Instantiated); ok && inst.Generic != nil {
		ft = subst.Params(ft, inst.Generic.TypeParams, inst.TypeArgs)
	}
	return ft, true
}

// specReturnOverride computes spec-based return type override.
func (s *Synthesizer) specReturnOverride(fnType typ.Type, astArgs []ast.Expr, argTypes []typ.Type) typ.Type {
	phase := api.PhaseScopeCompute
	if s.IsNarrowing() {
		phase = api.PhaseNarrowing
	}

	override := &intercept.SpecReturnOverride{
		Phase: phase,
	}
	if result := override.Override(fnType, astArgs); result != nil {
		return result
	}

	fn := intercept.ResolveSpecFunction(fnType)
	if fn == nil {
		return nil
	}
	return transform.ApplySpecReturnCases(fn, argTypes)
}

// unwrapCallResult converts CallResult to a slice of types.
func unwrapCallResult(result ops.CallResult) []typ.Type {
	if len(result.Returns) > 0 {
		return CopyTypes(result.Returns)
	}
	if tuple, ok := result.Type.(*typ.Tuple); ok {
		return CopyTypes(tuple.Elements)
	}
	return []typ.Type{result.Type}
}

// CopyTypes returns a copy of a type slice.
func CopyTypes(types []typ.Type) []typ.Type {
	if len(types) == 0 {
		return nil
	}
	result := make([]typ.Type, len(types))
	copy(result, types)
	return result
}

// applyPostCallTransforms applies effect-based return type transforms.
func (s *Synthesizer) applyPostCallTransforms(calleeType typ.Type, args []typ.Type, returns []typ.Type) []typ.Type {
	if len(returns) == 0 {
		return returns
	}

	fn := intercept.ResolveSpecFunction(calleeType)
	if fn == nil {
		return returns
	}

	var result []typ.Type
	for i := range returns {
		transformed := transform.ApplyEffectTransform(fn, args, i, returns[i])
		if transformed == nil || transformed == returns[i] {
			continue
		}
		if result == nil {
			result = make([]typ.Type, len(returns))
			copy(result, returns)
		}
		result[i] = transformed
	}
	if result != nil {
		return result
	}

	return returns
}

// callbackAwareReSynth creates an ArgReSynth that applies EnvOverlay from callback specs.
// For callback parameters with an EnvOverlay, the overlay globals are merged into the
// synthesizer's context so they are visible inside the callback body only.
func (s *Synthesizer) callbackAwareReSynth(calleeType typ.Type, sc *scope.State) ArgReSynth {
	return func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		expectedFn, ok := unwrap.Alias(expected).(*typ.Function)
		if !ok {
			return nil
		}

		fnExpr, ok := arg.(*ast.FunctionExpr)
		if !ok {
			ident, isIdent := arg.(*ast.IdentExpr)
			if !isIdent {
				return nil
			}
			fnExpr = s.functionLiteralForIdent(ident)
			if fnExpr == nil {
				return nil
			}
		}

		synthFn := s.SynthFunctionTypeWithExpected
		if overlay := callbackEnvOverlay(calleeType, idx); len(overlay) > 0 {
			synthFn = s.withEnvOverlay(overlay).SynthFunctionTypeWithExpected
		}
		ft := synthFn(fnExpr, sc, expectedFn)
		if ft == nil {
			return nil
		}
		return ft
	}
}

// callbackEnvOverlay extracts the EnvOverlay for a callback at the given parameter index.
func callbackEnvOverlay(calleeType typ.Type, paramIdx int) map[string]typ.Type {
	fn := intercept.ResolveSpecFunction(calleeType)
	if fn == nil || fn.Spec == nil {
		return nil
	}
	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return nil
	}
	cb := spec.GetCallback(paramIdx)
	if cb == nil || len(cb.EnvOverlay) == 0 {
		return nil
	}
	return cb.EnvOverlay
}

// withEnvOverlay returns a new Synthesizer with additional globals merged into the context.
func (s *Synthesizer) withEnvOverlay(overlay map[string]typ.Type) *Synthesizer {
	overlaidCtx := s.deps.CheckCtx
	if overlaidCtx != nil {
		overlaidCtx = overlaidCtx.WithGlobalOverlay(overlay)
	}
	overlaidDeps := &Deps{
		Ctx:                    s.deps.Ctx,
		Types:                  s.deps.Types,
		Scopes:                 s.deps.Scopes,
		Manifests:              s.deps.Manifests,
		CheckCtx:               overlaidCtx,
		Graphs:                 s.deps.Graphs,
		Flow:                   s.deps.Flow,
		Paths:                  s.deps.Paths,
		PreCache:               make(api.Cache),
		NarrowCache:            make(api.Cache),
		FunctionTypeInProgress: s.deps.FunctionTypeInProgress,
		ModuleBindings:         s.deps.ModuleBindings,
		ModuleAliases:          s.deps.ModuleAliases,
	}
	return NewSynthesizer(overlaidDeps, s.phase)
}

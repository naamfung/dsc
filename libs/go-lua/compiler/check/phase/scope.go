// scope.go implements Phase B (scope computation) of the analysis pipeline.
// This phase builds lexical scope states for each CFG point and extracts
// declared types from annotations.
//
// # SCOPE COMPUTATION
//
// Scopes are computed by walking the CFG in reverse postorder (RPO):
//  1. Start with base scope containing parameters and type parameters
//  2. For each node, merge predecessor scopes and apply node effects
//  3. ScopeEnter creates child scope; ScopeExit returns to parent
//  4. Assignments track local variable declarations
//  5. TypeDefs add type aliases to the scope
//
// # DECLARED TYPES
//
// Declared types are extracted from:
//   - Parameter type annotations
//   - Local variable type annotations
//   - Global type lookups
//   - Function definition signatures
//
// The output DeclaredTypes map is used by flow extraction and solving.
package phase

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ScopeGraph defines the CFG interface required for scope computation.
type ScopeGraph interface {
	basecfg.VersionedGraph
	Call(p cfg.Point) *cfg.CallInfo
	Assign(p cfg.Point) *cfg.AssignInfo
	FuncDef(p cfg.Point) *cfg.FuncDefInfo
	TypeDef(p cfg.Point) *cfg.TypeDefInfo
}

// scopeTypeResolver resolves a type definition to a concrete type at a CFG point.
type scopeTypeResolver func(info *cfg.TypeDefInfo, p cfg.Point, sc *scope.State) typ.Type

// scopeExprSynthesizer synthesizes the type of an expression at a CFG point.
type scopeExprSynthesizer func(expr ast.Expr, p cfg.Point, sc *scope.State) typ.Type

// scopeCallMutator applies call effects to update scope at a CFG point.
type scopeCallMutator func(info *cfg.CallInfo, p cfg.Point, sc *scope.State) *scope.State

// ScopeServices provides explicit dependencies for scope computation.
// This replaces the callback-heavy config with a single cohesive interface.
type ScopeServices interface {
	// ResolveTypeDef resolves type definition nodes to concrete types.
	ResolveTypeDef(info *cfg.TypeDefInfo, p cfg.Point, sc *scope.State) typ.Type
	// MutateCall applies call effects to scope state at a CFG point.
	MutateCall(info *cfg.CallInfo, p cfg.Point, sc *scope.State) *scope.State
}

// ScopeServicesFuncs is a simple adapter for providing ScopeServices via functions.
// Use this in tests or when wiring phase inputs without creating a dedicated type.
type ScopeServicesFuncs struct {
	TypeResolver scopeTypeResolver
	CallMutator  scopeCallMutator
}

func (s ScopeServicesFuncs) ResolveTypeDef(info *cfg.TypeDefInfo, p cfg.Point, sc *scope.State) typ.Type {
	if s.TypeResolver == nil {
		return nil
	}
	return s.TypeResolver(info, p, sc)
}

func (s ScopeServicesFuncs) MutateCall(info *cfg.CallInfo, p cfg.Point, sc *scope.State) *scope.State {
	if s.CallMutator == nil {
		return sc
	}
	return s.CallMutator(info, p, sc)
}

// ScopeOptions controls scope computation limits.
type ScopeOptions struct {
	MaxDepth      int
	DepthExceeded *bool
}

// RunScope executes Phase B (scope computation) and returns scope states and declared types.
//
// This phase:
//  1. Builds the function's base scope from parameters and type parameters
//  2. Extracts parameter types from annotations and synthesized signatures
//  3. Creates type resolution engine for this function
//  4. Computes scope state at each CFG point via RPO traversal
//  5. Extracts declared types from annotations throughout the function
//
// The output provides scope context for constraint extraction and type narrowing.
func RunScope(input ScopeInput) ScopeOutput {
	typeExprResolver := input.Resolve.TypeResolver

	depthExceeded := false
	base := BuildFunctionScope(input.Fn, input.Parent, typeExprResolver, input.MaxScopeDepth, &depthExceeded)

	var synthSig *typ.Function
	if input.SynthesizedFunctionSig != nil {
		synthSig = input.SynthesizedFunctionSig
	}

	var hints []typ.Type
	if input.ParamHintSignatures != nil && input.Fn != nil {
		hints = input.ParamHintSignatures[input.Fn]
	}
	paramTypes, paramAnnotated := ExtractParamTypes(input.Graph, input.Fn, typeExprResolver, synthSig, base, hints)

	// Inject synthesized self type into base scope only if base doesn't already
	// have a more specific self type (set by processNestedFunctions from field assignment context).
	if synthSig != nil && input.Fn != nil && input.Fn.ParList != nil {
		for i, name := range input.Fn.ParList.Names {
			if name == "" {
				continue
			}
			if input.Fn.ParList.Types != nil && i < len(input.Fn.ParList.Types) && input.Fn.ParList.Types[i] != nil {
				continue
			}
			if i < len(synthSig.Params) && synthSig.Params[i].Type != nil {
				if name == "self" && base.SelfType() == nil {
					base = base.WithSelf(synthSig.Params[i].Type)
				}
			}
		}
	}

	typeResolutionEngine := CreateTypeResolutionEngine(
		input.Ctx,
		input.Graph,
		input.GlobalTypes,
		paramTypes,
		base,
		input.Types,
		input.Manifests,
	)

	localTypeAnnotations := make(map[cfg.SymbolID]ast.TypeExpr)
	for _, p := range input.Graph.RPO() {
		if info := input.Graph.Assign(p); info != nil && info.IsLocal && len(info.TypeAnnotations) > 0 {
			info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Name == "" {
					return
				}
				ann := info.TypeAnnotationAt(i)
				if ann == nil {
					return
				}
				sym, ok := input.Graph.SymbolAt(p, target.Name)
				if !ok || sym == 0 {
					return
				}
				if _, exists := localTypeAnnotations[sym]; !exists {
					localTypeAnnotations[sym] = ann
				}
			})
		}
	}

	typeResolver := func(info *cfg.TypeDefInfo, p cfg.Point, sc *scope.State) typ.Type {
		if info != nil && info.TypeExpr != nil {
			if typeOf, ok := info.TypeExpr.(*ast.TypeOfExpr); ok {
				if ident, ok := typeOf.Expr.(*ast.IdentExpr); ok && typeExprResolver != nil {
					if sym, ok := input.Graph.SymbolAt(p, ident.Value); ok && sym != 0 {
						if ann, ok := localTypeAnnotations[sym]; ok && ann != nil {
							if resolved := typeExprResolver.ResolveType(ann, sc); resolved != nil {
								return resolved
							}
						}
					}
				}
			}
		}
		return typeResolutionEngine.ResolveTypeDefAt(info.Name, info.TypeExpr, scope.ToTypeParamExprs(info.TypeParams), sc, p)
	}
	exprSynth := func(expr ast.Expr, p cfg.Point, sc *scope.State) typ.Type {
		return typeResolutionEngine.SynthExprAt(expr, p, sc)
	}
	fnSignatureResolver := buildFnSignatureResolver(input.FunctionLiteralSignatures, input.ParamHintSignatures, typeResolutionEngine)

	callMutator := buildCallMutator(input.Types, input.Ctx, exprSynth)
	services := ScopeServicesFuncs{
		TypeResolver: typeResolver,
		CallMutator:  callMutator,
	}
	scopes := ComputeScopes(input.Graph, base, services, ScopeOptions{
		MaxDepth:      input.MaxScopeDepth,
		DepthExceeded: &depthExceeded,
	})

	declaredTypes, annotatedVars := buildDeclaredTypes(
		input.Graph,
		input.GlobalTypes,
		paramTypes,
		paramAnnotated,
		scopes,
		typeExprResolver,
		fnSignatureResolver,
		typeResolutionEngine,
		input.SiblingTypes,
		input.ReturnSummaries,
	)
	declaredTypes = applyModuleAliasExports(declaredTypes, input.ModuleAliases, input.Manifests)

	return ScopeOutput{
		BaseScope:                 base,
		Scopes:                    scopes,
		DeclaredTypes:             declaredTypes,
		AnnotatedVars:             annotatedVars,
		ParamTypes:                paramTypes,
		FunctionSignatureResolver: fnSignatureResolver,
		SiblingTypes:              input.SiblingTypes,
		DepthLimitExceeded:        depthExceeded,
	}
}

// buildFnSignatureResolver creates a function signature resolver that combines
// pre-computed literal signatures, parameter hints, and annotation-based resolution.
func buildFnSignatureResolver(
	literalSigs LiteralSigsProvider,
	paramHints map[*ast.FunctionExpr][]typ.Type,
	engine *synth.Engine,
) FunctionSignatureResolver {
	return FunctionSignatureResolverFunc(func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
		var sig *typ.Function
		if literalSigs != nil {
			if s := literalSigs.Lookup(fn); s != nil {
				sig = s
			}
		}
		if sig == nil {
			sig = engine.ResolveFunctionSignature(fn, sc)
		}
		if sig == nil {
			return nil
		}
		if paramHints == nil {
			return sig
		}
		hints := paramHints[fn]
		if len(hints) == 0 {
			return sig
		}
		return paramhints.MergeIntoSignature(fn, hints, sig)
	})
}

// extractParamTypes extracts parameter types from function definition.
// Returns param types and a set of symbols that have explicit annotations.
func ExtractParamTypes(
	graph *cfg.Graph,
	fn *ast.FunctionExpr,
	typeExprResolver TypeResolver,
	synthSig *typ.Function,
	base *scope.State,
	paramHints []typ.Type,
) (types map[cfg.SymbolID]typ.Type, annotated map[cfg.SymbolID]bool) {
	if fn == nil || fn.ParList == nil || graph == nil {
		return nil, nil
	}

	types = make(map[cfg.SymbolID]typ.Type)
	annotated = make(map[cfg.SymbolID]bool)

	slots := graph.ParamSlotsReadOnly()
	for _, slot := range slots {
		if slot.Symbol == 0 || slot.Name == "" {
			continue
		}

		// Binder/CFG-injected implicit self parameter has no source annotation.
		srcIdx, hasSource := slot.SourceParamIndex()
		if !hasSource {
			if base != nil && base.SelfType() != nil {
				types[slot.Symbol] = base.SelfType()
			} else {
				types[slot.Symbol] = typ.Unknown
			}
			continue
		}
		i := srcIdx

		var paramType typ.Type
		var hint typ.Type
		if paramHints != nil && i < len(paramHints) {
			hint = paramHints[i]
		}
		var isAnnotated bool
		var hasExplicitAnnotation bool
		if slot.TypeAnnotation != nil {
			if typeExprResolver != nil {
				paramType = typeExprResolver.ResolveType(slot.TypeAnnotation, base)
			} else {
				paramType = typ.Unknown
			}
			if typ.IsRefinableAnnotation(paramType) {
				if hint != nil {
					paramType = hint
				} else if synthSig != nil && i < len(synthSig.Params) && synthSig.Params[i].Type != nil {
					paramType = synthSig.Params[i].Type
				}
			} else {
				isAnnotated = true
				hasExplicitAnnotation = true
			}
		} else if hint != nil {
			paramType = hint
		} else if synthSig != nil && i < len(synthSig.Params) && synthSig.Params[i].Type != nil {
			paramType = synthSig.Params[i].Type
			isAnnotated = true
		} else if slot.Name == "self" && base != nil && base.SelfType() != nil {
			paramType = base.SelfType()
		} else {
			paramType = typ.Any
		}

		// Prefer scope self type over synthesized Any/Unknown for unannotated self.
		// This allows table-field method analysis to override placeholder literal signatures.
		if slot.Name == "self" && !hasExplicitAnnotation && base != nil && base.SelfType() != nil && paramType != nil {
			if paramType.Kind().IsPlaceholder() {
				paramType = base.SelfType()
			}
		}

		types[slot.Symbol] = paramType
		if isAnnotated {
			annotated[slot.Symbol] = true
		}
	}

	if len(types) == 0 {
		return nil, nil
	}
	return types, annotated
}

// buildDeclaredTypes builds declared types from annotations.
func buildDeclaredTypes(
	graph *cfg.Graph,
	globalTypes map[string]typ.Type,
	paramTypes map[cfg.SymbolID]typ.Type,
	paramAnnotated map[cfg.SymbolID]bool,
	scopes map[cfg.Point]*scope.State,
	typeExprResolver TypeResolver,
	fnSigResolver FunctionSignatureResolver,
	synthAPI api.SynthAPI,
	siblingTypes map[cfg.SymbolID]typ.Type,
	returnSummaries map[cfg.SymbolID][]typ.Type,
) (flow.DeclaredTypes, map[cfg.SymbolID]bool) {
	if graph == nil {
		return nil, nil
	}

	out := make(flow.DeclaredTypes)
	annotated := make(map[cfg.SymbolID]bool)
	bindings := graph.Bindings()
	alignWithSummary := func(sym cfg.SymbolID, fn *typ.Function) *typ.Function {
		if fn == nil || len(returnSummaries) == 0 || sym == 0 {
			return fn
		}
		if summary := returnSummaries[sym]; len(summary) > 0 {
			return returns.WithSummaryOrUnknown(fn, summary)
		}
		return fn
	}

	for _, sym := range cfg.SortedSymbolIDs(paramTypes) {
		t := paramTypes[sym]
		out[sym] = t
		if paramAnnotated[sym] {
			annotated[sym] = true
		}
	}

	// Apply global types once using the graph's global symbol map.
	if len(globalTypes) > 0 {
		for _, name := range cfg.SortedFieldNames(globalTypes) {
			t := globalTypes[name]
			if t == nil {
				continue
			}
			if sym, ok := graph.GlobalSymbol(name); ok {
				if _, exists := out[sym]; exists {
					continue
				}
				if bindings != nil {
					if kind, ok := bindings.Kind(sym); ok && kind != basecfg.SymbolGlobal {
						continue
					}
				}
				out[sym] = t
			}
		}
	}

	for _, p := range graph.RPO() {

		if info := graph.Assign(p); info != nil && info.IsLocal {
			if info.NumericFor != nil {
				target, ok := info.FirstTarget()
				if !ok {
					continue
				}
				if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
					if _, exists := out[target.Symbol]; !exists {
						out[target.Symbol] = typ.Integer
					}
				}
			}

			if len(info.IterExprs) > 0 && len(info.Targets) > 0 && synthAPI != nil {
				varTypes := synthAPI.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, nil)
				info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
					if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
						return
					}
					if _, exists := out[target.Symbol]; exists {
						return
					}
					varType := typ.Unknown
					if i < len(varTypes) && varTypes[i] != nil {
						varType = varTypes[i]
					}
					out[target.Symbol] = varType
				})
			}

			sc := scopes[p]
			info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Name == "" {
					return
				}
				sym, ok := graph.SymbolAt(p, target.Name)
				if !ok {
					return
				}
				if _, exists := out[sym]; exists {
					return
				}

				if fnExpr, ok := source.(*ast.FunctionExpr); ok && fnExpr != nil {
					if siblingTypes != nil {
						if siblingFn := siblingTypes[sym]; siblingFn != nil {
							out[sym] = siblingFn
							return
						}
					}
					if fnSigResolver != nil {
						if fnSig := fnSigResolver.ResolveFunctionSignature(fnExpr, sc); fnSig != nil {
							out[sym] = alignWithSummary(sym, fnSig)
						}
					}
					return
				}

				if ann := info.TypeAnnotationAt(i); ann != nil {
					if typeExprResolver != nil {
						if resolved := typeExprResolver.ResolveType(ann, sc); resolved != nil {
							out[sym] = resolved
							if !typ.IsSoft(resolved, typ.SoftAnnotationPolicy) {
								annotated[sym] = true
							}
						}
					}
				}
			})
		}

		if info := graph.FuncDef(p); info != nil && info.Name != "" && info.FuncExpr != nil {
			sym := info.Symbol
			if sym == 0 {
				// Fallback for unresolved symbols in legacy/broken binding scenarios.
				var ok bool
				sym, ok = graph.SymbolAt(p, info.Name)
				if !ok {
					continue
				}
			}
			if _, exists := out[sym]; exists {
				continue
			}
			if siblingTypes != nil {
				if siblingFn := siblingTypes[sym]; siblingFn != nil {
					out[sym] = siblingFn
					continue
				}
			}
			sc := scopes[p]
			if fnSigResolver != nil {
				if fnSig := fnSigResolver.ResolveFunctionSignature(info.FuncExpr, sc); fnSig != nil {
					out[sym] = alignWithSummary(sym, fnSig)
				}
			}
		}
	}

	if len(out) == 0 {
		return nil, nil
	}
	return out, annotated
}

// ComputeScopes walks the CFG in reverse postorder and computes scope state at each point.
// Scope state tracks the type namespace (type aliases, type parameters) and lexical metadata
// (local names, mutation flags). Value types are tracked separately in DeclaredTypes.
//
// NODE EFFECTS:
//   - ScopeEnter: Creates child scope, marks loop locals
//   - ScopeExit: Returns to parent scope, merges mutation metadata
//   - Assign: Marks local variable names
//   - TypeDef: Adds type alias to scope
//   - Call: Applies call effects (currently no-op, reserved for future use)
//
// MERGE SEMANTICS: When multiple predecessors exist, scopes are merged by
// taking the first predecessor's scope and adding mutation flags from others.
// Type namespace is assumed consistent across merge points.
func ComputeScopes(graph ScopeGraph, base *scope.State, services ScopeServices, opts ScopeOptions) map[cfg.Point]*scope.State {
	if graph == nil {
		return nil
	}
	if base == nil {
		base = scope.New()
	}

	result := make(map[cfg.Point]*scope.State)
	result[graph.Entry()] = base

	for _, p := range graph.RPO() {
		if p == graph.Entry() {
			continue
		}

		current := scopeFromPredecessor(graph, result, p, base)
		node := graph.Node(p)
		if node == nil {
			result[p] = current
			continue
		}

		switch node.Kind {
		case basecfg.NodeScopeEnter:
			current = enterScope(current, node, graph, opts)
		case basecfg.NodeScopeExit:
			if parent := current.Parent(); parent != nil {
				current = scope.MergeScopeExit(parent, current)
			} else {
				current = base
			}
		case basecfg.NodeAssign:
			current = applyAssign(graph, p, current)
		case basecfg.NodeCall:
			if services != nil {
				if info := graph.Call(p); info != nil {
					current = services.MutateCall(info, p, current)
				}
			}
		case basecfg.NodeTypeDef:
			current = applyTypeDef(graph, p, current, services)
		}

		result[p] = current
	}

	return result
}

func scopeFromPredecessor(graph ScopeGraph, result map[cfg.Point]*scope.State, p cfg.Point, base *scope.State) *scope.State {
	preds := graph.Predecessors(p)
	if len(preds) == 0 {
		return base
	}

	var merged *scope.State
	if s, ok := result[preds[0]]; ok {
		merged = s
	}
	if merged == nil {
		merged = base
	}

	for i := 1; i < len(preds); i++ {
		if s, ok := result[preds[i]]; ok && s != nil {
			merged = mergeScopes(merged, s)
		}
	}

	return merged
}

func enterScope(current *scope.State, node *basecfg.Node, graph ScopeGraph, opts ScopeOptions) *scope.State {
	if current == nil {
		current = scope.New()
	}
	if opts.MaxDepth > 0 && current.Depth()+1 > opts.MaxDepth {
		if opts.DepthExceeded != nil {
			*opts.DepthExceeded = true
		}
		return current
	}
	child := current.Child()
	if len(node.LoopLocals) > 0 && graph != nil {
		var localNames []string
		for _, sym := range node.LoopLocals {
			if name := graph.NameOf(sym); name != "" {
				localNames = append(localNames, name)
			}
		}
		if len(localNames) > 0 {
			child = child.WithLocalNames(localNames)
		}
	}
	return child
}

func mergeScopes(a *scope.State, b *scope.State) *scope.State {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	// Merge mutation metadata only
	out := a
	var mutated []string
	b.RangeMutations(func(name string) bool {
		if !a.IsMutated(name) {
			mutated = append(mutated, name)
		}
		return true
	})
	if len(mutated) > 0 {
		out = out.WithMutatedNames(mutated)
	}
	return out
}

func applyAssign(graph ScopeGraph, p cfg.Point, current *scope.State) *scope.State {
	// Handle function definitions
	if funcDef := graph.FuncDef(p); funcDef != nil {
		return applyFuncDef(funcDef, current)
	}

	info := graph.Assign(p)
	if info == nil || !info.IsLocal {
		return current
	}

	// Mark local names
	var localNames []string
	info.EachTarget(func(_ int, target cfg.AssignTarget) {
		if target.Kind == cfg.TargetIdent && target.Name != "" {
			localNames = append(localNames, target.Name)
		}
	})
	if len(localNames) > 0 {
		current = current.WithLocalNames(localNames)
	}

	return current
}

func applyFuncDef(info *cfg.FuncDefInfo, current *scope.State) *scope.State {
	if info.Name == "" || info.FuncExpr == nil {
		return current
	}
	switch info.TargetKind {
	case cfg.FuncDefGlobal:
		return current.WithLocalName(info.Name)
	}
	return current
}

func applyTypeDef(graph ScopeGraph, p cfg.Point, current *scope.State, services ScopeServices) *scope.State {
	info := graph.TypeDef(p)
	if info == nil || info.Name == "" {
		return current
	}
	var resolved typ.Type
	if services != nil {
		resolved = services.ResolveTypeDef(info, p, current)
	}
	if resolved == nil {
		resolved = typ.Unknown
	}
	if _, isGeneric := resolved.(*typ.Generic); isGeneric {
		return current.WithType(info.Name, resolved)
	}
	return current.WithType(info.Name, typ.NewAlias(info.Name, resolved))
}

// BuildFunctionScope creates the initial base scope for a function.
// This scope contains:
//   - Type parameters for generic functions
//   - Parameter names marked as local
//   - Variadic type if function accepts varargs
//
// Note: Parameter VALUE types are not stored in scope; they go in DeclaredTypes.
// Scope only tracks type namespace and lexical metadata.
func BuildFunctionScope(fn *ast.FunctionExpr, parent *scope.State, resolve TypeResolver, maxDepth int, depthExceeded *bool) *scope.State {
	if parent == nil {
		parent = scope.New()
	}
	s := parent
	if maxDepth > 0 && parent.Depth()+1 > maxDepth {
		if depthExceeded != nil {
			*depthExceeded = true
		}
	} else {
		s = parent.Child()
	}

	// Add type parameters for generic functions
	if len(fn.TypeParams) > 0 && resolve != nil {
		typeParams := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constr typ.Type
			if tp.Constraint != nil && resolve != nil {
				constr = resolve.ResolveType(tp.Constraint, s)
			}
			typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constr)
		}
		s = s.WithTypeParams(typeParams)
	}

	if fn.ParList == nil {
		return s
	}

	// Mark parameter names as local
	var localNames []string
	for _, param := range fn.ParList.Names {
		if param != "" {
			localNames = append(localNames, param)
		}
	}
	if len(localNames) > 0 {
		s = s.WithLocalNames(localNames)
	}

	if fn.ParList.HasVargs {
		variadicType := typ.Any
		if resolve != nil && fn.ParList.VarargType != nil {
			if t := resolve.ResolveType(fn.ParList.VarargType, s); t != nil {
				variadicType = t
			} else {
				variadicType = typ.Unknown
			}
		}
		s = s.WithVariadic(variadicType)
	}

	return s
}

// ResolveCallFunctionType resolves the function type for a call.
func ResolveCallFunctionType(
	info *cfg.CallInfo,
	p cfg.Point,
	sc *scope.State,
	types core.TypeOps,
	ctx *db.QueryContext,
	exprSynth scopeExprSynthesizer,
) *typ.Function {
	if info == nil || sc == nil || exprSynth == nil {
		return nil
	}

	// Method call: x:foo()
	if callsite.IsMethodLikeCallInfo(info) {
		recvType := exprSynth(info.Receiver, p, sc)
		if recvType == nil || types == nil {
			return nil
		}
		if mt, ok := types.Method(ctx, recvType, info.Method); ok {
			return unwrap.Function(mt)
		}
		if ft, ok := types.Field(ctx, recvType, info.Method); ok {
			return unwrap.Function(ft)
		}
		return nil
	}

	// Plain call: foo() or tbl.foo()
	if info.Callee != nil {
		if attr, ok := info.Callee.(*ast.AttrGetExpr); ok {
			objType := exprSynth(attr.Object, p, sc)
			name := ast.KeyName(attr.Key)
			if objType != nil && name != "" && types != nil {
				if ft, ok := types.Field(ctx, objType, name); ok {
					return unwrap.Function(ft)
				}
			}
		}
		calleeType := exprSynth(info.Callee, p, sc)
		return unwrap.Function(calleeType)
	}

	return nil
}

// buildCallMutator creates a CallMutator that only tracks mutation metadata.
func buildCallMutator(types core.TypeOps, ctx *db.QueryContext, exprSynth scopeExprSynthesizer) scopeCallMutator {
	return func(info *cfg.CallInfo, p cfg.Point, sc *scope.State) *scope.State {
		// CallMutator now only tracks mutation metadata, not value types
		return sc
	}
}

package phase

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// RunExtract executes the flow extraction phase.
func RunExtract(input FlowExtractInput) FlowExtractOutput {
	moduleAliases := input.ModuleAliases
	var typeResolverFn func(ast.TypeExpr, *scope.State) typ.Type
	if input.Resolve.TypeResolver != nil {
		typeResolverFn = input.Resolve.TypeResolver.ResolveType
	}
	var fnSigResolverFn func(*ast.FunctionExpr, *scope.State) *typ.Function
	if input.Scope.FunctionSignatureResolver != nil {
		fnSigResolverFn = input.Scope.FunctionSignatureResolver.ResolveFunctionSignature
	}

	extractionCtx := NewContextBuilder(input.PhaseEnv).
		WithScope(input.Scope).
		WithSiblingTypes(input.SiblingTypes).
		WithLiteralTypes(input.LiteralTypes).
		WithReturnSummaries(input.ReturnSummaries).
		BuildDeclared()

	engine := synth.New(synth.Config{
		Ctx:            input.Ctx,
		Types:          input.Types,
		Scopes:         input.Scope.Scopes,
		Manifests:      input.Manifests,
		Env:            extractionCtx,
		Phase:          api.PhaseScopeCompute,
		ModuleBindings: input.ModuleBindings,
		ModuleAliases:  moduleAliases,
	})

	inputs := flowbuild.Run(&core.FlowContext{
		Graph:    input.Graph,
		Scopes:   input.Scope.Scopes,
		CheckCtx: extractionCtx,
		CallCtx:  input.Ctx,
		TypeOps:  input.Types,
		Base:     input.Scope.BaseScope,
		Globals:  input.GlobalTypes,
		API:      engine,
		Services: core.FlowServicesFuncs{
			FnSigResolver:    fnSigResolverFn,
			TypeExprResolver: typeResolverFn,
		},
		InitialDeclaredTypes: input.Scope.DeclaredTypes,
		SiblingTypes:         input.SiblingTypes,
		LiteralTypes:         input.LiteralTypes,
		ModuleAliases:        moduleAliases,
		ModuleBindings:       input.ModuleBindings,
	})

	applyModuleAliasTypes(inputs, input.Manifests)

	params := ExtractParams(input.Fn, input.Scope.DeclaredTypes, input.Graph)
	// Return inference is performed in the return inference pass; flow uses Unknown here.
	returnType := typ.Unknown

	return FlowExtractOutput{
		Inputs:     inputs,
		Params:     params,
		ReturnType: returnType,
	}
}

func applyModuleAliasTypes(inputs *flow.Inputs, manifests io.ManifestQuerier) {
	if inputs == nil {
		return
	}
	inputs.DeclaredTypes = applyModuleAliasExports(inputs.DeclaredTypes, inputs.ModuleAliases, manifests)
}

// RunLiteral executes the function literal synthesis phase.
func RunLiteral(input LiteralInput) LiteralOutput {
	initialCtx := NewContextBuilder(input.PhaseEnv).
		WithScope(input.Scope).
		WithReturnSummaries(input.ReturnSummaries).
		BuildDeclared()

	engine := synth.New(synth.Config{
		Ctx:            input.Ctx,
		Types:          input.Types,
		Scopes:         input.Scope.Scopes,
		Manifests:      input.Manifests,
		Env:            initialCtx,
		Phase:          api.PhaseScopeCompute,
		ModuleBindings: input.ModuleBindings,
		ModuleAliases:  input.ModuleAliases,
	})

	fnLiteralTypes := synth.FunctionLiteralTypes(input.Graph, func(expr ast.Expr, p cfg.Point) typ.Type {
		return engine.TypeOf(expr, p)
	})

	var declaredReturns []typ.Type
	if input.Fn != nil && len(input.Fn.ReturnTypes) > 0 {
		declaredReturns = make([]typ.Type, len(input.Fn.ReturnTypes))
		for i, rt := range input.Fn.ReturnTypes {
			if rt != nil {
				declaredReturns[i] = engine.ResolveType(rt, input.Scope.BaseScope)
			}
		}
	}

	signatures := synth.FunctionLiteralSignatures(input.Graph, engine, declaredReturns)

	return LiteralOutput{
		LiteralTypes: fnLiteralTypes,
		Signatures:   signatures,
	}
}

// InferRefinement computes a FunctionRefinement from solved flow analysis.
// Examines return points to determine OnTrue/OnFalse/OnReturn constraints.
func InferRefinement(
	graph *cfg.Graph,
	solution *flow.Solution,
	params []flow.ParamInfo,
	returnType typ.Type,
) *constraint.FunctionRefinement {
	if graph == nil || solution == nil {
		return nil
	}

	return flow.InferFunctionRefinement(solution, graph.CFG(), params, returnType)
}

// ExtractParams extracts parameter info from a function expression.
// Uses the CFG graph's precomputed param symbols for Symbol IDs.
// paramTypes provides the types keyed by SymbolID.
func ExtractParams(fn *ast.FunctionExpr, paramTypes map[cfg.SymbolID]typ.Type, graph *cfg.Graph) []flow.ParamInfo {
	if fn == nil || fn.ParList == nil {
		return nil
	}

	var slots []cfg.ParamSlot
	if graph != nil {
		slots = graph.ParamSlotsReadOnly()
	}
	if len(slots) == 0 {
		params := make([]flow.ParamInfo, 0, len(fn.ParList.Names))
		for _, name := range fn.ParList.Names {
			params = append(params, flow.ParamInfo{Name: name, Type: typ.Unknown})
		}
		return params
	}

	params := make([]flow.ParamInfo, 0, len(fn.ParList.Names))
	for _, slot := range slots {
		if !slot.HasSourceParam() {
			continue
		}
		t := typ.Unknown
		if slot.Symbol != 0 {
			if pt, ok := paramTypes[slot.Symbol]; ok && pt != nil {
				t = pt
			}
		}
		params = append(params, flow.ParamInfo{Name: slot.Name, Symbol: slot.Symbol, Type: t})
	}
	return params
}

// EnrichWithKeysCollector detects if a function is a "keys collector"
// (returns keys of a parameter) and adds KeyOf constraint to OnReturn.
// This enables cross-module key-provenance tracking.
func EnrichWithKeysCollector(eff *constraint.FunctionRefinement, fn *ast.FunctionExpr) *constraint.FunctionRefinement {
	if fn == nil {
		return eff
	}

	info := keyscoll.DetectKeysCollector(fn)
	if info == nil {
		return eff
	}

	keyOf := constraint.KeyOf{
		Table: constraint.ParamPath(info.ParamIndex),
		Key:   constraint.RetPath(info.ReturnIndex),
	}

	if eff == nil {
		return &constraint.FunctionRefinement{
			OnReturn: constraint.FromConstraints(keyOf),
		}
	}

	return &constraint.FunctionRefinement{
		Row:        eff.Row,
		OnReturn:   constraint.And(eff.OnReturn, constraint.FromConstraints(keyOf)),
		OnTrue:     eff.OnTrue,
		OnFalse:    eff.OnFalse,
		Terminates: eff.Terminates,
	}
}

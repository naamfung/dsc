package phase

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
)

// RunNarrow executes the narrowing phase.
func RunNarrow(input NarrowInput) NarrowOutput {
	var declaredTypes flow.DeclaredTypes
	var annotatedVars map[cfg.SymbolID]bool
	if input.Extract.Inputs != nil {
		declaredTypes = input.Extract.Inputs.DeclaredTypes
		annotatedVars = input.Extract.Inputs.AnnotatedVars
	}
	if declaredTypes == nil {
		declaredTypes = input.Scope.DeclaredTypes
	}
	if len(input.Scope.AnnotatedVars) > 0 {
		if annotatedVars == nil {
			annotatedVars = make(map[cfg.SymbolID]bool, len(input.Scope.AnnotatedVars))
			for sym, v := range input.Scope.AnnotatedVars {
				annotatedVars[sym] = v
			}
		} else {
			for sym, v := range input.Scope.AnnotatedVars {
				if v {
					annotatedVars[sym] = true
				}
			}
		}
	}

	// Prefer graph bindings for local symbol resolution; fall back to module bindings.
	bindings := input.Graph.Bindings()
	if bindings == nil {
		bindings = input.ModuleBindings
	}

	narrowingCtx := NewContextBuilder(input.PhaseEnv).
		WithScope(input.Scope).
		WithBindings(bindings).
		WithDeclaredTypes(declaredTypes).
		WithAnnotatedVars(annotatedVars).
		WithSiblingTypes(input.SiblingTypes).
		WithLiteralTypes(input.LiteralTypes).
		WithSolution(input.Solve.Solution).
		WithNarrowReturnSummaries(input.NarrowReturnSummaries).
		BuildNarrow()

	engine := createNarrowedEngine(
		input.Ctx,
		input.Types,
		input.Manifests,
		input.Scope.Scopes,
		input.Solve.Solution,
		narrowingCtx,
		input.ModuleBindings,
		input.ModuleAliases,
	)

	fnEffect := InferRefinement(input.Graph, input.Solve.Solution, input.Extract.Params, input.Extract.ReturnType)
	fnEffect = EnrichWithKeysCollector(fnEffect, input.Fn)

	return NarrowOutput{
		Facts:      narrowingCtx.Types(),
		Refinement: fnEffect,
		Synth:      engine,
	}
}

func createNarrowedEngine(
	ctx *db.QueryContext,
	types core.TypeOps,
	manifests io.ManifestQuerier,
	scopes map[cfg.Point]*scope.State,
	solution *flow.Solution,
	checkCtx api.NarrowEnv,
	moduleBindings *bind.BindingTable,
	moduleAliases map[cfg.SymbolID]string,
) *synth.Engine {
	var bindings *bind.BindingTable
	if checkCtx != nil {
		bindings = checkCtx.Bindings()
	}
	if bindings == nil {
		bindings = moduleBindings
	}
	return synth.New(synth.Config{
		Ctx:            ctx,
		Types:          types,
		Scopes:         scopes,
		Flow:           solution,
		Paths:          newPathFromExprFunc(solution, bindings),
		Manifests:      manifests,
		Env:            checkCtx,
		Phase:          api.PhaseNarrowing,
		ModuleBindings: moduleBindings,
		ModuleAliases:  moduleAliases,
	})
}

// newPathFromExprFunc returns a PathFromExprFunc using bindings-based path extraction.
func newPathFromExprFunc(solution *flow.Solution, bindings *bind.BindingTable) api.PathFromExprFunc {
	return func(p cfg.Point, expr ast.Expr, _ *scope.State) constraint.Path {
		if solution == nil {
			return constraint.Path{}
		}
		constResolver := func(name string) *flow.ConstValue {
			if bindings == nil {
				return nil
			}
			return solution.ConstValueAt(p, name)
		}
		return path.FromExprWithBindings(expr, constResolver, bindings)
	}
}

package phase

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPhaseEnv_Fields(t *testing.T) {
	env := PhaseEnv{}

	if env.Ctx != nil {
		t.Error("Ctx should be nil by default")
	}
	if env.Graph != nil {
		t.Error("Graph should be nil by default")
	}
	if env.Fn != nil {
		t.Error("Fn should be nil by default")
	}
	if env.Types != nil {
		t.Error("Types should be nil by default")
	}
	if env.Manifests != nil {
		t.Error("Manifests should be nil by default")
	}
	if env.GlobalTypes != nil {
		t.Error("GlobalTypes should be nil by default")
	}
	if env.ModuleAliases != nil {
		t.Error("ModuleAliases should be nil by default")
	}
	if env.ModuleBindings != nil {
		t.Error("ModuleBindings should be nil by default")
	}
	if env.RefinementStore != nil {
		t.Error("RefinementStore should be nil by default")
	}
	if env.Scopes != nil {
		t.Error("Scopes should be nil by default")
	}
}

func TestPhaseEnv_Embedding(t *testing.T) {
	env := PhaseEnv{}

	resolveInput := ResolveInput{PhaseEnv: env}
	if resolveInput.Graph != nil {
		t.Error("embedded PhaseEnv should have nil Graph")
	}

	scopeInput := ScopeInput{PhaseEnv: env}
	if scopeInput.Fn != nil {
		t.Error("embedded PhaseEnv should have nil Fn")
	}

	literalInput := LiteralInput{PhaseEnv: env}
	if literalInput.Ctx != nil {
		t.Error("embedded PhaseEnv should have nil Ctx")
	}

	flowExtractInput := FlowExtractInput{PhaseEnv: env}
	if flowExtractInput.Types != nil {
		t.Error("embedded PhaseEnv should have nil Types")
	}

	flowSolveInput := FlowSolveInput{PhaseEnv: env}
	if flowSolveInput.Manifests != nil {
		t.Error("embedded PhaseEnv should have nil Manifests")
	}

	narrowInput := NarrowInput{PhaseEnv: env}
	if narrowInput.GlobalTypes != nil {
		t.Error("embedded PhaseEnv should have nil GlobalTypes")
	}
}

func TestResolveInput_Fields(t *testing.T) {
	input := ResolveInput{}
	if input.Bindings != nil {
		t.Error("Bindings should be nil by default")
	}
	if input.BaseScope != nil {
		t.Error("BaseScope should be nil by default")
	}
}

func TestScopeInput_Fields(t *testing.T) {
	input := ScopeInput{}
	if input.Parent != nil {
		t.Error("Parent should be nil by default")
	}
	if input.SynthesizedFunctionSig != nil {
		t.Error("SynthesizedFunctionSig should be nil by default")
	}
	if input.FunctionLiteralSignatures != nil {
		t.Error("FunctionLiteralSignatures should be nil by default")
	}
	if input.ParamHintSignatures != nil {
		t.Error("ParamHintSignatures should be nil by default")
	}
	if input.SiblingTypes != nil {
		t.Error("SiblingTypes should be nil by default")
	}
}

func TestLiteralInput_Fields(t *testing.T) {
	input := LiteralInput{}
	if input.SiblingTypes != nil {
		t.Error("SiblingTypes should be nil by default")
	}
}

func TestFlowExtractInput_Fields(t *testing.T) {
	input := FlowExtractInput{}
	if input.SiblingTypes != nil {
		t.Error("SiblingTypes should be nil by default")
	}
	if input.LiteralTypes != nil {
		t.Error("LiteralTypes should be nil by default")
	}
}

func TestFlowSolveInput_Fields(t *testing.T) {
	input := FlowSolveInput{}
	if input.Resolver != nil {
		t.Error("Resolver should be nil by default")
	}
}

func TestNarrowInput_Fields(t *testing.T) {
	input := NarrowInput{}
	if input.SiblingTypes != nil {
		t.Error("SiblingTypes should be nil by default")
	}
	if input.LiteralTypes != nil {
		t.Error("LiteralTypes should be nil by default")
	}
}

func TestResolveOutput_TypeResolver(t *testing.T) {
	called := false
	resolver := TypeResolverFunc(func(expr ast.TypeExpr, sc *scope.State) typ.Type {
		called = true
		return typ.Any
	})

	out := ResolveOutput{TypeResolver: resolver}
	result := out.TypeResolver.ResolveType(nil, nil)

	if !called {
		t.Error("expected resolver to be called")
	}
	if !typ.TypeEquals(result, typ.Any) {
		t.Errorf("expected any type, got %v", result)
	}
}

func TestScopeOutput_Fields(t *testing.T) {
	out := ScopeOutput{}

	if out.BaseScope != nil {
		t.Error("BaseScope should be nil by default")
	}
	if out.Scopes != nil {
		t.Error("Scopes should be nil by default")
	}
	if out.DeclaredTypes != nil {
		t.Error("DeclaredTypes should be nil by default")
	}
	if out.AnnotatedVars != nil {
		t.Error("AnnotatedVars should be nil by default")
	}
	if out.ParamTypes != nil {
		t.Error("ParamTypes should be nil by default")
	}
	if out.FunctionSignatureResolver != nil {
		t.Error("FunctionSignatureResolver should be nil by default")
	}
	if out.SiblingTypes != nil {
		t.Error("SiblingTypes should be nil by default")
	}
}

func TestScopeOutput_FunctionSignatureResolver(t *testing.T) {
	called := false
	resolver := FunctionSignatureResolverFunc(func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
		called = true
		return typ.Func().Build()
	})

	out := ScopeOutput{FunctionSignatureResolver: resolver}
	result := out.FunctionSignatureResolver.ResolveFunctionSignature(nil, nil)

	if !called {
		t.Error("expected resolver to be called")
	}
	if result == nil {
		t.Error("expected non-nil function type")
	}
}

func TestLiteralOutput_Fields(t *testing.T) {
	out := LiteralOutput{}

	if out.LiteralTypes != nil {
		t.Error("LiteralTypes should be nil by default")
	}
	if out.Signatures != nil {
		t.Error("Signatures should be nil by default")
	}
}

func TestFlowExtractOutput_Fields(t *testing.T) {
	out := FlowExtractOutput{}

	if out.Inputs != nil {
		t.Error("Inputs should be nil by default")
	}
	if out.Params != nil {
		t.Error("Params should be nil by default")
	}
	if out.ReturnType != nil {
		t.Error("ReturnType should be nil by default")
	}
}

func TestFlowSolveOutput_Fields(t *testing.T) {
	out := FlowSolveOutput{}

	if out.Solution != nil {
		t.Error("Solution should be nil by default")
	}
}

func TestNarrowOutput_Fields(t *testing.T) {
	out := NarrowOutput{}

	if out.Facts != nil {
		t.Error("Facts should be nil by default")
	}
	if out.Refinement != nil {
		t.Error("Effect should be nil by default")
	}
	if out.Synth != nil {
		t.Error("Synth should be nil by default")
	}
}

func TestNewContextBuilder(t *testing.T) {
	t.Run("nil graph", func(t *testing.T) {
		env := PhaseEnv{}
		builder := NewContextBuilder(env)
		if builder == nil {
			t.Fatal("expected non-nil builder")
		}
	})
}

func TestContextBuilder_WithScope(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	out := ScopeOutput{
		BaseScope:     &scope.State{},
		DeclaredTypes: make(flow.DeclaredTypes),
		AnnotatedVars: make(map[cfg.SymbolID]bool),
		SiblingTypes:  make(map[cfg.SymbolID]typ.Type),
	}

	result := builder.WithScope(out)
	if result != builder {
		t.Error("WithScope should return the same builder for chaining")
	}
}

func TestContextBuilder_WithLiterals(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	out := LiteralOutput{
		LiteralTypes: make(flow.DeclaredTypes),
	}

	result := builder.WithLiterals(out)
	if result != builder {
		t.Error("WithLiterals should return the same builder for chaining")
	}
}

func TestContextBuilder_WithSolution(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	result := builder.WithSolution(nil)
	if result != builder {
		t.Error("WithSolution should return the same builder for chaining")
	}
}

func TestContextBuilder_WithBindings(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	result := builder.WithBindings(&bind.BindingTable{})
	if result != builder {
		t.Error("WithBindings should return the same builder for chaining")
	}
}

func TestContextBuilder_WithBaseScope(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	result := builder.WithBaseScope(&scope.State{})
	if result != builder {
		t.Error("WithBaseScope should return the same builder for chaining")
	}
}

func TestContextBuilder_WithDeclaredTypes(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	result := builder.WithDeclaredTypes(make(flow.DeclaredTypes))
	if result != builder {
		t.Error("WithDeclaredTypes should return the same builder for chaining")
	}
}

func TestContextBuilder_WithAnnotatedVars(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	result := builder.WithAnnotatedVars(make(map[cfg.SymbolID]bool))
	if result != builder {
		t.Error("WithAnnotatedVars should return the same builder for chaining")
	}
}

func TestContextBuilder_WithSiblingTypes(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	result := builder.WithSiblingTypes(make(map[cfg.SymbolID]typ.Type))
	if result != builder {
		t.Error("WithSiblingTypes should return the same builder for chaining")
	}
}

func TestContextBuilder_WithLiteralTypes(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	result := builder.WithLiteralTypes(make(flow.DeclaredTypes))
	if result != builder {
		t.Error("WithLiteralTypes should return the same builder for chaining")
	}
}

func TestContextBuilder_Build(t *testing.T) {
	env := PhaseEnv{}
	builder := NewContextBuilder(env)

	ctx := builder.BuildDeclared()
	if ctx == nil {
		t.Fatal("expected non-nil Env")
	}
}

func TestContextBuilder_Chaining(t *testing.T) {
	env := PhaseEnv{}
	ctx := NewContextBuilder(env).
		WithBaseScope(&scope.State{}).
		WithDeclaredTypes(make(flow.DeclaredTypes)).
		WithAnnotatedVars(make(map[cfg.SymbolID]bool)).
		WithSiblingTypes(make(map[cfg.SymbolID]typ.Type)).
		WithLiteralTypes(make(flow.DeclaredTypes)).
		WithSolution(nil).
		BuildDeclared()

	if ctx == nil {
		t.Fatal("expected non-nil Env from chained builder")
	}
}

func TestContextBuilder_Phases(t *testing.T) {
	t.Run("declared phase", func(t *testing.T) {
		env := PhaseEnv{}
		ctx := NewContextBuilder(env).BuildDeclared()

		if ctx == nil {
			t.Fatal("expected non-nil Env")
		}
		if ctx.Phase() != api.PhaseScopeCompute {
			t.Errorf("Phase() = %v, want PhaseScopeCompute", ctx.Phase())
		}
	})

	t.Run("narrowing phase", func(t *testing.T) {
		env := PhaseEnv{}
		ctx := NewContextBuilder(env).
			WithSolution(&flow.Solution{}).
			BuildNarrow()

		if ctx == nil {
			t.Fatal("expected non-nil Env")
		}
		if ctx.Phase() != api.PhaseNarrowing {
			t.Errorf("Phase() = %v, want PhaseNarrowing", ctx.Phase())
		}
	})
}

func TestMergeParamHintsIntoSig_NilSig(t *testing.T) {
	fn := &ast.FunctionExpr{}
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(fn, hints, nil)
	if result != nil {
		t.Error("expected nil when sig is nil")
	}
}

func TestMergeParamHintsIntoSig_NilFn(t *testing.T) {
	sig := typ.Func().Param("x", typ.Any).Build()
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(nil, hints, sig)
	if result != sig {
		t.Error("expected original sig when fn is nil")
	}
}

func TestMergeParamHintsIntoSig_NilParList(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: nil}
	sig := typ.Func().Param("x", typ.Any).Build()
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result != sig {
		t.Error("expected original sig when ParList is nil")
	}
}

func TestMergeParamHintsIntoSig_EmptyHints(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	sig := typ.Func().Param("x", typ.Any).Build()
	var hints []typ.Type

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result != sig {
		t.Error("expected original sig when hints are empty")
	}
}

func TestMergeParamHintsIntoSig_NilHintElement(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	sig := typ.Func().Param("x", typ.Any).Build()
	hints := []typ.Type{nil}

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result != sig {
		t.Error("expected original sig when hint element is nil")
	}
}

func TestMergeParamHintsIntoSig_AppliesHint(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{nil},
		},
	}
	sig := typ.Func().Param("x", typ.Any).Build()
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(result.Params))
	}
	if !typ.TypeEquals(result.Params[0].Type, typ.Number) {
		t.Errorf("expected param type to be number, got %v", result.Params[0].Type)
	}
}

func TestMergeParamHintsIntoSig_PreservesAnnotatedParam(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
		},
	}
	sig := typ.Func().Param("x", typ.String).Build()
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result != sig {
		t.Error("expected original sig when param is annotated")
	}
}

func TestMergeParamHintsIntoSig_PreservesVariadic(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{nil},
		},
	}
	sig := typ.Func().Param("x", typ.Any).Variadic(typ.String).Build()
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Variadic == nil {
		t.Error("expected variadic to be preserved")
	}
}

func TestMergeParamHintsIntoSig_PreservesReturns(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{nil},
		},
	}
	sig := typ.Func().Param("x", typ.Any).Returns(typ.Boolean).Build()
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Returns) != 1 {
		t.Fatalf("expected 1 return, got %d", len(result.Returns))
	}
}

func TestMergeParamHintsIntoSig_PreservesOptionalParam(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{nil},
		},
	}
	sig := typ.Func().OptParam("x", typ.Any).Build()
	hints := []typ.Type{typ.Number}

	result := paramhints.MergeIntoSignature(fn, hints, sig)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(result.Params))
	}
	if !result.Params[0].Optional {
		t.Error("expected param to remain optional")
	}
}

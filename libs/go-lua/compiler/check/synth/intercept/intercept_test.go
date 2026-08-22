package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

type mockCallIntercept struct {
	skip  bool
	types []typ.Type
}

func (m mockCallIntercept) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	return Result{Skip: m.skip, Types: m.types}
}

type mockMethodIntercept struct {
	skip  bool
	types []typ.Type
}

func (m mockMethodIntercept) InterceptMethodCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	return Result{Skip: m.skip, Types: m.types}
}

func TestNewChain(t *testing.T) {
	chain := NewChain(nil, nil)
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestChain_InterceptCall_Empty(t *testing.T) {
	chain := NewChain(nil, nil)
	result := chain.InterceptCall(&ast.FuncCallExpr{}, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for empty chain")
	}
}

func TestChain_InterceptCall_NoMatch(t *testing.T) {
	chain := NewChain([]CallIntercept{
		mockCallIntercept{skip: false},
	}, nil)
	result := chain.InterceptCall(&ast.FuncCallExpr{}, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false when no intercept matches")
	}
}

func TestChain_InterceptCall_Match(t *testing.T) {
	chain := NewChain([]CallIntercept{
		mockCallIntercept{skip: true, types: []typ.Type{typ.String}},
	}, nil)
	result := chain.InterceptCall(&ast.FuncCallExpr{}, CallEnv{})
	if !result.Skip {
		t.Fatal("expected skip=true when intercept matches")
	}
	if len(result.Types) != 1 || result.Types[0] != typ.String {
		t.Fatal("expected string type")
	}
}

func TestChain_InterceptCall_FirstWins(t *testing.T) {
	chain := NewChain([]CallIntercept{
		mockCallIntercept{skip: true, types: []typ.Type{typ.String}},
		mockCallIntercept{skip: true, types: []typ.Type{typ.Integer}},
	}, nil)
	result := chain.InterceptCall(&ast.FuncCallExpr{}, CallEnv{})
	if result.Types[0] != typ.String {
		t.Fatal("expected first intercept to win")
	}
}

func TestChain_InterceptMethodCall_Empty(t *testing.T) {
	chain := NewChain(nil, nil)
	result := chain.InterceptMethodCall(&ast.FuncCallExpr{}, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for empty chain")
	}
}

func TestChain_InterceptMethodCall_Match(t *testing.T) {
	chain := NewChain(nil, []MethodIntercept{
		mockMethodIntercept{skip: true, types: []typ.Type{typ.Integer}},
	})
	result := chain.InterceptMethodCall(&ast.FuncCallExpr{}, CallEnv{})
	if !result.Skip {
		t.Fatal("expected skip=true")
	}
	if result.Types[0] != typ.Integer {
		t.Fatal("expected integer type")
	}
}

func TestCallEnv_Fields(t *testing.T) {
	ctx := CallEnv{
		Scope:      nil,
		Recurse:    func(e ast.Expr) typ.Type { return typ.Unknown },
		TypeLookup: func(name string) typ.Type { return nil },
	}
	if ctx.Recurse == nil {
		t.Fatal("expected non-nil recurse")
	}
	if ctx.TypeLookup == nil {
		t.Fatal("expected non-nil type lookup")
	}
}

func TestResult_Fields(t *testing.T) {
	r := Result{
		Types: []typ.Type{typ.String},
		Skip:  true,
	}
	if !r.Skip {
		t.Fatal("expected skip=true")
	}
	if len(r.Types) != 1 {
		t.Fatal("expected 1 type")
	}
}

func TestChainBuilder_Empty(t *testing.T) {
	builder := NewChainBuilder()
	chain := builder.Build()
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestChainBuilder_WithManifests(t *testing.T) {
	builder := NewChainBuilder().WithManifests(nil)
	chain := builder.Build()
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestChainBuilder_WithVariadicResolver(t *testing.T) {
	builder := NewChainBuilder().WithVariadicResolver(nil)
	chain := builder.Build()
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestSelectIntercept_NotSelectCall(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "print"},
	}
	result := s.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-select call")
	}
}

func TestSelectIntercept_HashPattern(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{&ast.StringExpr{Value: "#"}},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"select": selectFn,
		}),
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for select('#')")
	}
	if result.Types[0] != typ.Integer {
		t.Fatal("expected integer return type")
	}
}

func TestSelectIntercept_NoArgs(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
	}
	result := s.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for select with no args")
	}
}

func TestRequireIntercept_NilManifests(t *testing.T) {
	r := &RequireIntercept{Manifests: nil}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "module"}},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for nil manifests")
	}
}

func TestRequireIntercept_NotRequireCall(t *testing.T) {
	r := &RequireIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "print"},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-require call")
	}
}

func TestRequireIntercept_WrongArgCount(t *testing.T) {
	r := &RequireIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for wrong arg count")
	}
}

func TestTypeCastIntercept_NilScope(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyType"},
	}
	result := tc.InterceptCall(ex, CallEnv{Scope: nil})
	if result.Skip {
		t.Fatal("expected skip=false for nil scope")
	}
}

func TestTypeCastIntercept_NotIdent(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.StringExpr{Value: "test"},
	}
	result := tc.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-ident")
	}
}

func TestTypeIsIntercept_NilScope(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "is",
		Args:     []ast.Expr{&ast.StringExpr{Value: "test"}},
	}
	result := ti.InterceptMethodCall(ex, CallEnv{Scope: nil})
	if result.Skip {
		t.Fatal("expected skip=false for nil scope")
	}
}

func TestTypeIsIntercept_WrongMethod(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "notis",
		Args:     []ast.Expr{&ast.StringExpr{Value: "test"}},
	}
	result := ti.InterceptMethodCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for wrong method")
	}
}

func TestTypeIsIntercept_NoArgs(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "is",
		Args:     []ast.Expr{},
	}
	result := ti.InterceptMethodCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for no args")
	}
}

func TestIsRequireCall_NilExpr(t *testing.T) {
	if isRequireCall(nil, CallEnv{}) {
		t.Fatal("expected false for nil expr")
	}
}

func TestIsRequireCall_NilFunc(t *testing.T) {
	if isRequireCall(&ast.FuncCallExpr{Func: nil}, CallEnv{}) {
		t.Fatal("expected false for nil func")
	}
}

func TestIsSelectCall_NilExpr(t *testing.T) {
	if isSelectCall(nil, CallEnv{}) {
		t.Fatal("expected false for nil expr")
	}
}

func TestIsSelectCall_NilFunc(t *testing.T) {
	if isSelectCall(&ast.FuncCallExpr{Func: nil}, CallEnv{}) {
		t.Fatal("expected false for nil func")
	}
}

func TestApplyOverride_NilOverride(t *testing.T) {
	types := []typ.Type{typ.String}
	result := ApplyOverride(types, nil)
	if len(result) != 1 || result[0] != typ.String {
		t.Fatal("expected unchanged types for nil override")
	}
}

func TestApplyOverride_EmptyTypes(t *testing.T) {
	result := ApplyOverride(nil, typ.Integer)
	if result != nil {
		t.Fatal("expected nil for empty types")
	}
}

func TestApplyOverride_Replaces(t *testing.T) {
	types := []typ.Type{typ.String, typ.Boolean}
	result := ApplyOverride(types, typ.Integer)
	if result[0] != typ.Integer {
		t.Fatal("expected first type to be replaced")
	}
	if result[1] != typ.Boolean {
		t.Fatal("expected second type unchanged")
	}
}

func TestResolveSpecFunction_Nil(t *testing.T) {
	if ResolveSpecFunction(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestResolveSpecFunction_Function(t *testing.T) {
	fn := typ.Func().Build()
	result := ResolveSpecFunction(fn)
	if result != fn {
		t.Fatal("expected same function back")
	}
}

func TestResolveSpecFunction_Generic(t *testing.T) {
	fn := typ.Func().Build()
	gen := &typ.Generic{Body: fn}
	result := ResolveSpecFunction(gen)
	if result != fn {
		t.Fatal("expected unwrapped function")
	}
}

func TestResolveSpecFunction_NonFunction(t *testing.T) {
	if ResolveSpecFunction(typ.String) != nil {
		t.Fatal("expected nil for non-function")
	}
}

func TestSpecReturnOverride_NilFnType(t *testing.T) {
	s := &SpecReturnOverride{Phase: api.PhaseScopeCompute}
	result := s.Override(nil, nil)
	if result != nil {
		t.Fatal("expected nil for nil fn type")
	}
}

func TestSpecReturnOverride_WrongPhase(t *testing.T) {
	s := &SpecReturnOverride{Phase: api.PhaseTypeResolution}
	fn := typ.Func().Build()
	result := s.Override(fn, nil)
	if result != nil {
		t.Fatal("expected nil for wrong phase")
	}
}

func TestSelectIntercept_VarargWithResolver(t *testing.T) {
	resolver := &mockVariadicResolver{varType: typ.String}
	s := &SelectIntercept{VariadicResolver: resolver}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.Comma3Expr{},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"select": selectFn,
		}),
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for select(n, ...)")
	}
	if result.Types[0] != typ.String {
		t.Fatal("expected string type from variadic resolver")
	}
}

func TestSelectIntercept_VarargWithoutResolver(t *testing.T) {
	s := &SelectIntercept{VariadicResolver: nil}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.Comma3Expr{},
		},
	}
	result := s.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false when resolver is nil")
	}
}

func TestSelectIntercept_ConcreteIndex(t *testing.T) {
	s := &SelectIntercept{}
	recurse := func(e ast.Expr) typ.Type {
		if str, ok := e.(*ast.StringExpr); ok {
			return typ.LiteralString(str.Value)
		}
		return typ.Unknown
	}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.StringExpr{Value: "hello"},
			&ast.NumberExpr{Value: "42"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		Recurse: ExprSynth(recurse),
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"select": selectFn,
		}),
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for concrete index")
	}
}

func TestSelectIntercept_IndexOutOfRange(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "10"},
			&ast.StringExpr{Value: "hello"},
		},
	}
	result := s.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for out of range index")
	}
}

func TestSelectIntercept_OnlyFirstArg(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}
	result := s.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for only first arg")
	}
}

func TestRequireIntercept_NonStringArg(t *testing.T) {
	querier := &mockManifestQuerier{manifests: map[string]*io.Manifest{}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-string arg")
	}
}

func TestRequireIntercept_ModuleFound(t *testing.T) {
	exportType := typ.NewRecord().Field("foo", typ.String).Build()
	manifest := &io.Manifest{Export: exportType}
	querier := &mockManifestQuerier{manifests: map[string]*io.Manifest{"mymodule": manifest}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "mymodule"}},
	}
	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()
	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"require": requireFn,
		}),
	}
	result := r.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for found module")
	}
	if result.Types[0] != exportType {
		t.Fatal("expected export type")
	}
}

func TestRequireIntercept_ModuleNotFound(t *testing.T) {
	querier := &mockManifestQuerier{manifests: map[string]*io.Manifest{}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "unknown"}},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for unknown module")
	}
}

func TestRequireIntercept_ModuleFromImports(t *testing.T) {
	exportType := typ.NewRecord().Field("bar", typ.Integer).Build()
	manifest := &io.Manifest{Export: exportType}
	querier := &mockManifestQuerier{
		manifests: map[string]*io.Manifest{},
		imports:   map[string]*io.Manifest{"aliased": manifest},
	}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "aliased"}},
	}
	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()
	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"require": requireFn,
		}),
	}
	result := r.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for import alias")
	}
}

func TestRequireIntercept_ManifestNoEnrichedExport(t *testing.T) {
	manifest := &io.Manifest{Export: nil}
	querier := &mockManifestQuerier{manifests: map[string]*io.Manifest{"mymodule": manifest}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "mymodule"}},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false when enriched export is nil")
	}
}

func TestResolveSpecFunction_Alias(t *testing.T) {
	fn := typ.Func().Build()
	alias := typ.NewAlias("MyFunc", fn)
	result := ResolveSpecFunction(alias)
	if result != fn {
		t.Fatal("expected unwrapped function from alias")
	}
}

type mockVariadicResolver struct {
	varType typ.Type
}

func (r *mockVariadicResolver) VariadicType() typ.Type {
	return r.varType
}

type mockManifestQuerier struct {
	manifests map[string]*io.Manifest
	imports   map[string]*io.Manifest
}

func (q *mockManifestQuerier) Manifest(path string) *io.Manifest {
	if q.manifests == nil {
		return nil
	}
	return q.manifests[path]
}

func (q *mockManifestQuerier) Imports() map[string]*io.Manifest {
	return q.imports
}

// typeLookupForEffectTests returns a TypeLookup function that maps names to
// function types with specific effects.
func typeLookupForEffectTests(mapping map[string]typ.Type) func(string) typ.Type {
	return func(name string) typ.Type {
		if mapping != nil {
			return mapping[name]
		}
		return nil
	}
}

func TestRequireIntercept_EffectBased_ModuleLoadEffect(t *testing.T) {
	// A function with ModuleLoad effect but name != "require" triggers the intercept.
	exportType := typ.NewRecord().Field("foo", typ.String).Build()
	manifest := &io.Manifest{Export: exportType}
	querier := &mockManifestQuerier{manifests: map[string]*io.Manifest{"mymod": manifest}}

	loadFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()

	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "load_module"},
		Args: []ast.Expr{&ast.StringExpr{Value: "mymod"}},
	}

	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"load_module": loadFn,
		}),
	}

	result := r.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected intercept to fire for function with ModuleLoad effect")
	}
	if result.Types[0] != exportType {
		t.Fatal("expected export type from manifest")
	}
}

func TestRequireIntercept_EffectBased_NoEffect_NotRequireName(t *testing.T) {
	// A function named "myfunc" without ModuleLoad effect does NOT trigger require intercept.
	querier := &mockManifestQuerier{manifests: map[string]*io.Manifest{}}

	noEffectFn := typ.Func().
		Param("x", typ.String).
		Returns(typ.Any).
		Build()

	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "myfunc"},
		Args: []ast.Expr{&ast.StringExpr{Value: "something"}},
	}

	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"myfunc": noEffectFn,
		}),
	}

	result := r.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected intercept NOT to fire for function without ModuleLoad effect")
	}
}

func TestSelectIntercept_EffectBased_VariadicTransform(t *testing.T) {
	// A function with VariadicTransform effect but name != "select" triggers the intercept.
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Integer).
		Effects(effect.WithVariadicTransform()).
		Build()

	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "my_select"},
		Args: []ast.Expr{&ast.StringExpr{Value: "#"}},
	}

	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"my_select": selectFn,
		}),
	}

	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected intercept to fire for function with VariadicTransform effect")
	}
	if result.Types[0] != typ.Integer {
		t.Fatal("expected integer return type for select('#')")
	}
}

func TestCalleeHasEffect_NilTypeLookup(t *testing.T) {
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
	}
	ctx := CallEnv{TypeLookup: nil}
	if calleeHasEffect(ex, ctx, effect.Row.HasModuleLoad) {
		t.Fatal("expected false when TypeLookup is nil")
	}
}

func TestCalleeHasEffect_NonIdentCallee(t *testing.T) {
	ex := &ast.FuncCallExpr{
		Func: &ast.StringExpr{Value: "test"},
	}
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type { return nil },
	}
	if calleeHasEffect(ex, ctx, effect.Row.HasModuleLoad) {
		t.Fatal("expected false for non-ident callee")
	}
}

func TestCalleeHasEffect_NoEffectRow(t *testing.T) {
	plainFn := typ.Func().Build()
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "myfunc"},
	}
	ctx := CallEnv{
		TypeLookup: typeLookupForEffectTests(map[string]typ.Type{
			"myfunc": plainFn,
		}),
	}
	if calleeHasEffect(ex, ctx, effect.Row.HasModuleLoad) {
		t.Fatal("expected false for function without effect row")
	}
}

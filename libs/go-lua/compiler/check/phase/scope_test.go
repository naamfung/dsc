package phase

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestComputeScopes_NilGraph(t *testing.T) {
	result := ComputeScopes(nil, nil, nil, ScopeOptions{})
	if result != nil {
		t.Errorf("expected nil for nil graph, got %v", result)
	}
}

func TestComputeScopes_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := ComputeScopes(graph, nil, nil, ScopeOptions{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[graph.Entry()] == nil {
		t.Error("expected entry point to have scope")
	}
}

func TestComputeScopes_WithBaseScope(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	base := scope.New().WithType("T", typ.String)
	result := ComputeScopes(graph, base, nil, ScopeOptions{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	entryScope := result[graph.Entry()]
	if entryScope == nil {
		t.Fatal("expected non-nil entry scope")
	}
	if entryScope != base {
		t.Error("expected entry scope to be base")
	}
}

func TestBuildFunctionScope_NilParent(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	result := BuildFunctionScope(fn, nil, nil, 0, nil)
	if result == nil {
		t.Fatal("expected non-nil scope")
	}
}

func TestBuildFunctionScope_WithParent(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	parent := scope.New().WithType("T", typ.String)
	result := BuildFunctionScope(fn, parent, nil, 0, nil)
	if result == nil {
		t.Fatal("expected non-nil scope")
	}
	if result.Parent() != parent {
		t.Error("expected parent to be set")
	}
}

func TestBuildFunctionScope_WithParams(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
		},
	}
	result := BuildFunctionScope(fn, nil, nil, 0, nil)
	if result == nil {
		t.Fatal("expected non-nil scope")
	}
	if !result.IsLocal("a") {
		t.Error("expected 'a' to be local")
	}
	if !result.IsLocal("b") {
		t.Error("expected 'b' to be local")
	}
}

func TestBuildFunctionScope_WithVariadic(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			HasVargs: true,
		},
	}
	result := BuildFunctionScope(fn, nil, nil, 0, nil)
	if result == nil {
		t.Fatal("expected non-nil scope")
	}
	vt := result.VariadicType()
	if vt == nil {
		t.Error("expected variadic type to be set")
	}
}

func TestBuildFunctionScope_WithTypeParams(t *testing.T) {
	fn := &ast.FunctionExpr{
		TypeParams: []ast.TypeParamExpr{
			{Name: "T"},
		},
		ParList: &ast.ParList{},
	}
	resolver := TypeResolverFunc(func(expr ast.TypeExpr, sc *scope.State) typ.Type {
		return typ.Any
	})
	result := BuildFunctionScope(fn, nil, resolver, 0, nil)
	if result == nil {
		t.Fatal("expected non-nil scope")
	}
	tp, ok := result.LookupTypeParam("T")
	if !ok || tp == nil {
		t.Error("expected type param 'T' to be defined")
	}
}

func TestBuildFunctionScope_MaxDepthExceeded(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	parent := scope.New().Child().Child()
	if parent.Depth() != 2 {
		t.Fatalf("expected parent depth 2, got %d", parent.Depth())
	}
	exceeded := false
	result := BuildFunctionScope(fn, parent, nil, 2, &exceeded)
	if !exceeded {
		t.Fatal("expected depth limit exceeded")
	}
	if result != parent {
		t.Fatal("expected base scope to reuse parent when depth exceeded")
	}
}

func TestExtractParamTypes_NilFunction(t *testing.T) {
	types, annotated := ExtractParamTypes(nil, nil, nil, nil, nil, nil)
	if types != nil {
		t.Errorf("expected nil types, got %v", types)
	}
	if annotated != nil {
		t.Errorf("expected nil annotated, got %v", annotated)
	}
}

func TestExtractParamTypes_NilParList(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: nil}
	types, annotated := ExtractParamTypes(nil, fn, nil, nil, nil, nil)
	if types != nil {
		t.Errorf("expected nil types, got %v", types)
	}
	if annotated != nil {
		t.Errorf("expected nil annotated, got %v", annotated)
	}
}

func TestExtractParamTypes_EmptyParams(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{}}}
	graph := cfg.Build(fn)
	types, annotated := ExtractParamTypes(graph, fn, nil, nil, nil, nil)
	if types != nil {
		t.Errorf("expected nil types for empty params, got %v", types)
	}
	if annotated != nil {
		t.Errorf("expected nil annotated for empty params, got %v", annotated)
	}
}

func TestExtractParamTypes_WithParams(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}}}
	graph := cfg.Build(fn)
	types, _ := ExtractParamTypes(graph, fn, nil, nil, nil, nil)
	if types == nil {
		t.Fatal("expected non-nil types")
	}
	for _, ty := range types {
		if ty != typ.Any {
			t.Errorf("expected Any type for untyped param, got %v", ty)
		}
	}
}

func TestExtractParamTypes_WithAnnotation(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
		},
	}
	graph := cfg.Build(fn)
	base := scope.New()
	resolver := TypeResolverFunc(func(expr ast.TypeExpr, sc *scope.State) typ.Type {
		return typ.Number
	})
	types, annotated := ExtractParamTypes(graph, fn, resolver, nil, base, nil)
	if types == nil {
		t.Fatal("expected non-nil types")
	}
	if annotated == nil {
		t.Fatal("expected non-nil annotated")
	}
	for sym := range types {
		if !annotated[sym] {
			t.Error("expected param to be marked as annotated")
		}
	}
}

func TestResolveCallFunctionType_NilInfo(t *testing.T) {
	result := ResolveCallFunctionType(nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil info, got %v", result)
	}
}

func TestResolveCallFunctionType_NilScope(t *testing.T) {
	info := &cfg.CallInfo{}
	result := ResolveCallFunctionType(info, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil scope, got %v", result)
	}
}

func TestResolveCallFunctionType_NilSynth(t *testing.T) {
	info := &cfg.CallInfo{}
	sc := scope.New()
	result := ResolveCallFunctionType(info, 0, sc, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil synth, got %v", result)
	}
}

func TestBuildFnSignatureResolver_UsesLiteralSigs(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}}}
	// Pre-computed literal signature with a distinctive return type
	expectedSig := typ.Func().Param("x", typ.Number).Returns(typ.String).Build()

	literalSigs := LiteralSigsMap{
		fn: expectedSig,
	}

	// Create resolver with literal sigs but nil engine - if literal sigs work,
	// the engine won't be called and nil deref won't happen
	resolver := buildFnSignatureResolver(literalSigs, nil, nil)
	result := resolver.ResolveFunctionSignature(fn, nil)

	if result == nil {
		t.Fatal("literal signature was not used - result is nil")
	}
	if len(result.Returns) != 1 || !typ.TypeEquals(result.Returns[0], typ.String) {
		t.Errorf("expected literal signature with String return, got %v", result)
	}
}

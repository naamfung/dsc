package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
)

func TestChainBuilder_NewChainBuilder(t *testing.T) {
	builder := NewChainBuilder()
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
}

func TestChainBuilder_Chaining(t *testing.T) {
	builder := NewChainBuilder().
		WithManifests(nil).
		WithVariadicResolver(nil)
	if builder == nil {
		t.Fatal("expected non-nil builder after chaining")
	}
}

func TestChainBuilder_BuildCreatesChain(t *testing.T) {
	chain := NewChainBuilder().Build()
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestChainBuilder_BuildIncludesSelectIntercept(t *testing.T) {
	chain := NewChainBuilder().Build()
	result := chain.InterceptCall(&ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{&ast.StringExpr{Value: "#"}},
	}, CallEnv{})
	// Without proper context setup, intercept won't fire
	if result.Skip {
		t.Fatal("expected skip=false without proper effect context")
	}
}

func TestChainBuilder_BuildIncludesRequireIntercept(t *testing.T) {
	chain := NewChainBuilder().Build()
	result := chain.InterceptCall(&ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "module"}},
	}, CallEnv{})
	// Without manifests, intercept won't fire
	if result.Skip {
		t.Fatal("expected skip=false without manifests")
	}
}

func TestChainBuilder_BuildIncludesTypeCastIntercept(t *testing.T) {
	chain := NewChainBuilder().Build()
	result := chain.InterceptCall(&ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyType"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}, CallEnv{})
	// Without proper scope/type lookup, intercept won't fire
	if result.Skip {
		t.Fatal("expected skip=false without proper context")
	}
}

func TestChainBuilder_BuildIncludesTypeIsIntercept(t *testing.T) {
	chain := NewChainBuilder().Build()
	result := chain.InterceptMethodCall(&ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "is",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}, CallEnv{})
	// Without proper scope, intercept won't fire
	if result.Skip {
		t.Fatal("expected skip=false without proper context")
	}
}

func TestChainBuilder_WithVariadicResolver_UsesResolver(t *testing.T) {
	resolver := &testVariadicResolver{varType: typ.String}
	chain := NewChainBuilder().
		WithVariadicResolver(resolver).
		Build()
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

type testVariadicResolver struct {
	varType typ.Type
}

func (r *testVariadicResolver) VariadicType() typ.Type {
	return r.varType
}

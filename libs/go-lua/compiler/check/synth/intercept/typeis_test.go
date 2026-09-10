package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeIsIntercept_WrongMethod_SkipsFalse(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "new",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}
	result := ti.InterceptMethodCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for wrong method")
	}
}

func TestTypeIsIntercept_NoArgs_SkipsFalse(t *testing.T) {
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

func TestTypeIsIntercept_NonIdentReceiver(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.StringExpr{Value: "test"},
		Method:   "is",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}
	result := ti.InterceptMethodCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-ident receiver")
	}
}

func TestTypeIsIntercept_NilScope_SkipsFalse(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "is",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}
	result := ti.InterceptMethodCall(ex, CallEnv{Scope: nil})
	if result.Skip {
		t.Fatal("expected skip=false for nil scope")
	}
}

func TestTypeIsIntercept_NoMetaForName(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "UnknownType"},
		Method:   "is",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}
	sc := scope.New()
	result := ti.InterceptMethodCall(ex, CallEnv{Scope: sc})
	if result.Skip {
		t.Fatal("expected skip=false for unknown type")
	}
}

func TestTypeIsIntercept_TypeWithIsMethod(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "is",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}

	// Create a scope with a type definition
	// MetaForName wraps it in Meta, which has built-in :is method with TypeValueMethod effect
	sc := scope.New().WithType("MyType", typ.Integer)

	result := ti.InterceptMethodCall(ex, CallEnv{Scope: sc})
	// Meta types have built-in :is method, so skip should be true
	if !result.Skip {
		t.Fatal("expected skip=true for type with :is method")
	}
	if len(result.Types) != 2 {
		t.Fatalf("expected two return types, got %d", len(result.Types))
	}
	if result.Types[0] == nil || !result.Types[0].Equals(typ.NewOptional(typ.Integer)) {
		t.Fatal("expected optional value return type")
	}
	if result.Types[1] == nil || !result.Types[1].Equals(typ.NewOptional(typ.LuaError)) {
		t.Fatal("expected optional error return type")
	}
}

func TestTypeIsIntercept_AttrGetReceiver(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "module"},
			Key:    &ast.StringExpr{Value: "MyType"},
		},
		Method: "is",
		Args:   []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}
	result := ti.InterceptMethodCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-ident receiver")
	}
}

func TestTypeIsIntercept_MultipleArgs(t *testing.T) {
	ti := &TypeIsIntercept{}
	ex := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "MyType"},
		Method:   "is",
		Args: []ast.Expr{
			&ast.IdentExpr{Value: "x"},
			&ast.IdentExpr{Value: "y"},
		},
	}

	// Meta types have built-in :is method regardless of number of args passed
	sc := scope.New().WithType("MyType", typ.Integer)
	result := ti.InterceptMethodCall(ex, CallEnv{Scope: sc})
	if !result.Skip {
		t.Fatal("expected skip=true for meta type with :is method")
	}
	if len(result.Types) != 2 {
		t.Fatalf("expected two return types, got %d", len(result.Types))
	}
	if result.Types[0] == nil || !result.Types[0].Equals(typ.NewOptional(typ.Integer)) {
		t.Fatal("expected optional value return type")
	}
	if result.Types[1] == nil || !result.Types[1].Equals(typ.NewOptional(typ.LuaError)) {
		t.Fatal("expected optional error return type")
	}
}

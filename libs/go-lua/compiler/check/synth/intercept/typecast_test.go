package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeCastIntercept_NotIdent_SkipsFalse(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.StringExpr{Value: "test"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	result := tc.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-ident callee")
	}
}

func TestTypeCastIntercept_NilScope_SkipsFalse(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyType"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	result := tc.InterceptCall(ex, CallEnv{Scope: nil})
	if result.Skip {
		t.Fatal("expected skip=false for nil scope without type lookup")
	}
}

func TestTypeCastIntercept_NoCallableTypeEffect(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "myFunc"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	plainFn := typ.Func().
		Param("x", typ.Any).
		Returns(typ.String).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "myFunc" {
				return plainFn
			}
			return nil
		},
	}
	result := tc.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for function without CallableType effect")
	}
}

func TestTypeCastIntercept_WithCallableTypeEffect(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyType"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	castFn := typ.Func().
		Param("value", typ.Any).
		Returns(typ.Integer).
		Effects(effect.WithCallableType()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "MyType" {
				return castFn
			}
			return nil
		},
	}
	result := tc.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for CallableType effect")
	}
	if len(result.Types) != 1 {
		t.Fatal("expected one type")
	}
	if result.Types[0] != typ.Integer {
		t.Fatal("expected integer return type")
	}
}

func TestTypeCastIntercept_FallbackToRecurse(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyType"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	castFn := typ.Func().
		Param("value", typ.Any).
		Returns(typ.String).
		Effects(effect.WithCallableType()).
		Build()
	ctx := CallEnv{
		TypeLookup: nil,
		Recurse: func(e ast.Expr) typ.Type {
			if ident, ok := e.(*ast.IdentExpr); ok && ident.Value == "MyType" {
				return castFn
			}
			return nil
		},
	}
	result := tc.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true when recurse provides callable type")
	}
	if result.Types[0] != typ.String {
		t.Fatal("expected string return type")
	}
}

func TestTypeCastIntercept_NoReturns(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyType"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	castFn := typ.Func().
		Param("value", typ.Any).
		Effects(effect.WithCallableType()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "MyType" {
				return castFn
			}
			return nil
		},
	}
	result := tc.InterceptCall(ex, ctx)
	// Function has no returns, so should not skip
	if result.Skip {
		t.Fatal("expected skip=false for function with no returns")
	}
}

func TestTypeCastIntercept_FallbackToScopeMeta(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyType"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}

	// Create a scope with a type definition
	sc := scope.New().WithType("MyType", typ.Integer)

	castFn := typ.Func().
		Param("value", typ.Any).
		Effects(effect.WithCallableType()).
		Build()
	ctx := CallEnv{
		Scope: sc,
		TypeLookup: func(name string) typ.Type {
			if name == "MyType" {
				return castFn
			}
			return nil
		},
	}
	result := tc.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true when scope meta provides type")
	}
	if result.Types[0] != typ.Integer {
		t.Fatal("expected integer type from meta")
	}
}

func TestTypeCastIntercept_AttrGetExpr(t *testing.T) {
	tc := &TypeCastIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "module"},
			Key:    &ast.StringExpr{Value: "MyType"},
		},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	result := tc.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-ident callee")
	}
}

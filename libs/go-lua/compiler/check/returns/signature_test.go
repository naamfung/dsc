package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildSeedFunctionTypeWithBindings_ImplicitSelf(t *testing.T) {
	stmts, err := parse.ParseString(`
		function T:m(x)
			return x
		end
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	moduleFn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   stmts,
	}
	bindings := bind.Bind(moduleFn, []string{"T"})
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatal("expected function definition statement")
	}

	fnType, ok := BuildSeedFunctionTypeWithBindings(def.Func, nil, scope.New(), bindings).(*typ.Function)
	if !ok || fnType == nil {
		t.Fatal("expected function seed type")
	}
	if len(fnType.Params) != 2 {
		t.Fatalf("expected 2 params (self + x), got %d", len(fnType.Params))
	}
	if fnType.Params[0].Name != "self" {
		t.Fatalf("expected first param to be self, got %q", fnType.Params[0].Name)
	}
	if fnType.Params[1].Name != "x" {
		t.Fatalf("expected second param to be x, got %q", fnType.Params[1].Name)
	}
}

func TestBuildSeedFunctionTypeWithBindings_WithoutBindings_NoImplicitSelf(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}
	fnType, ok := BuildSeedFunctionTypeWithBindings(fn, nil, scope.New(), nil).(*typ.Function)
	if !ok || fnType == nil {
		t.Fatal("expected function seed type")
	}
	if len(fnType.Params) != 1 {
		t.Fatalf("expected 1 param without bindings, got %d", len(fnType.Params))
	}
	if fnType.Params[0].Name != "x" {
		t.Fatalf("expected param x, got %q", fnType.Params[0].Name)
	}
}

func TestBuildSeedFunctionTypeWithBindings_UnannotatedParamsStayOptional(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"a", "b"}},
	}
	fnType, ok := BuildSeedFunctionTypeWithBindings(fn, nil, scope.New(), nil).(*typ.Function)
	if !ok || fnType == nil {
		t.Fatal("expected function seed type")
	}
	if len(fnType.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fnType.Params))
	}
	if !fnType.Params[0].Optional || !fnType.Params[1].Optional {
		t.Fatalf("expected unannotated params to be optional, got %+v", fnType.Params)
	}
	if fnType.Variadic == nil || !typ.TypeEquals(fnType.Variadic, typ.Any) {
		t.Fatalf("expected variadic any for unannotated seed function, got %v", fnType.Variadic)
	}
}

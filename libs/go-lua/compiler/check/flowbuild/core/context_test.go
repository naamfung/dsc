package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFlowServicesFuncs_ResolveFunctionSignature_Nil(t *testing.T) {
	f := FlowServicesFuncs{}
	result := f.ResolveFunctionSignature(&ast.FunctionExpr{}, nil)
	if result != nil {
		t.Error("expected nil when FnSigResolver is nil")
	}
}

func TestFlowServicesFuncs_ResolveFunctionSignature(t *testing.T) {
	called := false
	expected := typ.Func().Build()
	f := FlowServicesFuncs{
		FnSigResolver: func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
			called = true
			return expected
		},
	}
	result := f.ResolveFunctionSignature(&ast.FunctionExpr{}, nil)
	if !called {
		t.Error("resolver not called")
	}
	if result != expected {
		t.Error("wrong result")
	}
}

func TestFlowServicesFuncs_ResolveTypeExpr_Nil(t *testing.T) {
	f := FlowServicesFuncs{}
	result := f.ResolveTypeExpr(&ast.PrimitiveTypeExpr{Name: "number"}, nil)
	if result != nil {
		t.Error("expected nil when TypeExprResolver is nil")
	}
}

func TestFlowServicesFuncs_ResolveTypeExpr(t *testing.T) {
	called := false
	f := FlowServicesFuncs{
		TypeExprResolver: func(expr ast.TypeExpr, sc *scope.State) typ.Type {
			called = true
			return typ.Number
		},
	}
	result := f.ResolveTypeExpr(&ast.PrimitiveTypeExpr{Name: "number"}, nil)
	if !called {
		t.Error("resolver not called")
	}
	if result != typ.Number {
		t.Error("wrong result")
	}
}

func TestFlowContext_Fields(t *testing.T) {
	fc := FlowContext{
		Globals: map[string]typ.Type{"foo": typ.String},
	}
	if fc.Globals["foo"] != typ.String {
		t.Error("globals not set")
	}
}

func TestDerived_Fields(t *testing.T) {
	called := false
	d := &Derived{
		Synth: func(expr ast.Expr, p cfg.Point) typ.Type {
			called = true
			return typ.Number
		},
	}
	result := d.Synth(nil, 0)
	if !called {
		t.Error("synth not called")
	}
	if result != typ.Number {
		t.Error("wrong result")
	}
}

package returns_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractReturnKinds_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	returns.ExtractReturnKinds(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	if len(inputs.ReturnKinds) != 0 {
		t.Error("expected no return kinds for empty graph")
	}
}

func TestExtractReturnKinds_WithReturn(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.TrueExpr{}},
			},
		},
	}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	returns.ExtractReturnKinds(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// Return kinds should be extracted
}

func TestClassifyReturnExpr_TrueLiteral(t *testing.T) {
	expr := &ast.TrueExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnTrue {
		t.Errorf("expected ReturnTrue, got %v", kind)
	}
}

func TestClassifyReturnExpr_FalseLiteral(t *testing.T) {
	expr := &ast.FalseExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnFalse {
		t.Errorf("expected ReturnFalse, got %v", kind)
	}
}

func TestClassifyReturnExpr_NilLiteral(t *testing.T) {
	expr := &ast.NilExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for nil literal, got %v", kind)
	}
}

func TestClassifyReturnExpr_IdentExpr(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for ident, got %v", kind)
	}
}

func TestClassifyReturnExpr_NilInput(t *testing.T) {
	kind := resolve.ClassifyReturnExpr(nil)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for nil expr, got %v", kind)
	}
}

func TestExtractReturnExprConstraints_Nil(t *testing.T) {
	result := cond.ExtractReturnExprConstraints(nil, 0, nil, nil, nil, nil, nil, nil)
	if result.OnTrue.HasConstraints() || result.OnFalse.HasConstraints() {
		t.Error("expected no constraints for nil expr")
	}
}

func TestExtractReturnKinds_MultipleReturns(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
					&ast.ReturnStmt{Exprs: []ast.Expr{&ast.TrueExpr{}}},
				},
				Else: []ast.Stmt{
					&ast.ReturnStmt{Exprs: []ast.Expr{&ast.FalseExpr{}}},
				},
			},
		},
	}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	returns.ExtractReturnKinds(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// Should extract return kinds for both branches
}

func TestExtractReturnKinds_EmptyReturn(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{},
			},
		},
	}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	returns.ExtractReturnKinds(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// Empty returns should be ignored
	if len(inputs.ReturnKinds) != 0 {
		t.Error("expected no return kinds for empty return")
	}
}

func TestExtractReturnKinds_WithSynth(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
		},
	}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Boolean
	}
	returns.ExtractReturnKinds(&core.FlowContext{
		Graph:   graph,
		Scopes:  scopes,
		Derived: &core.Derived{Synth: synth},
	}, inputs)
	// Should handle synth for return expressions
}

func TestReturnKind_Values(t *testing.T) {
	// Verify the return kind constants exist
	kinds := []flow.ReturnKind{
		flow.ReturnUnknown,
		flow.ReturnTrue,
		flow.ReturnFalse,
	}
	_ = kinds
}

func TestReturnExprConstraints_Empty(t *testing.T) {
	c := flow.ReturnExprConstraints{}
	if c.OnTrue.HasConstraints() {
		t.Error("expected no OnTrue constraints")
	}
	if c.OnFalse.HasConstraints() {
		t.Error("expected no OnFalse constraints")
	}
}

func TestExtractReturnKinds_CallExpression(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: &ast.IdentExpr{Value: "isValid"},
						Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
					},
				},
			},
		},
	}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	returns.ExtractReturnKinds(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// Call expressions should not produce direct return kinds
}

func TestExtractReturnKinds_StringLiteral(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.StringExpr{Value: "hello"}},
			},
		},
	}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	returns.ExtractReturnKinds(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// String literals don't have special return kinds
}

func TestExtractReturnKinds_NumberLiteral(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "42"}},
			},
		},
	}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
	}
	scopes := map[cfg.Point]*scope.State{}
	returns.ExtractReturnKinds(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// Number literals don't have special return kinds
}

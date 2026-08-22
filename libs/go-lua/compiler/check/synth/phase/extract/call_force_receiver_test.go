package extract

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
)

func TestForceMethodReceiverAtPoint_ForDotDefinedFieldFunction(t *testing.T) {
	src := `
		local T = {}
		function T.foo(x: number): number
			return x + 1
		end
		local n: number = T:foo(1)
	`

	stmts, err := parse.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	bindings := bind.Bind(fn, nil)
	graph := ccfg.BuildWithBindings(fn, bindings)

	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:    graph,
		Bindings: bindings,
	})
	synth := NewSynthesizer(&Deps{
		Ctx:            db.NewQueryContext(db.New()),
		CheckCtx:       checkCtx,
		ModuleBindings: bindings,
	}, api.PhaseTypeResolution)

	var (
		callPoint ccfg.Point
		callExpr  *ast.FuncCallExpr
	)
	graph.EachAssign(func(p ccfg.Point, info *ccfg.AssignInfo) {
		if info == nil {
			return
		}
		for _, call := range info.SourceCalls {
			if call != nil && call.Call != nil && call.Call.Method == "foo" {
				callPoint = p
				callExpr = call.Call
				return
			}
		}
	})

	if callExpr == nil {
		t.Fatal("expected method call source in assignment")
	}
	if !synth.forceMethodReceiverAtPoint(callPoint, callExpr) {
		t.Fatal("expected forceMethodReceiverAtPoint to be true for dot-defined field function")
	}
}

func TestForceMethodReceiverAtPoint_ForFieldAssignedFunctionLiteral(t *testing.T) {
	src := `
		local T = {}
		T.foo = function(x: number): number
			return x + 1
		end
		local n: number = T:foo(1)
	`

	stmts, err := parse.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	bindings := bind.Bind(fn, nil)
	graph := ccfg.BuildWithBindings(fn, bindings)

	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:    graph,
		Bindings: bindings,
	})
	synth := NewSynthesizer(&Deps{
		Ctx:            db.NewQueryContext(db.New()),
		CheckCtx:       checkCtx,
		ModuleBindings: bindings,
	}, api.PhaseTypeResolution)

	var (
		callPoint ccfg.Point
		callExpr  *ast.FuncCallExpr
	)
	graph.EachAssign(func(p ccfg.Point, info *ccfg.AssignInfo) {
		if info == nil {
			return
		}
		for _, call := range info.SourceCalls {
			if call != nil && call.Call != nil && call.Call.Method == "foo" {
				callPoint = p
				callExpr = call.Call
				return
			}
		}
	})

	if callExpr == nil {
		t.Fatal("expected method call source in assignment")
	}
	if !synth.forceMethodReceiverAtPoint(callPoint, callExpr) {
		t.Fatal("expected forceMethodReceiverAtPoint to be true for field-assigned function literal")
	}
}

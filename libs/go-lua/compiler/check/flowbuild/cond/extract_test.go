package cond

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkeffects "github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/parse"
	typecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractEdgeConstraints_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}
	ExtractEdgeConstraints(fc, inputs)
	if len(inputs.EdgeConditions) != 0 {
		t.Error("nil graph should produce no edge conditions")
	}
}

func TestExtractNumericConstraints_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}
	ExtractNumericConstraints(fc, inputs)
	if len(inputs.EdgeNumericConstraints) != 0 {
		t.Error("nil graph should produce no numeric constraints")
	}
}

func TestFindBranchEdges_EmptySuccessors(t *testing.T) {
	trueEdge, falseEdge := FindBranchEdges(nil, 0, nil)
	if trueEdge != 0 || falseEdge != 0 {
		t.Error("empty successors should return zero edges")
	}
}

func TestComputeDeadPoints_EmptyGraph(t *testing.T) {
	result := ComputeDeadPoints(nil, nil, nil, nil, nil)
	if result == nil {
		t.Error("should return non-nil map")
	}
}

func TestExtractLenOfPath_NilExpr(t *testing.T) {
	result := ExtractLenOfPath(nil, 0, nil)
	if !result.IsEmpty() {
		t.Error("nil expr should return empty path")
	}
}

func TestExtractLenOfPath_NonLenOp(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	result := ExtractLenOfPath(expr, 0, nil)
	if !result.IsEmpty() {
		t.Error("non-len expr should return empty path")
	}
}

func TestExtractCallOnReturnConstraints_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}
	result := ExtractCallOnReturnConstraints(fc, inputs)
	if len(result) != 0 {
		t.Error("nil graph should return empty map")
	}
}

func TestConstraintsFromCallOnReturn_NilInfo(t *testing.T) {
	result := ConstraintsFromCallOnReturn(nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("nil info should produce no constraints")
	}
}

func TestConstraintsFromCallOnReturn_OnlyAppliesMustConstraints(t *testing.T) {
	sym := typecfg.SymbolID(101)
	info := &cfg.CallInfo{
		CalleeSymbol: sym,
		Args:         []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}

	refinementLookup := func(id typecfg.SymbolID) *constraint.FunctionRefinement {
		if id != sym {
			return nil
		}
		p0 := constraint.NewPlaceholder(0)
		return &constraint.FunctionRefinement{
			OnReturn: constraint.FromDisjuncts([][]constraint.Constraint{
				{constraint.Truthy{Path: p0}},
				{constraint.Falsy{Path: p0}},
			}),
		}
	}

	result := ConstraintsFromCallOnReturn(
		info,
		0,
		nil,
		nil,
		nil,
		nil,
		refinementLookup,
		nil,
		nil,
		nil,
		nil,
	)

	if result.HasConstraints() {
		t.Fatalf("expected no propagated constraints for non-guaranteed OnReturn, got: %v", result.Disjuncts)
	}
}

func TestConstraintsFromAssignOnReturn_NilInfo(t *testing.T) {
	result := ConstraintsFromAssignOnReturn(nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("nil info should produce no constraints")
	}
}

func TestExtractEffectFromType_NilType(t *testing.T) {
	result := checkeffects.EffectFromType(nil)
	if result != nil {
		t.Error("nil type should return nil effect")
	}
}

func TestExtractEffectFromType_NonFunction(t *testing.T) {
	result := checkeffects.EffectFromType(typ.String)
	if result != nil {
		t.Error("non-function type should return nil effect")
	}
}

func TestExtractEffectFromType_FunctionNoRefinement(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	result := checkeffects.EffectFromType(fn)
	if result != nil {
		t.Error("function without refinement should return nil effect")
	}
}

func TestResolveCalleeToFunctionLiteral_NilCallee(t *testing.T) {
	result := resolve.ResolveCalleeToFunctionLiteral(nil, nil)
	if result != nil {
		t.Error("nil callee should return nil")
	}
}

func TestResolveCalleeToFunctionLiteral_FunctionExpr(t *testing.T) {
	fn := &ast.FunctionExpr{}
	result := resolve.ResolveCalleeToFunctionLiteral(fn, nil)
	if result != fn {
		t.Error("FunctionExpr should be returned as-is")
	}
}

func TestResolveSymbolToFunctionLiteral_NilGraph(t *testing.T) {
	result := resolve.ResolveSymbolToFunctionLiteral(nil, 1)
	if result != nil {
		t.Error("nil graph should return nil")
	}
}

func TestResolveSymbolToFunctionLiteral_ZeroSymbol(t *testing.T) {
	result := resolve.ResolveSymbolToFunctionLiteral(nil, 0)
	if result != nil {
		t.Error("zero symbol should return nil")
	}
}

func TestResolveExprToTableLiteral_NilExpr(t *testing.T) {
	result := resolve.ResolveExprToTableLiteral(nil, nil)
	if result != nil {
		t.Error("nil expr should return nil")
	}
}

func TestResolveExprToTableLiteral_NilGraph(t *testing.T) {
	tbl := &ast.TableExpr{}
	result := resolve.ResolveExprToTableLiteral(tbl, nil)
	if result != nil {
		t.Error("nil graph should return nil")
	}
}

func TestCallTerminates_NilInfo(t *testing.T) {
	result := CallTerminates(nil, 0, nil, nil, nil, nil, nil)
	if result {
		t.Error("nil info should return false")
	}
}

func TestCallTerminates_UsesCanonicalCandidatesWhenRawSymbolMissing(t *testing.T) {
	src := `
		local x = error("boom")
	`
	stmts, err := parse.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts}, "error")
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	var (
		point    typecfg.Point
		callInfo *cfg.CallInfo
		errorSym typecfg.SymbolID
	)
	graph.EachAssign(func(p typecfg.Point, info *cfg.AssignInfo) {
		if point != 0 || info == nil {
			return
		}
		call, _ := info.CallForTarget(0)
		if call == nil {
			return
		}
		point = p
		callInfo = call
		errorSym = call.CalleeSymbol
	})
	if point == 0 || callInfo == nil || errorSym == 0 {
		t.Fatal("expected assignment callsite with callee symbol")
	}

	// Simulate missing raw symbol. Canonical candidate lookup should recover
	// from call expression/bindings and still detect termination.
	callInfo.CalleeSymbol = 0

	refinementLookup := func(sym typecfg.SymbolID) *constraint.FunctionRefinement {
		if sym == errorSym {
			return &constraint.FunctionRefinement{Terminates: true}
		}
		return nil
	}

	if !CallTerminates(callInfo, point, nil, nil, refinementLookup, graph, nil) {
		t.Fatal("expected terminating call via canonical callee candidate")
	}
}

func TestCallTerminates_UsesModuleBindingNameFallback(t *testing.T) {
	src := `
		local x = error("boom")
	`
	stmts, err := parse.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts}, "error")
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	var (
		point    typecfg.Point
		callInfo *cfg.CallInfo
		errorSym typecfg.SymbolID
	)
	graph.EachAssign(func(p typecfg.Point, info *cfg.AssignInfo) {
		if point != 0 || info == nil {
			return
		}
		call, _ := info.CallForTarget(0)
		if call == nil {
			return
		}
		point = p
		callInfo = call
		errorSym = call.CalleeSymbol
	})
	if point == 0 || callInfo == nil || errorSym == 0 {
		t.Fatal("expected assignment callsite with callee symbol")
	}

	moduleBindings := bind.NewBindingTable()
	moduleBindings.SetName(errorSym, "error_alias")

	// Force canonical resolution through module-binding name fallback only.
	callInfo.CalleeSymbol = 0
	callInfo.Callee = &ast.IdentExpr{Value: "error_alias"}
	callInfo.CalleeName = "error_alias"

	refinementLookup := func(sym typecfg.SymbolID) *constraint.FunctionRefinement {
		if sym == errorSym {
			return &constraint.FunctionRefinement{Terminates: true}
		}
		return nil
	}

	if !CallTerminates(callInfo, point, nil, nil, refinementLookup, graph, moduleBindings) {
		t.Fatal("expected terminating call via module-binding callee candidate")
	}
}

func TestExtractPredicateLinkFromCallInfo_NilInfo(t *testing.T) {
	result := ExtractPredicateLinkFromCallInfo(nil, 0, 0, nil, nil, nil, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("nil info should return nil")
	}
}

func TestComputeDeadPoints_NilGraph(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Error("should not panic with nil graph")
		}
	}()
	_ = ComputeDeadPoints(nil, nil, nil, nil, nil)
}

func TestPointHasTerminatingCallSite_AssignSourceCall(t *testing.T) {
	src := `
		local x = error("boom")
		local y = 1
	`
	stmts, err := parse.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts}, "error")
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	var (
		xPoint   typecfg.Point
		yPoint   typecfg.Point
		errorSym typecfg.SymbolID
	)
	graph.EachAssign(func(p typecfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 || info.Targets[0].Kind != cfg.TargetIdent {
			return
		}
		switch info.Targets[0].Name {
		case "x":
			xPoint = p
			if call := info.SingleSourceCall(); call != nil {
				errorSym = call.CalleeSymbol
			}
		case "y":
			yPoint = p
		}
	})
	if xPoint == 0 || errorSym == 0 {
		t.Fatal("expected x assignment with resolvable error() call symbol")
	}

	refinementLookup := func(sym typecfg.SymbolID) *constraint.FunctionRefinement {
		if sym == errorSym {
			return &constraint.FunctionRefinement{Terminates: true}
		}
		return nil
	}

	if !PointHasTerminatingCallSite(graph, xPoint, nil, nil, refinementLookup, nil) {
		t.Fatal("expected terminating callsite at x assignment point")
	}
	if yPoint != 0 && PointHasTerminatingCallSite(graph, yPoint, nil, nil, refinementLookup, nil) {
		t.Fatal("did not expect terminating callsite at y assignment point")
	}
}

func TestComputeDeadPoints_AssignSourceCallTerminates(t *testing.T) {
	src := `
		local x = error("boom")
		local y = 1
	`
	stmts, err := parse.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts}, "error")
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	var (
		xPoint   typecfg.Point
		errorSym typecfg.SymbolID
	)
	graph.EachAssign(func(p typecfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 || info.Targets[0].Kind != cfg.TargetIdent || info.Targets[0].Name != "x" {
			return
		}
		xPoint = p
		if call := info.SingleSourceCall(); call != nil {
			errorSym = call.CalleeSymbol
		}
	})
	if xPoint == 0 || errorSym == 0 {
		t.Fatal("expected x assignment with resolvable error() call symbol")
	}

	refinementLookup := func(sym typecfg.SymbolID) *constraint.FunctionRefinement {
		if sym == errorSym {
			return &constraint.FunctionRefinement{Terminates: true}
		}
		return nil
	}

	dead := ComputeDeadPoints(graph, nil, nil, refinementLookup, nil)
	if len(dead) == 0 {
		t.Fatal("expected at least one dead point from terminating assignment call")
	}
}

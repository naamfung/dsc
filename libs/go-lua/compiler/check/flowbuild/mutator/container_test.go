package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractContainerMutatorAssignments_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}

	ExtractContainerMutatorAssignments(fc, inputs)

	if len(inputs.ContainerMutatorAssignments) != 0 {
		t.Errorf("expected no assignments for nil graph, got %d", len(inputs.ContainerMutatorAssignments))
	}
}

func TestExtractContainerMutatorAssignments_NilInputs(t *testing.T) {
	fc := &core.FlowContext{}

	ExtractContainerMutatorAssignments(fc, nil)
}

func TestContainerElementReturnInfo(t *testing.T) {
	info := ContainerElementReturnInfo{
		ReturnIndex: 1,
	}
	if info.ReturnIndex != 1 {
		t.Errorf("expected ReturnIndex 1, got %d", info.ReturnIndex)
	}
}

func TestContainerElementReturnFromCall_NilInfo(t *testing.T) {
	result := ContainerElementReturnFromCall(nil, 0, nil, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}

func TestContainerArgAtMethod_ReceiverAtIndex0(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	args := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.NumberExpr{Value: "2"},
	}

	result := callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 0)
	if result != receiver {
		t.Error("expected receiver at index 0")
	}
}

func TestContainerArgAtMethod_ArgsAfterReceiver(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	arg1 := &ast.NumberExpr{Value: "1"}
	arg2 := &ast.NumberExpr{Value: "2"}
	args := []ast.Expr{arg1, arg2}

	result := callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 1)
	if result != arg1 {
		t.Error("expected first arg at index 1")
	}

	result = callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 2)
	if result != arg2 {
		t.Error("expected second arg at index 2")
	}
}

func TestContainerArgAtMethod_OutOfBounds(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	args := []ast.Expr{&ast.NumberExpr{Value: "1"}}

	result := callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 5)
	if result != nil {
		t.Error("expected nil for out of bounds index")
	}
}

func TestExtractContainerMutatorAssignments_AssignmentCallSite(t *testing.T) {
	code := `
		local c = {}
		local _ = send(c, 1)
	`
	graph := buildGraph(t, code, "send")
	inputs := &flow.Inputs{
		Graph: graph,
	}

	ExtractContainerMutatorAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: containerSendSynth(),
		},
	}, inputs)

	if len(inputs.ContainerMutatorAssignments) != 1 {
		t.Fatalf("expected 1 container mutator assignment, got %d", len(inputs.ContainerMutatorAssignments))
	}
	symC, ok := graph.SymbolAt(graph.Exit(), "c")
	if !ok || symC == 0 {
		t.Fatal("expected symbol for c")
	}
	assign := inputs.ContainerMutatorAssignments[0]
	if assign.Target.Symbol != symC {
		t.Fatalf("expected target symbol %d, got %d", symC, assign.Target.Symbol)
	}
	if !typ.TypeEquals(assign.ValueType, typ.Integer) {
		t.Fatalf("expected value type integer, got %v", assign.ValueType)
	}
}

func containerSendSynth() func(ast.Expr, cfg.Point) typ.Type {
	spec := contract.NewSpec().WithEffects(effect.Mutate{
		Target: effect.ParamRef{Index: 0},
		Transform: effect.ContainerElementUnion{
			Container: effect.ParamRef{Index: 0},
			Value:     effect.ParamRef{Index: 1},
		},
	})
	send := typ.Func().
		Param("container", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Nil).
		Spec(spec).
		Build()

	return func(expr ast.Expr, _ cfg.Point) typ.Type {
		switch v := expr.(type) {
		case *ast.IdentExpr:
			if v.Value == "send" {
				return send
			}
		case *ast.NumberExpr:
			return typ.Integer
		}
		return typ.Unknown
	}
}

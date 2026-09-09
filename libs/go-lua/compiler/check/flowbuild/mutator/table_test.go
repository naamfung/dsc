package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/literal"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractTableMutatorAssignments_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}

	ExtractTableMutatorAssignments(fc, inputs)

	if len(inputs.TableMutatorAssignments) != 0 {
		t.Errorf("expected no assignments for nil graph, got %d", len(inputs.TableMutatorAssignments))
	}
}

func TestExtractTableMutatorAssignments_NilInputs(t *testing.T) {
	fc := &core.FlowContext{}

	ExtractTableMutatorAssignments(fc, nil)
}

func TestTableMutatorFromCall_NilInfo(t *testing.T) {
	result := TableMutatorFromCall(nil, 0, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}

func TestPositionalArgAt_EmptyArgs(t *testing.T) {
	result := callsite.PositionalArgAt(nil, 0)
	if result != nil {
		t.Error("expected nil for empty args")
	}

	result = callsite.PositionalArgAt([]ast.Expr{}, 0)
	if result != nil {
		t.Error("expected nil for empty slice")
	}
}

func TestPositionalArgAt_ValidIndex(t *testing.T) {
	arg0 := &ast.NumberExpr{Value: "1"}
	arg1 := &ast.NumberExpr{Value: "2"}
	arg2 := &ast.NumberExpr{Value: "3"}
	args := []ast.Expr{arg0, arg1, arg2}

	if callsite.PositionalArgAt(args, 0) != arg0 {
		t.Error("expected arg0 at index 0")
	}
	if callsite.PositionalArgAt(args, 1) != arg1 {
		t.Error("expected arg1 at index 1")
	}
	if callsite.PositionalArgAt(args, 2) != arg2 {
		t.Error("expected arg2 at index 2")
	}
}

func TestPositionalArgAt_NegativeIndex(t *testing.T) {
	arg0 := &ast.NumberExpr{Value: "1"}
	arg1 := &ast.NumberExpr{Value: "2"}
	arg2 := &ast.NumberExpr{Value: "3"}
	args := []ast.Expr{arg0, arg1, arg2}

	if callsite.PositionalArgAt(args, -1) != arg2 {
		t.Error("expected arg2 at index -1")
	}
	if callsite.PositionalArgAt(args, -2) != arg1 {
		t.Error("expected arg1 at index -2")
	}
	if callsite.PositionalArgAt(args, -3) != arg0 {
		t.Error("expected arg0 at index -3")
	}
}

func TestPositionalArgAt_OutOfBounds(t *testing.T) {
	args := []ast.Expr{&ast.NumberExpr{Value: "1"}}

	if callsite.PositionalArgAt(args, 5) != nil {
		t.Error("expected nil for index out of bounds")
	}
	if callsite.PositionalArgAt(args, -5) != nil {
		t.Error("expected nil for negative index out of bounds")
	}
}

func TestRuntimeArgAt_MethodCall(t *testing.T) {
	recv := &ast.IdentExpr{Value: "self"}
	arg := &ast.NumberExpr{Value: "1"}
	info := &cfg.CallInfo{
		Method:   "push",
		Receiver: recv,
		Args:     []ast.Expr{arg},
	}

	if got := callsite.RuntimeArgAt(info, 0); got != recv {
		t.Fatal("expected receiver at runtime index 0")
	}
	if got := callsite.RuntimeArgAt(info, 1); got != arg {
		t.Fatal("expected first arg at runtime index 1")
	}
}

func TestTableMutatorFromCall_MethodCallWithCalleeSymbol(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.TableMutator{
		Target: effect.ParamRef{Index: 0},
		Value:  effect.ParamRef{Index: 1},
	})
	fnType := typ.Func().
		Param("target", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Nil).
		Spec(spec).
		Build()

	info := &cfg.CallInfo{
		Method:       "push",
		Receiver:     &ast.IdentExpr{Value: "t"},
		CalleeSymbol: 42,
	}

	got := TableMutatorFromCall(
		info,
		0,
		nil,
		func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
			if sym == 42 {
				return fnType, true
			}
			return nil, false
		},
		nil,
		nil,
		nil,
	)
	if got == nil {
		t.Fatal("expected table mutator for method callee symbol")
	}
	if got.Target.Index != 0 || got.Value.Index != 1 {
		t.Fatalf("unexpected mutator indices: target=%d value=%d", got.Target.Index, got.Value.Index)
	}
}

func TestKeyTypeFromExpr_NilExpr(t *testing.T) {
	result := literal.KeyTypeFromExpr(nil, nil)
	if result != nil {
		t.Error("expected nil for nil expr")
	}
}

func TestExtractTableMutatorAssignments_AssignmentCallSite(t *testing.T) {
	code := `
		local t = {}
		local _ = table.insert(t, 1)
	`
	graph := buildGraph(t, code, "table")
	inputs := &flow.Inputs{
		Graph: graph,
	}

	ExtractTableMutatorAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: tableInsertSynth(),
		},
	}, inputs)

	if len(inputs.TableMutatorAssignments) != 1 {
		t.Fatalf("expected 1 table mutator assignment, got %d", len(inputs.TableMutatorAssignments))
	}
	symT, ok := graph.SymbolAt(graph.Exit(), "t")
	if !ok || symT == 0 {
		t.Fatal("expected symbol for t")
	}
	if inputs.TableMutatorAssignments[0].Target.Symbol != symT {
		t.Fatalf("expected target symbol %d, got %d", symT, inputs.TableMutatorAssignments[0].Target.Symbol)
	}
}

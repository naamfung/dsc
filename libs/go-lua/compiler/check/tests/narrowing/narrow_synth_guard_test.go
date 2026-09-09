package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/compiler/check/scope"
)

// Tests that hooks return empty diagnostics when NarrowSynth is nil.
// This enforces the design principle that hooks only run with post-flow context.

func TestHooksRequireNarrowSynth_CallHook(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	diags := hooks.CheckCalls(graph, nil, nil, nil, "test.lua")

	if len(diags) != 0 {
		t.Errorf("call hook should return empty diagnostics when NarrowSynth is nil, got %d", len(diags))
	}
}

func TestHooksRequireNarrowSynth_ReturnHook(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		ReturnTypes: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	graph := cfg.Build(fn)
	baseScope := scope.New()

	diags := hooks.CheckReturns(fn, graph, map[cfg.Point]*scope.State{}, baseScope, nil, nil, "test.lua")

	if len(diags) != 0 {
		t.Errorf("return hook should return empty diagnostics when declared synth is nil, got %d", len(diags))
	}
}

func TestHooksRequireNarrowSynth_FieldHook(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	diags := hooks.CheckFields(graph, nil, nil, "test.lua")

	if len(diags) != 0 {
		t.Errorf("field hook should return empty diagnostics when NarrowSynth is nil, got %d", len(diags))
	}
}

func TestHooksRequireNarrowSynth_AssignHook(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	diags := hooks.CheckAssignments(graph, map[cfg.Point]*scope.State{}, nil, nil, "test.lua")

	if len(diags) != 0 {
		t.Errorf("assign hook should return empty diagnostics when NarrowSynth is nil, got %d", len(diags))
	}
}

package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexerInfo_ZeroValue(t *testing.T) {
	var info mutator.IndexerInfo
	if info.KeyType != nil {
		t.Error("expected nil KeyType for zero value IndexerInfo")
	}
	if info.ValType != nil {
		t.Error("expected nil ValType for zero value IndexerInfo")
	}
}

func TestCollectFieldAssignments_NilGraph(t *testing.T) {
	result := CollectFieldAssignments(nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectFieldAssignments_EmptyGraph(t *testing.T) {
	graph := buildEmptyGraph()
	result := CollectFieldAssignments(graph, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectFieldAssignments_WithSynth(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	result := CollectFieldAssignments(graph, synth, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectFieldAssignments_WithFilter(t *testing.T) {
	graph := buildEmptyGraph()
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectFieldAssignments(graph, nil, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_NilGraph(t *testing.T) {
	result := CollectIndexerAssignments(nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectIndexerAssignments_EmptyGraph(t *testing.T) {
	graph := buildEmptyGraph()
	result := CollectIndexerAssignments(graph, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_WithSynth(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Integer
	}
	result := CollectIndexerAssignments(graph, synth, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_WithBindings(t *testing.T) {
	graph := buildEmptyGraph()
	bindings := &bind.BindingTable{}
	result := CollectIndexerAssignments(graph, nil, bindings, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_WithFilter(t *testing.T) {
	graph := buildEmptyGraph()
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectIndexerAssignments(graph, nil, nil, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_AllParams(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	bindings := &bind.BindingTable{}
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectIndexerAssignments(graph, synth, bindings, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

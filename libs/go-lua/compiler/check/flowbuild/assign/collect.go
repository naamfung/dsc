package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/typ"
)

// CollectFieldAssignments scans the graph for field assignments and groups them by base symbol.
// Returns a map: symbolID -> map[fieldName]typ.Type representing fields assigned to each symbol.
// The synth function is used to synthesize field value types.
// If filterSyms is non-nil, only symbols in the filter are collected.
func CollectFieldAssignments(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID]map[string]typ.Type {
	return overlaymut.CollectFieldAssignments(graph, synth, filterSyms)
}

// CollectIndexerAssignments scans the graph for dynamic index assignments (t[k] = v where k is non-const).
// Returns a map: symbolID -> []IndexerInfo representing index assignments to each symbol.
func CollectIndexerAssignments(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID][]mutator.IndexerInfo {
	return overlaymut.CollectIndexerAssignments(graph, synth, bindings, filterSyms)
}

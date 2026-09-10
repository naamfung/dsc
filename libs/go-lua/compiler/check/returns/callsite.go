package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// CollectCalledNestedFieldAssignments collects field assignments recorded for
// called nested functions that target symbols from the parent graph (captured variables).
//
// When a nested function assigns fields to a captured variable, those assignments
// affect the type of the variable in the parent scope. This function scans nested
// functions that are called from the parent graph and collects field assignments
// that target parent-scope variables.
//
// Example:
//
//	local t = {}
//	local function setup()
//	    t.name = "hello"  -- This assignment is collected
//	end
//	setup()  -- Because setup() is called, t gains field "name"
//
// The result maps symbols to their assigned field types, which can be merged
// into the symbol's type in the parent scope.
func CollectCalledNestedFieldAssignments(
	parent *cfg.Graph,
	bindings *bind.BindingTable,
	capturedByCallee map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type,
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
	if parent == nil || len(capturedByCallee) == 0 {
		return result
	}

	// Gather all symbols known in the parent graph (avoid per-point merges).
	parentSymbols := parent.AllSymbolIDs()

	// Find which local functions are called in the parent graph.
	trackedCallees := make(map[cfg.SymbolID]bool, len(capturedByCallee))
	for calleeSym := range capturedByCallee {
		trackedCallees[calleeSym] = true
	}
	calledSyms := make(map[cfg.SymbolID]bool)
	parent.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		for sym := range calledSymbolsFromCall(info, p, parent, bindings, resolveCalleeType, func(sym cfg.SymbolID) bool {
			return trackedCallees[sym]
		}) {
			calledSyms[sym] = true
		}
	})

	// Collect field assignments from called nested functions and merge into result.
	if len(calledSyms) == 0 {
		return result
	}
	for _, sym := range cfg.SortedSymbolIDs(calledSyms) {
		nestedFields := capturedByCallee[sym]
		if len(nestedFields) == 0 {
			continue
		}
		for _, baseSym := range cfg.SortedSymbolIDs(nestedFields) {
			fields := nestedFields[baseSym]
			if !parentSymbols[baseSym] {
				continue
			}
			if result[baseSym] == nil {
				result[baseSym] = make(map[string]typ.Type)
			}
			for _, fieldName := range cfg.SortedFieldNames(fields) {
				fieldType := fields[fieldName]
				if existing := result[baseSym][fieldName]; existing != nil {
					result[baseSym][fieldName] = typ.JoinPreferNonSoft(existing, fieldType)
				} else {
					result[baseSym][fieldName] = fieldType
				}
			}
		}
	}

	return result
}

// CollectCalledNestedContainerMutatorAssignments collects container mutations recorded for
// called nested functions that target symbols from the parent graph (captured variables).
//
// This supports cases where a nested function mutates a captured container (e.g., channel.send)
// and the nested function is invoked directly or passed as a callback to a function with a
// callback spec (e.g., coroutine.spawn).
func CollectCalledNestedContainerMutatorAssignments(
	parent *cfg.Graph,
	bindings *bind.BindingTable,
	capturedByCallee api.CapturedContainerMutations,
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
) []flow.ContainerMutatorAssignment {
	if parent == nil || len(capturedByCallee) == 0 {
		return nil
	}

	parentSymbols := parent.AllSymbolIDs()
	trackedCallees := make(map[cfg.SymbolID]bool, len(capturedByCallee))
	for calleeSym := range capturedByCallee {
		trackedCallees[calleeSym] = true
	}
	assignments := make([]flow.ContainerMutatorAssignment, 0)

	parent.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		calledSyms := calledSymbolsFromCall(info, p, parent, bindings, resolveCalleeType, func(sym cfg.SymbolID) bool {
			return trackedCallees[sym]
		})
		if len(calledSyms) == 0 {
			return
		}

		for _, sym := range cfg.SortedSymbolIDs(calledSyms) {
			nestedMutations := capturedByCallee[sym]
			if len(nestedMutations) == 0 {
				continue
			}
			for _, targetSym := range cfg.SortedSymbolIDs(nestedMutations) {
				mutations := nestedMutations[targetSym]
				if !parentSymbols[targetSym] {
					continue
				}
				root := resolve.RootNameFromGraphAndBindings(parent, bindings, targetSym, "")
				for _, mutation := range mutations {
					segs := make([]constraint.Segment, len(mutation.Segments))
					copy(segs, mutation.Segments)
					assignments = append(assignments, flow.ContainerMutatorAssignment{
						Point: p,
						Target: constraint.Path{
							Root:     root,
							Symbol:   targetSym,
							Segments: segs,
						},
						ValueType: mutation.ValueType,
					})
				}
			}
		}
	})

	return assignments
}

func calledSymbolsFromCall(
	info *cfg.CallInfo,
	p cfg.Point,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
	prefer func(cfg.SymbolID) bool,
) map[cfg.SymbolID]bool {
	calledSyms := make(map[cfg.SymbolID]bool)
	if info == nil {
		return calledSyms
	}

	selected := checkcallsite.SelectPreferredSymbol(
		checkcallsite.CallableCalleeSymbolCandidates(info, graph, bindings, bindings),
		prefer,
	)
	if selected != 0 {
		calledSyms[selected] = true
	}

	if resolveCalleeType != nil {
		if fnType := resolveCalleeType(info, p); fnType != nil {
			if spec := contract.ExtractSpec(fnType); spec != nil && len(spec.Callbacks) > 0 {
				paramIndexes := make([]int, 0, len(spec.Callbacks))
				for paramIdx := range spec.Callbacks {
					paramIndexes = append(paramIndexes, paramIdx)
				}
				sort.Ints(paramIndexes)
				for _, paramIdx := range paramIndexes {
					arg := checkcallsite.RuntimeArgAt(info, paramIdx)
					if sym := checkcallsite.CanonicalSymbolFromExprWithAliases(arg, 0, graph, bindings, bindings, prefer); sym != 0 {
						calledSyms[sym] = true
					}
				}
			}
		}
	}

	return calledSyms
}

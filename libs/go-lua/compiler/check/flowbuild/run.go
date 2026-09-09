// Package flowbuild implements flow constraint extraction from control flow graphs.
// This is the core of Phase B that transforms CFG structure into flow.Inputs,
// the constraint system that the flow solver uses to compute type narrowing.
//
// # EXTRACTION PIPELINE
//
// The Run function executes a multi-stage extraction pipeline:
//
//  1. Declarations: Extract type keys, declared types, and module aliases
//     from the CFG. This seeds the initial type information.
//
//  2. Const Propagation: Collect constant assignments and propagate constant
//     values through the CFG for use in branch analysis.
//
//  3. Assignments: Extract type assignments from local declarations,
//     assignments, and function definitions. This captures the flow of
//     types through variables.
//
//  4. Table Mutators: Extract assignments from table mutation operations
//     (table.insert, table.remove, etc.) that modify container types.
//
//  5. Return Classification: Classify return statements for multi-return
//     inference and nil-return detection.
//
//  6. Edge Constraints: Extract type constraints from branch conditions
//     (if, while, for) that narrow types on specific control flow edges.
//
//  7. Call Constraints: Extract OnReturn constraints from function calls
//     that narrow types based on call results (e.g., error returns).
//
//  8. Termination: Mark edges after terminating calls (error(), assert(false))
//     as unreachable with false conditions.
//
// # OUTPUT
//
// The output is a flow.Inputs struct containing all extracted constraints:
//   - DeclaredTypes: Symbol to type mappings
//   - EdgeConditions: Type constraints on CFG edges
//   - ConstValues: Constant value information
//   - PredicateLinks: Call-site predicate connections
//   - ReturnKinds: Return statement classifications
package flowbuild

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/constprop"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/decl"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// coreDecomposer implements flow.TypeDecomposer using query/core functions.
type coreDecomposer struct{}

func (coreDecomposer) ElementType(t typ.Type) typ.Type { return core.ElementType(t) }
func (coreDecomposer) KeyType(t typ.Type) typ.Type     { return core.KeyType(t) }
func (coreDecomposer) ValueType(t typ.Type) typ.Type   { return core.ValueType(t) }

// Run executes the complete flow constraint extraction pipeline.
// It processes the CFG to extract all type constraints that the flow solver
// needs to compute narrowed types at each program point.
//
// The extraction order is designed to build dependencies incrementally:
// declarations first, then assignments, then branch conditions. Each stage
// can use information from previous stages (e.g., const values in branches).
//
// Returns nil if the graph or its CFG is nil.
func Run(fc *fbcore.FlowContext) *flow.Inputs {
	if fc.Graph == nil || fc.Graph.CFG() == nil {
		return nil
	}

	inputs := initInputsFromContext(fc)

	// Declarations: type keys, declared types, module aliases.
	decl.ExtractTypeKeys(fc, inputs)
	decl.ExtractDeclaredTypes(fc, inputs)
	decl.ExtractModuleAliases(fc, inputs)

	// Compute derived resolvers and store in a separate derived bundle.
	derived := &fbcore.Derived{
		SymResolver:     resolve.BuildInputSymbolResolver(fc.CheckCtx, inputs),
		TypeKeyRes:      resolve.BuildContextTypeKeyResolver(fc.CheckCtx),
		RefinementBySym: resolve.BuildRefinementLookup(fc.CheckCtx),
	}
	if fc.API != nil {
		derived.Synth = fc.API.TypeOf
	}
	fc.Derived = derived

	// Const propagation.
	constprop.CollectConstAssignments(fc, inputs)
	constprop.PropagateAllConstValues(fc, inputs)

	// Assignments with const resolution.
	assign.ExtractAssignments(fc, inputs, keyscoll.BuildKeysCollectorDetector(fc.Graph, fc.ModuleBindings))

	// Table mutator assignments (table.insert-like).
	mutator.ExtractTableMutatorAssignments(fc, inputs)

	// Container mutator assignments (channel.send-like).
	mutator.ExtractContainerMutatorAssignments(fc, inputs)

	// Function definitions on table fields (function M.add()).
	assign.ExtractFuncDefAssignments(fc, inputs)

	// Return classification.
	returns.ExtractReturnKinds(fc, inputs)

	// Edge constraints from branches.
	cond.ExtractEdgeConstraints(fc, inputs)

	// Call OnReturn constraints, merged into edges.
	callConstraints := cond.ExtractCallOnReturnConstraints(fc, inputs)
	MergeCallConstraintsIntoEdges(inputs, callConstraints)

	// Mark terminating call edges as unreachable (error(), etc.).
	for _, p := range fc.Graph.RPO() {
		if fc.Derived == nil {
			continue
		}
		if !cond.PointHasTerminatingCallSite(fc.Graph, p, fc.Derived.Synth, fc.Derived.SymResolver, fc.Derived.RefinementBySym, fc.ModuleBindings) {
			continue
		}
		for _, succ := range fc.Graph.Successors(p) {
			inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{
				From:      p,
				To:        succ,
				Condition: constraint.FalseCondition(),
			})
		}
	}

	// Mark return points with no predecessors as dead.
	markDeadReturns(fc.Graph, inputs)

	// Numeric constraints.
	cond.ExtractNumericConstraints(fc, inputs)

	return inputs
}

// initInputsFromContext creates and seeds the Inputs struct from FlowContext.
func initInputsFromContext(fc *fbcore.FlowContext) *flow.Inputs {
	initialTypes := make(map[cfg.SymbolID]typ.Type)
	for sym, t := range fc.InitialDeclaredTypes {
		if sym != 0 && t != nil {
			initialTypes[sym] = t
		}
	}

	moduleAliases := make(map[cfg.SymbolID]string)
	for sym, path := range fc.ModuleAliases {
		moduleAliases[sym] = path
	}

	return &flow.Inputs{
		Graph:              fc.Graph,
		Decomposer:         coreDecomposer{},
		DeclaredTypes:      initialTypes,
		ConstValues:        make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		TypeKeys:           make(map[uint64]typ.Type),
		ReturnKinds:        make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints:  make(map[cfg.Point]flow.ReturnExprConstraints),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
		ModuleAliases:      moduleAliases,
		SiblingTypes:       fc.SiblingTypes,
		LiteralTypes:       fc.LiteralTypes,
	}
}

// MergeCallConstraintsIntoEdges merges call OnReturn conditions into edge constraints.
func MergeCallConstraintsIntoEdges(inputs *flow.Inputs, callConstraints map[cond.EdgeKey]constraint.Condition) {
	if len(callConstraints) == 0 {
		return
	}

	keys := make([]cond.EdgeKey, 0, len(callConstraints))
	for key := range callConstraints {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b cond.EdgeKey) int {
		if a.From != b.From {
			return int(a.From) - int(b.From)
		}
		return int(a.To) - int(b.To)
	})
	for _, key := range keys {
		c := callConstraints[key]
		if !c.HasConstraints() {
			continue
		}
		inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{
			From:      key.From,
			To:        key.To,
			Condition: c,
		})
	}
}

// markDeadReturns marks return points with no predecessors as dead.
func markDeadReturns(graph *cfg.Graph, inputs *flow.Inputs) {
	entry := graph.Entry()
	graph.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		if p == entry {
			return
		}
		if len(graph.Predecessors(p)) == 0 {
			if inputs.DeadPoints == nil {
				inputs.DeadPoints = make(map[cfg.Point]bool)
			}
			inputs.DeadPoints[p] = true
		}
	})
}

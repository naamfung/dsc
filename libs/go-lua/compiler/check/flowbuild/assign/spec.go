package assign

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CollectSpecNarrowedTypes collects spec-narrowed and inferred types using a worklist fixpoint.
// This handles transitive dependencies (e.g., ch = listen({message=true}), msg = ch:receive()).
//
// Algorithm:
//  1. Build dependency map: symbol -> assign points that use it as receiver
//  2. Seed worklist with points that have known types (spec-narrowed or synthesized)
//  3. Process worklist: synth with current SpecTypes, enqueue dependents if type changes
//  4. Continue until fixpoint (no new types added)
//
// Convergence: The algorithm terminates because:
//   - Each symbol can only be added to bySymbol once (monotone growth)
//   - Worklist only adds points when a new symbol is narrowed
//   - Finite number of symbols bounds iterations
//
// Determinism: Guaranteed by sorted point processing and stable queue order.
func CollectSpecNarrowedTypes(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	synthAPI api.SynthAPI,
	moduleBindings *bind.BindingTable,
) api.SpecTypes {
	bySymbol := make(api.SpecTypes)
	var bindings *bind.BindingTable
	if graph != nil {
		bindings = graph.Bindings()
	}
	if bindings == nil {
		bindings = moduleBindings
	}

	// Build dependency map: symbol -> points where it's used as method receiver
	deps := BuildReceiverDependencies(graph)

	// Collect and sort assign points for deterministic iteration
	points := graph.AssignPoints()

	// Seed phase: collect spec-narrowed types AND inferred types from method calls
	var worklist []cfg.Point
	for _, p := range points {
		info := graph.Assign(p)
		if info == nil {
			continue
		}
		sc := scopes[p]
		expanded := expandedAssignValues(synthAPI, info, p, nil)
		info.EachTarget(func(i int, target cfg.AssignTarget) {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}
			call, _ := info.CallForTarget(i)
			if call == nil {
				return
			}
			sym := target.Symbol

			// Try spec narrowing first
			if narrowed := NarrowReturnTypeBySpec(call, sc, synth, p, symResolver, graph, bindings, moduleBindings); narrowed != nil {
				bySymbol[sym] = narrowed
				worklist = append(worklist, deps[sym]...)
				return
			}

			// Fall back to regular synthesis for method calls only
			// This captures t = time.now() where the return type is known
			// Only capture non-union types to avoid interfering with narrowing
			if call.Method != "" && synth != nil {
				inferred := assignValueAt(expanded, i)
				if inferred == nil {
					if source := info.SourceAt(i); source != nil {
						inferred = synth(source, p)
					}
				}
				if typ.IsUnknownOrNil(inferred) {
					return
				}
				// Skip union types - they may need narrowing later
				if _, isUnion := unwrap.Alias(inferred).(*typ.Union); !isUnion {
					bySymbol[sym] = inferred
					worklist = append(worklist, deps[sym]...)
				}
			}
		})
	}

	// Fixpoint phase: process worklist until no new types are added
	for len(worklist) > 0 {
		// Sort for deterministic processing order
		slices.Sort(worklist)
		p := worklist[0]
		worklist = worklist[1:]

		info := graph.Assign(p)
		if info == nil {
			continue
		}
		expanded := expandedAssignValues(synthAPI, info, p, bySymbol)

		info.EachTarget(func(i int, target cfg.AssignTarget) {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}
			sym := target.Symbol

			// Skip if already has a type (monotone: don't overwrite)
			if _, exists := bySymbol[sym]; exists {
				return
			}

			// Check if this is a method call on a known receiver
			call, _ := info.CallForTarget(i)
			if call == nil {
				return
			}
			if call.Receiver == nil {
				return
			}
			recvSym := call.ReceiverSymbol
			if recvSym == 0 {
				return
			}
			if _, hasKnownRecv := bySymbol[recvSym]; !hasKnownRecv {
				return
			}

			// Synth method call with known receiver via synthAPI.
			// Use assignment-wide expansion so target-to-return mapping follows Lua
			// multi-return semantics (including trailing targets from final call).
			if value := assignValueAt(expanded, i); !typ.IsUnknownOrNil(value) {
				bySymbol[sym] = value
				// Enqueue points that depend on this newly typed symbol
				worklist = append(worklist, deps[sym]...)
			}
		})
	}

	return bySymbol
}

// BuildReceiverDependencies builds a map from symbol to points where it's used as method receiver.
func BuildReceiverDependencies(graph *cfg.Graph) map[cfg.SymbolID][]cfg.Point {
	deps := make(map[cfg.SymbolID][]cfg.Point)
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		seen := make(map[cfg.SymbolID]bool)
		info.EachSourceCall(func(_ int, call *cfg.CallInfo) {
			if call == nil {
				return
			}
			if call.Receiver == nil || call.ReceiverSymbol == 0 {
				return
			}
			if seen[call.ReceiverSymbol] {
				return
			}
			seen[call.ReceiverSymbol] = true
			deps[call.ReceiverSymbol] = append(deps[call.ReceiverSymbol], p)
		})
	})
	return deps
}

// NarrowReturnTypeBySpec checks if a call's return type should be narrowed
// based on the function's contract spec and inline literal argument values.
//
// This only handles inline table constructor literals like {message = true}.
// It does NOT handle variable references (e.g., local opts = {...}; f(opts)).
func NarrowReturnTypeBySpec(
	callInfo *cfg.CallInfo,
	sc *scope.State,
	synth func(ast.Expr, cfg.Point) typ.Type,
	p cfg.Point,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) typ.Type {
	if callInfo == nil || synth == nil {
		return nil
	}

	// Get the function type via synthesis
	var fnType typ.Type
	if callInfo.Callee != nil {
		fnType = synth(callInfo.Callee, p)
	}
	// Use canonical callsite symbol candidates for identity-based lookup.
	if fnType == nil && symResolver != nil {
		for _, calleeSym := range callsite.CallableCalleeSymbolCandidates(callInfo, graph, bindings, moduleBindings) {
			if calleeSym == 0 {
				continue
			}
			if t, ok := symResolver(p, calleeSym); ok && t != nil {
				fnType = t
				break
			}
		}
	}
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	if spec == nil || spec.Return == nil || len(spec.Return.Cases) == 0 {
		return nil
	}

	// Check each return case against inline literal arguments
	if t := transform.ReturnTypeFromSpec(spec, callInfo.Args); t != nil {
		return t
	}

	return nil
}

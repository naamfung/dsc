package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/typ"
)

// globalSetup records a _G.name = expr assignment (non-nil value).
type globalSetup struct {
	point cfg.Point
	name  string
	expr  ast.Expr
}

// globalClear records a _G.name = nil assignment.
type globalClear struct {
	point cfg.Point
	name  string
}

// paramCall records a call to a function parameter.
type paramCall struct {
	point      cfg.Point
	paramIndex int
}

// inferCallbackEnvOverlays detects the "setup -> param call -> cleanup" pattern
// using dominance and post-dominance to bracket the callback call.
// Returns map[paramIndex]map[globalName]typ.Type, or nil if no pattern detected.
func inferCallbackEnvOverlays(
	graph *cfg.Graph,
	paramSlots []cfg.ParamSlot,
	synthExpr func(ast.Expr, cfg.Point) typ.Type,
	moduleBindings *bind.BindingTable,
) map[int]map[string]typ.Type {
	if graph == nil || len(paramSlots) == 0 {
		return nil
	}

	paramSet := make(map[cfg.SymbolID]int, len(paramSlots))
	for _, slot := range paramSlots {
		if srcIdx, ok := slot.SourceParamIndex(); ok && slot.Symbol != 0 {
			paramSet[slot.Symbol] = srcIdx
		}
	}

	var setups []globalSetup
	var clears []globalClear
	var calls []paramCall

	// Collect global setups and clears from assignments.
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || info.IsLocal {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if target.Kind != cfg.TargetField {
				return
			}
			if target.BaseName != "_G" || len(target.FieldPath) != 1 {
				return
			}
			name := target.FieldPath[0]
			if src == nil {
				return
			}
			if _, isNil := src.(*ast.NilExpr); isNil {
				clears = append(clears, globalClear{point: p, name: name})
			} else {
				setups = append(setups, globalSetup{point: p, name: name, expr: src})
			}
		})
	})

	// Collect parameter calls.
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		sym := callsite.SelectPreferredSymbol(
			callsite.CallableCalleeSymbolCandidates(info, graph, graph.Bindings(), moduleBindings),
			func(candidate cfg.SymbolID) bool {
				_, ok := paramSet[candidate]
				return ok
			},
		)
		if idx, ok := paramSet[sym]; ok {
			calls = append(calls, paramCall{point: p, paramIndex: idx})
		}
	})

	if len(setups) == 0 || len(clears) == 0 || len(calls) == 0 {
		return nil
	}

	baseCFG := graph.CFG()
	if baseCFG == nil {
		return nil
	}

	idom, _ := analysis.ComputeDominators(baseCFG)
	postIdom, _ := analysis.ComputePostDominators(baseCFG)

	result := make(map[int]map[string]typ.Type)

	for _, call := range calls {
		for _, setup := range setups {
			if !analysis.Dominates(idom, setup.point, call.point) {
				continue
			}
			matched := false
			for _, clr := range clears {
				if clr.name != setup.name {
					continue
				}
				if analysis.PostDominates(postIdom, clr.point, call.point) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}

			t := synthExpr(setup.expr, setup.point)
			if t == nil {
				continue
			}

			if result[call.paramIndex] == nil {
				result[call.paramIndex] = make(map[string]typ.Type)
			}
			result[call.paramIndex][setup.name] = t
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

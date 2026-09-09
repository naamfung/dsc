package constprop

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/numparse"
)

// CollectConstAssignments collects constant assignments from the graph
// and stores them in inputs.ConstValues.
func CollectConstAssignments(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil || inputs == nil {
		return
	}
	values := make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue)

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}
			val := ConstValueFromExpr(source)
			if val == nil {
				return
			}

			sym := target.Symbol

			if values[sym] == nil {
				values[sym] = make(map[cfg.Point]*flow.ConstValue)
			}
			values[sym][p] = val
		})
	})

	inputs.ConstValues = values
}

// PropagateAllConstValues propagates const values for all variables
// using a worklist algorithm over the CFG.
func PropagateAllConstValues(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil || fc.Graph.CFG() == nil || inputs == nil {
		return
	}
	assigns := inputs.ConstValues
	out := make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue)
	for _, sym := range cfg.SortedSymbolIDs(assigns) {
		out[sym] = propagateConstValues(fc.Graph.CFG(), assigns[sym])
	}
	inputs.ConstValues = out
}

// ConstValueFromExpr extracts a constant value from an expression.
func ConstValueFromExpr(expr ast.Expr) *flow.ConstValue {
	switch e := expr.(type) {
	case *ast.StringExpr:
		return &flow.ConstValue{Kind: flow.ConstString, Str: e.Value}
	case *ast.NumberExpr:
		if idx, ok := numparse.ParseIntegerLiteral(e.Value); ok {
			return &flow.ConstValue{Kind: flow.ConstInt, Int: idx}
		}
		if f, ok := numparse.ParseFloatLiteral(e.Value); ok {
			return &flow.ConstValue{Kind: flow.ConstFloat, Float: f}
		}
	case *ast.TrueExpr:
		return &flow.ConstValue{Kind: flow.ConstBool, Bool: true}
	case *ast.FalseExpr:
		return &flow.ConstValue{Kind: flow.ConstBool, Bool: false}
	case *ast.NilExpr:
		return &flow.ConstValue{Kind: flow.ConstNil}
	}
	return nil
}

// ConstValueEqual compares two const values for equality.
func ConstValueEqual(a, b *flow.ConstValue) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case flow.ConstString:
		return a.Str == b.Str
	case flow.ConstInt:
		return a.Int == b.Int
	case flow.ConstFloat:
		return a.Float == b.Float
	case flow.ConstBool:
		return a.Bool == b.Bool
	case flow.ConstNil:
		return true
	case flow.ConstUnknown:
		return true
	default:
		return false
	}
}

// propagateConstValues propagates constant values through the CFG using a worklist algorithm.
func propagateConstValues(g *basecfg.CFG, assigns map[cfg.Point]*flow.ConstValue) map[cfg.Point]*flow.ConstValue {
	if g == nil {
		return nil
	}
	values := make(map[cfg.Point]*flow.ConstValue, g.Size())
	worklist := g.RPO()
	inQueue := make(map[cfg.Point]bool, len(worklist))
	for _, p := range worklist {
		inQueue[p] = true
	}

	for len(worklist) > 0 {
		p := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		inQueue[p] = false

		next := recomputeConstValue(g, p, assigns, values)
		if !ConstValueEqual(next, values[p]) {
			values[p] = next
			for _, succ := range g.Successors(p) {
				if !inQueue[succ] {
					worklist = append(worklist, succ)
					inQueue[succ] = true
				}
			}
		}
	}
	return values
}

// recomputeConstValue recomputes the const value at a CFG point.
func recomputeConstValue(g *basecfg.CFG, p cfg.Point, assigns map[cfg.Point]*flow.ConstValue, values map[cfg.Point]*flow.ConstValue) *flow.ConstValue {
	node := g.Node(p)
	if node == nil {
		return nil
	}
	switch node.Kind {
	case cfg.NodeEntry:
		return nil
	case cfg.NodeJoin, cfg.NodeExit, cfg.NodeScopeEnter, cfg.NodeScopeExit:
		return mergeConstPreds(g, p, values)
	}
	pred := g.Predecessor(p)
	predVals := values[pred]
	if node.Kind == cfg.NodeAssign {
		if assigns != nil {
			if v := assigns[p]; v != nil {
				return v
			}
		}
	}
	return predVals
}

// mergeConstPreds merges const values from predecessors.
func mergeConstPreds(g *basecfg.CFG, p cfg.Point, values map[cfg.Point]*flow.ConstValue) *flow.ConstValue {
	preds := g.Predecessors(p)
	if len(preds) == 0 {
		return nil
	}
	var merged *flow.ConstValue
	for _, pred := range preds {
		val := values[pred]
		if val == nil {
			continue
		}
		if merged == nil {
			merged = val
			continue
		}
		if !ConstValueEqual(merged, val) {
			return &flow.ConstValue{Kind: flow.ConstUnknown}
		}
	}
	return merged
}

package assign

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/typ"
)

func expandedAssignValues(synthAPI api.SynthAPI, info *cfg.AssignInfo, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	if synthAPI == nil || info == nil || len(info.Targets) == 0 || len(info.Sources) == 0 {
		return nil
	}
	return synthAPI.ExpandValuesWithSpecTypes(info.Sources, len(info.Targets), p, specTypes)
}

// rhsSpecTypesAtAssignPoint overlays assignment target symbols with their
// pre-assignment types (joined across predecessors). This preserves Lua's RHS
// evaluation order when synthesizing `x = f(x, ...)` at a single CFG point.
func rhsSpecTypesAtAssignPoint(
	graph *cfg.Graph,
	info *cfg.AssignInfo,
	p cfg.Point,
	base api.SpecTypes,
	resolve checkcallsite.SymbolTypeAtPoint,
) api.SpecTypes {
	if graph == nil || info == nil || resolve == nil || len(info.Targets) == 0 {
		return base
	}

	targetSyms := make(map[cfg.SymbolID]bool, len(info.Targets))
	for _, target := range info.Targets {
		if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
			targetSyms[target.Symbol] = true
		}
	}
	if len(targetSyms) == 0 {
		return base
	}

	var out api.SpecTypes
	override := func(sym cfg.SymbolID, t typ.Type) {
		if t == nil || t.Kind().IsPlaceholder() {
			return
		}
		if out == nil {
			if len(base) == 0 {
				out = make(api.SpecTypes, len(targetSyms))
			} else {
				out = make(api.SpecTypes, len(base)+len(targetSyms))
				for k, v := range base {
					out[k] = v
				}
			}
		}
		out[sym] = typ.PruneSoftUnionMembers(t)
	}

	for sym := range targetSyms {
		joined := checkcallsite.PreAssignmentTypeAtJoinOrPoint(graph, p, sym, checkcallsite.SymbolTypeAtPoint(resolve))
		override(sym, joined)
	}

	if out != nil {
		return out
	}
	return base
}

func assignValueAt(values []typ.Type, i int) typ.Type {
	if i < 0 || i >= len(values) {
		return nil
	}
	return values[i]
}

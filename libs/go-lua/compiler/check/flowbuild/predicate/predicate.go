package predicate

import (
	"strconv"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
)

// LookupPredicateLink finds the predicate link for a variable.
// When multiple links exist for the same name (different def points),
// returns the one with the highest def point for deterministic results.
func LookupPredicateLink(name string, inputs *flow.Inputs) *flow.PredicateLink {
	if inputs == nil || inputs.PredicateLinks == nil || name == "" {
		return nil
	}
	prefix := name + "@"
	bestDef := -1
	var bestLink flow.PredicateLink
	for key, link := range inputs.PredicateLinks {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			def, err := strconv.Atoi(key[len(prefix):])
			if err != nil {
				continue
			}
			if def > bestDef {
				bestDef = def
				bestLink = link
			}
		}
	}
	if bestDef < 0 {
		return nil
	}
	return &bestLink
}

// LinkKey generates a unique key for predicate links.
func LinkKey(name string, defPoint cfg.Point) string {
	return name + "@" + strconv.Itoa(int(defPoint))
}

// BuildConstResolver creates a const resolver function for a given CFG point.
// Uses the graph's SymbolAt to resolve name -> SymbolID for const lookup.
func BuildConstResolver(inputs *flow.Inputs, p cfg.Point) func(string) *flow.ConstValue {
	if inputs == nil || inputs.ConstValues == nil || inputs.Graph == nil {
		return nil
	}
	return func(name string) *flow.ConstValue {
		sym, ok := inputs.Graph.SymbolAt(p, name)
		if !ok {
			return nil
		}
		if at := inputs.ConstValues[sym]; at != nil {
			return at[p]
		}
		return nil
	}
}

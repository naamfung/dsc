package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

type symbolSet struct {
	order []cfg.SymbolID
	seen  map[cfg.SymbolID]struct{}
}

const symbolSetMapThreshold = 8

func newSymbolSet(capacity int) *symbolSet {
	if capacity < 0 {
		capacity = 0
	}

	return &symbolSet{
		order: make([]cfg.SymbolID, 0, capacity),
	}
}

func (s *symbolSet) ensureSeen() {
	if s.seen != nil {
		return
	}

	s.seen = make(map[cfg.SymbolID]struct{}, len(s.order)+1)
	for _, existing := range s.order {
		s.seen[existing] = struct{}{}
	}
}

func (s *symbolSet) Add(sym cfg.SymbolID) {
	if sym == 0 {
		return
	}

	if s.seen != nil {
		if _, ok := s.seen[sym]; ok {
			return
		}
		s.seen[sym] = struct{}{}
		s.order = append(s.order, sym)
		return
	}

	for _, existing := range s.order {
		if existing == sym {
			return
		}
	}

	if len(s.order) >= symbolSetMapThreshold {
		s.ensureSeen()
		s.seen[sym] = struct{}{}
	}

	s.order = append(s.order, sym)
}

func (s *symbolSet) Slice() []cfg.SymbolID {
	return s.order
}

// SelectPreferredSymbol returns the first candidate and, if prefer is non-nil, returns
// the first candidate that satisfies the predicate.
func SelectPreferredSymbol(candidates []cfg.SymbolID, prefer func(cfg.SymbolID) bool) cfg.SymbolID {
	selected := cfg.SymbolID(0)
	for _, sym := range candidates {
		if selected == 0 {
			selected = sym
		}
		if prefer != nil && prefer(sym) {
			return sym
		}
	}
	return selected
}

func addAliasExpansion(set *symbolSet, graph *cfg.Graph, sym cfg.SymbolID) {
	if set == nil || graph == nil || sym == 0 {
		return
	}
	graph.EachAliasSymbol(sym, func(candidate cfg.SymbolID) bool {
		set.Add(candidate)
		return false
	})
}

func expandAliasCandidates(base []cfg.SymbolID, graph *cfg.Graph) []cfg.SymbolID {
	if graph == nil || len(base) == 0 {
		return base
	}
	set := newSymbolSet(len(base) * 2)
	for _, sym := range base {
		addAliasExpansion(set, graph, sym)
	}
	candidates := set.Slice()
	if len(candidates) == 0 {
		return base
	}
	return candidates
}

func exprSymbolCandidates(
	expr ast.Expr,
	raw cfg.SymbolID,
	primary *bind.BindingTable,
	fallback *bind.BindingTable,
) []cfg.SymbolID {
	set := newSymbolSet(3)
	set.Add(raw)
	set.Add(SymbolFromExpr(expr, primary))
	if fallback != primary {
		set.Add(SymbolFromExpr(expr, fallback))
	}
	return set.Slice()
}

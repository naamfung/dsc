package callsite

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// CalleeSymbolCandidates returns deterministic candidate symbols for a callsite.
//
// Candidate order:
//  1. raw call callee symbol
//  2. symbol resolved from primary bindings using call expression
//  3. symbol resolved from fallback bindings using call expression
//  4. method symbol resolved from primary bindings (receiver + method)
//  5. method symbol resolved from fallback bindings (receiver + method)
//  6. binding symbols with matching callee name (primary, then fallback)
func CalleeSymbolCandidates(info *cfg.CallInfo, primary, fallback *bind.BindingTable) []cfg.SymbolID {
	if info == nil {
		return nil
	}
	set := newSymbolSet(4)
	for _, sym := range exprSymbolCandidates(info.Callee, info.CalleeSymbol, primary, fallback) {
		set.Add(sym)
	}
	if methodSym, ok := methodCalleeSymbolFromCall(primary, nil, info); ok {
		set.Add(methodSym)
	}
	if fallback != nil && fallback != primary {
		if methodSym, ok := methodCalleeSymbolFromCall(fallback, nil, info); ok {
			set.Add(methodSym)
		}
	}
	if info.CalleeName != "" {
		if primary != nil {
			for _, sym := range primary.SymbolsByNameReadOnly(info.CalleeName) {
				set.Add(sym)
			}
		}
		if fallback != nil && fallback != primary {
			for _, sym := range fallback.SymbolsByNameReadOnly(info.CalleeName) {
				set.Add(sym)
			}
		}
	}
	return set.Slice()
}

// CallableCalleeSymbolCandidates expands callee candidates through direct-alias
// chains and includes method symbols resolvable through alias receiver bases.
//
// Candidate order is preserved and symbols are deduplicated.
func CallableCalleeSymbolCandidates(
	info *cfg.CallInfo,
	graph *cfg.Graph,
	primary, fallback *bind.BindingTable,
) []cfg.SymbolID {
	base := CalleeSymbolCandidates(info, primary, fallback)
	if graph == nil {
		return base
	}
	set := newSymbolSet(len(base)*2 + 2)
	for _, sym := range expandAliasCandidates(base, graph) {
		set.Add(sym)
	}

	// Method calls may resolve method symbol only through an alias receiver base
	// (for example, Alias:run() where Alias = T and T.run is defined).
	if methodSym, ok := methodCalleeSymbolFromCall(primary, graph, info); ok {
		addAliasExpansion(set, graph, methodSym)
	}
	if fallback != nil && fallback != primary {
		if methodSym, ok := methodCalleeSymbolFromCall(fallback, graph, info); ok {
			addAliasExpansion(set, graph, methodSym)
		}
	}

	candidates := set.Slice()
	if len(candidates) == 0 {
		return base
	}
	return candidates
}

// ResolverCalleeSymbolCandidates returns canonical callee candidates for
// resolver contexts.
//
// Selection order:
//  1. callee path symbol from call extraction (when present)
//  2. canonical callsite candidates with alias expansion
//
// Order is deterministic and candidates are deduplicated.
//
// NOTE: this intentionally includes the base callee-path symbol (often receiver
// identity for method calls). Use this for resolver-style fallback lookups that
// can tolerate non-callable intermediate symbols; for strict callable lookup
// paths, prefer CallableCalleeSymbolCandidates.
func ResolverCalleeSymbolCandidates(
	info *cfg.CallInfo,
	graph *cfg.Graph,
	primary, fallback *bind.BindingTable,
) []cfg.SymbolID {
	if info == nil {
		return nil
	}
	set := newSymbolSet(4)
	set.Add(info.CalleePath.Symbol)
	for _, sym := range CallableCalleeSymbolCandidates(info, graph, primary, fallback) {
		set.Add(sym)
	}
	return set.Slice()
}

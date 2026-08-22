package api

import "github.com/wippyai/go-lua/compiler/check/scope"

// ParentScopeForGraph resolves the canonical parent scope for a graph.
// It prefers the stable parent-scope snapshot recorded in store and falls
// back to fallback when no stable parent is available.
func ParentScopeForGraph(store ParentScopes, graphID uint64, fallback *scope.State) *scope.State {
	if store == nil || graphID == 0 {
		return fallback
	}
	parentHash := store.GraphParentHashOf(graphID)
	if parentHash == 0 {
		return fallback
	}
	if parent := store.Parents()[parentHash]; parent != nil {
		return parent
	}
	return fallback
}

// ParentHashForGraph resolves the canonical parent hash for a graph.
// It prefers the stable graph-parent hash recorded in store and falls back to
// fallback.Hash() when no stable hash exists.
func ParentHashForGraph(store ParentScopes, graphID uint64, fallback *scope.State) uint64 {
	if store != nil && graphID != 0 {
		if parentHash := store.GraphParentHashOf(graphID); parentHash != 0 {
			return parentHash
		}
	}
	if fallback != nil {
		return fallback.Hash()
	}
	return 0
}

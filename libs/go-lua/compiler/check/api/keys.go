// Package api defines the checker contract types used across phases and layers.
package api

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
)

// GraphKey uniquely identifies a graph within a parent scope for query lookups.
// The key is stable and comparable, enabling memoization across iterations.
type GraphKey struct {
	GraphID    uint64 // Unique CFG ID from SessionStore.Graphs()
	ParentHash uint64 // Parent scope hash from SessionStore.Parents()
}

// SymbolKey uniquely identifies a symbol within a parent scope.
type SymbolKey struct {
	Symbol     cfg.SymbolID
	ParentHash uint64
}

// FuncKey uniquely identifies a function analysis request for memoization purposes.
// The key combines three components to ensure cache correctness:
//
//   - GraphID: Unique identifier for the function's control flow graph. Each CFG
//     receives a monotonically increasing ID during construction, ensuring distinct
//     functions have distinct GraphIDs even if they have identical source code.
//
//   - ParentHash: Hash of the parent scope state. Functions with identical code but
//     different lexical environments (e.g., different captured variables or type
//     definitions in scope) must be analyzed separately.
//
//   - StoreRevision: Counter incremented at each fixpoint iteration boundary.
//     This ensures cached results are invalidated when inter-function summaries
//     (return types, effects, sibling types) change, forcing recomputation with
//     updated cross-function information.
type FuncKey struct {
	GraphID       uint64
	ParentHash    uint64
	StoreRevision uint64
}

// KeyForGraph creates a GraphKey from a graph and parent scope.
func KeyForGraph(graph *cfg.Graph, parentHash uint64) GraphKey {
	var graphID uint64
	if graph != nil {
		graphID = graph.ID()
	}
	return GraphKey{
		GraphID:    graphID,
		ParentHash: parentHash,
	}
}

// CompareGraphKeys provides canonical ordering for GraphKey.
func CompareGraphKeys(a, b GraphKey) int {
	if a.GraphID < b.GraphID {
		return -1
	}
	if a.GraphID > b.GraphID {
		return 1
	}
	if a.ParentHash < b.ParentHash {
		return -1
	}
	if a.ParentHash > b.ParentHash {
		return 1
	}
	return 0
}

// SortedGraphKeys returns GraphKeys from m in canonical order.
func SortedGraphKeys[T any](m map[GraphKey]T) []GraphKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]GraphKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return CompareGraphKeys(keys[i], keys[j]) < 0 })
	return keys
}

package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/internal"
)

// ComputeSymbolSCCs computes strongly connected components for SymbolID graphs.
//
// This function implements Tarjan's algorithm (via internal.ComputeSCCs) to find
// strongly connected components in the local function call graph. Each SCC
// represents a set of mutually recursive functions that must be analyzed together.
//
// The SCCs are returned in reverse topological order: functions that don't call
// other local functions come first, followed by functions that only call those,
// and so on. This ordering ensures that when processing an SCC, all functions
// it depends on have already been analyzed.
//
// Type conversion is performed to bridge cfg.SymbolID and the uint64-based
// internal SCC implementation.
func ComputeSymbolSCCs(adj map[cfg.SymbolID][]cfg.SymbolID) [][]cfg.SymbolID {
	if len(adj) == 0 {
		return nil
	}

	// Convert to uint64 map with deterministic key/edge ordering.
	u64Adj := make(map[uint64][]uint64, len(adj))
	for _, k := range cfg.SortedSymbolIDs(adj) {
		edges := adj[k]
		if len(edges) > 1 {
			sorted := append([]cfg.SymbolID(nil), edges...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			edges = sorted
		}
		u64Edges := make([]uint64, len(edges))
		for i, e := range edges {
			u64Edges[i] = uint64(e)
		}
		u64Adj[uint64(k)] = u64Edges
	}

	// Compute SCCs
	u64SCCs := internal.ComputeSCCs(u64Adj)

	// Convert back to SymbolID
	sccs := make([][]cfg.SymbolID, len(u64SCCs))
	for i, scc := range u64SCCs {
		symSCC := make([]cfg.SymbolID, len(scc))
		for j, s := range scc {
			symSCC[j] = cfg.SymbolID(s)
		}
		sccs[i] = symSCC
	}

	return sccs
}

package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestComputeSymbolSCCs_MutualRecursion(t *testing.T) {
	// A calls B, B calls A - should be single SCC
	adj := map[cfg.SymbolID][]cfg.SymbolID{
		1: {2},
		2: {1},
	}
	result := ComputeSymbolSCCs(adj)
	if len(result) != 1 {
		t.Errorf("mutual recursion should produce 1 SCC, got %d", len(result))
	}
	if len(result[0]) != 2 {
		t.Errorf("SCC should contain both symbols, got %d", len(result[0]))
	}
}

func TestComputeSymbolSCCs_Chain(t *testing.T) {
	// A -> B -> C (no cycles) - should be 3 separate SCCs
	adj := map[cfg.SymbolID][]cfg.SymbolID{
		1: {2},
		2: {3},
		3: {},
	}
	result := ComputeSymbolSCCs(adj)
	if len(result) != 3 {
		t.Errorf("chain should produce 3 SCCs, got %d", len(result))
	}
}

func TestComputeSymbolSCCs_SelfRecursive(t *testing.T) {
	// A calls itself
	adj := map[cfg.SymbolID][]cfg.SymbolID{
		1: {1},
	}
	result := ComputeSymbolSCCs(adj)
	if len(result) != 1 {
		t.Errorf("self-recursive should produce 1 SCC, got %d", len(result))
	}
}

func TestComputeSymbolSCCs_MultipleSeparateSCCs(t *testing.T) {
	// Two separate mutual recursion groups
	adj := map[cfg.SymbolID][]cfg.SymbolID{
		1: {2},
		2: {1},
		3: {4},
		4: {3},
	}
	result := ComputeSymbolSCCs(adj)
	if len(result) != 2 {
		t.Errorf("two separate cycles should produce 2 SCCs, got %d", len(result))
	}
}

func TestComputeSymbolSCCs_TopologicalOrder(t *testing.T) {
	// A -> B (no cycle), verify B comes before A in result (reverse topo order)
	adj := map[cfg.SymbolID][]cfg.SymbolID{
		1: {2},
		2: {},
	}
	result := ComputeSymbolSCCs(adj)
	if len(result) != 2 {
		t.Fatalf("expected 2 SCCs, got %d", len(result))
	}
	// In reverse topological order, leaf nodes (no outgoing to unvisited) come first
	// So symbol 2 (no deps) should be in result[0]
	if result[0][0] != 2 {
		t.Errorf("leaf node should come first in reverse topo order, got %v", result[0][0])
	}
}

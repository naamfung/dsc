package internal

import (
	"reflect"
	"testing"
)

func TestComputeSCCs_Empty(t *testing.T) {
	t.Parallel()

	adj := map[uint64][]uint64{}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 0 {
		t.Errorf("expected empty, got %v", sccs)
	}
}

func TestComputeSCCs_SingleNode(t *testing.T) {
	t.Parallel()

	adj := map[uint64][]uint64{
		1: {},
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 1 || len(sccs[0]) != 1 || sccs[0][0] != 1 {
		t.Errorf("expected [[1]], got %v", sccs)
	}
}

func TestComputeSCCs_SelfLoop(t *testing.T) {
	t.Parallel()

	adj := map[uint64][]uint64{
		1: {1},
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 1 || len(sccs[0]) != 1 || sccs[0][0] != 1 {
		t.Errorf("expected [[1]], got %v", sccs)
	}
}

func TestComputeSCCs_TwoNodeCycle(t *testing.T) {
	t.Parallel()

	adj := map[uint64][]uint64{
		1: {2},
		2: {1},
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 1 {
		t.Fatalf("expected 1 SCC, got %d", len(sccs))
	}

	expected := []uint64{1, 2}
	if !reflect.DeepEqual(sccs[0], expected) {
		t.Errorf("expected %v, got %v", expected, sccs[0])
	}
}

func TestComputeSCCs_ThreeNodeCycle(t *testing.T) {
	t.Parallel()

	adj := map[uint64][]uint64{
		1: {2},
		2: {3},
		3: {1},
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 1 {
		t.Fatalf("expected 1 SCC, got %d", len(sccs))
	}

	expected := []uint64{1, 2, 3}
	if !reflect.DeepEqual(sccs[0], expected) {
		t.Errorf("expected %v, got %v", expected, sccs[0])
	}
}

func TestComputeSCCs_Chain(t *testing.T) {
	t.Parallel()

	// 1 -> 2 -> 3 (no cycles)
	adj := map[uint64][]uint64{
		1: {2},
		2: {3},
		3: {},
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 3 {
		t.Fatalf("expected 3 SCCs, got %d", len(sccs))
	}

	// Topological order: 3 first, then 2, then 1
	if sccs[0][0] != 3 || sccs[1][0] != 2 || sccs[2][0] != 1 {
		t.Errorf("expected topological order [3],[2],[1], got %v", sccs)
	}
}

func TestComputeSCCs_DiamondWithCycle(t *testing.T) {
	t.Parallel()

	// 1 -> 2 -> 4
	// 1 -> 3 -> 4
	// 4 -> 1 (creates cycle 1-2-4-1 and 1-3-4-1)
	adj := map[uint64][]uint64{
		1: {2, 3},
		2: {4},
		3: {4},
		4: {1},
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 1 {
		t.Fatalf("expected 1 SCC, got %d", len(sccs))
	}

	if len(sccs[0]) != 4 {
		t.Errorf("expected 4 nodes in SCC, got %d", len(sccs[0]))
	}
}

func TestComputeSCCs_Determinism(t *testing.T) {
	t.Parallel()

	adj := map[uint64][]uint64{
		5: {3},
		3: {1},
		1: {5},
		2: {4},
		4: {2},
	}

	// Run multiple times to verify determinism
	var firstResult [][]uint64

	for iteration := range 10 {
		sccs := ComputeSCCs(adj)
		if firstResult == nil {
			firstResult = sccs
		} else if !reflect.DeepEqual(sccs, firstResult) {
			t.Errorf("non-deterministic result on iteration %d: got %v, expected %v", iteration, sccs, firstResult)
		}
	}
}

func TestComputeSCCs_TopologicalOrder(t *testing.T) {
	t.Parallel()

	// A -> B means A depends on B.
	// Expected order: [B] first (no dependencies), then [A] (depends on B).
	adj := map[uint64][]uint64{
		1: {2}, // A(1) -> B(2)
		2: {},  // B(2) has no deps
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 2 {
		t.Fatalf("expected 2 SCCs, got %d: %v", len(sccs), sccs)
	}

	// B (node 2) should come before A (node 1) in topological order
	if sccs[0][0] != 2 {
		t.Errorf("expected first SCC to be [2], got %v", sccs[0])
	}

	if sccs[1][0] != 1 {
		t.Errorf("expected second SCC to be [1], got %v", sccs[1])
	}
}

func TestComputeSCCs_ComplexTopologicalOrder(t *testing.T) {
	t.Parallel()

	// Diamond: A -> B, A -> C, B -> D, C -> D
	// Expected order: D first, then B and C (in some order), then A
	adj := map[uint64][]uint64{
		1: {2, 3}, // A -> B, C
		2: {4},    // B -> D
		3: {4},    // C -> D
		4: {},     // D has no deps
	}
	sccs := ComputeSCCs(adj)

	if len(sccs) != 4 {
		t.Fatalf("expected 4 SCCs, got %d: %v", len(sccs), sccs)
	}

	// D (4) must come first
	if sccs[0][0] != 4 {
		t.Errorf("expected first SCC to be [4], got %v", sccs[0])
	}

	// A (1) must come last
	if sccs[3][0] != 1 {
		t.Errorf("expected last SCC to be [1], got %v", sccs[3])
	}

	// B and C must be in the middle (order between them is deterministic)
	middle := []uint64{sccs[1][0], sccs[2][0]}
	valid := (middle[0] == 2 && middle[1] == 3) || (middle[0] == 3 && middle[1] == 2)
	if !valid {
		t.Errorf("expected middle SCCs to be [2] and [3], got %v", middle)
	}
}

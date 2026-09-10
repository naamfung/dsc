package internal

import "sort"

// sccState holds state for Tarjan's algorithm.
type sccState struct {
	index    int
	indices  map[uint64]int
	lowlinks map[uint64]int
	onStack  map[uint64]bool
	stack    []uint64
	sccs     [][]uint64
	adj      map[uint64][]uint64
}

// ComputeSCCs computes strongly connected components using Tarjan's algorithm.
// Returns SCCs in topological order (dependencies resolved first).
func ComputeSCCs(adj map[uint64][]uint64) [][]uint64 {
	if len(adj) == 0 {
		return nil
	}

	state := &sccState{
		index:    0,
		indices:  make(map[uint64]int),
		lowlinks: make(map[uint64]int),
		onStack:  make(map[uint64]bool),
		stack:    nil,
		sccs:     nil,
		adj:      adj,
	}

	// Collect all nodes in deterministic order
	nodes := make([]uint64, 0, len(adj))
	for sym := range adj {
		nodes = append(nodes, sym)
	}

	sort.Slice(nodes, func(index, jindex int) bool {
		return nodes[index] < nodes[jindex]
	})

	for _, sym := range nodes {
		if _, visited := state.indices[sym]; !visited {
			state.strongconnect(sym)
		}
	}

	// Tarjan outputs in reverse topological order (leaves/dependencies first),
	// which is exactly what we want for fixpoint iteration.
	return state.sccs
}

func (s *sccState) strongconnect(vertex uint64) {
	s.indices[vertex] = s.index
	s.lowlinks[vertex] = s.index
	s.index++
	s.stack = append(s.stack, vertex)
	s.onStack[vertex] = true

	// Sort successors for determinism
	successors := s.adj[vertex]
	sortedSucc := make([]uint64, len(successors))
	copy(sortedSucc, successors)

	sort.Slice(sortedSucc, func(index, jindex int) bool {
		return sortedSucc[index] < sortedSucc[jindex]
	})

	for _, successor := range sortedSucc {
		if _, visited := s.indices[successor]; !visited {
			s.strongconnect(successor)

			if s.lowlinks[successor] < s.lowlinks[vertex] {
				s.lowlinks[vertex] = s.lowlinks[successor]
			}
		} else if s.onStack[successor] {
			if s.indices[successor] < s.lowlinks[vertex] {
				s.lowlinks[vertex] = s.indices[successor]
			}
		}
	}

	if s.lowlinks[vertex] == s.indices[vertex] {
		var scc []uint64

		for {
			node := s.stack[len(s.stack)-1]
			s.stack = s.stack[:len(s.stack)-1]
			s.onStack[node] = false

			scc = append(scc, node)

			if node == vertex {
				break
			}
		}

		// Sort SCC members for determinism
		sort.Slice(scc, func(index, jindex int) bool {
			return scc[index] < scc[jindex]
		})

		s.sccs = append(s.sccs, scc)
	}
}

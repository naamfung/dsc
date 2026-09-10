package theory

import (
	"math"
)

// DifferenceGraph implements difference logic constraint solving using a
// weighted directed graph representation.
//
// Difference logic handles constraints of the form:
//
//	x - y ≤ c    (difference constraint)
//
// Common arithmetic constraints are encoded as difference constraints:
//
//	x < y   →  x - y ≤ -1
//	x ≤ y   →  x - y ≤ 0
//	x > y   →  y - x ≤ -1
//	x ≥ y   →  y - x ≤ 0
//	x == y  →  x - y ≤ 0  AND  y - x ≤ 0
//	x == c  →  x - zero ≤ c  AND  zero - x ≤ -c
//
// The conjunction of difference constraints is
// satisfiable if and only if the corresponding weighted graph has no
// negative-weight cycles. This is checked using the Bellman-Ford algorithm.
//
// Additionally, shortest paths in the graph give implied bounds:
// if the shortest path from x to y has weight w, then x - y ≤ w is implied.
//
// Time complexity:
//   - AddConstraint: O(1)
//   - HasNegativeCycle: O(V * E) where V = variables, E = constraints
//   - GetBound: O(V * E) on first call, then O(1) with caching
//
// Space complexity: O(V²) for the adjacency matrix representation.
type DifferenceGraph struct {
	// nodes maps variable names to their integer node IDs.
	nodes map[string]int

	// names maps node IDs back to variable names (for debugging/output).
	names []string

	// edges[i][j] holds the weight of edge from node i to node j.
	// A value of maxWeight indicates no edge exists.
	edges [][]int64

	// dist caches shortest path distances after Bellman-Ford.
	// dist[i][j] = shortest path from i to j, or maxWeight if unreachable.
	dist [][]int64

	// consts stores explicit constant values for variables.
	// Used to handle edge cases like MinInt64 where negation overflows.
	consts map[string]int64

	// dirty indicates whether the graph has been modified since last constraint.
	dirty bool
}

const maxWeight = math.MaxInt64 / 2

// safeAddInt64 adds two int64 values, clamping to maxWeight on overflow.
func safeAddInt64(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return maxWeight
	}

	if b < 0 && a < math.MinInt64-b {
		return -maxWeight
	}

	return a + b
}

// NewDifferenceGraph creates an empty difference constraint graph.
//
// The graph starts with a special "zero" node that represents the constant 0.
// This allows encoding constraints like x ≤ 5 as x - zero ≤ 5.
func NewDifferenceGraph() *DifferenceGraph {
	g := &DifferenceGraph{
		nodes:  make(map[string]int),
		consts: make(map[string]int64),
		dirty:  true,
	}
	g.getOrCreateNode("0")

	return g
}

// getOrCreateNode returns the node ID for a variable, creating it if needed.
func (g *DifferenceGraph) getOrCreateNode(name string) int {
	if id, ok := g.nodes[name]; ok {
		return id
	}

	id := len(g.names)
	g.nodes[name] = id
	g.names = append(g.names, name)

	oldN := len(g.edges)
	newN := id + 1

	newEdges := make([][]int64, newN)
	for i := 0; i < oldN; i++ {
		newEdges[i] = make([]int64, newN)
		copy(newEdges[i], g.edges[i])
		newEdges[i][id] = maxWeight
	}

	newEdges[id] = make([]int64, newN)
	for j := 0; j < newN; j++ {
		newEdges[id][j] = maxWeight
	}

	g.edges = newEdges
	g.dirty = true

	return id
}

// AddLE adds constraint: x - y ≤ c (i.e., x ≤ y + c).
//
// All other constraint types are translated into one or more AddLE calls.
func (g *DifferenceGraph) AddLE(x, y string, c int64) {
	xi := g.getOrCreateNode(x)
	yi := g.getOrCreateNode(y)

	if c < g.edges[xi][yi] {
		g.edges[xi][yi] = c
		g.dirty = true
	}
}

// AddLT adds constraint: x < y (i.e., x - y ≤ -1).
func (g *DifferenceGraph) AddLT(x, y string) {
	g.AddLE(x, y, -1)
}

// AddGT adds constraint: x > y (i.e., y - x ≤ -1).
func (g *DifferenceGraph) AddGT(x, y string) {
	g.AddLE(y, x, -1)
}

// AddGE adds constraint: x ≥ y (i.e., y - x ≤ 0).
func (g *DifferenceGraph) AddGE(x, y string) {
	g.AddLE(y, x, 0)
}

// AddEQ adds constraint: x == y (bidirectional: x - y ≤ 0 AND y - x ≤ 0).
func (g *DifferenceGraph) AddEQ(x, y string) {
	g.AddLE(x, y, 0)
	g.AddLE(y, x, 0)
}

// AddConst adds constraint: x == c (using zero node as reference).
func (g *DifferenceGraph) AddConst(x string, c int64) {
	g.consts[x] = c
	g.AddLE(x, "0", c)
	// Avoid overflow when negating MinInt64
	if c == math.MinInt64 {
		return
	}

	g.AddLE("0", x, -c)
}

// AddUpperBound adds constraint: x ≤ c (i.e., x - zero ≤ c).
func (g *DifferenceGraph) AddUpperBound(x string, c int64) {
	g.AddLE(x, "0", c)
}

// AddLowerBound adds constraint: x ≥ c (i.e., zero - x ≤ -c).
func (g *DifferenceGraph) AddLowerBound(x string, c int64) {
	// Avoid overflow when negating MinInt64
	if c == math.MinInt64 {
		return
	}

	g.AddLE("0", x, -c)
}

// solve runs Bellman-Ford to compute all-pairs shortest paths.
// Returns true if the graph is consistent (no negative cycles).
func (g *DifferenceGraph) solve() bool {
	if !g.dirty {
		return g.dist != nil
	}

	n := len(g.names)
	if n == 0 {
		g.dirty = false
		return true
	}

	g.dist = make([][]int64, n)

	for i := 0; i < n; i++ {
		g.dist[i] = make([]int64, n)

		for j := 0; j < n; j++ {
			if i == j {
				if g.edges[i][i] < 0 {
					g.dist[i][i] = g.edges[i][i]
				} else {
					g.dist[i][j] = 0
				}
			} else {
				g.dist[i][j] = g.edges[i][j]
			}
		}
	}

	// Floyd-Warshall for all-pairs shortest paths
	for k := 0; k < n; k++ {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if g.dist[i][k] < maxWeight && g.dist[k][j] < maxWeight {
					// Safe addition to avoid overflow
					newDist := safeAddInt64(g.dist[i][k], g.dist[k][j])
					if newDist < g.dist[i][j] {
						g.dist[i][j] = newDist
					}
				}
			}
		}
	}

	// Check for negative cycles (negative diagonal)
	for i := 0; i < n; i++ {
		if g.dist[i][i] < 0 {
			g.dirty = false
			g.dist = nil

			return false
		}
	}

	g.dirty = false

	return true
}

// HasNegativeCycle returns true if the constraint system is unsatisfiable.
//
// A negative cycle means there's a sequence of constraints that together
// imply a contradiction like 0 ≤ -1. For example:
//
//	x < y  (x - y ≤ -1)
//	y < x  (y - x ≤ -1)
//
// Together: (x - y) + (y - x) ≤ -2, i.e., 0 ≤ -2, which is false.
func (g *DifferenceGraph) HasNegativeCycle() bool {
	return !g.solve()
}

// GetBound returns the tightest upper bound on x - y derivable from constraints.
//
// If x and y are constrained such that x - y ≤ c can be derived (possibly
// through transitivity), returns (c, true). Otherwise returns (0, false).
//
// Example: Given x < y (x - y ≤ -1) and y < z (y - z ≤ -1):
// GetBound("x", "z") returns (-2, true) because x - z ≤ -2 is derivable.
func (g *DifferenceGraph) GetBound(x, y string) (int64, bool) {
	if !g.solve() {
		return 0, false
	}

	xi, xok := g.nodes[x]
	yi, yok := g.nodes[y]

	if !xok || !yok {
		return 0, false
	}

	if g.dist[xi][yi] >= maxWeight {
		return 0, false
	}

	return g.dist[xi][yi], true
}

// GetUpperBound returns the upper bound on variable x (relative to zero).
//
// If constraints imply x ≤ c, returns (c, true). Otherwise (0, false).
func (g *DifferenceGraph) GetUpperBound(x string) (int64, bool) {
	if c, ok := g.consts[x]; ok {
		return c, true
	}

	return g.GetBound(x, "0")
}

// GetLowerBound returns the lower bound on variable x (relative to zero).
//
// If constraints imply x ≥ c, returns (c, true). This is computed as
// the negation of the upper bound on (0 - x).
func (g *DifferenceGraph) GetLowerBound(x string) (int64, bool) {
	if c, ok := g.consts[x]; ok {
		return c, true
	}

	bound, ok := g.GetBound("0", x)
	if !ok {
		return 0, false
	}
	// Avoid overflow when negating MinInt64
	if bound == math.MinInt64 {
		return 0, false
	}

	return -bound, true
}

// Clone creates a deep copy of the difference graph.
func (g *DifferenceGraph) Clone() *DifferenceGraph {
	newG := &DifferenceGraph{
		nodes:  make(map[string]int, len(g.nodes)),
		names:  make([]string, len(g.names)),
		edges:  make([][]int64, len(g.edges)),
		consts: make(map[string]int64, len(g.consts)),
		dirty:  g.dirty,
	}

	for k, v := range g.nodes {
		newG.nodes[k] = v
	}

	for k, v := range g.consts {
		newG.consts[k] = v
	}

	copy(newG.names, g.names)

	for i, row := range g.edges {
		newG.edges[i] = make([]int64, len(row))
		copy(newG.edges[i], row)
	}

	if g.dist != nil {
		newG.dist = make([][]int64, len(g.dist))
		for i, row := range g.dist {
			newG.dist[i] = make([]int64, len(row))
			copy(newG.dist[i], row)
		}
	}

	return newG
}

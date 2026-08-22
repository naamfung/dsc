// Package analysis provides pure graph analysis algorithms for CFGs.
package analysis

import (
	"slices"

	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// DomInfo holds dominator tree and dominance frontier information.
type DomInfo struct {
	ImmediateDominators map[basecfg.Point]basecfg.Point   // idom[n] = immediate dominator of n
	DominatorTree       map[basecfg.Point][]basecfg.Point // children in dominator tree
	DominanceFrontier   map[basecfg.Point][]basecfg.Point // DF[n] = dominance frontier of n
}

// DenseDomInfo stores dominator data in point-indexed slices.
// Missing entries are represented as nil/zero values.
type DenseDomInfo struct {
	DominatorTree     [][]basecfg.Point
	DominanceFrontier [][]basecfg.Point
}

type predecessorsReader interface {
	PredecessorsReadOnly(basecfg.Point) []basecfg.Point
}

type successorsReader interface {
	SuccessorsReadOnly(basecfg.Point) []basecfg.Point
}

type rpoReader interface {
	RPOReadOnly() []basecfg.Point
}

func predecessorsOf(g basecfg.Graph, point basecfg.Point) []basecfg.Point {
	if direct, ok := g.(predecessorsReader); ok {
		return direct.PredecessorsReadOnly(point)
	}

	return g.Predecessors(point)
}

func successorsOf(g basecfg.Graph, point basecfg.Point) []basecfg.Point {
	if direct, ok := g.(successorsReader); ok {
		return direct.SuccessorsReadOnly(point)
	}

	return g.Successors(point)
}

func rpoOf(g basecfg.Graph) []basecfg.Point {
	if direct, ok := g.(rpoReader); ok {
		return direct.RPOReadOnly()
	}

	return g.RPO()
}

// ComputeDominators computes immediate dominators and the dominator tree.
// Uses the Cooper-Harvey-Kennedy algorithm with RPO iteration.
func ComputeDominators(g basecfg.Graph) (idom map[basecfg.Point]basecfg.Point, domTree map[basecfg.Point][]basecfg.Point) {
	rpo := rpoOf(g)
	if len(rpo) == 0 {
		return make(map[basecfg.Point]basecfg.Point), make(map[basecfg.Point][]basecfg.Point)
	}

	graphSize := g.Size()

	if graphSize == 0 {
		return make(map[basecfg.Point]basecfg.Point), make(map[basecfg.Point][]basecfg.Point)
	}

	// Build RPO number lookup for intersection and deterministic sorting.
	rpoNum := make([]int, graphSize)
	for i, p := range rpo {
		if int(p) >= graphSize {
			continue
		}

		rpoNum[p] = i
	}

	entry := g.Entry()
	if int(entry) >= graphSize {
		return make(map[basecfg.Point]basecfg.Point), make(map[basecfg.Point][]basecfg.Point)
	}

	idomByPoint := make([]basecfg.Point, graphSize)
	hasIDom := make([]bool, graphSize)
	idomByPoint[entry] = entry
	hasIDom[entry] = true

	// intersect finds the common dominator of two nodes
	intersect := func(b1, b2 basecfg.Point) basecfg.Point {
		finger1, finger2 := b1, b2

		for finger1 != finger2 {
			for rpoNum[finger1] > rpoNum[finger2] {
				finger1 = idomByPoint[finger1]
			}

			for rpoNum[finger2] > rpoNum[finger1] {
				finger2 = idomByPoint[finger2]
			}
		}

		return finger1
	}

	// Iterate until fixed point
	changed := true
	for changed {
		changed = false
		for _, b := range rpo {
			if b == entry {
				continue
			}

			if int(b) >= graphSize {
				continue
			}

			preds := predecessorsOf(g, b)
			if len(preds) == 0 {
				continue
			}

			// Find first predecessor with defined idom
			var newIdom basecfg.Point

			found := false

			for _, p := range preds {
				if int(p) >= graphSize {
					continue
				}

				if hasIDom[p] {
					newIdom = p
					found = true

					break
				}
			}

			if !found {
				continue
			}

			// Intersect with other defined predecessors
			for _, p := range preds {
				if p == newIdom {
					continue
				}

				if int(p) >= graphSize {
					continue
				}

				if hasIDom[p] {
					newIdom = intersect(p, newIdom)
				}
			}

			if !hasIDom[b] || idomByPoint[b] != newIdom {
				idomByPoint[b] = newIdom
				hasIDom[b] = true
				changed = true
			}
		}
	}

	idom = make(map[basecfg.Point]basecfg.Point, len(rpo))
	for _, point := range rpo {
		if int(point) >= graphSize || !hasIDom[point] {
			continue
		}

		idom[point] = idomByPoint[point]
	}

	// Build dominator tree from idom
	domTree = make(map[basecfg.Point][]basecfg.Point, len(idom))

	for n, dom := range idom {
		if n != dom {
			domTree[dom] = append(domTree[dom], n)
		}
	}

	// Sort children for deterministic order.
	for p := range domTree {
		slices.SortFunc(domTree[p], func(a, b basecfg.Point) int {
			if rpoNum[a] < rpoNum[b] {
				return -1
			}
			if rpoNum[a] > rpoNum[b] {
				return 1
			}
			return 0
		})
	}

	return idom, domTree
}

// minPredecessorsForDF is the minimum number of predecessors needed for dominance frontier computation.
const minPredecessorsForDF = 2

func computeDominanceFrontierDense(
	g basecfg.Graph,
	rpo []basecfg.Point,
	rpoNum []int,
	idomByPoint []basecfg.Point,
	hasIDom []bool,
) [][]basecfg.Point {
	graphSize := g.Size()
	if graphSize == 0 || len(rpo) == 0 {
		return nil
	}

	dfByPoint := make([][]basecfg.Point, graphSize)
	runnerMark := make([]uint32, graphSize)
	var markEpoch uint32 = 1

	for _, block := range rpo {
		if int(block) >= graphSize {
			continue
		}

		preds := predecessorsOf(g, block)
		if len(preds) < minPredecessorsForDF {
			continue
		}

		blockIdx := int(block)
		if !hasIDom[blockIdx] {
			continue
		}
		domBlock := idomByPoint[blockIdx]
		markEpoch++
		if markEpoch == 0 {
			for i := range runnerMark {
				runnerMark[i] = 0
			}
			markEpoch = 1
		}

		for _, pred := range preds {
			if int(pred) >= graphSize {
				continue
			}

			runner := pred

			for runner != domBlock {
				runnerIdx := int(runner)
				if runnerIdx >= graphSize {
					break
				}

				if runnerMark[runnerIdx] == markEpoch {
					// Already processed this runner for this block.
					// Ancestors on this idom path were already covered too.
					break
				}
				runnerMark[runnerIdx] = markEpoch
				dfByPoint[runnerIdx] = append(dfByPoint[runnerIdx], block)

				if !hasIDom[runnerIdx] {
					break
				}

				dom := idomByPoint[runnerIdx]
				if dom == runner {
					break
				}

				runner = dom
			}
		}
	}

	for _, sortedDF := range dfByPoint {
		if len(sortedDF) <= 1 {
			continue
		}
		slices.SortFunc(sortedDF, func(a, b basecfg.Point) int {
			if rpoNum[a] < rpoNum[b] {
				return -1
			}
			if rpoNum[a] > rpoNum[b] {
				return 1
			}
			return 0
		})
	}

	return dfByPoint
}

// ComputeDominanceFrontier computes the dominance frontier for each node.
// DF[n] contains all nodes y such that n dominates a predecessor of y
// but does not strictly dominate y.
func ComputeDominanceFrontier(g basecfg.Graph, idom map[basecfg.Point]basecfg.Point) map[basecfg.Point][]basecfg.Point {
	rpo := rpoOf(g)
	if len(rpo) == 0 {
		return make(map[basecfg.Point][]basecfg.Point)
	}

	graphSize := g.Size()
	if graphSize == 0 {
		return make(map[basecfg.Point][]basecfg.Point)
	}

	// Build RPO number lookup for stable sorting.
	rpoNum := make([]int, graphSize)
	for i, point := range rpo {
		if int(point) >= graphSize {
			continue
		}

		rpoNum[point] = i
	}

	idomByPoint := make([]basecfg.Point, graphSize)
	hasIDom := make([]bool, graphSize)
	for point, dom := range idom {
		if int(point) >= graphSize {
			continue
		}

		idomByPoint[point] = dom
		hasIDom[point] = true
	}

	dfByPoint := computeDominanceFrontierDense(g, rpo, rpoNum, idomByPoint, hasIDom)

	df := make(map[basecfg.Point][]basecfg.Point, len(rpo))

	for pointIdx, sortedDF := range dfByPoint {
		if len(sortedDF) == 0 {
			continue
		}
		df[basecfg.Point(pointIdx)] = sortedDF
	}

	return df
}

// ComputeDomInfoDense computes dominator tree/frontier in dense point-indexed form.
func ComputeDomInfoDense(g basecfg.Graph) *DenseDomInfo {
	rpo := rpoOf(g)
	graphSize := g.Size()
	if len(rpo) == 0 || graphSize == 0 {
		return &DenseDomInfo{}
	}

	// Build RPO number lookup for intersection and deterministic sorting.
	rpoNum := make([]int, graphSize)
	for i, p := range rpo {
		if int(p) >= graphSize {
			continue
		}
		rpoNum[p] = i
	}

	entry := g.Entry()
	if int(entry) >= graphSize {
		return &DenseDomInfo{}
	}

	idomByPoint := make([]basecfg.Point, graphSize)
	hasIDom := make([]bool, graphSize)
	idomByPoint[entry] = entry
	hasIDom[entry] = true

	// intersect finds the common dominator of two nodes.
	intersect := func(pointA, pointB basecfg.Point) basecfg.Point {
		fingerA, fingerB := pointA, pointB
		for fingerA != fingerB {
			for rpoNum[fingerA] > rpoNum[fingerB] {
				fingerA = idomByPoint[fingerA]
			}
			for rpoNum[fingerB] > rpoNum[fingerA] {
				fingerB = idomByPoint[fingerB]
			}
		}
		return fingerA
	}

	// Iterate until fixed point.
	changed := true
	for changed {
		changed = false
		for _, block := range rpo {
			if block == entry || int(block) >= graphSize {
				continue
			}

			preds := predecessorsOf(g, block)
			if len(preds) == 0 {
				continue
			}

			var newIDom basecfg.Point
			found := false
			for _, pred := range preds {
				predIdx := int(pred)
				if predIdx >= graphSize {
					continue
				}
				if hasIDom[predIdx] {
					newIDom = pred
					found = true
					break
				}
			}
			if !found {
				continue
			}

			for _, pred := range preds {
				if pred == newIDom {
					continue
				}
				predIdx := int(pred)
				if predIdx >= graphSize {
					continue
				}
				if hasIDom[predIdx] {
					newIDom = intersect(pred, newIDom)
				}
			}

			blockIdx := int(block)
			if !hasIDom[blockIdx] || idomByPoint[blockIdx] != newIDom {
				idomByPoint[blockIdx] = newIDom
				hasIDom[blockIdx] = true
				changed = true
			}
		}
	}

	domTree := make([][]basecfg.Point, graphSize)
	for _, block := range rpo {
		blockIdx := int(block)
		if blockIdx >= graphSize || !hasIDom[blockIdx] {
			continue
		}
		dom := idomByPoint[blockIdx]
		if block != dom {
			domIdx := int(dom)
			if domIdx >= 0 && domIdx < graphSize {
				domTree[domIdx] = append(domTree[domIdx], block)
			}
		}
	}
	for _, children := range domTree {
		if len(children) <= 1 {
			continue
		}
		slices.SortFunc(children, func(a, b basecfg.Point) int {
			if rpoNum[a] < rpoNum[b] {
				return -1
			}
			if rpoNum[a] > rpoNum[b] {
				return 1
			}
			return 0
		})
	}

	df := computeDominanceFrontierDense(g, rpo, rpoNum, idomByPoint, hasIDom)

	return &DenseDomInfo{
		DominatorTree:     domTree,
		DominanceFrontier: df,
	}
}

// ComputeDomInfo computes all dominator information in one call.
func ComputeDomInfo(g basecfg.Graph) *DomInfo {
	idom, domTree := ComputeDominators(g)
	df := ComputeDominanceFrontier(g, idom)

	return &DomInfo{
		ImmediateDominators: idom,
		DominatorTree:       domTree,
		DominanceFrontier:   df,
	}
}

// Dominates returns true if a dominates b (a is on every path from entry to b).
func Dominates(idom map[basecfg.Point]basecfg.Point, pointA, pointB basecfg.Point) bool {
	if pointA == pointB {
		return true
	}

	runner := pointB

	for {
		dom, ok := idom[runner]
		if !ok || dom == runner {
			return false
		}

		if dom == pointA {
			return true
		}

		runner = dom
	}
}

// StrictlyDominates returns true if a strictly dominates b (a dominates b and a != b).
func StrictlyDominates(idom map[basecfg.Point]basecfg.Point, pointA, pointB basecfg.Point) bool {
	if pointA == pointB {
		return false
	}

	return Dominates(idom, pointA, pointB)
}

// reversedGraph reverses a CFG for post-dominator computation.
type reversedGraph struct {
	g basecfg.Graph
}

func (r *reversedGraph) ID() uint64                         { return r.g.ID() }
func (r *reversedGraph) Entry() basecfg.Point               { return r.g.Exit() }
func (r *reversedGraph) Exit() basecfg.Point                { return r.g.Entry() }
func (r *reversedGraph) Node(p basecfg.Point) *basecfg.Node { return r.g.Node(p) }
func (r *reversedGraph) Predecessors(p basecfg.Point) []basecfg.Point {
	return successorsOf(r.g, p)
}
func (r *reversedGraph) Successor(point basecfg.Point) basecfg.Point {
	preds := predecessorsOf(r.g, point)
	if len(preds) > 0 {
		return preds[0]
	}

	return point
}
func (r *reversedGraph) Successors(p basecfg.Point) []basecfg.Point {
	return predecessorsOf(r.g, p)
}
func (r *reversedGraph) Edges() []basecfg.Edge {
	edges := r.g.Edges()
	reversed := make([]basecfg.Edge, len(edges))

	for i, edge := range edges {
		reversed[i] = basecfg.Edge{From: edge.To, To: edge.From, Cond: edge.Cond}
	}

	return reversed
}
func (r *reversedGraph) Size() int { return r.g.Size() }
func (r *reversedGraph) EdgeCond(from, to basecfg.Point) (bool, bool) {
	return r.g.EdgeCond(to, from)
}
func (r *reversedGraph) IsJoin(p basecfg.Point) bool   { return r.g.IsBranch(p) }
func (r *reversedGraph) IsBranch(p basecfg.Point) bool { return r.g.IsJoin(p) }

// RPO computes reverse post-order from the exit node following reversed edges.
func (r *reversedGraph) RPO() []basecfg.Point {
	graphSize := r.g.Size()
	visited := make([]bool, graphSize)
	order := make([]basecfg.Point, 0, graphSize)

	var visit func(point basecfg.Point)
	visit = func(point basecfg.Point) {
		if int(point) >= graphSize || visited[point] {
			return
		}

		visited[point] = true

		for _, succ := range predecessorsOf(r.g, point) {
			visit(succ)
		}

		order = append(order, point)
	}

	visit(r.Entry())

	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order
}

// ComputePostDominators computes post-dominators by running the dominator
// algorithm on the reversed CFG.
func ComputePostDominators(graph basecfg.Graph) (map[basecfg.Point]basecfg.Point, map[basecfg.Point][]basecfg.Point) {
	return ComputeDominators(&reversedGraph{g: graph})
}

// PostDominates returns true if a post-dominates b (a is on every path from b to exit).
func PostDominates(postIdom map[basecfg.Point]basecfg.Point, pointA, pointB basecfg.Point) bool {
	return Dominates(postIdom, pointA, pointB)
}

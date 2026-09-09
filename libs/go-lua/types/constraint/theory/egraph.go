// Package theory provides constraint solving theories for SMT-style reasoning.
//
// This file implements an E-graph (Equivalence Graph) for path equality with
// congruence closure. This data structure tracks
// equalities between terms with function applications (field/index access).
//
// # Core Concepts
//
// An E-graph maintains equivalence classes of terms (paths) with these properties:
//
//   - Reflexivity: x == x
//   - Symmetry: x == y implies y == x
//   - Transitivity: x == y and y == z implies x == z
//   - Congruence: x == y implies x.f == y.f for any field/index f
//
// # Design
//
// The E-graph uses Union-Find with path compression and union by rank for
// efficient equivalence class operations (nearly O(1) amortized).
//
// Congruence is handled by tracking "children" of each path. When two paths
// are unified, their children with matching segments are also unified recursively.
//
// # Usage
//
//	eg := NewEGraph()
//	eg.Register(pathX)        // Register paths
//	eg.Register(pathY)
//	eg.Register(pathXField)   // x.field
//	eg.Register(pathYField)   // y.field
//	eg.AddEquality(pathX, pathY)  // x == y implies x.field == y.field
//
// # References
//
// - "Congruence Closure" by Nelson & Oppen (1980)
// - "egg: Fast and Extensible E-graphs" (2021)
package theory

import (
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
)

// EGraph implements an E-graph for path equality with congruence closure.
// It tracks equivalence classes of paths and automatically propagates
// equality through field/index access (congruence).
type EGraph struct {
	// Union-Find structure
	parent map[constraint.PathKey]constraint.PathKey
	rank   map[constraint.PathKey]int

	// Children map: for each path, tracks its field/index extensions
	// e.g., for path "x", children["x"][".field"] = "x.field"
	// This enables congruence: if x == y, then x.field == y.field
	children map[constraint.PathKey]map[string]constraint.PathKey

	// Reverse mapping: child path -> (parent path, segment)
	// Used for propagating narrowings up to parent paths
	parentOf map[constraint.PathKey]parentInfo
}

type parentInfo struct {
	parent  constraint.PathKey
	segment string
}

// NewEGraph creates a new E-graph.
func NewEGraph() *EGraph {
	return &EGraph{
		parent:   make(map[constraint.PathKey]constraint.PathKey),
		rank:     make(map[constraint.PathKey]int),
		children: make(map[constraint.PathKey]map[string]constraint.PathKey),
		parentOf: make(map[constraint.PathKey]parentInfo),
	}
}

// Clone creates a deep copy of the E-graph.
func (e *EGraph) Clone() *EGraph {
	c := &EGraph{
		parent:   make(map[constraint.PathKey]constraint.PathKey, len(e.parent)),
		rank:     make(map[constraint.PathKey]int, len(e.rank)),
		children: make(map[constraint.PathKey]map[string]constraint.PathKey, len(e.children)),
		parentOf: make(map[constraint.PathKey]parentInfo, len(e.parentOf)),
	}
	for k, v := range e.parent {
		c.parent[k] = v
	}
	for k, v := range e.rank {
		c.rank[k] = v
	}
	for k, segs := range e.children {
		c.children[k] = make(map[string]constraint.PathKey, len(segs))
		for s, child := range segs {
			c.children[k][s] = child
		}
	}
	for k, v := range e.parentOf {
		c.parentOf[k] = v
	}
	return c
}

// Register adds a path to the E-graph if not already present.
// Also registers the parent-child relationship for congruence.
func (e *EGraph) Register(path constraint.Path) {
	key := path.Key()
	e.makeSet(key)

	// Register parent-child relationship for congruence
	if len(path.Segments) > 0 {
		parentPath := constraint.Path{
			Root:     path.Root,
			Symbol:   path.Symbol,
			Segments: path.Segments[:len(path.Segments)-1],
		}
		parentKey := parentPath.Key()
		e.makeSet(parentKey)

		// Track as child of parent
		lastSeg := path.Segments[len(path.Segments)-1]
		segKey := segmentKey(lastSeg)

		if e.children[parentKey] == nil {
			e.children[parentKey] = make(map[string]constraint.PathKey)
		}
		e.children[parentKey][segKey] = key

		// Track reverse mapping
		e.parentOf[key] = parentInfo{parent: parentKey, segment: segKey}
	}
}

// RegisterKey adds a pre-resolved key to the E-graph.
// Use this when the key is already computed via canonical resolution.
// Note: This only registers the key itself, not parent-child relationships.
func (e *EGraph) RegisterKey(key constraint.PathKey) {
	e.makeSet(key)
}

// segmentKey returns a string key for a segment (for children map).
func segmentKey(seg constraint.Segment) string {
	return constraint.FormatSegments([]constraint.Segment{seg})
}

// makeSet ensures a path exists in the union-find structure.
func (e *EGraph) makeSet(key constraint.PathKey) {
	if _, ok := e.parent[key]; !ok {
		e.parent[key] = key
		e.rank[key] = 0
	}
}

// Find returns the representative of the equivalence class containing key.
// Uses path compression for efficiency.
func (e *EGraph) Find(key constraint.PathKey) constraint.PathKey {
	e.makeSet(key)
	if e.parent[key] != key {
		e.parent[key] = e.Find(e.parent[key]) // path compression
	}
	return e.parent[key]
}

// Union merges the equivalence classes of two paths.
// Returns true if they were in different classes (merge happened).
// Automatically propagates congruence to children.
func (e *EGraph) Union(x, y constraint.PathKey) bool {
	rootX := e.Find(x)
	rootY := e.Find(y)

	if rootX == rootY {
		return false // already equivalent
	}

	// Union by rank
	if e.rank[rootX] < e.rank[rootY] {
		rootX, rootY = rootY, rootX
	}
	e.parent[rootY] = rootX
	if e.rank[rootX] == e.rank[rootY] {
		e.rank[rootX]++
	}

	e.propagateCongruence(rootX, rootY)

	return true
}

// propagateCongruence merges children of two paths that have matching segments.
// This implements: x == y implies x.f == y.f for all fields f.
func (e *EGraph) propagateCongruence(rootX, rootY constraint.PathKey) {
	childrenX := e.children[rootX]
	childrenY := e.children[rootY]

	if childrenX == nil || childrenY == nil {
		// Merge children maps
		if childrenY != nil {
			if e.children[rootX] == nil {
				e.children[rootX] = make(map[string]constraint.PathKey)
			}
			keys := make([]string, 0, len(childrenY))
			for seg := range childrenY {
				keys = append(keys, seg)
			}
			sort.Strings(keys)
			for _, seg := range keys {
				child := childrenY[seg]
				if existing, ok := e.children[rootX][seg]; ok {
					// Both have this segment - unify the children (recursive congruence)
					e.Union(existing, child)
				} else {
					e.children[rootX][seg] = child
				}
			}
		}
		return
	}

	// Both have children - find matching segments and unify
	keysX := make([]string, 0, len(childrenX))
	for seg := range childrenX {
		keysX = append(keysX, seg)
	}
	sort.Strings(keysX)
	for _, seg := range keysX {
		childX := childrenX[seg]
		if childY, ok := childrenY[seg]; ok {
			e.Union(childX, childY) // Recursive congruence
		}
	}

	// Merge remaining children from Y to X
	keysY := make([]string, 0, len(childrenY))
	for seg := range childrenY {
		keysY = append(keysY, seg)
	}
	sort.Strings(keysY)
	for _, seg := range keysY {
		childY := childrenY[seg]
		if _, ok := childrenX[seg]; !ok {
			e.children[rootX][seg] = childY
		}
	}
}

// AddEquality adds x == y to the E-graph with congruence propagation.
func (e *EGraph) AddEquality(x, y constraint.Path) bool {
	return e.Union(x.Key(), y.Key())
}

// AreEqual checks if two paths are in the same equivalence class.
func (e *EGraph) AreEqual(x, y constraint.PathKey) bool {
	return e.Find(x) == e.Find(y)
}

// GetEquivalenceClass returns all paths equivalent to the given path.
func (e *EGraph) GetEquivalenceClass(key constraint.PathKey) []constraint.PathKey {
	root := e.Find(key)
	var result []constraint.PathKey
	for k := range e.parent {
		if e.Find(k) == root {
			result = append(result, k)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

// AllPaths returns all registered paths.
func (e *EGraph) AllPaths() []constraint.PathKey {
	result := make([]constraint.PathKey, 0, len(e.parent))
	for k := range e.parent {
		result = append(result, k)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

// ClassRepresentatives returns the representative of each equivalence class.
func (e *EGraph) ClassRepresentatives() []constraint.PathKey {
	seen := make(map[constraint.PathKey]bool)
	var result []constraint.PathKey
	for k := range e.parent {
		root := e.Find(k)
		if !seen[root] {
			seen[root] = true
			result = append(result, root)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

package flow

import (
	"slices"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
)

// buildEdgeNumericConstraints populates the edge numeric constraint map.
//
// Numeric constraints arise from comparison operators (x < y, x >= 10, etc.)
// and are used to detect unreachable edges and track integer bounds. This
// method collects all numeric constraints from the inputs and indexes them
// by edge key for efficient lookup during constraint checking.
//
// Multiple constraints on the same edge are accumulated into a slice,
// as they all must be satisfied when traversing that edge.
func (s *Solution) buildEdgeNumericConstraints() {
	if s.inputs == nil {
		return
	}
	for _, edge := range s.inputs.EdgeNumericConstraints {
		if len(edge.Constraints) == 0 {
			continue
		}
		key := edgeKey{from: edge.From, to: edge.To}
		s.edgeNumericConstraints[key] = append(s.edgeNumericConstraints[key], edge.Constraints...)
	}
}

// checkNumericConstraints performs numeric constraint analysis to detect unreachable edges.
//
// This method implements a worklist-based dataflow analysis that propagates
// numeric state (bounds, orderings, modular constraints) through the CFG.
// At each edge with numeric constraints, it checks if the constraints are
// satisfiable given the current state. Unsatisfiable edges are marked
// unreachable in s.unsatEdges.
//
// The algorithm:
//  1. Computes relevant CFG points (those reachable from constraint edges)
//  2. Initializes a worklist with all relevant non-entry points
//  3. Iterates until fixed point, computing numeric state at each point
//  4. After convergence, checks each edge's constraints for satisfiability
//
// The computed numeric states are stored in s.numericStates for later queries
// (e.g., BoundsAt to get variable bounds at a specific point).
func (s *Solution) checkNumericConstraints() {
	if s.inputs == nil || s.inputs.Graph == nil || len(s.edgeNumericConstraints) == 0 {
		return
	}

	c := s.inputs.Graph
	relevant := s.computeRelevantPoints()
	if len(relevant) == 0 {
		return
	}

	state := make(map[cfg.Point]*numeric.State, len(relevant))
	worklist := make([]cfg.Point, 0, len(relevant))
	inQueue := make(map[cfg.Point]bool, len(relevant))

	for p := range relevant {
		if p != c.Entry() {
			worklist = append(worklist, p)
			inQueue[p] = true
		}
	}
	slices.Sort(worklist)

	maxIter := len(relevant) * 10
	for iter := 0; len(worklist) > 0 && iter < maxIter; iter++ {
		p := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		inQueue[p] = false

		newState := s.computeNumericStateAt(c, p, state)
		oldState := state[p]

		if !newState.Equals(oldState) {
			if newState == nil || newState.IsTop() {
				delete(state, p)
			} else {
				state[p] = newState
			}
			for _, succ := range graphSuccessors(c, p) {
				if relevant[succ] && !inQueue[succ] {
					worklist = append(worklist, succ)
					inQueue[succ] = true
				}
			}
		}
	}

	// Store computed numeric states for later queries
	s.numericStates = state

	edgeKeys := make([]edgeKey, 0, len(s.edgeNumericConstraints))
	for key := range s.edgeNumericConstraints {
		edgeKeys = append(edgeKeys, key)
	}
	slices.SortFunc(edgeKeys, func(a, b edgeKey) int {
		if a.from != b.from {
			return int(a.from) - int(b.from)
		}
		return int(a.to) - int(b.to)
	})
	for _, key := range edgeKeys {
		edgeConstraints := s.edgeNumericConstraints[key]
		if len(edgeConstraints) == 0 {
			continue
		}
		fromState := state[key.from]
		edgeState := fromState.Clone()
		if edgeState == nil {
			edgeState = numeric.NewState()
		}
		// Resolve constraint paths at edge source point
		resolver := s.resolverAt(key.from)
		for _, nc := range edgeConstraints {
			edgeState.ApplyConstraintWithResolver(nc, resolver)
		}
		if !edgeState.CheckSatisfiability() {
			s.unsatEdges[key] = true
		}
	}
}

// computeRelevantPoints identifies CFG points needed for numeric constraint analysis.
//
// Not all CFG points need numeric analysis - only those that can influence or
// be influenced by numeric constraint edges. This method computes the minimal
// set of relevant points by:
//
//  1. Seeding with all points involved in numeric constraint edges
//  2. Adding the entry point (starting state for analysis)
//  3. Forward propagation: adding all successors of seeds
//  4. Backward propagation: adding all predecessors of relevant points
//
// The result is the smallest subgraph containing all constraint edges and
// their transitive dependencies, enabling efficient analysis without
// processing irrelevant parts of the CFG.
func (s *Solution) computeRelevantPoints() map[cfg.Point]bool {
	if s.inputs == nil || s.inputs.Graph == nil {
		return nil
	}

	c := s.inputs.Graph
	relevant := make(map[cfg.Point]bool)
	seeds := make([]cfg.Point, 0)

	edgeKeys := make([]edgeKey, 0, len(s.edgeNumericConstraints))
	for key := range s.edgeNumericConstraints {
		edgeKeys = append(edgeKeys, key)
	}
	slices.SortFunc(edgeKeys, func(a, b edgeKey) int {
		if a.from != b.from {
			return int(a.from) - int(b.from)
		}
		return int(a.to) - int(b.to)
	})
	for _, key := range edgeKeys {
		if !relevant[key.from] {
			relevant[key.from] = true
			seeds = append(seeds, key.from)
		}
		if !relevant[key.to] {
			relevant[key.to] = true
			seeds = append(seeds, key.to)
		}
	}

	if !relevant[c.Entry()] {
		relevant[c.Entry()] = true
	}

	for i := 0; i < len(seeds); i++ {
		p := seeds[i]
		for _, succ := range graphSuccessors(c, p) {
			if !relevant[succ] {
				relevant[succ] = true
				seeds = append(seeds, succ)
			}
		}
	}

	backSeeds := make([]cfg.Point, 0, len(relevant))
	for p := range relevant {
		backSeeds = append(backSeeds, p)
	}
	slices.Sort(backSeeds)

	for i := 0; i < len(backSeeds); i++ {
		p := backSeeds[i]
		for _, pred := range graphPredecessors(c, p) {
			if !relevant[pred] {
				relevant[pred] = true
				backSeeds = append(backSeeds, pred)
			}
		}
	}

	return relevant
}

// computeNumericStateAt computes the numeric state at a CFG point by joining predecessors.
//
// For each predecessor, this method:
//  1. Gets the predecessor's numeric state
//  2. Clones and applies edge constraints (narrowing bounds/orderings)
//  3. Rekeys bounds for phi nodes (mapping old versions to new)
//  4. Checks if the resulting state is still satisfiable
//
// Multiple predecessor states are joined (intersection of facts) to produce
// the final state at p. States that become "top" (no constraints) are filtered
// out to avoid losing precision in the join.
//
// Returns nil if no predecessor has a non-top state (point has no numeric facts).
func (s *Solution) computeNumericStateAt(c cfg.Graph, p cfg.Point, state map[cfg.Point]*numeric.State) *numeric.State {
	preds := graphPredecessors(c, p)
	if len(preds) == 0 {
		return nil
	}

	var predStates []*numeric.State
	for _, pred := range preds {
		predState := state[pred]
		edgeConstraints := s.edgeNumericConstraints[edgeKey{from: pred, to: p}]

		if predState == nil && len(edgeConstraints) == 0 {
			continue
		}

		edgeState := predState.Clone()
		if edgeState == nil {
			edgeState = numeric.NewState()
		}

		if len(edgeConstraints) > 0 {
			// Resolve constraint paths at edge source point (pred)
			resolver := s.resolverAt(pred)
			for _, nc := range edgeConstraints {
				edgeState.ApplyConstraintWithResolver(nc, resolver)
			}
		}

		// Rekey bounds for phi nodes at join point p
		edgeState = s.rekeyForPhis(edgeState, pred, p)

		if edgeState.IsTop() {
			continue
		}
		predStates = append(predStates, edgeState)
	}

	if len(predStates) == 0 {
		return nil
	}
	if len(predStates) == 1 {
		return predStates[0]
	}

	result := predStates[0]
	for i := 1; i < len(predStates); i++ {
		result = numeric.Join(result, predStates[i])
	}
	return result
}

// resolverAt creates a path resolver for a specific CFG point.
//
// The resolver converts constraint paths to their canonical versioned keys
// at the given point. This is needed because numeric constraints reference
// paths, but the state tracks bounds by versioned keys. The resolver handles
// SSA version lookup for each path.
//
// Returns nil if no path key resolver is available.
func (s *Solution) resolverAt(p cfg.Point) func(path constraint.Path) constraint.PathKey {
	if s.pkResolver == nil {
		return nil
	}
	return func(path constraint.Path) constraint.PathKey {
		return s.pkResolver.KeyAt(p, path)
	}
}

// rekeyForPhis renames keys in the state for phi nodes at point p.
//
// At phi nodes, SSA versions from different predecessors merge into a new
// version. Numeric bounds tracked for the predecessor version must be renamed
// to the phi target version to remain useful after the merge.
//
// For example, if phi at point 5 merges x@1 (from pred 3) and x@2 (from pred 4)
// into x@5, bounds on x@1 arriving from predecessor 3 must be rekeyed to x@5.
//
// The method builds a remap table from operand versions to target versions,
// then applies it to create a new state with renamed keys. The original state
// is not modified.
func (s *Solution) rekeyForPhis(state *numeric.State, pred, p cfg.Point) *numeric.State {
	if state == nil || s.inputs == nil || s.pkResolver == nil {
		return state
	}

	// Find phi nodes at point p
	var relevantPhis []cfg.PhiNode
	for _, phi := range s.inputs.Graph.PhiNodes() {
		if phi.Point == p {
			relevantPhis = append(relevantPhis, phi)
		}
	}

	if len(relevantPhis) == 0 {
		return state
	}

	// Build mapping from old keys to new keys
	keyRemap := make(map[constraint.PathKey]constraint.PathKey)
	for _, phi := range relevantPhis {
		// Find the operand version coming from pred
		for _, op := range phi.Operands {
			// Check if this operand's definition point is reachable from pred
			oldKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
			newKey := s.pkResolver.KeyAtVersion(phi.Target.Symbol, phi.Target.ID, nil)
			if oldKey != "" && newKey != "" && oldKey != newKey {
				keyRemap[oldKey] = newKey
			}
		}
	}

	if len(keyRemap) == 0 {
		return state
	}

	// Create new state with rekeyed bounds
	return state.Rekey(keyRemap)
}

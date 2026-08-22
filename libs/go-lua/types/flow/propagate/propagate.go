// Package propagate computes type constraints at CFG points via forward propagation.
//
// Given edge conditions (constraints on control flow edges from conditionals),
// propagation computes the combined condition at each program point by merging
// conditions from all incoming paths. The result enables flow-sensitive type
// narrowing: at each point, the condition describes what type facts must hold.
//
// The algorithm uses a worklist with RPO ordering for efficient convergence.
// Conditions are represented in DNF (disjunctive normal form) and combined
// using logical AND (through edges) and OR (at join points).
package propagate

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// Graph provides CFG traversal operations for propagation.
//
// This interface abstracts the control flow graph, allowing propagation to
// work with any graph implementation that provides the required operations.
// Typically, this is a *cfg.CFG or cfg.VersionedGraph.
type Graph interface {
	// Entry returns the entry point of the CFG (function start).
	Entry() cfg.Point
	// RPO returns CFG points in reverse postorder (optimal iteration order).
	RPO() []cfg.Point
	// Node returns the CFG node at point p, or nil if none.
	Node(p cfg.Point) *cfg.Node
	// Predecessors returns all CFG points that have edges to p.
	Predecessors(p cfg.Point) []cfg.Point
	// Successors returns all CFG points that p has edges to.
	Successors(p cfg.Point) []cfg.Point
}

type predecessorReadOnly interface {
	PredecessorsReadOnly(p cfg.Point) []cfg.Point
}

type successorReadOnly interface {
	SuccessorsReadOnly(p cfg.Point) []cfg.Point
}

func graphPredecessors(g Graph, p cfg.Point) []cfg.Point {
	if ro, ok := g.(predecessorReadOnly); ok {
		return ro.PredecessorsReadOnly(p)
	}
	return g.Predecessors(p)
}

func graphSuccessors(g Graph, p cfg.Point) []cfg.Point {
	if ro, ok := g.(successorReadOnly); ok {
		return ro.SuccessorsReadOnly(p)
	}
	return g.Successors(p)
}

// EdgeConditions maps CFG edges to their guard conditions.
//
// An edge condition describes what must be true when control flows along
// that edge. For example, an if-then edge has a Truthy condition for the
// test expression, while the else edge has a Falsy condition.
type EdgeConditions map[EdgeKey]constraint.Condition

// EdgeKey identifies a CFG edge by its source and target points.
//
// A CFG edge represents control flow from one program point to another.
// Edges have conditions when they come from conditional branches (if, while, etc.).
type EdgeKey struct {
	From cfg.Point
	To   cfg.Point
}

// Assignment describes a variable assignment at a CFG point.
//
// Used for constraint killing: when a variable is reassigned, constraints
// on that variable's previous value become invalid and must be removed from
// the propagated condition.
//
// For example, after "x = 5", a prior constraint "type(x) == string" no longer
// applies because x now has a different value.
type Assignment struct {
	// Point is the CFG point where the assignment occurs.
	Point cfg.Point
	// TargetSym is the symbol being assigned (provides unique identity).
	TargetSym cfg.SymbolID
	// TargetSegs specifies field path for nested assignments (x.foo.bar = ...).
	// Empty for simple variable assignments.
	TargetSegs []constraint.Segment
}

// Inputs provides all data needed for constraint propagation.
//
// The propagation algorithm uses these inputs to compute which conditions
// hold at each CFG point, accounting for edge conditions, dead code, and
// constraint killing from assignments.
type Inputs struct {
	// Graph provides CFG traversal.
	Graph Graph
	// EdgeConditions maps edges to their guard conditions.
	EdgeConditions EdgeConditions
	// DeadPoints marks CFG points that are unreachable (e.g., after error()).
	// Propagation treats these as having False condition.
	DeadPoints map[cfg.Point]bool
	// Assignments lists all variable assignments for constraint killing.
	Assignments []Assignment
}

// Result holds the computed conditions at each CFG point.
//
// After propagation, PointConditions maps each reachable CFG point to the
// disjunction of all path conditions that reach that point. Unreachable
// points have False conditions or are absent from the map.
type Result struct {
	// PointConditions maps CFG points to the conditions that hold there.
	// A point's condition is the OR of (predecessor condition AND edge condition)
	// for all incoming edges.
	PointConditions map[cfg.Point]constraint.Condition
}

// Propagate computes type constraints at each CFG point via forward dataflow.
//
// This is the main entry point for constraint propagation. The algorithm:
//
//  1. Initialize entry point with True condition
//  2. Process points in worklist order (initially RPO for efficiency)
//  3. For each point, compute new condition from predecessors
//  4. If condition changed, add successors to worklist
//  5. Repeat until no changes (fixed point)
//
// Condition computation at each point:
//   - For each predecessor with non-False condition:
//   - Combine predecessor condition with edge condition (AND)
//   - Apply loop preheader reinforcement for loop headers
//   - Kill constraints for reassigned variables
//   - Merge all incoming conditions (OR)
//
// The result maps each point to its computed condition. Points unreachable
// from entry have False conditions (or are absent).
func Propagate(inputs *Inputs) *Result {
	if inputs == nil || inputs.Graph == nil {
		return &Result{PointConditions: make(map[cfg.Point]constraint.Condition)}
	}

	g := inputs.Graph
	pointConditions := make(map[cfg.Point]constraint.Condition)
	pointConditions[g.Entry()] = constraint.TrueCondition()

	worklist := g.RPO()
	inQueue := make(map[cfg.Point]bool, len(worklist))
	for _, p := range worklist {
		inQueue[p] = true
	}

	// Cache preheader conditions for monotonic convergence
	preheaderConds := make(map[cfg.Point]constraint.Condition)

	for len(worklist) > 0 {
		p := worklist[0]
		worklist = worklist[1:]
		inQueue[p] = false

		newCond := computeConditionAtPoint(inputs, pointConditions, preheaderConds, p)
		oldCond := pointConditions[p]

		changed := false
		if oldCond.IsFalse() || (!newCond.IsFalse() && !oldCond.Subsumes(newCond)) {
			merged := constraint.Or(oldCond, newCond)
			if !merged.Equals(oldCond) {
				pointConditions[p] = merged
				changed = true
			}
		}

		if changed {
			for _, succ := range graphSuccessors(g, p) {
				if !inQueue[succ] {
					worklist = append(worklist, succ)
					inQueue[succ] = true
				}
			}
		}
	}

	return &Result{PointConditions: pointConditions}
}

// computeConditionAtPoint computes the condition for a single CFG point.
//
// The condition is computed by combining all incoming paths:
//   - For each predecessor, AND its condition with the edge condition
//   - Apply loop preheader reinforcement if at a loop header
//   - OR all the incoming conditions together
//   - Kill constraints for variables reassigned at this point
//
// Special handling for loop headers: The preheader condition (before loop entry)
// is reinforced on backedge paths to preserve invariants. Loop-variant variables
// are filtered out to avoid unsound conclusions.
func computeConditionAtPoint(
	inputs *Inputs,
	pointConditions map[cfg.Point]constraint.Condition,
	preheaderConds map[cfg.Point]constraint.Condition,
	p cfg.Point,
) constraint.Condition {
	g := inputs.Graph
	node := g.Node(p)
	if node == nil || node.Kind == cfg.NodeEntry {
		return constraint.TrueCondition()
	}

	preds := graphPredecessors(g, p)
	if len(preds) == 0 {
		return constraint.FalseCondition()
	}

	if inputs.DeadPoints != nil && inputs.DeadPoints[p] {
		return constraint.FalseCondition()
	}

	var predConds []constraint.Condition
	for _, pred := range preds {
		if inputs.DeadPoints != nil && inputs.DeadPoints[pred] {
			continue
		}

		predCond, exists := pointConditions[pred]
		if !exists {
			continue
		}
		if predCond.IsFalse() {
			continue
		}

		// Loop header preheader reinforcement
		if node != nil && node.LoopPreheaderSet && pred != node.LoopPreheader {
			preCond, cached := preheaderConds[p]
			if !cached {
				preheaderComputedCond, ok := pointConditions[node.LoopPreheader]
				if !ok {
					goto skipPreheaderReinforcement
				}
				preCond = preheaderComputedCond
				if preEdge, ok := inputs.EdgeConditions[EdgeKey{From: node.LoopPreheader, To: p}]; ok && preEdge.HasConstraints() {
					preCond = constraint.And(preCond, preEdge)
				}
				preheaderConds[p] = preCond
			}
			if preCond.IsFalse() {
				continue
			}
			if preCond.HasConstraints() {
				if len(node.LoopVars) > 0 {
					preCond = FilterConditionSymbols(preCond, node.LoopVars)
				}
				if predCond.HasConstraints() {
					predCond = constraint.And(predCond, preCond)
				} else {
					predCond = preCond
				}
			}
		}
	skipPreheaderReinforcement:

		edgeCond, ok := inputs.EdgeConditions[EdgeKey{From: pred, To: p}]
		if !ok || (!edgeCond.HasConstraints() && !edgeCond.IsFalse()) {
			edgeCond = constraint.TrueCondition()
		}

		var combinedCond constraint.Condition
		switch {
		case edgeCond.IsFalse():
			continue
		case !edgeCond.HasConstraints():
			combinedCond = predCond
		case !predCond.HasConstraints():
			combinedCond = edgeCond
		default:
			combinedCond = constraint.And(predCond, edgeCond)
		}
		if combinedCond.IsFalse() {
			continue
		}

		predConds = append(predConds, combinedCond)
	}

	if len(predConds) == 0 {
		return constraint.FalseCondition()
	}

	var result constraint.Condition
	if len(predConds) == 1 {
		result = predConds[0]
	} else {
		result = predConds[0]
		for i := 1; i < len(predConds); i++ {
			result = constraint.Or(result, predConds[i])
		}
	}

	result = KillRedefinedConditions(result, p, inputs.Assignments)

	return result
}

// FilterConditionSymbols removes constraints that reference specified symbols.
//
// This is used to filter out loop-variant constraints when reinforcing loop
// preheader conditions. If a variable is modified in the loop body, constraints
// about its preheader value are invalid on backedges.
//
// For example, given loop "for i = 1, 10 do ... end", constraints on i from
// the preheader (i == 1) don't hold after the first iteration.
//
// If any disjunct becomes empty after filtering, returns True (the constraint
// had no non-loop-variant component and should not restrict narrowing).
func FilterConditionSymbols(cond constraint.Condition, syms []cfg.SymbolID) constraint.Condition {
	if len(syms) == 0 || !cond.HasConstraints() {
		return cond
	}

	symSet := make(map[cfg.SymbolID]struct{}, len(syms))
	for _, sym := range syms {
		if sym != 0 {
			symSet[sym] = struct{}{}
		}
	}
	if len(symSet) == 0 {
		return cond
	}

	var newDisjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		if len(d) == 0 {
			return constraint.TrueCondition()
		}
		var kept []constraint.Constraint
		for _, c := range d {
			shouldKeep := true
			constraint.VisitPaths(c, func(p constraint.Path) bool {
				if p.Symbol == 0 {
					return false
				}
				if _, ok := symSet[p.Symbol]; ok {
					shouldKeep = false
					return true
				}
				return false
			})
			if shouldKeep {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			return constraint.TrueCondition()
		}
		newDisjuncts = append(newDisjuncts, kept)
	}

	if len(newDisjuncts) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromDisjuncts(newDisjuncts)
}

// KillRedefinedConditions removes constraints for paths assigned at a point.
//
// When a variable is reassigned, constraints about its previous value become
// invalid. This function filters out such "killed" constraints to maintain
// soundness. Without killing, propagation would incorrectly conclude that
// preassignment constraints still hold.
//
// Example: Given "if type(x) == 'string' then x = 5 end", after the assignment,
// the constraint HasType(x, string) must be killed because x is now a number.
//
// The function checks each constraint path against each assignment at point p.
// If any constraint path is affected by an assignment, that constraint is removed.
func KillRedefinedConditions(cond constraint.Condition, p cfg.Point, assignments []Assignment) constraint.Condition {
	if !cond.HasConstraints() {
		return cond
	}

	var assignedPaths []Assignment
	for _, assign := range assignments {
		if assign.Point == p && assign.TargetSym != 0 {
			assignedPaths = append(assignedPaths, assign)
		}
	}

	if len(assignedPaths) == 0 {
		return cond
	}

	var newDisjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		var kept []constraint.Constraint
		for _, c := range d {
			shouldKeep := true
			constraint.VisitPaths(c, func(cpath constraint.Path) bool {
				if cpath.Symbol == 0 {
					return false
				}
				for _, ap := range assignedPaths {
					if PathAffectedByAssignment(cpath, ap.TargetSym, ap.TargetSegs) {
						shouldKeep = false
						return true
					}
				}
				return false
			})
			if shouldKeep {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			return constraint.TrueCondition()
		}
		newDisjuncts = append(newDisjuncts, kept)
	}

	if len(newDisjuncts) == 0 {
		return constraint.TrueCondition()
	}

	return constraint.FromDisjuncts(newDisjuncts)
}

// PathAffectedByAssignment checks if a constraint path is invalidated by an assignment.
//
// A path is affected if:
//  1. The constraint path's symbol matches the assignment symbol
//  2. The assignment's segments are a prefix of (or equal to) the constraint's segments
//
// Examples:
//   - Assigning to x affects x, x.foo, x.foo.bar (all have x's value)
//   - Assigning to x.foo affects x.foo, x.foo.bar (but not x or x.baz)
//   - Assigning to x.foo[0] affects x.foo[0], x.foo[0].bar (but not x.foo)
//
// This captures that assigning to a path invalidates both the path itself
// and any nested paths that depend on the assigned value.
func PathAffectedByAssignment(cpath constraint.Path, assignSym cfg.SymbolID, assignSegs []constraint.Segment) bool {
	if cpath.Symbol != assignSym {
		return false
	}

	if len(assignSegs) == 0 {
		return true
	}

	if len(cpath.Segments) < len(assignSegs) {
		return false
	}

	for i, seg := range assignSegs {
		cseg := cpath.Segments[i]
		if cseg.Kind != seg.Kind || cseg.Name != seg.Name || cseg.Index != seg.Index {
			return false
		}
	}

	return true
}

package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
)

// buildEdgeConditions constructs the edge condition map from input constraints.
//
// Edge conditions are type constraints that guard control flow edges. For example,
// in an if-else statement, the true branch edge has a Truthy condition and the
// false branch edge has a Falsy condition.
//
// This method merges multiple conditions for the same edge using logical AND.
// If an edge already has a condition (from a previous constraint), the new
// condition is ANDed with it. This handles nested conditionals where multiple
// constraints apply to the same branch.
//
// Empty conditions (no constraints and not false) are skipped to avoid polluting
// the map with no-op entries. The resulting map is used during propagation to
// apply type narrowing along specific control flow paths.
func (s *Solution) buildEdgeConditions() {
	if s.inputs == nil || len(s.inputs.EdgeConditions) == 0 {
		return
	}

	for _, ec := range s.inputs.EdgeConditions {
		if !ec.Condition.HasConstraints() && !ec.Condition.IsFalse() {
			continue
		}
		key := edgeKey{from: ec.From, to: ec.To}
		if existing, ok := s.edgeConditions[key]; ok && (existing.HasConstraints() || existing.IsFalse()) {
			s.edgeConditions[key] = constraint.And(existing, ec.Condition)
		} else {
			s.edgeConditions[key] = ec.Condition
		}
	}
}

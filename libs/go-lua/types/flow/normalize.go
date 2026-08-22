package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
)

// Normalize enforces deterministic ordering for slice-based inputs.
//
// This does not change semantics; it only stabilizes iteration order for
// reproducible analysis and diagnostics.
func (in *Inputs) Normalize() {
	if in == nil {
		return
	}

	if len(in.Assignments) > 1 {
		sort.Slice(in.Assignments, func(i, j int) bool {
			ai := in.Assignments[i]
			aj := in.Assignments[j]
			if ai.Point != aj.Point {
				return ai.Point < aj.Point
			}
			if pathLess(ai.TargetPath, aj.TargetPath) {
				return true
			}
			if pathLess(aj.TargetPath, ai.TargetPath) {
				return false
			}
			if pathLess(ai.SourcePath, aj.SourcePath) {
				return true
			}
			if pathLess(aj.SourcePath, ai.SourcePath) {
				return false
			}
			return iteratorSourceLess(ai.IterSource, aj.IterSource)
		})
	}

	if len(in.EdgeConditions) > 1 {
		sort.Slice(in.EdgeConditions, func(i, j int) bool {
			a := in.EdgeConditions[i]
			b := in.EdgeConditions[j]
			if a.From != b.From {
				return a.From < b.From
			}
			if a.To != b.To {
				return a.To < b.To
			}
			// Condition canonicalization handles determinism; no further tie-break needed.
			return false
		})
	}

	if len(in.EdgeNumericConstraints) > 1 {
		sort.Slice(in.EdgeNumericConstraints, func(i, j int) bool {
			a := in.EdgeNumericConstraints[i]
			b := in.EdgeNumericConstraints[j]
			if a.From != b.From {
				return a.From < b.From
			}
			if a.To != b.To {
				return a.To < b.To
			}
			return false
		})
	}

	if len(in.IndexerAssignments) > 1 {
		sort.Slice(in.IndexerAssignments, func(i, j int) bool {
			a := in.IndexerAssignments[i]
			b := in.IndexerAssignments[j]
			if a.Point != b.Point {
				return a.Point < b.Point
			}
			if a.Symbol != b.Symbol {
				return a.Symbol < b.Symbol
			}
			if a.Root != b.Root {
				return a.Root < b.Root
			}
			if segmentsLess(a.Segments, b.Segments) {
				return true
			}
			if segmentsLess(b.Segments, a.Segments) {
				return false
			}
			if a.KeySymbol != b.KeySymbol {
				return a.KeySymbol < b.KeySymbol
			}
			if a.KeyVar != b.KeyVar {
				return a.KeyVar < b.KeyVar
			}
			return false
		})
	}

	if len(in.TableMutatorAssignments) > 1 {
		sort.Slice(in.TableMutatorAssignments, func(i, j int) bool {
			a := in.TableMutatorAssignments[i]
			b := in.TableMutatorAssignments[j]
			if a.Point != b.Point {
				return a.Point < b.Point
			}
			if pathLess(a.Target, b.Target) {
				return true
			}
			if pathLess(b.Target, a.Target) {
				return false
			}
			if a.KeySymbol != b.KeySymbol {
				return a.KeySymbol < b.KeySymbol
			}
			if a.KeyVar != b.KeyVar {
				return a.KeyVar < b.KeyVar
			}
			if pathLess(a.ValuePath, b.ValuePath) {
				return true
			}
			if pathLess(b.ValuePath, a.ValuePath) {
				return false
			}
			return false
		})
	}

	if len(in.ContainerMutatorAssignments) > 1 {
		sort.Slice(in.ContainerMutatorAssignments, func(i, j int) bool {
			a := in.ContainerMutatorAssignments[i]
			b := in.ContainerMutatorAssignments[j]
			if a.Point != b.Point {
				return a.Point < b.Point
			}
			if pathLess(a.Target, b.Target) {
				return true
			}
			if pathLess(b.Target, a.Target) {
				return false
			}
			if pathLess(a.ValuePath, b.ValuePath) {
				return true
			}
			if pathLess(b.ValuePath, a.ValuePath) {
				return false
			}
			return false
		})
	}

	if len(in.WideningEvents) > 1 {
		sort.Slice(in.WideningEvents, func(i, j int) bool {
			a := in.WideningEvents[i]
			b := in.WideningEvents[j]
			if a.SCCIndex != b.SCCIndex {
				return a.SCCIndex < b.SCCIndex
			}
			if a.Symbol != b.Symbol {
				return a.Symbol < b.Symbol
			}
			return false
		})
	}
}

func iteratorSourceLess(a, b *IteratorSource) bool {
	if a == nil {
		return b != nil
	}
	if b == nil {
		return false
	}
	if pathLess(a.Path, b.Path) {
		return true
	}
	if pathLess(b.Path, a.Path) {
		return false
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.VarIndex < b.VarIndex
}

func pathLess(a, b constraint.Path) bool {
	if a.Symbol != b.Symbol {
		return a.Symbol < b.Symbol
	}
	if a.Root != b.Root {
		return a.Root < b.Root
	}
	if a.Version != b.Version {
		return a.Version < b.Version
	}
	return segmentsLess(a.Segments, b.Segments)
}

func segmentsLess(a, b []constraint.Segment) bool {
	min := len(a)
	if len(b) < min {
		min = len(b)
	}
	for i := 0; i < min; i++ {
		if a[i].Kind != b[i].Kind {
			return a[i].Kind < b[i].Kind
		}
		if a[i].Name != b[i].Name {
			return a[i].Name < b[i].Name
		}
		if a[i].Index != b[i].Index {
			return a[i].Index < b[i].Index
		}
	}
	return len(a) < len(b)
}

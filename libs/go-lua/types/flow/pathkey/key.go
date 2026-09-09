// key.go provides path key construction and manipulation utilities.
//
// Path keys are the canonical string identifiers for versioned paths in the
// flow solver. This file provides functions for building, parsing, and
// comparing path keys.
package pathkey

import (
	"sync"

	"github.com/wippyai/go-lua/types/constraint"
)

type eqPair struct {
	left, right constraint.Path
}

var parseSuffixCache sync.Map

type pathSet struct {
	paths map[uint64][]constraint.Path
}

// SegmentsSuffix converts path segments to a suffix string for key construction.
//
// The suffix format matches Lua syntax:
//   - Field segments: ".field"
//   - Integer index: "[0]"
//   - String index: "[\"key\"]" (with escaping)
//
// This is a thin wrapper around constraint.FormatSegments for API consistency
// within the pathkey package.
func SegmentsSuffix(segs []constraint.Segment) string {
	return constraint.FormatSegments(segs)
}

// ParseSuffix converts a suffix string like ".field" or "[0]" to segments.
//
// This is the inverse of SegmentsSuffix. It parses path suffixes extracted
// from canonical keys back into structured segments.
//
// Supported syntax:
//   - ".field" -> SegmentField{Name: "field"}
//   - "[0]" -> SegmentIndexInt{Index: 0}
//   - "[\"key\"]" -> SegmentIndexString{Name: "key"}
//   - "[key]" -> SegmentIndexString{Name: "key"} (legacy form)
//
// The parser handles multiple chained segments: ".foo[0].bar" produces
// [SegmentField{foo}, SegmentIndexInt{0}, SegmentField{bar}].
func ParseSuffix(suffix string) []constraint.Segment {
	if suffix == "" {
		return nil
	}
	if cached, ok := parseSuffixCache.Load(suffix); ok {
		return cached.([]constraint.Segment)
	}
	segs := parseSuffixSlow(suffix)
	if segs == nil {
		return nil
	}
	if cached, loaded := parseSuffixCache.LoadOrStore(suffix, segs); loaded {
		return cached.([]constraint.Segment)
	}
	return segs
}

func parseSuffixSlow(suffix string) []constraint.Segment {
	var segs []constraint.Segment
	i := 0
	for i < len(suffix) {
		switch suffix[i] {
		case '.':
			i++
			start := i
			for i < len(suffix) && suffix[i] != '.' && suffix[i] != '[' {
				i++
			}
			if start >= i {
				return nil
			}
			name := suffix[start:i]
			if !IsIdentName(name) {
				return nil
			}
			segs = append(segs, constraint.Segment{Kind: constraint.SegmentField, Name: name})
		case '[':
			i++
			if i >= len(suffix) {
				return nil
			}

			if suffix[i] == '"' {
				i++
				var out []byte
				for i < len(suffix) {
					ch := suffix[i]
					if ch == '\\' {
						if i+1 >= len(suffix) {
							return nil
						}
						escaped := suffix[i+1]
						if escaped != '\\' && escaped != '"' {
							return nil
						}
						out = append(out, escaped)
						i += 2
						continue
					}
					if ch == '"' {
						break
					}
					out = append(out, ch)
					i++
				}

				if i >= len(suffix) || suffix[i] != '"' {
					return nil
				}
				i++
				if i >= len(suffix) || suffix[i] != ']' {
					return nil
				}
				i++

				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexString, Name: string(out)})
				continue
			}

			start := i
			for i < len(suffix) && suffix[i] != ']' {
				i++
			}
			if start >= i {
				return nil
			}

			name := suffix[start:i]
			if i >= len(suffix) || suffix[i] != ']' {
				return nil
			}
			i++

			if idx, ok := ParseIntLiteral(name); ok {
				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx})
			} else {
				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexString, Name: name})
			}
		default:
			return nil
		}
	}
	return segs
}

// PathRelated returns true if target and other paths share identity and overlap.
//
// Two paths are related if:
//  1. They share the same symbol (SSA identity) or both are placeholders with
//     the same Root name
//  2. One is a prefix of the other (including equality)
//
// This relationship is used for constraint filtering: a constraint on x.foo
// is related to path x (constraint might narrow x) and to path x.foo.bar
// (constraint directly applies).
//
// Strictly requires symbol matching - no name-based fallback. If one path has
// a symbol and the other doesn't, they are not related. This prevents unsound
// constraint propagation across different scopes.
//
// Version mismatches do not block relation checks. Versioned staleness is
// handled by assignment-driven condition killing, while relation itself must
// stay stable across SSA versions so unaffected constraints (for sibling/root
// paths) survive field-only redefinitions.
func PathRelated(target constraint.Path, other constraint.Path) bool {
	if target.IsEmpty() || other.IsEmpty() {
		return false
	}

	// Both must have symbols, and they must match
	if target.Symbol != 0 && other.Symbol != 0 {
		if target.Symbol != other.Symbol {
			return false
		}
	} else if target.Symbol == 0 && other.Symbol == 0 {
		// Both are placeholders - compare by Root
		if target.Root != other.Root {
			return false
		}
	} else {
		// One has symbol, one doesn't - NOT related (no name fallback)
		return false
	}

	return SegmentsPrefix(target.Segments, other.Segments) || SegmentsPrefix(other.Segments, target.Segments)
}

// SegmentsPrefix returns true if a is a prefix of b (or equal).
//
// Segment comparison is exact: both kind and value must match.
// An empty slice is a prefix of any slice.
func SegmentsPrefix(a, b []constraint.Segment) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// FilterConstraintsForPath returns constraints relevant to a target path.
//
// A constraint is kept if it references the target path or an equivalent path
// (via EqPath constraints). This filtering is used before applying constraints
// to narrow a specific path, avoiding application of unrelated constraints.
//
// The function handles several cases:
//   - Direct reference: Constraint path related to target
//   - Equivalence: Constraint path equivalent to target via EqPath
//   - Field/Index relationships: Value paths from FieldEqualsPath constraints
//
// Special handling for NotEquals constraints: They narrow the target, not the
// value side. If filtering for a value-only path, NotEquals are dropped.
func FilterConstraintsForPath(constraints []constraint.Constraint, target constraint.Path) []constraint.Constraint {
	if len(constraints) == 0 || target.IsEmpty() {
		return constraints
	}

	pairs, relatedValuePaths := collectPathFilterFacts(constraints, target)
	if len(pairs) == 0 && relatedValuePaths.len() == 0 {
		var filtered []constraint.Constraint
		for _, c := range constraints {
			if shouldDropAsymmetricNotEquals(c, target) {
				continue
			}
			if constraintAnyPathMatches(c, func(p constraint.Path) bool {
				return PathRelated(target, p)
			}) {
				filtered = append(filtered, c)
			}
		}
		return filtered
	}

	equivalentPaths := collectEquivalentPathsFromPairs(pairs, target)
	var filtered []constraint.Constraint
	for _, c := range constraints {
		// Field/Index NotEquals constraints are asymmetric: they narrow the target,
		// not the value. If we're filtering for the value side only, drop them.
		if shouldDropAsymmetricNotEquals(c, target) {
			continue
		}
		if constraintAnyPathMatches(c, func(p constraint.Path) bool {
			if PathRelated(target, p) {
				return true
			}
			if equivalentPaths.has(p) {
				return true
			}
			return relatedValuePaths.has(p)
		}) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// CollectEquivalentPaths finds all paths equivalent to target via equality constraints.
//
// Equivalence is transitive: if x == y and y == z, then x, y, and z are all
// equivalent. The function uses a fixed-point algorithm to compute the full
// equivalence closure.
//
// Supported constraint types for equivalence:
//   - EqPath: x == y means x and y are equivalent
//   - FieldEqualsPath: x.field == y means x.field and y are equivalent
//   - IndexEqualsPath: x[i] == y means x[i] and y are equivalent
//
// Returns a set of PathKeys that are transitively equivalent to target.
func CollectEquivalentPaths(constraints []constraint.Constraint, target constraint.Path) map[constraint.PathKey]bool {
	pairs, _ := collectPathFilterFacts(constraints, constraint.Path{})
	set := collectEquivalentPathsFromPairs(pairs, target)
	result := make(map[constraint.PathKey]bool)
	set.visit(func(path constraint.Path) {
		result[path.Key()] = true
	})
	return result
}

func collectPathFilterFacts(constraints []constraint.Constraint, target constraint.Path) ([]eqPair, *pathSet) {
	var pairs []eqPair
	relatedValuePaths := newPathSet()

	for _, c := range constraints {
		switch v := c.(type) {
		case constraint.EqPath:
			pairs = append(pairs, eqPair{
				left:  v.Left,
				right: v.Right,
			})
		case constraint.FieldEqualsPath:
			fieldPath := v.Target.Append(constraint.Segment{Kind: constraint.SegmentField, Name: v.Field})
			if !fieldPath.IsEmpty() {
				pairs = append(pairs, eqPair{
					left:  fieldPath,
					right: v.Value,
				})
			}
			if !target.IsEmpty() && PathRelated(target, v.Target) {
				relatedValuePaths.add(v.Value)
			}
		case constraint.IndexEqualsPath:
			pairs = append(pairs, eqPair{
				left:  v.Target,
				right: v.Value,
			})
			if !target.IsEmpty() && PathRelated(target, v.Target) {
				relatedValuePaths.add(v.Value)
			}
		}
	}

	return pairs, relatedValuePaths
}

func collectEquivalentPathsFromPairs(pairs []eqPair, target constraint.Path) *pathSet {
	result := newPathSet()
	result.add(target)
	if len(pairs) == 0 {
		return result
	}

	changed := true
	for changed {
		changed = false
		for _, pair := range pairs {
			leftIn := result.has(pair.left)
			rightIn := result.has(pair.right)
			if leftIn && !rightIn {
				changed = result.add(pair.right) || changed
			} else if rightIn && !leftIn {
				changed = result.add(pair.left) || changed
			}
		}
	}
	return result
}

func newPathSet() *pathSet {
	return &pathSet{paths: make(map[uint64][]constraint.Path)}
}

func (s *pathSet) add(path constraint.Path) bool {
	if s == nil || path.IsEmpty() {
		return false
	}
	hash := path.Hash()
	bucket := s.paths[hash]
	for _, existing := range bucket {
		if existing.Equal(path) {
			return false
		}
	}
	s.paths[hash] = append(bucket, path)
	return true
}

func (s *pathSet) has(path constraint.Path) bool {
	if s == nil || path.IsEmpty() {
		return false
	}
	for _, existing := range s.paths[path.Hash()] {
		if existing.Equal(path) {
			return true
		}
	}
	return false
}

func (s *pathSet) len() int {
	if s == nil {
		return 0
	}
	total := 0
	for _, bucket := range s.paths {
		total += len(bucket)
	}
	return total
}

func (s *pathSet) visit(fn func(constraint.Path)) {
	if s == nil {
		return
	}
	for _, bucket := range s.paths {
		for _, path := range bucket {
			fn(path)
		}
	}
}

func shouldDropAsymmetricNotEquals(c constraint.Constraint, target constraint.Path) bool {
	return constraint.VisitConstraint(c, constraint.ConstraintVisitor[bool]{
		FieldNotEqualsPath: func(v constraint.FieldNotEqualsPath) bool {
			valueRelated := PathRelated(target, v.Value)
			targetRelated := PathRelated(target, v.Target)
			return valueRelated && !targetRelated
		},
		IndexNotEqualsPath: func(v constraint.IndexNotEqualsPath) bool {
			valueRelated := PathRelated(target, v.Value)
			targetRelated := PathRelated(target, v.Target)
			return valueRelated && !targetRelated
		},
		Default: func(constraint.Constraint) bool {
			return false
		},
	})
}

func constraintAnyPathMatches(c constraint.Constraint, match func(constraint.Path) bool) bool {
	return constraint.VisitConstraint(c, constraint.ConstraintVisitor[bool]{
		Truthy: func(v constraint.Truthy) bool {
			if match(v.Path) {
				return true
			}
			return matchParentFieldPath(v.Path, match)
		},
		Falsy: func(v constraint.Falsy) bool {
			if match(v.Path) {
				return true
			}
			return matchParentFieldPath(v.Path, match)
		},
		IsNil: func(v constraint.IsNil) bool {
			return match(v.Path)
		},
		NotNil: func(v constraint.NotNil) bool {
			return match(v.Path)
		},
		HasType: func(v constraint.HasType) bool {
			return match(v.Path)
		},
		NotHasType: func(v constraint.NotHasType) bool {
			return match(v.Path)
		},
		HasField: func(v constraint.HasField) bool {
			return match(v.Path)
		},
		FieldEquals: func(v constraint.FieldEquals) bool {
			if match(v.Target) {
				return true
			}
			return matchParentFieldPath(v.Target, match)
		},
		FieldNotEquals: func(v constraint.FieldNotEquals) bool {
			if match(v.Target) {
				return true
			}
			return matchParentFieldPath(v.Target, match)
		},
		IndexEquals: func(v constraint.IndexEquals) bool {
			return match(v.Target)
		},
		IndexNotEquals: func(v constraint.IndexNotEquals) bool {
			return match(v.Target)
		},
		EqPath: func(v constraint.EqPath) bool {
			return match(v.Left) || match(v.Right)
		},
		NotEqPath: func(v constraint.NotEqPath) bool {
			return match(v.Left) || match(v.Right)
		},
		FieldEqualsPath: func(v constraint.FieldEqualsPath) bool {
			if match(v.Target) || match(v.Value) {
				return true
			}
			return matchParentFieldPath(v.Target, match)
		},
		FieldNotEqualsPath: func(v constraint.FieldNotEqualsPath) bool {
			if match(v.Target) || match(v.Value) {
				return true
			}
			return matchParentFieldPath(v.Target, match)
		},
		IndexEqualsPath: func(v constraint.IndexEqualsPath) bool {
			return match(v.Target) || match(v.Value)
		},
		IndexNotEqualsPath: func(v constraint.IndexNotEqualsPath) bool {
			return match(v.Target) || match(v.Value)
		},
		KeyOf: func(v constraint.KeyOf) bool {
			return match(v.Table) || match(v.Key)
		},
		Default: func(constraint.Constraint) bool {
			return false
		},
	})
}

func matchParentFieldPath(path constraint.Path, match func(constraint.Path) bool) bool {
	if len(path.Segments) == 0 {
		return false
	}
	if path.Segments[len(path.Segments)-1].Kind != constraint.SegmentField {
		return false
	}
	parent := constraint.Path{Root: path.Root, Symbol: path.Symbol}
	if len(path.Segments) > 1 {
		// Paths() semantics use a parent path without version and avoid mutating the source path.
		parent.Segments = path.Segments[:len(path.Segments)-1]
	}
	return match(parent)
}

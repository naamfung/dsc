// Package numeric provides abstract interpretation for numeric constraints.
//
// This package implements a theory solver for integer arithmetic, tracking:
//   - Interval bounds: lower and upper limits for variables
//   - Modular residues: congruence relations (x % m == r)
//   - Difference constraints: relationships between pairs (x - y <= c)
//   - Symbolic length bounds: array length references
//
// The solver uses Bellman-Ford to detect unsatisfiable constraint sets via
// negative cycle detection in the difference constraint graph.
package numeric

import (
	"math"
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
)

// relationKey identifies a difference constraint between two PathKeys.
//
// A difference constraint has the form X - Y <= C, meaning "X is at most C
// greater than Y". The key identifies the pair of variables being constrained.
type relationKey struct {
	X constraint.PathKey
	Y constraint.PathKey
}

type lenRefBound struct {
	Array  constraint.PathKey
	Offset int64
}

// State represents a compact abstract state for numeric constraints.
//
// Uses interval bounds and modular residues instead of storing raw constraints.
// All maps use constraint.PathKey for versioned variable identity.
//
// The state supports lattice operations: Join computes the intersection of
// facts (for control flow merge), and constraint application refines the state.
type State struct {
	// bounds maps PathKey to its known interval [lower, upper].
	bounds map[constraint.PathKey]Interval

	// modular maps PathKey to modular residue (x % m == r).
	modular map[constraint.PathKey]ModResidue

	// relations stores difference constraints (x - y <= c).
	relations map[relationKey]int64

	// lenRefs maps variable PathKey to array PathKey with offset.
	// Entry x -> {arr, off} means x <= len(arr) + off.
	lenRefs map[constraint.PathKey]lenRefBound

	// unsat is true if the state is unsatisfiable.
	unsat bool
}

// Interval represents a closed integer interval [lower, upper].
//
// Special values:
//   - Lower = MinInt64: unbounded below (negative infinity)
//   - Upper = MaxInt64: unbounded above (positive infinity)
//
// An interval with Lower > Upper represents an empty (unsatisfiable) set.
type Interval struct {
	Lower int64
	Upper int64
}

// ModResidue represents a modular arithmetic constraint: x % modulus == residue.
//
// For example, ModResidue{Modulus: 2, Residue: 0} represents even numbers.
type ModResidue struct {
	Modulus int64
	Residue int64
}

// unbounded interval constants.
var (
	unboundedInterval = Interval{Lower: math.MinInt64, Upper: math.MaxInt64}
)

// NewState creates an empty (top) numeric state.
func NewState() *State {
	return &State{
		bounds:    make(map[constraint.PathKey]Interval),
		modular:   make(map[constraint.PathKey]ModResidue),
		relations: make(map[relationKey]int64),
		lenRefs:   make(map[constraint.PathKey]lenRefBound),
	}
}

// Bottom returns the unsatisfiable state (bottom of the lattice).
//
// A Bottom state represents a contradiction: the constraints are mutually
// unsatisfiable. Any path condition that reaches Bottom is unreachable.
func Bottom() *State {
	return &State{unsat: true}
}

// IsUnsat returns true if the state represents a contradiction.
//
// An unsatisfiable state indicates that the combination of numeric constraints
// has no solution. This can happen when bounds conflict (x > 5 AND x < 3) or
// when difference constraints form a negative cycle.
func (s *State) IsUnsat() bool {
	return s != nil && s.unsat
}

// Clone creates a deep copy of the state for speculative evaluation.
//
// All internal maps are copied so modifications to the clone don't affect
// the original. Used during disjunction evaluation where each branch needs
// its own independent state.
func (s *State) Clone() *State {
	if s == nil {
		return nil
	}

	if s.unsat {
		return Bottom()
	}

	c := &State{
		bounds:    make(map[constraint.PathKey]Interval, len(s.bounds)),
		modular:   make(map[constraint.PathKey]ModResidue, len(s.modular)),
		relations: make(map[relationKey]int64, len(s.relations)),
		lenRefs:   make(map[constraint.PathKey]lenRefBound, len(s.lenRefs)),
	}
	for _, k := range constraint.SortedPathKeys(s.bounds) {
		c.bounds[k] = s.bounds[k]
	}

	for _, k := range constraint.SortedPathKeys(s.modular) {
		c.modular[k] = s.modular[k]
	}

	for _, k := range sortedRelationKeys(s.relations) {
		c.relations[k] = s.relations[k]
	}

	for _, k := range constraint.SortedPathKeys(s.lenRefs) {
		c.lenRefs[k] = s.lenRefs[k]
	}

	return c
}

// Join computes the intersection of two states for control flow merge.
//
// At join points (phi nodes), we keep only facts that hold on all incoming paths.
// This is the meet operation in the abstract interpretation lattice:
//
//   - Bounds: Keep variables present in both, take interval intersection.
//     If Lower > Upper after intersection, result is Bottom (unsatisfiable).
//
//   - Modular: Keep only identical modular constraints from both states.
//     Different modular constraints on the same variable are incompatible.
//
//   - Relations: Keep constraints present in both, take maximum (loosest) bound.
//     max(a.relations[k], b.relations[k]) represents the weakest constraint
//     that holds on both paths.
//
//   - LenRefs: Keep only identical length references from both states.
//
// If both states are nil or Bottom, returns the non-Bottom one.
// If the result is Top (empty maps), returns nil to save memory.
func Join(a, b *State) *State {
	if a == nil && b == nil {
		return nil
	}

	if a == nil || a.unsat {
		return b.Clone()
	}

	if b == nil || b.unsat {
		return a.Clone()
	}

	result := NewState()

	// Bounds: keep only variables in both, intersect intervals.
	for _, v := range constraint.SortedPathKeys(a.bounds) {
		ai := a.bounds[v]
		if bi, ok := b.bounds[v]; ok {
			merged := intersectIntervals(ai, bi)
			if merged.Lower > merged.Upper {
				return Bottom()
			}

			if merged != unboundedInterval {
				result.bounds[v] = merged
			}
		}
	}

	// Modular: keep only if identical in both.
	for _, v := range constraint.SortedPathKeys(a.modular) {
		am := a.modular[v]
		if bm, ok := b.modular[v]; ok {
			if am.Modulus == bm.Modulus && am.Residue == bm.Residue {
				result.modular[v] = am
			}
		}
	}

	// Relations: keep only if present in both, take maximum (loosest bound).
	for _, k := range sortedRelationKeys(a.relations) {
		av := a.relations[k]
		if bv, ok := b.relations[k]; ok {
			result.relations[k] = maxInt64(av, bv)
		}
	}

	// LenRefs: keep only if identical in both.
	for _, v := range constraint.SortedPathKeys(a.lenRefs) {
		ref := a.lenRefs[v]
		if bref, ok := b.lenRefs[v]; ok && ref == bref {
			result.lenRefs[v] = ref
		}
	}

	if result.isTop() {
		return nil
	}

	return result
}

// intersectIntervals computes the intersection of two intervals.
//
// The intersection is the range of values that satisfy both intervals:
// Lower = max(a.Lower, b.Lower), Upper = min(a.Upper, b.Upper).
//
// If the result has Lower > Upper, the intersection is empty, indicating
// unsatisfiability (no value can satisfy both constraints).
func intersectIntervals(a, b Interval) Interval {
	return Interval{
		Lower: maxInt64(a.Lower, b.Lower),
		Upper: minInt64(a.Upper, b.Upper),
	}
}

// ApplyConstraintWithResolver refines the state with a numeric constraint.
//
// Uses the provided resolver to convert constraint paths to versioned PathKeys.
// The resolver maps paths to their SSA versions at the current CFG point,
// ensuring constraints bind to the correct variable incarnation.
// If resolution fails (returns empty key), the constraint is silently skipped.
func (s *State) ApplyConstraintWithResolver(c constraint.NumericConstraint, resolve constraint.PathResolver) {
	if s == nil || s.unsat || resolve == nil {
		return
	}

	constraint.VisitNumericConstraint(c, constraint.NumericConstraintVisitor[struct{}]{
		Le: func(nc constraint.Le) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyLeWithConst(xKey, yKey, nc.C)
			return struct{}{}
		},
		Lt: func(nc constraint.Lt) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyLt(xKey, yKey)
			return struct{}{}
		},
		Ge: func(nc constraint.Ge) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyGe(xKey, yKey)
			return struct{}{}
		},
		Gt: func(nc constraint.Gt) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyGt(xKey, yKey)
			return struct{}{}
		},
		Eq: func(nc constraint.Eq) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyEq(xKey, yKey)
			return struct{}{}
		},
		EqConst: func(nc constraint.EqConst) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyEqConst(xKey, nc.C)
			return struct{}{}
		},
		LeConst: func(nc constraint.LeConst) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyLeConst(xKey, nc.C)
			return struct{}{}
		},
		GeConst: func(nc constraint.GeConst) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyGeConst(xKey, nc.C)
			return struct{}{}
		},
		ModEq: func(nc constraint.ModEq) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyModEq(xKey, nc.M, nc.R)
			return struct{}{}
		},
		LeLenOf: func(nc constraint.LeLenOf) struct{} {
			xKey := resolve(nc.X)
			arrKey := resolve(nc.Array)
			if xKey == "" || arrKey == "" {
				return struct{}{}
			}
			s.applyLeLenOf(xKey, arrKey, nc.Offset)
			return struct{}{}
		},
	})
}

func (s *State) applyLe(x, y constraint.PathKey) {
	s.applyLeWithConst(x, y, 0)
}

func (s *State) applyLeWithConst(x, y constraint.PathKey, c int64) {
	// x - y <= c
	key := relationKey{X: x, Y: y}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, c)
	} else {
		s.relations[key] = c
	}
}

func (s *State) ApplyLt(x, y constraint.PathKey) {
	s.applyLt(x, y)
}

func (s *State) applyLt(x, y constraint.PathKey) {
	// x < y  =>  x - y <= -1
	key := relationKey{X: x, Y: y}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, -1)
	} else {
		s.relations[key] = -1
	}
}

func (s *State) ApplyLe(x, y constraint.PathKey) {
	s.applyLe(x, y)
}

func (s *State) ApplyGe(x, y constraint.PathKey) {
	s.applyGe(x, y)
}

func (s *State) applyGe(x, y constraint.PathKey) {
	// x >= y  =>  y - x <= 0
	key := relationKey{X: y, Y: x}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, 0)
	} else {
		s.relations[key] = 0
	}
}

func (s *State) ApplyGt(x, y constraint.PathKey) {
	s.applyGt(x, y)
}

func (s *State) applyGt(x, y constraint.PathKey) {
	// x > y  =>  y - x <= -1
	key := relationKey{X: y, Y: x}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, -1)
	} else {
		s.relations[key] = -1
	}
}

func (s *State) ApplyEq(x, y constraint.PathKey) {
	s.applyEq(x, y)
}

func (s *State) applyEq(x, y constraint.PathKey) {
	// x == y  =>  x - y <= 0 and y - x <= 0
	s.applyLe(x, y)
	s.applyLe(y, x)
}

func (s *State) ApplyEqConst(v constraint.PathKey, c int64) {
	s.applyEqConst(v, c)
}

func (s *State) applyEqConst(v constraint.PathKey, c int64) {
	s.bounds[v] = Interval{Lower: c, Upper: c}
}

func (s *State) ApplyLeConst(v constraint.PathKey, c int64) {
	s.applyLeConst(v, c)
}

func (s *State) applyLeConst(v constraint.PathKey, c int64) {
	if b, ok := s.bounds[v]; ok {
		b.Upper = minInt64(b.Upper, c)
		if b.Lower > b.Upper {
			s.unsat = true
			return
		}
		s.bounds[v] = b
	} else {
		s.bounds[v] = Interval{Lower: math.MinInt64, Upper: c}
	}
}

func (s *State) ApplyGeConst(v constraint.PathKey, c int64) {
	s.applyGeConst(v, c)
}

func (s *State) applyGeConst(v constraint.PathKey, c int64) {
	if b, ok := s.bounds[v]; ok {
		b.Lower = maxInt64(b.Lower, c)
		if b.Lower > b.Upper {
			s.unsat = true
			return
		}
		s.bounds[v] = b
	} else {
		s.bounds[v] = Interval{Lower: c, Upper: math.MaxInt64}
	}
}

func (s *State) ApplyModEq(v constraint.PathKey, m, r int64) {
	s.applyModEq(v, m, r)
}

func (s *State) applyModEq(v constraint.PathKey, m, r int64) {
	if existing, ok := s.modular[v]; ok {
		if existing.Modulus != m || existing.Residue != r {
			s.unsat = true
		}
	} else {
		s.modular[v] = ModResidue{Modulus: m, Residue: r}
	}
}

func (s *State) ApplyLeLenOf(v, arr constraint.PathKey) {
	s.applyLeLenOf(v, arr, 0)
}

func (s *State) ApplyLeLenOfWithOffset(v, arr constraint.PathKey, offset int64) {
	s.applyLeLenOf(v, arr, offset)
}

func (s *State) applyLeLenOf(v, arr constraint.PathKey, offset int64) {
	s.lenRefs[v] = lenRefBound{Array: arr, Offset: offset}
}

// BoundsFor returns the interval bounds for a PathKey.
//
// Returns (lower, upper, true) if the key has known bounds, or (0, 0, false)
// if the key has no constraints. This is the direct lookup without transitive
// inference; use BoundsForWithTheory for tighter bounds via transitive closure.
func (s *State) BoundsFor(key constraint.PathKey) (lower, upper int64, ok bool) {
	if s == nil || s.bounds == nil {
		return 0, 0, false
	}
	interval, found := s.bounds[key]
	if !found {
		return 0, 0, false
	}
	return interval.Lower, interval.Upper, true
}

// LenRefFor returns the array key if variable has a symbolic length bound.
//
// A length reference means "key <= #arrKey" (variable is bounded by array length).
// This is used to prove array access safety when the index is bounded by the array.
func (s *State) LenRefFor(key constraint.PathKey) (arrKey constraint.PathKey, ok bool) {
	if s == nil || s.lenRefs == nil {
		return "", false
	}
	ref, ok := s.lenRefs[key]
	if !ok {
		return "", false
	}
	return ref.Array, true
}

// LenRefWithOffsetFor returns array key and offset for symbolic length bound.
func (s *State) LenRefWithOffsetFor(key constraint.PathKey) (arrKey constraint.PathKey, offset int64, ok bool) {
	if s == nil || s.lenRefs == nil {
		return "", 0, false
	}
	ref, ok := s.lenRefs[key]
	if !ok {
		return "", 0, false
	}
	return ref.Array, ref.Offset, true
}

// CheckSatisfiability verifies the state has no contradictions.
//
// Checks interval bounds for consistency (lower <= upper) and uses
// Bellman-Ford to detect negative cycles in the difference constraint graph.
// A negative cycle indicates an unsatisfiable constraint set.
func (s *State) CheckSatisfiability() bool {
	if s == nil || s.unsat {
		return false
	}

	// Check bounds consistency.
	for _, key := range constraint.SortedPathKeys(s.bounds) {
		b := s.bounds[key]
		if b.Lower > b.Upper {
			s.unsat = true
			return false
		}
	}

	// Check relation consistency using Bellman-Ford on difference graph.
	if len(s.relations) > 0 {
		if !s.checkDifferenceConstraints() {
			s.unsat = true
			return false
		}
	}

	return true
}

// checkDifferenceConstraints uses Bellman-Ford to detect negative cycles.
//
// Difference constraints (x - y <= c) form a weighted directed graph.
// A negative cycle implies a contradiction: some variable must be less
// than itself. Bellman-Ford detects this by running |V| relaxation rounds
// and checking if any edge can still be relaxed.
func (s *State) checkDifferenceConstraints() bool {
	if len(s.relations) == 0 {
		return true
	}

	// Collect variables.
	vars := make(map[constraint.PathKey]bool)

	for _, k := range sortedRelationKeys(s.relations) {
		vars[k.X] = true
		vars[k.Y] = true
	}

	// Initialize distances from virtual source.
	dist := make(map[constraint.PathKey]int64)
	for _, v := range constraint.SortedPathKeys(vars) {
		dist[v] = 0
	}

	// Relax edges |V| times.
	n := len(vars)
	for i := 0; i < n; i++ {
		changed := false

		for _, k := range sortedRelationKeys(s.relations) {
			w := s.relations[k]
			// x - y <= w  =>  dist[x] <= dist[y] + w
			if dist[k.Y]+w < dist[k.X] {
				dist[k.X] = dist[k.Y] + w
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	// Check for negative cycle (one more iteration).
	for _, k := range sortedRelationKeys(s.relations) {
		w := s.relations[k]
		if dist[k.Y]+w < dist[k.X] {
			return false
		}
	}

	return true
}

// Equals checks if two states are semantically equal.
//
// Two states are equal if they have the same unsat flag and identical maps
// for bounds, modular constraints, relations, and length references.
// nil and Top (empty maps) states are considered equal.
func (s *State) Equals(other *State) bool {
	if s == nil && other == nil {
		return true
	}

	if s == nil {
		return other.isTop()
	}

	if other == nil {
		return s.isTop()
	}

	if s.unsat != other.unsat {
		return false
	}

	if s.unsat {
		return true
	}

	if len(s.bounds) != len(other.bounds) {
		return false
	}

	if len(s.modular) != len(other.modular) {
		return false
	}

	if len(s.relations) != len(other.relations) {
		return false
	}

	if len(s.lenRefs) != len(other.lenRefs) {
		return false
	}

	for _, k := range constraint.SortedPathKeys(s.bounds) {
		v := s.bounds[k]
		if ov, ok := other.bounds[k]; !ok || v != ov {
			return false
		}
	}

	for _, k := range constraint.SortedPathKeys(s.modular) {
		v := s.modular[k]
		if ov, ok := other.modular[k]; !ok || v != ov {
			return false
		}
	}

	for _, k := range sortedRelationKeys(s.relations) {
		v := s.relations[k]
		if ov, ok := other.relations[k]; !ok || v != ov {
			return false
		}
	}

	for _, k := range constraint.SortedPathKeys(s.lenRefs) {
		v := s.lenRefs[k]
		if ov, ok := other.lenRefs[k]; !ok || v != ov {
			return false
		}
	}

	return true
}

// IsTop returns true if the state is Top (no constraints, all values possible).
func (s *State) IsTop() bool {
	return s.isTop()
}

// isTop returns true if the state represents no constraints (top of lattice).
//
// A Top state is nil or has empty maps for all constraint categories.
// Top states don't restrict values at all and can be omitted from storage.
func (s *State) isTop() bool {
	if s == nil {
		return true
	}

	if s.unsat {
		return false
	}

	return len(s.bounds) == 0 && len(s.modular) == 0 && len(s.relations) == 0 && len(s.lenRefs) == 0
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}

	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}

	return b
}

// Bounds returns the raw bounds map for theory solver initialization.
//
// This provides direct access to the bounds map for building the theory
// solver's difference graph. The returned map should not be modified.
func (s *State) Bounds() map[constraint.PathKey]Interval {
	if s == nil {
		return nil
	}
	return s.bounds
}

// Relations returns the raw relations map for theory solver initialization.
//
// The map is transformed from the internal flat format (relationKey -> int64)
// to a nested format (X -> Y -> int64) for easier iteration. Each entry
// represents a difference constraint X - Y <= C.
func (s *State) Relations() map[constraint.PathKey]map[constraint.PathKey]int64 {
	if s == nil || len(s.relations) == 0 {
		return nil
	}
	result := make(map[constraint.PathKey]map[constraint.PathKey]int64)
	for _, rel := range sortedRelationKeys(s.relations) {
		if result[rel.X] == nil {
			result[rel.X] = make(map[constraint.PathKey]int64)
		}
		result[rel.X][rel.Y] = s.relations[rel]
	}
	return result
}

// Modular returns the raw modular constraints map.
//
// Each entry maps a path key to its modular residue constraint.
// The returned map should not be modified.
func (s *State) Modular() map[constraint.PathKey]ModResidue {
	if s == nil {
		return nil
	}
	return s.modular
}

// SetUnsat marks the state as unsatisfiable (contradiction detected).
//
// Once unsat, the state remains unsat regardless of further constraint
// applications. This is used when external analysis (theory solver) detects
// unsatisfiability that wasn't caught by local constraint application.
func (s *State) SetUnsat() {
	if s != nil {
		s.unsat = true
	}
}

// Rekey creates a new state with keys remapped according to the provided mapping.
//
// At phi nodes, SSA versions from different predecessors merge into new versions.
// This method renames constraints to use the merged version keys, enabling
// proper constraint propagation across phi boundaries.
func (s *State) Rekey(remap map[constraint.PathKey]constraint.PathKey) *State {
	if s == nil || len(remap) == 0 {
		return s
	}

	if s.unsat {
		return Bottom()
	}

	result := &State{
		bounds:    make(map[constraint.PathKey]Interval, len(s.bounds)),
		modular:   make(map[constraint.PathKey]ModResidue, len(s.modular)),
		relations: make(map[relationKey]int64, len(s.relations)),
		lenRefs:   make(map[constraint.PathKey]lenRefBound, len(s.lenRefs)),
	}

	// Remap bounds
	for _, k := range constraint.SortedPathKeys(s.bounds) {
		v := s.bounds[k]
		newKey := k
		if mapped, ok := remap[k]; ok {
			newKey = mapped
		}
		result.bounds[newKey] = v
	}

	// Remap modular constraints
	for _, k := range constraint.SortedPathKeys(s.modular) {
		v := s.modular[k]
		newKey := k
		if mapped, ok := remap[k]; ok {
			newKey = mapped
		}
		result.modular[newKey] = v
	}

	// Remap relations (both X and Y)
	for _, rel := range sortedRelationKeys(s.relations) {
		c := s.relations[rel]
		newX := rel.X
		newY := rel.Y
		if mapped, ok := remap[rel.X]; ok {
			newX = mapped
		}
		if mapped, ok := remap[rel.Y]; ok {
			newY = mapped
		}
		result.relations[relationKey{X: newX, Y: newY}] = c
	}

	// Remap length references (both variable and array keys)
	for _, k := range constraint.SortedPathKeys(s.lenRefs) {
		ref := s.lenRefs[k]
		newK := k
		newArr := ref.Array
		if mapped, ok := remap[k]; ok {
			newK = mapped
		}
		if mapped, ok := remap[ref.Array]; ok {
			newArr = mapped
		}
		ref.Array = newArr
		result.lenRefs[newK] = ref
	}

	return result
}

func sortedRelationKeys(m map[relationKey]int64) []relationKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]relationKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].X != keys[j].X {
			return keys[i].X < keys[j].X
		}
		return keys[i].Y < keys[j].Y
	})
	return keys
}

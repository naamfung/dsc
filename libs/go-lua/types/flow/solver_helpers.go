package flow

import (
	"slices"
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

// symbolTypeSource configures how to initialize symbol types in the solution's values map.
//
// This struct encapsulates the common pattern of iterating over a symbol-to-type map,
// resolving canonical keys for each symbol, and storing the types in the solution.
// Different type sources (DeclaredTypes, SiblingTypes, LiteralTypes) have slightly
// different behaviors controlled by the configuration fields.
//
// Fields:
//   - types: The source map from SymbolID to Type to initialize from.
//   - tryDeclPoint: If true and the entry point key is empty, try the symbol's
//     declaration point as a fallback. Used for local variables that aren't
//     visible at function entry.
//   - skipIfExists: If true, don't overwrite existing entries in the values map.
//     Used to establish priority ordering (DeclaredTypes > SiblingTypes > LiteralTypes).
type symbolTypeSource struct {
	types        map[cfg.SymbolID]typ.Type
	tryDeclPoint bool
	skipIfExists bool
}

// initSymbolTypes initializes the solution's values map from a symbol-to-type mapping.
//
// This method extracts the common initialization pattern used for DeclaredTypes,
// SiblingTypes, and LiteralTypes. For each symbol in the source map:
//
//  1. Resolves the canonical key at the function entry point
//  2. If tryDeclPoint is set and entry key is empty, tries the declaration point
//  3. If skipIfExists is set, skips symbols that already have values
//  4. Stores the type in the values map keyed by the canonical key
//
// Symbols are processed in sorted order to ensure deterministic initialization,
// which is important for reproducible analysis results and debugging.
//
// This method is safe to call with nil or empty types maps.
func (s *Solution) initSymbolTypes(src symbolTypeSource) {
	if len(src.types) == 0 {
		return
	}

	g := s.inputs.Graph
	if g == nil {
		return
	}

	entry := g.Entry()

	// Sort symbols for deterministic processing order
	syms := make([]cfg.SymbolID, 0, len(src.types))
	for sym := range src.types {
		syms = append(syms, sym)
	}
	slices.Sort(syms)

	for _, sym := range syms {
		t := src.types[sym]
		if t == nil {
			continue
		}

		// Try to resolve key at entry point first
		path := constraint.Path{Symbol: sym}
		key := s.pkResolver.KeyAt(entry, path)

		// Fall back to declaration point for local variables
		if key == "" && src.tryDeclPoint {
			if declPoint, ok := g.DeclarationPoint(sym); ok && declPoint != 0 {
				key = s.pkResolver.KeyAt(declPoint, path)
			}
		}

		if key == "" {
			continue
		}

		// Respect priority ordering if skipIfExists is set
		keyStr := string(key)
		if src.skipIfExists {
			if _, exists := s.values[keyStr]; exists {
				continue
			}
		}

		s.setValue(keyStr, t)
	}
}

func (s *Solution) setValue(key string, t typ.Type) {
	if s == nil || s.values == nil || key == "" {
		return
	}
	s.values[key] = t
	if s.fieldOverlayCache == nil {
		return
	}
	_, _, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(key))
	if !ok || suffix == "" {
		return
	}
	delete(s.fieldOverlayCache, key[:len(key)-len(suffix)])
}

// dependencyMap tracks which CFG points depend on a given canonical key.
//
// During worklist iteration, when a key's type value changes, all dependent
// points must be re-processed. This map enables efficient lookup of those
// dependencies without re-scanning the entire input on each iteration.
//
// The map is built once before the worklist loop and consulted whenever
// a key's value changes. Keys are canonical path keys (e.g., "sym1@1.field"),
// and values are slices of CFG points that read from that key.
type dependencyMap map[string][]cfg.Point

func symbolDependencyKey(sym cfg.SymbolID) string {
	return "$sym:" + strconv.FormatUint(uint64(sym), 10)
}

// register adds a dependency from a canonical key to a CFG point.
//
// If the key is empty, the registration is silently ignored. This allows
// callers to pass potentially-empty keys without checking first.
//
// The same point may be registered multiple times for different keys,
// and the same (key, point) pair may be registered multiple times
// (duplicates are handled during worklist iteration via the inQueue map).
func (dm dependencyMap) register(key constraint.PathKey, point cfg.Point) {
	if key != "" {
		dm[string(key)] = append(dm[string(key)], point)
	}
}

func (dm dependencyMap) registerSymbol(sym cfg.SymbolID, point cfg.Point) {
	if sym == 0 {
		return
	}
	dm[symbolDependencyKey(sym)] = append(dm[symbolDependencyKey(sym)], point)
}

// buildPhiDependencies constructs a map from version keys to phi points.
//
// Phi nodes merge values from multiple control flow paths. When any operand's
// value changes, the phi must be re-evaluated to compute the new merged type.
// This method builds the reverse mapping: for each operand version key, which
// phi points depend on it.
//
// Example: If phi at point 5 merges x@1 and x@2, this returns:
//
//	{"sym(x)@1": [5], "sym(x)@2": [5]}
//
// Returns nil if the graph is nil.
func (s *Solution) buildPhiDependencies() dependencyMap {
	g := s.inputs.Graph
	if g == nil {
		return nil
	}

	deps := make(dependencyMap)
	for _, phi := range g.PhiNodes() {
		for _, op := range phi.Operands {
			opKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
			deps.register(opKey, phi.Point)
		}
	}
	return deps
}

// buildAssignmentDependencies constructs a map from source keys to assignment points.
//
// When the type of a source path changes, assignments that read from that source
// must be re-processed. This includes:
//   - Regular assignments with a source path (e.g., y = x)
//   - Iterator source paths (e.g., for k, v in pairs(t))
//   - Table mutator value paths (e.g., table.insert(t, value))
//   - Map element sources (e.g., v = t[k] with dynamic key)
//   - Container element sources (e.g., v = ch:receive())
//
// This enables incremental re-evaluation during worklist iteration.
func (s *Solution) buildAssignmentDependencies() dependencyMap {
	deps := make(dependencyMap)

	// Regular assignments
	for _, assign := range s.inputs.Assignments {
		if assign.SourcePath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.SourcePath)
			deps.register(srcKey, assign.Point)
		}
		if assign.IterSource != nil && assign.IterSource.Path.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.IterSource.Path)
			deps.register(srcKey, assign.Point)
			deps.registerSymbol(assign.IterSource.Path.Symbol, assign.Point)
		}
		if assign.MapElementSource != nil && assign.MapElementSource.MapPath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.MapElementSource.MapPath)
			deps.register(srcKey, assign.Point)
			deps.registerSymbol(assign.MapElementSource.MapPath.Symbol, assign.Point)
		}
		if assign.ContainerElementSource != nil && assign.ContainerElementSource.ContainerPath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(assign.Point, assign.ContainerElementSource.ContainerPath)
			deps.register(srcKey, assign.Point)
			deps.registerSymbol(assign.ContainerElementSource.ContainerPath.Symbol, assign.Point)
		}
	}

	// Table mutator value paths
	for _, tm := range s.inputs.TableMutatorAssignments {
		if tm.ValuePath.Symbol != 0 {
			srcKey := s.pkResolver.KeyAt(tm.Point, tm.ValuePath)
			deps.register(srcKey, tm.Point)
		}
	}

	return deps
}

// buildEdgeConditionDependencies constructs a map from keys to edge condition source points.
//
// Edge conditions contain type constraints that depend on variable types at the
// source point. When a variable's type changes, edge conditions that reference
// it may produce different narrowing results.
//
// This method extracts all path keys referenced in edge conditions and maps them
// to the source points. A deduplication set prevents the same (key, point) pair
// from being registered multiple times, which would cause redundant work.
func (s *Solution) buildEdgeConditionDependencies() dependencyMap {
	deps := make(dependencyMap)

	// Deduplication set for (key, point) pairs
	type ecDep struct {
		key   string
		point cfg.Point
	}
	seen := make(map[ecDep]bool)

	for _, ec := range s.inputs.EdgeConditions {
		// Iterate through all disjuncts and their constraints
		for i := 0; i < ec.Condition.NumDisjuncts(); i++ {
			for _, c := range ec.Condition.DisjunctConstraints(i) {
				for _, path := range c.Paths() {
					if path.Symbol == 0 {
						continue
					}
					verKey := s.pkResolver.KeyAt(ec.From, path)
					if verKey == "" {
						continue
					}
					dep := ecDep{key: string(verKey), point: ec.From}
					if seen[dep] {
						continue
					}
					seen[dep] = true
					deps.register(verKey, ec.From)
				}
			}
		}
	}

	return deps
}

// addDependentPoints adds CFG points that depend on changed keys to the worklist.
//
// This function is the core of incremental worklist propagation. When a key's
// value changes, all points that depend on that key (via phi nodes, assignments,
// or edge conditions) must be added to the worklist for re-processing.
//
// The inQueue map prevents duplicate additions - if a point is already queued,
// it's skipped. This is critical for termination and efficiency.
//
// Parameters:
//   - deps: The dependency map to consult
//   - changedKeys: Keys whose values changed in the current iteration
//   - worklist: The current worklist to append to
//   - inQueue: Set of points already in the worklist
//
// Returns the updated worklist (may be the same slice if no growth occurred).
func addDependentPoints(deps dependencyMap, changedKeys []string, worklist []cfg.Point, inQueue map[cfg.Point]bool) []cfg.Point {
	if len(changedKeys) == 0 {
		return worklist
	}
	keys := append([]string(nil), changedKeys...)
	sort.Strings(keys)

	pending := make(map[cfg.Point]bool)
	points := make([]cfg.Point, 0)
	for _, key := range keys {
		for _, point := range deps[key] {
			if inQueue[point] || pending[point] {
				continue
			}
			pending[point] = true
			points = append(points, point)
		}
		if sym := pathkey.KeySymbolUnchecked(constraint.PathKey(key)); sym != 0 {
			for _, point := range deps[symbolDependencyKey(sym)] {
				if inQueue[point] || pending[point] {
					continue
				}
				pending[point] = true
				points = append(points, point)
			}
		}
	}
	slices.Sort(points)
	for _, point := range points {
		worklist = append(worklist, point)
		inQueue[point] = true
	}
	return worklist
}

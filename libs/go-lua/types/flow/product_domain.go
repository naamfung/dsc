package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/constraint/theory"
	"github.com/wippyai/go-lua/types/flow/domain"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// ProductDomain combines Type, Numeric, and Shape domains into a single
// abstract domain for constraint solving in flow-sensitive type analysis.
//
// ProductDomain is the central abstraction for narrowing types based on
// control flow constraints. It orchestrates three specialized subdomains,
// each handling different aspects of constraint narrowing:
//
//   - Type domain: Handles HasType, NotHasType, Truthy, and Falsy constraints.
//     Narrows union types by adding or removing type alternatives. For example,
//     a Truthy constraint on `string|nil` narrows to `string`.
//
//   - Numeric domain: Tracks numeric range constraints (Lt, Le, Gt, Ge) and
//     modular arithmetic (ModEq). Maintains interval bounds for number values.
//     For example, `x >= 0` establishes a lower bound of 0 for x.
//
//   - Shape domain: Handles structural constraints on tables including field
//     presence/absence, index constraints, and record shape narrowing. Uses
//     the wrapped Solver for complex shape reasoning.
//
// The E-graph component tracks path equality constraints (EqPath) and propagates
// narrowings across equivalence classes using congruence closure. When two paths
// are proven equal via `x == y`, type narrowings learned about one automatically
// apply to both. This is essential for code patterns like:
//
//	if a == b and type(a) == "string" then
//	    -- both a and b are narrowed to string
//	end
//
// ProductDomain routes constraint atoms to the appropriate subdomain via
// domain.ClassifyAtom, applies conjunctions by collecting atoms and leftovers,
// and handles disjunctions by cloning the domain state, speculatively applying
// each disjunct, and joining the results.
//
// The domain satisfies the abstract domain interface with Clone, Join, and
// IsUnsat operations, enabling the flow analysis to track type information
// along different control flow paths and merge it at join points.
type ProductDomain struct {
	Type    *domain.TypeDomain
	Numeric *numeric.Domain
	Shape   *domain.ShapeDomain
	EGraph  *theory.EGraph
	env     constraint.Env
}

// NewProductDomain creates a new ProductDomain initialized with the given environment.
//
// The environment provides type resolution functions needed by the subdomains:
//
//   - env.PathTypeAt: Returns the base type at a given path key. Used to look up
//     original types before narrowing is applied.
//
//   - env.ResolvePath: Converts a constraint.Path to its canonical PathKey. Used
//     for consistent key resolution across all operations.
//
//   - env.ResolveType: Resolves type keys (like "string", "number") to actual types.
//
// All three subdomains (Type, Numeric, Shape) share the same environment, ensuring
// consistent type resolution. The E-graph is initialized empty and will be
// populated when EqPath constraints are applied.
//
// Example usage:
//
//	env := constraint.Env{
//	    PathTypeAt: func(key constraint.PathKey) typ.Type { return types[key] },
//	    ResolvePath: resolver.KeyAt,
//	}
//	domain := NewProductDomain(env)
//	domain.ApplyCondition(condition) // Apply narrowing constraints
//	narrowedType := domain.TypeAt(key) // Get narrowed type
func NewProductDomain(env constraint.Env) *ProductDomain {
	return &ProductDomain{
		Type:    domain.NewTypeDomain(env),
		Numeric: numeric.NewDomain(env),
		Shape:   domain.NewShapeDomain(env),
		EGraph:  theory.NewEGraph(),
		env:     env,
	}
}

// ApplyAtom applies a constraint atom to the appropriate subdomain(s).
//
// Atoms are the primitive building blocks of constraints. Each atom type is
// routed to the appropriate subdomain based on domain.ClassifyAtom:
//
//   - AtomClassType: Routed to Type domain. Includes HasType, NotHasType,
//     Truthy, and Falsy atoms that narrow union types.
//
//   - AtomClassNumeric: Routed to Numeric domain. Includes Lt, Le, Gt, Ge,
//     and ModEq atoms that establish numeric bounds.
//
//   - AtomClassBoth: Applied to both Type and Numeric domains. Used for atoms
//     that have implications in both domains (e.g., equality with a constant).
//
//   - AtomClassNone: No-op, returns true. Used for atoms that don't affect
//     type narrowing.
//
// Returns false if the atom makes the domain unsatisfiable (proves a contradiction).
// For example, applying HasType("string") to a path known to be `number` returns
// false since a value cannot be both string and number.
//
// For AtomClassBoth atoms, both domains must accept the atom; if either returns
// false, the overall result is false.
func (d *ProductDomain) ApplyAtom(atom constraint.Atom) bool {
	switch domain.ClassifyAtom(atom) {
	case domain.AtomClassType:
		return d.Type.ApplyAtom(atom)
	case domain.AtomClassNumeric:
		return d.Numeric.ApplyAtom(atom)
	case domain.AtomClassBoth:
		return d.Type.ApplyAtom(atom) && d.Numeric.ApplyAtom(atom)
	case domain.AtomClassNone:
		return true
	default:
		return true
	}
}

// ApplyLeftoverConstraint applies a constraint that couldn't be converted to an atom.
//
// Leftover constraints are those that cannot be represented as simple atoms.
// These include complex structural constraints like:
//
//   - Field presence checks: HasField{Path, FieldName}
//   - Index constraints: HasIndex{Path, IndexType}
//   - Metatable constraints: HasMetatable{Path}
//   - Record shape constraints that require structural reasoning
//
// These constraints are routed to the Shape domain, which uses the wrapped
// Solver for more sophisticated shape analysis.
//
// The method first resolves the target path to a canonical key using
// env.ResolvePath. If the constraint has no paths or the path cannot be
// resolved, the constraint is silently accepted (returns true) since we
// cannot reason about it without a target.
//
// Returns false if the constraint proves the domain unsatisfiable.
func (d *ProductDomain) ApplyLeftoverConstraint(c constraint.Constraint) bool {
	path, ok := constraint.FirstPath(c)
	if !ok {
		return true
	}
	if d.env.ResolvePath == nil {
		return true
	}
	target := d.env.ResolvePath(path)
	if target == "" {
		return true
	}
	return d.Shape.ApplyConstraint(c, target)
}

// ApplyConjunction applies all constraints in a conjunction (AND of constraints).
//
// This is the primary method for applying multiple constraints that must all
// hold simultaneously. The method performs several steps in order:
//
//  1. Constraint conversion: Uses constraint.ToAtomsWithResolver to convert
//     high-level constraints into atoms (primitive narrowings) and leftover
//     constraints (complex shapes). The resolver ensures paths are converted
//     to canonical keys.
//
//  2. Congruence closure: Builds an E-graph from EqPath constraints. When
//     `x == y` is asserted, the E-graph records that paths x and y are in
//     the same equivalence class. Later narrowings to x will propagate to y.
//
//  3. Atom application: Applies all atoms to their respective subdomains
//     (Type and/or Numeric based on classification). Returns false immediately
//     if any atom proves unsatisfiability.
//
//  4. Type narrowing propagation: Propagates Type domain narrowings across
//     equivalence classes via propagateTypeNarrowingsCC. If x and y are
//     equivalent and x is narrowed to string, y is also narrowed to string.
//
//  5. Leftover application: Applies leftover constraints to the Shape domain
//     with access to Type domain narrowings for context.
//
//  6. Shape narrowing propagation: Propagates Shape domain narrowings across
//     equivalence classes via propagateShapeNarrowingsCC.
//
// Returns false if any step proves the domain unsatisfiable.
func (d *ProductDomain) ApplyConjunction(constraints []constraint.Constraint) bool {
	// Use canonical key resolver for path→key conversion
	result := constraint.ToAtomsWithResolver(constraints, d.env.ResolvePath)

	// Build congruence closure from EqPath constraints
	d.buildCongruenceClosure(result.Atoms, constraints)

	// Apply atoms (Type + Numeric)
	for _, atom := range result.Atoms {
		if !d.ApplyAtom(atom) {
			return false
		}
	}

	// Propagate Type domain narrowings across equivalence classes
	d.propagateTypeNarrowingsCC()
	if d.Type.IsUnsat() {
		return false
	}

	// Update PathTypeAt to include Type domain narrowings for leftover constraints
	originalPathTypeAt := d.Shape.Solver.Env.PathTypeAt
	narrowedPathTypeAt := func(key constraint.PathKey) typ.Type {
		if narrowed := d.Type.NarrowedTypeAt(key); narrowed != nil {
			return narrowed
		}
		if originalPathTypeAt != nil {
			return originalPathTypeAt(key)
		}
		return nil
	}
	d.Shape.Solver.Env.PathTypeAt = narrowedPathTypeAt
	d.Shape.Env.PathTypeAt = narrowedPathTypeAt

	// Apply leftovers (Shape domain via wrapped Solver)
	for _, c := range result.Leftover {
		if !d.ApplyLeftoverConstraint(c) {
			return false
		}
	}

	// Propagate Shape narrowings across equivalence classes
	d.propagateShapeNarrowingsCC()

	return true
}

// buildCongruenceClosure builds the E-graph from EqPath constraints.
//
// The E-graph implements congruence closure, a fundamental algorithm for
// reasoning about equality. When constraints include EqPath(x, y) assertions,
// this method records that paths x and y refer to the same value.
//
// The algorithm proceeds in two phases:
//
//  1. Registration: All paths referenced by any constraint are registered
//     in the E-graph. This ensures every path has an entry even if not
//     involved in an equality.
//
//  2. Union: For each EqPath constraint, the left and right paths are
//     unified in the E-graph. After union, Find(left) == Find(right).
//
// Paths are converted to canonical keys via env.ResolvePath before
// registration. If ResolvePath is nil, no E-graph is built and path
// equality constraints are ignored.
//
// The resulting E-graph is used by propagateTypeNarrowingsCC and
// propagateShapeNarrowingsCC to share narrowings across equivalent paths.
//
// Example: Given constraints [EqPath(a, b), HasType(a, "string")]:
//  1. Register keys for a and b
//  2. Union a and b (now Find(a) == Find(b))
//  3. When Type domain narrows a to string, propagation narrows b to string
func (d *ProductDomain) buildCongruenceClosure(atoms []constraint.Atom, constraints []constraint.Constraint) {
	if d.env.ResolvePath == nil {
		return
	}
	resolve := d.env.ResolvePath

	for _, c := range constraints {
		constraint.VisitPaths(c, func(path constraint.Path) bool {
			key := resolve(path)
			if key != "" {
				d.EGraph.RegisterKey(key)
			}
			return false
		})
	}

	for _, c := range constraints {
		if eq, ok := c.(constraint.EqPath); ok {
			leftKey := resolve(eq.Left)
			rightKey := resolve(eq.Right)
			if leftKey != "" && rightKey != "" {
				d.EGraph.Union(leftKey, rightKey)
			}
		}
	}
}

// propagateTypeNarrowingsCC propagates Type domain narrowings across equivalence classes.
//
// After applying constraints, some paths may have narrowed types in the Type
// domain. If those paths are equivalent to other paths via EqPath constraints,
// the narrowings must propagate to maintain soundness.
//
// The algorithm:
//
//  1. For each path in the E-graph, look up its narrowed type from the Type domain.
//
//  2. Group types by equivalence class root (via EGraph.Find). All paths in the
//     same class should have compatible types.
//
//  3. Compute the intersection of types within each class. If paths in a class
//     have types string and string|number, the intersection is string.
//
//  4. If intersection is empty (e.g., string vs number), mark domain unsatisfiable.
//     This represents a contradiction: two equal paths cannot have disjoint types.
//
//  5. Apply the class-wide intersection back to all paths in the class.
//
// This enables code like:
//
//	if a == b then
//	    if type(a) == "string" then
//	        -- b is also narrowed to string here
//	    end
//	end
//
// The method modifies d.Type.Narrowed in place and may set d.Type.Unsat = true.
func (d *ProductDomain) propagateTypeNarrowingsCC() {
	allPaths := d.EGraph.AllPaths()
	if len(allPaths) == 0 {
		return
	}

	classTypes := make(map[constraint.PathKey]typ.Type)
	for _, key := range allPaths {
		root := d.EGraph.Find(key)
		t := d.Type.TypeAt(key)
		if t == nil {
			continue
		}

		if existing, ok := classTypes[root]; ok {
			if !narrow.TypesOverlap(existing, t) {
				d.Type.Unsat = true
				return
			}
			intersection := narrow.Intersect(existing, t)
			if intersection == nil || intersection.Kind().IsNever() {
				d.Type.Unsat = true
				return
			}
			classTypes[root] = intersection
		} else {
			classTypes[root] = t
		}
	}

	for _, key := range allPaths {
		root := d.EGraph.Find(key)
		if classType, ok := classTypes[root]; ok {
			base := d.Type.TypeAt(key)
			if base == nil {
				continue
			}
			if !typ.TypeEquals(classType, base) {
				d.Type.Narrowed[key] = classType
			}
		}
	}
}

// propagateShapeNarrowingsCC propagates Shape domain narrowings across equivalence classes.
//
// Similar to propagateTypeNarrowingsCC but for structural (shape) narrowings.
// When constraints narrow the shape of a table (e.g., proving it has a field),
// equivalent paths should receive the same shape narrowings.
//
// The algorithm:
//
//  1. Collect all shape narrowings from d.Shape.Narrowed.
//
//  2. Group narrowings by equivalence class root (via EGraph.Find).
//
//  3. Compute intersection of shape types within each class. For records,
//     this means keeping only fields present in all class members.
//
//  4. For each path in the E-graph that lacks a direct narrowing but belongs
//     to a class with narrowings, apply the class intersection.
//
// Unlike Type domain propagation, Shape domain does not mark unsatisfiability
// for incompatible shapes. Instead, the intersection may be empty or become
// an open record, which is still valid (represents "unknown shape").
//
// Shape narrowings are particularly important for tables with computed keys
// or aliased references:
//
//	local t = {}
//	local ref = t
//	t.foo = 1
//	-- ref should know about .foo field through shape propagation
//
// The method modifies d.Shape.Narrowed in place.
func (d *ProductDomain) propagateShapeNarrowingsCC() {
	if len(d.Shape.Narrowed) == 0 {
		return
	}

	classNarrowings := make(map[constraint.PathKey]typ.Type)
	for _, key := range constraint.SortedPathKeys(d.Shape.Narrowed) {
		narrowed := d.Shape.Narrowed[key]
		root := d.EGraph.Find(key)
		if existing, ok := classNarrowings[root]; ok {
			intersection := narrow.Intersect(existing, narrowed)
			if intersection != nil && !intersection.Kind().IsNever() {
				classNarrowings[root] = intersection
			}
		} else {
			classNarrowings[root] = narrowed
		}
	}

	for _, key := range d.EGraph.AllPaths() {
		root := d.EGraph.Find(key)
		if narrowed, ok := classNarrowings[root]; ok {
			if d.Shape.Narrowed[key] == nil {
				base := d.Shape.TypeAt(key)
				if base != nil {
					intersection := narrow.Intersect(base, narrowed)
					if intersection != nil && !intersection.Kind().IsNever() {
						d.Shape.Narrowed[key] = intersection
					}
				}
			}
		}
	}
}

// ApplyCondition applies a DNF (Disjunctive Normal Form) condition.
//
// A DNF condition is an OR of ANDs: (A1 AND A2) OR (B1 AND B2 AND B3) OR ...
// Each disjunct is a conjunction of primitive constraints.
//
// The algorithm handles conditions based on their structure:
//
//   - False condition (empty disjuncts): Returns false immediately. No possible
//     narrowing can make a false condition true.
//
//   - True condition (single empty disjunct): Returns true. No constraints to apply.
//
//   - Single disjunct: Applies as a conjunction directly to the current domain.
//     No cloning or joining needed.
//
//   - Multiple disjuncts: Uses speculative evaluation with join:
//     1. For each disjunct, clone the current domain state
//     2. Apply the conjunction to the clone
//     3. If satisfiable, accumulate into the result via Join
//     4. If all disjuncts are unsatisfiable, return false
//
// The Join operation computes the least upper bound (widening) of two domain
// states. For types, this means computing unions. For numeric ranges, this
// means taking the outer bounds. The result represents "values that could
// come from either branch."
//
// This enables reasoning about conditionals like:
//
//	if type(x) == "string" or type(x) == "number" then
//	    -- x is narrowed to string|number
//	end
//
// The method mutates the receiver to match the accumulated result.
func (d *ProductDomain) ApplyCondition(cond constraint.Condition) bool {
	if cond.IsFalse() {
		return false
	}
	if cond.IsTrue() {
		return true
	}

	if len(cond.Disjuncts) == 1 {
		return d.ApplyConjunction(cond.Disjuncts[0])
	}

	var accumulator *ProductDomain
	for _, disjunct := range cond.Disjuncts {
		clone := d.Clone().(*ProductDomain)
		if clone.ApplyConjunction(disjunct) {
			if accumulator == nil {
				accumulator = clone
			} else {
				accumulator = accumulator.Join(clone).(*ProductDomain)
			}
		}
	}

	if accumulator == nil {
		return false
	}

	d.Type = accumulator.Type
	d.Numeric = accumulator.Numeric
	d.Shape = accumulator.Shape
	d.EGraph = accumulator.EGraph
	return true
}

// TypeAt returns the narrowed type for a PathKey, combining all domain information.
//
// This is the primary query method for retrieving type information after
// constraint application. It combines narrowings from multiple sources:
//
//  1. Type domain narrowing: If the Type domain has a narrowed type for this
//     key (from HasType, Truthy, etc. constraints), it takes precedence.
//
//  2. Shape domain narrowing: If the Shape domain has a narrowed type
//     (from structural constraints), it's considered.
//
//  3. Combined narrowing: If both domains have narrowings, computes their
//     intersection. A value must satisfy both type and shape constraints.
//
//  4. Base type fallback: If no narrowings exist, falls back to env.PathTypeAt
//     to retrieve the original declared or inferred type.
//
// Returns nil if the path has no type information in any source.
//
// Example usage:
//
//	// After applying HasType(x, "string") constraint:
//	t := domain.TypeAt("x@1")  // Returns typ.String
//
//	// After applying field presence constraint:
//	t := domain.TypeAt("t@1")  // Returns record type with proven fields
//
// The intersection semantics ensure soundness: if Type domain says "string"
// and Shape domain says "has field .len", the result is their intersection
// (which would be string, since strings have .len in Lua).
func (d *ProductDomain) TypeAt(key constraint.PathKey) typ.Type {
	typeNarrowed := d.Type.NarrowedTypeAt(key)
	shapeNarrowed := d.Shape.NarrowedTypeAt(key)

	if typeNarrowed != nil && shapeNarrowed != nil {
		return narrow.Intersect(typeNarrowed, shapeNarrowed)
	}

	if typeNarrowed != nil {
		return typeNarrowed
	}
	if shapeNarrowed != nil {
		return shapeNarrowed
	}

	if d.env.PathTypeAt != nil {
		return d.env.PathTypeAt(key)
	}
	return nil
}

// IsUnsat returns true if the domain state represents a contradiction.
//
// A domain is unsatisfiable when the applied constraints are mutually
// inconsistent. This can happen in several ways:
//
//   - Type contradiction: HasType("string") followed by HasType("number")
//     on the same path. No value can be both string and number.
//
//   - Numeric contradiction: x > 10 followed by x < 5. No number satisfies both.
//
//   - Shape contradiction: Record proven to lack a field that's required.
//
// When any subdomain becomes unsatisfiable, the entire ProductDomain is
// considered unsatisfiable. This is used during ApplyCondition to detect
// dead branches and during ApplyConjunction to detect invalid constraints.
//
// Returns true if Type, Numeric, OR Shape subdomain is unsatisfiable.
func (d *ProductDomain) IsUnsat() bool {
	return d.Type.IsUnsat() || d.Numeric.IsUnsat() || d.Shape.IsUnsat()
}

// Clone creates a deep copy of the ProductDomain.
//
// Clone is essential for speculative evaluation in ApplyCondition. When
// evaluating disjunctions (OR conditions), each branch is explored on a
// cloned domain to avoid polluting the original state with tentative narrowings.
//
// The clone includes deep copies of:
//   - Type domain: All narrowed type mappings
//   - Numeric domain: All numeric bounds and constraints
//   - Shape domain: All structural narrowings
//   - E-graph: All path equivalences
//
// The environment (env) is shared by reference since it contains only
// function pointers that don't change during analysis.
//
// Clone operations are a significant cost in constraint solving. The
// ProductDomain minimizes this by only cloning the narrowed values maps,
// not the base types (which are immutable and shared).
//
// Example:
//
//	original := NewProductDomain(env)
//	original.ApplyAtom(hasTypeString)  // Narrows x to string
//
//	clone := original.Clone().(*ProductDomain)
//	clone.ApplyAtom(hasTypeNumber)  // Narrows x to number in clone only
//
//	original.TypeAt("x")  // Still string
//	clone.TypeAt("x")     // Now number
func (d *ProductDomain) Clone() domain.Domain {
	return &ProductDomain{
		Type:    d.Type.Clone().(*domain.TypeDomain),
		Numeric: d.Numeric.Clone().(*numeric.Domain),
		Shape:   d.Shape.Clone().(*domain.ShapeDomain),
		EGraph:  d.EGraph.Clone(),
		env:     d.env,
	}
}

// Join computes the least upper bound of two ProductDomain states.
//
// Join is used to merge type information at control flow join points, such as
// after if-else branches or at loop headers. The result represents values that
// could have come from either domain.
//
// For each subdomain, Join computes a widening:
//
//   - Type domain: Types are unioned. If d has x:string and o has x:number,
//     the joined result has x:string|number.
//
//   - Numeric domain: Bounds are widened. If d has x>5 and o has x>10,
//     the joined result has x>5 (the weaker bound).
//
//   - Shape domain: Structural constraints are intersected. Only fields
//     proven in BOTH domains are preserved.
//
//   - E-graph: Equivalences are intersected. Only path equalities that hold
//     in BOTH domains are preserved. If d has a==b but o doesn't, the joined
//     result does not have a==b.
//
// The E-graph join is conservative: we only keep equivalences where both
// domains agree on the same root. This prevents unsound conclusions when
// equality only holds on one branch.
//
// Example:
//
//	// Branch 1: x is string
//	d := NewProductDomain(env)
//	d.ApplyAtom(hasTypeString)
//
//	// Branch 2: x is number
//	o := NewProductDomain(env)
//	o.ApplyAtom(hasTypeNumber)
//
//	// After join: x is string|number
//	joined := d.Join(o).(*ProductDomain)
//
// Join is commutative: d.Join(o) equals o.Join(d).
func (d *ProductDomain) Join(other domain.Domain) domain.Domain {
	o := other.(*ProductDomain)
	joinedEG := theory.NewEGraph()
	for _, k := range d.EGraph.AllPaths() {
		rootD := d.EGraph.Find(k)
		rootO := o.EGraph.Find(k)
		if rootD == rootO {
			joinedEG.Union(k, rootD)
		}
	}
	return &ProductDomain{
		Type:    d.Type.Join(o.Type).(*domain.TypeDomain),
		Numeric: d.Numeric.Join(o.Numeric).(*numeric.Domain),
		Shape:   d.Shape.Join(o.Shape).(*domain.ShapeDomain),
		EGraph:  joinedEG,
		env:     d.env,
	}
}

// NarrowedChildPaths returns all narrowed paths that are children of the given parent key.
//
// This method collects narrowings for all paths that extend the parent path,
// such as fields or indices. Given a parent key "t@1", it returns narrowings
// for paths like "t@1.foo", "t@1.bar", "t@1[0]", etc.
//
// The method combines narrowings from both Type and Shape domains:
//
//   - Type domain narrowings: Direct type constraints on child paths
//   - Shape domain narrowings: Structural constraints that narrow field types
//
// When both domains have narrowings for the same child path, their intersection
// is computed. This ensures the returned types reflect all known constraints.
//
// The parentKey is matched as a prefix with a dot separator. For example:
//   - Parent "sym1@1" matches children "sym1@1.foo", "sym1@1.bar"
//   - Parent "sym1@1" does NOT match "sym1@10" (different version)
//   - Parent "sym1@1" does NOT match "sym1@1foo" (no separator)
//
// This is used during type synthesis to collect narrowings learned about
// table fields, which can then be merged back into the parent table type.
//
// Example:
//
//	// After narrowing t.x to string and t.y to number:
//	children := domain.NarrowedChildPaths("t@1")
//	// Returns: {"t@1.x": string, "t@1.y": number}
//
// Returns an empty map if no child paths have narrowings.
func (d *ProductDomain) NarrowedChildPaths(parentKey constraint.PathKey) map[constraint.PathKey]typ.Type {
	result := make(map[constraint.PathKey]typ.Type)
	parent := string(parentKey)

	for _, key := range constraint.SortedPathKeys(d.Type.Narrowed) {
		t := d.Type.Narrowed[key]
		if domain.IsChildPath(parent, string(key)) {
			result[key] = t
		}
	}
	for _, key := range constraint.SortedPathKeys(d.Shape.Narrowed) {
		t := d.Shape.Narrowed[key]
		if domain.IsChildPath(parent, string(key)) {
			if existing, ok := result[key]; ok {
				result[key] = narrow.Intersect(existing, t)
			} else {
				result[key] = t
			}
		}
	}
	return result
}

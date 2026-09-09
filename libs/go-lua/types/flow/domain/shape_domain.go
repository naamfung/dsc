package domain

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// ShapeDomain handles structural narrowing for tables and records.
//
// ShapeDomain is one of the three subdomains in ProductDomain. It handles
// constraints that affect the structural shape of types, particularly tables:
//
//   - Field presence: HasField constraints proving a table has a field
//   - Field types: Narrowing field types based on usage
//   - Index constraints: HasIndex proving indexability with a type
//   - Metatable constraints: Proving metatable presence or structure
//
// Unlike TypeDomain which handles primitive type predicates, ShapeDomain uses
// constraint.Solver for sophisticated shape reasoning. The Solver can:
//
//   - Apply constraints to record types, narrowing fields
//   - Handle open records (unknown additional fields)
//   - Process map component constraints
//   - Chain constraints through nested paths
//
// When a constraint targets a nested path (e.g., t.user.name), ShapeDomain
// propagates narrowings to ancestor paths. If t.user.name is proven to be
// string, this may narrow t.user and t as well by proving they have the
// required structure.
//
// The domain maintains a Narrowed map from path keys to narrowed types.
// Missing keys fall back to env.PathTypeAt for base types.
type ShapeDomain struct {
	Narrowed map[constraint.PathKey]typ.Type
	Solver   constraint.Solver
	Env      constraint.Env
	Unsat    bool
}

// NewShapeDomain creates a new ShapeDomain with the given environment.
//
// The Solver is initialized with the same environment, ensuring consistent
// type resolution for nested constraint application.
func NewShapeDomain(env constraint.Env) *ShapeDomain {
	return &ShapeDomain{
		Narrowed: make(map[constraint.PathKey]typ.Type),
		Solver:   constraint.Solver{Env: env},
		Env:      env,
	}
}

// TypeAt returns the type for a PathKey, preferring narrowed types over base types.
//
// Lookup order:
//  1. d.Narrowed[key] - explicitly narrowed type from shape constraints
//  2. env.PathTypeAt(key) - base type from declarations
//
// Returns nil if the key has no type in either source.
func (d *ShapeDomain) TypeAt(key constraint.PathKey) typ.Type {
	if t, ok := d.Narrowed[key]; ok {
		return t
	}
	if d.Env.PathTypeAt != nil {
		return d.Env.PathTypeAt(key)
	}
	return nil
}

// NarrowedTypeAt returns only the explicitly narrowed type, not the base type.
//
// Unlike TypeAt, this returns nil if there is no shape narrowing, even if
// the key has a base type. Used to check whether narrowing has occurred.
func (d *ShapeDomain) NarrowedTypeAt(key constraint.PathKey) typ.Type {
	return d.Narrowed[key]
}

// ApplyConstraint applies a structural constraint and propagates to ancestors.
//
// The method performs several steps:
//
//  1. Look up the base type for the target path
//  2. Apply the constraint via Solver.ApplyToSingle to get a narrowed type
//  3. If narrowed type differs from base, store it in Narrowed map
//  4. Walk up ancestor paths and apply the constraint to each
//
// Ancestor propagation is crucial for nested field constraints. Given:
//
//	local t = {}  -- t: {}
//	if t.user.name then
//	    -- constraint: HasField(t.user, "name")
//	end
//
// The constraint must propagate to prove t has a "user" field too.
// The method walks from the deepest path segment up to the root.
//
// Returns false and sets Unsat=true if the constraint proves unsatisfiability.
func (d *ShapeDomain) ApplyConstraint(c constraint.Constraint, target constraint.PathKey) bool {
	if d.Unsat {
		return false
	}

	base := d.TypeAt(target)
	if base == nil {
		return true
	}

	narrowed := d.Solver.ApplyToSingle([]constraint.Constraint{c}, target, base, d.Env.ResolvePath)

	if narrowed == nil || narrowed.Kind().IsNever() {
		d.Unsat = true
		return false
	}

	if !typ.TypeEquals(narrowed, base) {
		d.Narrowed[target] = narrowed
	}

	// Propagate to ALL ancestor paths for nested field constraints
	path, ok := constraint.FirstPath(c)
	if ok && len(path.Segments) > 0 {
		// Walk up the path tree, propagating to each ancestor
		for depth := len(path.Segments) - 1; depth >= 0; depth-- {
			ancestorPath := constraint.Path{
				Root:     path.Root,
				Symbol:   path.Symbol,
				Segments: path.Segments[:depth],
			}
			if d.Env.ResolvePath == nil {
				continue
			}
			ancestorKey := d.Env.ResolvePath(ancestorPath)
			if ancestorKey == "" || ancestorKey == target {
				continue
			}
			ancestorBase := d.TypeAt(ancestorKey)
			if ancestorBase == nil {
				continue
			}
			ancestorNarrowed := d.Solver.ApplyToSingle([]constraint.Constraint{c}, ancestorKey, ancestorBase, d.Env.ResolvePath)
			if ancestorNarrowed != nil && !ancestorNarrowed.Kind().IsNever() && !typ.TypeEquals(ancestorNarrowed, ancestorBase) {
				d.Narrowed[ancestorKey] = ancestorNarrowed
			}
		}
	}

	return true
}

// IsUnsat returns true if the domain has proven a structural contradiction.
func (d *ShapeDomain) IsUnsat() bool { return d.Unsat }

// Clone creates a deep copy of the ShapeDomain for speculative evaluation.
//
// The Narrowed map is deep copied; Solver and Env are shared by reference.
func (d *ShapeDomain) Clone() Domain {
	c := &ShapeDomain{
		Narrowed: make(map[constraint.PathKey]typ.Type, len(d.Narrowed)),
		Solver:   d.Solver,
		Env:      d.Env,
		Unsat:    d.Unsat,
	}
	for _, k := range constraint.SortedPathKeys(d.Narrowed) {
		c.Narrowed[k] = d.Narrowed[k]
	}
	return c
}

// Join computes the least upper bound of two ShapeDomain states.
//
// Join semantics for shape domains:
//   - If either domain is unsatisfiable, return a clone of the other
//   - For keys in both domains, compute type union (join.Two)
//   - Keys only in one domain are dropped (no narrowing survives)
//
// For records, type union preserves only fields present in both types.
// For example, joining {foo: string, bar: number} with {foo: string}
// yields {foo: string} (bar is not present in both).
func (d *ShapeDomain) Join(other Domain) Domain {
	o := other.(*ShapeDomain)
	if d.Unsat {
		return o.Clone()
	}
	if o.Unsat {
		return d.Clone()
	}

	result := &ShapeDomain{
		Narrowed: make(map[constraint.PathKey]typ.Type),
		Solver:   d.Solver,
		Env:      d.Env,
	}

	// Type union for keys in both (meet of facts)
	for _, key := range constraint.SortedPathKeys(d.Narrowed) {
		dt := d.Narrowed[key]
		if ot, ok := o.Narrowed[key]; ok {
			result.Narrowed[key] = typ.JoinPreferNonSoft(dt, ot)
		}
	}
	return result
}

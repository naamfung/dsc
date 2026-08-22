package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// ObjectClass classifies types for garbage collection optimization.
//
// Lua uses reference counting with cycle detection for memory management.
// Knowing whether a type can participate in cycles allows the runtime to
// optimize collection strategies:
//
//   - Terminating objects never need cycle detection
//   - Linking objects need reference tracking but not cycle detection
//   - Cyclic objects require full cycle detection
//
// This classification is conservative: if a type might form cycles, it is
// classified as Cyclic even if no actual cycles exist at runtime.
type ObjectClass int

const (
	// Terminating indicates the type cannot reference any object that could
	// form cycles. Examples: numbers, strings, booleans, nil.
	// These objects can be collected immediately when references drop to zero.
	Terminating ObjectClass = iota

	// Linking indicates the type is acyclic itself but may hold references
	// to potentially cyclic objects. Example: a record with only primitive
	// fields but with a metatable.
	// These objects need reference counting but not cycle detection.
	Linking

	// Cyclic indicates the type can participate in reference cycles.
	// Examples: tables that can reference themselves, functions with closures.
	// These objects require full cycle detection during garbage collection.
	Cyclic
)

// String returns the human-readable name of the object class.
func (c ObjectClass) String() string {
	switch c {
	case Terminating:
		return "terminating"
	case Linking:
		return "linking"
	default:
		return "cyclic"
	}
}

// CanFormCycle returns true if values of this type can potentially form cycles.
//
// The analysis is conservative and structural:
//   - Primitive types (nil, boolean, number, string) cannot form cycles
//   - Records can form cycles if any field type can form cycles
//   - Arrays can form cycles if the element type can form cycles
//   - Functions always can form cycles (closures may capture tables)
//   - Interfaces and type variables are conservatively cyclic
//   - References (unresolved types) are conservatively cyclic
//
// Visited tracking prevents infinite recursion on recursive type definitions.
func CanFormCycle(t typ.Type) bool {
	return canFormCycleVisited(t, make(map[typ.Type]bool))
}

// canFormCycleVisited recursively checks cycle potential with visited tracking.
// Returns true for nil types as a conservative default.
func canFormCycleVisited(t typ.Type, visited map[typ.Type]bool) bool {
	if t == nil {
		return true
	}

	if visited[t] {
		return true
	}

	visited[t] = true

	return typ.Visit(t, typ.Visitor[bool]{
		Literal: func(lit *typ.Literal) bool {
			return false
		},
		Record: func(r *typ.Record) bool {
			for _, field := range r.Fields {
				if canFormCycleVisited(field.Type, visited) {
					return true
				}
			}

			return false
		},
		Map: func(m *typ.Map) bool {
			return canFormCycleVisited(m.Key, visited) || canFormCycleVisited(m.Value, visited)
		},
		Function: func(fn *typ.Function) bool {
			return true
		},
		Array: func(a *typ.Array) bool {
			return canFormCycleVisited(a.Element, visited)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if canFormCycleVisited(elem, visited) {
					return true
				}
			}

			return false
		},
		Optional: func(o *typ.Optional) bool {
			return canFormCycleVisited(o.Inner, visited)
		},
		Union: func(u *typ.Union) bool {
			for _, alt := range u.Members {
				if canFormCycleVisited(alt, visited) {
					return true
				}
			}

			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if canFormCycleVisited(member, visited) {
					return true
				}
			}

			return false
		},
		Interface: func(i *typ.Interface) bool {
			return true
		},
		TypeVar: func(tv *typ.TypeVar) bool {
			return true
		},
		Generic: func(g *typ.Generic) bool {
			return true
		},
		Ref: func(r *typ.Ref) bool {
			return true
		},
		Alias: func(a *typ.Alias) bool {
			return canFormCycleVisited(a.Target, visited)
		},
		Default: func(t typ.Type) bool {
			k := t.Kind()
			if k == kind.Nil || k == kind.Boolean || k == kind.Number ||
				k == kind.Integer || k == kind.String || k == kind.Never {
				return false
			}

			return true
		},
	})
}

// IsProvenAcyclic returns true if the type is provably acyclic.
//
// This is the inverse of CanFormCycle. A true result guarantees no cycles
// can form; a false result means cycles are possible (but not certain).
func IsProvenAcyclic(t typ.Type) bool {
	return !CanFormCycle(t)
}

// GetObjectClass determines the garbage collection class for a type.
//
// The classification process:
//  1. If provably acyclic (primitives, etc.), returns Terminating
//  2. For records, checks if fields contain any/unknown types (returns Cyclic)
//  3. For records, computes field reachability to detect self-references
//  4. If no field can reach itself, returns Linking
//  5. Otherwise returns Cyclic
//
// This classification enables garbage collection optimizations at compile time.
func GetObjectClass(t typ.Type) ObjectClass {
	if IsProvenAcyclic(t) {
		return Terminating
	}

	if rec, ok := t.(*typ.Record); ok {
		for _, f := range rec.Fields {
			if f.Type != nil && f.Type.Kind().IsPlaceholder() {
				return Cyclic
			}
		}

		reach := ComputeReachability(rec)
		if IsAcyclicByReach(rec, reach) {
			return Linking
		}
	}

	return Cyclic
}

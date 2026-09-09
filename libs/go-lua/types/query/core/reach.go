package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// ReachMatrix tracks which fields can transitively reach other fields within a record.
//
// This is used for cycle detection: if field A can reach field B, and B can reach A,
// then the type can form cycles. The matrix is the transitive closure of the direct
// "contains" relation between fields.
//
// Example for type {parent: Self?, children: Self[]}:
//
//	CanReach["parent"]["parent"] = true  (cycle: parent -> parent)
//	CanReach["parent"]["children"] = true
//	CanReach["children"]["parent"] = true
//	CanReach["children"]["children"] = true  (cycle: children -> children)
type ReachMatrix struct {
	// CanReach[A][B] is true if field A can transitively contain field B's type.
	CanReach map[string]map[string]bool
}

// ComputeReachability computes the transitive closure of field references.
//
// Given a record type, this builds a matrix showing which fields can transitively
// contain references to other fields of the same record. The algorithm:
//  1. For each field, find all other record fields reachable through its type
//  2. Apply Floyd-Warshall to compute transitive closure
//
// This is used by [IsAcyclicByReach] to determine if a record can form cycles.
func ComputeReachability(t *typ.Record) *ReachMatrix {
	if t == nil {
		return &ReachMatrix{CanReach: make(map[string]map[string]bool)}
	}

	reach := make(map[string]map[string]bool)
	for _, field := range t.Fields {
		reach[field.Name] = make(map[string]bool)

		visited := make(map[typ.Type]bool)

		reachable := findReachableFields(field.Type, t, visited)
		for _, r := range reachable {
			reach[field.Name][r] = true
		}
	}

	fields := make([]string, len(t.Fields))
	for i, f := range t.Fields {
		fields[i] = f.Name
	}

	for _, k := range fields {
		for _, i := range fields {
			for _, j := range fields {
				if reach[i][k] && reach[k][j] {
					reach[i][j] = true
				}
			}
		}
	}

	return &ReachMatrix{CanReach: reach}
}

// findReachableFields finds all field names of target reachable from type t.
// Used to build the initial "direct contains" relation before transitive closure.
func findReachableFields(t typ.Type, target *typ.Record, visited map[typ.Type]bool) []string {
	if t == nil || visited[t] {
		return nil
	}

	visited[t] = true

	return typ.Visit(t, typ.Visitor[[]string]{
		Record: func(r *typ.Record) []string {
			if r == target {
				names := make([]string, len(r.Fields))
				for i, f := range r.Fields {
					names[i] = f.Name
				}

				return names
			}

			var result []string
			for _, f := range r.Fields {
				result = append(result, findReachableFields(f.Type, target, visited)...)
			}

			return result
		},
		Array: func(a *typ.Array) []string {
			return findReachableFields(a.Element, target, visited)
		},
		Optional: func(o *typ.Optional) []string {
			return findReachableFields(o.Inner, target, visited)
		},
		Union: func(u *typ.Union) []string {
			var result []string
			for _, alt := range u.Members {
				result = append(result, findReachableFields(alt, target, visited)...)
			}

			return result
		},
		Intersection: func(in *typ.Intersection) []string {
			var result []string
			for _, member := range in.Members {
				result = append(result, findReachableFields(member, target, visited)...)
			}

			return result
		},
		Ref: func(r *typ.Ref) []string {
			// Refs are unresolved - cannot determine reachability
			return nil
		},
		Alias: func(a *typ.Alias) []string {
			return findReachableFields(a.Target, target, visited)
		},
		Default: func(t typ.Type) []string {
			return nil
		},
	})
}

// IsAcyclicByReach returns true if no field can transitively reach itself.
//
// A record is acyclic by reachability if, after computing the transitive closure,
// no field X has CanReach[X][X] = true. This is a sufficient (but not necessary)
// condition for proving a type cannot form reference cycles.
func IsAcyclicByReach(t *typ.Record, reach *ReachMatrix) bool {
	if t == nil || reach == nil {
		return true
	}

	for _, field := range t.Fields {
		if reach.CanReach[field.Name][field.Name] {
			return false
		}
	}

	return true
}

// FieldCanCycle returns true if a specific field can contribute to cycles.
//
// A field contributes to cycles if its type can contain references back to
// the containing record. This is used for more granular cycle analysis.
func FieldCanCycle(t *typ.Record, fieldName string) bool {
	if t == nil {
		return false
	}

	var fieldType typ.Type

	for _, f := range t.Fields {
		if f.Name == fieldName {
			fieldType = f.Type
			break
		}
	}

	if fieldType == nil {
		return false
	}

	return CanContain(fieldType, t)
}

// CanContain returns true if the container type can hold references to target.
//
// This is a structural check that traverses composite types (records, arrays,
// maps, unions, etc.) to determine if a value of target type could be stored
// somewhere within a value of container type.
func CanContain(container, target typ.Type) bool {
	return canContainWithVisited(container, target, make(map[typ.Type]bool))
}

// canContainWithVisited recursively checks containment with cycle detection.
func canContainWithVisited(container, target typ.Type, visited map[typ.Type]bool) bool {
	if container == nil {
		return false
	}

	if visited[container] {
		return false
	}

	visited[container] = true

	if container == target {
		return true
	}

	switch container.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
		return false
	}

	return typ.Visit(container, typ.Visitor[bool]{
		Record: func(r *typ.Record) bool {
			if r == target {
				return true
			}

			for _, field := range r.Fields {
				if canContainWithVisited(field.Type, target, visited) {
					return true
				}
			}

			return false
		},
		Array: func(a *typ.Array) bool {
			return canContainWithVisited(a.Element, target, visited)
		},
		Map: func(m *typ.Map) bool {
			return canContainWithVisited(m.Key, target, visited) ||
				canContainWithVisited(m.Value, target, visited)
		},
		Optional: func(o *typ.Optional) bool {
			return canContainWithVisited(o.Inner, target, visited)
		},
		Union: func(u *typ.Union) bool {
			for _, alt := range u.Members {
				if canContainWithVisited(alt, target, visited) {
					return true
				}
			}

			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if canContainWithVisited(member, target, visited) {
					return true
				}
			}

			return false
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if canContainWithVisited(elem, target, visited) {
					return true
				}
			}

			return false
		},
		Ref: func(r *typ.Ref) bool {
			// Refs are unresolved - conservative
			return true
		},
		Alias: func(a *typ.Alias) bool {
			return canContainWithVisited(a.Target, target, visited)
		},
		Default: func(t typ.Type) bool {
			return t.Kind().IsPlaceholder()
		},
	})
}

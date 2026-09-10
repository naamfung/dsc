package core

import (
	"sort"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func stopDepth(t typ.Type, depth int) bool {
	return t == nil || typ.DepthExceeded(depth)
}

// TypeNames returns the string representations of a slice of types.
//
// This is a convenience function for debugging and error messages.
// Nil types are represented as "<nil>".
func TypeNames(types []typ.Type) []string {
	if types == nil {
		return nil
	}

	names := make([]string, len(types))

	for i, t := range types {
		if t == nil {
			names[i] = "<nil>"
		} else {
			names[i] = t.String()
		}
	}

	return names
}

// AllFields returns all field names from a type that has fields.
//
// For records, returns the field names directly.
// For interfaces, returns the method names.
// For unions, returns fields common to ALL members.
// For intersections, returns fields from ANY member.
// For wrappers (Alias, Optional, Instantiated), unwraps first.
//
// Returns nil if the type has no fields.
func AllFields(t typ.Type) []string {
	return allFieldsDepth(t, 0)
}

// allFieldsDepth recursively collects field names with depth limiting.
func allFieldsDepth(t typ.Type, depth int) []string {
	if stopDepth(t, depth) {
		return nil
	}

	return typ.Visit(t, typ.Visitor[[]string]{
		Record: func(r *typ.Record) []string {
			if len(r.Fields) == 0 {
				return nil
			}

			names := make([]string, len(r.Fields))
			for i, f := range r.Fields {
				names[i] = f.Name
			}

			return names
		},
		Interface: func(i *typ.Interface) []string {
			if len(i.Methods) == 0 {
				return nil
			}

			names := make([]string, 0, len(i.Methods))
			for _, m := range i.Methods {
				names = append(names, m.Name)
			}

			return names
		},
		Recursive: func(rec *typ.Recursive) []string {
			if rec.Body == nil || rec.Body == rec {
				return nil
			}
			return allFieldsDepth(rec.Body, depth+1)
		},
		Alias: func(a *typ.Alias) []string {
			return allFieldsDepth(a.Target, depth+1)
		},
		Optional: func(o *typ.Optional) []string {
			return allFieldsDepth(o.Inner, depth+1)
		},
		Instantiated: func(inst *typ.Instantiated) []string {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return nil
			}
			return allFieldsDepth(resolved, depth+1)
		},
		Union: func(u *typ.Union) []string {
			if len(u.Members) == 0 {
				return nil
			}
			var common map[string]bool
			for _, member := range u.Members {
				names := allFieldsDepth(member, depth+1)
				if len(names) == 0 {
					return nil
				}
				if common == nil {
					common = make(map[string]bool, len(names))
					for _, n := range names {
						common[n] = true
					}
					continue
				}
				next := make(map[string]bool, len(names))
				for _, n := range names {
					if common[n] {
						next[n] = true
					}
				}
				common = next
				if len(common) == 0 {
					return nil
				}
			}
			result := make([]string, 0, len(common))
			for n := range common {
				result = append(result, n)
			}
			sort.Strings(result)
			return result
		},
		Intersection: func(in *typ.Intersection) []string {
			if len(in.Members) == 0 {
				return nil
			}
			seen := make(map[string]bool)
			for _, member := range in.Members {
				for _, n := range allFieldsDepth(member, depth+1) {
					seen[n] = true
				}
			}
			if len(seen) == 0 {
				return nil
			}
			result := make([]string, 0, len(seen))
			for n := range seen {
				result = append(result, n)
			}
			sort.Strings(result)
			return result
		},
		Default: func(t typ.Type) []string {
			return nil
		},
	})
}

// AllFieldTypes returns field names mapped to their types.
//
// Similar to AllFields but includes the type of each field. For unions,
// common field types are unioned together. For intersections, common field
// types are intersected. Handles wrappers (Alias, Optional, Instantiated).
//
// Returns nil if the type has no fields.
func AllFieldTypes(t typ.Type) map[string]typ.Type {
	return allFieldTypesDepth(t, 0)
}

// AllFieldTypesResolved returns field names mapped to their types with Self resolved.
//
// Self is a special type representing the containing type, used in interfaces
// and records with self-referential methods. This function substitutes Self
// with the actual containing type, producing fully resolved field types.
//
// Example: Interface { clone(): Self } resolved on type T gives { clone(): T }
func AllFieldTypesResolved(t typ.Type) map[string]typ.Type {
	fields := AllFieldTypes(t)
	if fields == nil {
		return nil
	}
	// Substitute Self for all types - Records can have method fields with Self params too
	for name, ft := range fields {
		fields[name] = subst.Self(ft, t)
	}
	return fields
}

// allFieldTypesDepth recursively collects field types with depth limiting.
func allFieldTypesDepth(t typ.Type, depth int) map[string]typ.Type {
	if stopDepth(t, depth) {
		return nil
	}

	return typ.Visit(t, typ.Visitor[map[string]typ.Type]{
		Record: func(r *typ.Record) map[string]typ.Type {
			if len(r.Fields) == 0 {
				return nil
			}
			fields := make(map[string]typ.Type, len(r.Fields))
			for _, f := range r.Fields {
				fields[f.Name] = f.Type
			}
			return fields
		},
		Interface: func(i *typ.Interface) map[string]typ.Type {
			if len(i.Methods) == 0 {
				return nil
			}
			fields := make(map[string]typ.Type, len(i.Methods))
			for _, m := range i.Methods {
				fields[m.Name] = m.Type
			}
			return fields
		},
		Recursive: func(rec *typ.Recursive) map[string]typ.Type {
			if rec.Body == nil || rec.Body == rec {
				return nil
			}
			return allFieldTypesDepth(rec.Body, depth+1)
		},
		Alias: func(a *typ.Alias) map[string]typ.Type {
			return allFieldTypesDepth(a.Target, depth+1)
		},
		Optional: func(o *typ.Optional) map[string]typ.Type {
			return allFieldTypesDepth(o.Inner, depth+1)
		},
		Instantiated: func(inst *typ.Instantiated) map[string]typ.Type {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return nil
			}
			return allFieldTypesDepth(resolved, depth+1)
		},
		Union: func(u *typ.Union) map[string]typ.Type {
			if len(u.Members) == 0 {
				return nil
			}
			var out map[string]typ.Type
			for _, member := range u.Members {
				fields := allFieldTypesDepth(member, depth+1)
				if len(fields) == 0 {
					return nil
				}
				if out == nil {
					out = make(map[string]typ.Type, len(fields))
					for name, ft := range fields {
						if ft != nil {
							out[name] = ft
						}
					}
					continue
				}
				for name, existing := range out {
					ft, ok := fields[name]
					if !ok || ft == nil {
						delete(out, name)
						continue
					}
					if existing == nil {
						out[name] = ft
						continue
					}
					out[name] = typ.NewUnion(existing, ft)
				}
				if len(out) == 0 {
					return nil
				}
			}
			return out
		},
		Intersection: func(in *typ.Intersection) map[string]typ.Type {
			if len(in.Members) == 0 {
				return nil
			}
			out := make(map[string]typ.Type)
			for _, member := range in.Members {
				fields := allFieldTypesDepth(member, depth+1)
				if len(fields) == 0 {
					continue
				}
				for name, ft := range fields {
					if ft == nil {
						continue
					}
					if existing, ok := out[name]; ok && existing != nil {
						out[name] = typ.NewIntersection(existing, ft)
					} else {
						out[name] = ft
					}
				}
			}
			if len(out) == 0 {
				return nil
			}
			return out
		},
		Default: func(t typ.Type) map[string]typ.Type {
			return nil
		},
	})
}

// AllMethods returns all method names from a type with methods.
//
// For interfaces, returns method names from the interface definition.
// For records with metatables, returns field names from the metatable.
// Handles wrappers (Alias, Optional) by unwrapping first.
//
// Returns nil if the type has no methods.
func AllMethods(t typ.Type) []string {
	return allMethodsDepth(t, 0)
}

// allMethodsDepth recursively collects method names with depth limiting.
func allMethodsDepth(t typ.Type, depth int) []string {
	if stopDepth(t, depth) {
		return nil
	}

	return typ.Visit(t, typ.Visitor[[]string]{
		Interface: func(i *typ.Interface) []string {
			if len(i.Methods) == 0 {
				return nil
			}

			names := make([]string, 0, len(i.Methods))
			for _, m := range i.Methods {
				names = append(names, m.Name)
			}

			return names
		},
		Record: func(r *typ.Record) []string {
			if r.Metatable == nil {
				return nil
			}
			// Metatable methods are the fields of the metatable record
			if meta, ok := r.Metatable.(*typ.Record); ok {
				if len(meta.Fields) == 0 {
					return nil
				}

				names := make([]string, len(meta.Fields))
				for i, f := range meta.Fields {
					names[i] = f.Name
				}

				return names
			}

			return allMethodsDepth(r.Metatable, depth+1)
		},
		Recursive: func(rec *typ.Recursive) []string {
			if rec.Body == nil || rec.Body == rec {
				return nil
			}
			return allMethodsDepth(rec.Body, depth+1)
		},
		Alias: func(a *typ.Alias) []string {
			return allMethodsDepth(a.Target, depth+1)
		},
		Optional: func(o *typ.Optional) []string {
			return allMethodsDepth(o.Inner, depth+1)
		},
		Default: func(t typ.Type) []string {
			return nil
		},
	})
}

// Length returns the length of a type if known at compile time.
//
// Returns the statically known length for:
//   - Tuples: number of elements
//   - String literals: length of the string value
//
// Returns -1 if the length is not statically determinable (arrays, maps, etc.).
func Length(t typ.Type) int {
	return lengthDepth(t, 0)
}

// lengthDepth recursively computes length with depth limiting.
func lengthDepth(t typ.Type, depth int) int {
	if stopDepth(t, depth) {
		return -1
	}

	return typ.Visit(t, typ.Visitor[int]{
		Tuple: func(tup *typ.Tuple) int {
			return len(tup.Elements)
		},
		Literal: func(lit *typ.Literal) int {
			if s, ok := lit.Value.(string); ok {
				return len(s)
			}

			return -1
		},
		Recursive: func(rec *typ.Recursive) int {
			if rec.Body == nil || rec.Body == rec {
				return -1
			}
			return lengthDepth(rec.Body, depth+1)
		},
		Alias: func(a *typ.Alias) int {
			return lengthDepth(a.Target, depth+1)
		},
		Default: func(t typ.Type) int {
			return -1
		},
	})
}

// Iterable returns true if the type can be used in a for-in loop.
//
// Iterable types include:
//   - Arrays, Maps, Tuples, Records (table types)
//   - Strings (iterates over characters)
//   - Any (assumed iterable)
//   - Unions where ALL members are iterable
//   - Intersections where ANY member is iterable
//
// Returns false for non-collection types (numbers, booleans, functions).
func Iterable(t typ.Type) bool {
	return iterableDepth(t, 0)
}

// iterableDepth recursively checks iterability with depth limiting.
func iterableDepth(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Array: func(a *typ.Array) bool {
			return true
		},
		Map: func(m *typ.Map) bool {
			return true
		},
		Tuple: func(tup *typ.Tuple) bool {
			return true
		},
		Record: func(r *typ.Record) bool {
			return true
		},
		Recursive: func(rec *typ.Recursive) bool {
			if rec.Body == nil || rec.Body == rec {
				return false
			}
			return iterableDepth(rec.Body, depth+1)
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if !iterableDepth(member, depth+1) {
					return false
				}
			}

			return len(u.Members) > 0
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if iterableDepth(member, depth+1) {
					return true
				}
			}

			return false
		},
		Optional: func(o *typ.Optional) bool {
			return iterableDepth(o.Inner, depth+1)
		},
		Alias: func(a *typ.Alias) bool {
			return iterableDepth(a.Target, depth+1)
		},
		Default: func(t typ.Type) bool {
			return t.Kind() == kind.String || typ.IsAny(t)
		},
	})
}

// Comparable returns true if values of this type can be compared with == and ~=.
//
// In Lua, all values can be compared for equality (any two values can be tested
// with == or ~=). Only nil types are not comparable.
func Comparable(t typ.Type) bool {
	return t != nil
}

// Ordered returns true if values of this type can be compared with <, >, <=, >=.
//
// Only numeric and string types support ordering in Lua. Numbers are ordered
// numerically; strings are ordered lexicographically.
//
// For unions, all members must be ordered (and ideally of the same base type
// for meaningful comparison, though this function doesn't enforce that).
func Ordered(t typ.Type) bool {
	return orderedDepth(t, 0)
}

// orderedDepth recursively checks orderability with depth limiting.
func orderedDepth(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Literal: func(lit *typ.Literal) bool {
			switch lit.Value.(type) {
			case int, int64, int32, float64, float32, string:
				return true
			}

			return false
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if !orderedDepth(member, depth+1) {
					return false
				}
			}

			return len(u.Members) > 0
		},
		Alias: func(a *typ.Alias) bool {
			return orderedDepth(a.Target, depth+1)
		},
		Default: func(t typ.Type) bool {
			switch t.Kind() {
			case kind.Number, kind.Integer, kind.String, kind.Any:
				return true
			default:
				return false
			}
		},
	})
}

// ContainsNil returns true if the type can contain nil values.
//
// A type contains nil if:
//   - It is Optional (T?)
//   - It is a union with a nil member
//   - It is nil, any, or unknown
//
// This is used for nil safety checks and to determine if Optional wrapping
// is necessary.
func ContainsNil(t typ.Type) bool {
	return containsNilDepth(t, 0)
}

// ElementType returns the element type of an array-like type.
//
// For arrays, returns the element type directly.
// For tuples, returns the union of all element types.
// For maps, returns the value type.
// For records with map components, returns the map value type.
// For unions, returns the union of element types from members.
//
// Returns nil for non-container types.
func ElementType(t typ.Type) typ.Type {
	return elementTypeDepth(t, 0)
}

// elementTypeDepth recursively extracts element types with depth limiting.
func elementTypeDepth(t typ.Type, depth int) typ.Type {
	if stopDepth(t, depth) {
		return nil
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Array: func(a *typ.Array) typ.Type {
			return a.Element
		},
		Tuple: func(tup *typ.Tuple) typ.Type {
			if len(tup.Elements) == 0 {
				return nil
			}
			return typ.NewUnion(tup.Elements...)
		},
		Map: func(m *typ.Map) typ.Type {
			return m.Value
		},
		Record: func(r *typ.Record) typ.Type {
			if r.HasMapComponent() {
				return r.MapValue
			}
			return nil
		},
		Optional: func(o *typ.Optional) typ.Type {
			return elementTypeDepth(o.Inner, depth+1)
		},
		Alias: func(a *typ.Alias) typ.Type {
			return elementTypeDepth(a.Target, depth+1)
		},
		Union: func(u *typ.Union) typ.Type {
			var types []typ.Type
			for _, m := range u.Members {
				if et := elementTypeDepth(m, depth+1); et != nil {
					types = append(types, et)
				}
			}
			if len(types) == 0 {
				return nil
			}
			return typ.NewUnion(types...)
		},
		Default: func(t typ.Type) typ.Type {
			return nil
		},
	})
}

// KeyType returns the key type of a map-like type.
//
// For maps, returns the key type directly.
// For arrays and tuples, returns integer (1-based indexing).
// For records, returns string (or union of literal field names for precision).
// For unions, returns the union of key types from members.
//
// Returns nil for non-container types.
func KeyType(t typ.Type) typ.Type {
	return keyTypeDepth(t, 0)
}

// keyTypeDepth recursively extracts key types with depth limiting.
func keyTypeDepth(t typ.Type, depth int) typ.Type {
	if stopDepth(t, depth) {
		return nil
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Map: func(m *typ.Map) typ.Type {
			return m.Key
		},
		Array: func(a *typ.Array) typ.Type {
			return typ.Integer
		},
		Tuple: func(tup *typ.Tuple) typ.Type {
			return typ.Integer
		},
		Record: func(r *typ.Record) typ.Type {
			if r.HasMapComponent() {
				if r.MapKey.Kind() == kind.String {
					return typ.String
				}
				return typ.NewUnion(typ.String, r.MapKey)
			}
			// Use literal field names when available for precision.
			if len(r.Fields) > 0 {
				lits := make([]typ.Type, 0, len(r.Fields))
				for _, f := range r.Fields {
					if f.Name != "" {
						lits = append(lits, typ.LiteralString(f.Name))
					}
				}
				if len(lits) > 0 {
					return typ.NewUnion(lits...)
				}
			}
			return typ.String
		},
		Optional: func(o *typ.Optional) typ.Type {
			return keyTypeDepth(o.Inner, depth+1)
		},
		Alias: func(a *typ.Alias) typ.Type {
			return keyTypeDepth(a.Target, depth+1)
		},
		Union: func(u *typ.Union) typ.Type {
			var types []typ.Type
			for _, m := range u.Members {
				if kt := keyTypeDepth(m, depth+1); kt != nil {
					types = append(types, kt)
				}
			}
			if len(types) == 0 {
				return nil
			}
			return typ.NewUnion(types...)
		},
		Default: func(t typ.Type) typ.Type {
			return nil
		},
	})
}

// ValueType returns the value type of a map-like type.
//
// For maps, returns the value type directly.
// For arrays, returns the element type.
// For tuples, returns the union of element types.
// For records, returns the union of all field types (and map value if present).
// For unions, returns the union of value types from members.
//
// Returns nil for non-container types.
func ValueType(t typ.Type) typ.Type {
	return valueTypeDepth(t, 0)
}

// valueTypeDepth recursively extracts value types with depth limiting.
func valueTypeDepth(t typ.Type, depth int) typ.Type {
	if stopDepth(t, depth) {
		return nil
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Map: func(m *typ.Map) typ.Type {
			return m.Value
		},
		Array: func(a *typ.Array) typ.Type {
			return a.Element
		},
		Tuple: func(tup *typ.Tuple) typ.Type {
			if len(tup.Elements) == 0 {
				return nil
			}
			return typ.NewUnion(tup.Elements...)
		},
		Record: func(r *typ.Record) typ.Type {
			var types []typ.Type
			for _, f := range r.Fields {
				types = append(types, f.Type)
			}
			if r.HasMapComponent() {
				types = append(types, r.MapValue)
			}
			if len(types) == 0 {
				return nil
			}
			return typ.NewUnion(types...)
		},
		Optional: func(o *typ.Optional) typ.Type {
			return valueTypeDepth(o.Inner, depth+1)
		},
		Alias: func(a *typ.Alias) typ.Type {
			return valueTypeDepth(a.Target, depth+1)
		},
		Union: func(u *typ.Union) typ.Type {
			var types []typ.Type
			for _, m := range u.Members {
				if vt := valueTypeDepth(m, depth+1); vt != nil {
					types = append(types, vt)
				}
			}
			if len(types) == 0 {
				return nil
			}
			return typ.NewUnion(types...)
		},
		Default: func(t typ.Type) typ.Type {
			return nil
		},
	})
}

// containsNilDepth recursively checks nil containment with depth limiting.
func containsNilDepth(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return true
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return true
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if containsNilDepth(m, depth+1) {
					return true
				}
			}

			return false
		},
		Alias: func(a *typ.Alias) bool {
			return containsNilDepth(a.Target, depth+1)
		},
		Default: func(t typ.Type) bool {
			k := t.Kind()
			return k == kind.Nil || k.IsPlaceholder()
		},
	})
}

// IsArrayLike returns true if the type is a Map or Array.
//
// This is used to determine if a type can be treated as an indexable collection
// for operations like iteration and length.
func IsArrayLike(t typ.Type) bool {
	return isArrayLikeDepth(t, 0)
}

// isArrayLikeDepth recursively checks array-likeness with depth limiting.
func isArrayLikeDepth(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	unwrapped := unwrap.Alias(t)
	if inst, ok := unwrapped.(*typ.Instantiated); ok {
		if resolved, err := ResolveInstantiated(inst); err == nil {
			return isArrayLikeDepth(resolved, depth+1)
		}
	}
	return typ.Visit(unwrapped, typ.Visitor[bool]{
		Map: func(m *typ.Map) bool {
			return true
		},
		Array: func(a *typ.Array) bool {
			return true
		},
		Default: func(t typ.Type) bool {
			return false
		},
	})
}

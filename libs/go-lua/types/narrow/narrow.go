package narrow

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

var tableTopType = typ.NewInterface("table", nil)

// narrowConfig defines customization points for recursive type narrowing.
//
// Each narrowing operation (RemoveNil, ToTruthy, etc.) creates a narrowConfig
// with handlers specialized for that operation. The handlers define how to
// process optional types, union types, and leaf (primitive/composite) types.
//
// The recurse function passed to handlers enables recursive processing of
// nested types while maintaining the same narrowing behavior throughout
// the type tree.
type narrowConfig struct {
	// handleOptional processes optional types (T?).
	// The recurse function applies the same narrowing to nested types.
	handleOptional func(opt *typ.Optional, recurse func(typ.Type) typ.Type) typ.Type

	// handleUnion processes union types (T | U | V).
	// The recurse function applies the same narrowing to nested types.
	handleUnion func(u *typ.Union, recurse func(typ.Type) typ.Type) typ.Type

	// handleLeaf processes non-composite types (primitives, records, functions, etc.).
	handleLeaf func(t typ.Type) typ.Type
}

// narrowType applies a narrowing configuration to traverse and transform a type.
//
// This is the main entry point for the narrowing machinery. It sets up the
// recursive traversal and delegates to [narrowTypeImpl] for the actual work.
func narrowType(t typ.Type, cfg narrowConfig) typ.Type {
	var recurse func(typ.Type) typ.Type
	recurse = func(inner typ.Type) typ.Type {
		return narrowTypeImpl(inner, cfg, recurse)
	}
	return narrowTypeImpl(t, cfg, recurse)
}

// narrowTypeImpl implements the recursive type narrowing traversal.
//
// It handles the following type wrappers before delegating to config handlers:
//   - nil: Returns Never (narrowing nil always produces the empty type).
//   - Instantiated: Expands generic instantiation and recurses.
//   - Alias: Recurses into target, preserving alias wrapper if changed.
//   - Intersection: Recurses into all members; any Never makes result Never.
//   - Optional: Delegates to handleOptional.
//   - Union: Delegates to handleUnion.
//   - Other: Delegates to handleLeaf.
func narrowTypeImpl(t typ.Type, cfg narrowConfig, recurse func(typ.Type) typ.Type) typ.Type {
	if t == nil {
		return typ.Never
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Instantiated: func(inst *typ.Instantiated) typ.Type {
			if expanded := unwrap.Instantiated(inst); expanded != inst {
				return recurse(expanded)
			}
			return t
		},
		Alias: func(a *typ.Alias) typ.Type {
			inner := recurse(a.Target)
			if inner == nil || inner.Kind().IsNever() {
				return inner
			}
			if inner == a.Target {
				return t
			}
			return typ.NewAlias(a.Name, inner)
		},
		Intersection: func(in *typ.Intersection) typ.Type {
			var kept []typ.Type
			for _, m := range in.Members {
				nm := recurse(m)
				if nm == nil || nm.Kind().IsNever() {
					return typ.Never
				}
				kept = append(kept, nm)
			}
			return typ.NewIntersection(kept...)
		},
		Optional: func(o *typ.Optional) typ.Type {
			return cfg.handleOptional(o, recurse)
		},
		Union: func(u *typ.Union) typ.Type {
			return cfg.handleUnion(u, recurse)
		},
		Default: func(t typ.Type) typ.Type {
			return cfg.handleLeaf(t)
		},
	})
}

// RemoveNil removes nil from a type, producing the non-nullable subset.
//
// This operation is used when narrowing after a "x ~= nil" or "x" truthiness
// check in Lua. It handles all type structures that can contain nil:
//
// # Behavior by Type
//
//   - nil: Returns Never (removing nil from nil leaves nothing).
//   - Optional<T>: Returns T (the inner non-nil type).
//   - Union containing nil: Returns union without nil members.
//   - Union containing Optional<T>: Unwraps optional, keeps T.
//   - Other types: Returns unchanged (already non-nullable).
//
// # Examples
//
//	RemoveNil(typ.Nil)                    // Never
//	RemoveNil(typ.NewOptional(typ.String)) // String
//	RemoveNil(typ.NewUnion(typ.String, typ.Nil)) // String
//	RemoveNil(typ.Number)                 // Number (unchanged)
func RemoveNil(t typ.Type) typ.Type {
	if t == nil || t == typ.Nil {
		return typ.Never
	}
	return narrowType(t, narrowConfig{
		handleOptional: func(opt *typ.Optional, _ func(typ.Type) typ.Type) typ.Type {
			return opt.Inner
		},
		handleUnion: func(u *typ.Union, _ func(typ.Type) typ.Type) typ.Type {
			var kept []typ.Type
			for _, m := range u.Members {
				if m == nil || m.Kind() == kind.Nil {
					continue
				}
				if opt, ok := m.(*typ.Optional); ok {
					kept = append(kept, opt.Inner)
					continue
				}
				kept = append(kept, m)
			}
			if len(kept) == 0 {
				return typ.Never
			}
			return typ.NewUnion(kept...)
		},
		handleLeaf: func(t typ.Type) typ.Type {
			if t.Kind() == kind.Nil {
				return typ.Never
			}
			return t
		},
	})
}

// RemoveFalse removes literal false from a type, keeping only truthy boolean values.
//
// This operation narrows boolean types after checks that exclude false.
// Combined with [RemoveNil], it implements full truthiness narrowing.
//
// # Behavior by Type
//
//   - Literal false: Returns Never.
//   - Literal true: Returns true (unchanged).
//   - Boolean: Returns true (the truthy boolean literal).
//   - Optional<T>: Recurses into T; if T becomes Never, returns nil.
//   - Union: Filters out members that become Never after recursion.
//   - Other types: Returns unchanged (non-boolean types are unaffected).
//
// # Examples
//
//	RemoveFalse(typ.LiteralBool(false))  // Never
//	RemoveFalse(typ.Boolean)             // true
//	RemoveFalse(typ.String)              // String (unchanged)
func RemoveFalse(t typ.Type) typ.Type {
	if t == nil {
		return typ.Never
	}
	return narrowType(t, narrowConfig{
		handleOptional: func(opt *typ.Optional, recurse func(typ.Type) typ.Type) typ.Type {
			inner := recurse(opt.Inner)
			if inner == nil || inner.Kind().IsNever() {
				return typ.Nil
			}
			return typ.NewOptional(inner)
		},
		handleUnion: func(u *typ.Union, recurse func(typ.Type) typ.Type) typ.Type {
			var kept []typ.Type
			for _, m := range u.Members {
				if rm := recurse(m); rm != nil && !rm.Kind().IsNever() {
					kept = append(kept, rm)
				}
			}
			if len(kept) == 0 {
				return typ.Never
			}
			return typ.NewUnion(kept...)
		},
		handleLeaf: func(t typ.Type) typ.Type {
			if lit, ok := t.(*typ.Literal); ok {
				if b, ok := lit.Value.(bool); ok && !b {
					return typ.Never
				}
			}
			if t.Kind() == kind.Boolean {
				return typ.True
			}
			return t
		},
	})
}

// ToTruthy narrows a type to its truthy subset by removing nil and false.
//
// In Lua, only nil and false are falsy. This operation produces the type
// that remains after a truthiness check succeeds (the "then" branch of
// "if x then").
//
// ToTruthy is equivalent to RemoveFalse(RemoveNil(t)).
//
// # Examples
//
//	ToTruthy(typ.NewOptional(typ.String))          // String
//	ToTruthy(typ.Boolean)                          // true
//	ToTruthy(typ.NewUnion(typ.String, typ.Nil))    // String
//	ToTruthy(typ.NewUnion(typ.Nil, typ.False))     // Never
func ToTruthy(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	return RemoveFalse(RemoveNil(t))
}

// ToFalsy narrows a type to its falsy subset, keeping only nil and false.
//
// In Lua, only nil and false are falsy. This operation produces the type
// that remains after a truthiness check fails (the "else" branch of
// "if x then").
//
// # Behavior by Type
//
//   - nil: Returns nil.
//   - Literal false: Returns false.
//   - Literal true: Returns Never (true is truthy).
//   - Boolean: Returns false (the falsy boolean literal).
//   - Optional<T>: Returns nil | ToFalsy(T).
//   - Union: Collects falsy parts from each member.
//   - Placeholder types (Any, Unknown): Returns nil | false (conservative).
//   - FieldAccess, IndexAccess: Returns nil | false (deferred types).
//   - Other types: Returns Never (all other types are truthy).
//
// # Examples
//
//	ToFalsy(typ.NewOptional(typ.String))  // nil
//	ToFalsy(typ.Boolean)                  // false
//	ToFalsy(typ.String)                   // Never
//	ToFalsy(typ.Any)                      // nil | false
func ToFalsy(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	return narrowType(t, narrowConfig{
		handleOptional: func(opt *typ.Optional, recurse func(typ.Type) typ.Type) typ.Type {
			inner := recurse(opt.Inner)
			if inner == nil || inner.Kind().IsNever() {
				return typ.Nil
			}
			return typ.NewUnion(typ.Nil, inner)
		},
		handleUnion: func(u *typ.Union, recurse func(typ.Type) typ.Type) typ.Type {
			var falsy []typ.Type
			for _, m := range u.Members {
				if f := recurse(m); f != nil && !f.Kind().IsNever() {
					falsy = append(falsy, f)
				}
			}
			if len(falsy) == 0 {
				return typ.Never
			}
			return typ.NewUnion(falsy...)
		},
		handleLeaf: func(t typ.Type) typ.Type {
			return typ.Visit(t, typ.Visitor[typ.Type]{
				Literal: func(lit *typ.Literal) typ.Type {
					if b, ok := lit.Value.(bool); ok && !b {
						return t
					}
					return typ.Never
				},
				FieldAccess: func(f *typ.FieldAccess) typ.Type {
					return typ.NewUnion(typ.Nil, typ.LiteralBool(false))
				},
				IndexAccess: func(i *typ.IndexAccess) typ.Type {
					return typ.NewUnion(typ.Nil, typ.LiteralBool(false))
				},
				Default: func(t typ.Type) typ.Type {
					k := t.Kind()
					if k.IsPlaceholder() {
						return typ.NewUnion(typ.Nil, typ.LiteralBool(false))
					}
					switch k {
					case kind.Nil:
						return t
					case kind.Boolean:
						return typ.LiteralBool(false)
					default:
						return typ.Never
					}
				},
			})
		},
	})
}

// TypesOverlap checks if two types have any potential runtime overlap.
//
// Two types overlap if either is a subtype of the other. This is used by
// [ExcludeType] to determine which union members to remove.
//
// Returns false if either argument is nil.
//
// # Examples
//
//	TypesOverlap(typ.String, typ.String)                    // true
//	TypesOverlap(typ.String, typ.NewUnion(typ.String, typ.Number)) // true
//	TypesOverlap(typ.String, typ.Number)                    // false
func TypesOverlap(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	return subtype.IsSubtype(a, b) || subtype.IsSubtype(b, a)
}

// ExcludeType removes union members that overlap with the excluded type.
//
// For discriminated union narrowing, this operation removes variants that
// match a specific type after a negative type check.
//
// # Behavior by Type
//
//   - Union: Removes members that overlap with excluded; returns remaining.
//   - Optional<T>: If T overlaps with excluded, returns nil; else unchanged.
//   - Placeholder (Any, Unknown): Returns unchanged (cannot narrow).
//   - Other: Returns Never if overlaps with excluded; else unchanged.
//
// # Examples
//
//	ExcludeType(typ.NewUnion(typ.String, typ.Number), typ.String) // Number
//	ExcludeType(typ.String, typ.String)                           // Never
//	ExcludeType(typ.String, typ.Number)                           // String
func ExcludeType(t typ.Type, excluded typ.Type) typ.Type {
	if t == nil || excluded == nil {
		return t
	}
	return narrowType(t, narrowConfig{
		handleOptional: func(opt *typ.Optional, recurse func(typ.Type) typ.Type) typ.Type {
			inner := recurse(opt.Inner)
			if inner == nil || inner.Kind().IsNever() {
				return typ.Nil
			}
			if inner == opt.Inner {
				return opt
			}
			return typ.NewOptional(inner)
		},
		handleUnion: func(u *typ.Union, _ func(typ.Type) typ.Type) typ.Type {
			var kept []typ.Type
			for _, m := range u.Members {
				if !TypesOverlap(m, excluded) {
					kept = append(kept, m)
				}
			}
			if len(kept) == 0 {
				return typ.Never
			}
			if len(kept) == len(u.Members) {
				return u
			}
			if len(kept) == 1 {
				return kept[0]
			}
			return typ.NewUnion(kept...)
		},
		handleLeaf: func(t typ.Type) typ.Type {
			if t.Kind().IsPlaceholder() {
				return t
			}
			if TypesOverlap(t, excluded) {
				return typ.Never
			}
			return t
		},
	})
}

// ExcludeKind removes types matching the target kind from a type.
//
// This operation narrows after a negative typeof check like "type(x) ~= 'string'".
// It removes all union members whose kind matches the target.
//
// # Lua Kind Mapping
//
// In Lua, typeof returns: "nil", "boolean", "number", "string", "function",
// "table", "thread", "userdata". The kind.Record matches all table-like types
// (Record, Map, Array, Tuple, Interface, Intersection).
//
// # Behavior by Type
//
//   - Union: Removes members matching target kind.
//   - Optional: If target is Nil, returns inner; else recurses.
//   - Placeholder (Any, Unknown): Returns unchanged.
//   - Interface: Returns unchanged (interfaces need type-based exclusion).
//   - Other: Returns Never if kind matches; else unchanged.
//
// # Examples
//
//	ExcludeKind(typ.NewUnion(typ.String, typ.Number), kind.String) // Number
//	ExcludeKind(typ.NewOptional(typ.String), kind.Nil)              // String
func ExcludeKind(t typ.Type, target kind.Kind) typ.Type {
	if t == nil {
		return nil
	}
	return narrowType(t, narrowConfig{
		handleOptional: func(opt *typ.Optional, recurse func(typ.Type) typ.Type) typ.Type {
			if target == kind.Nil {
				return opt.Inner
			}
			inner := recurse(opt.Inner)
			if inner == nil || inner.Kind().IsNever() {
				return typ.Nil
			}
			return typ.NewOptional(inner)
		},
		handleUnion: func(u *typ.Union, _ func(typ.Type) typ.Type) typ.Type {
			var kept []typ.Type
			for _, m := range u.Members {
				if !KindMatches(m, target) {
					kept = append(kept, m)
				}
			}
			if len(kept) == 0 {
				return typ.Never
			}
			if len(kept) == 1 {
				return kept[0]
			}
			return typ.NewUnion(kept...)
		},
		handleLeaf: func(t typ.Type) typ.Type {
			if t.Kind().IsPlaceholder() {
				return t
			}
			if t.Kind() == kind.Interface {
				return t
			}
			if KindMatches(t, target) {
				return typ.Never
			}
			return t
		},
	})
}

// KindMatches checks if a type matches the target Lua typeof kind.
//
// This function implements Lua's typeof semantics for type narrowing.
// It handles special cases where multiple type system kinds map to a
// single Lua typeof result.
//
// # Special Cases
//
//   - kind.Record matches: Record, Map, Array, Tuple, Interface, Intersection
//     (all are "table" in Lua typeof).
//   - kind.Number matches: Number and Integer (integer is a subtype of number).
//   - Instantiated types: Match based on the underlying generic body's kind.
//
// # Examples
//
//	KindMatches(typ.String, kind.String)           // true
//	KindMatches(typ.Integer, kind.Number)          // true
//	KindMatches(typ.NewArray(typ.String), kind.Record) // true (arrays are tables)
func KindMatches(t typ.Type, target kind.Kind) bool {
	if t == nil {
		return false
	}
	k := t.Kind()
	if k == target {
		return true
	}

	// Instantiated types match based on their underlying body's kind.
	if k == kind.Instantiated {
		if inst, ok := t.(*typ.Instantiated); ok && inst.Generic != nil {
			return KindMatches(inst.Generic.Body, target)
		}
	}

	// "table" in Lua includes Record, Map, Array, Tuple, Interface, Intersection.
	if target == kind.Record {
		switch k {
		case kind.Record, kind.Map, kind.Array, kind.Tuple, kind.Interface, kind.Intersection:
			return true
		}
	}

	// Integer is also a number in Lua.
	if target == kind.Number && k == kind.Integer {
		return true
	}
	return false
}

// Intersect computes the intersection of two types.
//
// Type intersection produces the type containing values that belong to
// both input types. This is used for narrowing when multiple constraints
// apply to the same variable.
//
// # Algorithm
//
//  1. If either type is a placeholder (Any, Unknown), return the other.
//  2. Unwrap aliases and instantiated generics.
//  3. If either is an intersection, merge members.
//  4. If a <: b, return a (more specific); if b <: a, return b.
//  5. For unions, filter to overlapping members.
//  6. Otherwise, create a new intersection type.
//
// # Examples
//
//	Intersect(typ.String, typ.Any)                              // String
//	Intersect(typ.NewUnion(typ.String, typ.Number), typ.String) // String
//	Intersect(typ.String, typ.Number)                           // String & Number
func Intersect(a, b typ.Type) typ.Type {
	if a == nil || b == nil {
		return nil
	}

	if a.Kind().IsPlaceholder() {
		return b
	}
	if b.Kind().IsPlaceholder() {
		return a
	}

	// Unwrap aliases and instantiated generics.
	if ua, ok := a.(*typ.Alias); ok {
		return Intersect(ua.Target, b)
	}
	if ub, ok := b.(*typ.Alias); ok {
		return Intersect(a, ub.Target)
	}
	if expanded := unwrap.Instantiated(a); expanded != a {
		return Intersect(expanded, b)
	}
	if expanded := unwrap.Instantiated(b); expanded != b {
		return Intersect(a, expanded)
	}

	// Handle intersection types: a & (b1 & b2) = a & b1 & b2.
	if ia, ok := a.(*typ.Intersection); ok {
		members := append([]typ.Type{}, ia.Members...)
		members = append(members, b)
		return typ.NewIntersection(members...)
	}
	if ib, ok := b.(*typ.Intersection); ok {
		members := append([]typ.Type{a}, ib.Members...)
		return typ.NewIntersection(members...)
	}

	if subtype.IsSubtype(a, b) {
		return a
	}
	if subtype.IsSubtype(b, a) {
		return b
	}

	if ua, ok := a.(*typ.Union); ok {
		filtered := filterUnionByOverlap(ua, b)
		if filtered != nil {
			return filtered
		}
	}
	if ub, ok := b.(*typ.Union); ok {
		filtered := filterUnionByOverlap(ub, a)
		if filtered != nil {
			return filtered
		}
	}

	return typ.NewIntersection(a, b)
}

// filterUnionByOverlap filters a union to members that overlap with another type.
//
// This helper is used by [Intersect] to narrow unions. It keeps only those
// union members that have some overlap with the other type.
//
// Returns Never if no members overlap. Returns nil if the union is nil.
func filterUnionByOverlap(u *typ.Union, other typ.Type) typ.Type {
	if u == nil || other == nil {
		return nil
	}

	var kept []typ.Type
	for _, m := range u.Members {
		if TypesOverlap(m, other) {
			kept = append(kept, m)
		}
	}

	if len(kept) == 0 {
		return typ.Never
	}
	return typ.NewUnion(kept...)
}

// FilterByKind narrows a type to keep only parts matching the target kind.
//
// This operation narrows after a positive typeof check like "type(x) == 'string'".
// It is the inverse of [ExcludeKind].
//
// # Behavior by Type
//
//   - Placeholder (Any, Unknown): Returns the canonical type for the kind.
//   - Union: Keeps members matching target kind.
//   - Optional: If target is Nil, returns nil; else recurses into inner.
//   - Other: Returns type if kind matches; else Never.
//
// # Examples
//
//	FilterByKind(typ.NewUnion(typ.String, typ.Number), kind.String) // String
//	FilterByKind(typ.Any, kind.Number)                               // Number
//	FilterByKind(typ.NewOptional(typ.String), kind.Nil)              // nil
func FilterByKind(t typ.Type, target kind.Kind) typ.Type {
	if t == nil {
		return nil
	}
	if t.Kind().IsPlaceholder() {
		return TypeForKind(target)
	}
	return narrowType(t, narrowConfig{
		handleOptional: func(opt *typ.Optional, recurse func(typ.Type) typ.Type) typ.Type {
			if target == kind.Nil {
				return typ.Nil
			}
			inner := recurse(opt.Inner)
			if inner == nil || inner.Kind().IsNever() {
				return typ.Never
			}
			return inner
		},
		handleUnion: func(u *typ.Union, _ func(typ.Type) typ.Type) typ.Type {
			var kept []typ.Type
			for _, m := range u.Members {
				narrowed := FilterByKind(m, target)
				if narrowed != nil && !narrowed.Kind().IsNever() {
					kept = append(kept, narrowed)
				}
			}
			if len(kept) == 0 {
				return typ.Never
			}
			if len(kept) == 1 {
				return kept[0]
			}
			return typ.NewUnion(kept...)
		},
		handleLeaf: func(t typ.Type) typ.Type {
			if t.Kind().IsPlaceholder() {
				return TypeForKind(target)
			}
			if KindMatches(t, target) {
				return t
			}
			return typ.Never
		},
	})
}

// TypeForKind returns the canonical type for a Lua typeof kind.
//
// This mapping is used when narrowing placeholder types (Any, Unknown)
// by kind. It returns the broadest type for each Lua typeof result.
//
// # Mapping
//
//   - Nil: typ.Nil
//   - Boolean: typ.Boolean
//   - Number: typ.Number
//   - Integer: typ.Integer
//   - String: typ.String
//   - Function: fun(...any) -> any
//   - Record: builtin table top marker interface
//   - Any: typ.Any
//   - Other: typ.Unknown (no canonical type available)
func TypeForKind(k kind.Kind) typ.Type {
	switch k {
	case kind.Nil:
		return typ.Nil
	case kind.Boolean:
		return typ.Boolean
	case kind.Number:
		return typ.Number
	case kind.Integer:
		return typ.Integer
	case kind.String:
		return typ.String
	case kind.Function:
		return typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()
	case kind.Record:
		return tableTopType
	case kind.Any:
		return typ.Any
	default:
		return typ.Unknown
	}
}

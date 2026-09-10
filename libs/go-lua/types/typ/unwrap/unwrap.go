// Package unwrap provides type unwrapping, extraction, and predicate operations.
//
// These are pure operations on types that do not depend on subtype checking.
// For operations requiring subtype checking, see types/query/core.
package unwrap

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// Underlying returns the underlying type by unwrapping transparent wrappers.
// Unwraps: Alias, Optional (to inner type).
// Does NOT unwrap: Instantiated (requires type substitution), Union, Ref.
func Underlying(t typ.Type) typ.Type {
	return underlyingDepth(t, typ.NewGuard())
}

func underlyingDepth(t typ.Type, guard internal.RecursionGuard) typ.Type {
	return typ.VisitWithGuard(t, guard, nil, func(next internal.RecursionGuard) typ.Visitor[typ.Type] {
		return typ.Visitor[typ.Type]{
			Alias: func(a *typ.Alias) typ.Type {
				return underlyingDepth(a.UnaliasedTarget(), next)
			},
			Optional: func(o *typ.Optional) typ.Type {
				return underlyingDepth(o.Inner, next)
			},
			Default: func(t typ.Type) typ.Type {
				return t
			},
		}
	})
}

// Alias unwraps only Alias wrappers, preserving Optional.
func Alias(t typ.Type) typ.Type {
	return unwrapAliasDepth(t, typ.NewGuard())
}

func unwrapAliasDepth(t typ.Type, guard internal.RecursionGuard) typ.Type {
	return typ.VisitWithGuard(t, guard, nil, func(next internal.RecursionGuard) typ.Visitor[typ.Type] {
		return typ.Visitor[typ.Type]{
			Alias: func(a *typ.Alias) typ.Type {
				return unwrapAliasDepth(a.UnaliasedTarget(), next)
			},
			Default: func(t typ.Type) typ.Type {
				return t
			},
		}
	})
}

// Optional unwraps Optional to get the inner non-nil type.
// Also unwraps Alias. Returns nil if type is nil or Nil.
func Optional(t typ.Type) typ.Type {
	return unwrapOptionalDepth(t, typ.NewGuard())
}

func unwrapOptionalDepth(t typ.Type, guard internal.RecursionGuard) typ.Type {
	return typ.VisitWithGuard(t, guard, nil, func(next internal.RecursionGuard) typ.Visitor[typ.Type] {
		return typ.Visitor[typ.Type]{
			Alias: func(a *typ.Alias) typ.Type {
				return unwrapOptionalDepth(a.UnaliasedTarget(), next)
			},
			Optional: func(o *typ.Optional) typ.Type {
				return unwrapOptionalDepth(o.Inner, next)
			},
			Default: func(t typ.Type) typ.Type {
				return t
			},
		}
	})
}

// IsOptionalLike returns true if the type is Optional or contains nil.
func IsOptionalLike(t typ.Type) bool {
	if t == nil {
		return true
	}

	u := Alias(t)
	if u == nil {
		return true
	}
	return typ.Visit(u, typ.Visitor[bool]{
		Optional: func(*typ.Optional) bool {
			return true
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if IsOptionalLike(m) {
					return true
				}
			}

			return false
		},
		Default: func(t typ.Type) bool {
			k := t.Kind()
			return k == kind.Nil || k.IsPlaceholder()
		},
	})
}

// IsSingleton returns true if the type represents exactly one value (nil or a literal).
func IsSingleton(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = Alias(t)
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Literal:
		return true
	}
	return false
}

// IsEmptyRecord returns true if t is a record with no fields and no map component.
func IsEmptyRecord(t typ.Type) bool {
	if t == nil {
		return false
	}
	rec, ok := t.(*typ.Record)
	return ok && len(rec.Fields) == 0 && !rec.HasMapComponent()
}

// IsContainer returns true if t is an array, map, tuple, or record.
func IsContainer(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Array, kind.Map, kind.Tuple, kind.Record:
		return true
	}
	return false
}

// IsBuiltinTableTop reports whether t is the builtin "table" top marker type.
//
// The checker models `table` as an interface named "table" with no methods.
// This marker means "some Lua table shape", not a closed interface.
func IsBuiltinTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = Alias(t)
	if t == nil {
		return false
	}
	iface, ok := t.(*typ.Interface)
	return ok && iface.Name == "table" && len(iface.Methods) == 0
}

// Function extracts a Function type, unwrapping Alias and Optional.
func Function(t typ.Type) *typ.Function {
	return unwrapFunctionDepth(t, typ.NewGuard())
}

func unwrapFunctionDepth(t typ.Type, guard internal.RecursionGuard) *typ.Function {
	return typ.VisitWithGuard(t, guard, nil, func(next internal.RecursionGuard) typ.Visitor[*typ.Function] {
		return typ.Visitor[*typ.Function]{
			Function: func(fn *typ.Function) *typ.Function {
				return fn
			},
			Optional: func(o *typ.Optional) *typ.Function {
				return unwrapFunctionDepth(o.Inner, next)
			},
			Recursive: func(rec *typ.Recursive) *typ.Function {
				if rec.Body == nil || rec.Body == rec {
					return nil
				}
				return unwrapFunctionDepth(rec.Body, next)
			},
			Alias: func(a *typ.Alias) *typ.Function {
				return unwrapFunctionDepth(a.UnaliasedTarget(), next)
			},
			Default: func(t typ.Type) *typ.Function {
				return nil
			},
		}
	})
}

// Record extracts a Record type, unwrapping Alias and Optional.
func Record(t typ.Type) *typ.Record {
	return unwrapRecordDepth(t, typ.NewGuard())
}

func unwrapRecordDepth(t typ.Type, guard internal.RecursionGuard) *typ.Record {
	return typ.VisitWithGuard(t, guard, nil, func(next internal.RecursionGuard) typ.Visitor[*typ.Record] {
		return typ.Visitor[*typ.Record]{
			Record: func(rec *typ.Record) *typ.Record {
				return rec
			},
			Recursive: func(rec *typ.Recursive) *typ.Record {
				if rec.Body == nil || rec.Body == rec {
					return nil
				}
				return unwrapRecordDepth(rec.Body, next)
			},
			Alias: func(a *typ.Alias) *typ.Record {
				return unwrapRecordDepth(a.UnaliasedTarget(), next)
			},
			Optional: func(o *typ.Optional) *typ.Record {
				return unwrapRecordDepth(o.Inner, next)
			},
			Instantiated: func(inst *typ.Instantiated) *typ.Record {
				expanded := subst.ExpandInstantiated(inst)
				if expanded == nil || expanded == t {
					return nil
				}
				return unwrapRecordDepth(expanded, next)
			},
			Default: func(t typ.Type) *typ.Record {
				return nil
			},
		}
	})
}

// Union extracts a Union type, unwrapping Alias and Optional.
func Union(t typ.Type) *typ.Union {
	return unwrapUnionDepth(t, typ.NewGuard())
}

func unwrapUnionDepth(t typ.Type, guard internal.RecursionGuard) *typ.Union {
	return typ.VisitWithGuard(t, guard, nil, func(next internal.RecursionGuard) typ.Visitor[*typ.Union] {
		return typ.Visitor[*typ.Union]{
			Union: func(u *typ.Union) *typ.Union {
				return u
			},
			Recursive: func(rec *typ.Recursive) *typ.Union {
				if rec.Body == nil || rec.Body == rec {
					return nil
				}
				return unwrapUnionDepth(rec.Body, next)
			},
			Alias: func(a *typ.Alias) *typ.Union {
				return unwrapUnionDepth(a.UnaliasedTarget(), next)
			},
			Optional: func(o *typ.Optional) *typ.Union {
				return unwrapUnionDepth(o.Inner, next)
			},
			Instantiated: func(inst *typ.Instantiated) *typ.Union {
				expanded := subst.ExpandInstantiated(inst)
				if expanded == nil || expanded == t {
					return nil
				}
				return unwrapUnionDepth(expanded, next)
			},
			Default: func(t typ.Type) *typ.Union {
				return nil
			},
		}
	})
}

// IsLiteralString returns true if the type is a string literal.
func IsLiteralString(t typ.Type) bool {
	unwrapped := Alias(t)
	if lit, ok := unwrapped.(*typ.Literal); ok {
		_, isStr := lit.Value.(string)
		return isStr
	}
	return false
}

// ToKind unwraps a type until the requested kind is found.
// Follows aliases to find the underlying type of the specified kind.
// Returns nil if the requested kind is not found or if aliases form a cycle.
func ToKind(t typ.Type, k kind.Kind) typ.Type {
	return unwrapToKindDepth(t, k, typ.NewGuard())
}

func unwrapToKindDepth(t typ.Type, k kind.Kind, guard internal.RecursionGuard) typ.Type {
	if t == nil {
		return nil
	}
	if t.Kind() == k {
		return t
	}
	return typ.VisitWithGuard(t, guard, nil, func(next internal.RecursionGuard) typ.Visitor[typ.Type] {
		return typ.Visitor[typ.Type]{
			Recursive: func(rec *typ.Recursive) typ.Type {
				if rec.Body == nil || rec.Body == rec {
					return nil
				}
				return unwrapToKindDepth(rec.Body, k, next)
			},
			Alias: func(a *typ.Alias) typ.Type {
				return unwrapToKindDepth(a.UnaliasedTarget(), k, next)
			},
			Default: func(t typ.Type) typ.Type {
				return nil
			},
		}
	})
}

// IsNilType returns true if the type is exactly nil.
func IsNilType(t typ.Type) bool {
	return t != nil && t.Kind() == kind.Nil
}

// Instantiated expands an Instantiated type to its structural form.
// Returns the expanded type if t is an Instantiated and expansion produces
// a different type. Returns t unchanged otherwise.
func Instantiated(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	inst, ok := t.(*typ.Instantiated)
	if !ok {
		return t
	}
	expanded := subst.ExpandInstantiated(inst)
	if expanded == nil || expanded == t {
		return t
	}
	return expanded
}

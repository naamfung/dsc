package ops

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// CanBeFalsy reports whether a type can represent a falsy value (nil or false).
//
// In Lua, only two values are falsy: nil and false. All other values
// (including 0, empty string, empty table) are truthy.
//
// Type analysis:
//   - Optional types: Always falsy (can be nil)
//   - Union types: Falsy if ANY member can be falsy
//   - Intersection types: Falsy only if ALL members can be falsy
//   - Literal false: Falsy
//   - Boolean type: Falsy (could be false)
//   - Nil type: Falsy
//   - Arrays, maps, records, functions: Never falsy
//   - Placeholder types: Conservatively falsy
func CanBeFalsy(t typ.Type) bool {
	return canBeFalsyGuard(t, typ.NewGuard())
}

func canBeFalsyGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Optional:
		return true

	case *typ.Union:
		for _, m := range v.Members {
			if canBeFalsyGuard(m, next) {
				return true
			}
		}

		return false

	case *typ.Intersection:
		// Intersection is falsy only if ALL members can be falsy
		for _, m := range v.Members {
			if !canBeFalsyGuard(m, next) {
				return false
			}
		}

		return true

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return canBeFalsyGuard(v.Target, next)

	case *typ.Literal:
		if b, ok := v.Value.(bool); ok && !b {
			return true
		}

		return false

	case *typ.TypeParam:
		if v.Constraint != nil {
			return canBeFalsyGuard(v.Constraint, next)
		}

		return true // unconstrained type param can be any type including nil

	case *typ.Array, *typ.Map, *typ.Record, *typ.Tuple, *typ.Function:
		// These types are always truthy
		return false

	default:
		k := t.Kind()
		return k == kind.Nil || k == kind.Boolean || k.IsPlaceholder()
	}
}

// IsFalsy reports whether a type is definitely falsy (always nil or false).
//
// Returns true only for:
//   - typ.Nil
//   - Literal false
//
// For types that may or may not be falsy (like boolean or optional),
// returns false. Use CanBeFalsy for that check.
func IsFalsy(t typ.Type) bool {
	if t == nil {
		return false
	}

	if t.Kind() == kind.Nil {
		return true
	}

	if lit, ok := t.(*typ.Literal); ok {
		if b, isBool := lit.Value.(bool); isBool && !b {
			return true
		}
	}

	return false
}

// IsTruthy reports whether a type is definitely truthy (can never be nil or false).
//
// Returns true for types like string, number, tables, functions, etc.
// Returns false for optional types, boolean, nil, and types that could be falsy.
func IsTruthy(t typ.Type) bool {
	return t != nil && !CanBeFalsy(t)
}

// ExtractFirstValue returns the first element if t is a Tuple,
// otherwise returns t unchanged. In Lua, when a multi-return function
// is used in a context expecting single value, only the first is used.
func ExtractFirstValue(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}

	if tuple, ok := t.(*typ.Tuple); ok && len(tuple.Elements) > 0 {
		return tuple.Elements[0]
	}

	return t
}

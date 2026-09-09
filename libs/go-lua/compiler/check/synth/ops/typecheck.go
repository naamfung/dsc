package ops

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// IsNumeric checks if a type supports arithmetic operations (+, -, *, /, %).
//
// A type is numeric if:
//   - It has kind Number or Integer
//   - It's a literal number (int64 or float64)
//   - It's a union/intersection where all members are numeric
//   - It's an alias to a numeric type
//   - It's a type parameter with a numeric constraint
//
// Optional types are NOT numeric - they must be narrowed first.
// Placeholder types (any, unknown) are considered numeric for flexibility.
func IsNumeric(t typ.Type) bool {
	return isNumericGuard(t, typ.NewGuard())
}

func isNumericGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if !isNumericGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !isNumericGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return isNumericGuard(v.Target, next)

	case *typ.Optional:
		// Optional types are NOT numeric - must be narrowed first
		return false

	case *typ.Literal:
		switch v.Value.(type) {
		case float64, int64:
			return true
		default:
			return false
		}

	case *typ.TypeParam:
		if v.Constraint != nil {
			return isNumericGuard(v.Constraint, next)
		}

		return false

	default:
		k := t.Kind()
		return k == kind.Number || k == kind.Integer || k.IsPlaceholder()
	}
}

// IsOrderable checks if a type supports comparison operators (<, <=, >, >=).
//
// A type is orderable if:
//   - It has kind Number, Integer, or String
//   - It's a literal number or string
//   - It's a union/intersection where all members are orderable
//   - It's an alias to an orderable type
//
// Notably, booleans and tables are NOT orderable in Lua.
func IsOrderable(t typ.Type) bool {
	return isOrderableGuard(t, typ.NewGuard())
}

func isOrderableGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if !isOrderableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !isOrderableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return isOrderableGuard(v.Target, next)

	case *typ.Optional:
		return false

	case *typ.Literal:
		switch v.Value.(type) {
		case float64, int64, string:
			return true
		default:
			return false
		}

	case *typ.TypeParam:
		if v.Constraint != nil {
			return isOrderableGuard(v.Constraint, next)
		}

		return false

	default:
		k := t.Kind()
		return k == kind.Number || k == kind.Integer || k == kind.String || k.IsPlaceholder()
	}
}

// MayBeOrderable checks whether a type could support ordering operators
// (<, <=, >, >=). Unlike IsOrderable (all branches), this is conservative:
// true if any feasible branch is orderable.
func MayBeOrderable(t typ.Type) bool {
	return mayBeOrderableGuard(t, typ.NewGuard())
}

func mayBeOrderableGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if mayBeOrderableGuard(m, next) {
				return true
			}
		}
		return false

	case *typ.Intersection:
		for _, m := range v.Members {
			if mayBeOrderableGuard(m, next) {
				return true
			}
		}
		return false

	case *typ.Alias:
		if v.Target == nil {
			return false
		}
		return mayBeOrderableGuard(v.Target, next)

	case *typ.Optional:
		if v.Inner == nil {
			return false
		}
		return mayBeOrderableGuard(v.Inner, next)

	case *typ.Literal:
		switch v.Value.(type) {
		case string, float64, int64:
			return true
		default:
			return false
		}

	case *typ.TypeParam:
		if v.Constraint != nil {
			return mayBeOrderableGuard(v.Constraint, next)
		}
		return false
	}

	k := t.Kind()
	return k == kind.String || k == kind.Number || k == kind.Integer || k.IsPlaceholder()
}

// IsStringable checks if a type can be used with string concatenation (..).
//
// In Lua, both strings and numbers can be concatenated (numbers are coerced).
// Types implementing Error interface or with __tostring metamethod also work.
//
// A type is stringable if:
//   - It has kind String, Number, or Integer
//   - It's the Error interface type
//   - It's a subtype of string (has __tostring)
//   - It's a literal string or number
//   - It's a union/intersection where all members are stringable
func IsStringable(t typ.Type) bool {
	return isStringableGuard(t, typ.NewGuard())
}

func isStringableGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}

	t = ExtractFirstValue(t)
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}
	if t != nil && t.Equals(typ.LuaError) {
		return true
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if !isStringableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !isStringableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return isStringableGuard(v.Target, next)

	case *typ.Optional:
		return false

	case *typ.Literal:
		switch v.Value.(type) {
		case string, float64, int64:
			return true
		default:
			return false
		}

	case *typ.TypeParam:
		if v.Constraint != nil {
			return isStringableGuard(v.Constraint, next)
		}

		return false
	case *typ.Interface:
		if v.Name == "Error" {
			return true
		}

		return false

	default:
		k := t.Kind()
		if k.IsPlaceholder() {
			return true
		}

		if k == kind.String || k == kind.Number || k == kind.Integer {
			return true
		}
		// Types with __tostring metamethod are subtypes of string
		return subtype.IsSubtype(t, typ.String)
	}
}

// MayBeStringable checks whether a type could participate in string
// concatenation. Unlike IsStringable (which requires all branches to be
// stringable), this returns true if any feasible branch is stringable.
func MayBeStringable(t typ.Type) bool {
	return mayBeStringableGuard(t, typ.NewGuard())
}

func mayBeStringableGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if mayBeStringableGuard(m, next) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, m := range v.Members {
			if mayBeStringableGuard(m, next) {
				return true
			}
		}
		return false
	case *typ.Alias:
		if v.Target == nil {
			return false
		}
		return mayBeStringableGuard(v.Target, next)
	case *typ.Optional:
		if v.Inner == nil {
			return false
		}
		return mayBeStringableGuard(v.Inner, next)
	case *typ.TypeParam:
		if v.Constraint != nil {
			return mayBeStringableGuard(v.Constraint, next)
		}
		return false
	case *typ.Interface:
		return v.Name == "Error"
	case *typ.Literal:
		switch v.Value.(type) {
		case string, float64, int64:
			return true
		default:
			return false
		}
	default:
		k := t.Kind()
		if k.IsPlaceholder() {
			return true
		}
		if k == kind.String || k == kind.Number || k == kind.Integer {
			return true
		}
		return subtype.IsSubtype(t, typ.String)
	}
}

// HasLength checks if a type supports the length operator (#).
//
// In Lua, the following types have length:
//   - Strings (byte count)
//   - Arrays (element count)
//   - Tables/records (via __len metamethod or table length)
//   - Tuples (element count)
//   - Maps (entry count)
func HasLength(t typ.Type) bool {
	return hasLengthGuard(t, typ.NewGuard())
}

func hasLengthGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(t) {
		return true
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Array, *typ.Map, *typ.Record, *typ.Tuple:
		return true

	case *typ.Union:
		for _, m := range v.Members {
			if !hasLengthGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !hasLengthGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return hasLengthGuard(v.Target, next)

	case *typ.Optional:
		return false

	case *typ.Literal:
		_, isStr := v.Value.(string)
		return isStr

	case *typ.TypeParam:
		if v.Constraint != nil {
			return hasLengthGuard(v.Constraint, next)
		}

		return false

	default:
		k := t.Kind()
		if k.IsPlaceholder() {
			return true
		}

		return k == kind.String
	}
}

// MayHaveLength checks whether a type could support the length operator (#).
//
// This is a conservative predicate used by diagnostics: it returns true when
// any feasible runtime value may have length, avoiding false positives for
// optional/union values that can be length-capable after control-flow guards.
func MayHaveLength(t typ.Type) bool {
	return mayHaveLengthGuard(t, typ.NewGuard())
}

func mayHaveLengthGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(t) {
		return true
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Array, *typ.Map, *typ.Record, *typ.Tuple:
		return true

	case *typ.Union:
		for _, m := range v.Members {
			if mayHaveLengthGuard(m, next) {
				return true
			}
		}
		return false

	case *typ.Intersection:
		for _, m := range v.Members {
			if mayHaveLengthGuard(m, next) {
				return true
			}
		}
		return false

	case *typ.Alias:
		if v.Target == nil {
			return false
		}
		return mayHaveLengthGuard(v.Target, next)

	case *typ.Optional:
		if v.Inner == nil {
			return false
		}
		return mayHaveLengthGuard(v.Inner, next)

	case *typ.Literal:
		_, isStr := v.Value.(string)
		return isStr

	case *typ.TypeParam:
		if v.Constraint != nil {
			return mayHaveLengthGuard(v.Constraint, next)
		}
		return false
	}

	return t.Kind().IsPlaceholder() || t.Kind() == kind.String
}

// IsStringOnly checks if type is string (not number).
func IsStringOnly(t typ.Type) bool {
	if t == nil {
		return false
	}

	if lit, ok := t.(*typ.Literal); ok {
		_, isStr := lit.Value.(string)
		return isStr
	}

	return t.Kind() == kind.String
}

// IsBitwiseNumeric checks if a type supports bitwise operators (&, |, ~, <<, >>).
//
// Only integer and number types (and placeholders) support bitwise operations.
// Optional types are rejected until narrowed.
func IsBitwiseNumeric(t typ.Type) bool {
	return isBitwiseNumericGuard(t, typ.NewGuard())
}

func isBitwiseNumericGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if !isBitwiseNumericGuard(m, next) {
				return false
			}
		}
		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !isBitwiseNumericGuard(m, next) {
				return false
			}
		}
		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}
		return isBitwiseNumericGuard(v.Target, next)

	case *typ.Optional:
		return false

	case *typ.Literal:
		return v.Base == kind.Integer || v.Base == kind.Number
	}

	k := t.Kind()
	return k == kind.Integer || k == kind.Number || k.IsPlaceholder()
}

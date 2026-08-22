// Package transform provides return type transforms owned by the compiler layer.
// This package applies spec and effect transforms to synthesized return types.
package transform

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ApplySpecReturnCases evaluates contract.ReturnSpec cases against argument types.
// This is pure type-based matching that works when argument types are resolved
// to literal types. The compiler uses this as a fallback when AST-pattern
// matching (for inline table constructors) doesn't produce a result.
//
// Ownership: This function provides the pure type-based logic. The compiler
// owns the decision of when to apply spec returns and coordinates between
// AST-pattern matching and type-based matching.
func ApplySpecReturnCases(fn *typ.Function, args []typ.Type) typ.Type {
	if fn == nil || fn.Spec == nil {
		return nil
	}
	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil || spec.Return == nil {
		return nil
	}

	for _, rc := range spec.Return.Cases {
		if specReturnCaseMatchesTypes(rc.When, args) {
			return rc.Type
		}
	}

	if spec.Return.Default != nil {
		return spec.Return.Default
	}
	return nil
}

func specReturnCaseMatchesTypes(when constraint.Condition, args []typ.Type) bool {
	return conditionAnyDisjunctMatches(when, func(c constraint.Constraint) bool {
		return specConstraintMatchesTypes(c, args)
	})
}

func specConstraintMatchesTypes(c constraint.Constraint, args []typ.Type) bool {
	switch v := c.(type) {
	case constraint.FieldEquals:
		return specFieldEqualsMatchesTypes(v, args)
	default:
		return false
	}
}

func specFieldEqualsMatchesTypes(fe constraint.FieldEquals, args []typ.Type) bool {
	idx, ok := constraint.PlaceholderArgIndex(fe.Target, len(args))
	if !ok {
		return false
	}
	return typeFieldMatchesLiteral(args[idx], fe.Field, fe.Value)
}

func typeFieldMatchesLiteral(t typ.Type, field string, lit *typ.Literal) bool {
	if t == nil || lit == nil || field == "" {
		return false
	}

	switch v := unwrap.Alias(t).(type) {
	case *typ.Record:
		f := v.GetField(field)
		if f == nil || f.Type == nil {
			return false
		}
		if lf, ok := f.Type.(*typ.Literal); ok {
			return typ.TypeEquals(lf, lit)
		}
		return false
	case *typ.Optional:
		return false
	case *typ.Union:
		for _, m := range v.Members {
			if !typeFieldMatchesLiteral(m, field, lit) {
				return false
			}
		}
		return len(v.Members) > 0
	default:
		return false
	}
}

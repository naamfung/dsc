package ops

import (
	"fmt"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// InferTypeArgsWithExpectedAndMode performs bidirectional inference with optional
// forced receiver consumption for method calls.
func InferTypeArgsWithExpectedAndMode(fn *typ.Function, args []typ.Type, isMethod bool, receiver typ.Type, expectedReturn typ.Type, forceMethodReceiver bool) ([]typ.Type, error) {
	if fn == nil || len(fn.TypeParams) == 0 {
		return nil, nil
	}

	typeVars := make(map[string]*typ.TypeVar)
	for i, tp := range fn.TypeParams {
		typeVars[tp.Name] = typ.NewTypeVar(i + 1)
	}

	paramTypes := make([]typ.Type, len(fn.Params))
	for i, p := range fn.Params {
		paramTypes[i] = SubstituteTypeVars(p.Type, typeVars)
	}

	cs := constraint.NewInferSet()

	inputs := args
	consumeReceiver := methodConsumesReceiverSimple(fn, receiver, isMethod, forceMethodReceiver)
	if consumeReceiver {
		inputs = make([]typ.Type, 0, len(args)+1)
		inputs = append(inputs, receiver)
		inputs = append(inputs, args...)
	}

	for i, arg := range inputs {
		paramIdx := i

		var expected typ.Type

		if paramIdx < len(paramTypes) {
			expected = paramTypes[paramIdx]
		} else if fn.Variadic != nil {
			expected = SubstituteTypeVars(fn.Variadic, typeVars)
		} else {
			break
		}
		// Expand Instantiated types for structural matching
		expected = subst.ExpandInstantiated(expected)
		arg = subst.ExpandInstantiated(arg)
		arg = normalizeArgForGenericInference(expected, arg)
		constraint.MatchContra(expected, arg, cs)
	}

	// Match expected return type against function return type for bidirectional inference.
	// This enables inferring T in `get<T>(): T?` from `local x: string? = get()`.
	// Skip if expected is nil, unknown, or any - these provide no useful constraints.
	// For union expected types, distribute matching over each member.
	if expectedReturn != nil && len(fn.Returns) > 0 {
		expKind := expectedReturn.Kind()
		if !expKind.IsPlaceholder() {
			returnType := SubstituteTypeVars(fn.Returns[0], typeVars)
			returnType = subst.ExpandInstantiated(returnType)

			// Handle union expected types by matching against each member
			if union, ok := unwrap.Alias(expectedReturn).(*typ.Union); ok {
				for _, member := range union.Members {
					member = unwrap.Alias(member)
					member = subst.ExpandInstantiated(member)
					constraint.MatchCo(returnType, member, cs)
				}
			} else {
				// Unwrap alias to get structural type for matching
				expectedReturn = unwrap.Alias(expectedReturn)
				expectedReturn = subst.ExpandInstantiated(expectedReturn)
				// Match covariant: return <: expected (function produces subtype of what's expected)
				constraint.MatchCo(returnType, expectedReturn, cs)
			}
		}
	}

	solution, err := cs.Solve()
	if err != nil {
		return nil, fmt.Errorf("infer: constraint solve failed: %w", err)
	}

	result := make([]typ.Type, len(fn.TypeParams))

	for i, tp := range fn.TypeParams {
		tv := typeVars[tp.Name]
		if solved, ok := solution[tv.ID]; ok && solved != nil {
			result[i] = solved
		} else {
			result[i] = typ.Unknown
		}
	}

	// Validate that inferred type arguments satisfy their constraints
	for i, tp := range fn.TypeParams {
		if tp.Constraint != nil && !typ.IsAbsentOrUnknown(result[i]) {
			if !subtype.IsSubtype(result[i], tp.Constraint) {
				return nil, fmt.Errorf("infer: type argument %s does not satisfy constraint %s", result[i], tp.Constraint)
			}
		}
	}

	return result, nil
}

func normalizeArgForGenericInference(expected, arg typ.Type) typ.Type {
	if expected == nil || arg == nil {
		return arg
	}
	if _, ok := unwrap.Alias(expected).(*typ.Array); !ok {
		return arg
	}

	joinElems := func(elems []typ.Type) typ.Type {
		var joined typ.Type
		for _, elem := range elems {
			if elem == nil {
				continue
			}
			if joined == nil {
				joined = elem
			} else {
				joined = typ.JoinPreferNonSoft(joined, elem)
			}
		}
		return joined
	}

	var collectElems func(t typ.Type, out *[]typ.Type) bool
	collectElems = func(t typ.Type, out *[]typ.Type) bool {
		if t == nil {
			return false
		}
		switch v := unwrap.Alias(t).(type) {
		case *typ.Array:
			*out = append(*out, v.Element)
			return true
		case *typ.Tuple:
			if len(v.Elements) == 0 {
				return false
			}
			*out = append(*out, v.Elements...)
			return true
		case *typ.Union:
			collectedAny := false
			for _, m := range v.Members {
				if collectElems(m, out) {
					collectedAny = true
					continue
				}
				*out = append(*out, m)
				collectedAny = true
			}
			return collectedAny
		case *typ.Literal:
			*out = append(*out, v)
			return true
		default:
			return false
		}
	}

	var elems []typ.Type
	if !collectElems(arg, &elems) {
		return arg
	}
	elemType := joinElems(elems)
	if elemType == nil {
		return arg
	}
	return typ.NewArray(elemType)
}

// SubstituteTypeVars replaces TypeParam with TypeVar in a type.
func SubstituteTypeVars(t typ.Type, vars map[string]*typ.TypeVar) typ.Type {
	subs := make(map[string]typ.Type, len(vars))
	for name, tv := range vars {
		subs[name] = tv
	}
	return subst.Substitute(t, subs)
}

// InstantiateFunction creates a concrete function type by substituting type arguments.
//
// Given a generic function and type arguments (one per type parameter),
// replaces all occurrences of type parameters with their corresponding
// type arguments throughout parameter types, return types, and constraints.
//
// Returns the original function if it's not generic or if typeArgs length
// doesn't match the number of type parameters.
func InstantiateFunction(fn *typ.Function, typeArgs []typ.Type) *typ.Function {
	if fn == nil || len(fn.TypeParams) == 0 {
		return fn
	}

	if len(typeArgs) != len(fn.TypeParams) {
		return fn
	}

	result := subst.Params(fn, fn.TypeParams, typeArgs)
	if f, ok := result.(*typ.Function); ok {
		return f
	}

	return fn
}

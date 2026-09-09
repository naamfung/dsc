package transform

import (
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ApplyEffectTransform applies return type effects to compute the actual return type.
// If the function has a contract.Spec with a Return effect, the transform is applied
// to derive the concrete return type from the argument types.
func ApplyEffectTransform(fn *typ.Function, args []typ.Type, returnIdx int, baseReturn typ.Type) typ.Type {
	if fn == nil || fn.Spec == nil {
		return baseReturn
	}

	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return baseReturn
	}

	// Error-return pattern: value is optional when an error return is present.
	if er := spec.Effects.GetErrorReturn(returnIdx); er != nil {
		if !unwrap.IsOptionalLike(baseReturn) {
			return typ.NewOptional(baseReturn)
		}
	}

	ret := spec.Effects.GetReturn(returnIdx)
	if ret == nil || ret.Transform == nil {
		return baseReturn
	}

	switch transform := ret.Transform.(type) {
	case effect.SameAs:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			return resolved
		}
		return baseReturn
	case effect.ElementOf:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			if elem := querycore.ElementType(resolved); elem != nil {
				return elem
			}
		}
		return baseReturn
	case effect.OptionalElementOf:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			if elem := querycore.ElementType(resolved); elem != nil {
				return typ.NewOptional(elem)
			}
		}
		return baseReturn
	case effect.DeepElementOf:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			if elem := deepElementType(resolved); elem != nil {
				return elem
			}
		}
		return baseReturn
	case effect.StringUnpackValue:
		if resolved := resolveParamType(args, transform.Format); resolved != nil {
			if unpacked := unpackFirstValueType(resolved); unpacked != nil {
				return unpacked
			}
		}
		return baseReturn
	case effect.CallbackReturn:
		if resolved := resolveParamType(args, transform.CallbackParam); resolved != nil {
			if cbRet := callbackReturnType(resolved); cbRet != nil {
				return cbRet
			}
		}
		return baseReturn
	case effect.ArrayOfCallbackReturn:
		if resolved := resolveParamType(args, transform.CallbackParam); resolved != nil {
			if cbRet := callbackReturnType(resolved); cbRet != nil {
				return typ.NewArray(cbRet)
			}
		}
		return baseReturn
	case effect.SelectResultOfCases:
		result := buildSelectResultUnion(args, transform)
		if result != nil {
			return result
		}
		return baseReturn
	default:
		return baseReturn
	}
}

func resolveParamType(args []typ.Type, ref effect.ParamRef) typ.Type {
	idx, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok || idx < 0 || idx >= len(args) {
		return nil
	}
	return args[idx]
}

func callbackReturnType(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Function:
		if len(v.Returns) == 0 || v.Returns[0] == nil {
			return nil
		}
		return v.Returns[0]
	case *typ.Optional:
		return callbackReturnType(v.Inner)
	case *typ.Union:
		var members []typ.Type
		for _, m := range v.Members {
			if rt := callbackReturnType(m); rt != nil {
				members = append(members, rt)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	default:
		if typ.IsAny(t) {
			return typ.Any
		}
		if typ.IsUnknown(t) {
			return typ.Unknown
		}
		return nil
	}
}

func deepElementType(t typ.Type) typ.Type {
	current := t
	last := querycore.ElementType(current)
	for last != nil {
		current = last
		next := querycore.ElementType(current)
		if next == nil {
			return current
		}
		last = next
	}
	return nil
}

// buildSelectResultUnion builds a union of result records from SelectCase elements.
func buildSelectResultUnion(args []typ.Type, transform effect.SelectResultOfCases) typ.Type {
	casesIdx, ok := effect.ResolveParamIndex(transform.Cases, len(args))
	if !ok {
		return nil
	}

	casesArg := args[casesIdx]
	if casesArg == nil {
		return nil
	}

	caseTypes := extractSelectCaseElements(casesArg)
	if len(caseTypes) == 0 {
		return nil
	}

	addDefault := selectDefaultPossible(args, casesArg, transform)

	var resultTypes []typ.Type
	seen := make(map[uint64]bool)

	for caseIdx, caseType := range caseTypes {
		channelType, valueType := extractSelectCaseParts(caseType)
		if channelType == nil {
			// Keep unknown/any case elements conservative; skip concrete non-case fields.
			if !typ.IsAny(caseType) && !typ.IsUnknown(caseType) {
				continue
			}
			channelType = typ.Any
			valueType = typ.Any
		}

		builder := typ.NewRecord().
			Field("channel", channelType).
			Field("ok", typ.Boolean).
			Field("value", valueType).
			// Preserve case multiplicity even when channel/value types are equal.
			// This keeps identity-sensitive narrowing sound for `result.channel ~= ch`.
			Field("__select_case_id", typ.LiteralInt(int64(caseIdx)))

		if addDefault {
			builder = builder.OptField("default", typ.Boolean)
		}

		resultRecord := builder.Build()

		h := resultRecord.Hash()
		if !seen[h] {
			seen[h] = true
			resultTypes = append(resultTypes, resultRecord)
		}
	}

	if len(resultTypes) == 0 {
		return nil
	}

	if len(resultTypes) == 1 {
		return resultTypes[0]
	}

	return typ.NewUnion(resultTypes...)
}

func selectDefaultPossible(args []typ.Type, casesArg typ.Type, transform effect.SelectResultOfCases) bool {
	if idx, ok := effect.ResolveParamIndex(transform.Default, len(args)); ok {
		if defaultArgMayEnableSelectDefault(args[idx]) {
			return true
		}
	}
	return casesArgHasDefaultFlag(casesArg)
}

func defaultArgMayEnableSelectDefault(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}

	switch v := t.(type) {
	case *typ.Optional:
		return defaultArgMayEnableSelectDefault(v.Inner)
	case *typ.Union:
		for _, m := range v.Members {
			if defaultArgMayEnableSelectDefault(m) {
				return true
			}
		}
		return false
	case *typ.Literal:
		if v.Base == kind.Boolean {
			b, _ := v.Value.(bool)
			return b
		}
		return false
	default:
		return t.Kind() == kind.Boolean || typ.IsAny(t) || typ.IsUnknown(t)
	}
}

func casesArgHasDefaultFlag(casesArg typ.Type) bool {
	casesArg = unwrap.Alias(casesArg)
	if casesArg == nil {
		return false
	}

	switch v := casesArg.(type) {
	case *typ.Optional:
		return casesArgHasDefaultFlag(v.Inner)
	case *typ.Union:
		for _, m := range v.Members {
			if casesArgHasDefaultFlag(m) {
				return true
			}
		}
		return false
	case *typ.Record:
		if field := v.GetField("default"); field != nil {
			return defaultArgMayEnableSelectDefault(field.Type)
		}
		// Conservative map-component handling: {[string]: boolean}-style cases tables
		// can carry a "default" flag even when not modeled as a named field.
		if v.MapKey == nil || v.MapValue == nil {
			return false
		}
		return typ.TypeMatchesLiteral(v.MapKey, typ.LiteralString("default")) &&
			defaultArgMayEnableSelectDefault(v.MapValue)
	default:
		return false
	}
}

// extractSelectCaseElements extracts individual SelectCase types from a cases argument.
func extractSelectCaseElements(casesArg typ.Type) []typ.Type {
	if casesArg == nil {
		return nil
	}

	switch v := casesArg.(type) {
	case *typ.Tuple:
		result := make([]typ.Type, len(v.Elements))
		copy(result, v.Elements)
		return result
	case *typ.Record:
		if len(v.Fields) == 0 {
			return nil
		}
		result := make([]typ.Type, 0, len(v.Fields))
		for _, f := range v.Fields {
			if f.Name == "default" {
				continue
			}
			if f.Type != nil {
				result = append(result, f.Type)
			}
		}
		return result
	case *typ.Array:
		if v.Element != nil {
			if u, ok := v.Element.(*typ.Union); ok {
				result := make([]typ.Type, len(u.Members))
				copy(result, u.Members)
				return result
			}
			return []typ.Type{v.Element}
		}
	case *typ.Union:
		result := make([]typ.Type, len(v.Members))
		copy(result, v.Members)
		return result
	}

	return []typ.Type{casesArg}
}

// extractSelectCaseParts extracts the channel and value types from a SelectCase<Ch, T>.
func extractSelectCaseParts(caseType typ.Type) (channel typ.Type, value typ.Type) {
	if caseType == nil {
		return nil, typ.Unknown
	}

	inst, ok := caseType.(*typ.Instantiated)
	if !ok || inst.Generic == nil {
		return nil, typ.Unknown
	}

	if len(inst.TypeArgs) < 2 {
		return nil, typ.Unknown
	}

	return inst.TypeArgs[0], inst.TypeArgs[1]
}

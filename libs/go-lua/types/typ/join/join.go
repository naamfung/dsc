// Package join provides type join operations for control flow merging.
//
// Join computes the least upper bound (union) of types at merge points
// where multiple control flow paths converge.
package join

import "github.com/wippyai/go-lua/types/typ"

// ReturnVectors merges two multi-return type vectors at join points.
//
// Each position is joined independently using typ.JoinReturnSlot. Shorter vectors
// are padded with Nil since Lua returns nil for missing values.
func ReturnVectors(a, b []typ.Type) []typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	result := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi typ.Type
		if i < len(a) {
			ai = a[i]
		} else {
			ai = typ.Nil
		}
		if i < len(b) {
			bi = b[i]
		} else {
			bi = typ.Nil
		}
		result[i] = typ.JoinReturnSlot(ai, bi)
	}
	return result
}

// WithReturns returns a copy of sig with the given return types grafted on.
// nil entries in returns are normalized to Unknown.
// If sig is nil, returns nil.
func WithReturns(sig *typ.Function, returns []typ.Type) *typ.Function {
	if sig == nil {
		return nil
	}

	builder := typ.Func()
	for _, tp := range sig.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range sig.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if sig.Variadic != nil {
		builder = builder.Variadic(sig.Variadic)
	}

	normalized := make([]typ.Type, len(returns))
	copy(normalized, returns)
	for i, t := range normalized {
		if t == nil {
			normalized[i] = typ.Unknown
		}
	}
	builder = builder.Returns(normalized...)

	if sig.Effects != nil {
		builder = builder.Effects(sig.Effects)
	}
	if sig.Spec != nil {
		builder = builder.Spec(sig.Spec)
	}
	if sig.Refinement != nil {
		builder = builder.WithRefinement(sig.Refinement)
	}
	return builder.Build()
}

// WithReturnsOrUnknown returns a signature with return slots from `returns`,
// defaulting to a single unknown return when no summary is available.
//
// If sig carries only placeholder returns (unknown/nil entries), summary
// returns replace those placeholders.
func WithReturnsOrUnknown(sig *typ.Function, returns []typ.Type) *typ.Function {
	if sig == nil {
		return nil
	}
	if len(returns) == 0 {
		if len(sig.Returns) > 0 {
			return sig
		}
		return WithReturns(sig, []typ.Type{typ.Unknown})
	}
	if len(sig.Returns) == 0 || typ.IsUnknownOnlyOrEmpty(sig.Returns) {
		return WithReturns(sig, returns)
	}
	if len(sig.Returns) == len(returns) {
		hasPlaceholder := false
		for _, ret := range sig.Returns {
			if ret != nil && ret.Kind().IsPlaceholder() {
				hasPlaceholder = true
				break
			}
		}
		if hasPlaceholder {
			return WithReturns(sig, returns)
		}
		return sig
	}
	for i, ret := range sig.Returns {
		if i >= len(returns) {
			break
		}
		if ret != nil && ret.Kind().IsPlaceholder() {
			return WithReturns(sig, returns)
		}
	}
	return sig
}

package typ

import "github.com/wippyai/go-lua/internal"

// EffectInfo describes function effects (e.g., io, throw).
// Implemented by effect.Row.
type EffectInfo interface {
	internal.Equaler
	IsEffectInfo()
}

// SpecInfo describes function specifications (pre/post conditions).
// Implemented by *contract.Spec.
type SpecInfo interface {
	internal.Equaler
	IsSpecInfo()
}

// RefinementInfo describes type refinements from function calls.
// Implemented by *constraint.FunctionRefinement.
type RefinementInfo interface {
	internal.Equaler
	IsRefinementInfo()
}

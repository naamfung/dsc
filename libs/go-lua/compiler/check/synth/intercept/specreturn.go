package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// SpecReturnOverride handles contract-based return type specialization.
//
// Functions with contract specifications (contract.Spec) can declare conditional
// return types based on argument values. For example:
//
//	function format(options: {mode: "json" | "xml"}): JsonResult | XmlResult
//	  -- @spec return when options.mode == "json" -> JsonResult
//	  -- @spec return when options.mode == "xml" -> XmlResult
//
// This component provides AST-level pattern matching for such specifications,
// enabling the type checker to refine return types when inline literals are used:
//
//	local result = format({mode = "json"})  -- Inferred as JsonResult
//
// Two-tier approach:
// - Tier 1 (this): AST-pattern matching for inline table constructor literals
// - Tier 2 (transform.ApplySpecReturnCases): Type-based matching for variable arguments
//
// SpecReturnOverride computes spec-based return type overrides using AST-pattern matching.
//
// Ownership: The compiler owns spec return handling through a two-tier approach:
//   - Tier 1 (this): AST-pattern matching for inline table constructor literals
//   - Tier 2: Type-based matching via transform.ApplySpecReturnCases
//
// The synth/ops layer does not apply spec returns internally - all spec logic
// is coordinated here in the compiler. This allows
// the compiler to use AST inspection for cases where type inference hasn't
// yet resolved to literal types.
//
// AST-pattern matching can detect {field = "value"} inline literals directly,
// while type-based matching requires the field to have a literal type in the
// type system.
type SpecReturnOverride struct {
	Phase api.Phase
}

// Override computes a spec return type override for a call.
// Returns nil if no override applies.
func (s *SpecReturnOverride) Override(fnType typ.Type, args []ast.Expr) typ.Type {
	if fnType == nil {
		return nil
	}
	if s.Phase != api.PhaseScopeCompute && s.Phase != api.PhaseNarrowing {
		return nil
	}

	fn := ResolveSpecFunction(fnType)
	if fn == nil || fn.Spec == nil {
		return nil
	}

	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return nil
	}

	return transform.ReturnTypeFromSpec(spec, args)
}

// ApplyOverride applies a spec return override to call result types.
// If override is non-nil, replaces the first return type.
func ApplyOverride(types []typ.Type, override typ.Type) []typ.Type {
	if override == nil || len(types) == 0 {
		return types
	}

	result := make([]typ.Type, len(types))
	copy(result, types)
	result[0] = override
	return result
}

// ResolveSpecFunction extracts the function type from a potentially wrapped type.
// Handles aliases, generics, and instantiated types.
func ResolveSpecFunction(t typ.Type) *typ.Function {
	if t == nil {
		return nil
	}

	t = unwrap.Alias(t)

	if g, ok := t.(*typ.Generic); ok {
		t = g.Body
	}

	if inst, ok := t.(*typ.Instantiated); ok {
		resolved, err := core.ResolveInstantiated(inst)
		if err == nil && resolved != nil {
			t = resolved
		}
	}

	fn, ok := t.(*typ.Function)
	if !ok {
		return nil
	}

	return fn
}

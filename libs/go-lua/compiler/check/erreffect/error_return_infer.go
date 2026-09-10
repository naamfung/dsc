package erreffect

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// AttachInferredErrorReturnSpec enriches function types with a canonical
// ErrorReturn effect when the function body proves the `(value, err)` pattern.
func AttachInferredErrorReturnSpec(
	fn *typ.Function,
	graph *cfg.Graph,
	solution *flow.Solution,
	synth api.Synth,
) *typ.Function {
	if fn == nil || graph == nil || synth == nil || len(fn.Returns) != 2 {
		return fn
	}
	if HasErrorReturnLabel(fn) {
		return fn
	}
	base := synth.Narrow()
	if base == nil {
		base = synth
	}
	if !HasStrictInverseReturnPattern(graph, solution, base, 0, 1) {
		return fn
	}

	return AttachErrorReturnSpec(fn, 0, 1)
}

func HasErrorReturnLabel(fn *typ.Function) bool {
	spec := contract.ExtractSpec(fn)
	if spec == nil {
		return false
	}
	for _, label := range spec.Effects.Labels {
		if _, ok := label.(effect.ErrorReturn); ok {
			return true
		}
	}
	return false
}

func HasStrictInverseReturnPattern(
	graph *cfg.Graph,
	solution *flow.Solution,
	synth api.BaseSynth,
	valueIdx int,
	errorIdx int,
) bool {
	if graph == nil || synth == nil {
		return false
	}
	var sawSuccess bool
	var sawFailure bool
	var incompatible bool
	var classified bool

	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if incompatible || info == nil {
			return
		}
		if solution != nil && solution.IsPointDead(p) {
			return
		}
		// Skip synthetic implicit return nodes; explicit `return` without values
		// is a real nil,nil return and should block inference.
		if len(info.Exprs) == 0 && info.Stmt == nil {
			return
		}

		values := synth.ExpandValues(info.Exprs, 2, p)
		if valueIdx >= len(values) || errorIdx >= len(values) {
			incompatible = true
			return
		}

		valueState, okValue := classifyNilState(values[valueIdx])
		errorState, okError := classifyNilState(values[errorIdx])
		if !okValue && implicitReturnSlotIsNil(info.Exprs, valueIdx) {
			valueState, okValue = nilOnly, true
		}
		if !okError && implicitReturnSlotIsNil(info.Exprs, errorIdx) {
			errorState, okError = nilOnly, true
		}
		if !okValue || !okError {
			incompatible = true
			return
		}
		classified = true

		switch {
		case valueState == nilOnly && errorState == nonNilOnly:
			sawFailure = true
		case valueState == nonNilOnly && errorState == nilOnly:
			sawSuccess = true
		default:
			incompatible = true
		}
	})

	return classified && !incompatible && sawSuccess && sawFailure
}

func AttachErrorReturnSpec(fn *typ.Function, valueIndex, errorIndex int) *typ.Function {
	if fn == nil {
		return fn
	}
	if HasErrorReturnLabel(fn) {
		return fn
	}
	spec, ok := cloneContractSpec(fn)
	if !ok {
		return fn
	}
	spec.Effects = spec.Effects.With(effect.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex})
	return cloneFunctionWithSpec(fn, spec)
}

type nilState uint8

const (
	nilUnknown nilState = iota
	nilOnly
	nonNilOnly
)

func classifyNilState(t typ.Type) (nilState, bool) {
	if t == nil {
		return nilUnknown, false
	}
	u := unwrap.Alias(t)
	if u == nil {
		return nilUnknown, false
	}
	if u.Kind() == kind.Never {
		return nilUnknown, false
	}
	if u.Kind() == kind.Nil {
		return nilOnly, true
	}
	if core.ContainsNil(u) {
		return nilUnknown, false
	}
	return nonNilOnly, true
}

func cloneContractSpec(fn *typ.Function) (*contract.Spec, bool) {
	if fn == nil {
		return nil, false
	}
	if fn.Spec == nil {
		return contract.NewSpec(), true
	}
	spec := contract.ExtractSpec(fn)
	if spec == nil {
		return nil, false
	}
	clone := *spec
	return &clone, true
}

func implicitReturnSlotIsNil(exprs []ast.Expr, idx int) bool {
	if idx < 0 || idx < len(exprs) || len(exprs) == 0 {
		return false
	}
	last := exprs[len(exprs)-1]
	switch last.(type) {
	case *ast.FuncCallExpr, *ast.Comma3Expr:
		return false
	default:
		return true
	}
}

func cloneFunctionWithSpec(fn *typ.Function, spec *contract.Spec) *typ.Function {
	if fn == nil || spec == nil {
		return fn
	}
	builder := typ.Func()
	for _, tp := range fn.TypeParams {
		builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, param := range fn.Params {
		if param.Optional {
			builder.OptParam(param.Name, param.Type)
		} else {
			builder.Param(param.Name, param.Type)
		}
	}
	if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder.Effects(fn.Effects)
	}
	builder.Spec(spec)
	if fn.Refinement != nil {
		builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}

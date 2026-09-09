package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/types/flow"
)

// ExtractReturnKinds classifies return statements.
func ExtractReturnKinds(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc == nil || fc.Graph == nil || inputs == nil {
		return
	}
	derived := fc.Derived
	if derived == nil {
		derived = &core.Derived{}
	}
	fc.Graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if len(info.Exprs) == 0 {
			return
		}

		kind := resolve.ClassifyReturnExpr(info.Exprs[0])
		if kind != flow.ReturnUnknown {
			inputs.ReturnKinds[p] = kind
			return
		}

		constResolver := predicate.BuildConstResolver(inputs, p)

		sc := fc.Scopes[p]
		exprConstraints := cond.ExtractReturnExprConstraints(info.Exprs[0], p, sc, inputs, derived.TypeKeyRes, derived.Synth, constResolver, derived.SymResolver)
		if exprConstraints.OnTrue.HasConstraints() || exprConstraints.OnFalse.HasConstraints() {
			inputs.ReturnConstraints[p] = exprConstraints
		}
	})
}

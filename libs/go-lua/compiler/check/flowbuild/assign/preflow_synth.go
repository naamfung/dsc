package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	fbpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type narrowResolverAdapter struct {
	ctx *db.QueryContext
	ops core.TypeOps
}

var _ narrow.Resolver = (*narrowResolverAdapter)(nil)

func (r narrowResolverAdapter) Field(t typ.Type, name string) (typ.Type, bool) {
	if r.ops == nil {
		return nil, false
	}
	return r.ops.Field(r.ctx, t, name)
}

func (r narrowResolverAdapter) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if r.ops == nil {
		return nil, false
	}
	return r.ops.Index(r.ctx, t, key)
}

// buildPreflowBranchSolution solves only branch/numeric edge facts that are
// already available before assignment extraction completes.
//
// This gives local inference access to canonical branch narrowing such as
// discriminant checks on parameters, without depending on later assignment-
// derived facts or full post-extraction solve.
func buildPreflowBranchSolution(fc *fbcore.FlowContext, inputs *flow.Inputs) *flow.Solution {
	if fc == nil || inputs == nil || inputs.Graph == nil || fc.TypeOps == nil {
		return nil
	}

	temp := *inputs
	temp.EdgeConditions = nil
	temp.EdgeNumericConstraints = nil

	cond.ExtractEdgeConstraints(fc, &temp)
	cond.ExtractNumericConstraints(fc, &temp)

	return flow.Solve(&temp, narrowResolverAdapter{ctx: fc.CallCtx, ops: fc.TypeOps})
}

// synthWithOverlayAndPreflow wraps base synthesis with overlay lookup and a
// preflow branch-narrowing view for identifiers and attribute/index reads.
//
// This keeps assignment inference on the canonical synthesis path while letting
// recursive field/index expressions observe already-provable branch facts.
func synthWithOverlayAndPreflow(
	overlay map[cfg.SymbolID]typ.Type,
	bindings *bind.BindingTable,
	inputs *flow.Inputs,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflow *flow.Solution,
	base func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	var synth func(ast.Expr, cfg.Point) typ.Type

	synth = func(expr ast.Expr, p cfg.Point) typ.Type {
		if expr == nil {
			return nil
		}

		if ident, ok := expr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if t, exists := overlay[sym]; exists {
					return t
				}
			}
		}

		if preflow != nil && bindings != nil && inputs != nil {
			constResolver := predicate.BuildConstResolver(inputs, p)
			if path := fbpath.FromExprWithBindings(expr, constResolver, bindings); !path.IsEmpty() {
				if narrowed := preflow.NarrowedTypeAt(p, path); !typ.IsAbsentOrUnknown(narrowed) {
					return narrowed
				}
			}
		}

		if attr, ok := expr.(*ast.AttrGetExpr); ok && typeOps != nil {
			objType := synth(attr.Object, p)
			if !typ.IsAbsentOrUnknown(objType) {
				switch key := attr.Key.(type) {
				case *ast.StringExpr:
					if ft, ok := typeOps.Field(callCtx, objType, key.Value); ok && !typ.IsAbsentOrUnknown(ft) {
						return ft
					}
					if it, ok := typeOps.Index(callCtx, objType, typ.LiteralString(key.Value)); ok && !typ.IsAbsentOrUnknown(it) {
						return it
					}
				default:
					keyType := synth(attr.Key, p)
					if !typ.IsAbsentOrUnknown(keyType) {
						if it, ok := typeOps.Index(callCtx, objType, keyType); ok && !typ.IsAbsentOrUnknown(it) {
							return it
						}
					}
				}
			}
		}

		if base == nil {
			return nil
		}
		return base(expr, p)
	}

	return synth
}

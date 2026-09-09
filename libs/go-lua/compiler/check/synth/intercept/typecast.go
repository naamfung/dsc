package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TypeCastIntercept handles type constructor/cast calls like TypeName(expr).
//
// Type names in the type system can be used as callable constructors:
//
//	type Point = {x: number, y: number}
//	local p = Point({x = 1, y = 2})  -- Returns Point
//
// Detection is effect-based: the callee must have the CallableType effect.
// This distinguishes type casts from regular function calls.
//
// Returns the target type (from the synthetic callable function's return type).
// Falls back to scope meta lookup when TypeLookup is unavailable.
type TypeCastIntercept struct{}

func (t *TypeCastIntercept) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	ident, ok := ex.Func.(*ast.IdentExpr)
	if !ok {
		return Result{}
	}

	// Effect-based dispatch: check for CallableType effect
	if calleeHasEffect(ex, ctx, effect.Row.HasCallableType) {
		var fnType typ.Type
		if ctx.TypeLookup != nil {
			fnType = ctx.TypeLookup(ident.Value)
		}
		if fnType == nil && ctx.Recurse != nil {
			fnType = ctx.Recurse(ex.Func)
		}
		fn := unwrap.Function(fnType)
		if fn != nil && len(fn.Returns) > 0 {
			return Result{
				Types: []typ.Type{fn.Returns[0]},
				Skip:  true,
			}
		}
		if ctx.Scope != nil {
			if meta := ctx.Scope.MetaForName(ident.Value); meta != nil {
				return Result{
					Types: []typ.Type{meta.Of},
					Skip:  true,
				}
			}
		}
	}
	return Result{}
}

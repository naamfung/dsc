package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// RequireIntercept handles Lua's require() function for module loading.
//
// When the type checker encounters require("module"), this intercept:
// 1. Looks up the module manifest by path
// 2. Returns the module's enriched export type
//
// The enriched export includes:
//   - The module's public API type
//   - Type definitions exported by the module
//   - Any effect annotations on the module
//
// Detection is effect-based: callee must have the ModuleLoad effect.
// Module resolution checks both direct paths and import aliases.
type RequireIntercept struct {
	Manifests io.ManifestQuerier
}

func (r *RequireIntercept) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	if r.Manifests == nil {
		return Result{}
	}

	if !isRequireCall(ex, ctx) {
		return Result{}
	}

	if len(ex.Args) != 1 {
		return Result{}
	}

	strArg, ok := ex.Args[0].(*ast.StringExpr)
	if !ok {
		return Result{}
	}

	moduleName := strArg.Value
	enriched := io.LookupEnrichedExport(r.Manifests, moduleName)
	if enriched == nil {
		return Result{}
	}

	return Result{
		Types: []typ.Type{enriched},
		Skip:  true,
	}
}

func isRequireCall(ex *ast.FuncCallExpr, ctx CallEnv) bool {
	if ex == nil || ex.Func == nil {
		return false
	}
	return calleeHasEffect(ex, ctx, effect.Row.HasModuleLoad)
}

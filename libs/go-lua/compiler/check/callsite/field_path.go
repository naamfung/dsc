package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// FieldPathWithBaseSymbol resolves an expression to (base symbol, canonical binder key)
// for static path access chains (for example obj.f, obj["k"], obj[1]).
//
// The returned path uses the canonical segment encoding:
//   - .field
//   - ["key"]
//   - [1]
func FieldPathWithBaseSymbol(bindings *bind.BindingTable, expr ast.Expr) (cfg.SymbolID, string, bool) {
	baseSym, segs, ok := StaticPathWithBaseSymbol(bindings, expr)
	if !ok || baseSym == 0 {
		return 0, "", false
	}
	if len(segs) == 0 {
		return baseSym, "", true
	}

	path, ok := bind.FieldPathKeyFromSegments(segs)
	if !ok {
		return 0, "", false
	}

	return baseSym, path, true
}

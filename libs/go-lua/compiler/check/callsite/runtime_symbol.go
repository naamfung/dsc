package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// SymbolOrCreateFieldFromExpr resolves a symbol from an expression.
//
// It first uses existing symbol bindings (identifiers, function literals, existing field
// symbols). If the expression is a static field path with no existing field symbol, it
// creates one via bindings.GetOrCreateFieldSymbol to provide canonical identity.
func SymbolOrCreateFieldFromExpr(expr ast.Expr, bindings *bind.BindingTable) cfg.SymbolID {
	if expr == nil || bindings == nil {
		return 0
	}
	if sym := SymbolFromExpr(expr, bindings); sym != 0 {
		return sym
	}
	baseSym, segments, ok := StaticPathWithBaseSymbol(bindings, expr)
	if !ok || baseSym == 0 || len(segments) == 0 {
		return 0
	}
	fieldPath, ok := bind.FieldPathKeyFromSegments(segments)
	if !ok {
		return 0
	}
	return bindings.GetOrCreateFieldSymbol(baseSym, fieldPath)
}

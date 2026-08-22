package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// SymbolFromExpr extracts a bound symbol from an expression.
//
// Supported forms:
//   - identifier expressions
//   - function literal expressions (via function-literal symbol binding)
//   - static field paths (obj.f or obj.f.g) with existing binder field symbols
func SymbolFromExpr(expr ast.Expr, bindings *bind.BindingTable) cfg.SymbolID {
	if expr == nil || bindings == nil {
		return 0
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym, ok := bindings.SymbolOf(e); ok {
			return sym
		}
	case *ast.FunctionExpr:
		if sym, ok := bindings.FuncLitSymbol(e); ok {
			return sym
		}
	case *ast.AttrGetExpr:
		if sym, ok := staticFieldSymbolFromAttrGet(bindings, e); ok {
			return sym
		}
	}
	return 0
}

func staticFieldSymbolFromAttrGet(bindings *bind.BindingTable, attr *ast.AttrGetExpr) (cfg.SymbolID, bool) {
	if bindings == nil || attr == nil {
		return 0, false
	}

	baseSym, segments, ok := StaticPathWithBaseSymbol(bindings, attr)
	if !ok || baseSym == 0 || len(segments) == 0 {
		return 0, false
	}

	path, ok := bind.FieldPathKeyFromSegments(segments)
	if !ok {
		return 0, false
	}

	return bindings.FieldSymbol(baseSym, path)
}

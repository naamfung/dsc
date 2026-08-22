package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// FunctionLiteralForSymbol resolves a function symbol to its function literal.
//
// Resolution order:
//  1. Binding table reverse lookup for literal symbols.
//  2. Function definition nodes in the graph.
//  3. Assignment sources in the graph (target symbol = function literal).
func FunctionLiteralForSymbol(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	sym cfg.SymbolID,
) *ast.FunctionExpr {
	if sym == 0 {
		return nil
	}
	if bindings != nil {
		if fn, ok := bindings.FuncLitBySymbol(sym); ok && fn != nil {
			return fn
		}
	}
	if graph == nil {
		return nil
	}

	var found *ast.FunctionExpr
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if found != nil || info == nil {
			return
		}
		if info.Symbol == sym {
			found = info.FuncExpr
		}
	})
	if found != nil {
		return found
	}

	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if found != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if found != nil {
				return
			}
			if target.Symbol != sym {
				return
			}
			if fn, ok := source.(*ast.FunctionExpr); ok {
				found = fn
			}
		})
	})

	return found
}

// FunctionLiteralForGraphSymbol resolves only graph-local stable function
// bindings for a symbol.
//
// Canonical boundary:
//   - include graph-local/global function definitions
//   - include local identifier assignments of function literals
//   - exclude mutable field-path symbols, whose current callable type must come
//     from value flow at the call site rather than binder symbol backtracking
func FunctionLiteralForGraphSymbol(graph *cfg.Graph, sym cfg.SymbolID) *ast.FunctionExpr {
	if sym == 0 || graph == nil {
		return nil
	}

	var found *ast.FunctionExpr
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if found != nil || info == nil || info.Symbol != sym {
			return
		}
		found = info.FuncExpr
	})
	if found != nil {
		return found
	}

	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if found != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if found != nil {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Symbol != sym {
				return
			}
			if fn, ok := source.(*ast.FunctionExpr); ok {
				found = fn
			}
		})
	})

	return found
}

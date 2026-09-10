package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// CanonicalSymbolFromExprWithAliases chooses a stable symbol identity for an
// expression and expands candidates through direct-alias chains when graph is
// provided.
func CanonicalSymbolFromExprWithAliases(
	expr ast.Expr,
	raw cfg.SymbolID,
	graph *cfg.Graph,
	primary *bind.BindingTable,
	fallback *bind.BindingTable,
	prefer func(cfg.SymbolID) bool,
) cfg.SymbolID {
	base := exprSymbolCandidates(expr, raw, primary, fallback)
	if len(base) == 0 {
		return 0
	}
	if graph == nil {
		return SelectPreferredSymbol(base, prefer)
	}
	candidates := expandAliasCandidates(base, graph)
	return SelectPreferredSymbol(candidates, prefer)
}

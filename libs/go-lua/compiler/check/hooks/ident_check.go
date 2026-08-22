// ident_check.go implements undefined variable detection for the type checker.
//
// This pass validates that all identifier expressions refer to defined variables
// at their use point. It catches "undefined variable 'x'" errors.
//
// # RESOLUTION ORDER
//
// For each identifier, the checker attempts resolution in order:
//  1. Binding table lookup (local variables from CFG)
//  2. Type name lookup (type definitions in scope)
//
// If all lookups fail, the identifier is undefined.
//
// # SPECIAL CASES
//
// The underscore "_" identifier is always allowed (discard pattern).
// Type names are recognized via scope.LookupType to avoid false positives
// when types are used as values (e.g., metatables).
package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/diag"
)

// CheckIdents validates that all identifier expressions are defined at their use point.
func CheckIdents(graph *cfg.Graph, scopes map[cfg.Point]*scope.State, sourceName string) []diag.Diagnostic {
	if graph == nil {
		return nil
	}

	var diags []diag.Diagnostic
	checker := &identChecker{
		graph:      graph,
		scopes:     scopes,
		sourceName: sourceName,
		diags:      &diags,
	}

	for _, p := range graph.RPO() {
		info := graph.Info(p)
		if info == nil {
			continue
		}
		checker.point = p
		checker.scope = scopes[p]
		checker.checkNodeInfo(info)
	}

	return diags
}

type identChecker struct {
	graph      *cfg.Graph
	scopes     map[cfg.Point]*scope.State
	sourceName string
	point      cfg.Point
	scope      *scope.State
	diags      *[]diag.Diagnostic
}

func (c *identChecker) checkNodeInfo(info cfg.NodeInfo) {
	switch v := info.(type) {
	case *cfg.AssignInfo:
		for _, expr := range v.Sources {
			c.checkIdentExpr(expr)
		}
		for _, expr := range v.IterExprs {
			c.checkIdentExpr(expr)
		}
		if v.NumericFor != nil {
			c.checkIdentExpr(v.NumericFor.Init)
			c.checkIdentExpr(v.NumericFor.Limit)
			c.checkIdentExpr(v.NumericFor.Step)
		}
		for _, target := range v.Targets {
			if target.Kind == cfg.TargetField || target.Kind == cfg.TargetIndex {
				c.checkIdentExpr(target.Base)
				c.checkIdentExpr(target.Key)
			}
		}
	case *cfg.CallInfo:
		c.checkIdentExpr(v.Callee)
		c.checkIdentExpr(v.Receiver)
		for _, arg := range v.Args {
			c.checkIdentExpr(arg)
		}
	case *cfg.ReturnInfo:
		for _, expr := range v.Exprs {
			c.checkIdentExpr(expr)
		}
	case *cfg.BranchInfo:
		c.checkIdentExpr(v.Condition)
	}
}

func (c *identChecker) checkIdentExpr(expr ast.Expr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		c.checkIdent(e)
	case *ast.AttrGetExpr:
		c.checkIdentExpr(e.Object)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			c.checkIdentExpr(field.Value)
		}
	case *ast.FuncCallExpr:
		c.checkIdentExpr(e.Func)
		c.checkIdentExpr(e.Receiver)
		for _, arg := range e.Args {
			c.checkIdentExpr(arg)
		}
	case *ast.LogicalOpExpr:
		c.checkIdentExpr(e.Lhs)
		c.checkIdentExpr(e.Rhs)
	case *ast.RelationalOpExpr:
		c.checkIdentExpr(e.Lhs)
		c.checkIdentExpr(e.Rhs)
	case *ast.StringConcatOpExpr:
		c.checkIdentExpr(e.Lhs)
		c.checkIdentExpr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		c.checkIdentExpr(e.Lhs)
		c.checkIdentExpr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		c.checkIdentExpr(e.Expr)
	case *ast.UnaryNotOpExpr:
		c.checkIdentExpr(e.Expr)
	case *ast.UnaryLenOpExpr:
		c.checkIdentExpr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		c.checkIdentExpr(e.Expr)
	case *ast.CastExpr:
		c.checkIdentExpr(e.Expr)
	case *ast.FunctionExpr:
	}
}

func (c *identChecker) checkIdent(ident *ast.IdentExpr) {
	if ident == nil || ident.Value == "" {
		return
	}
	if ident.Value == "_" {
		return
	}

	if bindings := c.graph.Bindings(); bindings != nil {
		if _, ok := bindings.SymbolOf(ident); ok {
			return
		}
	}

	if c.scope != nil {
		if _, isType := c.scope.LookupType(ident.Value); isType {
			return
		}
	}

	pos := diag.Position{
		File:   c.sourceName,
		Line:   ident.Line(),
		Column: ident.Column(),
	}
	*c.diags = append(*c.diags, diag.Diagnostic{
		Severity: diag.SeverityError,
		Code:     diag.ErrUndefined,
		Position: pos,
		Span:     ast.SpanOf(ident),
		Message:  "undefined variable '" + ident.Value + "'",
	})
}

package cfg

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/compiler/pathseg"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// AddLinearEdge adds an edge from the current point to next, then updates current.
func (b *Builder) AddLinearEdge(next basecfg.Point) {
	if b.CurrentLive {
		b.Cfg.AddEdge(b.Current, next, false)
	}

	b.Current = next
}

// AddConditionEdges creates branch nodes for a condition expression.
func (b *Builder) AddConditionEdges(expr ast.Expr, thenTarget, elseTarget basecfg.Point) basecfg.Point {
	if expr == nil {
		branch := b.AddCondBranch(nil)
		b.Cfg.AddEdge(branch, thenTarget, true)
		b.Cfg.AddEdge(branch, elseTarget, false)

		return branch
	}

	if e, ok := expr.(*ast.LogicalOpExpr); ok {
		switch e.Operator {
		case "and":
			lhs := b.AddCondBranch(e.Lhs)
			rhs := b.AddConditionEdges(e.Rhs, thenTarget, elseTarget)
			b.Cfg.AddEdge(lhs, rhs, true)
			b.Cfg.AddEdge(lhs, elseTarget, false)

			return lhs
		case "or":
			lhs := b.AddCondBranch(e.Lhs)
			rhs := b.AddConditionEdges(e.Rhs, thenTarget, elseTarget)
			b.Cfg.AddEdge(lhs, thenTarget, true)
			b.Cfg.AddEdge(lhs, rhs, false)

			return lhs
		}
	}

	branch := b.AddCondBranch(expr)
	b.Cfg.AddEdge(branch, thenTarget, true)
	b.Cfg.AddEdge(branch, elseTarget, false)

	return branch
}

// AddCondBranch creates a branch node for a condition.
func (b *Builder) AddCondBranch(expr ast.Expr) basecfg.Point {
	condVarPath, condCheck := extraction.ExtractCondition(expr)

	// Try to resolve the condition symbol using bindings
	var condSym basecfg.SymbolID
	if ident, ok := expr.(*ast.IdentExpr); ok {
		condSym, _ = b.symbolFromIdent(ident)
	} else if condVarPath != "" {
		// For complex expressions, extract root and try to resolve
		rootName := extraction.ExtractRootName(condVarPath)
		if rootName != "" {
			// Try to find root ident in expression tree
			if rootIdent := findRootIdent(expr); rootIdent != nil {
				condSym, _ = b.symbolFromIdent(rootIdent)
			}
		}
	}

	p := b.Cfg.AddBranch(condSym, condCheck)

	b.ScopeTracker.SnapshotVisibility(p)

	b.Info[p] = &BranchInfo{
		CondVar:    condVarPath,
		CondSymbol: condSym,
		CondCheck:  condCheck,
		Condition:  expr,
	}

	// Scan condition expression for nested functions
	b.scanExprForFuncsWithContext(p, expr, nil, "")

	return p
}

// findRootIdent finds the root identifier in an expression.
func findRootIdent(expr ast.Expr) *ast.IdentExpr {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e
	case *ast.AttrGetExpr:
		return findRootIdent(e.Object)
	case *ast.RelationalOpExpr:
		if ident := findRootIdent(e.Lhs); ident != nil {
			return ident
		}

		return findRootIdent(e.Rhs)
	case *ast.UnaryNotOpExpr:
		return findRootIdent(e.Expr)
	}

	return nil
}

// ResolvePendingGotos resolves forward goto references.
func (b *Builder) ResolvePendingGotos() {
	if len(b.Pending) == 0 {
		return
	}
	labels := make([]string, 0, len(b.Pending))
	for label := range b.Pending {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	for _, label := range labels {
		froms := b.Pending[label]
		target, ok := b.Labels[label]
		if !ok {
			continue
		}

		for _, from := range froms {
			b.Cfg.AddEdge(from, target, false)
		}
	}
}

// ProcessExprs extracts identifier names and collects nested functions in one pass.
func (b *Builder) ProcessExprs(p basecfg.Point, exprs []ast.Expr) []string {
	if len(exprs) == 0 {
		return nil
	}

	names := extractIdentNames(exprs)
	b.scanExprsForFuncs(p, exprs)

	return names
}

func (b *Builder) scanExprsForFuncs(p basecfg.Point, exprs []ast.Expr) {
	for _, expr := range exprs {
		b.scanExprForFuncsWithContext(p, expr, nil, "")
	}
}

// scanExprForFuncsWithContext scans expressions for nested functions with table context.
// baseSym and basePath provide context when scanning table literal fields.
func (b *Builder) scanExprForFuncsWithContext(p basecfg.Point, expr ast.Expr, baseSym *basecfg.SymbolID, basePath string) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.FunctionExpr:
		// Assign symbol to function literal
		var sym basecfg.SymbolID

		if b.Bindings != nil {
			sym = b.Bindings.GetOrCreateFuncLitSymbol(e)
		}

		b.Nested = append(b.Nested, NestedFunc{Point: p, Func: e, Symbol: sym})
	case *ast.TableExpr:
		for _, field := range e.Fields {
			b.scanExprForFuncsWithContext(p, field.Key, baseSym, basePath)
			// For table literal field values, we don't have a direct path symbol yet
			// unless the table is assigned to a known variable
			b.scanExprForFuncsWithContext(p, field.Value, baseSym, basePath)
		}

	case *ast.FuncCallExpr:
		b.scanExprForFuncsWithContext(p, e.Func, nil, "")
		b.scanExprForFuncsWithContext(p, e.Receiver, nil, "")

		for _, arg := range e.Args {
			b.scanExprForFuncsWithContext(p, arg, nil, "")
		}
	case *ast.ArithmeticOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.StringConcatOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.RelationalOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.LogicalOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.UnaryMinusOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.UnaryNotOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.UnaryLenOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.UnaryBNotOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.AttrGetExpr:
		b.scanExprForFuncsWithContext(p, e.Object, nil, "")
		b.scanExprForFuncsWithContext(p, e.Key, nil, "")
	}
}

// scanExprForFuncsWithSymbol scans expressions and creates field symbols for table literal fields.
// baseSym is the symbol of the variable the expression is assigned to.
func (b *Builder) scanExprForFuncsWithSymbol(p basecfg.Point, expr ast.Expr, baseSym basecfg.SymbolID) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.FunctionExpr:
		// Function literal already handled by LocalAssign
		var sym basecfg.SymbolID

		if b.Bindings != nil {
			sym = b.Bindings.GetOrCreateFuncLitSymbol(e)
		}

		b.Nested = append(b.Nested, NestedFunc{Point: p, Func: e, Symbol: sym})
	case *ast.TableExpr:
		// Process table literal fields and create field symbols
		for _, field := range e.Fields {
			b.processTableField(p, baseSym, field)
		}
	case *ast.FuncCallExpr:
		b.scanExprForFuncsWithContext(p, e.Func, nil, "")
		b.scanExprForFuncsWithContext(p, e.Receiver, nil, "")

		for _, arg := range e.Args {
			b.scanExprForFuncsWithContext(p, arg, nil, "")
		}
	case *ast.ArithmeticOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.StringConcatOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.RelationalOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.LogicalOpExpr:
		b.scanExprForFuncsWithContext(p, e.Lhs, nil, "")
		b.scanExprForFuncsWithContext(p, e.Rhs, nil, "")
	case *ast.UnaryMinusOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.UnaryNotOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.UnaryLenOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.UnaryBNotOpExpr:
		b.scanExprForFuncsWithContext(p, e.Expr, nil, "")
	case *ast.AttrGetExpr:
		b.scanExprForFuncsWithContext(p, e.Object, nil, "")
		b.scanExprForFuncsWithContext(p, e.Key, nil, "")
	}
}

// processTableField processes a table field, creating field symbols for function values.
func (b *Builder) processTableField(p basecfg.Point, baseSym basecfg.SymbolID, field *ast.Field) {
	if field == nil {
		return
	}

	fieldSeg, hasStaticField := pathseg.StaticTableFieldKeySegment(field.Key)

	// If we have a base symbol and static field key, create field symbol for function values
	if baseSym != 0 && hasStaticField && b.Bindings != nil {
		if fnExpr, ok := field.Value.(*ast.FunctionExpr); ok && fnExpr != nil {
			// Create field symbol and associate with function literal
			fieldSym := b.getOrCreateFieldPathSymbol(baseSym, []constraint.Segment{fieldSeg})
			b.Bindings.SetFuncLitSymbol(fnExpr, fieldSym)
			b.Nested = append(b.Nested, NestedFunc{Point: p, Func: fnExpr, Symbol: fieldSym})

			return
		}
	}

	// Recursively process key and value
	b.scanExprForFuncsWithContext(p, field.Key, nil, "")

	// For nested tables, pass the field symbol as base
	if baseSym != 0 && hasStaticField && b.Bindings != nil {
		if tableExpr, ok := field.Value.(*ast.TableExpr); ok && tableExpr != nil {
			fieldSym := b.getOrCreateFieldPathSymbol(baseSym, []constraint.Segment{fieldSeg})
			for _, innerField := range tableExpr.Fields {
				b.processTableField(p, fieldSym, innerField)
			}

			return
		}
	}

	b.scanExprForFuncsWithContext(p, field.Value, nil, "")
}

// StealScopeVisibility transfers ownership of the visibility map from the
// ScopeTracker to the caller. The Builder must not be used after this call.
func (b *Builder) StealScopeVisibility() map[basecfg.Point]map[string]basecfg.SymbolID {
	m := b.ScopeTracker.visibility
	b.ScopeTracker.visibility = nil

	return m
}

// StealGlobals transfers ownership of the globals map from the ScopeTracker.
func (b *Builder) StealGlobals() map[string]basecfg.SymbolID {
	m := b.ScopeTracker.globals
	b.ScopeTracker.globals = nil

	return m
}

// StealDeclPoints transfers ownership of the declaration points map.
// The Builder must not be used after this call.
func (b *Builder) StealDeclPoints() map[basecfg.SymbolID]basecfg.Point {
	m := b.ScopeTracker.declPoints
	b.ScopeTracker.declPoints = nil

	return m
}

// StealSymbolNames transfers ownership of the symbol names map.
// The Builder must not be used after this call.
func (b *Builder) StealSymbolNames() map[basecfg.SymbolID]string {
	m := b.ScopeTracker.symbolNames
	b.ScopeTracker.symbolNames = nil

	return m
}

// StealSymbolKinds transfers ownership of the symbol kinds map.
// The Builder must not be used after this call.
func (b *Builder) StealSymbolKinds() map[basecfg.SymbolID]basecfg.SymbolKind {
	m := b.ScopeTracker.symbolKinds
	b.ScopeTracker.symbolKinds = nil

	return m
}

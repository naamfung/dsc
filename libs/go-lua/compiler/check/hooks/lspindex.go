// Package hooks provides diagnostic generation passes for the type checker.
// This file implements LSP symbol extraction during type checking.
package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

// LSPIndexer extracts symbols, references, and call edges during type checking.
type LSPIndexer struct {
	Symbols   *index.SymbolIndex
	CallGraph *index.CallGraph
}

// NewLSPIndexer creates an indexer with the given indexes.
func NewLSPIndexer(symbols *index.SymbolIndex, callGraph *index.CallGraph) *LSPIndexer {
	return &LSPIndexer{
		Symbols:   symbols,
		CallGraph: callGraph,
	}
}

// WithLSPIndex creates a pass that extracts symbols and call edges for LSP.
func WithLSPIndex(indexer *LSPIndexer) check.Option {
	if indexer == nil || indexer.Symbols == nil {
		return check.WithPass(func(*check.Session, *ast.FunctionExpr, *api.FuncResult) []diag.Diagnostic {
			return nil
		})
	}
	return check.WithPass(func(sess *check.Session, fn *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic {
		indexer.extractFromFunction(sess, fn, result)
		return nil
	})
}

// extractFromFunction extracts all symbols and call edges from a function.
func (idx *LSPIndexer) extractFromFunction(sess *check.Session, fn *ast.FunctionExpr, result *api.FuncResult) {
	if result == nil || result.Graph == nil {
		return
	}

	file := sess.SourceName
	graph := result.Graph
	bindings := graph.Bindings()
	if bindings == nil {
		return
	}

	funcName := extractFuncName(fn, graph)

	// Extract parameters
	idx.extractParameters(file, graph, result, funcName)

	// Extract local variables and function definitions
	idx.extractLocals(file, graph, result, funcName)

	// Extract function definitions
	idx.extractFuncDefs(file, graph, result, funcName)

	// Extract type definitions
	idx.extractTypeDefs(file, graph, result)

	// Extract references and calls
	idx.extractReferencesAndCalls(file, graph, result, funcName)
}

// extractFuncName returns the function name for scoping.
func extractFuncName(fn *ast.FunctionExpr, graph *cfg.Graph) string {
	if fn == nil {
		return ""
	}
	// FunctionExpr does not carry a name; named defs are tracked via FuncDefInfo.
	return ""
}

// extractParameters extracts function parameter symbols.
func (idx *LSPIndexer) extractParameters(file string, graph *cfg.Graph, result *api.FuncResult, scope string) {
	if idx.Symbols == nil {
		return
	}

	paramSlots := graph.ParamSlotsReadOnly()
	fn := graph.Func()

	if fn == nil || len(paramSlots) == 0 {
		return
	}

	for _, slot := range paramSlots {
		if !slot.HasSourceParam() || slot.Symbol == 0 {
			continue
		}
		name := slot.Name
		if name == "" {
			continue
		}

		// Use function span as best available fallback (parameter names have no AST nodes).
		span := astSpan(fn)
		if !span.Valid() {
			continue
		}

		// Get type from declared facts
		var paramType typ.Type
		if result.Facts != nil {
			tv := result.Facts.DeclaredAt(graph.Entry(), slot.Symbol)
			if tv.Type != nil {
				paramType = tv.Type
			}
		}

		idx.Symbols.AddDefinition(file, name, index.SymbolParameter, paramType, span, scope)
	}
}

// extractLocals extracts local variable definitions.
func (idx *LSPIndexer) extractLocals(file string, graph *cfg.Graph, result *api.FuncResult, scope string) {
	if idx.Symbols == nil {
		return
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}

		info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}

			span := targetSpan(target)
			if !span.Valid() {
				span = localNameSpan(info, i)
			}
			if !span.Valid() {
				return
			}

			// Get type from synth
			var varType typ.Type
			if result.NarrowSynth != nil {
				if source != nil {
					varType = result.NarrowSynth.TypeOf(source, p)
				}
			}

			idx.Symbols.AddDefinition(file, target.Name, index.SymbolVariable, varType, span, scope)
		})
	})
}

// extractFuncDefs extracts function definitions.
func (idx *LSPIndexer) extractFuncDefs(file string, graph *cfg.Graph, result *api.FuncResult, parentScope string) {
	if idx.Symbols == nil {
		return
	}

	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil || info.Name == "" {
			return
		}

		span := funcDefSpan(info)
		if !span.Valid() {
			return
		}

		kind := index.SymbolFunction
		if info.TargetKind == cfg.FuncDefMethod {
			kind = index.SymbolMethod
		}

		// Get function type
		var funcType typ.Type
		if result.NarrowSynth != nil && info.FuncExpr != nil {
			funcType = result.NarrowSynth.TypeOf(info.FuncExpr, p)
		}

		idx.Symbols.AddDefinition(file, info.Name, kind, funcType, span, parentScope)
	})
}

// extractTypeDefs extracts type definitions.
func (idx *LSPIndexer) extractTypeDefs(file string, graph *cfg.Graph, result *api.FuncResult) {
	if idx.Symbols == nil {
		return
	}

	graph.EachTypeDef(func(p cfg.Point, info *cfg.TypeDefInfo) {
		if info == nil || info.Name == "" {
			return
		}

		span := typeDefSpan(info)
		if !span.Valid() {
			return
		}

		// Get type from scope
		var defType typ.Type
		if sc := result.Scopes[p]; sc != nil {
			if t, ok := sc.LookupType(info.Name); ok {
				defType = t
			}
		}

		idx.Symbols.AddDefinition(file, info.Name, index.SymbolType, defType, span, "")
	})
}

// extractReferencesAndCalls extracts references and call graph edges.
func (idx *LSPIndexer) extractReferencesAndCalls(file string, graph *cfg.Graph, result *api.FuncResult, callerName string) {
	bindings := graph.Bindings()
	if bindings == nil {
		return
	}

	// Extract references and calls from assignment sources
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}

		info.EachSource(func(_ int, src ast.Expr) {
			idx.extractExprRefs(file, src, p, graph, bindings)
			idx.extractExprCalls(file, callerName, src, p, graph)
		})
	})

	// Extract references and calls from call statements.
	graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		callExpr := info.Call
		if callExpr == nil {
			return
		}

		idx.extractExprRefs(file, callExpr, p, graph, bindings)
		idx.addCallEdge(file, callerName, info, p, graph)

		// Extract nested calls inside callee/receiver/args (avoid re-adding the call itself).
		if info.Callee != nil {
			idx.extractExprCalls(file, callerName, info.Callee, p, graph)
		}
		if info.Receiver != nil {
			idx.extractExprCalls(file, callerName, info.Receiver, p, graph)
		}
		for _, arg := range info.Args {
			idx.extractExprCalls(file, callerName, arg, p, graph)
		}
	})

	// Extract references and calls from return expressions
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for _, expr := range info.Exprs {
			idx.extractExprRefs(file, expr, p, graph, bindings)
			idx.extractExprCalls(file, callerName, expr, p, graph)
		}
	})

	// Extract references and calls from branch conditions
	graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info == nil || info.Condition == nil {
			return
		}
		idx.extractExprRefs(file, info.Condition, p, graph, bindings)
		idx.extractExprCalls(file, callerName, info.Condition, p, graph)
	})
}

// extractExprRefs extracts references from an expression.
func (idx *LSPIndexer) extractExprRefs(file string, expr ast.Expr, p cfg.Point, graph *cfg.Graph, bindings *bind.BindingTable) {
	if idx.Symbols == nil || expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		idx.extractIdentRef(file, e, bindings)

	case *ast.AttrGetExpr:
		idx.extractExprRefs(file, e.Object, p, graph, bindings)
		// Note: key could also be a reference in dynamic access

	case *ast.FuncCallExpr:
		idx.extractExprRefs(file, e.Func, p, graph, bindings)
		for _, arg := range e.Args {
			idx.extractExprRefs(file, arg, p, graph, bindings)
		}

	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field.Value != nil {
				idx.extractExprRefs(file, field.Value, p, graph, bindings)
			}
		}

	case *ast.ArithmeticOpExpr:
		idx.extractExprRefs(file, e.Lhs, p, graph, bindings)
		idx.extractExprRefs(file, e.Rhs, p, graph, bindings)

	case *ast.RelationalOpExpr:
		idx.extractExprRefs(file, e.Lhs, p, graph, bindings)
		idx.extractExprRefs(file, e.Rhs, p, graph, bindings)

	case *ast.LogicalOpExpr:
		idx.extractExprRefs(file, e.Lhs, p, graph, bindings)
		idx.extractExprRefs(file, e.Rhs, p, graph, bindings)

	case *ast.StringConcatOpExpr:
		idx.extractExprRefs(file, e.Lhs, p, graph, bindings)
		idx.extractExprRefs(file, e.Rhs, p, graph, bindings)

	case *ast.UnaryNotOpExpr:
		idx.extractExprRefs(file, e.Expr, p, graph, bindings)
	case *ast.UnaryLenOpExpr:
		idx.extractExprRefs(file, e.Expr, p, graph, bindings)
	case *ast.UnaryBNotOpExpr:
		idx.extractExprRefs(file, e.Expr, p, graph, bindings)
	}
}

// extractExprCalls extracts call edges from an expression tree.
func (idx *LSPIndexer) extractExprCalls(file, callerName string, expr ast.Expr, p cfg.Point, graph *cfg.Graph) {
	if idx.CallGraph == nil || expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		info := cfg.BuildCallInfo(e, false)
		idx.addCallEdge(file, callerName, info, p, graph)
		idx.extractExprCalls(file, callerName, e.Func, p, graph)
		idx.extractExprCalls(file, callerName, e.Receiver, p, graph)
		for _, arg := range e.Args {
			idx.extractExprCalls(file, callerName, arg, p, graph)
		}

	case *ast.AttrGetExpr:
		idx.extractExprCalls(file, callerName, e.Object, p, graph)
		idx.extractExprCalls(file, callerName, e.Key, p, graph)

	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field.Value != nil {
				idx.extractExprCalls(file, callerName, field.Value, p, graph)
			}
		}

	case *ast.ArithmeticOpExpr:
		idx.extractExprCalls(file, callerName, e.Lhs, p, graph)
		idx.extractExprCalls(file, callerName, e.Rhs, p, graph)

	case *ast.RelationalOpExpr:
		idx.extractExprCalls(file, callerName, e.Lhs, p, graph)
		idx.extractExprCalls(file, callerName, e.Rhs, p, graph)

	case *ast.LogicalOpExpr:
		idx.extractExprCalls(file, callerName, e.Lhs, p, graph)
		idx.extractExprCalls(file, callerName, e.Rhs, p, graph)

	case *ast.StringConcatOpExpr:
		idx.extractExprCalls(file, callerName, e.Lhs, p, graph)
		idx.extractExprCalls(file, callerName, e.Rhs, p, graph)

	case *ast.UnaryMinusOpExpr:
		idx.extractExprCalls(file, callerName, e.Expr, p, graph)
	case *ast.UnaryNotOpExpr:
		idx.extractExprCalls(file, callerName, e.Expr, p, graph)
	case *ast.UnaryLenOpExpr:
		idx.extractExprCalls(file, callerName, e.Expr, p, graph)
	case *ast.UnaryBNotOpExpr:
		idx.extractExprCalls(file, callerName, e.Expr, p, graph)

	case *ast.CastExpr:
		idx.extractExprCalls(file, callerName, e.Expr, p, graph)
	case *ast.NonNilAssertExpr:
		idx.extractExprCalls(file, callerName, e.Expr, p, graph)
	}
}

// extractIdentRef extracts a reference for an identifier.
func (idx *LSPIndexer) extractIdentRef(file string, ident *ast.IdentExpr, bindings *bind.BindingTable) {
	if ident == nil || bindings == nil {
		return
	}

	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return
	}

	// Find the definition for this symbol
	defSym := idx.Symbols.LookupByName(file, ident.Value)
	if defSym == nil {
		return
	}

	span := astSpan(ident)
	if !span.Valid() {
		return
	}

	// Don't add reference if it's at the definition location
	if span == defSym.DefSpan {
		return
	}

	idx.Symbols.AddReference(file, defSym, span)
}

// addCallEdge adds a call edge to the call graph.
func (idx *LSPIndexer) addCallEdge(file, callerName string, info *cfg.CallInfo, p cfg.Point, graph *cfg.Graph) {
	if idx.CallGraph == nil || info == nil {
		return
	}

	calleeName := info.CalleeName
	if calleeName == "" {
		return
	}

	callSpan := callSpan(info)
	if !callSpan.Valid() {
		return
	}

	// Get caller span from graph function
	callerSpan := diag.Span{}
	if fn := graph.Func(); fn != nil {
		callerSpan = astSpan(fn)
	}

	// Callee file is same file for local functions
	calleeFile := file

	// Get callee span
	calleeSpan := diag.Span{}
	if calleeSym := idx.Symbols.LookupByName(file, calleeName); calleeSym != nil {
		calleeSpan = calleeSym.DefSpan
	}

	idx.CallGraph.AddCall(file, callerName, callerSpan, calleeFile, calleeName, calleeSpan, callSpan)
}

// Span extraction helpers

func astSpan(node ast.PositionHolder) diag.Span {
	if node == nil {
		return diag.Span{}
	}
	line, col := node.Line(), node.Column()
	endLine, endCol := node.LastLine(), node.LastColumn()
	if line == 0 {
		return diag.Span{}
	}
	return diag.Span{
		StartLine: line,
		StartCol:  col,
		EndLine:   endLine,
		EndCol:    endCol,
	}
}

func positionSpan(pos ast.Position) diag.Span {
	if pos.Line == 0 {
		return diag.Span{}
	}
	endLine := pos.EndLine
	endCol := pos.EndColumn
	if endLine == 0 {
		endLine = pos.Line
	}
	if endCol == 0 {
		endCol = pos.Column
	}
	return diag.Span{
		StartLine: pos.Line,
		StartCol:  pos.Column,
		EndLine:   endLine,
		EndCol:    endCol,
	}
}

func localNameSpan(info *cfg.AssignInfo, idx int) diag.Span {
	if info == nil || idx < 0 || info.Stmt == nil {
		return diag.Span{}
	}
	stmt, ok := info.Stmt.(*ast.LocalAssignStmt)
	if !ok {
		return diag.Span{}
	}
	if idx >= len(stmt.NamePositions) {
		return diag.Span{}
	}
	return positionSpan(stmt.NamePositions[idx])
}

func targetSpan(target cfg.AssignTarget) diag.Span {
	if target.Expr != nil {
		return astSpan(target.Expr)
	}
	return diag.Span{}
}

func funcDefSpan(info *cfg.FuncDefInfo) diag.Span {
	if info == nil {
		return diag.Span{}
	}
	// Use the function expression span
	if info.FuncExpr != nil {
		return astSpan(info.FuncExpr)
	}
	return diag.Span{}
}

func typeDefSpan(info *cfg.TypeDefInfo) diag.Span {
	if info == nil {
		return diag.Span{}
	}
	// TypeExpr provides position for type definitions
	if info.TypeExpr != nil {
		return astSpan(info.TypeExpr)
	}
	return diag.Span{}
}

func callSpan(info *cfg.CallInfo) diag.Span {
	if info == nil {
		return diag.Span{}
	}
	if info.Call != nil {
		return astSpan(info.Call)
	}
	if info.Callee == nil {
		return diag.Span{}
	}
	return astSpan(info.Callee)
}

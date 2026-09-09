// control_check.go implements control flow validation for the type checker.
//
// This pass validates structural control flow constraints that don't require
// type information:
//   - Break statements must be inside loops
//   - Goto labels must be defined before use
//   - Labels must be unique within their scope
//   - Duplicate local declarations in the same statement
//
// The checker walks the AST (not CFG) since these are syntactic constraints.
// Each function creates a new scope for labels/gotos.
//
// # VALIDATION RULES
//
// Break: Tracked via loopDepth counter. Error if loopDepth == 0.
//
// Goto/Label: Labels are collected during traversal. After traversal,
// all goto targets are validated against collected labels.
//
// Duplicate Locals: Within a single local statement, each name must be unique
// (except for "_" which is the discard pattern).
package hooks

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/diag"
)

// CheckControl validates control flow constructs in the given statements.
func CheckControl(stmts []ast.Stmt, sourceName string) []diag.Diagnostic {
	cc := &controlChecker{
		labels:     make(map[string]*ast.LabelStmt),
		sourceName: sourceName,
	}
	cc.checkStmts(stmts)
	cc.validateGotos()
	return cc.diags
}

type gotoTarget struct {
	stmt  *ast.GotoStmt
	label string
}

type controlChecker struct {
	labels     map[string]*ast.LabelStmt
	gotos      []gotoTarget
	loopDepth  int
	diags      []diag.Diagnostic
	sourceName string
}

func (cc *controlChecker) addError(node ast.PositionHolder, code diag.Code, format string, args ...any) {
	pos := diag.Position{File: cc.sourceName}
	span := diag.Span{}
	if node != nil {
		pos.Line = node.Line()
		pos.Column = node.Column()
		span = ast.SpanOf(node)
	}
	cc.diags = append(cc.diags, diag.Diagnostic{
		Severity: diag.SeverityError,
		Code:     code,
		Position: pos,
		Span:     span,
		Message:  fmt.Sprintf(format, args...),
	})
}

func (cc *controlChecker) checkStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		cc.checkStmt(stmt)
	}
}

func (cc *controlChecker) checkStmt(stmt ast.Stmt) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.LabelStmt:
		if prev := cc.labels[s.Name]; prev != nil {
			cc.addError(s, diag.ErrDuplicateDeclaration, "duplicate label '%s'", s.Name)
			return
		}
		cc.labels[s.Name] = s
	case *ast.GotoStmt:
		cc.gotos = append(cc.gotos, gotoTarget{stmt: s, label: s.Label})
	case *ast.BreakStmt:
		if cc.loopDepth == 0 {
			cc.addError(s, diag.ErrInvalidOperand, "break outside loop")
		}
	case *ast.WhileStmt:
		cc.loopDepth++
		cc.checkControlExpr(s.Condition)
		cc.checkStmts(s.Stmts)
		cc.loopDepth--
	case *ast.RepeatStmt:
		cc.loopDepth++
		cc.checkStmts(s.Stmts)
		cc.checkControlExpr(s.Condition)
		cc.loopDepth--
	case *ast.NumberForStmt:
		cc.loopDepth++
		cc.checkControlExpr(s.Init)
		cc.checkControlExpr(s.Limit)
		cc.checkControlExpr(s.Step)
		cc.checkStmts(s.Stmts)
		cc.loopDepth--
	case *ast.GenericForStmt:
		cc.loopDepth++
		for _, expr := range s.Exprs {
			cc.checkControlExpr(expr)
		}
		cc.checkStmts(s.Stmts)
		cc.loopDepth--
	case *ast.IfStmt:
		cc.checkControlExpr(s.Condition)
		cc.checkStmts(s.Then)
		cc.checkStmts(s.Else)
	case *ast.DoBlockStmt:
		cc.checkStmts(s.Stmts)
	case *ast.FuncDefStmt:
		if s.Func != nil {
			cc.checkFunction(s.Func)
		}
	case *ast.LocalAssignStmt:
		seen := make(map[string]bool)
		for _, name := range s.Names {
			if name == "_" {
				continue
			}
			if seen[name] {
				cc.addError(s, diag.ErrDuplicateDeclaration, "duplicate local '%s'", name)
			}
			seen[name] = true
		}
		for _, expr := range s.Exprs {
			cc.checkControlExpr(expr)
		}
	case *ast.AssignStmt:
		for _, expr := range s.Rhs {
			cc.checkControlExpr(expr)
		}
	case *ast.ReturnStmt:
		for _, expr := range s.Exprs {
			cc.checkControlExpr(expr)
		}
	case *ast.FuncCallStmt:
		cc.checkControlExpr(s.Expr)
	case *ast.TypeDefStmt:
	}
}

func (cc *controlChecker) checkControlExpr(expr ast.Expr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.FunctionExpr:
		cc.checkFunction(e)
	case *ast.AttrGetExpr:
		cc.checkControlExpr(e.Object)
		cc.checkControlExpr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field.Key != nil {
				cc.checkControlExpr(field.Key)
			}
			if field.Value != nil {
				cc.checkControlExpr(field.Value)
			}
		}
	case *ast.FuncCallExpr:
		cc.checkControlExpr(e.Func)
		cc.checkControlExpr(e.Receiver)
		for _, arg := range e.Args {
			cc.checkControlExpr(arg)
		}
	case *ast.LogicalOpExpr:
		cc.checkControlExpr(e.Lhs)
		cc.checkControlExpr(e.Rhs)
	case *ast.RelationalOpExpr:
		cc.checkControlExpr(e.Lhs)
		cc.checkControlExpr(e.Rhs)
	case *ast.StringConcatOpExpr:
		cc.checkControlExpr(e.Lhs)
		cc.checkControlExpr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		cc.checkControlExpr(e.Lhs)
		cc.checkControlExpr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		cc.checkControlExpr(e.Expr)
	case *ast.UnaryNotOpExpr:
		cc.checkControlExpr(e.Expr)
	case *ast.UnaryLenOpExpr:
		cc.checkControlExpr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		cc.checkControlExpr(e.Expr)
	}
}

func (cc *controlChecker) checkFunction(fn *ast.FunctionExpr) {
	if fn == nil {
		return
	}
	nested := &controlChecker{
		labels:     make(map[string]*ast.LabelStmt),
		sourceName: cc.sourceName,
	}
	nested.checkStmts(fn.Stmts)
	nested.validateGotos()
	cc.diags = append(cc.diags, nested.diags...)
}

func (cc *controlChecker) validateGotos() {
	for _, gt := range cc.gotos {
		if _, ok := cc.labels[gt.label]; !ok {
			cc.addError(gt.stmt, diag.ErrUndefined, "undefined label '%s'", gt.label)
		}
	}
}

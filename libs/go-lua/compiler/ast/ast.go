// Package ast defines the abstract syntax tree for Lua source code.
//
// The AST represents the syntactic structure of Lua programs, including
// expressions, statements, and type annotations. Each node carries source
// position information for error reporting.
//
// Key node types:
//   - Stmt: statements (assignment, function def, control flow, etc.)
//   - Expr: expressions (literals, identifiers, operators, calls, etc.)
//   - TypeExpr: type annotations for the optional type system
package ast

import "github.com/wippyai/go-lua/types/diag"

// PositionHolder provides source location info for AST nodes.
type PositionHolder interface {
	Line() int
	SetLine(int)
	LastLine() int
	SetLastLine(int)
	Column() int
	SetColumn(int)
	LastColumn() int
	SetLastColumn(int)
	SetPosFromToken(Position)
	SetLastPosFromToken(Position)
	CopyPos(PositionHolder)
	CopyLastPos(PositionHolder)
}

// Node is the base struct embedded in all AST nodes.
type Node struct {
	line     int
	lastline int
	col      int
	lastcol  int
}

// Line returns the starting line number.
func (n *Node) Line() int {
	return n.line
}

// SetLine sets the starting line number.
func (n *Node) SetLine(line int) {
	n.line = line
}

// LastLine returns the ending line number.
func (n *Node) LastLine() int {
	return n.lastline
}

// SetLastLine sets the ending line number.
func (n *Node) SetLastLine(line int) {
	n.lastline = line
}

// Column returns the starting column number.
func (n *Node) Column() int {
	return n.col
}

// SetColumn sets the starting column number.
func (n *Node) SetColumn(col int) {
	n.col = col
}

// LastColumn returns the ending column number.
func (n *Node) LastColumn() int {
	return n.lastcol
}

// SetLastColumn sets the ending column number.
func (n *Node) SetLastColumn(col int) {
	n.lastcol = col
}

// SpanOf extracts a diagnostic span from a PositionHolder.
func SpanOf(p PositionHolder) diag.Span {
	if p == nil {
		return diag.Span{}
	}
	return diag.Span{
		StartLine: p.Line(),
		StartCol:  p.Column(),
		EndLine:   p.LastLine(),
		EndCol:    p.LastColumn(),
	}
}

// SetPosFromToken sets line and column from a token position.
func (n *Node) SetPosFromToken(pos Position) {
	n.line = pos.Line
	n.col = pos.Column
	if pos.EndLine > 0 {
		n.lastline = pos.EndLine
	} else {
		n.lastline = pos.Line
	}
	if pos.EndColumn > 0 {
		n.lastcol = pos.EndColumn
	} else {
		n.lastcol = pos.Column
	}
}

// SetLastPosFromToken sets lastline and lastcol from a token position.
func (n *Node) SetLastPosFromToken(pos Position) {
	if pos.EndLine > 0 {
		n.lastline = pos.EndLine
	} else {
		n.lastline = pos.Line
	}
	if pos.EndColumn > 0 {
		n.lastcol = pos.EndColumn
	} else {
		n.lastcol = pos.Column
	}
}

// CopyPos copies line and column from another PositionHolder.
func (n *Node) CopyPos(src PositionHolder) {
	n.line = src.Line()
	n.col = src.Column()
}

// CopyLastPos copies lastline and lastcol from another PositionHolder.
func (n *Node) CopyLastPos(src PositionHolder) {
	n.lastline = src.LastLine()
	n.lastcol = src.LastColumn()
}

// Field represents a key-value pair in a table constructor.
type Field struct {
	Key   Expr
	Value Expr
}

// ParList represents a function parameter list.
type ParList struct {
	HasVargs   bool
	VarargType TypeExpr // Type annotation for variadic (...: T)
	Names      []string
	Types      []TypeExpr // Type annotations, parallel to Names (nil entries = inferred)
}

// FuncName represents a function name in a function definition statement.
type FuncName struct {
	Func     Expr
	Receiver Expr
	Method   string
}

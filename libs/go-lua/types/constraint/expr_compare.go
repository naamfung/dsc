package constraint

import "fmt"

// ExprRel represents a comparison operator for expressions.
type ExprRel uint8

const (
	ExprEq ExprRel = iota
	ExprNe
	ExprLt
	ExprLe
	ExprGt
	ExprGe
)

func (r ExprRel) String() string {
	switch r {
	case ExprEq:
		return "=="
	case ExprNe:
		return "!="
	case ExprLt:
		return "<"
	case ExprLe:
		return "<="
	case ExprGt:
		return ">"
	case ExprGe:
		return ">="
	default:
		return "?"
	}
}

// ExprCompare represents a relational constraint between expressions.
type ExprCompare struct {
	Rel   ExprRel
	Left  Expr
	Right Expr
}

func (c ExprCompare) String() string {
	return fmt.Sprintf("(%s %s %s)", c.Left, c.Rel, c.Right)
}

// Equals returns true if two ExprCompare values are structurally equal.
func (c ExprCompare) Equals(other ExprCompare) bool {
	return c.Rel == other.Rel && ExprEquals(c.Left, other.Left) && ExprEquals(c.Right, other.Right)
}

// EqExpr creates an equality comparison.
func EqExpr(left, right Expr) ExprCompare { return ExprCompare{Rel: ExprEq, Left: left, Right: right} }

// NeExpr creates an inequality comparison.
func NeExpr(left, right Expr) ExprCompare { return ExprCompare{Rel: ExprNe, Left: left, Right: right} }

// LtExpr creates a less-than comparison.
func LtExpr(left, right Expr) ExprCompare { return ExprCompare{Rel: ExprLt, Left: left, Right: right} }

// LeExpr creates a less-than-or-equal comparison.
func LeExpr(left, right Expr) ExprCompare { return ExprCompare{Rel: ExprLe, Left: left, Right: right} }

// GtExpr creates a greater-than comparison.
func GtExpr(left, right Expr) ExprCompare { return ExprCompare{Rel: ExprGt, Left: left, Right: right} }

// GeExpr creates a greater-than-or-equal comparison.
func GeExpr(left, right Expr) ExprCompare { return ExprCompare{Rel: ExprGe, Left: left, Right: right} }

package numconst

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/numparse"
)

// NegateConstraints negates a slice of constraints.
func NegateConstraints(items []constraint.Constraint) []constraint.Constraint {
	if len(items) == 0 {
		return nil
	}
	out := make([]constraint.Constraint, 0, len(items))
	for _, item := range items {
		if n, ok := constraint.NegateConstraint(item); ok {
			out = append(out, n)
		}
	}
	return out
}

// NumericConstraintFromComparisonWithBindings extracts a numeric constraint from a comparison using bindings.
func NumericConstraintFromComparisonWithBindings(op string, lhs, rhs ast.Expr, p cfg.Point, inputs *flow.Inputs, bindings *bind.BindingTable) constraint.NumericConstraint {
	leftPath := path.FromExprWithBindings(lhs, nil, bindings)
	rightPath := path.FromExprWithBindings(rhs, nil, bindings)

	leftConst, leftIsConst := IntConstFromExpr(lhs)
	rightConst, rightIsConst := IntConstFromExpr(rhs)

	switch op {
	case "<":
		if !leftPath.IsEmpty() && !rightPath.IsEmpty() {
			return constraint.Lt{X: leftPath, Y: rightPath}
		}
		if !leftPath.IsEmpty() && rightIsConst {
			return constraint.LeConst{X: leftPath, C: rightConst - 1}
		}
		if leftIsConst && !rightPath.IsEmpty() {
			return constraint.GeConst{X: rightPath, C: leftConst + 1}
		}
	case ">":
		if !leftPath.IsEmpty() && !rightPath.IsEmpty() {
			return constraint.Gt{X: leftPath, Y: rightPath}
		}
		if !leftPath.IsEmpty() && rightIsConst {
			return constraint.GeConst{X: leftPath, C: rightConst + 1}
		}
		if leftIsConst && !rightPath.IsEmpty() {
			return constraint.LeConst{X: rightPath, C: leftConst - 1}
		}
	case "<=":
		if !leftPath.IsEmpty() && !rightPath.IsEmpty() {
			return constraint.Le{X: leftPath, Y: rightPath, C: 0}
		}
		if !leftPath.IsEmpty() && rightIsConst {
			return constraint.LeConst{X: leftPath, C: rightConst}
		}
		if leftIsConst && !rightPath.IsEmpty() {
			return constraint.GeConst{X: rightPath, C: leftConst}
		}
	case ">=":
		if !leftPath.IsEmpty() && !rightPath.IsEmpty() {
			return constraint.Ge{X: leftPath, Y: rightPath}
		}
		if !leftPath.IsEmpty() && rightIsConst {
			return constraint.GeConst{X: leftPath, C: rightConst}
		}
		if leftIsConst && !rightPath.IsEmpty() {
			return constraint.LeConst{X: rightPath, C: leftConst}
		}
	}
	return nil
}

// NegateNumericConstraint returns the negation of a numeric constraint.
func NegateNumericConstraint(c constraint.NumericConstraint) constraint.NumericConstraint {
	if c == nil {
		return nil
	}
	switch v := c.(type) {
	case constraint.Lt:
		return constraint.Ge(v)
	case constraint.Gt:
		return constraint.Le{X: v.X, Y: v.Y, C: 0}
	case constraint.Le:
		return constraint.Gt{X: v.X, Y: v.Y}
	case constraint.Ge:
		return constraint.Lt(v)
	case constraint.LeConst:
		return constraint.GeConst{X: v.X, C: v.C + 1}
	case constraint.GeConst:
		return constraint.LeConst{X: v.X, C: v.C - 1}
	default:
		return nil
	}
}

// IntConstFromExpr extracts an integer constant from an expression.
func IntConstFromExpr(expr ast.Expr) (int64, bool) {
	switch v := expr.(type) {
	case *ast.NumberExpr:
		if i, ok := numparse.ParseIntegerLiteral(v.Value); ok {
			return i, true
		}
	case *ast.UnaryMinusOpExpr:
		if inner, ok := IntConstFromExpr(v.Expr); ok {
			return -inner, true
		}
	}
	return 0, false
}

package transform

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// ReturnTypeFromSpec returns the first matching spec return case type
// for the given call arguments. Returns nil if no case matches.
func ReturnTypeFromSpec(spec *contract.Spec, args []ast.Expr) typ.Type {
	if spec == nil || spec.Return == nil || len(spec.Return.Cases) == 0 {
		return nil
	}

	for _, rc := range spec.Return.Cases {
		if returnCaseMatchesArgs(rc.When, args) {
			return rc.Type
		}
	}

	return nil
}

// returnCaseMatchesArgs checks if the condition matches inline literal arguments.
// For DNF conditions: returns true if any disjunct has all its constraints match.
func returnCaseMatchesArgs(when constraint.Condition, args []ast.Expr) bool {
	return conditionAnyDisjunctMatches(when, func(c constraint.Constraint) bool {
		return constraintMatchesArgs(c, args)
	})
}

// constraintMatchesArgs checks if a single constraint matches inline literal arguments.
func constraintMatchesArgs(c constraint.Constraint, args []ast.Expr) bool {
	switch v := c.(type) {
	case constraint.FieldEquals:
		return fieldEqualsMatchesArgs(v, args)
	}
	return false
}

// fieldEqualsMatchesArgs checks if a FieldEquals constraint matches an inline table literal.
func fieldEqualsMatchesArgs(fe constraint.FieldEquals, args []ast.Expr) bool {
	paramIdx, ok := constraint.PlaceholderArgIndex(fe.Target, len(args))
	if !ok {
		return false
	}

	arg := args[paramIdx]
	if arg == nil {
		return false
	}

	// Only handles inline table constructors, not variable references.
	return tableConstructorFieldMatchesLiteral(arg, fe.Field, fe.Value)
}

// tableConstructorFieldMatchesLiteral checks if an inline table constructor
// has a field with the expected literal value.
// Only handles direct table literals like {fieldName = value}, not variables.
func tableConstructorFieldMatchesLiteral(expr ast.Expr, fieldName string, expected *typ.Literal) bool {
	if expected == nil {
		return false
	}

	tbl, ok := expr.(*ast.TableExpr)
	if !ok {
		return false
	}

	for _, field := range tbl.Fields {
		// Check for string key: {["fieldName"] = value}
		if keyExpr, ok := field.Key.(*ast.StringExpr); ok && keyExpr.Value == fieldName {
			return exprMatchesLiteral(field.Value, expected)
		}
		// Check for identifier key: {fieldName = value}
		if keyIdent, ok := field.Key.(*ast.IdentExpr); ok && keyIdent.Value == fieldName {
			return exprMatchesLiteral(field.Value, expected)
		}
	}

	return false
}

// exprMatchesLiteral checks if an AST expression matches a literal type value.
func exprMatchesLiteral(expr ast.Expr, lit *typ.Literal) bool {
	if expr == nil || lit == nil {
		return false
	}

	switch v := expr.(type) {
	case *ast.TrueExpr:
		if b, ok := lit.Value.(bool); ok {
			return b
		}
	case *ast.FalseExpr:
		if b, ok := lit.Value.(bool); ok {
			return !b
		}
	case *ast.StringExpr:
		if s, ok := lit.Value.(string); ok {
			return v.Value == s
		}
	case *ast.NumberExpr:
		switch val := lit.Value.(type) {
		case int64:
			if parsed, ok := numparse.ParseIntegerLiteral(v.Value); ok {
				return parsed == val
			}
		case float64:
			if parsed, ok := numparse.ParseFloatLiteral(v.Value); ok {
				return parsed == val
			}
		}
	}

	return false
}

package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// synthExprWithUnionExpected handles union expected types by trying each member.
func (s *Synthesizer) synthExprWithUnionExpected(
	expr ast.Expr,
	sc *scope.State,
	p cfg.Point,
	recurse ExprSynth,
	expected typ.Type,
) typ.Type {
	union, ok := unwrap.Alias(expected).(*typ.Union)
	if !ok {
		return s.synthExprWithExpectedSingle(expr, sc, p, recurse, expected)
	}

	if table, ok := expr.(*ast.TableExpr); ok {
		if match := core.TryDiscriminatedUnionMember(table, expected); match != nil {
			return s.SynthTableWithExpected(table, sc, recurse, match.Member)
		}
	}

	if fn, ok := expr.(*ast.FunctionExpr); ok {
		paramCount := 0
		if fn.ParList != nil {
			paramCount = len(fn.ParList.Names)
		}
		if compatible := core.CompatibleFunctionFromUnion(paramCount, expected); compatible != nil {
			return s.SynthFunctionTypeWithExpected(fn, sc, compatible)
		}
	}

	var results []typ.Type
	for _, member := range union.Members {
		result := s.synthExprWithExpectedSingle(expr, sc, p, recurse, member)
		if result != nil && s.isSubtype(result, member) {
			results = append(results, result)
		}
	}

	if len(results) == 0 {
		return s.synthExprCore(expr, sc, p, s.deps.Flow, recurse)
	}

	return typ.NewUnion(results...)
}

// isSubtype checks subtype relationship using memoized query if available.
func (s *Synthesizer) isSubtype(sub, super typ.Type) bool {
	if s.deps.Types != nil {
		return s.deps.Types.IsSubtype(s.deps.Ctx, sub, super)
	}
	return subtype.IsSubtype(sub, super)
}

// synthExprWithExpectedSingle handles non-union expected types.
func (s *Synthesizer) synthExprWithExpectedSingle(
	expr ast.Expr,
	sc *scope.State,
	p cfg.Point,
	recurse ExprSynth,
	expected typ.Type,
) typ.Type {
	switch ex := expr.(type) {
	case *ast.TableExpr:
		return s.SynthTableWithExpected(ex, sc, recurse, expected)
	case *ast.FunctionExpr:
		var expectedFn *typ.Function
		if expected != nil {
			expectedFn, _ = unwrap.Alias(expected).(*typ.Function)
		}
		return s.SynthFunctionTypeWithExpected(ex, sc, expectedFn)
	case *ast.FuncCallExpr:
		types := s.SynthCallCoreWithExpected(ex, p, sc, recurse, expected)
		if len(types) == 0 {
			return typ.Nil
		}
		return types[0]
	case *ast.IdentExpr:
		if expectedFn, ok := unwrap.Alias(expected).(*typ.Function); ok {
			if fnExpr := s.functionLiteralForIdent(ex); fnExpr != nil {
				return s.SynthFunctionTypeWithExpected(fnExpr, sc, expectedFn)
			}
		}
		inferred := s.synthExprCore(expr, sc, p, s.deps.Flow, recurse)
		if shouldRefineIdentWithExpected(inferred, expected) {
			return expected
		}
		return inferred
	case *ast.AttrGetExpr:
		if expectedFn, ok := unwrap.Alias(expected).(*typ.Function); ok {
			if fnType := s.expectedGraphLocalFunctionValueType(ex, p, sc, expectedFn, nil); fnType != nil {
				return fnType
			}
		}
		return s.synthExprCore(expr, sc, p, s.deps.Flow, recurse)
	default:
		return s.synthExprCore(expr, sc, p, s.deps.Flow, recurse)
	}
}

func shouldRefineIdentWithExpected(inferred, expected typ.Type) bool {
	if inferred == nil || expected == nil {
		return false
	}
	// Preserve explicit top annotations (`any`) and only use expected typing
	// when identifier inference is unresolved.
	return typ.IsUnknown(unwrap.Alias(inferred))
}

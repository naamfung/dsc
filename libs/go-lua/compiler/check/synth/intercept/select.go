package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// VariadicTypeResolver provides access to the variadic (...) type in scope.
//
// Used by SelectIntercept to determine the type of select(n, ...) calls
// when selecting from the variadic arguments.
type VariadicTypeResolver interface {
	VariadicType() typ.Type
}

// SelectIntercept handles Lua's select() builtin for variadic manipulation.
//
// Patterns handled:
//   - select("#", ...) -> integer (count of variadic arguments)
//   - select(n, ...) -> nth variadic element type
//   - select(n, a, b, c) -> tail return types from the nth argument onward
//
// Detection is effect-based: callee must have VariadicTransform effect.
//
// When selecting from varargs, uses VariadicResolver to get the variadic type.
// When selecting a concrete index from literal arguments, returns the tail
// types from that position onward.
type SelectIntercept struct {
	// VariadicResolver resolves the variadic type in current scope.
	// If nil, varargs pattern is not handled.
	VariadicResolver VariadicTypeResolver
}

func (s *SelectIntercept) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	if !isSelectCall(ex, ctx) {
		return Result{}
	}

	types := s.selectReturnTypes(ex, ctx)
	if len(types) == 0 {
		return Result{}
	}

	return Result{
		Types: types,
		Skip:  true,
	}
}

func isSelectCall(ex *ast.FuncCallExpr, ctx CallEnv) bool {
	if ex == nil || ex.Func == nil {
		return false
	}
	return calleeHasEffect(ex, ctx, effect.Row.HasVariadicTransform)
}

func (s *SelectIntercept) selectReturnTypes(ex *ast.FuncCallExpr, ctx CallEnv) []typ.Type {
	if ex == nil || len(ex.Args) == 0 {
		return nil
	}

	// select("#", ...) returns integer
	if str, ok := ex.Args[0].(*ast.StringExpr); ok && str.Value == "#" {
		return []typ.Type{typ.Integer}
	}

	if len(ex.Args) < 2 {
		return nil
	}

	// If selecting from varargs, use the variadic type.
	if _, ok := ex.Args[1].(*ast.Comma3Expr); ok {
		if s.VariadicResolver != nil {
			return []typ.Type{s.VariadicResolver.VariadicType()}
		}
		return nil
	}

	// If selecting a concrete index, use the type of that argument.
	if num, ok := ex.Args[0].(*ast.NumberExpr); ok {
		idx, ok := numparse.ParseIntegralLiteral(num.Value)
		if !ok || idx <= 0 {
			if !ok || idx == 0 {
				return nil
			}
			// Negative indices count from the end: -1 is last argument.
			n := len(ex.Args) - 1
			pos := n + int(idx) // idx is negative
			if pos < 0 || pos >= n || ctx.Recurse == nil {
				return nil
			}
			return tailTypes(ex.Args[pos+1:], ctx.Recurse)
		}
		i := int(idx) - 1
		if i >= 0 && i < len(ex.Args)-1 && ctx.Recurse != nil {
			return tailTypes(ex.Args[i+1:], ctx.Recurse)
		}
	}

	return nil
}

func tailTypes(args []ast.Expr, recurse ExprSynth) []typ.Type {
	if recurse == nil || len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, 0, len(args))
	for _, arg := range args {
		out = append(out, recurse(arg))
	}
	return out
}

// Package intercept provides AST-based call interception for special function handling.
//
// The type checker uses intercepts to handle function calls that require
// AST-level analysis beyond what pure type-based synthesis can provide.
// This includes:
//
//   - Type assertions (type(x) == "string")
//   - Type casts via callable types (Point(x, y))
//   - Special functions like require, select, pairs
//   - Effect-based dispatch (functions marked with specific effects)
//
// Intercepts examine the AST structure of calls and can return specialized
// type results. They are processed in chain order, with the first matching
// intercept determining the result.
//
// Call flow:
// 1. Synthesizer encounters FuncCallExpr
// 2. Chain.InterceptCall checks each CallIntercept
// 3. If any returns Skip=true, uses that result
// 4. Otherwise, proceeds with normal call synthesis
package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Result contains the outcome of attempting to intercept a function call.
//
// If Skip is true, Types contains the final return types and normal call
// synthesis should be bypassed. If Skip is false, the intercept did not
// handle the call and synthesis should continue normally.
type Result struct {
	// Types contains the return types if intercepted.
	Types []typ.Type

	// Skip indicates that the normal call synthesis should be skipped.
	// If true, Types contains the final result.
	Skip bool
}

// ExprSynth synthesizes the type of an expression.
type ExprSynth func(ast.Expr) typ.Type

// CallEnv provides the context needed by intercepts to analyze and type calls.
//
// Intercepts use CallEnv to:
//   - Resolve type names and function types via TypeLookup
//   - Synthesize types of sub-expressions via Recurse
//   - Access scope state for variable lookups via Scope
type CallEnv struct {
	// Scope provides scope state for type resolution.
	Scope *scope.State

	// Recurse synthesizes the type of sub-expressions.
	Recurse ExprSynth

	// TypeLookup resolves an identifier name to its declared function type.
	// For globals (require, select, type): returns function type from GlobalTypes.
	// For type names (Number, Point): returns synthetic callable function type.
	// Returns nil if the name is not a recognized function or type.
	TypeLookup func(name string) typ.Type
}

// CallIntercept handles AST-specific patterns in direct function calls.
//
// Implementations examine the call AST and return a Result with Skip=true
// if they handle the call, or Skip=false to pass to the next intercept.
type CallIntercept interface {
	// InterceptCall checks if this intercept handles the given call.
	// Returns a Result with Skip=true if the call was handled.
	InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result
}

// MethodIntercept intercepts method calls to handle AST-specific patterns.
type MethodIntercept interface {
	// InterceptMethodCall checks if this intercept handles the given method call.
	// Returns a Result with Skip=true if the call was handled.
	InterceptMethodCall(ex *ast.FuncCallExpr, ctx CallEnv) Result
}

// calleeHasEffect checks whether the callee of a function call has a specific effect.
// Returns true if the callee identifier resolves to a function type whose effect row
// satisfies the given check predicate.
func calleeHasEffect(ex *ast.FuncCallExpr, ctx CallEnv, check func(effect.Row) bool) bool {
	if ex == nil {
		return false
	}
	if ident, ok := ex.Func.(*ast.IdentExpr); ok && ctx.TypeLookup != nil {
		if t := ctx.TypeLookup(ident.Value); t != nil {
			if hasEffectInType(t, check) {
				return true
			}
		}
	}
	if ident, ok := ex.Func.(*ast.IdentExpr); ok && ctx.Scope != nil {
		if meta := ctx.Scope.MetaForName(ident.Value); meta != nil {
			// Treat type names as callable types for effect-based dispatch.
			fn := typ.Func().
				Param("value", typ.Any).
				Returns(meta.Of).
				Effects(effect.WithCallableType()).
				Build()
			if row, ok := fn.Effects.(effect.Row); ok && check(row) {
				return true
			}
		}
	}
	if ctx.Recurse != nil && ex.Func != nil {
		if t := ctx.Recurse(ex.Func); t != nil {
			if hasEffectInType(t, check) {
				return true
			}
		}
	}
	return false
}

func hasEffectInType(t typ.Type, check func(effect.Row) bool) bool {
	if t == nil {
		return false
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Function:
		if row, ok := v.Effects.(effect.Row); ok {
			return check(row)
		}
	case *typ.Optional:
		return hasEffectInType(v.Inner, check)
	case *typ.Union:
		for _, m := range v.Members {
			if hasEffectInType(m, check) {
				return true
			}
		}
	case *typ.Intersection:
		for _, m := range v.Members {
			if hasEffectInType(m, check) {
				return true
			}
		}
	}
	return false
}

// Chain aggregates multiple intercepts and runs them in sequence.
//
// For both calls and method calls, the chain processes intercepts in
// registration order. The first intercept that returns Skip=true determines
// the final result. If no intercept matches, an empty Result is returned.
type Chain struct {
	callIntercepts   []CallIntercept
	methodIntercepts []MethodIntercept
}

// NewChain creates a new intercept chain with the given intercepts.
func NewChain(calls []CallIntercept, methods []MethodIntercept) *Chain {
	return &Chain{
		callIntercepts:   calls,
		methodIntercepts: methods,
	}
}

// InterceptCall runs all call intercepts in order.
// Returns the first result with Skip=true, or an empty result if none matched.
func (c *Chain) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	for _, intercept := range c.callIntercepts {
		if result := intercept.InterceptCall(ex, ctx); result.Skip {
			return result
		}
	}
	return Result{}
}

// InterceptMethodCall runs all method intercepts in order.
// Returns the first result with Skip=true, or an empty result if none matched.
func (c *Chain) InterceptMethodCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	for _, intercept := range c.methodIntercepts {
		if result := intercept.InterceptMethodCall(ex, ctx); result.Skip {
			return result
		}
	}
	return Result{}
}

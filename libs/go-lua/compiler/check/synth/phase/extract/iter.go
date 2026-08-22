package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// inferIterVarsCore infers types for generic for loop variables using provided synthesis functions.
func (s *Synthesizer) inferIterVarsCore(exprs []ast.Expr, count int, synthOne func(ast.Expr) typ.Type, synthCall func(*ast.FuncCallExpr, int) []typ.Type) []typ.Type {
	if len(exprs) == 0 {
		return nil
	}

	if call, ok := exprs[0].(*ast.FuncCallExpr); ok {
		if types := synthCall(call, count); types != nil {
			return types
		}
	}

	iterType := synthOne(exprs[0])
	if fn := unwrap.Function(iterType); fn != nil && len(fn.Returns) > 0 {
		types := make([]typ.Type, count)
		for i := 0; i < count && i < len(fn.Returns); i++ {
			types[i] = fn.Returns[i]
		}
		for i := len(fn.Returns); i < count; i++ {
			types[i] = typ.Unknown
		}
		return types
	}

	types := make([]typ.Type, count)
	for i := range types {
		types[i] = typ.Unknown
	}
	return types
}

// inferIterVarsFromCallCore extracts iterator variable types from a function call with Iterator effect.
func (s *Synthesizer) inferIterVarsFromCallCore(call *ast.FuncCallExpr, count int, synthOne func(ast.Expr) typ.Type) []typ.Type {
	fnType := synthOne(call.Func)
	fn := unwrap.Function(fnType)
	if fn == nil || fn.Spec == nil {
		return nil
	}

	spec, ok := fn.Spec.(*contract.Spec)
	if !ok {
		return nil
	}

	iter := spec.GetIterator()
	if iter == nil {
		return nil
	}

	sourceIdx, ok := effect.ResolveParamIndex(iter.Source, len(call.Args))
	if !ok {
		return nil
	}
	sourceType := synthOne(call.Args[sourceIdx])
	if sourceType == nil {
		return nil
	}

	types := make([]typ.Type, count)
	for i := range types {
		types[i] = typ.Unknown
	}

	switch iter.Kind {
	case effect.IterateIndexed:
		if count > 0 {
			types[0] = typ.Integer
		}
		if count > 1 {
			if elem := core.ElementType(sourceType); elem != nil {
				types[1] = elem
			} else if sourceType.Kind().IsPlaceholder() {
				// Iterating over dynamic containers (any/unknown) should keep
				// loop values dynamic rather than collapsing to nil downstream.
				types[1] = typ.Any
			}
		}
	case effect.IterateKeyed:
		if count > 0 {
			if kt := core.KeyType(sourceType); kt != nil {
				types[0] = kt
			} else if sourceType.Kind().IsPlaceholder() {
				types[0] = typ.Any
			}
		}
		if count > 1 {
			if vt := core.ValueType(sourceType); vt != nil {
				types[1] = vt
			} else if sourceType.Kind().IsPlaceholder() {
				types[1] = typ.Any
			}
		}
	}

	return types
}

// inferIterVars infers types for generic for loop variables from iterator expressions.
func (s *Synthesizer) inferIterVars(exprs []ast.Expr, count int, p cfg.Point, narrower api.FlowOps) []typ.Type {
	synthOne := func(expr ast.Expr) typ.Type { return s.SynthExpr(expr, p, narrower) }
	return s.inferIterVarsCore(exprs, count, synthOne,
		func(call *ast.FuncCallExpr, n int) []typ.Type {
			return s.inferIterVarsFromCallCore(call, n, synthOne)
		},
	)
}

// inferIterVarsWithSpec infers iterator variable types using overlay types for lookup.
func (s *Synthesizer) inferIterVarsWithSpec(exprs []ast.Expr, count int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	synthOne := func(expr ast.Expr) typ.Type { return s.synthExprWithSpec(expr, p, specTypes) }
	return s.inferIterVarsCore(exprs, count, synthOne,
		func(call *ast.FuncCallExpr, n int) []typ.Type {
			return s.inferIterVarsFromCallCore(call, n, synthOne)
		},
	)
}

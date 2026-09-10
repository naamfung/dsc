package paramhints

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

type HintJoinFn func(prev, next typ.Type) typ.Type

// MergeIntoSignature replaces unannotated parameter slots (and refinable
// top-like annotations) with call-site hints.
func MergeIntoSignature(fn *ast.FunctionExpr, hints []typ.Type, sig *typ.Function) *typ.Function {
	if sig == nil || fn == nil || fn.ParList == nil {
		return sig
	}
	modified := false
	for i, p := range sig.Params {
		if i >= len(hints) || hints[i] == nil {
			continue
		}
		if i < len(fn.ParList.Types) && fn.ParList.Types[i] != nil {
			if !typ.IsRefinableAnnotation(p.Type) {
				continue
			}
		}
		if !typ.TypeEquals(p.Type, hints[i]) {
			modified = true
		}
	}
	if !modified {
		return sig
	}

	builder := typ.Func()
	for i, p := range sig.Params {
		paramType := p.Type
		if i < len(hints) && hints[i] != nil {
			annotated := i < len(fn.ParList.Types) && fn.ParList.Types[i] != nil
			if !annotated || typ.IsRefinableAnnotation(paramType) {
				paramType = hints[i]
			}
		}
		if p.Optional {
			builder = builder.OptParam(p.Name, paramType)
		} else {
			builder = builder.Param(p.Name, paramType)
		}
	}
	if sig.Variadic != nil {
		builder = builder.Variadic(sig.Variadic)
	}
	if len(sig.Returns) > 0 {
		builder = builder.Returns(sig.Returns...)
	}
	if sig.Effects != nil {
		builder = builder.Effects(sig.Effects)
	}
	if sig.Spec != nil {
		builder = builder.Spec(sig.Spec)
	}
	if sig.Refinement != nil {
		builder = builder.WithRefinement(sig.Refinement)
	}
	return builder.Build()
}

func WidenParamHintType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Literal:
		switch v.Base {
		case kind.Boolean:
			return typ.Boolean
		case kind.Integer:
			return typ.Integer
		case kind.Number:
			return typ.Number
		case kind.String:
			return typ.String
		}
	case *typ.Optional:
		inner := WidenParamHintType(v.Inner)
		if inner != v.Inner && inner != nil {
			return typ.NewOptional(inner)
		}
	case *typ.Alias:
		if v.Target != nil {
			return WidenParamHintType(v.Target)
		}
	case *typ.Union:
		changed := false
		members := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			wm := WidenParamHintType(m)
			if wm != m {
				changed = true
			}
			members = append(members, wm)
		}
		if changed {
			return typ.NewUnion(members...)
		}
	case *typ.Record:
		builder := typ.NewRecord()
		changed := false
		if !v.Open {
			// Call-site table literals should not over-constrain unannotated params.
			// Widen record hints to open records so optional field probes remain valid.
			builder.SetOpen(true)
			changed = true
		} else {
			builder.SetOpen(true)
		}
		for _, f := range v.Fields {
			ft := WidenParamHintType(f.Type)
			if ft != f.Type {
				changed = true
			}
			if f.Optional {
				builder.OptField(f.Name, ft)
			} else {
				builder.Field(f.Name, ft)
			}
		}
		if v.MapKey != nil && v.MapValue != nil {
			k := WidenParamHintType(v.MapKey)
			val := WidenParamHintType(v.MapValue)
			if k != v.MapKey || val != v.MapValue {
				changed = true
			}
			builder.MapComponent(k, val)
		}
		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if changed {
			return builder.Build()
		}
	}
	return t
}

// NormalizeHintType applies canonical widening and soft-member pruning.
func NormalizeHintType(t typ.Type) typ.Type {
	return typ.PruneSoftUnionMembers(WidenParamHintType(t))
}

// EnsureHintCapacity grows hint vector to at least size.
func EnsureHintCapacity(hints []typ.Type, size int) []typ.Type {
	if size <= len(hints) {
		return hints
	}
	expanded := make([]typ.Type, size)
	copy(expanded, hints)
	return expanded
}

// MergeHintAt normalizes and joins one hint into vector slot idx.
func MergeHintAt(hints []typ.Type, idx int, hint typ.Type, join HintJoinFn) ([]typ.Type, bool) {
	if idx < 0 {
		return hints, false
	}
	hint = NormalizeHintType(hint)
	if !IsInformativeHintType(hint) {
		return hints, false
	}
	hints = EnsureHintCapacity(hints, idx+1)

	joinFn := join
	if joinFn == nil {
		joinFn = typ.JoinPreferNonSoft
	}
	prev := hints[idx]
	merged := joinFn(prev, hint)
	if typ.TypeEquals(prev, merged) {
		return hints, false
	}
	hints[idx] = merged
	return hints, true
}

// MergeCallArgHintAt merges a call-argument observation into a parameter hint
// slot. Unlike MergeHintAt, unresolved/top-like argument observations are
// preserved as uncertainty evidence so later literal calls cannot over-specialize
// unannotated parameters.
func MergeCallArgHintAt(hints []typ.Type, idx int, argType typ.Type, join HintJoinFn, unknownOnNil bool) ([]typ.Type, bool) {
	if idx < 0 {
		return hints, false
	}
	argType = NormalizeHintType(argType)
	if argType == nil {
		if !unknownOnNil {
			return hints, false
		}
		argType = typ.Unknown
	}
	hints = EnsureHintCapacity(hints, idx+1)

	joinFn := join
	if joinFn == nil {
		joinFn = typ.JoinPreferNonSoft
	}

	prev := NormalizeHintType(hints[idx])
	if prev == nil {
		prev = hints[idx]
	}

	mergeTopAware := func(a, b typ.Type) typ.Type {
		if a == nil {
			return b
		}
		if b == nil {
			return a
		}
		if typ.IsAny(a) || typ.IsAny(b) {
			return typ.Any
		}
		if typ.IsUnknown(a) {
			return b
		}
		if typ.IsUnknown(b) {
			return a
		}
		return joinFn(a, b)
	}

	topLikeArg := typ.IsAny(argType) || typ.IsUnknown(argType)
	if !topLikeArg && !IsInformativeHintType(argType) {
		return hints, false
	}

	merged := mergeTopAware(prev, argType)
	if typ.TypeEquals(hints[idx], merged) {
		return hints, false
	}
	hints[idx] = merged
	return hints, true
}

// IsInformativeHintType reports whether a type carries useful call-site
// information for parameter hint propagation.
//
// It intentionally rejects top-like and empty placeholder shapes that tend to
// poison hints, while preserving structured hints such as maps/arrays with
// partial information (for example `{[string]: any[]}`).
func IsInformativeHintType(t typ.Type) bool {
	return isInformativeHintType(t, typ.NewGuard())
}

func isInformativeHintType(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	if t.Kind().IsDeferred() {
		return false
	}

	k := t.Kind()
	if k.IsPlaceholder() || k == kind.Nil || k == kind.Never {
		return false
	}

	switch v := t.(type) {
	case *typ.Optional:
		return isInformativeHintType(v.Inner, next)
	case *typ.Union:
		for _, m := range v.Members {
			if isInformativeHintType(m, next) {
				return true
			}
		}
		return false
	case *typ.Alias:
		if v.Target == nil {
			return false
		}
		return isInformativeHintType(v.Target, next)
	}

	if r, ok := t.(*typ.Record); ok {
		if len(r.Fields) == 0 && !r.HasMapComponent() && !r.Open {
			return false
		}
	}

	return true
}

// BuildParamHintSigView builds a function-expression keyed hint map for this graph.
// It merges per-iteration scratch hints with symbol-based hints from the store.
// Scratch hints take precedence over symbol-derived hints.
func BuildParamHintSigView(
	store api.StoreView,
	graph *cfg.Graph,
	parent *scope.State,
	stdlib *scope.State,
) map[*ast.FunctionExpr][]typ.Type {
	if store == nil || graph == nil || parent == nil {
		return nil
	}

	// Use stable snapshot param hints during analysis.
	symHints := store.GetParamHintsSnapshot(graph, parent)

	out := make(map[*ast.FunctionExpr][]typ.Type)

	if len(symHints) > 0 {
		for _, sym := range cfg.SortedSymbolIDs(symHints) {
			hints := symHints[sym]
			if len(hints) == 0 {
				continue
			}
			hasHint := false
			for _, hint := range hints {
				if hint != nil {
					hasHint = true
					break
				}
			}
			if !hasHint {
				continue
			}
			fn := store.FuncForSymbol(sym)
			if fn == nil {
				continue
			}
			if _, exists := out[fn]; !exists {
				out[fn] = hints
			}
		}
	}

	// If this graph is a nested function, pull param hints from the parent graph
	// and apply them to the current function signature.
	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			fallback := (*scope.State)(nil)
			if _, isNestedParent := store.NestedMetaFor(parentGraph.ID()); !isNestedParent {
				fallback = stdlib
			}
			parentScope := api.ParentScopeForGraph(store, parentGraph.ID(), fallback)
			if parentScope != nil {
				parentHints := store.GetParamHintsSnapshot(parentGraph, parentScope)
				if len(parentHints) > 0 {
					fn := store.FuncForGraph(graph)
					if fn == nil {
						fn = graph.Func()
					}
					if fn != nil {
						if sym, ok := store.SymbolForFunc(fn); ok {
							if hints := parentHints[sym]; len(hints) > 0 {
								out[fn] = hints
							}
						}
					}
				}
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

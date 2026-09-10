// expr.go implements expression-level type synthesis helpers.
//
// This file contains specialized synthesis for complex expression patterns:
//   - Logical operators (and/or) with short-circuit narrowing
//   - Attribute access with field/method resolution
//   - Table constructors with bidirectional typing
//   - Arithmetic and unary operators
//   - Expected type propagation for contextual inference
//
// # LOGICAL OPERATOR NARROWING
//
// For `x and y`, if x is truthy then y is evaluated with x narrowed to truthy.
// For `x or y`, if x is falsy then y is evaluated with x narrowed to falsy.
// This enables patterns like: `x and x.field` where x may be nil.
//
// # EXPECTED TYPE HANDLING
//
// When an expected type is provided (from assignment or function parameter),
// expressions are synthesized with contextual typing. This is important for:
//   - Table literals: fields inferred from expected record type
//   - Function literals: parameters inferred from expected function type
//   - Union discrimination: selecting the best matching union member
package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/numparse"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// localNarrowOps wraps a api.FlowOps and overrides NarrowedTypeAt for one path.
type localNarrowOps struct {
	inner        api.FlowOps
	overridePath constraint.Path
	overrideType typ.Type
}

func (n *localNarrowOps) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if path.Equal(n.overridePath) {
		return n.overrideType
	}
	if n.inner != nil {
		return n.inner.NarrowedTypeAt(p, path)
	}
	return nil
}

func (n *localNarrowOps) BoundsAt(p cfg.Point, name string) (int64, int64, bool) {
	if n.inner != nil {
		return n.inner.BoundsAt(p, name)
	}
	return 0, 0, false
}

func (n *localNarrowOps) ArrayLenBoundAt(p cfg.Point, varName string) (string, bool) {
	if n.inner != nil {
		return n.inner.ArrayLenBoundAt(p, varName)
	}
	return "", false
}

func (n *localNarrowOps) ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (string, int64, bool) {
	if n.inner != nil {
		return n.inner.ArrayLenBoundWithOffsetAt(p, varName)
	}
	return "", 0, false
}

func (n *localNarrowOps) IsPointDead(p cfg.Point) bool {
	if n.inner != nil {
		return n.inner.IsPointDead(p)
	}
	return false
}

func (n *localNarrowOps) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	if n.inner != nil {
		return n.inner.HasKeyOf(p, tablePath, keyPath)
	}
	return false
}

// synthAttrGetCore synthesizes type for attribute access using the shared core.
func (s *Synthesizer) synthAttrGetCore(ex *ast.AttrGetExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	objType := recurse(ex.Object)

	if narrower != nil && s.deps.Paths != nil {
		path := s.deps.Paths(p, ex, sc)
		if !path.IsEmpty() {
			narrowed := narrower.NarrowedTypeAt(p, path)
			if narrowed != nil {
				if specialized := s.stableLocalFunctionValueType(ex, p, sc, narrowed, nil); specialized != nil {
					return specialized
				}
				if typ.IsUnknown(unwrap.Alias(narrowed)) && typ.IsAny(unwrap.Alias(objType)) {
					goto skipNarrowedAttr
				}
				return narrowed
			}
		}
	}

skipNarrowedAttr:

	var manifestPath string
	if ident, ok := ex.Object.(*ast.IdentExpr); ok && s.deps.Manifests != nil && s.deps.CheckCtx != nil {
		if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				manifestPath = s.deps.CheckCtx.ModuleAlias(sym)
			}
		}
	}

	switch key := ex.Key.(type) {
	case *ast.StringExpr:
		if ft, ok := s.deps.Types.Field(s.deps.Ctx, objType, key.Value); ok {
			if manifestPath != "" {
				ft = enrichWithManifest(s.deps.Manifests, ft, manifestPath, key.Value)
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, ft, nil); specialized != nil {
				return specialized
			}
			return ft
		}
		if ft := fieldOnPartialUnion(objType, key.Value, s.deps.Types, s.deps.Ctx); ft != nil {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, ft, nil); specialized != nil {
				return specialized
			}
			return ft
		}
		if vt := mapValueType(objType); vt != nil {
			return vt
		}
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, typ.LiteralString(key.Value)); ok {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
	case *ast.NumberExpr:
		keyType := ops.ParseNumber(key.Value)
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType); ok {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
	case *ast.IdentExpr:
		keyType := recurse(key)
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType); ok {
			if narrower != nil {
				if narrowedResult := s.narrowTupleIndex(objType, key.Value, it, p, narrower); narrowedResult != nil {
					return narrowedResult
				}
				if narrowedResult := s.narrowArrayIndexByLenBound(it, ex.Object, key.Value, 0, p, sc, narrower); narrowedResult != nil {
					return narrowedResult
				}
				// Check for KeyOf constraint to unwrap optional on map index
				if opt, ok := it.(*typ.Optional); ok && s.deps.Paths != nil && s.deps.CheckCtx != nil {
					if tablePath := s.deps.Paths(p, ex.Object, sc); !tablePath.IsEmpty() {
						if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
							if keySym, ok := bindings.SymbolOf(key); ok && keySym != 0 {
								keyPath := constraint.Path{Root: key.Value, Symbol: keySym}
								if narrower.HasKeyOf(p, tablePath, keyPath) {
									return opt.Inner
								}
							}
						}
					}
				}
				if s.deps.Paths != nil && s.deps.CheckCtx != nil {
					if tablePath := s.deps.Paths(p, ex.Object, sc); !tablePath.IsEmpty() {
						if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
							if keySym, ok := bindings.SymbolOf(key); ok && keySym != 0 {
								keyPath := constraint.Path{Root: key.Value, Symbol: keySym}
								if narrower.HasKeyOf(p, tablePath, keyPath) {
									if refined := narrow.RemoveNil(it); !typ.IsNever(refined) {
										it = refined
									}
								}
							}
						}
					}
				}
				if derived := s.indexFromKeyOf(objType, ex.Object, key, p, sc, narrower); derived != nil {
					if keyType != nil && keyType.Kind().IsPlaceholder() {
						return derived
					}
					if shouldPreferKeyOfIndex(it) {
						return derived
					}
				}
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
		if derived := s.indexFromKeyOf(objType, ex.Object, key, p, sc, narrower); derived != nil {
			return derived
		}
	default:
		keyType := recurse(ex.Key)
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType); ok {
			if narrower != nil {
				if varName, offset, ok := indexVarOffsetFromExpr(ex.Key); ok {
					if narrowedResult := s.narrowArrayIndexByLenBound(it, ex.Object, varName, offset, p, sc, narrower); narrowedResult != nil {
						return narrowedResult
					}
				}
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
	}

	return typ.Unknown
}

func (s *Synthesizer) indexFromKeyOf(objType typ.Type, objExpr ast.Expr, key *ast.IdentExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	if s == nil || key == nil || narrower == nil || s.deps.Paths == nil || s.deps.CheckCtx == nil {
		return nil
	}
	bindings := s.deps.CheckCtx.Bindings()
	if bindings == nil {
		return nil
	}
	tablePath := s.deps.Paths(p, objExpr, sc)
	if tablePath.IsEmpty() {
		return nil
	}
	keySym, ok := bindings.SymbolOf(key)
	if !ok || keySym == 0 {
		return nil
	}
	keyPath := constraint.Path{Root: key.Value, Symbol: keySym}
	if !narrower.HasKeyOf(p, tablePath, keyPath) {
		return nil
	}
	tableType := objType
	if tableType == nil || tableType.Kind().IsPlaceholder() {
		if narrowed := narrower.NarrowedTypeAt(p, tablePath); narrowed != nil {
			tableType = narrowed
		}
	}
	derivedKey := querycore.KeyType(tableType)
	if derivedKey == nil {
		return nil
	}
	if it, ok := s.deps.Types.Index(s.deps.Ctx, tableType, derivedKey); ok {
		if opt, ok := it.(*typ.Optional); ok {
			return opt.Inner
		}
		return it
	}
	return nil
}

func shouldPreferKeyOfIndex(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, m := range v.Members {
			if shouldPreferKeyOfIndex(m) {
				return true
			}
		}
		return false
	default:
		if t.Kind().IsPlaceholder() {
			return true
		}
		if t.Kind() == kind.Nil {
			return true
		}
		return false
	}
}

// narrowTupleIndex checks if a tuple index can be narrowed using integer bounds.
func (s *Synthesizer) narrowTupleIndex(objType typ.Type, varName string, indexResult typ.Type, p cfg.Point, narrower api.FlowOps) typ.Type {
	tuple, ok := unwrap.Alias(objType).(*typ.Tuple)
	if !ok || len(tuple.Elements) == 0 {
		return nil
	}

	if narrower == nil {
		return nil
	}

	lower, upper, hasBounds := narrower.BoundsAt(p, varName)
	if !hasBounds {
		return nil
	}

	tupleLen := int64(len(tuple.Elements))
	if lower >= 1 && upper <= tupleLen {
		narrowed := narrow.RemoveNil(indexResult)
		if !typ.IsNever(narrowed) {
			return narrowed
		}
	}

	return nil
}

func (s *Synthesizer) narrowArrayIndexByLenBound(indexResult typ.Type, objExpr ast.Expr, varName string, offset int64, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	opt, ok := indexResult.(*typ.Optional)
	if !ok || narrower == nil || s == nil || s.deps.Paths == nil {
		return nil
	}
	lower, _, hasBounds := narrower.BoundsAt(p, varName)
	if !hasBounds {
		return nil
	}
	if lower+offset < 1 {
		return nil
	}
	arrKey, lenOffset, hasLenRef := narrower.ArrayLenBoundWithOffsetAt(p, varName)
	if !hasLenRef {
		return nil
	}
	tablePath := s.deps.Paths(p, objExpr, sc)
	if tablePath.IsEmpty() {
		return nil
	}
	if string(tablePath.Key()) != arrKey {
		return nil
	}
	if lenOffset > -offset {
		return nil
	}
	return opt.Inner
}

func indexVarOffsetFromExpr(expr ast.Expr) (string, int64, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if e.Value == "" {
			return "", 0, false
		}
		return e.Value, 0, true
	case *ast.ArithmeticOpExpr:
		ident, ok := e.Lhs.(*ast.IdentExpr)
		if !ok || ident.Value == "" {
			return "", 0, false
		}
		if e.Operator != "+" && e.Operator != "-" {
			return "", 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return "", 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return ident.Value, k, true
	}
	return "", 0, false
}

func intConstFromExpr(expr ast.Expr) (int64, bool) {
	switch v := expr.(type) {
	case *ast.NumberExpr:
		return numparse.ParseIntegerLiteral(v.Value)
	case *ast.UnaryMinusOpExpr:
		if n, ok := intConstFromExpr(v.Expr); ok {
			return -n, true
		}
	}
	return 0, false
}

// fieldOnPartialUnion handles field access on unions where some but not all
// members have the field.
func fieldOnPartialUnion(t typ.Type, name string, types querycore.TypeOps, ctx *db.QueryContext) typ.Type {
	u, ok := unwrap.Alias(t).(*typ.Union)
	if !ok {
		return nil
	}

	var fieldTypes []typ.Type
	hasField := false

	for _, m := range u.Members {
		if ft, ok := types.Field(ctx, m, name); ok {
			fieldTypes = append(fieldTypes, ft)
			hasField = true
		} else {
			fieldTypes = append(fieldTypes, typ.Nil)
		}
	}

	if !hasField {
		return nil
	}

	return typ.NewUnion(fieldTypes...)
}

// mapValueType returns the value type for map-like types without adding optional.
func mapValueType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Map:
		return v.Value
	case *typ.Optional:
		return mapValueType(v.Inner)
	case *typ.Union:
		var types []typ.Type
		for _, m := range v.Members {
			if vt := mapValueType(m); vt != nil {
				types = append(types, vt)
			}
		}
		if len(types) == 1 {
			return types[0]
		}
		if len(types) > 1 {
			return typ.NewUnion(types...)
		}
		return nil
	case *typ.Instantiated:
		if resolved, err := querycore.ResolveInstantiated(v); err == nil {
			return mapValueType(resolved)
		}
	}
	return nil
}

// synthLogicalOpCore synthesizes type for logical operators.
func (s *Synthesizer) synthLogicalOpCore(ex *ast.LogicalOpExpr, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)
	right := recurse(ex.Rhs)

	switch ex.Operator {
	case "and":
		return ops.LogicalAndTyped(left, right)
	case "or":
		return ops.LogicalOrTyped(left, right)
	default:
		return typ.Unknown
	}
}

// synthLogicalOpWithNarrowing synthesizes logical op with LHS path narrowing in RHS.
// For `and`: RHS sees LHS narrowed to truthy.
// For `or`: RHS sees LHS narrowed to falsy.
func (s *Synthesizer) synthLogicalOpWithNarrowing(ex *ast.LogicalOpExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)

	// Extract path for LHS expression
	var lhsPath constraint.Path
	if s.deps.Paths != nil {
		lhsPath = s.deps.Paths(p, ex.Lhs, sc)
	} else if ident, ok := ex.Lhs.(*ast.IdentExpr); ok {
		if s.deps.CheckCtx != nil {
			if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
				if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
					lhsPath = constraint.Path{Root: ident.Value, Symbol: sym}
				}
			}
		}
	}

	if !lhsPath.IsEmpty() && ops.CanBeFalsy(left) {
		var narrowedType typ.Type
		switch ex.Operator {
		case "and":
			narrowedType = narrow.ToTruthy(left)
		case "or":
			narrowedType = narrow.ToFalsy(left)
		}

		if !typ.IsNever(narrowedType) {
			wrapped := &localNarrowOps{
				inner:        narrower,
				overridePath: lhsPath,
				overrideType: narrowedType,
			}
			wrappedRecurse := func(expr ast.Expr) typ.Type {
				return s.SynthExpr(expr, p, wrapped)
			}
			right := wrappedRecurse(ex.Rhs)
			switch ex.Operator {
			case "and":
				return ops.LogicalAndTyped(left, right)
			case "or":
				return ops.LogicalOrTyped(left, right)
			}
		}
	}

	return s.synthLogicalOpCore(ex, recurse)
}

// synthArithmeticOpCore synthesizes type for arithmetic operators.
func (s *Synthesizer) synthArithmeticOpCore(ex *ast.ArithmeticOpExpr, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)
	right := recurse(ex.Rhs)
	return s.deps.Types.BinaryOp(s.deps.Ctx, left, ex.Operator, right)
}

// synthUnaryMinusCore synthesizes type for unary minus.
func (s *Synthesizer) synthUnaryMinusCore(ex *ast.UnaryMinusOpExpr, recurse ExprSynth) typ.Type {
	operand := recurse(ex.Expr)
	return s.deps.Types.UnaryOp(s.deps.Ctx, "-", operand)
}

// expandValuesCore expands expression list to types using provided synthesis functions.
func (s *Synthesizer) expandValuesCore(exprs []ast.Expr, needed int, single func(ast.Expr) typ.Type, multi func(ast.Expr) []typ.Type) []typ.Type {
	if len(exprs) == 0 {
		return nil
	}
	result := make([]typ.Type, 0, needed)

	for i, expr := range exprs {
		if i == len(exprs)-1 {
			result = append(result, multi(expr)...)
		} else {
			result = append(result, single(expr))
		}
	}

	for len(result) < needed {
		result = append(result, typ.Nil)
	}

	return result
}

// expandValues expands expression list to types.
func (s *Synthesizer) expandValues(exprs []ast.Expr, needed int, p cfg.Point, narrower api.FlowOps) []typ.Type {
	return s.expandValuesCore(exprs, needed,
		func(expr ast.Expr) typ.Type { return s.SynthExpr(expr, p, narrower) },
		func(expr ast.Expr) []typ.Type { return s.MultiTypeOf(expr, p) },
	)
}

// expandValuesWithSpec expands expression list with spec-narrowed type lookup.
func (s *Synthesizer) expandValuesWithSpec(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	return s.expandValuesCore(exprs, needed,
		func(expr ast.Expr) typ.Type { return s.synthExprWithSpec(expr, p, specTypes) },
		func(expr ast.Expr) []typ.Type { return s.synthMultiWithSpec(expr, p, specTypes) },
	)
}

// synthExprWithSpec synthesizes expression type with spec-narrowed lookup.
func (s *Synthesizer) synthExprWithSpec(expr ast.Expr, p cfg.Point, specTypes api.SpecTypes) typ.Type {
	if expr == nil {
		return typ.Nil
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		multi := s.synthMultiWithSpec(call, p, specTypes)
		if len(multi) == 0 || multi[0] == nil {
			return typ.Unknown
		}
		return multi[0]
	}
	if ident, ok := expr.(*ast.IdentExpr); ok {
		if sym := s.LookupSymbol(ident); sym != 0 {
			if t, exists := specTypes[sym]; exists {
				return t
			}
		}
	}
	sc := s.deps.ScopeAt(p)
	recurse := func(ex ast.Expr) typ.Type { return s.synthExprWithSpec(ex, p, specTypes) }
	return s.synthExprCore(expr, sc, p, nil, recurse)
}

// synthMultiWithSpec synthesizes multi-return expression with spec-narrowed lookup.
func (s *Synthesizer) synthMultiWithSpec(expr ast.Expr, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	sc := s.deps.ScopeAt(p)
	recurse := func(ex ast.Expr) typ.Type { return s.synthExprWithSpec(ex, p, specTypes) }
	return s.synthMultiCore(expr, sc, recurse,
		func(call *ast.FuncCallExpr) []typ.Type {
			if call.Receiver != nil {
				if recvIdent, ok := call.Receiver.(*ast.IdentExpr); ok {
					if sym := s.LookupSymbol(recvIdent); sym != 0 {
						if recvType, exists := specTypes[sym]; exists {
							return s.SynthCallWithReceiverType(call, p, sc, recvType, recurse)
						}
					}
				}
			}
			return s.synthCallCoreWithCaptureTypes(call, p, sc, nil, recurse, nil, specTypes)
		},
	)
}

// enrichWithManifest enriches a field type with manifest information.
func enrichWithManifest(manifests io.ManifestQuerier, ft typ.Type, modulePath, fieldName string) typ.Type {
	if manifests == nil {
		return ft
	}
	manifest := io.LookupManifest(manifests, modulePath)
	if manifest == nil {
		return ft
	}

	if enriched, ok := manifest.LookupValue(fieldName); ok && enriched != nil {
		return enriched
	}
	return ft
}

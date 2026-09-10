// field_check.go implements field access validation for the type checker.
//
// This pass runs after flow analysis to validate that field accesses are valid
// on their narrowed types. It catches errors like:
//   - Accessing non-existent fields on records/interfaces
//   - Indexing non-indexable types
//   - Invalid operands in arithmetic expressions
//
// The checker walks the CFG and examines AttrGetExpr nodes, checking each
// field access against the narrowed type of the receiver at that program point.
//
// For union types, the error message describes which union members have the
// field and which are missing it, helping the user understand why narrowing
// is needed.
package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	checksynth "github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	flowjoin "github.com/wippyai/go-lua/types/flow/join"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CheckFields validates field accesses on narrowed types.
func CheckFields(graph *cfg.Graph, narrowSynth api.Synth, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	if graph == nil || narrowSynth == nil || narrowView == nil {
		return nil
	}

	bindings := graph.Bindings()
	resolver := fieldResolverImpl{view: narrowView, synth: narrowSynth, bindings: bindings}

	var diags []diag.Diagnostic
	seen := make(map[ast.Expr]bool)

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		assignView, assignResolver := applyAssignPreStateNarrowing(graph, info, p, narrowView, resolver)
		info.EachSource(func(_ int, source ast.Expr) {
			diags = append(diags, checkFieldExpr(source, p, assignView, assignResolver, seen, sourceName)...)
		})
		if info.NumericFor != nil {
			diags = append(diags, checkNumericFor(info.NumericFor, p, narrowView, sourceName)...)
		}
	})

	graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		if info.Callee != nil {
			diags = append(diags, checkFieldExpr(info.Callee, p, narrowView, resolver, seen, sourceName)...)
		}
		if info.Receiver != nil {
			diags = append(diags, checkFieldExpr(info.Receiver, p, narrowView, resolver, seen, sourceName)...)
		}
		for _, arg := range info.Args {
			diags = append(diags, checkFieldExpr(arg, p, narrowView, resolver, seen, sourceName)...)
		}
	})

	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for _, expr := range info.Exprs {
			diags = append(diags, checkFieldExpr(expr, p, narrowView, resolver, seen, sourceName)...)
		}
	})

	return diags
}

type fieldResolverImpl struct {
	view     api.BaseSynth
	synth    api.Synth
	bindings *bind.BindingTable
}

func (r fieldResolverImpl) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	return r.view.TypeOf(expr, p)
}

func (r fieldResolverImpl) Field(t typ.Type, name string) (typ.Type, bool) {
	if r.synth == nil {
		return nil, false
	}
	return r.synth.Field(t, name)
}

type localNarrowView struct {
	base         api.BaseSynth
	bindings     *bind.BindingTable
	overridePath constraint.Path
	overrideType typ.Type
}

func (v *localNarrowView) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if v == nil || v.base == nil {
		return typ.Unknown
	}
	if v.overrideType != nil && v.bindings != nil {
		if p := path.FromExprWithBindings(expr, nil, v.bindings); !p.IsEmpty() && p.Equal(v.overridePath) {
			return v.overrideType
		}
	}
	return v.base.TypeOf(expr, p)
}

func (v *localNarrowView) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	if v == nil || v.base == nil {
		return typ.Unknown
	}
	return v.base.TypeOfWithExpected(expr, p, expected)
}

func (v *localNarrowView) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	if v == nil || v.base == nil {
		return nil
	}
	return v.base.MultiTypeOf(expr, p)
}

func (v *localNarrowView) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	if v == nil || v.base == nil {
		return nil
	}
	return v.base.FunctionType(fn, sc)
}

func (v *localNarrowView) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	if v == nil || v.base == nil {
		return nil
	}
	return v.base.ExpandValues(exprs, needed, p)
}

func (v *localNarrowView) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	if v == nil || v.base == nil {
		return nil
	}
	return v.base.InferIterVars(exprs, count, p)
}

func (v *localNarrowView) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if v == nil || v.base == nil {
		return typ.Unknown
	}
	return v.base.ResolveType(expr, sc)
}

func (v *localNarrowView) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	if v == nil || v.base == nil {
		return nil
	}
	return v.base.ResolveReturnTypes(types, sc)
}

func checkFieldExpr(expr ast.Expr, p cfg.Point, narrowView api.BaseSynth, resolver fieldResolverImpl, seen map[ast.Expr]bool, sourceName string) []diag.Diagnostic {
	if expr == nil || seen[expr] {
		return nil
	}
	seen[expr] = true

	var diags []diag.Diagnostic

	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		diags = append(diags, checkAttrGet(e, p, narrowView, resolver, seen, sourceName)...)
	case *ast.FuncCallExpr:
		diags = append(diags, checkFieldExpr(e.Func, p, narrowView, resolver, seen, sourceName)...)
		for _, arg := range e.Args {
			diags = append(diags, checkFieldExpr(arg, p, narrowView, resolver, seen, sourceName)...)
		}
	case *ast.TableExpr:
		for _, f := range e.Fields {
			diags = append(diags, checkFieldExpr(f.Value, p, narrowView, resolver, seen, sourceName)...)
		}
	case *ast.LogicalOpExpr:
		diags = append(diags, checkFieldExpr(e.Lhs, p, narrowView, resolver, seen, sourceName)...)
		lhsType := narrowView.TypeOf(e.Lhs, p)
		if e.Operator == "and" && ops.IsFalsy(lhsType) {
			return diags
		}
		if e.Operator == "or" && ops.IsTruthy(lhsType) {
			return diags
		}
		rhsView, rhsResolver := applyLogicalOpNarrowing(e, p, narrowView, resolver)
		diags = append(diags, checkFieldExpr(e.Rhs, p, rhsView, rhsResolver, seen, sourceName)...)
	case *ast.RelationalOpExpr:
		diags = append(diags, checkFieldExpr(e.Lhs, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkFieldExpr(e.Rhs, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkRelational(e, p, narrowView, sourceName)...)
	case *ast.ArithmeticOpExpr:
		diags = append(diags, checkFieldExpr(e.Lhs, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkFieldExpr(e.Rhs, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkArithmetic(e, p, narrowView, sourceName)...)
	case *ast.StringConcatOpExpr:
		diags = append(diags, checkFieldExpr(e.Lhs, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkFieldExpr(e.Rhs, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkStringConcat(e, p, narrowView, sourceName)...)
	case *ast.UnaryMinusOpExpr:
		diags = append(diags, checkFieldExpr(e.Expr, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkUnaryMinus(e, p, narrowView, sourceName)...)
	case *ast.UnaryLenOpExpr:
		diags = append(diags, checkUnaryLength(e, p, narrowView, sourceName)...)
	case *ast.UnaryBNotOpExpr:
		diags = append(diags, checkFieldExpr(e.Expr, p, narrowView, resolver, seen, sourceName)...)
		diags = append(diags, checkUnaryBNot(e, p, narrowView, sourceName)...)
	case *ast.UnaryNotOpExpr:
		diags = append(diags, checkFieldExpr(e.Expr, p, narrowView, resolver, seen, sourceName)...)
	}

	return diags
}

func applyLogicalOpNarrowing(
	expr *ast.LogicalOpExpr,
	p cfg.Point,
	view api.BaseSynth,
	resolver fieldResolverImpl,
) (api.BaseSynth, fieldResolverImpl) {
	if expr == nil || view == nil {
		return view, resolver
	}

	if resolver.bindings == nil {
		return view, resolver
	}

	switch expr.Operator {
	case "and", "or":
	default:
		return view, resolver
	}

	lhsType := view.TypeOf(expr.Lhs, p)
	if lhsType == nil || !ops.CanBeFalsy(lhsType) {
		return view, resolver
	}

	lhsPath := path.FromExprWithBindings(expr.Lhs, nil, resolver.bindings)
	if lhsPath.IsEmpty() {
		return view, resolver
	}

	var narrowed typ.Type
	if expr.Operator == "and" {
		narrowed = narrow.ToTruthy(lhsType)
	} else {
		narrowed = narrow.ToFalsy(lhsType)
	}

	if narrowed == nil || narrowed.Kind().IsNever() {
		return view, resolver
	}

	localView := &localNarrowView{
		base:         view,
		bindings:     resolver.bindings,
		overridePath: lhsPath,
		overrideType: narrowed,
	}

	return localView, fieldResolverImpl{view: localView, synth: resolver.synth, bindings: resolver.bindings}
}

func applyAssignPreStateNarrowing(
	graph *cfg.Graph,
	info *cfg.AssignInfo,
	p cfg.Point,
	view api.BaseSynth,
	resolver fieldResolverImpl,
) (api.BaseSynth, fieldResolverImpl) {
	if graph == nil || info == nil || view == nil || resolver.bindings == nil {
		return view, resolver
	}
	if len(info.Targets) == 0 {
		return view, resolver
	}

	assignView := view
	assignResolver := resolver
	for _, target := range info.Targets {
		if target.Expr == nil {
			continue
		}
		targetPath := path.FromExprWithBindings(target.Expr, nil, resolver.bindings)
		if targetPath.IsEmpty() {
			continue
		}
		preType := preAssignmentExprType(graph, target.Expr, p, view)
		if preType == nil || preType.Kind().IsPlaceholder() {
			continue
		}
		if target.Kind != cfg.TargetIdent {
			currentType := assignView.TypeOf(target.Expr, p)
			if currentType != nil && !currentType.Kind().IsPlaceholder() && !currentType.Kind().IsNever() {
				// For field/index targets, apply pre-state overlays only when they
				// are at least as specific as the current point type; otherwise we
				// risk broadening stable map/record flows.
				if !subtype.IsSubtype(preType, currentType) {
					continue
				}
			}
		}
		localView := &localNarrowView{
			base:         assignView,
			bindings:     resolver.bindings,
			overridePath: targetPath,
			overrideType: preType,
		}
		assignView = localView
		assignResolver = fieldResolverImpl{
			view:     localView,
			synth:    resolver.synth,
			bindings: resolver.bindings,
		}
	}

	return assignView, assignResolver
}

func preAssignmentExprType(graph *cfg.Graph, expr ast.Expr, p cfg.Point, view api.BaseSynth) typ.Type {
	if graph == nil || expr == nil || view == nil {
		return nil
	}
	preds := graph.Predecessors(p)
	if len(preds) == 0 {
		return nil
	}

	types := make([]typ.Type, 0, len(preds))
	for _, pred := range preds {
		if t := view.TypeOf(expr, pred); t != nil {
			types = append(types, t)
		}
	}
	if len(types) == 0 {
		return nil
	}
	return flowjoin.Types(types...)
}

func checkArithmetic(e *ast.ArithmeticOpExpr, p cfg.Point, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	check := func(expr ast.Expr) *diag.Diagnostic {
		t := narrowView.TypeOf(expr, p)
		if t == nil || ops.IsNumeric(t) {
			return nil
		}
		msg := "cannot perform arithmetic on " + typ.FormatShort(t) + ", expected number"
		_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
		return &diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrInvalidOperand,
			Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
			Span:     ast.SpanOf(expr),
			Message:  msg,
			Help:     help,
		}
	}
	if d := check(e.Lhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	if d := check(e.Rhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	return nil
}

func checkRelational(e *ast.RelationalOpExpr, p cfg.Point, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	switch e.Operator {
	case "<", "<=", ">", ">=":
	default:
		return nil
	}

	check := func(expr ast.Expr) *diag.Diagnostic {
		t := narrowView.TypeOf(expr, p)
		if t == nil || ops.MayBeOrderable(t) || typ.IsNever(t) || t.Kind() == kind.Nil {
			return nil
		}
		msg := "cannot compare " + typ.FormatShort(t) + ", expected orderable type"
		_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
		return &diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrInvalidOperand,
			Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
			Span:     ast.SpanOf(expr),
			Message:  msg,
			Help:     help,
		}
	}
	if d := check(e.Lhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	if d := check(e.Rhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	return nil
}

func checkStringConcat(e *ast.StringConcatOpExpr, p cfg.Point, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	check := func(expr ast.Expr) *diag.Diagnostic {
		t := narrowView.TypeOf(expr, p)
		if t == nil || ops.MayBeStringable(t) || typ.IsNever(t) || t.Kind() == kind.Nil || isOptionalFalseOnly(t) {
			return nil
		}
		msg := "cannot concatenate " + typ.FormatShort(t) + ", expected string or number"
		_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
		return &diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrInvalidOperand,
			Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
			Span:     ast.SpanOf(expr),
			Message:  msg,
			Help:     help,
		}
	}
	if d := check(e.Lhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	if d := check(e.Rhs); d != nil {
		return []diag.Diagnostic{*d}
	}
	return nil
}

func isOptionalFalseOnly(t typ.Type) bool {
	t = unwrap.Alias(t)
	opt, ok := t.(*typ.Optional)
	if !ok || opt == nil || opt.Inner == nil {
		return false
	}
	inner := unwrap.Alias(opt.Inner)
	lit, ok := inner.(*typ.Literal)
	if !ok || lit == nil {
		return false
	}
	v, ok := lit.Value.(bool)
	return ok && !v
}

func checkUnaryMinus(e *ast.UnaryMinusOpExpr, p cfg.Point, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	t := narrowView.TypeOf(e.Expr, p)
	if t == nil || ops.IsNumeric(t) {
		return nil
	}
	msg := "cannot apply unary - to " + typ.FormatShort(t) + ", expected number"
	_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
	return []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Code:     diag.ErrInvalidOperand,
		Position: diag.Position{File: sourceName, Line: e.Expr.Line(), Column: e.Expr.Column()},
		Span:     ast.SpanOf(e.Expr),
		Message:  msg,
		Help:     help,
	}}
}

func checkUnaryLength(e *ast.UnaryLenOpExpr, p cfg.Point, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	t := narrowView.TypeOf(e.Expr, p)
	if t == nil || ops.MayHaveLength(t) || typ.IsNever(t) || t.Kind() == kind.Nil {
		return nil
	}
	msg := "cannot apply length operator to " + typ.FormatShort(t) + ", expected string or table"
	_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
	return []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Code:     diag.ErrInvalidOperand,
		Position: diag.Position{File: sourceName, Line: e.Expr.Line(), Column: e.Expr.Column()},
		Span:     ast.SpanOf(e.Expr),
		Message:  msg,
		Help:     help,
	}}
}

func checkUnaryBNot(e *ast.UnaryBNotOpExpr, p cfg.Point, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	t := narrowView.TypeOf(e.Expr, p)
	if t == nil || ops.IsBitwiseNumeric(t) {
		return nil
	}
	msg := "cannot apply unary ~ to " + typ.FormatShort(t) + ", expected integer"
	_, help := diag.ContextualHelp(diag.ErrInvalidOperand, msg, "")
	return []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Code:     diag.ErrInvalidOperand,
		Position: diag.Position{File: sourceName, Line: e.Expr.Line(), Column: e.Expr.Column()},
		Span:     ast.SpanOf(e.Expr),
		Message:  msg,
		Help:     help,
	}}
}

func checkNumericFor(info *cfg.NumericForInfo, p cfg.Point, narrowView api.BaseSynth, sourceName string) []diag.Diagnostic {
	var diags []diag.Diagnostic
	check := func(expr ast.Expr, part string) {
		if expr == nil {
			return
		}
		t := narrowView.TypeOf(expr, p)
		if t != nil && !ops.IsNumeric(t) {
			msg := "for loop " + part + " must be numeric, got " + typ.FormatShort(t)
			_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
			diags = append(diags, diag.Diagnostic{
				Severity: diag.SeverityError,
				Code:     diag.ErrTypeMismatch,
				Position: diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()},
				Span:     ast.SpanOf(expr),
				Message:  msg,
				Help:     help,
			})
		}
	}
	check(info.Init, "init")
	check(info.Limit, "limit")
	check(info.Step, "step")
	return diags
}

func checkAttrGet(e *ast.AttrGetExpr, p cfg.Point, narrowView api.BaseSynth, resolver fieldResolverImpl, seen map[ast.Expr]bool, sourceName string) []diag.Diagnostic {
	var diags []diag.Diagnostic

	diags = append(diags, checkFieldExpr(e.Object, p, narrowView, resolver, seen, sourceName)...)

	objType := narrowView.TypeOf(e.Object, p)

	if !isStringKeyExpr(e.Key) {
		diags = append(diags, checkIndexAccess(e, p, narrowView, resolver, objType, sourceName)...)
		return diags
	}

	fieldName := ast.KeyName(e.Key)
	result := checksynth.ResolveFieldAccess(resolver, e, objType, fieldName, p)

	if result.SkipCheck {
		return diags
	}

	if result.NotIndexable {
		pos := diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()}
		span := ast.SpanOf(e)
		if e.Object != nil && e.Object.Line() > 0 {
			pos.Line = e.Object.Line()
			pos.Column = e.Object.Column()
			span = ast.SpanOf(e.Object)
		}
		msg := "cannot index type " + typ.FormatShort(objType)
		_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
		diags = append(diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrTypeMismatch,
			Position: pos,
			Span:     span,
			Message:  msg,
			Help:     help,
		})
		return diags
	}

	if !result.Found {
		pos := diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()}
		span := ast.SpanOf(e)
		if e.Key != nil && e.Key.Line() > 0 {
			pos.Line = e.Key.Line()
			pos.Column = e.Key.Column()
			span = ast.SpanOf(e.Key)
		}
		msg := formatMissingField(fieldName, objType)
		_, help := diag.ContextualHelp(diag.ErrNoField, msg, "")
		diags = append(diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     diag.ErrNoField,
			Position: pos,
			Span:     span,
			Message:  msg,
			Help:     help,
		})
	}

	return diags
}

func isStringKeyExpr(key ast.Expr) bool {
	if key == nil {
		return false
	}
	_, ok := key.(*ast.StringExpr)
	return ok
}

func checkIndexAccess(e *ast.AttrGetExpr, p cfg.Point, narrowView api.BaseSynth, resolver fieldResolverImpl, objType typ.Type, sourceName string) []diag.Diagnostic {
	if e == nil || narrowView == nil {
		return nil
	}
	if objType == nil || objType.Kind().IsPlaceholder() {
		return nil
	}
	keyType := narrowView.TypeOf(e.Key, p)
	if keyType == nil {
		// Treat unresolved key type as unknown at indexability boundary.
		// This preserves dynamic Lua semantics for computed keys while still
		// allowing container-specific checks to reject impossible accesses.
		keyType = typ.Unknown
	}

	if rec := unwrap.Record(objType); rec != nil && !rec.HasMapComponent() && !rec.Open {
		// Closed records support dynamic string indexing (Lua table semantics).
		// Non-string keys remain invalid.
		keyKind := keyType.Kind()
		allowsStringIndex := keyKind.IsPlaceholder() || subtype.IsSubtype(keyType, typ.String)
		if !allowsStringIndex {
			return []diag.Diagnostic{indexError(objType, e, sourceName)}
		}
	}

	var ok bool
	if resolver.synth != nil {
		_, ok = resolver.synth.CallQuery().Index(resolver.synth.Context(), objType, keyType)
	} else {
		_, ok = querycore.Index(objType, keyType)
	}
	if ok {
		return nil
	}

	return []diag.Diagnostic{indexError(objType, e, sourceName)}
}

func indexError(objType typ.Type, e *ast.AttrGetExpr, sourceName string) diag.Diagnostic {
	pos := diag.Position{File: sourceName, Line: e.Line(), Column: e.Column()}
	span := ast.SpanOf(e)
	if e.Object != nil && e.Object.Line() > 0 {
		pos.Line = e.Object.Line()
		pos.Column = e.Object.Column()
		span = ast.SpanOf(e.Object)
	}
	msg := "cannot index type " + typ.FormatShort(objType)
	_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
	return diag.Diagnostic{
		Severity: diag.SeverityError,
		Code:     diag.ErrTypeMismatch,
		Position: pos,
		Span:     span,
		Message:  msg,
		Help:     help,
	}
}

func formatMissingField(field string, objType typ.Type) string {
	if union, ok := objType.(*typ.Union); ok && len(union.Members) > 1 {
		var withField, withoutField []string
		for _, m := range union.Members {
			name := memberTypeName(m)
			if hasField(m, field) {
				withField = append(withField, name)
			} else {
				withoutField = append(withoutField, name)
			}
		}
		if len(withField) > 0 && len(withoutField) > 0 {
			return "field '" + field + "' missing on " + joinNames(withoutField) +
				" (present on " + joinNames(withField) + ")"
		}
	}
	return "field '" + field + "' does not exist on type " + typ.FormatShort(objType)
}

func memberTypeName(t typ.Type) string {
	switch v := t.(type) {
	case *typ.Interface:
		if v.Name != "" {
			return v.Name
		}
	case *typ.Alias:
		return v.Name
	case *typ.Record:
		if len(v.Fields) > 0 {
			for _, f := range v.Fields {
				if f.Name == "id" || f.Name == "type" || f.Name == "name" {
					if lit, ok := f.Type.(*typ.Literal); ok {
						return "{" + f.Name + ": " + lit.String() + ", ...}"
					}
					return "{" + f.Name + ": " + typ.FormatShort(f.Type) + ", ...}"
				}
			}
			f := v.Fields[0]
			return "{" + f.Name + ": ..., ...}"
		}
		return "{}"
	case *typ.Recursive:
		if v.Name != "" {
			return v.Name
		}
	case *typ.Instantiated:
		if v.Generic != nil && v.Generic.Name != "" {
			return v.Generic.Name + "<...>"
		}
	}
	s := typ.FormatShort(t)
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
}

func hasField(t typ.Type, field string) bool {
	switch v := t.(type) {
	case *typ.Record:
		for _, f := range v.Fields {
			if f.Name == field {
				return true
			}
		}
		return v.HasMapComponent()
	case *typ.Interface:
		for _, m := range v.Methods {
			if m.Name == field {
				return true
			}
		}
		return false
	case *typ.Alias:
		return hasField(v.Target, field)
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && hasField(v.Body, field)
	case *typ.Optional:
		return hasField(v.Inner, field)
	}
	return false
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	if len(names) == 2 {
		return names[0] + " and " + names[1]
	}
	result := names[0]
	for i := 1; i < len(names)-1; i++ {
		result += ", " + names[i]
	}
	return result + ", and " + names[len(names)-1]
}

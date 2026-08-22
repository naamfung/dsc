// Package resolve provides symbol and type resolution utilities for flow constraint extraction.
//
// This package bridges between AST nodes and the constraint system by providing:
//   - Symbol name resolution (SymbolID -> display name)
//   - Type resolution from various sources (inputs, overlays, scope)
//   - Path extraction from expressions (for constraint targeting)
//   - Effect lookup for function symbols
//
// # RESOLUTION HIERARCHY
//
// Type lookups follow a priority order:
//  1. SpecTypes overlay (contextually inferred types)
//  2. flow.Inputs.DeclaredTypes (explicit annotations)
//  3. Synthesizer fallback (structural inference)
//
// This allows inferred types to override base declarations while still
// falling back to synthesis for unannotated expressions.
//
// # SYMBOL VS NAME RESOLUTION
//
// All resolution uses SymbolID (unique identifier) rather than string names.
// Name strings are only used for display purposes (error messages, constraints).
// This avoids shadowing issues where the same name refers to different variables.
//
// # REF TYPE RESOLUTION
//
// typ.Ref types (forward references) are resolved via scope.LookupType.
// This is necessary because type definitions may not be available at
// constraint extraction time.
package resolve

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type symbolNamer interface {
	NameOf(sym cfg.SymbolID) string
}

func rootNameFromSymbolSource(source symbolNamer, sym cfg.SymbolID, fallback string) string {
	if sym == 0 || source == nil {
		return fallback
	}
	if name := source.NameOf(sym); name != "" {
		return name
	}
	return fallback
}

// RootName returns the display name for a symbol, using NameOf when available.
func RootName(graph *cfg.Graph, sym cfg.SymbolID, fallback string) string {
	return rootNameFromSymbolSource(graph, sym, fallback)
}

// RootNameFromBindings returns the display name for a symbol using bindings.
func RootNameFromBindings(bindings *bind.BindingTable, sym cfg.SymbolID, fallback string) string {
	if sym != 0 && bindings != nil {
		if name := bindings.Name(sym); name != "" {
			return name
		}
	}
	return fallback
}

// RootNameFromGraphAndBindings resolves display name with binding-first, graph-second fallback.
func RootNameFromGraphAndBindings(graph *cfg.Graph, bindings *bind.BindingTable, sym cfg.SymbolID, fallback string) string {
	name := RootNameFromBindings(bindings, sym, "")
	if name != "" {
		return name
	}
	return RootName(graph, sym, fallback)
}

// GetBindings returns the binding table from inputs.
// Requires inputs.Graph to be a *cfg.Graph (not just the VersionedGraph interface).
func GetBindings(inputs *flow.Inputs) *bind.BindingTable {
	if inputs == nil || inputs.Graph == nil {
		return nil
	}
	if g, ok := inputs.Graph.(*cfg.Graph); ok {
		return g.Bindings()
	}
	return nil
}

// RootFromSymbol returns the display name for a symbol, falling back to the provided name.
func RootFromSymbol(inputs *flow.Inputs, sym cfg.SymbolID, fallback string) string {
	if inputs == nil {
		return fallback
	}
	return rootNameFromSymbolSource(inputs.Graph, sym, fallback)
}

// ClassifyReturnExpr determines if a return expression returns true, false, or unknown.
func ClassifyReturnExpr(expr ast.Expr) flow.ReturnKind {
	switch e := expr.(type) {
	case *ast.TrueExpr:
		return flow.ReturnTrue
	case *ast.FalseExpr:
		return flow.ReturnFalse
	case *ast.IdentExpr:
		if e.Value == "true" {
			return flow.ReturnTrue
		}
		if e.Value == "false" {
			return flow.ReturnFalse
		}
	}
	return flow.ReturnUnknown
}

// ResolveSymbolToFunctionLiteral resolves a symbol to a function literal defined
// in the current graph (local/global function definitions or assignments).
func ResolveSymbolToFunctionLiteral(graph *cfg.Graph, sym cfg.SymbolID) *ast.FunctionExpr {
	if sym == 0 {
		return nil
	}
	var bindings *bind.BindingTable
	if graph != nil {
		bindings = graph.Bindings()
	}
	return callsite.FunctionLiteralForSymbol(graph, bindings, sym)
}

// ResolveExprToTableLiteral resolves expression to a table literal when possible.
// Supports direct table expressions and identifier references to local table literals.
func ResolveExprToTableLiteral(expr ast.Expr, graph *cfg.Graph) *ast.TableExpr {
	if expr == nil || graph == nil {
		return nil
	}

	if tbl, ok := expr.(*ast.TableExpr); ok {
		return tbl
	}

	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return nil
	}

	bindings := graph.Bindings()
	if bindings == nil {
		return nil
	}

	sym, found := bindings.SymbolOf(ident)
	if !found || sym == 0 {
		return nil
	}

	var tableLit *ast.TableExpr
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if tableLit != nil || info == nil || !info.IsLocal {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Symbol == sym && target.Kind == cfg.TargetIdent {
				if tbl, ok := source.(*ast.TableExpr); ok {
					tableLit = tbl
				}
			}
		})
	})

	return tableLit
}

// ResolveCalleeToFunctionLiteral resolves a callee expression to a function literal.
//
// Supported forms:
//   - direct function literal: function(...) ... end
//   - table field function via static table literal:
//     local t = { f = function(...) ... end }; t.f(...)
func ResolveCalleeToFunctionLiteral(callee ast.Expr, graph *cfg.Graph) *ast.FunctionExpr {
	if callee == nil {
		return nil
	}

	if fn, ok := callee.(*ast.FunctionExpr); ok {
		return fn
	}

	attr, ok := callee.(*ast.AttrGetExpr)
	if !ok || graph == nil {
		return nil
	}

	calleeSeg, ok := pathseg.StaticAttrKeySegment(attr.Key)
	if !ok {
		return nil
	}

	tableLit := ResolveExprToTableLiteral(attr.Object, graph)
	if tableLit == nil {
		return nil
	}

	for _, field := range tableLit.Fields {
		fieldSeg, ok := pathseg.StaticTableFieldKeySegment(field.Key)
		if !ok || fieldSeg != calleeSeg {
			continue
		}
		if fn, ok := field.Value.(*ast.FunctionExpr); ok {
			return fn
		}
	}

	return nil
}

// Ref resolves typ.Ref to its actual type using scope type lookup.
// This normalizes types before they enter flow inputs to avoid Ref vs Alias mismatches.
func Ref(t typ.Type, sc *scope.State) typ.Type {
	if t == nil || sc == nil {
		return t
	}
	if ref, ok := t.(*typ.Ref); ok {
		if resolved, ok := sc.LookupType(ref.Name); ok && resolved != nil {
			return resolved
		}
	}
	return t
}

// selectConcreteOrPlaceholder applies canonical symbol-resolver preference:
// return concrete types immediately, and keep placeholders as fallback.
func selectConcreteOrPlaceholder(candidate typ.Type, fallback *typ.Type) (typ.Type, bool) {
	if candidate == nil {
		return nil, false
	}
	if candidate.Kind().IsPlaceholder() {
		if fallback != nil {
			*fallback = candidate
		}
		return nil, false
	}
	return candidate, true
}

func selectFromTypeMap(types map[cfg.SymbolID]typ.Type, sym cfg.SymbolID, fallback *typ.Type) (typ.Type, bool) {
	if types == nil {
		return nil, false
	}
	candidate, ok := types[sym]
	if !ok {
		return nil, false
	}
	return selectConcreteOrPlaceholder(candidate, fallback)
}

// selectFromTypeMaps returns the first non-placeholder type for sym across maps,
// preserving the last placeholder as fallback when no concrete type exists.
func selectFromTypeMaps(sym cfg.SymbolID, fallback *typ.Type, maps ...map[cfg.SymbolID]typ.Type) (typ.Type, bool) {
	for _, m := range maps {
		if selected, ok := selectFromTypeMap(m, sym, fallback); ok {
			return selected, true
		}
	}
	return nil, false
}

// resolveGlobalOrFallback finalizes symbol resolution by preferring global type
// bindings and otherwise returning the placeholder fallback (if any).
func resolveGlobalOrFallback(ctx api.BaseEnv, sym cfg.SymbolID, fallback typ.Type) (typ.Type, bool) {
	if ctx != nil {
		if t, ok := ctx.GlobalType(sym); ok && t != nil {
			return t, true
		}
	}
	if fallback != nil {
		return fallback, true
	}
	return nil, false
}

// BuildContextSymbolResolver creates a symbol type resolver from Env.
func BuildContextSymbolResolver(ctx api.BaseEnv) func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	if ctx == nil || ctx.Types() == nil {
		return func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
			return nil, false
		}
	}
	return func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		var fallback typ.Type
		tv := ctx.Types().EffectiveTypeAt(p, sym)
		if tv.State == flow.StateResolved {
			if selected, ok := selectConcreteOrPlaceholder(tv.Type, &fallback); ok {
				return selected, true
			}
		}
		return resolveGlobalOrFallback(ctx, sym, fallback)
	}
}

// BuildInputSymbolResolver creates a symbol type resolver that prefers flow inputs
// (literal/sibling/declared types) before falling back to globals.
func BuildInputSymbolResolver(ctx api.BaseEnv, inputs *flow.Inputs) func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	if inputs == nil {
		return BuildContextSymbolResolver(ctx)
	}
	return func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		var fallback typ.Type
		if selected, ok := selectFromTypeMaps(
			sym,
			&fallback,
			inputs.LiteralTypes,
			inputs.SiblingTypes,
			inputs.DeclaredTypes,
		); ok {
			return selected, true
		}
		return resolveGlobalOrFallback(ctx, sym, fallback)
	}
}

// BuildContextTypeKeyResolver creates a type key resolver from Env.
func BuildContextTypeKeyResolver(ctx api.BaseEnv) func(string, *scope.State) (narrow.TypeKey, bool) {
	return func(name string, sc *scope.State) (narrow.TypeKey, bool) {
		if key, ok := narrow.KnownBuiltinTypeKey(name); ok {
			return key, true
		}
		if ctx != nil && ctx.TypeNames() != nil {
			if t, ok := ctx.TypeNames().LookupType(name); ok && t != nil {
				return narrow.HashTypeKey(t.Hash()), true
			}
		}
		if sc != nil {
			if t, ok := sc.LookupType(name); ok && t != nil {
				return narrow.HashTypeKey(t.Hash()), true
			}
		}
		return narrow.TypeKey{}, false
	}
}

// BuildRefinementLookup creates the refinement lookup function from Env.
// Returns symbol-based lookup only - all functions have symbols.
func BuildRefinementLookup(ctx api.BaseEnv) constraint.RefinementLookupBySym {
	if ctx == nil || ctx.Refinements() == nil {
		return nil
	}
	refinements := ctx.Refinements()
	return func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		return refinements.LookupBySym(sym)
	}
}

// SynthWithOverlay returns a synth function that resolves ident expressions via the
// overlay map, falling back to base for everything else.
func SynthWithOverlay(overlay map[cfg.SymbolID]typ.Type, bindings *bind.BindingTable, base func(ast.Expr, cfg.Point) typ.Type) func(ast.Expr, cfg.Point) typ.Type {
	return func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok {
			if bindings != nil {
				if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
					if t, exists := overlay[sym]; exists {
						return t
					}
				}
			}
		}
		return base(expr, p)
	}
}

// BuildAssignmentTypeResolver creates a resolver that looks up types from extracted assignments.
func BuildAssignmentTypeResolver(inputs *flow.Inputs) func(cfg.SymbolID) typ.Type {
	if inputs == nil {
		return nil
	}

	latestBySymbol := make(map[cfg.SymbolID]typ.Type)
	for i := len(inputs.Assignments) - 1; i >= 0; i-- {
		a := inputs.Assignments[i]
		sym := a.TargetPath.Symbol
		if sym == 0 || a.Type == nil {
			continue
		}
		if _, exists := latestBySymbol[sym]; !exists {
			latestBySymbol[sym] = a.Type
		}
	}

	return func(sym cfg.SymbolID) typ.Type {
		if sym == 0 {
			return nil
		}
		if t, ok := latestBySymbol[sym]; ok {
			return t
		}
		if t, ok := inputs.DeclaredTypes[sym]; ok {
			return t
		}
		return nil
	}
}

// IteratorSourceInfo captures iterator source data for generic for loops.
type IteratorSourceInfo struct {
	Path constraint.Path
	Kind flow.IteratorKind
}

// ExtractIteratorSource extracts iterator source info from generic for expressions.
// Returns nil if no iterator spec is found or source path is not resolvable.
func ExtractIteratorSource(
	iterExprs []ast.Expr,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	constResolver func(string) *flow.ConstValue,
	bindings *bind.BindingTable,
) *IteratorSourceInfo {
	if len(iterExprs) == 0 {
		return nil
	}

	call, ok := iterExprs[0].(*ast.FuncCallExpr)
	if !ok || call == nil {
		return nil
	}

	var fnType typ.Type
	if synth != nil && call.Func != nil && call.Method == "" && call.Receiver == nil {
		fnType = synth(call.Func, p)
	}
	if fnType == nil && symResolver != nil && call.Func != nil {
		if ident, ok := call.Func.(*ast.IdentExpr); ok && bindings != nil {
			if sym, found := bindings.SymbolOf(ident); found && sym != 0 {
				fnType, _ = symResolver(p, sym)
			}
		}
	}
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	var iterKind flow.IteratorKind
	idx := 0
	if spec != nil {
		iter := spec.GetIterator()
		if iter == nil {
			return nil
		}
		var ok bool
		idx, ok = effect.ResolveParamIndex(iter.Source, len(call.Args))
		if !ok {
			return nil
		}
		switch iter.Kind {
		case effect.IterateIndexed:
			iterKind = flow.IterateIndexed
		case effect.IterateKeyed:
			iterKind = flow.IterateKeyed
		default:
			return nil
		}
	} else {
		ident, ok := call.Func.(*ast.IdentExpr)
		if !ok || ident == nil {
			return nil
		}
		if bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if symKind, ok := bindings.Kind(sym); ok && symKind != cfg.SymbolGlobal {
					return nil
				}
			}
		}
		switch ident.Value {
		case "ipairs":
			iterKind = flow.IterateIndexed
		case "pairs":
			iterKind = flow.IterateKeyed
		default:
			return nil
		}
		idx = 0
	}

	if idx < 0 || idx >= len(call.Args) {
		return nil
	}
	sourceExpr := call.Args[idx]
	if sourceExpr == nil {
		return nil
	}

	sourcePath := path.FromExprWithBindings(sourceExpr, constResolver, bindings)
	if sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return nil
	}

	return &IteratorSourceInfo{Path: sourcePath, Kind: iterKind}
}

// CalleeType resolves the function type for a call site.
// For method calls, resolves the receiver type (via CalleePath.Symbol, assignmentTypes,
// symResolver, synth) and looks up the method. For non-method calls, synthesizes the
// callee directly. Symbol resolver lookup uses canonical callsite candidates with
// binding-table fallback.
func CalleeType(
	info *cfg.CallInfo,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	assignmentTypes func(cfg.SymbolID) typ.Type,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) typ.Type {
	if info == nil {
		return nil
	}

	var fnType typ.Type

	if callsite.IsMethodCallInfo(info) {
		var receiverType typ.Type

		// Use CalleePath.Symbol as primary identity for receiver
		receiverSym := info.CalleePath.Symbol
		if receiverSym == 0 {
			receiverSym = info.ReceiverSymbol
		}

		if receiverSym != 0 && assignmentTypes != nil {
			receiverType = assignmentTypes(receiverSym)
		}

		if receiverType == nil && receiverSym != 0 && symResolver != nil {
			if rt, ok := symResolver(p, receiverSym); ok && rt != nil {
				receiverType = rt
			}
		}

		if receiverType == nil && synth != nil {
			receiverType = synth(info.Receiver, p)
		}

		if !typ.IsAbsentOrUnknown(receiverType) {
			fnType, _ = core.Method(receiverType, info.Method)
		}
	} else if synth != nil && info.Callee != nil {
		fnType = synth(info.Callee, p)
	}

	if fnType == nil && symResolver != nil {
		for _, sym := range callsite.ResolverCalleeSymbolCandidates(info, graph, bindings, moduleBindings) {
			if t, ok := symResolver(p, sym); ok && t != nil {
				fnType = t
				break
			}
		}
	}

	return fnType
}

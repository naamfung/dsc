// Package extract implements the core expression type synthesis engine.
//
// The synthesizer traverses AST expressions and computes their types based on:
//   - Literal values (nil, true, false, numbers, strings)
//   - Variable lookups (identifiers resolved via scope and flow state)
//   - Field/method access (attribute get expressions)
//   - Function calls (with generic instantiation and method dispatch)
//   - Table constructors (with bidirectional type checking)
//   - Operators (arithmetic, logical, relational, etc.)
//
// The synthesizer operates in two modes:
//
// Pre-flow (api.PhaseScopeCompute / api.PhaseTypeResolution):
// Uses declared types and structural inference without flow analysis.
//
// Narrowing (api.PhaseNarrowing):
// Incorporates flow-sensitive narrowing from control flow analysis.
// Type guards, assignments, and predicates refine types at each program point.
//
// Caching: The synthesizer uses two caches to avoid redundant computation:
//   - PreCache: Stores results from declared phase (base types)
//   - NarrowCache: Stores results from narrowed phase (flow-refined types)
package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/resolve"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExprSynth is the function signature for recursive expression synthesis.
// Used as callback to allow synthExprCore to recursively synthesize sub-expressions.
type ExprSynth func(expr ast.Expr) typ.Type

// Synthesizer is the core type synthesis engine for expressions.
//
// It implements a recursive descent over the AST, computing types for each
// expression kind. The synthesizer maintains dependencies (Deps) for accessing
// scope state, type queries, flow information, and caches.
//
// Key methods:
//   - TypeOf: Main entry point for expression synthesis
//   - SynthExpr: Core synthesis with optional flow narrowing
//   - SynthCallCore: Function call synthesis with generic handling
//   - SynthTableCore: Table constructor synthesis
//   - SynthFunctionType: Function signature extraction
type Synthesizer struct {
	deps  *Deps
	phase api.Phase
}

// NewSynthesizer creates a new extract synthesizer.
func NewSynthesizer(deps *Deps, phase api.Phase) *Synthesizer {
	return &Synthesizer{
		deps:  deps,
		phase: phase,
	}
}

// IsNarrowing reports whether the synthesizer is in the narrowed phase.
func (s *Synthesizer) IsNarrowing() bool {
	return s.phase == api.PhaseNarrowing
}

// Phase returns the current synthesis phase.
func (s *Synthesizer) Phase() api.Phase {
	return s.phase
}

// Context returns the query context for type database operations.
func (s *Synthesizer) Context() *db.QueryContext {
	return s.deps.Ctx
}

// Scopes returns the CFG point to scope state mapping.
func (s *Synthesizer) Scopes() api.ScopeMap {
	return s.deps.Scopes
}

// AllowReturnTransforms reports whether return transforms are enabled.
func (s *Synthesizer) AllowReturnTransforms() bool {
	return s.IsNarrowing()
}

// CallQuery returns the type operations interface for call resolution.
func (s *Synthesizer) CallQuery() core.TypeOps {
	return s.GetCallQuery()
}

// Entry returns the CFG entry point for the current function graph.
func (s *Synthesizer) Entry() cfg.Point {
	return s.deps.Entry()
}

// Deps returns the underlying dependencies.
func (s *Synthesizer) Deps() *Deps {
	return s.deps
}

// TypeOf synthesizes the type of an expression at a CFG point (no narrowing).
func (s *Synthesizer) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if cached, ok := s.deps.PreCache.Get(expr, p); ok {
		return cached
	}
	t := s.SynthExpr(expr, p, nil)
	s.deps.PreCache.Put(expr, p, t)
	return t
}

// TypeOfWithExpected synthesizes expression type with expected type context (no narrowing).
func (s *Synthesizer) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	if expected == nil {
		return s.TypeOf(expr, p)
	}
	sc := s.deps.ScopeAt(p)
	recurse := func(ex ast.Expr) typ.Type { return s.SynthExpr(ex, p, nil) }
	return s.SynthExprWithExpectedCore(expr, sc, p, recurse, expected)
}

// MultiTypeOf synthesizes multiple types for multi-value expressions (no narrowing).
func (s *Synthesizer) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	return s.multiTypeOf(expr, p, nil)
}

// SynthMulti synthesizes multiple types for multi-value expressions with optional flow narrowing.
func (s *Synthesizer) SynthMulti(expr ast.Expr, p cfg.Point, narrower api.FlowOps) []typ.Type {
	return s.multiTypeOf(expr, p, narrower)
}

func (s *Synthesizer) multiTypeOf(expr ast.Expr, p cfg.Point, narrower api.FlowOps) []typ.Type {
	sc := s.deps.ScopeAt(p)
	recurse := func(ex ast.Expr) typ.Type { return s.SynthExpr(ex, p, narrower) }
	return s.synthMultiCore(expr, sc, recurse,
		func(call *ast.FuncCallExpr) []typ.Type {
			return s.SynthCallCore(call, p, sc, narrower, recurse)
		},
	)
}

// ExpandValues expands expression list to needed count (no narrowing).
func (s *Synthesizer) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	return s.expandValues(exprs, needed, p, nil)
}

// ExpandValuesWithSpecTypes expands expression list with spec-narrowed type lookup.
func (s *Synthesizer) ExpandValuesWithSpecTypes(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	if len(specTypes) == 0 {
		return s.expandValues(exprs, needed, p, nil)
	}
	return s.expandValuesWithSpec(exprs, needed, p, specTypes)
}

// InferIterVars infers iterator variable types (no narrowing).
func (s *Synthesizer) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	return s.inferIterVars(exprs, count, p, nil)
}

// InferIterVarsWithSpecTypes infers iterator variable types with overlay lookup.
func (s *Synthesizer) InferIterVarsWithSpecTypes(exprs []ast.Expr, count int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	if len(specTypes) == 0 {
		return s.inferIterVars(exprs, count, p, nil)
	}
	return s.inferIterVarsWithSpec(exprs, count, p, specTypes)
}

// SynthExprAt synthesizes the type of an expression at a CFG point using scope.
func (s *Synthesizer) SynthExprAt(expr ast.Expr, p cfg.Point, sc *scope.State) typ.Type {
	if expr == nil {
		return typ.Nil
	}
	recurse := func(ex ast.Expr) typ.Type { return s.SynthExprAt(ex, p, sc) }
	return s.synthExprCore(expr, sc, p, nil, recurse)
}

// Resolver returns a type resolver.
func (s *Synthesizer) Resolver() *resolve.Resolver {
	return resolve.New(resolve.Config{
		Manifests: s.deps.Manifests,
		ExprSynth: func(expr ast.Expr, p cfg.Point) typ.Type {
			return s.SynthExpr(expr, p, nil)
		},
		Bindings:       s.deps.ModuleBindings,
		ModuleBindings: s.deps.ModuleBindings,
		ModuleAliases:  s.deps.ModuleAliases,
	})
}

// ResolveType resolves a type expression to a concrete type.
func (s *Synthesizer) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	return s.Resolver().ResolveType(expr, sc)
}

// ResolveReturnTypes resolves multiple return type expressions, expanding tuples.
func (s *Synthesizer) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	return s.Resolver().ResolveReturnTypes(types, sc)
}

// ResolveFunctionSignature builds a function type from annotations without body inference.
func (s *Synthesizer) ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return s.Resolver().ResolveFunctionSignature(fn, sc)
}

// ResolveTypeDef resolves a type definition, handling generic type parameters.
func (s *Synthesizer) ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
	return s.Resolver().ResolveTypeDef(name, typeExpr, typeParams, sc)
}

// SynthExpr synthesizes the type of an expression using CFG point for scope lookup.
func (s *Synthesizer) SynthExpr(expr ast.Expr, p cfg.Point, narrower api.FlowOps) typ.Type {
	if expr == nil {
		return typ.Nil
	}
	sc := s.deps.ScopeAt(p)
	recurse := func(ex ast.Expr) typ.Type { return s.SynthExpr(ex, p, narrower) }
	return s.synthExprCore(expr, sc, p, narrower, recurse)
}

// synthExprCore is the shared expression synthesizer implementation.
func (s *Synthesizer) synthExprCore(expr ast.Expr, sc *scope.State, p cfg.Point, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	switch ex := expr.(type) {
	case *ast.NilExpr:
		return typ.Nil
	case *ast.TrueExpr:
		return typ.True
	case *ast.FalseExpr:
		return typ.False
	case *ast.NumberExpr:
		return ops.ParseNumber(ex.Value)
	case *ast.StringExpr:
		return typ.LiteralString(ex.Value)
	case *ast.Comma3Expr:
		return s.synthComma3(sc)
	case *ast.IdentExpr:
		return s.synthIdentCore(ex, p, sc, narrower)
	case *ast.AttrGetExpr:
		return s.synthAttrGetCore(ex, p, sc, narrower, recurse)
	case *ast.TableExpr:
		return s.SynthTableCore(ex, sc, recurse)
	case *ast.FuncCallExpr:
		types := s.SynthCallCore(ex, p, sc, narrower, recurse)
		if len(types) > 0 {
			return types[0]
		}
		return typ.Nil
	case *ast.FunctionExpr:
		return s.FunctionType(ex, sc)
	case *ast.LogicalOpExpr:
		if s.IsNarrowing() && narrower != nil {
			return s.synthLogicalOpWithNarrowing(ex, p, sc, narrower, recurse)
		}
		return s.synthLogicalOpCore(ex, recurse)
	case *ast.RelationalOpExpr:
		return typ.Boolean
	case *ast.StringConcatOpExpr:
		return typ.String
	case *ast.ArithmeticOpExpr:
		return s.synthArithmeticOpCore(ex, recurse)
	case *ast.UnaryMinusOpExpr:
		return s.synthUnaryMinusCore(ex, recurse)
	case *ast.UnaryNotOpExpr:
		return typ.Boolean
	case *ast.UnaryLenOpExpr:
		operand := recurse(ex.Expr)
		return s.deps.Types.UnaryOp(s.deps.Ctx, "#", operand)
	case *ast.UnaryBNotOpExpr:
		return typ.Integer
	case *ast.CastExpr:
		return s.ResolveType(ex.Type, sc)
	case *ast.NonNilAssertExpr:
		inner := recurse(ex.Expr)
		return narrow.RemoveNil(inner)
	default:
		return typ.Unknown
	}
}

// synthMultiCore synthesizes multiple types for multi-value expressions using provided functions.
func (s *Synthesizer) synthMultiCore(expr ast.Expr, sc *scope.State, synthSingle func(ast.Expr) typ.Type, synthCall func(*ast.FuncCallExpr) []typ.Type) []typ.Type {
	if expr == nil {
		return nil
	}

	switch ex := expr.(type) {
	case *ast.FuncCallExpr:
		return synthCall(ex)
	case *ast.Comma3Expr:
		if vt := sc.VariadicType(); vt != nil {
			return []typ.Type{vt}
		}
		return []typ.Type{typ.Unknown}
	default:
		return []typ.Type{synthSingle(expr)}
	}
}

// synthIdentCore synthesizes type for an identifier.
func (s *Synthesizer) synthIdentCore(ex *ast.IdentExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	ctx := s.deps.CheckCtx
	if ctx == nil {
		return typ.Unknown
	}

	var sym cfg.SymbolID
	var moduleSym cfg.SymbolID
	if bindings := ctx.Bindings(); bindings != nil {
		sym, _ = bindings.SymbolOf(ex)
	}
	if sym == 0 {
		if graph := ctx.Graph(); graph != nil {
			if resolved, ok := graph.SymbolAt(p, ex.Value); ok && resolved != 0 {
				sym = resolved
			}
		}
	}
	if s.deps.ModuleBindings != nil {
		moduleSym, _ = s.deps.ModuleBindings.SymbolOf(ex)
		if sym == 0 {
			sym = moduleSym
		}
	}

	if sym == 0 {
		if ctx != nil {
			if globalTypes := ctx.GlobalTypes(); globalTypes != nil {
				if t, ok := globalTypes[ex.Value]; ok && t != nil {
					return t
				}
			}
		}
		if sc != nil {
			if t, ok := sc.LookupType(ex.Value); ok && t != nil {
				return typ.NewMeta(t)
			}
		}
		return typ.Unknown
	}

	// For "self" identifier, check scope's self type first.
	// This ensures methods assigned via field assignment (obj.method = function(self)...)
	// get the correct self type before falling back to parameter type lookup.
	if ex.Value == "self" && sc != nil {
		if selfType := sc.SelfType(); selfType != nil {
			return selfType
		}
	}

	if s.IsNarrowing() && narrower != nil {
		path := constraint.Path{Root: ex.Value, Symbol: sym}
		if narrowed := narrower.NarrowedTypeAt(p, path); narrowed != nil {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, narrowed, nil); specialized != nil {
				return specialized
			}
			// Guard against unsound narrowing for annotated symbols by ensuring
			// the narrowed type remains a subtype of the declared type. Function
			// signatures from declared overlays are also authoritative and should
			// never be widened by flow facts.
			if types := ctx.Types(); types != nil {
				declared := types.DeclaredAt(p, sym)
				if declared.Type != nil && declared.State == flow.StateResolved {
					if typ.IsAny(unwrap.Alias(declared.Type)) && typ.IsUnknown(unwrap.Alias(narrowed)) {
						return declared.Type
					}
					requireSubtype := types.IsAnnotated(sym)
					if !requireSubtype {
						requireSubtype = unwrap.Function(declared.Type) != nil
					}
					if requireSubtype && !subtype.IsSubtype(narrowed, declared.Type) {
						goto fallback
					}
				}
			}
			return narrowed
		}
	}

fallback:
	if types := ctx.Types(); types != nil {
		tv := types.EffectiveTypeAt(p, sym)
		if tv.State == flow.StateResolved && tv.Type != nil {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, tv.Type, nil); specialized != nil {
				return specialized
			}
			// Prefer concrete resolved types over module aliases.
			// Allow module aliases to override unknown/any placeholders.
			if tv.Type.Kind().IsPlaceholder() {
				if types.IsAnnotated(sym) {
					declared := types.DeclaredAt(p, sym)
					if declared.State == flow.StateResolved && declared.Type != nil {
						if typ.IsAny(unwrap.Alias(declared.Type)) {
							return declared.Type
						}
					}
				}
				// defer to module alias below if available
			} else {
				return tv.Type
			}
		}
		if moduleSym != 0 && moduleSym != sym {
			moduleTV := types.EffectiveTypeAt(p, moduleSym)
			if moduleTV.State == flow.StateResolved && moduleTV.Type != nil {
				if specialized := s.stableLocalFunctionValueType(ex, p, sc, moduleTV.Type, nil); specialized != nil {
					return specialized
				}
				if moduleTV.Type.Kind().IsPlaceholder() {
					// keep looking for better sources
				} else {
					return moduleTV.Type
				}
			}
		}
	}

	// Module alias lookup (require("mod")) as fallback when no concrete type is resolved.
	moduleAliasSym := sym
	if moduleAliasSym == 0 {
		moduleAliasSym = moduleSym
	}
	if modulePath := ctx.ModuleAlias(moduleAliasSym); modulePath != "" {
		if exportType := io.LookupEnrichedExport(s.deps.Manifests, modulePath); exportType != nil {
			return exportType
		}
	}

	if types := ctx.Types(); types != nil {
		tv := types.EffectiveTypeAt(p, sym)
		if tv.State == flow.StateResolved && tv.Type != nil {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, tv.Type, nil); specialized != nil {
				return specialized
			}
			return tv.Type
		}
		if moduleSym != 0 && moduleSym != sym {
			moduleTV := types.EffectiveTypeAt(p, moduleSym)
			if moduleTV.State == flow.StateResolved && moduleTV.Type != nil {
				if specialized := s.stableLocalFunctionValueType(ex, p, sc, moduleTV.Type, nil); specialized != nil {
					return specialized
				}
				return moduleTV.Type
			}
		}
	}

	if t, ok := ctx.GlobalType(sym); ok && t != nil {
		return t
	}
	if moduleSym != 0 && moduleSym != sym {
		if t, ok := ctx.GlobalType(moduleSym); ok && t != nil {
			return t
		}
	}

	// Type names used in expression context (e.g. Config = Config in a return
	// table) resolve to a Meta type wrapping the declared type. This enables
	// type values to flow through module exports as first-class values,
	// supporting patterns like mylib.Config:is(data) across module boundaries.
	if sc != nil {
		if t, ok := sc.LookupType(ex.Value); ok && t != nil {
			return typ.NewMeta(t)
		}
	}

	return typ.Unknown
}

// synthComma3 synthesizes type for varargs (...).
func (s *Synthesizer) synthComma3(sc *scope.State) typ.Type {
	if sc != nil {
		if vt := sc.VariadicType(); vt != nil {
			return vt
		}
	}
	return typ.Unknown
}

// SynthExprWithExpectedCore synthesizes expression with expected type context.
func (s *Synthesizer) SynthExprWithExpectedCore(expr ast.Expr, sc *scope.State, p cfg.Point, recurse ExprSynth, expected typ.Type) typ.Type {
	if _, ok := unwrap.Alias(expected).(*typ.Union); ok {
		return s.synthExprWithUnionExpected(expr, sc, p, recurse, expected)
	}
	return s.synthExprWithExpectedSingle(expr, sc, p, recurse, expected)
}

// LookupSymbol resolves symbol from bindings for an identifier.
func (s *Synthesizer) LookupSymbol(ident *ast.IdentExpr) cfg.SymbolID {
	if s.deps.CheckCtx == nil {
		return 0
	}
	if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
		sym, _ := bindings.SymbolOf(ident)
		return sym
	}
	return 0
}

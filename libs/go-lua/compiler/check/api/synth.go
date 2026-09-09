// Package api defines canonical interfaces for the type synthesis subsystem.
// These interfaces decouple the synthesis engine implementation (synth.Engine)
// from its consumers (hooks, flowbuild, phase runners, etc.).
//
// # INTERFACE HIERARCHY
//
// The synthesis interfaces form a hierarchy based on capabilities:
//
//	BaseSynth: Core type synthesis operations
//	  - TypeOf: Single expression type
//	  - MultiTypeOf: Multi-return expression types
//	  - FunctionType: Function signature synthesis
//	  - ResolveType: Type annotation resolution
//
//	Synth: Full synthesis with narrowing support
//	  - All BaseSynth methods
//	  - Narrow(): Returns narrowed-phase synthesis
//	  - Method/Field: Type member access
//	  - ResolveFunctionSignature: Full signature resolution
//
//	FlowQuery: Flow-narrowed type queries at CFG points
//	  - EffectiveTypeAt: Narrowed type for symbol at point
//	  - ExcludesTypeAt: Type exclusion from flow analysis
//
//	FlowOps: Advanced flow operations for constraint solving
//	  - NarrowedTypeAt: Path-based narrowed type
//	  - BoundsAt: Numeric bounds from flow analysis
//	  - IsPointDead: Reachability check
//
// # PHASE SEPARATION
//
// Synthesis operates in two phases:
//
//	Declared: Uses declared types from annotations only. No flow refinements.
//	Narrowing: Uses flow-refined types when available, falls back to declared.
//
// BaseSynth is the return type of Narrow() to prevent recursive interface
// definitions while supporting phase-aware synthesis.
package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// BaseSynth is the core type synthesis interface for expression type queries.
// It provides essential operations needed for type checking without phase-specific
// concerns. Used as the return type for Narrow() to avoid recursive interface definitions.
//
// EXPRESSION SYNTHESIS:
//   - TypeOf: Returns the type of a single expression at a CFG point
//   - TypeOfWithExpected: Type synthesis with expected type for bidirectional inference
//   - MultiTypeOf: Returns all return types for multi-return expressions (calls, varargs)
//
// VALUE EXPANSION:
//   - ExpandValues: Expands a list of expressions to a specified count (for assignments)
//   - InferIterVars: Infers iterator variable types for for-in loops
//
// TYPE RESOLUTION:
//   - ResolveType: Resolves a type annotation expression to a concrete type
//   - ResolveReturnTypes: Resolves return type annotations for functions
//   - FunctionType: Synthesizes the complete type for a function expression
type BaseSynth interface {
	// TypeOf returns the type of expr at CFG point p.
	// For multi-return expressions, returns only the first return type.
	TypeOf(expr ast.Expr, p cfg.Point) typ.Type

	// TypeOfWithExpected returns the type of expr with expected type context.
	// Used for bidirectional type inference (e.g., literal table with expected record type).
	TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type

	// MultiTypeOf returns all return types for multi-return expressions.
	// For single-return expressions, returns a single-element slice.
	MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type

	// FunctionType computes the complete function type including params and returns.
	FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function

	// ExpandValues expands expressions to the needed count for multi-assign.
	// Last expression may contribute multiple values (call, vararg).
	ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type

	// InferIterVars infers types for iterator variables in for-in loops.
	InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type

	// ResolveType resolves a type annotation expression to a concrete type.
	ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type

	// ResolveReturnTypes resolves return type annotations for a function.
	ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type
}

// Synth is the full type synthesis interface with phase awareness and member access.
// Implemented by synth.Engine and used throughout the type checking pipeline.
//
// PHASE AWARENESS: Synth supports both declared and narrowed phases through the
// Narrow() method which returns a BaseSynth operating in narrowed mode.
//
// MEMBER ACCESS: Method() and Field() provide type-aware member lookup supporting
// records, interfaces, classes, and metatables.
//
// TYPE RESOLUTION: ResolveFunctionSignature and ResolveTypeDef handle complex
// type constructs including generic type parameters.
type Synth interface {
	BaseSynth

	// ResolveFunctionSignature resolves a function's complete signature including
	// type parameters, parameter types, return types, and effects.
	ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function

	// ResolveTypeDef resolves a type definition with optional type parameters.
	// Used for @type aliases and class definitions.
	ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type

	// Narrow returns a BaseSynth operating in narrowed phase.
	// Narrowed synthesis uses flow-refined types when available.
	Narrow() BaseSynth

	// Method returns the type of a method on a type, if it exists.
	// Handles records, interfaces, classes, and metatable __index methods.
	Method(t typ.Type, name string) (typ.Type, bool)

	// Field returns the type of a field on a type, if it exists.
	// Handles records, interfaces, and map component access.
	Field(t typ.Type, name string) (typ.Type, bool)

	// SynthWithExpected synthesizes with expected type for bidirectional inference.
	SynthWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type

	// CallQuery returns the TypeOps for call resolution and type operations.
	CallQuery() core.TypeOps

	// AllowReturnTransforms returns whether spec-based return transforms are enabled.
	AllowReturnTransforms() bool

	// Context returns the query context for database operations.
	Context() *db.QueryContext
}

// FlowQuery provides flow-narrowed type queries for use during synthesis.
// These queries access the flow solution to determine effective types after
// control-flow-based narrowing (type guards, nil checks, etc.).
type FlowQuery interface {
	// EffectiveTypeAt returns the effective type of a symbol at a CFG point.
	// Combines declared type with flow refinements.
	EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue

	// NarrowedTypeAt returns the exact narrowed type for a source path at a point.
	// Used when diagnostics need the solved path-sensitive type, not just symbol-level facts.
	NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type

	// ExcludesTypeAt checks if flow analysis excludes a type at a point.
	// Used for narrowing union types when branches eliminate possibilities.
	ExcludesTypeAt(p cfg.Point, path constraint.Path, declared typ.Type) bool
}

// FlowOps provides advanced flow operations for constraint solving and narrowing.
// Used by the flow solver and narrowing phase for fine-grained type refinement.
type FlowOps interface {
	// NarrowedTypeAt returns the narrowed type for a path at a point.
	// Paths may include field accesses (e.g., x.y.z).
	NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type

	// BoundsAt returns numeric bounds for a variable at a point.
	// Used for array length inference and index bounds checking.
	BoundsAt(p cfg.Point, name string) (lower, upper int64, ok bool)

	// ArrayLenBoundAt returns the array variable whose length bounds this variable.
	// Used for index-length relationship tracking.
	ArrayLenBoundAt(p cfg.Point, varName string) (arrKey string, ok bool)

	// ArrayLenBoundWithOffsetAt returns array variable and offset for symbolic bound:
	// varName <= len(array) + offset.
	ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (arrKey string, offset int64, ok bool)

	// IsPointDead returns whether a CFG point is unreachable.
	IsPointDead(p cfg.Point) bool

	// HasKeyOf checks if table contains a key from another path.
	// Used for key-existence narrowing after table access patterns.
	HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool
}

// LiteralSynth provides synthesis capabilities for function literals.
type LiteralSynth interface {
	TypeOf(expr ast.Expr, p cfg.Point) typ.Type
	SynthFunctionTypeWithExpected(fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function) *typ.Function
	Scopes() ScopeMap
	Entry() cfg.Point
}

// ScopeMap provides scope state at CFG points.
type ScopeMap = map[cfg.Point]*scope.State

// ExprSynth synthesizes types for expressions.
type ExprSynth = func(ast.Expr, cfg.Point) typ.Type

// PathFromExprFunc builds a flow path from an expression at a CFG point.
type PathFromExprFunc func(p cfg.Point, expr ast.Expr, sc *scope.State) constraint.Path

// SynthAPI provides type synthesis operations for flow extraction.
// synth.Engine satisfies this interface directly.
type SynthAPI interface {
	TypeOf(expr ast.Expr, p cfg.Point) typ.Type
	ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type
	InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type
	ExpandValuesWithSpecTypes(exprs []ast.Expr, needed int, p cfg.Point, specTypes SpecTypes) []typ.Type
	InferIterVarsWithSpecTypes(exprs []ast.Expr, count int, p cfg.Point, specTypes SpecTypes) []typ.Type
}

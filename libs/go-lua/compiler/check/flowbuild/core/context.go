// Package base provides shared context types for flowbuild operations.
package core

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// FlowContext provides all dependencies needed for flow constraint extraction.
// This unified context replaces multiple Config types that had overlapping fields.
type FlowContext struct {
	// Core graph and scope data
	Graph  *cfg.Graph
	Scopes map[cfg.Point]*scope.State

	// Type checking environment
	CheckCtx api.BaseEnv

	// Query context for memoization
	CallCtx *db.QueryContext

	// Type operations
	TypeOps core.TypeOps

	// Synthesis API for expression type queries
	API api.SynthAPI

	// Base scope for the function
	Base *scope.State

	// Global type namespace
	Globals map[string]typ.Type

	// Services provides signature and type expression resolution.
	Services FlowServices

	// Pre-computed type facts
	InitialDeclaredTypes flow.DeclaredTypes
	SiblingTypes         map[cfg.SymbolID]typ.Type
	LiteralTypes         map[cfg.SymbolID]typ.Type

	// Module-level data
	ModuleAliases  map[cfg.SymbolID]string
	ModuleBindings *bind.BindingTable

	// Derived holds computed resolvers (populated by flowbuild.Run).
	Derived *Derived
}

// FlowServices defines required resolution services for flow extraction.
type FlowServices interface {
	ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function
	ResolveTypeExpr(expr ast.TypeExpr, sc *scope.State) typ.Type
}

// Derived contains computed helpers populated during flow extraction.
// These are intentionally separated to keep FlowContext immutable.
type Derived struct {
	Synth           func(ast.Expr, cfg.Point) typ.Type
	SymResolver     func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
	TypeKeyRes      func(string, *scope.State) (narrow.TypeKey, bool)
	RefinementBySym constraint.RefinementLookupBySym
}

// FlowServicesFuncs adapts function fields to FlowServices.
type FlowServicesFuncs struct {
	FnSigResolver    func(*ast.FunctionExpr, *scope.State) *typ.Function
	TypeExprResolver func(ast.TypeExpr, *scope.State) typ.Type
}

func (f FlowServicesFuncs) ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	if f.FnSigResolver == nil {
		return nil
	}
	return f.FnSigResolver(fn, sc)
}

func (f FlowServicesFuncs) ResolveTypeExpr(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if f.TypeExprResolver == nil {
		return nil
	}
	return f.TypeExprResolver(expr, sc)
}

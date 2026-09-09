// Package nested provides pure helper functions for nested function discovery in Lua.
//
// Lua allows functions to be defined anywhere: as local statements, inside
// table constructors, as method definitions, or as anonymous values. This
// package provides utilities for discovering and classifying these nested
// function definitions.
//
// # Data Types
//
// This package exports data types that describe nested functions:
//   - Child: A discovered nested function with its identity resolved
//   - FuncInfo: A Child extended with its synthesized function type
//   - ScopeGroup: A group of functions sharing the same parent scope
//
// # Pure Functions
//
// All functions in this package are pure: they take inputs and produce outputs
// without side effects. The orchestration of nested function processing is
// handled by the check package, which uses these helpers.
//
// # Scope Groups
//
// Functions defined at the same lexical level share a scope group. This enables
// mutual recursion: function A can call function B, and B can call A, even if
// B is defined after A in the source code.
//
// # Constructor Pattern
//
// The package detects the Lua OOP constructor pattern (see constructor.go):
//
//	function T.new()
//	    local self = setmetatable({}, T)
//	    self.field = value  -- These become instance fields
//	    return self
//	end
//
// # Self Type Resolution
//
// Helper functions support self-type resolution for methods:
//   - FindTableLiteralOwner: For table literal methods
//   - FindFieldAssignmentBase: For field assignment methods
//   - EnrichSelfTypeWithConstructorFields: For constructor-enriched self
package nested

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// Child holds the resolved identity of a nested function definition.
//
// Each Child represents a function discovered in the parent function's CFG.
// The Child captures the function's definition point, enclosing scope, and
// identity (name, symbol, locality).
type Child struct {
	NF       cfg.NestedFunc
	DefScope *scope.State
	FuncDef  *cfg.FuncDefInfo
	FuncName string
	FuncSym  cfg.SymbolID
	IsLocal  bool
}

// FuncInfo extends Child with data needed during scope group processing.
type FuncInfo struct {
	Child
}

// ScopeGroup holds a group of functions sharing the same parent scope.
//
// Functions in a scope group can see each other's types during analysis,
// enabling mutual recursion. The group is processed as a unit: sibling
// types are computed, then each function is checked with that context.
type ScopeGroup struct {
	Hash     uint64
	Funcs    []*FuncInfo
	MinPoint cfg.Point
}

// Store provides access to session storage for enrichment functions.
//
// This interface abstracts the SessionStore, providing access to literal
// signatures and constructor fields without depending on the session package.
type Store interface {
	LookupConstructorFields(classSym cfg.SymbolID) map[string]typ.Type
}

// GatherChildren iterates the graph's nested functions and resolves each one's
// definition scope and identity.
//
// For each nested function in the CFG, this function:
//   - Looks up the definition scope at the function's point
//   - Retrieves any FuncDefInfo (for named function definitions)
//   - Resolves the function's name, symbol, and locality
//
// The result is a slice of Child structs ready for grouping and processing.
func GatherChildren(graph *cfg.Graph, scopes map[cfg.Point]*scope.State, fallback *scope.State) []Child {
	if graph == nil {
		return nil
	}
	nestedFuncs := graph.NestedFunctions()
	if len(nestedFuncs) == 0 {
		return nil
	}
	children := make([]Child, 0, len(nestedFuncs))
	for _, nf := range nestedFuncs {
		if nf.Func == nil {
			continue
		}
		defScope := scopes[nf.Point]
		if defScope == nil {
			defScope = fallback
		}
		funcDef := graph.FuncDef(nf.Point)
		funcName, funcSym, isLocal := ResolveNestedFuncIdentity(graph, nf, funcDef)
		children = append(children, Child{
			NF:       nf,
			DefScope: defScope,
			FuncDef:  funcDef,
			FuncName: funcName,
			FuncSym:  funcSym,
			IsLocal:  isLocal,
		})
	}
	return children
}

// ResolveNestedFuncIdentity determines the name, symbol, and locality of a nested function.
//
// The identity is resolved by checking (in order):
//  1. FuncDefInfo: Named function definitions provide name, symbol, and locality
//  2. Local assignment: `local f = function()` provides target name and symbol
//  3. NestedFunc symbol: Anonymous functions may still have an assigned symbol
//
// Returns the function name (may be empty), symbol ID (may be 0), and whether
// the function is locally scoped.
func ResolveNestedFuncIdentity(graph *cfg.Graph, nf cfg.NestedFunc, funcDef *cfg.FuncDefInfo) (string, cfg.SymbolID, bool) {
	if funcDef != nil {
		return funcDef.Name, funcDef.Symbol, funcDef.TargetKind == cfg.FuncDefGlobal
	}
	if assignInfo := graph.Assign(nf.Point); assignInfo != nil && assignInfo.IsLocal {
		if len(assignInfo.Targets) == 1 && assignInfo.Targets[0].Kind == cfg.TargetIdent {
			if len(assignInfo.Sources) == 1 && assignInfo.Sources[0] == nf.Func {
				return assignInfo.Targets[0].Name, assignInfo.Targets[0].Symbol, true
			}
		}
	}
	if nf.Symbol != 0 {
		return "", nf.Symbol, true
	}
	return "", 0, false
}

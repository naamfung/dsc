// Package siblings provides sibling type construction for nested function analysis.
//
// Sibling types are function types visible to other functions in the same scope group,
// enabling mutual recursion and forward references within a scope block. When multiple
// functions are defined at the same scope level (e.g., in the same do block), they
// form a sibling group that can call each other.
//
// # Problem Statement
//
// Consider this Lua code:
//
//	local function even(n) return n == 0 or odd(n-1) end
//	local function odd(n)  return n ~= 0 and even(n-1) end
//
// Without sibling type propagation, `even` cannot see `odd`'s type (not yet defined),
// and vice versa. This package enables both functions to see each other's types
// during type checking by computing a unified sibling type map for the group.
//
// # Build Algorithm
//
// The Build function constructs sibling types through four steps:
//  1. Seed from previous iteration (monotonic accumulation across fixpoint iterations)
//  2. Merge captured variable types from the parent scope
//  3. Add sibling function types enriched with return summaries
//  4. Overlay literal signatures for refined function types
//
// The result is a SymbolID -> Type map that can be injected into the type environment
// when analyzing any function in the group.
//
// # Integration with Fixpoint
//
// Sibling types are recomputed on each fixpoint iteration as return summaries improve.
// The monotonic accumulation (step 1) ensures that types only grow more precise,
// guaranteeing convergence.
package siblings

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FuncEntry captures the minimum info needed for sibling type construction.
//
// Each entry represents a function defined in the scope group. The entry includes
// the AST node, CFG location, symbol identity, locality flag, and synthesized
// function type. Non-local functions (e.g., module-level definitions) are included
// for captured variable resolution but do not contribute their types to siblings.
type FuncEntry struct {
	Func    *ast.FunctionExpr
	Point   cfg.Point
	Symbol  cfg.SymbolID
	IsLocal bool
}

// BuildConfig holds inputs for sibling type construction.
//
// This configuration bundles all dependencies needed by the Build function.
// The services (captured symbols, type lookup, record enrichment) are provided
// by the checker session to avoid tight coupling between packages.
type BuildConfig struct {
	// Funcs are the function entries in this scope group.
	Funcs []FuncEntry

	// GroupHash identifies the scope group.
	GroupHash uint64

	// SiblingTypesPrev are sibling types from the previous iteration (monotonic accumulation).
	SiblingTypesPrev map[cfg.SymbolID]typ.Type

	// FuncTypes are canonical local function types for this scope group.
	FuncTypes map[cfg.SymbolID]typ.Type

	// Services provides required lookups for sibling construction.
	Services BuildServices
}

// BuildServices provides lookups for sibling construction.
type BuildServices interface {
	CapturedSymbols(fn *ast.FunctionExpr) []cfg.SymbolID
	TypeAtPoint(point cfg.Point, sym cfg.SymbolID) typ.Type
	EnrichRecord(rec *typ.Record, sym cfg.SymbolID) typ.Type
}

// BuildServicesFuncs adapts functions to BuildServices.
type BuildServicesFuncs struct {
	CapturedSymbolsFn func(fn *ast.FunctionExpr) []cfg.SymbolID
	TypeAtPointFn     func(point cfg.Point, sym cfg.SymbolID) typ.Type
	EnrichRecordFn    func(rec *typ.Record, sym cfg.SymbolID) typ.Type
}

func (b BuildServicesFuncs) CapturedSymbols(fn *ast.FunctionExpr) []cfg.SymbolID {
	if b.CapturedSymbolsFn == nil {
		return nil
	}
	return b.CapturedSymbolsFn(fn)
}

func (b BuildServicesFuncs) TypeAtPoint(point cfg.Point, sym cfg.SymbolID) typ.Type {
	if b.TypeAtPointFn == nil {
		return nil
	}
	return b.TypeAtPointFn(point, sym)
}

func (b BuildServicesFuncs) EnrichRecord(rec *typ.Record, sym cfg.SymbolID) typ.Type {
	if b.EnrichRecordFn == nil {
		return nil
	}
	return b.EnrichRecordFn(rec, sym)
}

// Build constructs the sibling types map for a scope group.
//
// The algorithm proceeds in four phases:
//
// Phase 1 - Seed from Previous: Copy types from SiblingTypesPrev to preserve
// types accumulated in prior fixpoint iterations. This ensures monotonicity:
// types only grow more precise, never regress.
//
// Phase 2 - Captured Variables: For each function, find captured variables
// from the parent scope and add their types. This enables nested functions
// to see types of variables defined in enclosing scopes.
//
// Phase 3 - Sibling Functions: Add canonical function types for locally-defined siblings.
//
// The result maps each symbol to its best-known type. Functions in the group
// use this map as an overlay during type checking to resolve sibling references.
func Build(c BuildConfig) map[cfg.SymbolID]typ.Type {
	if len(c.Funcs) == 0 {
		return nil
	}

	result := make(map[cfg.SymbolID]typ.Type, len(c.Funcs)*4)

	// Step 1: Seed from SiblingTypesPrev for monotonic accumulation.
	for sym, ty := range c.SiblingTypesPrev {
		result[sym] = ty
	}

	// Step 2: Merge captured variable types.
	if c.Services != nil {
		for _, entry := range c.Funcs {
			if entry.Func == nil {
				continue
			}
			captured := c.Services.CapturedSymbols(entry.Func)
			for _, sym := range captured {
				if sym == 0 {
					continue
				}
				capturedType := c.Services.TypeAtPoint(entry.Point, sym)
				if capturedType == nil {
					continue
				}
				if typ.IsSoft(capturedType, typ.SoftAnnotationPolicy) {
					continue
				}
				if rec, ok := unwrap.Alias(capturedType).(*typ.Record); ok {
					if enriched := c.Services.EnrichRecord(rec, sym); enriched != nil {
						capturedType = enriched
					}
				}
				prev := result[sym]
				if typ.IsSoft(prev, typ.SoftAnnotationPolicy) && !typ.IsSoft(capturedType, typ.SoftAnnotationPolicy) {
					result[sym] = capturedType
				} else {
					result[sym] = typ.JoinPreferNonSoft(prev, capturedType)
				}
			}
		}
	}

	// Step 3: Add canonical local function types.
	for _, entry := range c.Funcs {
		if !entry.IsLocal || entry.Symbol == 0 {
			continue
		}
		fnType := c.FuncTypes[entry.Symbol]
		if fnType == nil {
			continue
		}
		result[entry.Symbol] = returns.MergeFunctionFactType(result[entry.Symbol], fnType)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// Compute extracts sibling types for a function's scope group from the store.
//
// The store is keyed by group hash (computed from the parent scope). This
// function looks up the sibling types for the given group and returns them.
// Returns nil if no sibling types exist for the group.
func Compute(store map[uint64]map[cfg.SymbolID]typ.Type, groupHash uint64) map[cfg.SymbolID]typ.Type {
	if store == nil {
		return nil
	}
	if siblings := store[groupHash]; len(siblings) > 0 {
		return siblings
	}
	return nil
}

// Copy returns a shallow copy of a sibling types map.
func Copy(m map[cfg.SymbolID]typ.Type) map[cfg.SymbolID]typ.Type {
	if m == nil {
		return nil
	}
	cp := make(map[cfg.SymbolID]typ.Type, len(m))
	for sym, t := range m {
		cp[sym] = t
	}
	return cp
}

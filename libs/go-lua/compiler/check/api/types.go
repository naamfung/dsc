// types.go provides utility types and functions for type synthesis operations.
// These are used throughout the checking pipeline for type caching and temporary overlays.
package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// SpecTypes provides a temporary type overlay for a single synthesis operation.
// It maps symbols to types that should override the normal lookup chain without
// mutating the underlying scope or declared types.
//
// USAGE: SpecTypes is used during constraint extraction when a local narrowing
// context needs to be applied (e.g., spec-based type guards, local type assertions).
// The overlay is temporary and discarded after the operation completes.
//
// EXAMPLE: During table spec matching, SpecTypes holds the narrowed field types
// that should be visible when synthesizing the table's value expressions.
type SpecTypes = map[cfg.SymbolID]typ.Type

// Cache stores synthesized types for expressions at CFG points.
// This provides memoization within a single synthesis operation to avoid
// recomputing types for the same expression at the same point.
//
// KEY STRUCTURE: Cache keys combine the expression (by pointer identity) with
// the CFG point. This means the same expression at different points has different
// cache entries, correctly handling flow-dependent type narrowing.
//
// LIFETIME: Cache instances are typically created per-phase and discarded after.
// They should not be shared across fixpoint iterations.
type Cache map[cacheKey]typ.Type

// cacheKey combines expression and CFG point for cache lookup.
// Uses pointer identity for expressions - two structurally equal expressions
// at different locations are distinct cache entries.
type cacheKey struct {
	expr interface{}
	p    cfg.Point
}

// Get retrieves a cached type.
func (c Cache) Get(expr interface{}, p cfg.Point) (typ.Type, bool) {
	t, ok := c[cacheKey{expr, p}]
	return t, ok
}

// Put stores a type in the cache.
func (c Cache) Put(expr interface{}, p cfg.Point, t typ.Type) {
	if c == nil {
		return
	}
	c[cacheKey{expr, p}] = t
}

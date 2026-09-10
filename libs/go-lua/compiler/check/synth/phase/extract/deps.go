package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
)

// Deps aggregates all dependencies needed by the Synthesizer.
//
// Rather than passing many parameters to each method, Deps groups:
//   - Query context for type database operations (Ctx)
//   - Type operations for subtype checks, field lookups, etc. (Types)
//   - Scope map for variable type lookups at program points (Scopes)
//   - Module manifests for require() resolution (Manifests)
//   - Type checking environment with diagnostics (CheckCtx)
//   - Flow analysis results for narrowing (Flow, optional)
//   - Path extraction for constraint tracking (Paths, optional)
//   - Caches for synthesis memoization (PreCache, NarrowCache)
//   - Module-level bindings and aliases for cross-function synthesis
type Deps struct {
	Ctx    *db.QueryContext
	Types  core.TypeOps
	Scopes api.ScopeMap
	// DefaultScope is used when a point has no explicit scope entry.
	// This allows sparse scope maps for transient inference passes.
	DefaultScope *scope.State
	Manifests    io.ManifestQuerier
	CheckCtx     api.BaseEnv
	Graphs       api.GraphProvider

	Flow  api.FlowOps
	Paths api.PathFromExprFunc

	PreCache    api.Cache
	NarrowCache api.Cache

	// FunctionTypeInProgress guards call-point local function specialization
	// against recursion across temporary synthesizers.
	FunctionTypeInProgress map[functionTypeProgressKey]bool

	// Module-level bindings for nested function CFG building.
	ModuleBindings *bind.BindingTable

	// Module-level aliases from require() statements.
	ModuleAliases map[cfg.SymbolID]string
}

type functionTypeProgressKey struct {
	Func         *ast.FunctionExpr
	CapturePoint cfg.Point
}

// NewDeps creates a new Deps instance.
func NewDeps(ctx *db.QueryContext, types core.TypeOps, scopes api.ScopeMap, manifests io.ManifestQuerier, checkCtx api.BaseEnv) *Deps {
	return &Deps{
		Ctx:                    ctx,
		Types:                  types,
		Scopes:                 scopes,
		Manifests:              manifests,
		CheckCtx:               checkCtx,
		Graphs:                 api.GraphsFrom(ctx),
		PreCache:               make(api.Cache),
		NarrowCache:            make(api.Cache),
		FunctionTypeInProgress: make(map[functionTypeProgressKey]bool),
	}
}

// Graph returns the versioned graph from checkCtx.
func (d *Deps) Graph() cfg.VersionedGraph {
	if d.CheckCtx == nil {
		return nil
	}
	return d.CheckCtx.Graph()
}

// Entry returns the entry point of the graph.
func (d *Deps) Entry() cfg.Point {
	if g := d.Graph(); g != nil {
		return g.Entry()
	}
	return 0
}

// ScopeAt returns scope for point p with optional default fallback.
func (d *Deps) ScopeAt(p cfg.Point) *scope.State {
	if d == nil {
		return nil
	}
	if d.Scopes != nil {
		if sc := d.Scopes[p]; sc != nil {
			return sc
		}
	}
	return d.DefaultScope
}

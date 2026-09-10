// session.go defines the Session type that holds per-run state for type checking.
// Session is the primary interface for accessing analysis results. SessionStore
// lives in compiler/check/store and manages fixpoint iteration state and interproc
// snapshots.
//
// # LIFECYCLE SEPARATION
//
// SessionStore is structured into three lifecycle-scoped components (see store package):
//
//   - ModuleStore: Immutable module-wide state created once at check start.
//     Contains binding tables, CFG graphs, and module aliases. Never modified
//     during fixpoint iteration.
//
//   - IterationStore: Iteration-local state used during fixpoint convergence
//     (revision counter and constructor field collection).
//
//   - IterationScratch: Single-iteration state cleared at each boundary.
//     Tracks which literals have been analyzed, pending parameter hints,
//     and change detection flags.
//
// # SNAPSHOT PROTOCOL
//
// Interproc facts and effects follow a snapshot protocol:
//
//   - During iteration: Functions read from the previous snapshot
//   - At boundary: New facts/effects replace the snapshot
//   - Convergence: Iteration stops when snapshots are unchanged
//
// This protocol ensures all functions within an iteration see consistent
// cross-function information, enabling deterministic analysis regardless
// of function processing order.
//
// # PARALLELIZATION
//
// Functions within a single fixpoint iteration are independent (read-only inputs).
// To parallelize: run with per-worker db.QueryContext, merge results into next
// maps deterministically at iteration boundary. db.QueryContext is NOT safe for
// concurrent access from multiple goroutines.
package check

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Session holds all state and results for analyzing a single Lua module.
// One Session is created per Check call and contains the complete analysis output
// including per-function results, diagnostics, and inter-function channel data.
//
// USAGE PATTERN:
//
//	sess := checker.Check(source, "module.lua")
//	for _, diag := range sess.Diagnostics {
//	    fmt.Println(diag)
//	}
//	exportType := sess.ExportType()
//
// CONCURRENCY: db.QueryContext is NOT safe for concurrent access. For parallel
// analysis, create one Session per worker with independent QueryContexts, then
// merge results. Functions within a single fixpoint iteration read from shared
// snapshots and write to independent per-iteration maps, enabling future parallelization.
//
// MEMORY MANAGEMENT: Call Release() after extracting Manifest data to free heavy
// allocations (CFGs, scopes, flow data). The Session remains valid for Diagnostics
// access after Release.
type Session struct {
	// Ctx provides query infrastructure for memoization. NOT concurrency-safe.
	Ctx *db.QueryContext

	// SourceName identifies the module for diagnostic reporting and path resolution.
	SourceName string

	// Imports tracks loaded module manifests keyed by require path.
	Imports map[string]*io.Manifest

	// Store holds all inter-function channel state. Attached to Ctx for query access.
	Store *store.SessionStore

	// Results contains per-function analysis output keyed by FunctionExpr pointer.
	// Includes root function and all nested functions.
	Results map[*ast.FunctionExpr]*api.FuncResult

	// RootFunc is the synthetic FunctionExpr wrapping the module chunk.
	// Its body contains the module's top-level statements.
	RootFunc *ast.FunctionExpr

	// RootResult provides direct access to the root function's analysis output.
	// Equivalent to Results[RootFunc] but avoids map lookup.
	RootResult *api.FuncResult

	// Diagnostics accumulates type errors, warnings, and suggestions from all phases.
	// Sorted by position after analysis completes.
	Diagnostics []diag.Diagnostic

	// Terminators tracks which functions are known to never return (error, assert(false)).
	Terminators map[string]bool

	// ManifestVars captures @manifest annotations for module metadata export.
	ManifestVars map[string]string

	// cfgCache caches built CFG graphs to avoid rebuilding for the same function.
	cfgCache map[*ast.FunctionExpr]*cfg.Graph

	// coreStore provides extension storage for analysis cores.
	coreStore map[any]any

	// scopeDepthDiagEmitted tracks diagnostics for scope depth limit per function.
	scopeDepthDiagEmitted map[*ast.FunctionExpr]bool
}

// Source returns the session source name.
func (s *Session) Source() string {
	if s == nil {
		return ""
	}
	return s.SourceName
}

// Context returns the query context for the session.
func (s *Session) Context() *db.QueryContext {
	if s == nil {
		return nil
	}
	return s.Ctx
}

// StoreHandle returns the session store as an interation-capable interface.
func (s *Session) StoreHandle() api.IterationStore {
	if s == nil {
		return nil
	}
	return s.Store
}

// ResultsMap returns the function result map.
func (s *Session) ResultsMap() map[*ast.FunctionExpr]*api.FuncResult {
	if s == nil {
		return nil
	}
	return s.Results
}

// RootFuncNode returns the root function for this session.
func (s *Session) RootFuncNode() *ast.FunctionExpr {
	if s == nil {
		return nil
	}
	return s.RootFunc
}

// SetRootFuncNode sets the root function for this session.
func (s *Session) SetRootFuncNode(fn *ast.FunctionExpr) {
	if s == nil {
		return
	}
	s.RootFunc = fn
}

// RootResultValue returns the root function result.
func (s *Session) RootResultValue() *api.FuncResult {
	if s == nil {
		return nil
	}
	return s.RootResult
}

// SetRootResultValue sets the root function result.
func (s *Session) SetRootResultValue(result *api.FuncResult) {
	if s == nil {
		return
	}
	s.RootResult = result
}

// ResetDiagnostics clears all diagnostics.
func (s *Session) ResetDiagnostics() {
	if s == nil {
		return
	}
	s.Diagnostics = nil
}

// AppendDiagnostics appends diagnostics to the session.
func (s *Session) AppendDiagnostics(diags ...diag.Diagnostic) {
	if s == nil || len(diags) == 0 {
		return
	}
	s.Diagnostics = append(s.Diagnostics, diags...)
}

// DiagnosticsSlice returns the current diagnostics slice.
func (s *Session) DiagnosticsSlice() []diag.Diagnostic {
	if s == nil {
		return nil
	}
	return s.Diagnostics
}

// ScopeDepthDiagState returns the map tracking scope depth diagnostics.
func (s *Session) ScopeDepthDiagState() map[*ast.FunctionExpr]bool {
	if s == nil {
		return nil
	}
	if s.scopeDepthDiagEmitted == nil {
		s.scopeDepthDiagEmitted = make(map[*ast.FunctionExpr]bool)
	}
	return s.scopeDepthDiagEmitted
}

// New creates a session for checking a file.
func New(ctx *db.QueryContext, name string) *Session {
	store := store.NewSessionStore()
	api.AttachStore(ctx, store)
	sess := &Session{
		Ctx:                   ctx,
		SourceName:            name,
		Store:                 store,
		Imports:               make(map[string]*io.Manifest),
		Results:               make(map[*ast.FunctionExpr]*api.FuncResult),
		Terminators:           make(map[string]bool),
		ManifestVars:          make(map[string]string),
		cfgCache:              make(map[*ast.FunctionExpr]*cfg.Graph),
		scopeDepthDiagEmitted: make(map[*ast.FunctionExpr]bool),
	}
	api.AttachGraphs(ctx, sess)
	return sess
}

// GetOrBuildCFG returns a cached CFG for the function or builds and caches a new one.
func (s *Session) GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil {
		return nil
	}
	if g := s.cfgCache[fn]; g != nil {
		return g
	}
	g := cfg.BuildWithBindings(fn, s.Store.ModuleBindings())
	s.cfgCache[fn] = g
	return g
}

// RegisterGraphHierarchy registers the root graph and all nested graphs.
// This populates graph/function maps and nested metadata for query lookups.
func (s *Session) RegisterGraphHierarchy(root *cfg.Graph) {
	if s == nil || root == nil || s.Store == nil {
		return
	}
	visited := make(map[uint64]bool)
	var walk func(g *cfg.Graph)
	walk = func(g *cfg.Graph) {
		if g == nil {
			return
		}
		id := g.ID()
		if id == 0 || visited[id] {
			return
		}
		visited[id] = true
		if fn := g.Func(); fn != nil {
			s.Store.RegisterGraph(g, fn)
			if bindings := g.Bindings(); bindings != nil {
				if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
					s.Store.RegisterFunctionRef(sym, fn, g, 0, 0)
				}
			}
		}
		// Register local function assignments within this graph.
		g.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
			if info == nil || !info.IsLocal || len(info.Targets) == 0 {
				return
			}
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					return
				}
				if fnExpr, ok := source.(*ast.FunctionExpr); ok && fnExpr != nil {
					child := s.GetOrBuildCFG(fnExpr)
					if child == nil {
						return
					}
					s.Store.RegisterGraph(child, fnExpr)
					s.Store.RegisterNestedMeta(child.ID(), g.ID(), p)
					s.Store.RegisterFunctionRef(target.Symbol, fnExpr, child, g.ID(), p)
				}
			})
		})
		g.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Symbol == 0 || info.FuncExpr == nil {
				return
			}
			child := s.GetOrBuildCFG(info.FuncExpr)
			if child == nil {
				return
			}
			s.Store.RegisterGraph(child, info.FuncExpr)
			s.Store.RegisterNestedMeta(child.ID(), g.ID(), p)
			s.Store.RegisterFunctionRef(info.Symbol, info.FuncExpr, child, g.ID(), p)
		})
		for _, nf := range g.NestedFunctions() {
			if nf.Func == nil {
				continue
			}
			child := s.GetOrBuildCFG(nf.Func)
			if child == nil {
				continue
			}
			s.Store.RegisterGraph(child, nf.Func)
			s.Store.RegisterNestedMeta(child.ID(), g.ID(), nf.Point)
			nestedSym := nf.Symbol
			if nestedSym == 0 && child.Bindings() != nil {
				if sym, ok := child.Bindings().FuncLitSymbol(nf.Func); ok {
					nestedSym = sym
				}
			}
			if nestedSym != 0 {
				s.Store.RegisterFunctionRef(nestedSym, nf.Func, child, g.ID(), nf.Point)
			}
			walk(child)
		}
	}
	walk(root)
}

// PluginStore stores a value for a core extension.
func (s *Session) PluginStore(key, val any) {
	if s == nil {
		return
	}
	if s.coreStore == nil {
		s.coreStore = make(map[any]any)
	}
	s.coreStore[key] = val
}

// PluginLoad retrieves a value stored by a core extension.
func (s *Session) PluginLoad(key any) any {
	if s == nil || s.coreStore == nil {
		return nil
	}
	return s.coreStore[key]
}

// ExportType computes the module's exported type from return statements.
// This is the primary method for extracting the module's public interface for
// manifest generation and import resolution.
//
// RETURN TYPE COMPUTATION:
// The export type is computed from all return statements in the root function:
//   - Multiple returns are joined into a union type
//   - Unreachable returns (dead code) are excluded via flow analysis
//   - Empty returns contribute nil to the union
//
// EFFECT ENRICHMENT:
// If the returned value is a table/record, its method types are enriched with
// function refinements computed during analysis. This ensures exported functions
// carry their refinement summaries.
//
// USAGE:
//
//	sess := checker.Check(source, "module.lua")
//	exportType := sess.ExportType()
//	manifest := io.NewManifest(modulePath, exportType, sess.ExportTypes())
func (s *Session) ExportType() typ.Type {
	if s == nil {
		return typ.Nil
	}
	var refinements map[cfg.SymbolID]*constraint.FunctionRefinement
	if s.Store != nil {
		if s.Store.InterprocPrev != nil {
			refinements = s.Store.InterprocPrev.Refinements
		}
	}
	return modules.ExportType(s.RootResult, refinements)
}

// Release frees heavy allocations to reduce memory pressure after analysis.
// Call this after extracting all needed data (ExportType, ExportTypes, Diagnostics).
//
// WHAT IS FREED:
//   - CFG graphs and binding tables
//   - Scope states and flow solutions
//   - Inter-function channel data
//   - Synthesis engines
//
// WHAT REMAINS VALID:
//   - Diagnostics slice (still accessible)
//   - Session struct itself
//
// WHEN TO CALL: After manifest generation in batch processing scenarios.
// For single-file analysis, releasing is optional as Session is GC'd normally.
func (s *Session) Release() {
	if s == nil {
		return
	}

	// Clear store maps
	if s.Store != nil {
		// Clear module store
		if s.Store.Module != nil {
			s.Store.Module.ModuleBindings = nil
			clear(s.Store.Module.Graphs)
			clear(s.Store.Module.Funcs)
			clear(s.Store.Module.GraphToFunc)
			clear(s.Store.Module.Parents)
			clear(s.Store.Module.ModuleAliases)
		}

		// Clear interproc snapshots
		if s.Store.InterprocPrev != nil {
			clear(s.Store.InterprocPrev.Facts)
			clear(s.Store.InterprocPrev.Refinements)
			clear(s.Store.InterprocPrev.ConstructorFields)
		}
		if s.Store.InterprocNext != nil {
			clear(s.Store.InterprocNext.Facts)
			clear(s.Store.InterprocNext.Refinements)
			clear(s.Store.InterprocNext.ConstructorFields)
		}

		// Clear iteration scratch (empty placeholder)
		_ = s.Store.Scratch
	}

	// Clear per-function results
	for _, result := range s.Results {
		if result != nil {
			result.Graph = nil
			result.BaseScope = nil
			result.Scopes = nil
			result.Facts = nil
			result.FlowInputs = nil
			result.FlowSolution = nil
			result.NarrowSynth = nil
		}
	}
	clear(s.Results)
	clear(s.cfgCache)

	s.RootFunc = nil
	s.RootResult = nil
	s.coreStore = nil
	clear(s.scopeDepthDiagEmitted)
}

// ExportTypes extracts module-local type definitions for manifest generation.
// Returns a map of type names to their resolved types.
//
// INCLUDED: Types defined via @type or type alias syntax in this module.
// EXCLUDED: Standard library types, imported types from other modules.
//
// The returned map is used to populate Manifest.Types for import resolution.
// When another module imports this one, these types become available in
// the importer's type namespace.
func (s *Session) ExportTypes() map[string]typ.Type {
	if s == nil {
		return nil
	}
	return modules.ExportTypes(s.RootResult)
}

// ExportManifest builds a module manifest from this session using canonical export policy.
//
// The manifest includes:
//   - Export type from root returns (with converged function refinements applied)
//   - Exported type definitions
//   - Function summaries for exported functions (ensures/effects for cross-module narrowing)
//
// Use this helper instead of manually stitching ExportType/ExportTypes so callers do not
// accidentally omit function summaries.
func (s *Session) ExportManifest(modulePath string) *io.Manifest {
	if s == nil {
		return nil
	}
	if modulePath == "" {
		modulePath = s.SourceName
	}

	manifest := io.NewManifest(modulePath)
	exportType := s.ExportType()
	manifest.SetExport(exportType)

	for typeName, t := range s.ExportTypes() {
		manifest.DefineType(typeName, t)
	}

	modules.ExportFunctionSummaries(manifest, exportType, s.RootGraph(), s.RefinementsForExport())
	return manifest
}

// RefinementsForExport extracts computed function refinements for manifest generation.
// Returns refinements from the final converged interproc snapshot.
//
// The returned map associates each function's SymbolID with its computed refinement,
// including IO effects (row), termination status, and conditional effects.
// This enables importers to see side effect information for exported functions.
func (s *Session) RefinementsForExport() map[cfg.SymbolID]*constraint.FunctionRefinement {
	if s == nil || s.Store == nil {
		return nil
	}
	if s.Store.InterprocPrev == nil {
		return nil
	}
	return modules.CopyRefinementsForExport(s.Store.InterprocPrev.Refinements)
}

// RootGraph returns the root function's control flow graph.
// The root function wraps the module's top-level statements. Its CFG contains:
//   - Module-level assignments and function definitions
//   - Module-level control flow (if, for, while)
//   - Return statements for module export
//
// Use for advanced introspection or manifest generation that needs CFG access.
func (s *Session) RootGraph() *cfg.Graph {
	if s == nil || s.RootResult == nil {
		return nil
	}
	return s.RootResult.Graph
}

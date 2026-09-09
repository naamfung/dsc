// Store interfaces define the contract between the type checker and its
// backing storage for interprocedural analysis data. These interfaces
// enable different storage implementations (in-memory, persistent) and
// support testing with mock implementations.
//
// The interfaces form a hierarchy with increasing capability:
//
//	ModuleStore     - Module-level bindings and aliases
//	GraphStore      - CFG graph lookup by ID
//	ParentScopes    - Parent scope lookup for nested functions
//	NestedMetaStore - Nested function metadata
//	SnapshotStore   - Stable interproc fact snapshots
//	FunctionRefs    - Symbol/function bidirectional lookup
//	StoreView       - Read-only combination of above
//	NestedStore     - StoreView + constructor field storage
//	IterationStore  - Full mutation capability for fixpoint
package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// FunctionRef is the canonical mapping for a function symbol.
// It records the relationship between a function's symbol ID, its CFG,
// its parent context, and the AST node. Used by interprocedural analysis
// to resolve function identities across call sites.
type FunctionRef struct {
	Sym           cfg.SymbolID
	GraphID       uint64
	ParentGraphID uint64
	DefPoint      cfg.Point
	Func          *ast.FunctionExpr
}

// NestedMeta holds parent metadata for a nested function graph.
type NestedMeta struct {
	ParentGraphID uint64
	DefPoint      cfg.Point
}

// GraphProvider maps function literals to CFGs.
type GraphProvider interface {
	GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph
}

// ModuleStore provides module-level bindings and alias maps.
type ModuleStore interface {
	ModuleBindings() *bind.BindingTable
	ModuleAliases() map[cfg.SymbolID]string
}

// GraphStore provides access to known CFGs.
type GraphStore interface {
	Graphs() map[uint64]*cfg.Graph
}

// ParentScopes provides parent scope lookup by graph.
type ParentScopes interface {
	Parents() map[uint64]*scope.State
	GraphParentHashOf(graphID uint64) uint64
	GraphKeyFor(graph *cfg.Graph, parent *scope.State) (GraphKey, bool)
}

// NestedMetaStore provides nested graph metadata.
type NestedMetaStore interface {
	NestedMetaFor(graphID uint64) (NestedMeta, bool)
}

// SnapshotStore exposes stable interproc fact snapshots.
type SnapshotStore interface {
	GetParamHintsSnapshot(graph *cfg.Graph, parent *scope.State) ParamHints
	GetReturnSummariesSnapshot(graph *cfg.Graph, parent *scope.State) ReturnSummaries
	GetNarrowReturnSummariesSnapshot(graph *cfg.Graph, parent *scope.State) NarrowReturnSummaries
	GetCapturedTypesSnapshot(graph *cfg.Graph, parent *scope.State) CapturedTypes
	GetCapturedFieldAssignsSnapshot(graph *cfg.Graph, parent *scope.State) CapturedFieldAssigns
	GetCapturedContainerMutationsSnapshot(graph *cfg.Graph, parent *scope.State) CapturedContainerMutations
	GetLocalFuncTypesSnapshot(graph *cfg.Graph, parent *scope.State) FuncTypes
	GetLiteralSigsSnapshot(graph *cfg.Graph, parent *scope.State) LiteralSigs
}

// FunctionRefs provides symbol/function lookup for function graphs.
type FunctionRefs interface {
	RegisterFunctionRef(sym cfg.SymbolID, fn *ast.FunctionExpr, graph *cfg.Graph, parentGraphID uint64, defPoint cfg.Point)
	FunctionRefBySym(sym cfg.SymbolID) *FunctionRef
	FuncForSymbol(sym cfg.SymbolID) *ast.FunctionExpr
	FuncForGraph(graph *cfg.Graph) *ast.FunctionExpr
	SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool)
}

// StoreView is the minimal interface required by pre-flow return inference.
type StoreView interface {
	ModuleStore
	GraphStore
	ParentScopes
	NestedMetaStore
	SnapshotStore
	FunctionRefs
}

// ConstructorFieldStore provides constructor field storage.
type ConstructorFieldStore interface {
	StoreConstructorFields(classSym cfg.SymbolID, fields map[string]typ.Type)
	LookupConstructorFields(classSym cfg.SymbolID) map[string]typ.Type
}

// InterprocFactSink provides write access to per-iteration interproc facts.
type InterprocFactSink interface {
	UpdateInterprocFactsNext(key GraphKey, update func(*Facts))
}

// NestedStore is the store interface required by nested processing.
type NestedStore interface {
	StoreView
	ConstructorFieldStore
	InterprocFactSink
}

// LiteralSigSource is used by phase runners to supply literal signatures.
type LiteralSigSource interface {
	GetLiteralSigsSnapshot(graph *cfg.Graph, parent *scope.State) LiteralSigs
}

// LiteralSigSink accepts literal signature results from analysis.
type LiteralSigSink interface {
	StoreLiteralSigs(graphID uint64, sigs map[*ast.FunctionExpr]*typ.Function)
}

// IterationStore provides mutation operations required by the fixpoint driver.
type IterationStore interface {
	NestedStore
	LiteralSigSink

	ClearIterationChannels()
	FixpointSwap() bool
	FixpointChannelDiffs() []string
	Revision() uint64
	BumpRevision()

	RefinementStore() RefinementStore
	StoreFunctionRefinement(sym cfg.SymbolID, eff *constraint.FunctionRefinement)

	SetModuleBindings(bindings *bind.BindingTable)
	SetModuleAliases(aliases map[cfg.SymbolID]string)
	SetParentScope(parentHash uint64, parent *scope.State)
	SetGraphParentHash(graphID, parentHash uint64)

	UpdateInterprocFactsNext(key GraphKey, update func(*Facts))
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (GraphKey, bool)
}

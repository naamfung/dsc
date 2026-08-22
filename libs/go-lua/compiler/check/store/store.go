package store

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

type SessionStore struct {
	// Module contains module-wide immutable state.
	// Created once at the start of checking and shared by all CFG builds.
	Module *ModuleStore

	// Iteration contains iteration-local state for fixpoint convergence.
	Iteration *IterationStore

	// Scratch contains iteration-local state cleared each cycle.
	Scratch *IterationScratch

	// InterprocPrev holds the stable interproc snapshot used during analysis.
	// Updated at fixpoint boundaries to provide a consistent view.
	InterprocPrev *InterprocState
	// InterprocNext accumulates facts/effects produced during the current iteration.
	InterprocNext *InterprocState

	// GraphParentHash records the parent scope hash for each graph ID.
	GraphParentHash map[uint64]uint64

	// lastSwapDiffs records which channels changed during the most recent FixpointSwap.
	// Stored per-session to avoid cross-session contamination.
	lastSwapDiffs []string

	phase api.Phase
}

// InterprocState holds interprocedural facts and refinements for an iteration snapshot.
type InterprocState struct {
	Facts             map[api.GraphKey]api.Facts
	Refinements       map[cfg.SymbolID]*constraint.FunctionRefinement
	ConstructorFields api.ConstructorFields
}

// NewInterprocState creates an initialized interproc snapshot.
func NewInterprocState() *InterprocState {
	return &InterprocState{
		Facts:             make(map[api.GraphKey]api.Facts),
		Refinements:       make(map[cfg.SymbolID]*constraint.FunctionRefinement),
		ConstructorFields: make(api.ConstructorFields),
	}
}

// ModuleStore holds module-wide immutable state.
// Created once at the start of checking and never modified during iteration.
type ModuleStore struct {
	// ModuleBindings is the binding table for the entire module.
	ModuleBindings *bind.BindingTable

	// Graphs maps graph IDs to CFG graphs.
	Graphs map[uint64]*cfg.Graph

	// Funcs maps graph IDs to FunctionExpr nodes.
	Funcs map[uint64]*ast.FunctionExpr

	// GraphToFunc maps CFG graphs to their FunctionExpr nodes for O(1) lookup.
	GraphToFunc map[*cfg.Graph]*ast.FunctionExpr
	// FuncToGraph maps FunctionExpr nodes to their CFG graphs for O(1) lookup.
	FuncToGraph map[*ast.FunctionExpr]*cfg.Graph
	// Functions stores canonical mappings between symbols and function graphs.
	Functions *FunctionRegistry

	// Parents maps scope hashes to parent scope states.
	Parents map[uint64]*scope.State

	// ModuleAliases maps symbol IDs to module paths from require() assignments.
	// Computed once from the chunk graph and shared with all nested functions.
	ModuleAliases map[cfg.SymbolID]string

	// NestedMeta maps child graph IDs to their parent graph and definition point.
	NestedMeta map[uint64]api.NestedMeta
}

// FunctionRegistry provides symbol <-> function graph lookup.
type FunctionRegistry struct {
	BySym     map[cfg.SymbolID]*api.FunctionRef
	ByFunc    map[*ast.FunctionExpr]*api.FunctionRef
	ByGraphID map[uint64]*api.FunctionRef
}

// IterationStore holds iteration-local state for fixpoint iteration.
type IterationStore struct {
	// Revision is bumped at fixpoint iteration boundary.
	// Included in FuncKey to invalidate cached results when interproc facts/effects change.
	Revision uint64
}

// IterationScratch holds iteration-local state cleared each cycle.
// Not double-buffered; reset at each iteration boundary.
type IterationScratch struct {
	// LiteralSigsByGraphID stores literal signatures computed this iteration.
	LiteralSigsByGraphID map[uint64]map[*ast.FunctionExpr]*typ.Function
}

// effectsEqual compares two FunctionRefinements for structural equality.
func effectsEqual(a, b *constraint.FunctionRefinement) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
}

// effectsMapEqual compares two effect maps for structural equality.
func effectsMapEqual(a, b map[cfg.SymbolID]*constraint.FunctionRefinement) bool {
	if len(a) != len(b) {
		return false
	}
	for _, sym := range cfg.SortedSymbolIDs(a) {
		if !effectsEqual(a[sym], b[sym]) {
			return false
		}
	}
	return true
}

// interprocFactsMapEqual compares two interproc facts maps.
func interprocFactsMapEqual(a, b map[api.GraphKey]api.Facts) bool {
	if len(a) != len(b) {
		return false
	}
	for _, key := range api.SortedGraphKeys(a) {
		if !returns.FactsEqual(a[key], b[key]) {
			return false
		}
	}
	return true
}

// widenInterprocFacts merges next facts into prev using monotone union.
func widenInterprocFacts(prev, next map[api.GraphKey]api.Facts) map[api.GraphKey]api.Facts {
	if len(prev) == 0 && len(next) == 0 {
		return make(map[api.GraphKey]api.Facts)
	}
	out := make(map[api.GraphKey]api.Facts, len(prev)+len(next))
	for _, key := range api.SortedGraphKeys(prev) {
		out[key] = prev[key]
	}
	for _, key := range api.SortedGraphKeys(next) {
		facts := next[key]
		if existing, ok := out[key]; ok {
			out[key] = returns.WidenFacts(existing, facts)
		} else {
			out[key] = facts
		}
	}
	return out
}

// NewSessionStore creates an initialized store with all sub-structs.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		Module:          NewModuleStore(),
		Iteration:       NewIterationStore(),
		Scratch:         NewIterationScratch(),
		InterprocPrev:   NewInterprocState(),
		InterprocNext:   NewInterprocState(),
		GraphParentHash: make(map[uint64]uint64),
		phase:           api.PhaseScopeCompute,
	}
}

// SetPhase sets the current check phase for snapshot access checks.
func (s *SessionStore) SetPhase(phase api.Phase) {
	if s == nil {
		return
	}
	s.phase = phase
}

// Phase returns the current check phase.
func (s *SessionStore) Phase() api.Phase {
	if s == nil {
		return api.PhaseScopeCompute
	}
	return s.phase
}

func (s *SessionStore) requirePhase(allowed ...api.Phase) {
	if s == nil {
		return
	}
	for _, phase := range allowed {
		if s.phase == phase {
			return
		}
	}
	panic("store: snapshot accessed in wrong phase")
}

// WithPhase runs fn with a temporary phase, restoring the prior phase afterward.
func (s *SessionStore) WithPhase(phase api.Phase, fn func()) {
	if fn == nil {
		return
	}
	if s == nil {
		fn()
		return
	}
	prev := s.phase
	s.phase = phase
	defer func() {
		s.phase = prev
	}()
	fn()
}

// NewModuleStore creates an initialized module store.
func NewModuleStore() *ModuleStore {
	return &ModuleStore{
		Graphs:        make(map[uint64]*cfg.Graph),
		Funcs:         make(map[uint64]*ast.FunctionExpr),
		GraphToFunc:   make(map[*cfg.Graph]*ast.FunctionExpr),
		FuncToGraph:   make(map[*ast.FunctionExpr]*cfg.Graph),
		Functions:     newFunctionRegistry(),
		Parents:       make(map[uint64]*scope.State),
		ModuleAliases: make(map[cfg.SymbolID]string),
		NestedMeta:    make(map[uint64]api.NestedMeta),
	}
}

// NewIterationStore creates an initialized iteration store.
func NewIterationStore() *IterationStore {
	return &IterationStore{}
}

// NewIterationScratch creates an initialized iteration scratch.
func NewIterationScratch() *IterationScratch {
	return &IterationScratch{
		LiteralSigsByGraphID: make(map[uint64]map[*ast.FunctionExpr]*typ.Function),
	}
}

func (s *SessionStore) ensureInterprocStates() {
	if s == nil {
		return
	}
	if s.InterprocPrev == nil {
		s.InterprocPrev = NewInterprocState()
	}
	if s.InterprocNext == nil {
		s.InterprocNext = NewInterprocState()
	}
}

func (s *SessionStore) resetScratch() {
	if s == nil {
		return
	}
	if s.Scratch == nil {
		s.Scratch = NewIterationScratch()
		return
	}
	s.Scratch.LiteralSigsByGraphID = make(map[uint64]map[*ast.FunctionExpr]*typ.Function)
}

func swapSnapshotChannel[T any](
	prev *T,
	next *T,
	merge func(prev, next T) T,
	equal func(a, b T) bool,
	reset func() T,
) bool {
	if prev == nil || next == nil || merge == nil || equal == nil || reset == nil {
		return false
	}
	merged := merge(*prev, *next)
	changed := !equal(*prev, merged)
	*prev = merged
	*next = reset()
	return changed
}

func (s *SessionStore) swapInterprocChannels() []string {
	s.ensureInterprocStates()

	// Channel policies:
	// - Refinements/ConstructorFields are overwrite channels (next snapshot replaces prev).
	// - InterprocFacts is a widening channel (monotone merge across iterations).
	channels := []struct {
		name string
		swap func() bool
	}{
		{
			name: "Refinements",
			swap: func() bool {
				return swapSnapshotChannel(
					&s.InterprocPrev.Refinements,
					&s.InterprocNext.Refinements,
					func(_prev, next map[cfg.SymbolID]*constraint.FunctionRefinement) map[cfg.SymbolID]*constraint.FunctionRefinement {
						return next
					},
					effectsMapEqual,
					func() map[cfg.SymbolID]*constraint.FunctionRefinement {
						return make(map[cfg.SymbolID]*constraint.FunctionRefinement)
					},
				)
			},
		},
		{
			name: "InterprocFacts",
			swap: func() bool {
				return swapSnapshotChannel(
					&s.InterprocPrev.Facts,
					&s.InterprocNext.Facts,
					widenInterprocFacts,
					interprocFactsMapEqual,
					func() map[api.GraphKey]api.Facts {
						return make(map[api.GraphKey]api.Facts)
					},
				)
			},
		},
		{
			name: "ConstructorFields",
			swap: func() bool {
				return swapSnapshotChannel(
					&s.InterprocPrev.ConstructorFields,
					&s.InterprocNext.ConstructorFields,
					func(_prev, next api.ConstructorFields) api.ConstructorFields {
						return next
					},
					returns.ConstructorFieldsEqual,
					func() api.ConstructorFields {
						return make(api.ConstructorFields)
					},
				)
			},
		},
	}

	diffs := make([]string, 0, len(channels))
	for _, channel := range channels {
		if channel.swap() {
			diffs = append(diffs, channel.name)
		}
	}
	return diffs
}

// FixpointSwap performs the iteration boundary swap for all iteration-local channels.
// This is the critical operation that advances the fixpoint iteration by making
// current results available for the next iteration.
//
// OPERATIONS PERFORMED:
//  1. Compare each channel's prev and next for equality
//  2. Move next → prev (current results become baseline for next iteration)
//  3. Allocate fresh next maps (empty for accumulating new results)
//  4. Clear iteration-local scratch state
//  5. Record which channels changed for diagnostic reporting
//
// CHANGE DETECTION: Each channel uses type-appropriate equality:
//   - Refinements: FunctionRefinement.Equals (structural comparison)
//   - ConstructorFields: typ.TypeEquals (structural equality)
//
// RETURN VALUE: Returns true if any channel changed, signaling another iteration
// is needed. Returns false when all channels stabilize (fixpoint reached).
func (s *SessionStore) FixpointSwap() bool {
	diffs := s.swapInterprocChannels()

	s.resetScratch()

	// Record which channels changed for diagnostic reporting
	s.lastSwapDiffs = diffs

	return len(diffs) > 0
}

// FixpointChannelDiffs returns the names of channels that changed during the
// most recent FixpointSwap call.
func (s *SessionStore) FixpointChannelDiffs() []string {
	if s == nil {
		return nil
	}
	if len(s.lastSwapDiffs) == 0 {
		return nil
	}
	out := make([]string, len(s.lastSwapDiffs))
	copy(out, s.lastSwapDiffs)
	return out
}

// Revision returns the current revision counter.
func (s *SessionStore) Revision() uint64 {
	if s == nil || s.Iteration == nil {
		return 0
	}
	return s.Iteration.Revision
}

// BumpRevision increments the revision counter.
func (s *SessionStore) BumpRevision() {
	if s == nil {
		return
	}
	if s.Iteration == nil {
		s.Iteration = NewIterationStore()
	}
	s.Iteration.Revision++
}

// LookupRefinementBySym returns the refinement for a function by its SymbolID.
// Reads from the stable interproc refinement snapshot for order-independent analysis.
func (s *SessionStore) LookupRefinementBySym(sym cfg.SymbolID) *constraint.FunctionRefinement {
	if sym == 0 {
		return nil
	}
	if s.InterprocPrev == nil || s.InterprocPrev.Refinements == nil {
		return nil
	}
	return s.InterprocPrev.Refinements[sym]
}

// StoreFunctionRefinement records a function refinement for the current iteration.
func (s *SessionStore) StoreFunctionRefinement(sym cfg.SymbolID, eff *constraint.FunctionRefinement) {
	if s == nil || sym == 0 || eff == nil {
		return
	}
	s.ensureInterprocStates()
	if s.InterprocNext.Refinements == nil {
		s.InterprocNext.Refinements = make(map[cfg.SymbolID]*constraint.FunctionRefinement)
	}
	if existing := s.InterprocNext.Refinements[sym]; effectsEqual(existing, eff) {
		return
	}
	s.InterprocNext.Refinements[sym] = eff
}

// StoreConstructorFields stores constructor fields for a class symbol.
func (s *SessionStore) StoreConstructorFields(classSym cfg.SymbolID, fields map[string]typ.Type) {
	if classSym == 0 || len(fields) == 0 {
		return
	}
	s.ensureInterprocStates()
	if s.InterprocNext.ConstructorFields == nil {
		s.InterprocNext.ConstructorFields = make(api.ConstructorFields)
	}
	dst := s.InterprocNext.ConstructorFields[classSym]
	if dst == nil {
		dst = make(map[string]typ.Type)
		s.InterprocNext.ConstructorFields[classSym] = dst
	}
	for name, t := range fields {
		if existing := dst[name]; existing != nil {
			dst[name] = typ.JoinPreferNonSoft(existing, t)
		} else {
			dst[name] = t
		}
	}
}

// LookupConstructorFields returns constructor fields from the stable snapshot.
func (s *SessionStore) LookupConstructorFields(classSym cfg.SymbolID) map[string]typ.Type {
	if s == nil || classSym == 0 {
		return nil
	}
	if s.InterprocPrev == nil {
		return nil
	}
	return s.InterprocPrev.ConstructorFields[classSym]
}

// ClearIterationChannels clears all inter-function channel state for a fresh run.
func (s *SessionStore) ClearIterationChannels() {
	if s == nil {
		return
	}
	if s.Iteration == nil {
		s.Iteration = NewIterationStore()
	}
	if s.Scratch == nil {
		s.Scratch = NewIterationScratch()
	}
	s.InterprocPrev = NewInterprocState()
	s.InterprocNext = NewInterprocState()
	s.resetScratch()
	s.Iteration.Revision = 0
	s.lastSwapDiffs = nil
}

// RefinementStore returns a view over the stable interproc refinement snapshot.
func (s *SessionStore) RefinementStore() api.RefinementStore {
	if s == nil || s.InterprocPrev == nil {
		return &snapshotRefinementStore{refinements: nil}
	}
	return &snapshotRefinementStore{refinements: s.InterprocPrev.Refinements}
}

// ModuleBindings returns the module binding table.
func (s *SessionStore) ModuleBindings() *bind.BindingTable {
	return s.Module.ModuleBindings
}

// SetModuleBindings installs the module binding table.
func (s *SessionStore) SetModuleBindings(bindings *bind.BindingTable) {
	if s == nil || s.Module == nil {
		return
	}
	s.Module.ModuleBindings = bindings
}

// Graphs returns the graph map.
func (s *SessionStore) Graphs() map[uint64]*cfg.Graph {
	return s.Module.Graphs
}

// GraphKeyFor returns the interproc graph key for a graph and parent scope.
func (s *SessionStore) GraphKeyFor(graph *cfg.Graph, parent *scope.State) (api.GraphKey, bool) {
	if s == nil || graph == nil {
		return api.GraphKey{}, false
	}
	parentHash := api.ParentHashForGraph(s, graph.ID(), parent)
	if parentHash == 0 {
		return api.GraphKey{}, false
	}
	return api.KeyForGraph(graph, parentHash), true
}

// ParentGraphKeyForSymbol returns the graph key for the parent graph that owns sym.
func (s *SessionStore) ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool) {
	if s == nil || sym == 0 {
		return api.GraphKey{}, false
	}
	ref := s.FunctionRefBySym(sym)
	if ref == nil {
		return api.GraphKey{}, false
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	graph := s.Graphs()[parentGraphID]
	if graph == nil {
		return api.GraphKey{}, false
	}
	parentHash := s.GraphParentHashOf(parentGraphID)
	if parentHash == 0 {
		return api.GraphKey{}, false
	}
	return api.KeyForGraph(graph, parentHash), true
}

func initInterprocFacts(f *api.Facts) {
	if f.FunctionFacts == nil {
		f.FunctionFacts = make(api.FunctionFacts)
	}
	if f.ParamHints == nil {
		f.ParamHints = make(map[cfg.SymbolID][]typ.Type)
	}
	if f.LiteralSigs == nil {
		f.LiteralSigs = make(map[*ast.FunctionExpr]*typ.Function)
	}
	if f.CapturedTypes == nil {
		f.CapturedTypes = make(api.CapturedTypes)
	}
	if f.CapturedFields == nil {
		f.CapturedFields = make(api.CapturedFieldAssigns)
	}
}

// UpdateInterprocFactsNext updates interproc facts for the next iteration.
// This is the public entry point used by post-flow analysis to record results.
func (s *SessionStore) UpdateInterprocFactsNext(key api.GraphKey, update func(*api.Facts)) {
	if s == nil {
		return
	}
	s.ensureInterprocStates()
	facts := s.InterprocNext.Facts[key]
	initInterprocFacts(&facts)
	update(&facts)
	returns.NormalizeFunctionFactChannels(&facts)
	s.InterprocNext.Facts[key] = facts
}

// Funcs returns the function map.
func (s *SessionStore) Funcs() map[uint64]*ast.FunctionExpr {
	return s.Module.Funcs
}

// RegisterGraph registers a graph and its associated function for lookup.
func (s *SessionStore) RegisterGraph(graph *cfg.Graph, fn *ast.FunctionExpr) {
	if graph == nil || fn == nil {
		return
	}
	graphID := graph.ID()
	s.Module.Graphs[graphID] = graph
	s.Module.Funcs[graphID] = fn
	s.Module.GraphToFunc[graph] = fn
	s.Module.FuncToGraph[fn] = graph
}

func newFunctionRegistry() *FunctionRegistry {
	return &FunctionRegistry{
		BySym:     make(map[cfg.SymbolID]*api.FunctionRef),
		ByFunc:    make(map[*ast.FunctionExpr]*api.FunctionRef),
		ByGraphID: make(map[uint64]*api.FunctionRef),
	}
}

// RegisterFunctionRef records a canonical mapping for a function symbol.
func (s *SessionStore) RegisterFunctionRef(
	sym cfg.SymbolID,
	fn *ast.FunctionExpr,
	graph *cfg.Graph,
	parentGraphID uint64,
	defPoint cfg.Point,
) {
	if s == nil || s.Module == nil || sym == 0 || fn == nil || graph == nil {
		return
	}
	reg := s.Module.Functions
	if reg == nil {
		reg = newFunctionRegistry()
		s.Module.Functions = reg
	}
	if existing := reg.BySym[sym]; existing != nil {
		// Preserve parent graph metadata when it was already recorded.
		if existing.ParentGraphID != 0 && parentGraphID == 0 {
			parentGraphID = existing.ParentGraphID
		}
		if existing.DefPoint != 0 && defPoint == 0 {
			defPoint = existing.DefPoint
		}
	}
	ref := &api.FunctionRef{
		Sym:           sym,
		GraphID:       graph.ID(),
		ParentGraphID: parentGraphID,
		DefPoint:      defPoint,
		Func:          fn,
	}
	reg.BySym[sym] = ref
	reg.ByFunc[fn] = ref
	reg.ByGraphID[graph.ID()] = ref
}

// FunctionRefBySym returns the function ref for a symbol.
func (s *SessionStore) FunctionRefBySym(sym cfg.SymbolID) *api.FunctionRef {
	if s == nil || s.Module == nil || s.Module.Functions == nil || sym == 0 {
		return nil
	}
	return s.Module.Functions.BySym[sym]
}

// FunctionRefByFunc returns the function ref for a function literal.
func (s *SessionStore) FunctionRefByFunc(fn *ast.FunctionExpr) *api.FunctionRef {
	if s == nil || s.Module == nil || s.Module.Functions == nil || fn == nil {
		return nil
	}
	return s.Module.Functions.ByFunc[fn]
}

// FunctionRefByGraphID returns the function ref for a graph ID.
func (s *SessionStore) FunctionRefByGraphID(graphID uint64) *api.FunctionRef {
	if s == nil || s.Module == nil || s.Module.Functions == nil || graphID == 0 {
		return nil
	}
	return s.Module.Functions.ByGraphID[graphID]
}

// FuncForSymbol returns the function literal for a symbol.
func (s *SessionStore) FuncForSymbol(sym cfg.SymbolID) *ast.FunctionExpr {
	if ref := s.FunctionRefBySym(sym); ref != nil {
		return ref.Func
	}
	return nil
}

// GraphForSymbol returns the graph for a symbol.
func (s *SessionStore) GraphForSymbol(sym cfg.SymbolID) *cfg.Graph {
	if ref := s.FunctionRefBySym(sym); ref != nil {
		return s.Module.Graphs[ref.GraphID]
	}
	return nil
}

// SymbolForFunc returns the symbol for a function literal.
func (s *SessionStore) SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool) {
	if ref := s.FunctionRefByFunc(fn); ref != nil && ref.Sym != 0 {
		return ref.Sym, true
	}
	return 0, false
}

// RegisterNestedMeta records the parent graph and definition point for a nested graph.
func (s *SessionStore) RegisterNestedMeta(childGraphID, parentGraphID uint64, defPoint cfg.Point) {
	if s == nil || s.Module == nil || childGraphID == 0 || parentGraphID == 0 {
		return
	}
	s.Module.NestedMeta[childGraphID] = api.NestedMeta{
		ParentGraphID: parentGraphID,
		DefPoint:      defPoint,
	}
}

// NestedMetaFor returns nested metadata for a graph ID.
func (s *SessionStore) NestedMetaFor(graphID uint64) (api.NestedMeta, bool) {
	if s == nil || s.Module == nil || graphID == 0 {
		return api.NestedMeta{}, false
	}
	meta, ok := s.Module.NestedMeta[graphID]
	return meta, ok
}

// SetGraphParentHash records the parent scope hash for a graph ID.
func (s *SessionStore) SetGraphParentHash(graphID, parentHash uint64) {
	if s == nil || graphID == 0 {
		return
	}
	s.GraphParentHash[graphID] = parentHash
}

// GraphParentHashOf returns the parent scope hash for a graph ID.
func (s *SessionStore) GraphParentHashOf(graphID uint64) uint64 {
	if s == nil || graphID == 0 {
		return 0
	}
	return s.GraphParentHash[graphID]
}

// FuncForGraph returns the function associated with a graph.
func (s *SessionStore) FuncForGraph(graph *cfg.Graph) *ast.FunctionExpr {
	if graph == nil || s.Module == nil {
		return nil
	}
	return s.Module.GraphToFunc[graph]
}

// GraphForFunc returns the graph associated with a function.
func (s *SessionStore) GraphForFunc(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil || s.Module == nil {
		return nil
	}
	if graph := s.Module.FuncToGraph[fn]; graph != nil {
		return graph
	}
	ids := make([]uint64, 0, len(s.Module.Funcs))
	for id := range s.Module.Funcs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		f := s.Module.Funcs[id]
		if f == fn {
			graph := s.Module.Graphs[id]
			if graph != nil {
				s.Module.FuncToGraph[fn] = graph
			}
			return graph
		}
	}
	return nil
}

// Parents returns the parent scope map.
func (s *SessionStore) Parents() map[uint64]*scope.State {
	return s.Module.Parents
}

// SetParentScope records a parent scope for a given hash.
func (s *SessionStore) SetParentScope(parentHash uint64, parent *scope.State) {
	if s == nil || s.Module == nil {
		return
	}
	s.Module.Parents[parentHash] = parent
}

// ModuleAliases returns the module aliases map.
func (s *SessionStore) ModuleAliases() map[cfg.SymbolID]string {
	return s.Module.ModuleAliases
}

// SetModuleAliases installs the module alias map.
func (s *SessionStore) SetModuleAliases(aliases map[cfg.SymbolID]string) {
	if s == nil || s.Module == nil {
		return
	}
	s.Module.ModuleAliases = aliases
}

// GetInterprocFactsSnapshot returns the stable interproc facts snapshot for a graph.
func (s *SessionStore) GetInterprocFactsSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) api.Facts {
	if s == nil || s.InterprocPrev == nil || s.InterprocPrev.Facts == nil || graph == nil || parent == nil {
		return api.Facts{}
	}
	key := api.KeyForGraph(graph, parent.Hash())
	return s.InterprocPrev.Facts[key]
}

// GetParamHintsSnapshot returns param hints from the stable interproc snapshot.
func (s *SessionStore) GetParamHintsSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) map[cfg.SymbolID][]typ.Type {
	s.requirePhase(api.PhaseScopeCompute)
	return s.GetInterprocFactsSnapshot(graph, parent).ParamHints
}

// GetReturnSummariesSnapshot returns return summaries from the stable interproc snapshot.
func (s *SessionStore) GetReturnSummariesSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) map[cfg.SymbolID][]typ.Type {
	s.requirePhase(api.PhaseScopeCompute)
	return returns.SummaryViewFromFacts(s.GetInterprocFactsSnapshot(graph, parent))
}

// GetNarrowReturnSummariesSnapshot returns post-flow return summaries from the stable snapshot.
func (s *SessionStore) GetNarrowReturnSummariesSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) map[cfg.SymbolID][]typ.Type {
	s.requirePhase(api.PhaseNarrowing)
	return returns.NarrowViewFromFacts(s.GetInterprocFactsSnapshot(graph, parent))
}

// GetLocalFuncTypesSnapshot returns canonical local function types from the stable interproc snapshot.
func (s *SessionStore) GetLocalFuncTypesSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) map[cfg.SymbolID]typ.Type {
	s.requirePhase(api.PhaseScopeCompute)
	return returns.FuncTypeViewFromFacts(s.GetInterprocFactsSnapshot(graph, parent))
}

// GetLiteralSigsSnapshot returns literal signatures from the stable interproc snapshot.
func (s *SessionStore) GetLiteralSigsSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) map[*ast.FunctionExpr]*typ.Function {
	s.requirePhase(api.PhaseScopeCompute, api.PhaseNarrowing)
	return s.GetInterprocFactsSnapshot(graph, parent).LiteralSigs
}

// GetCapturedTypesSnapshot returns captured variable types from the stable interproc snapshot.
func (s *SessionStore) GetCapturedTypesSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) api.CapturedTypes {
	s.requirePhase(api.PhaseScopeCompute)
	return s.GetInterprocFactsSnapshot(graph, parent).CapturedTypes
}

// StoreLiteralSigs records literal signatures for the current iteration.
func (s *SessionStore) StoreLiteralSigs(graphID uint64, sigs map[*ast.FunctionExpr]*typ.Function) {
	if s == nil || graphID == 0 || len(sigs) == 0 {
		return
	}
	if s.Scratch == nil {
		s.Scratch = NewIterationScratch()
	}
	if s.Scratch.LiteralSigsByGraphID == nil {
		s.Scratch.LiteralSigsByGraphID = make(map[uint64]map[*ast.FunctionExpr]*typ.Function)
	}
	s.Scratch.LiteralSigsByGraphID[graphID] = sigs
}

// ScratchLiteralSigs returns literal signatures computed in the current iteration.
// This is an iteration-local cache used to avoid re-synthesizing literal signatures
// for nested functions within the same fixpoint cycle.
func (s *SessionStore) ScratchLiteralSigs(graphID uint64) map[*ast.FunctionExpr]*typ.Function {
	if s == nil || s.Scratch == nil || s.Scratch.LiteralSigsByGraphID == nil {
		return nil
	}
	return s.Scratch.LiteralSigsByGraphID[graphID]
}

// GetCapturedFieldAssignsSnapshot returns captured field assignments from the stable interproc snapshot.
func (s *SessionStore) GetCapturedFieldAssignsSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) api.CapturedFieldAssigns {
	s.requirePhase(api.PhaseScopeCompute, api.PhaseNarrowing)
	return s.GetInterprocFactsSnapshot(graph, parent).CapturedFields
}

// GetCapturedContainerMutationsSnapshot returns captured container mutations from the stable interproc snapshot.
func (s *SessionStore) GetCapturedContainerMutationsSnapshot(
	graph *cfg.Graph,
	parent *scope.State,
) api.CapturedContainerMutations {
	s.requirePhase(api.PhaseScopeCompute, api.PhaseNarrowing)
	return s.GetInterprocFactsSnapshot(graph, parent).CapturedContainers
}

// snapshotRefinementStore implements api.RefinementStore using the stable snapshot.
type snapshotRefinementStore struct {
	refinements map[cfg.SymbolID]*constraint.FunctionRefinement
}

func (o *snapshotRefinementStore) LookupRefinementBySym(sym cfg.SymbolID) *constraint.FunctionRefinement {
	if o == nil || sym == 0 {
		return nil
	}
	if o.refinements == nil {
		return nil
	}
	if refinement := o.refinements[sym]; refinement != nil {
		return refinement
	}
	return nil
}

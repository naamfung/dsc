// env.go defines phase-typed environments for synthesis.
// DeclaredEnv is used pre-flow; NarrowEnv is used post-flow.
// This split prevents pre-flow return summaries from being accessed in narrowing.
package api

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// Phase identifies which pipeline stage is active.
// The phase also determines whether flow-refined types are available:
//   - PhaseNarrowing enables flow-refined types
//   - all earlier phases are declared-only
type Phase uint8

const (
	PhaseScopeCompute Phase = iota
	PhaseTypeResolution
	PhaseNarrowing
)

func (p Phase) String() string {
	switch p {
	case PhaseScopeCompute:
		return "ScopeCompute"
	case PhaseTypeResolution:
		return "TypeResolution"
	case PhaseNarrowing:
		return "Narrowing"
	default:
		return "Unknown"
	}
}

// BaseEnv is the shared environment interface for synthesis.
// It intentionally excludes return summaries to prevent cross-phase misuse.
type BaseEnv interface {
	Phase() Phase
	Graph() cfg.VersionedGraph
	Types() flow.TypeFacts
	Consts() *flow.Solution
	Refinements() RefinementFacts
	TypeNames() *scope.State
	Bindings() *bind.BindingTable
	ModuleAliases() map[cfg.SymbolID]string
	ModuleAlias(sym cfg.SymbolID) string
	GlobalType(sym cfg.SymbolID) (typ.Type, bool)
	GlobalTypes() map[string]typ.Type
	WithGlobalOverlay(overlay map[string]typ.Type) BaseEnv
}

// DeclaredEnv provides access to pre-flow return summaries.
type DeclaredEnv interface {
	BaseEnv
	ReturnSummaries() map[cfg.SymbolID][]typ.Type
}

// NarrowEnv provides access to post-flow return summaries.
type NarrowEnv interface {
	BaseEnv
	NarrowReturnSummaries() map[cfg.SymbolID][]typ.Type
}

type envBase struct {
	phase         Phase
	graph         cfg.VersionedGraph
	bindings      *bind.BindingTable
	types         flow.TypeFacts
	solution      *flow.Solution
	refinements   RefinementFacts
	typeNames     *scope.State
	moduleAliases map[cfg.SymbolID]string
	globalTypes   map[string]typ.Type
}

func (b *envBase) withGlobalOverlay(overlay map[string]typ.Type) *envBase {
	if b == nil || len(overlay) == 0 {
		return b
	}
	merged := make(map[string]typ.Type, len(b.globalTypes)+len(overlay))
	for k, v := range b.globalTypes {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	next := *b
	next.globalTypes = merged
	return &next
}

type envCommon struct {
	base *envBase
}

func (c *envCommon) withGlobalOverlay(overlay map[string]typ.Type) *envCommon {
	if c == nil || len(overlay) == 0 {
		return c
	}
	return &envCommon{base: c.base.withGlobalOverlay(overlay)}
}

// Phase returns the current checking phase.
func (c *envCommon) Phase() Phase {
	if c == nil || c.base == nil {
		return PhaseScopeCompute
	}
	return c.base.phase
}

// Graph returns the versioned CFG graph.
func (c *envCommon) Graph() cfg.VersionedGraph {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.graph
}

// Types returns the type facts provider.
func (c *envCommon) Types() flow.TypeFacts {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.types
}

// Consts returns the flow solution for constant value lookup.
func (c *envCommon) Consts() *flow.Solution {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.solution
}

// Refinements returns the refinement facts provider.
func (c *envCommon) Refinements() RefinementFacts {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.refinements
}

// TypeNames returns the scope state for type name resolution.
func (c *envCommon) TypeNames() *scope.State {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.typeNames
}

// Bindings returns the binding table for AST-based symbol resolution.
func (c *envCommon) Bindings() *bind.BindingTable {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.bindings
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (c *envCommon) ModuleAliases() map[cfg.SymbolID]string {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.moduleAliases
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (c *envCommon) ModuleAlias(sym cfg.SymbolID) string {
	if c == nil || c.base == nil || c.base.moduleAliases == nil {
		return ""
	}
	return c.base.moduleAliases[sym]
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (c *envCommon) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if c == nil || c.base == nil || c.base.globalTypes == nil || sym == 0 {
		return nil, false
	}
	if c.base.bindings == nil {
		return nil, false
	}
	kind, ok := c.base.bindings.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		return nil, false
	}
	if name := c.base.bindings.Name(sym); name != "" {
		if t, found := c.base.globalTypes[name]; found && t != nil {
			return t, true
		}
	}
	return nil, false
}

// GlobalTypes returns the global type map.
func (c *envCommon) GlobalTypes() map[string]typ.Type {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.globalTypes
}

// DeclaredEnvImpl is the concrete declared-phase environment.
type DeclaredEnvImpl struct {
	*envCommon
	returnSummaries map[cfg.SymbolID][]typ.Type
}

// NarrowEnvImpl is the concrete narrowing-phase environment.
type NarrowEnvImpl struct {
	*envCommon
	narrowReturns map[cfg.SymbolID][]typ.Type
}

var _ BaseEnv = (*DeclaredEnvImpl)(nil)
var _ BaseEnv = (*NarrowEnvImpl)(nil)
var _ DeclaredEnv = (*DeclaredEnvImpl)(nil)
var _ NarrowEnv = (*NarrowEnvImpl)(nil)

// Phase returns the current checking phase.
func (e *DeclaredEnvImpl) Phase() Phase {
	if e == nil {
		return PhaseScopeCompute
	}
	return e.envCommon.Phase()
}

// Phase returns the current checking phase.
func (e *NarrowEnvImpl) Phase() Phase {
	if e == nil {
		return PhaseScopeCompute
	}
	return e.envCommon.Phase()
}

// Graph returns the versioned CFG graph.
func (e *DeclaredEnvImpl) Graph() cfg.VersionedGraph {
	if e == nil {
		return nil
	}
	return e.envCommon.Graph()
}

// Graph returns the versioned CFG graph.
func (e *NarrowEnvImpl) Graph() cfg.VersionedGraph {
	if e == nil {
		return nil
	}
	return e.envCommon.Graph()
}

// Types returns the type facts provider.
func (e *DeclaredEnvImpl) Types() flow.TypeFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Types()
}

// Types returns the type facts provider.
func (e *NarrowEnvImpl) Types() flow.TypeFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Types()
}

// Consts returns the flow solution for constant value lookup.
func (e *DeclaredEnvImpl) Consts() *flow.Solution {
	if e == nil {
		return nil
	}
	return e.envCommon.Consts()
}

// Consts returns the flow solution for constant value lookup.
func (e *NarrowEnvImpl) Consts() *flow.Solution {
	if e == nil {
		return nil
	}
	return e.envCommon.Consts()
}

// Refinements returns the refinement facts provider.
func (e *DeclaredEnvImpl) Refinements() RefinementFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Refinements()
}

// Refinements returns the refinement facts provider.
func (e *NarrowEnvImpl) Refinements() RefinementFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Refinements()
}

// TypeNames returns the scope state for type name resolution.
func (e *DeclaredEnvImpl) TypeNames() *scope.State {
	if e == nil {
		return nil
	}
	return e.envCommon.TypeNames()
}

// TypeNames returns the scope state for type name resolution.
func (e *NarrowEnvImpl) TypeNames() *scope.State {
	if e == nil {
		return nil
	}
	return e.envCommon.TypeNames()
}

// Bindings returns the binding table for AST-based symbol resolution.
func (e *DeclaredEnvImpl) Bindings() *bind.BindingTable {
	if e == nil {
		return nil
	}
	return e.envCommon.Bindings()
}

// Bindings returns the binding table for AST-based symbol resolution.
func (e *NarrowEnvImpl) Bindings() *bind.BindingTable {
	if e == nil {
		return nil
	}
	return e.envCommon.Bindings()
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (e *DeclaredEnvImpl) ModuleAliases() map[cfg.SymbolID]string {
	if e == nil {
		return nil
	}
	return e.envCommon.ModuleAliases()
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (e *NarrowEnvImpl) ModuleAliases() map[cfg.SymbolID]string {
	if e == nil {
		return nil
	}
	return e.envCommon.ModuleAliases()
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (e *DeclaredEnvImpl) ModuleAlias(sym cfg.SymbolID) string {
	if e == nil {
		return ""
	}
	return e.envCommon.ModuleAlias(sym)
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (e *NarrowEnvImpl) ModuleAlias(sym cfg.SymbolID) string {
	if e == nil {
		return ""
	}
	return e.envCommon.ModuleAlias(sym)
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (e *DeclaredEnvImpl) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if e == nil {
		return nil, false
	}
	return e.envCommon.GlobalType(sym)
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (e *NarrowEnvImpl) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if e == nil {
		return nil, false
	}
	return e.envCommon.GlobalType(sym)
}

// GlobalTypes returns the global type map.
func (e *DeclaredEnvImpl) GlobalTypes() map[string]typ.Type {
	if e == nil {
		return nil
	}
	return e.envCommon.GlobalTypes()
}

// GlobalTypes returns the global type map.
func (e *NarrowEnvImpl) GlobalTypes() map[string]typ.Type {
	if e == nil {
		return nil
	}
	return e.envCommon.GlobalTypes()
}

// WithGlobalOverlay returns a derived Env with additional globals merged in.
func (e *DeclaredEnvImpl) WithGlobalOverlay(overlay map[string]typ.Type) BaseEnv {
	if e == nil {
		return e
	}
	if len(overlay) == 0 {
		return e
	}
	next := *e
	next.envCommon = e.withGlobalOverlay(overlay)
	return &next
}

// WithGlobalOverlay returns a derived Env with additional globals merged in.
func (e *NarrowEnvImpl) WithGlobalOverlay(overlay map[string]typ.Type) BaseEnv {
	if e == nil {
		return e
	}
	if len(overlay) == 0 {
		return e
	}
	next := *e
	next.envCommon = e.withGlobalOverlay(overlay)
	return &next
}

// ReturnSummaries returns the return type summaries for sibling functions.
func (e *DeclaredEnvImpl) ReturnSummaries() map[cfg.SymbolID][]typ.Type {
	if e == nil {
		return nil
	}
	return e.returnSummaries
}

// NarrowReturnSummaries returns post-flow return summaries for narrowing.
func (e *NarrowEnvImpl) NarrowReturnSummaries() map[cfg.SymbolID][]typ.Type {
	if e == nil {
		return nil
	}
	return e.narrowReturns
}

// DeclaredEnvConfig holds inputs for building a declared-phase Env.
type DeclaredEnvConfig struct {
	Graph           cfg.VersionedGraph
	Bindings        *bind.BindingTable
	DeclaredTypes   flow.DeclaredTypes
	AnnotatedVars   map[cfg.SymbolID]bool
	BaseScope       *scope.State
	RefinementStore RefinementStore
	ModuleAliases   map[cfg.SymbolID]string
	GlobalTypes     map[string]typ.Type
	SiblingTypes    map[cfg.SymbolID]typ.Type
	LiteralTypes    flow.DeclaredTypes
	ReturnSummaries map[cfg.SymbolID][]typ.Type
}

// NarrowEnvConfig holds inputs for building a narrowing-phase Env.
type NarrowEnvConfig struct {
	Graph                 cfg.VersionedGraph
	Bindings              *bind.BindingTable
	DeclaredTypes         flow.DeclaredTypes
	AnnotatedVars         map[cfg.SymbolID]bool
	Solution              *flow.Solution
	BaseScope             *scope.State
	RefinementStore       RefinementStore
	ModuleAliases         map[cfg.SymbolID]string
	GlobalTypes           map[string]typ.Type
	SiblingTypes          map[cfg.SymbolID]typ.Type
	LiteralTypes          flow.DeclaredTypes
	NarrowReturnSummaries map[cfg.SymbolID][]typ.Type
}

func newEnvBase(
	phase Phase,
	graph cfg.VersionedGraph,
	bindings *bind.BindingTable,
	types flow.TypeFacts,
	solution *flow.Solution,
	refinements RefinementFacts,
	typeNames *scope.State,
	moduleAliases map[cfg.SymbolID]string,
	globalTypes map[string]typ.Type,
) *envBase {
	return &envBase{
		phase:         phase,
		graph:         graph,
		bindings:      bindings,
		types:         types,
		solution:      solution,
		refinements:   refinements,
		typeNames:     typeNames,
		moduleAliases: moduleAliases,
		globalTypes:   globalTypes,
	}
}

// NewDeclaredEnv creates a declared-phase Env.
func NewDeclaredEnv(cfg DeclaredEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		PhaseScopeCompute,
		cfg.Graph,
		cfg.Bindings,
		newUnifiedTypeFacts(cfg.Graph, cfg.DeclaredTypes, cfg.SiblingTypes, cfg.LiteralTypes, cfg.AnnotatedVars, nil),
		nil,
		NewRefinementFacts(cfg.RefinementStore),
		cfg.BaseScope,
		cfg.ModuleAliases,
		cfg.GlobalTypes,
	)
	return &DeclaredEnvImpl{envCommon: &envCommon{base: base}, returnSummaries: cfg.ReturnSummaries}
}

// NewNarrowEnv creates a narrowing-phase Env.
func NewNarrowEnv(cfg NarrowEnvConfig) *NarrowEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		PhaseNarrowing,
		cfg.Graph,
		cfg.Bindings,
		newUnifiedTypeFacts(cfg.Graph, cfg.DeclaredTypes, cfg.SiblingTypes, cfg.LiteralTypes, cfg.AnnotatedVars, cfg.Solution),
		cfg.Solution,
		NewRefinementFacts(cfg.RefinementStore),
		cfg.BaseScope,
		cfg.ModuleAliases,
		cfg.GlobalTypes,
	)
	return &NarrowEnvImpl{envCommon: &envCommon{base: base}, narrowReturns: cfg.NarrowReturnSummaries}
}

// ReturnInferenceEnvConfig holds inputs for return type inference.
type ReturnInferenceEnvConfig struct {
	Graph           cfg.VersionedGraph
	Bindings        *bind.BindingTable
	BaseScope       *scope.State
	DeclaredTypes   flow.DeclaredTypes
	GlobalTypes     map[string]typ.Type
	ModuleAliases   map[cfg.SymbolID]string
	ReturnSummaries map[cfg.SymbolID][]typ.Type
}

// NewReturnInferenceEnv creates a declared-phase Env for return inference.
func NewReturnInferenceEnv(cfg ReturnInferenceEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		PhaseScopeCompute,
		cfg.Graph,
		cfg.Bindings,
		newUnifiedTypeFacts(cfg.Graph, cfg.DeclaredTypes, nil, nil, nil, nil),
		nil,
		NewRefinementFacts(nil),
		cfg.BaseScope,
		cfg.ModuleAliases,
		cfg.GlobalTypes,
	)
	return &DeclaredEnvImpl{envCommon: &envCommon{base: base}, returnSummaries: cfg.ReturnSummaries}
}

// unifiedTypeFacts implements flow.TypeFacts with layered type source lookup.
type unifiedTypeFacts struct {
	graph         cfg.VersionedGraph
	declaredTypes flow.DeclaredTypes
	siblingTypes  map[cfg.SymbolID]typ.Type
	literalTypes  flow.DeclaredTypes
	annotatedVars map[cfg.SymbolID]bool
	solution      *flow.Solution
}

func newUnifiedTypeFacts(
	graph cfg.VersionedGraph,
	declared flow.DeclaredTypes,
	siblings map[cfg.SymbolID]typ.Type,
	literals flow.DeclaredTypes,
	annotated map[cfg.SymbolID]bool,
	solution *flow.Solution,
) flow.TypeFacts {
	return &unifiedTypeFacts{
		graph:         graph,
		declaredTypes: declared,
		siblingTypes:  siblings,
		literalTypes:  literals,
		annotatedVars: annotated,
		solution:      solution,
	}
}

// DeclaredAt returns the declared type for a symbol at a CFG point.
// Searches: literal types, sibling function types, then declared types.
func (f *unifiedTypeFacts) DeclaredAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if sym == 0 {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	// For explicitly annotated symbols, prefer the declared type over literal overlays.
	if f.annotatedVars != nil && f.annotatedVars[sym] {
		if f.declaredTypes != nil {
			if t, ok := f.declaredTypes[sym]; ok && t != nil {
				return f.toTypedValue(t)
			}
		}
	}
	if f.siblingTypes != nil {
		if t, ok := f.siblingTypes[sym]; ok && t != nil {
			return f.toTypedValue(t)
		}
	}
	if f.declaredTypes != nil {
		if t, ok := f.declaredTypes[sym]; ok && t != nil {
			return f.toTypedValue(t)
		}
	}
	// Literal overlays are synthesized from function/table literals and can lag
	// behind canonical declared/sibling symbol types during fixpoint iterations.
	// Keep them as fallback only when no canonical symbol type is available.
	if f.literalTypes != nil {
		if f.annotatedVars == nil || !f.annotatedVars[sym] {
			if t, ok := f.literalTypes[sym]; ok && t != nil {
				return f.toTypedValue(t)
			}
		}
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

// RefinedAt returns the flow-refined type for a symbol at a CFG point.
// Returns nil type if no solution is available or symbol is unknown.
func (f *unifiedTypeFacts) RefinedAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if f == nil || sym == 0 {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	if f.solution == nil {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	return f.solution.RefinedAt(p, sym)
}

// EffectiveTypeAt returns the effective type for a symbol at a CFG point.
// Prefers refined (narrowed) type if available, otherwise falls back to declared.
func (f *unifiedTypeFacts) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	refined := f.RefinedAt(p, sym)
	if refined.Type != nil && refined.State == flow.StateResolved {
		return refined
	}
	return f.DeclaredAt(p, sym)
}

// IsAnnotated returns true if the symbol has an explicit type annotation.
func (f *unifiedTypeFacts) IsAnnotated(sym cfg.SymbolID) bool {
	if f.annotatedVars == nil {
		return false
	}
	return f.annotatedVars[sym]
}

func (f *unifiedTypeFacts) toTypedValue(t typ.Type) flow.TypedValue {
	if typ.IsUnknown(t) {
		return flow.TypedValue{Type: t, State: flow.StateUnknown}
	}
	return flow.TypedValue{Type: t, State: flow.StateResolved}
}

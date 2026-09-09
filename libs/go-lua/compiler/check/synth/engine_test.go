package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/extract"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type mockTypeQuerier struct{}

func (m mockTypeQuerier) Field(_ *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if rec, ok := t.(*typ.Record); ok {
		if f := rec.GetField(name); f != nil {
			return f.Type, true
		}
	}
	if mp, ok := t.(*typ.Map); ok {
		if mp.Value != nil {
			return typ.NewOptional(mp.Value), true
		}
	}
	return nil, false
}

func (m mockTypeQuerier) Index(_ *db.QueryContext, t typ.Type, key typ.Type) (typ.Type, bool) {
	if arr, ok := t.(*typ.Array); ok {
		return arr.Element, true
	}
	return nil, false
}

func (m mockTypeQuerier) Method(_ *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	return nil, false
}

func (m mockTypeQuerier) BinaryOp(_ *db.QueryContext, left typ.Type, op string, right typ.Type) typ.Type {
	return typ.Number
}

func (m mockTypeQuerier) UnaryOp(_ *db.QueryContext, op string, operand typ.Type) typ.Type {
	return querycore.UnaryOp(op, operand)
}

func (m mockTypeQuerier) IsSubtype(_ *db.QueryContext, sub, super typ.Type) bool {
	return false
}

func (m mockTypeQuerier) ExpandInstantiated(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}

func (m mockTypeQuerier) Widen(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}

func (m mockTypeQuerier) WidenForInference(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}

type mockFlowOps struct {
	narrowed map[cfg.SymbolID]typ.Type
}

func (m mockFlowOps) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if m.narrowed == nil {
		return nil
	}
	return m.narrowed[path.Symbol]
}

func (m mockFlowOps) BoundsAt(p cfg.Point, name string) (lower, upper int64, ok bool) {
	return 0, 0, false
}

func (m mockFlowOps) ArrayLenBoundAt(p cfg.Point, varName string) (arrKey string, ok bool) {
	return "", false
}

func (m mockFlowOps) ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (arrKey string, offset int64, ok bool) {
	return "", 0, false
}

func (m mockFlowOps) IsPointDead(p cfg.Point) bool {
	return false
}

func (m mockFlowOps) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	return false
}

type mockGraph struct {
	symbols map[string]cfg.SymbolID
}

func (m mockGraph) ID() uint64                                                  { return 0 }
func (m mockGraph) Entry() cfg.Point                                            { return 0 }
func (m mockGraph) Exit() cfg.Point                                             { return 0 }
func (m mockGraph) Node(p cfg.Point) *cfg.Node                                  { return nil }
func (m mockGraph) RPO() []cfg.Point                                            { return nil }
func (m mockGraph) Predecessors(p cfg.Point) []cfg.Point                        { return nil }
func (m mockGraph) Successor(p cfg.Point) cfg.Point                             { return 0 }
func (m mockGraph) Successors(p cfg.Point) []cfg.Point                          { return nil }
func (m mockGraph) Edges() []cfg.Edge                                           { return nil }
func (m mockGraph) Size() int                                                   { return 0 }
func (m mockGraph) EdgeCond(from, to cfg.Point) (bool, bool)                    { return false, false }
func (m mockGraph) IsJoin(p cfg.Point) bool                                     { return false }
func (m mockGraph) IsBranch(p cfg.Point) bool                                   { return false }
func (m mockGraph) PhiNodes() []cfg.PhiNode                                     { return nil }
func (m mockGraph) VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version    { return cfg.Version{} }
func (m mockGraph) AllVisibleVersions(p cfg.Point) map[cfg.SymbolID]cfg.Version { return nil }
func (m mockGraph) SymbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	if m.symbols == nil {
		return 0, false
	}
	sym, ok := m.symbols[name]
	return sym, ok
}
func (m mockGraph) AllSymbolsAt(p cfg.Point) map[string]cfg.SymbolID { return m.symbols }
func (m mockGraph) DefVersionAt(p cfg.Point, sym cfg.SymbolID) (cfg.Version, bool) {
	return cfg.Version{}, false
}
func (m mockGraph) DeclarationPoint(sym cfg.SymbolID) (cfg.Point, bool) { return 0, false }
func (m mockGraph) ParamNames() []string                                { return nil }
func (m mockGraph) ParamSymbols() []cfg.SymbolID                        { return nil }
func (m mockGraph) ParamDeclPoints() []cfg.Point                        { return nil }
func (m mockGraph) NameOf(sym cfg.SymbolID) string {
	for name, s := range m.symbols {
		if s == sym {
			return name
		}
	}
	return ""
}
func (m mockGraph) SymbolKind(sym cfg.SymbolID) (cfg.SymbolKind, bool) {
	return cfg.SymbolUnknown, false
}

func TestNewNarrowedEngine(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	engine := New(Config{
		Ctx:    ctx,
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	})

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestEngine_ImplementsSynth(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	})

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	var _ Synth = engine
}

func TestEngine_Narrow_NilWithoutFlow(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	})

	narrow := engine.Narrow()
	if narrow != nil {
		t.Fatal("Narrow() should return nil without flow querier")
	}
}

func TestEngine_Narrow_NonNilWithFlow(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   mockFlowOps{},
		Phase:  api.PhaseNarrowing,
	})

	narrow := engine.Narrow()
	if narrow == nil {
		t.Fatal("Narrow() should return non-nil with flow querier")
	}
}

func TestEngine_CachesInitialized(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	})

	// Verify caches work by synthesizing same expression twice
	expr := &ast.NumberExpr{Value: "42"}
	t1 := engine.TypeOf(expr, 0)
	t2 := engine.TypeOf(expr, 0)
	if t1 != t2 {
		t.Fatal("caching not working")
	}
}

func TestEngine_UsesProvidedCaches(t *testing.T) {
	preCache := make(api.Cache)
	narrowCache := make(api.Cache)

	engine := New(Config{
		Ctx:         db.NewQueryContext(db.New()),
		Types:       mockTypeQuerier{},
		Scopes:      make(api.ScopeMap),
		PreCache:    preCache,
		NarrowCache: narrowCache,
	})

	// Verify caches work by synthesizing same expression twice
	expr := &ast.NumberExpr{Value: "42"}
	t1 := engine.TypeOf(expr, 0)
	t2 := engine.TypeOf(expr, 0)
	if t1 != t2 {
		t.Fatal("caching not working with provided caches")
	}
}

func TestEngine_PhaseGating_DeclaredDisallowsReturnTransforms(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Phase:  api.PhaseScopeCompute,
	})

	if engine.AllowReturnTransforms() {
		t.Fatal("DeclaredEngine should not allow return transforms")
	}
}

func TestEngine_PhaseGating_NarrowingAllowsReturnTransforms(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   mockFlowOps{},
		Phase:  api.PhaseNarrowing,
	})

	if !engine.AllowReturnTransforms() {
		t.Fatal("PhaseNarrowing should allow return transforms")
	}
}

func TestEngine_PhaseGating_TypeResolutionDisallowsReturnTransforms(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Phase:  api.PhaseTypeResolution,
	})

	if engine.AllowReturnTransforms() {
		t.Fatal("DeclaredEngine should not allow return transforms")
	}
}

func TestEngine_PhaseGating_ScopeComputeDisallowsReturnTransforms(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Phase:  api.PhaseScopeCompute,
	})

	if engine.AllowReturnTransforms() {
		t.Fatal("DeclaredEngine should not allow return transforms")
	}
}

func TestEngine_PhaseGating_DefaultPhaseDisallowsReturnTransforms(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	})

	if engine.AllowReturnTransforms() {
		t.Fatal("DeclaredEngine should not allow return transforms")
	}
}

// Phase Isolation Tests - Prove that phases cannot cross boundaries

func TestDeclaredEngine_DoesNotAllowReturnTransforms(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Phase:  api.PhaseScopeCompute,
	})

	if engine.AllowReturnTransforms() {
		t.Fatal("DeclaredEngine should never allow return transforms")
	}
}

func TestDeclaredEngine_TypeResolutionPhase(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Phase:  api.PhaseTypeResolution,
	})

	if engine.AllowReturnTransforms() {
		t.Fatal("DeclaredEngine in TypeResolution should not allow return transforms")
	}
	if engine.Phase() != api.PhaseTypeResolution {
		t.Fatal("DeclaredEngine should preserve phase")
	}
}

func TestNarrowedEngine_AllowsReturnTransforms(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   mockFlowOps{},
		Phase:  api.PhaseNarrowing,
	})

	if !engine.AllowReturnTransforms() {
		t.Fatal("NarrowedEngine should allow return transforms")
	}
}

func TestNarrowedEngine_HasFlow(t *testing.T) {
	flow := mockFlowOps{narrowed: map[cfg.SymbolID]typ.Type{1: typ.String}}
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   flow,
		Phase:  api.PhaseNarrowing,
	})

	// Verify narrowing is active by checking that Narrow() returns non-nil
	if engine.Narrow() == nil {
		t.Fatal("NarrowedEngine should have flow configured")
	}
}

func TestNarrowedEngine_IsInNarrowingPhase(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   mockFlowOps{},
		Phase:  api.PhaseNarrowing,
	})

	if engine.Phase() != api.PhaseNarrowing {
		t.Fatalf("NarrowedEngine should be in PhaseNarrowing, got %v", engine.Phase())
	}
	if !engine.IsNarrowing() {
		t.Fatal("NarrowedEngine.IsNarrowing() should return true")
	}
}

func TestDeclaredEngine_ImplementsSynth(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	})

	var _ extract.Synth = engine
}

func TestNarrowedEngine_ImplementsSynth(t *testing.T) {
	engine := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   mockFlowOps{},
		Phase:  api.PhaseNarrowing,
	})

	var _ Synth = engine
}

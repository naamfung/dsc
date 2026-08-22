package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type mockGraph struct {
	symbols map[cfg.Point]map[string]cfg.SymbolID
	names   map[cfg.SymbolID]string
	decls   map[cfg.SymbolID]cfg.Point
	kinds   map[cfg.SymbolID]cfg.SymbolKind
}

func newMockGraph() *mockGraph {
	return &mockGraph{
		symbols: make(map[cfg.Point]map[string]cfg.SymbolID),
		names:   make(map[cfg.SymbolID]string),
		decls:   make(map[cfg.SymbolID]cfg.Point),
		kinds:   make(map[cfg.SymbolID]cfg.SymbolKind),
	}
}

func (g *mockGraph) addSymbol(p cfg.Point, name string, sym cfg.SymbolID, kind cfg.SymbolKind) {
	if g.symbols[p] == nil {
		g.symbols[p] = make(map[string]cfg.SymbolID)
	}
	g.symbols[p][name] = sym
	g.names[sym] = name
	g.decls[sym] = p
	g.kinds[sym] = kind
}

func (g *mockGraph) ID() uint64                               { return 1 }
func (g *mockGraph) Entry() cfg.Point                         { return 1 }
func (g *mockGraph) Exit() cfg.Point                          { return 100 }
func (g *mockGraph) Node(p cfg.Point) *cfg.Node               { return nil }
func (g *mockGraph) RPO() []cfg.Point                         { return nil }
func (g *mockGraph) Predecessors(p cfg.Point) []cfg.Point     { return nil }
func (g *mockGraph) Successor(p cfg.Point) cfg.Point          { return 0 }
func (g *mockGraph) Successors(p cfg.Point) []cfg.Point       { return nil }
func (g *mockGraph) Edges() []cfg.Edge                        { return nil }
func (g *mockGraph) Size() int                                { return 0 }
func (g *mockGraph) EdgeCond(from, to cfg.Point) (bool, bool) { return false, false }
func (g *mockGraph) IsJoin(p cfg.Point) bool                  { return false }
func (g *mockGraph) IsBranch(p cfg.Point) bool                { return false }

func (g *mockGraph) VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	if g.decls[sym] != 0 {
		return cfg.Version{Root: g.names[sym], Symbol: sym, ID: 1}
	}
	return cfg.Version{}
}

func (g *mockGraph) AllVisibleVersions(p cfg.Point) map[cfg.SymbolID]cfg.Version { return nil }
func (g *mockGraph) PhiNodes() []cfg.PhiNode                                     { return nil }

func (g *mockGraph) SymbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	if syms := g.symbols[p]; syms != nil {
		if sym, ok := syms[name]; ok {
			return sym, true
		}
	}
	return 0, false
}

func (g *mockGraph) AllSymbolsAt(p cfg.Point) map[string]cfg.SymbolID { return g.symbols[p] }

func (g *mockGraph) DeclarationPoint(sym cfg.SymbolID) (cfg.Point, bool) {
	if p, ok := g.decls[sym]; ok {
		return p, true
	}
	return 0, false
}

func (g *mockGraph) NameOf(sym cfg.SymbolID) string { return g.names[sym] }

func (g *mockGraph) SymbolKind(sym cfg.SymbolID) (cfg.SymbolKind, bool) {
	if k, ok := g.kinds[sym]; ok {
		return k, true
	}
	return cfg.SymbolUnknown, false
}

func (g *mockGraph) ParamNames() []string         { return nil }
func (g *mockGraph) ParamSymbols() []cfg.SymbolID { return nil }
func (g *mockGraph) ParamDeclPoints() []cfg.Point { return nil }

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseTypeResolution, "TypeResolution"},
		{PhaseScopeCompute, "ScopeCompute"},
		{PhaseNarrowing, "Narrowing"},
		{Phase(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestDeclaredEnv_NilSafety(t *testing.T) {
	var env *DeclaredEnvImpl
	if env.Phase() != PhaseScopeCompute {
		t.Errorf("nil.Phase() = %v, want PhaseScopeCompute", env.Phase())
	}
	if env.Graph() != nil {
		t.Error("nil.Graph() should be nil")
	}
	if env.Types() != nil {
		t.Error("nil.Types() should be nil")
	}
	if env.Consts() != nil {
		t.Error("nil.Consts() should be nil")
	}
	if env.Refinements() != nil {
		t.Error("nil.Refinements() should be nil")
	}
	if env.TypeNames() != nil {
		t.Error("nil.TypeNames() should be nil")
	}
}

func TestNewEnv_NilGraph(t *testing.T) {
	env := NewDeclaredEnv(DeclaredEnvConfig{Graph: nil})
	if env != nil {
		t.Error("expected nil environment for nil graph")
	}
	env2 := NewNarrowEnv(NarrowEnvConfig{Graph: nil})
	if env2 != nil {
		t.Error("expected nil environment for nil graph")
	}
}

func TestDeclaredEnv_TypeFacts(t *testing.T) {
	graph := newMockGraph()
	point := cfg.Point(10)
	symX := cfg.SymbolID(1)
	graph.addSymbol(point, "x", symX, cfg.SymbolLocal)

	env := NewDeclaredEnv(DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: flow.DeclaredTypes{symX: typ.Number},
		BaseScope:     scope.NewWithBuiltins(),
	})

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Phase() != PhaseScopeCompute {
		t.Errorf("Phase() = %v, want PhaseScopeCompute", env.Phase())
	}

	facts := env.Types()
	if facts == nil {
		t.Fatal("expected non-nil TypeFacts")
	}

	declared := facts.DeclaredAt(point, symX)
	if declared.Type != typ.Number {
		t.Errorf("DeclaredAt() = %v, want Number", declared.Type)
	}
}

func TestNarrowingEnv_TypeFacts(t *testing.T) {
	graph := newMockGraph()
	point := cfg.Point(10)
	symX := cfg.SymbolID(1)
	graph.addSymbol(point, "x", symX, cfg.SymbolLocal)

	inputs := &flow.Inputs{
		Graph:         graph,
		DeclaredTypes: flow.DeclaredTypes{symX: typ.Any},
	}

	resolver := &core.FuncResolver{
		FieldFunc: func(t typ.Type, name string) (typ.Type, bool) { return nil, false },
		IndexFunc: func(t typ.Type, key typ.Type) (typ.Type, bool) { return nil, false },
	}

	solution := flow.Solve(inputs, resolver)

	env := NewNarrowEnv(NarrowEnvConfig{
		Graph:         graph,
		DeclaredTypes: flow.DeclaredTypes{symX: typ.Any},
		Solution:      solution,
		BaseScope:     scope.NewWithBuiltins(),
	})

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Phase() != PhaseNarrowing {
		t.Errorf("Phase() = %v, want PhaseNarrowing", env.Phase())
	}
}

func TestTypeNameFacts(t *testing.T) {
	baseScope := scope.NewWithBuiltins()
	baseScope = baseScope.WithType("MyType", typ.String)

	env := NewDeclaredEnv(DeclaredEnvConfig{
		Graph:     newMockGraph(),
		BaseScope: baseScope,
	})

	typeNames := env.TypeNames()
	if typeNames == nil {
		t.Fatal("expected non-nil TypeNames")
	}

	numType, ok := typeNames.LookupType("number")
	if !ok || numType != typ.Number {
		t.Errorf("LookupType(number) = (%v, %v), want (Number, true)", numType, ok)
	}

	myType, ok := typeNames.LookupType("MyType")
	if !ok || myType != typ.String {
		t.Errorf("LookupType(MyType) = (%v, %v), want (String, true)", myType, ok)
	}
}

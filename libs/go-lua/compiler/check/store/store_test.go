package store

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewInterprocState(t *testing.T) {
	state := NewInterprocState()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Facts == nil {
		t.Error("Facts map should be initialized")
	}
	if state.Refinements == nil {
		t.Error("Refinements map should be initialized")
	}
	if state.ConstructorFields == nil {
		t.Error("ConstructorFields map should be initialized")
	}
}

func TestEffectsEqual_BothNil(t *testing.T) {
	if !effectsEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
}

func TestEffectsEqual_OneNil(t *testing.T) {
	eff := &constraint.FunctionRefinement{}
	if effectsEqual(eff, nil) {
		t.Error("non-nil and nil should not be equal")
	}
	if effectsEqual(nil, eff) {
		t.Error("nil and non-nil should not be equal")
	}
}

func TestEffectsEqual_Same(t *testing.T) {
	eff := &constraint.FunctionRefinement{Terminates: true}
	if !effectsEqual(eff, eff) {
		t.Error("same reference should be equal")
	}
}

func TestEffectsMapEqual_Empty(t *testing.T) {
	if !effectsMapEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
	if !effectsMapEqual(map[cfg.SymbolID]*constraint.FunctionRefinement{}, map[cfg.SymbolID]*constraint.FunctionRefinement{}) {
		t.Error("two empty maps should be equal")
	}
}

func TestEffectsMapEqual_DifferentLength(t *testing.T) {
	a := map[cfg.SymbolID]*constraint.FunctionRefinement{1: {}}
	b := map[cfg.SymbolID]*constraint.FunctionRefinement{}
	if effectsMapEqual(a, b) {
		t.Error("maps of different length should not be equal")
	}
}

func TestInterprocFactsMapEqual_Empty(t *testing.T) {
	if !interprocFactsMapEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
	if !interprocFactsMapEqual(map[api.GraphKey]api.Facts{}, map[api.GraphKey]api.Facts{}) {
		t.Error("two empty maps should be equal")
	}
}

func TestInterprocFactsMapEqual_DifferentLength(t *testing.T) {
	a := map[api.GraphKey]api.Facts{{GraphID: 1}: {}}
	b := map[api.GraphKey]api.Facts{}
	if interprocFactsMapEqual(a, b) {
		t.Error("maps of different length should not be equal")
	}
}

func TestWidenInterprocFacts_Empty(t *testing.T) {
	result := widenInterprocFacts(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Error("expected empty map")
	}
}

func TestWidenInterprocFacts_OnlyPrev(t *testing.T) {
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.String}},
			},
		},
	}
	result := widenInterprocFacts(prev, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_OnlyNext(t *testing.T) {
	next := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.Number}},
			},
		},
	}
	result := widenInterprocFacts(nil, next)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_Merge(t *testing.T) {
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.String}},
			},
		},
	}
	next := map[api.GraphKey]api.Facts{
		{GraphID: 2}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.Number}},
			},
		},
	}
	result := widenInterprocFacts(prev, next)
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestReturnSummariesFromFacts_FallsBackToCanonical(t *testing.T) {
	facts := api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {
				Summary: []typ.Type{typ.String},
			},
		},
	}
	got := returns.SummaryViewFromFacts(facts)
	if len(got) != 1 || len(got[cfg.SymbolID(1)]) != 1 || got[cfg.SymbolID(1)][0] != typ.String {
		t.Fatalf("unexpected summary view: %#v", got)
	}
}

func TestNarrowReturnSummariesFromFacts_FallsBackToCanonical(t *testing.T) {
	facts := api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(2): {
				Narrow: []typ.Type{typ.Number},
			},
		},
	}
	got := returns.NarrowViewFromFacts(facts)
	if len(got) != 1 || len(got[cfg.SymbolID(2)]) != 1 || got[cfg.SymbolID(2)][0] != typ.Number {
		t.Fatalf("unexpected narrow view: %#v", got)
	}
}

func TestLocalFuncTypesFromFacts_FallsBackToCanonical(t *testing.T) {
	fn := typ.Func().Returns(typ.Boolean).Build()
	facts := api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(3): {
				Func: fn,
			},
		},
	}
	got := returns.FuncTypeViewFromFacts(facts)
	if len(got) != 1 || !typ.TypeEquals(got[cfg.SymbolID(3)], fn) {
		t.Fatalf("unexpected func type view: %#v", got)
	}
}

func TestSessionStore_Fields(t *testing.T) {
	s := &SessionStore{
		Module: &ModuleStore{
			Graphs: make(map[uint64]*cfg.Graph),
		},
		Iteration: &IterationStore{
			Revision: 5,
		},
	}
	if s.Module == nil {
		t.Error("Module should be set")
	}
	if s.Iteration.Revision != 5 {
		t.Error("Revision should be 5")
	}
}

func TestModuleStore_Fields(t *testing.T) {
	m := &ModuleStore{
		Graphs:        make(map[uint64]*cfg.Graph),
		Parents:       make(map[uint64]*scope.State),
		ModuleAliases: map[cfg.SymbolID]string{1: "test"},
	}
	if m.ModuleAliases[1] != "test" {
		t.Error("ModuleAliases not set correctly")
	}
}

func TestFunctionRegistry_Fields(t *testing.T) {
	r := &FunctionRegistry{
		BySym:     make(map[cfg.SymbolID]*api.FunctionRef),
		ByFunc:    make(map[*ast.FunctionExpr]*api.FunctionRef),
		ByGraphID: make(map[uint64]*api.FunctionRef),
	}
	if r.BySym == nil {
		t.Error("BySym should be initialized")
	}
}

func TestIterationStore_Fields(t *testing.T) {
	i := &IterationStore{Revision: 10}
	if i.Revision != 10 {
		t.Error("Revision not set")
	}
}

func TestIterationScratch_Fields(t *testing.T) {
	s := &IterationScratch{
		LiteralSigsByGraphID: make(map[uint64]map[*ast.FunctionExpr]*typ.Function),
	}
	if s.LiteralSigsByGraphID == nil {
		t.Error("LiteralSigsByGraphID should be initialized")
	}
}

func TestFixpointSwap_TracksChannelDiffsAndResetsNext(t *testing.T) {
	s := NewSessionStore()

	s.InterprocNext.Refinements[1] = &constraint.FunctionRefinement{Terminates: true}
	s.InterprocNext.Facts[api.GraphKey{GraphID: 7, ParentHash: 11}] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.String}},
		},
	}
	s.InterprocNext.ConstructorFields[3] = map[string]typ.Type{
		"v": typ.Number,
	}

	if !s.FixpointSwap() {
		t.Fatal("expected fixpoint swap to report changes")
	}

	diffs := s.FixpointChannelDiffs()
	if len(diffs) != 3 {
		t.Fatalf("expected 3 channel diffs, got %v", diffs)
	}
	if diffs[0] != "Refinements" || diffs[1] != "InterprocFacts" || diffs[2] != "ConstructorFields" {
		t.Fatalf("unexpected diff order/content: %v", diffs)
	}

	if len(s.InterprocPrev.Refinements) != 1 || s.InterprocPrev.Refinements[1] == nil {
		t.Fatalf("expected prev effects populated, got %#v", s.InterprocPrev.Refinements)
	}
	if len(s.InterprocNext.Refinements) != 0 {
		t.Fatalf("expected next effects reset, got %#v", s.InterprocNext.Refinements)
	}
	if len(s.InterprocPrev.Facts) != 1 {
		t.Fatalf("expected prev facts populated, got %#v", s.InterprocPrev.Facts)
	}
	if len(s.InterprocNext.Facts) != 0 {
		t.Fatalf("expected next facts reset, got %#v", s.InterprocNext.Facts)
	}
	if len(s.InterprocPrev.ConstructorFields) != 1 {
		t.Fatalf("expected prev constructor fields populated, got %#v", s.InterprocPrev.ConstructorFields)
	}
	if len(s.InterprocNext.ConstructorFields) != 0 {
		t.Fatalf("expected next constructor fields reset, got %#v", s.InterprocNext.ConstructorFields)
	}
}

func TestClearIterationChannels_InitializesMissingState(t *testing.T) {
	s := &SessionStore{}
	s.ClearIterationChannels()

	if s.Iteration == nil {
		t.Fatal("expected iteration store to be initialized")
	}
	if s.Scratch == nil {
		t.Fatal("expected scratch to be initialized")
	}
	if s.InterprocPrev == nil || s.InterprocNext == nil {
		t.Fatal("expected interproc states to be initialized")
	}
	if s.Scratch.LiteralSigsByGraphID == nil {
		t.Fatal("expected scratch literal signatures map to be initialized")
	}
}

func TestBumpRevision_InitializesIterationStore(t *testing.T) {
	s := &SessionStore{}
	s.BumpRevision()
	if got := s.Revision(); got != 1 {
		t.Fatalf("expected revision 1, got %d", got)
	}
}

func TestFixpointChannelDiffs_ReturnsCopy(t *testing.T) {
	s := NewSessionStore()
	s.StoreFunctionRefinement(1, &constraint.FunctionRefinement{Terminates: true})
	if !s.FixpointSwap() {
		t.Fatal("expected change from effect swap")
	}

	diffs := s.FixpointChannelDiffs()
	if len(diffs) == 0 {
		t.Fatal("expected non-empty diffs")
	}
	diffs[0] = "MUTATED"

	diffs2 := s.FixpointChannelDiffs()
	if len(diffs2) == 0 || diffs2[0] == "MUTATED" {
		t.Fatalf("expected defensive copy, got %v", diffs2)
	}
}

func TestClearIterationChannels_ResetsRevision(t *testing.T) {
	s := NewSessionStore()
	s.BumpRevision()
	s.BumpRevision()
	if got := s.Revision(); got != 2 {
		t.Fatalf("expected revision 2, got %d", got)
	}

	s.ClearIterationChannels()
	if got := s.Revision(); got != 0 {
		t.Fatalf("expected revision reset to 0, got %d", got)
	}
}

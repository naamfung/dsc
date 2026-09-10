package infer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestComputeReturnSummariesForGraph_Empty(t *testing.T) {
	inferencer := New(Config{})
	summaries, funcTypes, diags := inferencer.ComputeForGraph(RunContext{}, nil, nil)
	if summaries != nil {
		t.Error("nil graph should return nil summaries")
	}
	if funcTypes != nil {
		t.Error("nil graph should return nil function types")
	}
	if len(diags) != 0 {
		t.Error("nil graph should return no diagnostics")
	}
}

func TestSeedSummariesFromSeed_UsesKnownFunctionSymbolsOnly(t *testing.T) {
	localFuncs := map[cfg.SymbolID]*returns.LocalFuncInfo{
		1: nil,
		2: nil,
	}
	seed := map[cfg.SymbolID][]typ.Type{
		1: {typ.String},
		3: {typ.Number}, // not in local funcs; should be ignored
	}

	got := seedSummariesFromSeed(localFuncs, seed)
	if len(got) != 1 {
		t.Fatalf("expected one seeded summary, got %d", len(got))
	}
	if seeded := got[1]; len(seeded) != 1 || !typ.TypeEquals(seeded[0], typ.String) {
		t.Fatalf("unexpected seeded summary for symbol 1: %v", seeded)
	}
	if _, ok := got[3]; ok {
		t.Fatalf("unexpected seed for unknown symbol 3: %v", got[3])
	}
}

func TestSeedSummariesFromSeed_HandlesNilSeed(t *testing.T) {
	localFuncs := map[cfg.SymbolID]*returns.LocalFuncInfo{
		1: nil,
	}
	got := seedSummariesFromSeed(localFuncs, nil)
	if got == nil {
		t.Fatal("expected non-nil summary map")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty summary map, got %v", got)
	}
}

func TestUniformFunctionScopes_UsesBaseForAllPoints(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	base := scope.New()

	scopes := uniformFunctionScopes(graph, base)
	if scopes == nil {
		t.Fatal("expected non-nil scopes")
	}

	graph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		if got := scopes[p]; got != base {
			t.Fatalf("expected base scope at point %d, got %p want %p", p, got, base)
		}
	})
	if got := scopes[graph.Entry()]; got != base {
		t.Fatalf("expected base scope at entry, got %p want %p", got, base)
	}
}

func TestExtractMapComponentType_Union(t *testing.T) {
	union := typ.NewUnion(
		typ.NewMap(typ.String, typ.Number),
		typ.NewRecord().MapComponent(typ.String, typ.Integer).Build(),
	)
	key, value, ok := extractMapComponentType(union)
	if !ok {
		t.Fatal("expected map component from union")
	}
	if !typ.TypeEquals(key, typ.String) {
		t.Fatalf("expected key type string, got %v", key)
	}
	if !typ.TypeEquals(value, typ.Number) {
		t.Fatalf("expected merged value type number, got %v", value)
	}
}

func TestReconcileSoftAnnotatedInference_PrefersAnnotatedMapOverEmptyRecord(t *testing.T) {
	base := typ.NewMap(typ.String, typ.Number)
	inferred := typ.NewRecord().Build()

	got := reconcileSoftAnnotatedInference(base, inferred)
	if !typ.TypeEquals(got, base) {
		t.Fatalf("expected base annotated map preserved, got %v", got)
	}
}

func TestReconcileSoftAnnotatedInference_RecordTemplateKeepsFields(t *testing.T) {
	base := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	inferred := typ.NewMap(typ.String, typ.Integer)

	got := reconcileSoftAnnotatedInference(base, inferred)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", got)
	}
	if !rec.HasMapComponent() {
		t.Fatal("expected map component on merged record")
	}
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("expected map key string, got %v", rec.MapKey)
	}
	if !typ.TypeEquals(rec.MapValue, typ.Number) {
		t.Fatalf("expected map value number, got %v", rec.MapValue)
	}
	foundName := false
	for _, f := range rec.Fields {
		if f.Name == "name" && typ.TypeEquals(f.Type, typ.String) {
			foundName = true
			break
		}
	}
	if !foundName {
		t.Fatalf("expected 'name: string' field preserved, got %+v", rec.Fields)
	}
}

func TestCollectAllReturnSummaries_NormalizesAndFilters(t *testing.T) {
	inferencer := New(Config{})
	ctx := &returnInferenceContext{
		summaries: map[cfg.SymbolID][]typ.Type{
			0: {typ.String}, // invalid symbol id, ignored
			1: nil,          // empty summary, ignored
			2: {nil, typ.String},
		},
	}

	got := inferencer.collectAllReturnSummaries(ctx)
	if len(got) != 1 {
		t.Fatalf("expected one normalized summary, got %d (%v)", len(got), got)
	}
	summary := got[2]
	if len(summary) != 2 {
		t.Fatalf("expected 2-slot summary, got %v", summary)
	}
	if !typ.TypeEquals(summary[0], typ.Nil) {
		t.Fatalf("expected first slot normalized to nil, got %v", summary[0])
	}
	if !typ.TypeEquals(summary[1], typ.String) {
		t.Fatalf("expected second slot string, got %v", summary[1])
	}
}

func TestResolveLocalFunctionSummary_UsesCurrentSummaryWithoutStore(t *testing.T) {
	inferencer := New(Config{})

	got := inferencer.resolveLocalFunctionSummary(nil, map[cfg.SymbolID][]typ.Type{
		1: {typ.String},
	}, 1)
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("expected string summary, got %v", got)
	}

	unknownOnly := inferencer.resolveLocalFunctionSummary(nil, map[cfg.SymbolID][]typ.Type{
		1: {typ.Unknown},
	}, 1)
	if len(unknownOnly) != 1 || !typ.TypeEquals(unknownOnly[0], typ.Unknown) {
		t.Fatalf("expected unknown summary without store fallback, got %v", unknownOnly)
	}

	if got := inferencer.resolveLocalFunctionSummary(nil, nil, 0); got != nil {
		t.Fatalf("expected nil summary for symbol 0, got %v", got)
	}
}

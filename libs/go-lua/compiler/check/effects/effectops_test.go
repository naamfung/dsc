package effects

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPropagate_NilResult(t *testing.T) {
	result := Propagate(nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil result, got %v", result)
	}
}

func TestPropagate_EmptyResult(t *testing.T) {
	result := Propagate(&api.FuncResult{}, nil)
	if result == nil {
		t.Fatal("expected non-nil effect for empty result")
	}
}

func TestPropagate_WithLocalEffect(t *testing.T) {
	fnEffect := &constraint.FunctionRefinement{
		Terminates: true,
	}
	result := Propagate(&api.FuncResult{FnRefinement: fnEffect}, nil)
	if result == nil {
		t.Fatal("expected non-nil effect")
	}
	if !result.Terminates {
		t.Error("expected Terminates to be true")
	}
}

func TestLookupRefinementBySym_NilStore(t *testing.T) {
	result := LookupRefinementBySym(nil, nil, nil, 1)
	if result != nil {
		t.Errorf("expected nil for nil store, got %v", result)
	}
}

func TestLookupRefinementBySym_ZeroSym(t *testing.T) {
	result := LookupRefinementBySym(nil, nil, nil, 0)
	if result != nil {
		t.Errorf("expected nil for zero symbol, got %v", result)
	}
}

func TestTerminatesFromReachability_NilResult(t *testing.T) {
	if TerminatesFromReachability(nil) {
		t.Error("expected false for nil result")
	}
}

func TestTerminatesFromReachability_NilFlowSolution(t *testing.T) {
	if TerminatesFromReachability(&api.FuncResult{}) {
		t.Error("expected false for nil flow solution")
	}
}

func TestEffectFromType_Nil(t *testing.T) {
	if EffectFromType(nil) != nil {
		t.Error("expected nil for nil type")
	}
}

func TestEffectFromType_NonFunction(t *testing.T) {
	if EffectFromType(typ.String) != nil {
		t.Error("expected nil for non-function type")
	}
}

func TestEffectFromType_PureFunction(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	result := EffectFromType(fn)
	if result != nil {
		t.Errorf("expected nil for pure function, got %v", result)
	}
}

func TestEffectFromType_WithEffects(t *testing.T) {
	row := effect.Empty.With(effect.IO{})
	fn := typ.Func().Returns(typ.String).Effects(row).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect for function with effects")
	}
}

func TestEffectFromType_NeverReturn(t *testing.T) {
	fn := typ.Func().Returns(typ.Never).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect for never-returning function")
	}
	if !result.Terminates {
		t.Error("expected Terminates to be true for never-returning function")
	}
}

func TestEffectFromType_WithRefinement(t *testing.T) {
	eff := &constraint.FunctionRefinement{Terminates: true}
	fn := typ.Func().Returns(typ.String).WithRefinement(eff).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect")
	}
	if !result.Terminates {
		t.Error("expected refinement to be returned")
	}
}

func TestEnrichExportWithEffects_NilExport(t *testing.T) {
	result := EnrichExportWithEffects(nil, "", nil, nil)
	if result != nil {
		t.Error("expected nil for nil export")
	}
}

func TestEnrichExportWithEffects_NilGraph(t *testing.T) {
	rec := typ.NewRecord().Build()
	result := EnrichExportWithEffects(rec, "", nil, nil)
	if result != rec {
		t.Error("expected original record returned when graph is nil")
	}
}

func TestEnrichExportWithEffects_EmptyEffects(t *testing.T) {
	rec := typ.NewRecord().Build()
	result := EnrichExportWithEffects(rec, "", map[cfg.SymbolID]*constraint.FunctionRefinement{}, nil)
	if result != rec {
		t.Error("expected original record returned when effects map is empty")
	}
}

func TestEnrichExportWithEffects_NonRecordNonInterface(t *testing.T) {
	result := EnrichExportWithEffects(typ.String, "", map[cfg.SymbolID]*constraint.FunctionRefinement{1: {}}, nil)
	if result != typ.String {
		t.Error("expected original type returned for non-record/non-interface")
	}
}

func TestExportFieldNameFromEffectSymbol(t *testing.T) {
	tests := []struct {
		name     string
		rootName string
		symbol   string
		want     string
		wantOK   bool
	}{
		{name: "direct no root", rootName: "", symbol: "validate", want: "validate", wantOK: true},
		{name: "single dot no root", rootName: "", symbol: "M.validate", want: "validate", wantOK: true},
		{name: "nested no root rejected", rootName: "", symbol: "M.api.validate", want: "", wantOK: false},
		{name: "root direct", rootName: "M", symbol: "M.validate", want: "validate", wantOK: true},
		{name: "root nested rejected", rootName: "M", symbol: "M.api.validate", want: "", wantOK: false},
		{name: "root mismatch rejected", rootName: "M", symbol: "N.validate", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := exportFieldNameFromEffectSymbol(tt.rootName, tt.symbol)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("field=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnrichExportWithEffects_PreservesRecordQualifiersAndMetatable(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local function validate(v)
			return v
		end
		return validate
	`, "validate")

	symValidate, ok := graph.SymbolAt(graph.Entry(), "validate")
	if !ok || symValidate == 0 {
		t.Fatal("expected symbol for validate")
	}

	fn := typ.Func().Param("v", typ.String).Returns(typ.Boolean).Build()
	meta := typ.NewRecord().Field("__index", typ.Any).Build()
	export := typ.NewRecord().
		OptReadonlyField("validate", fn).
		Field("plain", typ.Number).
		MapComponent(typ.String, typ.Integer).
		SetOpen(true).
		Metatable(meta).
		Build()

	effectsBySym := map[cfg.SymbolID]*constraint.FunctionRefinement{
		symValidate: {Terminates: true},
	}

	enriched := EnrichExportWithEffects(export, "", effectsBySym, graph)
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", enriched)
	}

	field := rec.GetField("validate")
	if field == nil {
		t.Fatal("expected validate field")
	}
	if !field.Optional {
		t.Fatal("validate field optional flag should be preserved")
	}
	if !field.Readonly {
		t.Fatal("validate field readonly flag should be preserved")
	}
	fnType, ok := field.Type.(*typ.Function)
	if !ok {
		t.Fatalf("validate field should be function, got %T", field.Type)
	}
	if fnType.Refinement == nil {
		t.Fatal("validate function should be enriched with refinement")
	}

	plain := rec.GetField("plain")
	if plain == nil || plain.Optional || plain.Readonly {
		t.Fatalf("plain field flags should remain unchanged: %+v", plain)
	}
	if !rec.HasMapComponent() || !typ.TypeEquals(rec.MapKey, typ.String) || !typ.TypeEquals(rec.MapValue, typ.Integer) {
		t.Fatalf("map component should be preserved, got key=%v value=%v", rec.MapKey, rec.MapValue)
	}
	if !rec.Open {
		t.Fatal("record open flag should be preserved")
	}
	if !typ.TypeEquals(rec.Metatable, meta) {
		t.Fatalf("metatable should be preserved, got %v want %v", rec.Metatable, meta)
	}
}

func TestPropagate_CollectsEffectFromAssignmentCallSite(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local x = f()
		return x
	`, "f")

	symF, ok := graph.SymbolAt(graph.Entry(), "f")
	if !ok || symF == 0 {
		t.Fatal("expected symbol for f")
	}

	result := Propagate(&api.FuncResult{
		Graph:        graph,
		FnRefinement: &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == symF {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect from assignment call site, got %#v", result.Row)
	}
}

func TestPropagate_CollectsEffectFromReturnCallSite(t *testing.T) {
	graph := buildGraphForEffects(t, `
		return f()
	`, "f")

	symF, ok := graph.SymbolAt(graph.Entry(), "f")
	if !ok || symF == 0 {
		t.Fatal("expected symbol for f")
	}

	result := Propagate(&api.FuncResult{
		Graph:        graph,
		FnRefinement: &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == symF {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect from return call site, got %#v", result.Row)
	}
}

func TestPropagate_UsesCanonicalCandidatesWhenRawSymbolMissing(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local x = f()
		return x
	`, "f")

	symF, ok := graph.SymbolAt(graph.Entry(), "f")
	if !ok || symF == 0 {
		t.Fatal("expected symbol for f")
	}

	// Simulate missing raw call symbol. Propagation should still resolve
	// callee via canonical callsite candidates from call expression/bindings.
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			info.CalleeSymbol = 0
		}
	})

	result := Propagate(&api.FuncResult{
		Graph:        graph,
		FnRefinement: &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == symF {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect via canonical candidate lookup, got %#v", result.Row)
	}
}

func TestPropagate_UsesModuleBindingNameFallback(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local x = f()
		return x
	`)

	const fallbackSym cfg.SymbolID = 777
	moduleBindings := bind.NewBindingTable()
	moduleBindings.SetName(fallbackSym, "f_alias")

	// Force callsite identity recovery through module-binding name fallback.
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			info.Callee = nil
			info.CalleeSymbol = 0
			info.CalleeName = "f_alias"
		}
	})

	result := Propagate(&api.FuncResult{
		Graph:          graph,
		ModuleBindings: moduleBindings,
		FnRefinement:   &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == fallbackSym {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect via module-binding name fallback, got %#v", result.Row)
	}
}

func buildGraphForEffects(t *testing.T, code string, globals ...string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, globals...)
	if graph == nil {
		t.Fatal("expected graph")
	}
	return graph
}

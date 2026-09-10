package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeFunctionFactIntoFacts_InitialWrite(t *testing.T) {
	facts := &api.Facts{}
	sym := cfg.SymbolID(11)
	fn := typ.Func().Returns(typ.String).Build()

	MergeFunctionFactIntoFacts(facts, sym, FunctionFactCandidate{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    fn,
	})

	if got := facts.ReturnSummaries[sym]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.NarrowReturns[sym]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FuncTypes[sym]; !typ.TypeEquals(got, fn) {
		t.Fatalf("func mismatch: got %v", got)
	}
}

func TestMergeFunctionFactIntoFacts_MatchesKernelReconcile(t *testing.T) {
	sym := cfg.SymbolID(17)
	existingFn := typ.Func().Returns(typ.Number).Build()
	candidateFn := typ.Func().Returns(typ.String).Build()
	facts := &api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Summary: []typ.Type{typ.Number},
				Narrow:  []typ.Type{typ.Number},
				Func:    existingFn,
			},
		},
		ReturnSummaries: api.ReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		FuncTypes: api.FuncTypes{
			sym: existingFn,
		},
	}
	candidate := FunctionFactCandidate{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    candidateFn,
	}
	existing := readFunctionFactFromFacts(facts, sym)
	expected := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  existing.Summary,
		ExistingNarrow:   existing.Narrow,
		ExistingFunc:     existing.Func,
		CandidateSummary: candidate.Summary,
		CandidateNarrow:  candidate.Narrow,
		CandidateFunc:    candidate.Func,
	})

	MergeFunctionFactIntoFacts(facts, sym, candidate)

	if got := facts.ReturnSummaries[sym]; !ReturnTypesEqual(got, expected.Summary) {
		t.Fatalf("summary mismatch: got %v want %v", got, expected.Summary)
	}
	if got := facts.NarrowReturns[sym]; !ReturnTypesEqual(got, expected.Narrow) {
		t.Fatalf("narrow mismatch: got %v want %v", got, expected.Narrow)
	}
	if got := facts.FuncTypes[sym]; !typ.TypeEquals(got, expected.Func) {
		t.Fatalf("func mismatch: got %v want %v", got, expected.Func)
	}
}

func TestMergeFunctionFactsIntoFacts_BatchMerge(t *testing.T) {
	symSummary := cfg.SymbolID(21)
	symNarrow := cfg.SymbolID(22)
	symFunc := cfg.SymbolID(23)
	facts := &api.Facts{}
	funcType := typ.Func().Returns(typ.Boolean).Build()

	MergeFunctionFactsIntoFacts(
		facts,
		api.ReturnSummaries{
			symSummary: []typ.Type{typ.String},
		},
		api.NarrowReturnSummaries{
			symNarrow: []typ.Type{typ.Number},
		},
		api.FuncTypes{
			symFunc: funcType,
		},
	)

	if got := facts.ReturnSummaries[symSummary]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.NarrowReturns[symNarrow]; !ReturnTypesEqual(got, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FuncTypes[symFunc]; !typ.TypeEquals(got, funcType) {
		t.Fatalf("func mismatch: got %v", got)
	}
}

func TestReconcileFunctionFact_NarrowSummaryReplacesOpenTopPlaceholder(t *testing.T) {
	openTop := typ.NewRecord().SetOpen(true).Build()
	existingFunc := typ.Func().Returns(openTop).Build()
	candidateFunc := typ.Func().Returns(openTop).Build()
	narrow := []typ.Type{typ.NewArray(typ.Unknown)}

	out := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  []typ.Type{openTop},
		ExistingNarrow:   nil,
		ExistingFunc:     existingFunc,
		CandidateSummary: []typ.Type{openTop},
		CandidateNarrow:  narrow,
		CandidateFunc:    candidateFunc,
	})

	if !ReturnTypesEqual(normalizeAndPruneReturnVector(out.Summary), normalizeAndPruneReturnVector(narrow)) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, narrow)
	}

	fn, ok := out.Func.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Func)
	}
	if !ReturnTypesEqual(normalizeAndPruneReturnVector(fn.Returns), normalizeAndPruneReturnVector(narrow)) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, narrow)
	}
}

func TestReconcileFunctionFact_NarrowSummaryRepairsNeverArtifact(t *testing.T) {
	bad := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}
	good := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Unknown).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}
	existingFunc := typ.Func().Returns(bad...).Build()

	out := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary: bad,
		ExistingNarrow:  nil,
		ExistingFunc:    existingFunc,
		CandidateNarrow: good,
	})

	if !ReturnTypesEqual(out.Summary, good) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, good)
	}
	if !ReturnTypesEqual(out.Narrow, good) {
		t.Fatalf("narrow mismatch: got %v want %v", out.Narrow, good)
	}
	fn, ok := out.Func.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Func)
	}
	if !ReturnTypesEqual(fn.Returns, good) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, good)
	}
}

func TestMergeFunctionFactIntoFacts_ReadsLegacyAndWritesCanonical(t *testing.T) {
	sym := cfg.SymbolID(41)
	facts := &api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		FuncTypes: api.FuncTypes{
			sym: typ.Func().Returns(typ.Number).Build(),
		},
	}

	MergeFunctionFactIntoFacts(facts, sym, FunctionFactCandidate{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    typ.Func().Returns(typ.String).Build(),
	})

	ff, ok := facts.FunctionFacts[sym]
	if !ok {
		t.Fatal("expected canonical FunctionFacts entry")
	}
	if !ReturnTypesEqual(ff.Summary, facts.ReturnSummaries[sym]) {
		t.Fatalf("summary drift: canonical=%v legacy=%v", ff.Summary, facts.ReturnSummaries[sym])
	}
	if !ReturnTypesEqual(ff.Narrow, facts.NarrowReturns[sym]) {
		t.Fatalf("narrow drift: canonical=%v legacy=%v", ff.Narrow, facts.NarrowReturns[sym])
	}
	if !typ.TypeEquals(ff.Func, facts.FuncTypes[sym]) {
		t.Fatalf("func drift: canonical=%v legacy=%v", ff.Func, facts.FuncTypes[sym])
	}
}

func TestNormalizeFunctionFactChannels_PromotesLegacyIntoCanonical(t *testing.T) {
	sym := cfg.SymbolID(77)
	fn := typ.Func().Returns(typ.Number).Build()
	facts := &api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		FuncTypes: api.FuncTypes{
			sym: fn,
		},
	}

	NormalizeFunctionFactChannels(facts)

	ff, ok := facts.FunctionFacts[sym]
	if !ok {
		t.Fatal("expected canonical FunctionFacts entry from legacy channels")
	}
	if !ReturnTypesEqual(ff.Summary, facts.ReturnSummaries[sym]) {
		t.Fatalf("summary drift: canonical=%v legacy=%v", ff.Summary, facts.ReturnSummaries[sym])
	}
	if !ReturnTypesEqual(ff.Narrow, facts.NarrowReturns[sym]) {
		t.Fatalf("narrow drift: canonical=%v legacy=%v", ff.Narrow, facts.NarrowReturns[sym])
	}
	if !typ.TypeEquals(ff.Func, facts.FuncTypes[sym]) {
		t.Fatalf("func drift: canonical=%v legacy=%v", ff.Func, facts.FuncTypes[sym])
	}
}

func TestFunctionFactViews_UseLegacyChannelsWhenCanonicalMissing(t *testing.T) {
	sym := cfg.SymbolID(88)
	fn := typ.Func().Returns(typ.String).Build()
	facts := api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			sym: []typ.Type{typ.String},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			sym: []typ.Type{typ.String},
		},
		FuncTypes: api.FuncTypes{
			sym: fn,
		},
	}

	summaries := SummaryViewFromFacts(facts)
	if got := summaries[sym]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary view mismatch: got %v", got)
	}

	narrows := NarrowViewFromFacts(facts)
	if got := narrows[sym]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow view mismatch: got %v", got)
	}

	funcs := FuncTypeViewFromFacts(facts)
	if got := funcs[sym]; !typ.TypeEquals(got, fn) {
		t.Fatalf("func view mismatch: got %v", got)
	}
}

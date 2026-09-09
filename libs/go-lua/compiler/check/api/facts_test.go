package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFacts_Zero(t *testing.T) {
	f := Facts{}
	if f.FunctionFacts != nil {
		t.Error("zero Facts should have nil FunctionFacts")
	}
	if f.ReturnSummaries != nil {
		t.Error("zero Facts should have nil ReturnSummaries")
	}
	if f.NarrowReturns != nil {
		t.Error("zero Facts should have nil NarrowReturns")
	}
	if f.ParamHints != nil {
		t.Error("zero Facts should have nil ParamHints")
	}
	if f.FuncTypes != nil {
		t.Error("zero Facts should have nil FuncTypes")
	}
	if f.LiteralSigs != nil {
		t.Error("zero Facts should have nil LiteralSigs")
	}
	if f.CapturedTypes != nil {
		t.Error("zero Facts should have nil CapturedTypes")
	}
	if f.CapturedFields != nil {
		t.Error("zero Facts should have nil CapturedFields")
	}
	if f.CapturedContainers != nil {
		t.Error("zero Facts should have nil CapturedContainers")
	}
}

func TestReturnSummaries_Basic(t *testing.T) {
	summaries := make(ReturnSummaries)
	sym := cfg.SymbolID(1)
	summaries[sym] = []typ.Type{typ.String, typ.Nil}

	rets, ok := summaries[sym]
	if !ok {
		t.Fatal("expected symbol to be in summaries")
	}
	if len(rets) != 2 {
		t.Errorf("expected 2 return types, got %d", len(rets))
	}
}

func TestParamHints_Basic(t *testing.T) {
	hints := make(ParamHints)
	sym := cfg.SymbolID(1)
	hints[sym] = []typ.Type{typ.Number, typ.String}

	params, ok := hints[sym]
	if !ok {
		t.Fatal("expected symbol to be in hints")
	}
	if len(params) != 2 {
		t.Errorf("expected 2 param hints, got %d", len(params))
	}
}

func TestFuncTypes_Basic(t *testing.T) {
	funcTypes := make(FuncTypes)
	sym := cfg.SymbolID(1)
	fn := typ.Func().Param("x", typ.Number).Returns(typ.String).Build()
	funcTypes[sym] = fn

	retrieved, ok := funcTypes[sym]
	if !ok {
		t.Fatal("expected symbol to be in funcTypes")
	}
	if retrieved == nil {
		t.Error("expected non-nil function type")
	}
}

func TestCapturedTypes_Basic(t *testing.T) {
	captured := make(CapturedTypes)
	sym := cfg.SymbolID(1)
	captured[sym] = typ.String

	retrieved, ok := captured[sym]
	if !ok {
		t.Fatal("expected symbol to be in captured")
	}
	if retrieved != typ.String {
		t.Error("expected string type")
	}
}

func TestCapturedFieldAssigns_Basic(t *testing.T) {
	assigns := make(CapturedFieldAssigns)
	nestedSym := cfg.SymbolID(1)
	capturedSym := cfg.SymbolID(2)

	assigns[nestedSym] = map[cfg.SymbolID]map[string]typ.Type{
		capturedSym: {
			"foo": typ.String,
			"bar": typ.Number,
		},
	}

	nestedAssigns, ok := assigns[nestedSym]
	if !ok {
		t.Fatal("expected nested symbol in assigns")
	}
	fields, ok := nestedAssigns[capturedSym]
	if !ok {
		t.Fatal("expected captured symbol in nested assigns")
	}
	if len(fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(fields))
	}
	if fields["foo"] != typ.String {
		t.Error("expected foo to be string")
	}
}

func TestCapturedContainerMutations_Basic(t *testing.T) {
	mutations := make(CapturedContainerMutations)
	nestedSym := cfg.SymbolID(1)
	capturedSym := cfg.SymbolID(2)

	mutations[nestedSym] = map[cfg.SymbolID][]ContainerMutation{
		capturedSym: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "ch"}},
				ValueType: typ.Number,
			},
		},
	}

	nestedMutations, ok := mutations[nestedSym]
	if !ok {
		t.Fatal("expected nested symbol in mutations")
	}
	list, ok := nestedMutations[capturedSym]
	if !ok {
		t.Fatal("expected captured symbol in nested mutations")
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(list))
	}
	if list[0].ValueType != typ.Number {
		t.Error("expected value type to be number")
	}
}

func TestContainerMutationKey(t *testing.T) {
	m := ContainerMutation{
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "queue"},
			{Kind: constraint.SegmentIndexString, Name: "jobs"},
			{Kind: constraint.SegmentIndexInt, Index: 2},
		},
		ValueType: typ.String,
	}
	if got, want := ContainerMutationKey(m), ".queue[\"jobs\"][2]"; got != want {
		t.Fatalf("ContainerMutationKey() = %q, want %q", got, want)
	}
}

func TestFacts_WithData(t *testing.T) {
	f := Facts{
		FunctionFacts: FunctionFacts{
			4: {
				Summary: []typ.Type{typ.Boolean},
				Narrow:  []typ.Type{typ.Boolean},
				Func:    typ.Func().Returns(typ.Boolean).Build(),
			},
		},
		ReturnSummaries: ReturnSummaries{
			1: []typ.Type{typ.String},
		},
		ParamHints: ParamHints{
			2: []typ.Type{typ.Number},
		},
		FuncTypes: FuncTypes{
			3: typ.Func().Build(),
		},
	}

	if len(f.ReturnSummaries) != 1 {
		t.Error("expected 1 return summary")
	}
	if len(f.FunctionFacts) != 1 {
		t.Error("expected 1 function fact")
	}
	if len(f.ParamHints) != 1 {
		t.Error("expected 1 param hint")
	}
	if len(f.FuncTypes) != 1 {
		t.Error("expected 1 func type")
	}
}

package siblings

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuild_Empty(t *testing.T) {
	conf := BuildConfig{}
	result := Build(conf)
	if result != nil {
		t.Error("empty config should return nil")
	}
}

func TestBuild_WithFuncs(t *testing.T) {
	fnType := typ.Func().Returns(typ.String).Build()
	conf := BuildConfig{
		Funcs: []FuncEntry{
			{Symbol: 1, IsLocal: true},
		},
		FuncTypes: map[cfg.SymbolID]typ.Type{1: fnType},
	}
	result := Build(conf)
	if result == nil {
		t.Fatal("should return result")
	}
	if result[1] == nil {
		t.Error("should include function type")
	}
}

func TestBuild_WithPrev(t *testing.T) {
	conf := BuildConfig{
		Funcs: []FuncEntry{
			{Symbol: 1, IsLocal: true},
		},
		FuncTypes: map[cfg.SymbolID]typ.Type{1: typ.Func().Build()},
		SiblingTypesPrev: map[cfg.SymbolID]typ.Type{
			2: typ.String,
		},
	}
	result := Build(conf)
	if result[2] != typ.String {
		t.Error("should include prev types")
	}
}

func TestMergeSiblingType_BothNilViaBuildAPI(t *testing.T) {
	result := returns.MergeFunctionFactType(nil, nil)
	if result != nil {
		t.Error("both nil should return nil")
	}
}

func TestMergeSiblingType_PrevNilViaBuildAPI(t *testing.T) {
	result := returns.MergeFunctionFactType(nil, typ.String)
	if result != typ.String {
		t.Error("prev nil should return next")
	}
}

func TestMergeSiblingType_NextNilViaBuildAPI(t *testing.T) {
	result := returns.MergeFunctionFactType(typ.String, nil)
	if result != typ.String {
		t.Error("next nil should return prev")
	}
}

func TestMergeSiblingType_FunctionsViaBuildAPI(t *testing.T) {
	prevFn := typ.Func().Build()
	nextFn := typ.Func().Returns(typ.String).Build()
	result := returns.MergeFunctionFactType(prevFn, nextFn)
	if result == nil {
		t.Fatal("should return merged function")
	}
	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatal("should be function type")
	}
	if len(fn.Returns) == 0 {
		t.Error("should prefer function with returns")
	}
}

func TestMergeSiblingType_FunctionAliasesViaBuildAPI(t *testing.T) {
	prevFn := typ.NewAlias("Prev", typ.Func().Build())
	nextFn := typ.NewAlias("Next", typ.Func().Returns(typ.String).Build())
	result := returns.MergeFunctionFactType(prevFn, nextFn)
	if !typ.TypeEquals(result, nextFn) {
		t.Fatalf("expected function alias with returns to be preferred, got %v", result)
	}
}

func TestCompute_Nil(t *testing.T) {
	result := Compute(nil, 0)
	if result != nil {
		t.Error("nil store should return nil")
	}
}

func TestCompute_Found(t *testing.T) {
	store := map[uint64]map[cfg.SymbolID]typ.Type{
		1: {1: typ.String},
	}
	result := Compute(store, 1)
	if result == nil {
		t.Error("should find stored siblings")
	}
}

func TestCompute_NotFound(t *testing.T) {
	store := map[uint64]map[cfg.SymbolID]typ.Type{
		1: {1: typ.String},
	}
	result := Compute(store, 2)
	if result != nil {
		t.Error("should return nil for missing hash")
	}
}

func TestCopy_Nil(t *testing.T) {
	result := Copy(nil)
	if result != nil {
		t.Error("copy of nil should be nil")
	}
}

func TestCopy(t *testing.T) {
	original := map[cfg.SymbolID]typ.Type{1: typ.String}
	copied := Copy(original)
	if copied == nil {
		t.Fatal("copy should not be nil")
	}
	if copied[1] != typ.String {
		t.Error("copy should have same values")
	}
	copied[2] = typ.Number
	if original[2] != nil {
		t.Error("modifying copy should not affect original")
	}
}

func TestFuncEntry(t *testing.T) {
	entry := FuncEntry{
		Symbol:  1,
		Point:   10,
		IsLocal: true,
	}
	if entry.Symbol != 1 {
		t.Error("Symbol should be set")
	}
	if entry.Point != 10 {
		t.Error("Point should be set")
	}
}

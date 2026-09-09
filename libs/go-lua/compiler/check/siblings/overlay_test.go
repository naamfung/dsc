package siblings

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildOverlay_Empty(t *testing.T) {
	conf := OverlayConfig{}
	result := BuildOverlay(conf)
	if len(result) != 0 {
		t.Error("empty config should return empty overlay")
	}
}

func TestBuildOverlay_WithSummaries(t *testing.T) {
	conf := OverlayConfig{
		Summaries: map[cfg.SymbolID][]typ.Type{
			1: {typ.String},
			2: {typ.Number},
		},
		CurrentSym: 1,
	}
	result := BuildOverlay(conf)
	if result[1] != nil {
		t.Error("current symbol should be excluded")
	}
	if result[2] == nil {
		t.Error("other symbols should be included")
	}
}

func TestBuildOverlay_ExcludesCurrent(t *testing.T) {
	conf := OverlayConfig{
		Summaries: map[cfg.SymbolID][]typ.Type{
			1: {typ.String},
		},
		CurrentSym: 1,
	}
	result := BuildOverlay(conf)
	if _, exists := result[cfg.SymbolID(1)]; exists {
		t.Error("current symbol should not be in overlay")
	}
}

func TestOverlayEntry(t *testing.T) {
	entry := OverlayEntry{
		Symbol: 1,
		Func:   nil,
	}
	if entry.Symbol != 1 {
		t.Error("Symbol should be set")
	}
}

func TestBuildOverlay_SeedsSiblingsWithoutSummaries(t *testing.T) {
	seedType := typ.Func().Param("x", typ.Number).Build()
	fn := &ast.FunctionExpr{}
	conf := OverlayConfig{
		Siblings: []OverlayEntry{
			{Symbol: 1, Func: fn},
		},
		CurrentSym: 999,
		Services: OverlayServicesFuncs{
			SeedTypeFn: func(f *ast.FunctionExpr) typ.Type {
				return seedType
			},
		},
	}
	result := BuildOverlay(conf)
	if result[1] != seedType {
		t.Error("should seed sibling without summary using SeedType")
	}
}

func TestBuildOverlay_SummaryOverridesSeed(t *testing.T) {
	fn := &ast.FunctionExpr{}
	conf := OverlayConfig{
		Summaries: map[cfg.SymbolID][]typ.Type{
			1: {typ.String},
		},
		Siblings: []OverlayEntry{
			{Symbol: 1, Func: fn},
		},
		CurrentSym: 999,
		Services: OverlayServicesFuncs{
			SeedTypeFn: func(f *ast.FunctionExpr) typ.Type {
				return typ.Func().Returns(typ.Number).Build()
			},
		},
	}
	result := BuildOverlay(conf)
	fn2, ok := result[1].(*typ.Function)
	if !ok {
		t.Fatal("should produce function type")
	}
	if len(fn2.Returns) == 0 || fn2.Returns[0] != typ.String {
		t.Error("summary should take precedence")
	}
}

func TestBuildOverlay_NilSeedType(t *testing.T) {
	fn := &ast.FunctionExpr{}
	conf := OverlayConfig{
		Siblings: []OverlayEntry{
			{Symbol: 1, Func: fn},
		},
		CurrentSym: 999,
		Services:   nil,
	}
	result := BuildOverlay(conf)
	if result[1] != nil {
		t.Error("nil SeedType should skip seeding")
	}
}

func TestBuildOverlay_NilFunc(t *testing.T) {
	conf := OverlayConfig{
		Siblings: []OverlayEntry{
			{Symbol: 1, Func: nil},
		},
		CurrentSym: 999,
		Services: OverlayServicesFuncs{
			SeedTypeFn: func(f *ast.FunctionExpr) typ.Type {
				return typ.Func().Build()
			},
		},
	}
	result := BuildOverlay(conf)
	if result[1] != nil {
		t.Error("nil Func should skip seeding")
	}
}

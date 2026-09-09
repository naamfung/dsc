package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestResolveCalleeEffect_PrefersLookup(t *testing.T) {
	info := &cfg.CallInfo{
		CalleeSymbol: 7,
		Callee:       &ast.IdentExpr{Value: "f"},
	}
	lookupEff := &constraint.FunctionRefinement{Terminates: true}
	typeEff := &constraint.FunctionRefinement{OnReturn: constraint.TrueCondition()}

	got := ResolveCalleeEffect(
		info,
		0,
		nil,
		nil,
		nil,
		func(sym cfg.SymbolID) *constraint.FunctionRefinement {
			if sym == 7 {
				return lookupEff
			}
			return nil
		},
		func(ast.Expr, cfg.Point) typ.Type {
			return typ.Func().WithRefinement(typeEff).Build()
		},
		nil,
		func(t typ.Type) *constraint.FunctionRefinement {
			fn := unwrap.Function(t)
			if fn != nil {
				if eff, ok := fn.Refinement.(*constraint.FunctionRefinement); ok {
					return eff
				}
			}
			return nil
		},
	)
	if got != lookupEff {
		t.Fatalf("expected lookup effect, got %v", got)
	}
}

func TestResolveCalleeEffect_FallsBackToSymbolTypeWhenSynthNoEffect(t *testing.T) {
	info := &cfg.CallInfo{
		CalleeSymbol: 3,
		Callee:       &ast.IdentExpr{Value: "g"},
	}
	want := &constraint.FunctionRefinement{Terminates: true}

	got := ResolveCalleeEffect(
		info,
		42,
		nil,
		nil,
		nil,
		nil,
		func(ast.Expr, cfg.Point) typ.Type {
			return typ.Func().Returns(typ.String).Build()
		},
		func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
			if p == 42 && sym == 3 {
				return typ.Func().WithRefinement(want).Build(), true
			}
			return nil, false
		},
		func(t typ.Type) *constraint.FunctionRefinement {
			fn, ok := t.(*typ.Function)
			if !ok || fn == nil {
				return nil
			}
			eff, _ := fn.Refinement.(*constraint.FunctionRefinement)
			return eff
		},
	)
	if got != want {
		t.Fatalf("expected symbol-resolved effect, got %v", got)
	}
}

func TestResolveCalleeType_PrefersSynthThenSymbolFallback(t *testing.T) {
	info := &cfg.CallInfo{
		CalleeSymbol: 9,
		Callee:       &ast.IdentExpr{Value: "h"},
	}

	synthType := typ.Func().Returns(typ.String).Build()
	got := ResolveCalleeType(
		info,
		0,
		nil,
		nil,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return synthType },
		func(cfg.Point, cfg.SymbolID) (typ.Type, bool) { return typ.Number, true },
	)
	if !typ.TypeEquals(got, synthType) {
		t.Fatalf("expected synth type, got %v", got)
	}

	fallbackType := typ.Func().Returns(typ.Number).Build()
	got = ResolveCalleeType(
		info,
		0,
		nil,
		nil,
		nil,
		nil,
		func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
			if sym == 9 {
				return fallbackType, true
			}
			return nil, false
		},
	)
	if !typ.TypeEquals(got, fallbackType) {
		t.Fatalf("expected fallback symbol type, got %v", got)
	}
}

func TestResolveCalleeType_UsesCalleePathSymbolCandidate(t *testing.T) {
	info := &cfg.CallInfo{
		CalleeSymbol: 9,
		Callee:       &ast.IdentExpr{Value: "h"},
		CalleePath:   constraint.Path{Symbol: 11},
	}

	fallbackType := typ.Func().Returns(typ.Number).Build()
	got := ResolveCalleeType(
		info,
		0,
		nil,
		nil,
		nil,
		nil,
		func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
			if sym == 11 {
				return fallbackType, true
			}
			return nil, false
		},
	)
	if !typ.TypeEquals(got, fallbackType) {
		t.Fatalf("expected callee-path fallback symbol type, got %v", got)
	}
}

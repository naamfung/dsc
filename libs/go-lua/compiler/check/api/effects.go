package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// RefinementFacts provides function refinement lookup.
type RefinementFacts interface {
	LookupBySym(sym cfg.SymbolID) *constraint.FunctionRefinement
}

// RefinementStore provides methods for storing and retrieving function refinements.
type RefinementStore interface {
	LookupRefinementBySym(sym cfg.SymbolID) *constraint.FunctionRefinement
}

// storeRefinementFacts implements RefinementFacts backed by a RefinementStore.
type storeRefinementFacts struct {
	store RefinementStore
}

// NewRefinementFacts creates RefinementFacts backed by a RefinementStore.
func NewRefinementFacts(store RefinementStore) RefinementFacts {
	if store == nil {
		return nilRefinementFacts{}
	}
	return &storeRefinementFacts{store: store}
}

func (f *storeRefinementFacts) LookupBySym(sym cfg.SymbolID) *constraint.FunctionRefinement {
	if f.store == nil || sym == 0 {
		return nil
	}
	return f.store.LookupRefinementBySym(sym)
}

// nilRefinementFacts is a no-op RefinementFacts implementation.
type nilRefinementFacts struct{}

func (nilRefinementFacts) LookupBySym(cfg.SymbolID) *constraint.FunctionRefinement { return nil }

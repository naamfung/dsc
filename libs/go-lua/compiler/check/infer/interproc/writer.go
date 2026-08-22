package interproc

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

type factsWriteStore interface {
	UpdateInterprocFactsNext(key api.GraphKey, update func(*api.Facts))
	StoreLiteralSigs(graphID uint64, sigs map[*ast.FunctionExpr]*typ.Function)
	GraphKeyFor(graph *cfg.Graph, parent *scope.State) (api.GraphKey, bool)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool)
}

type interprocFactWriter struct {
	store factsWriteStore
}

func newInterprocFactWriter(store factsWriteStore) interprocFactWriter {
	return interprocFactWriter{store: store}
}

func (w interprocFactWriter) updateParentFactsForSymbol(sym cfg.SymbolID, update func(*api.Facts)) bool {
	if w.store == nil || sym == 0 || update == nil {
		return false
	}
	parentKey, ok := w.store.ParentGraphKeyForSymbol(sym)
	if !ok {
		return false
	}
	w.store.UpdateInterprocFactsNext(parentKey, update)
	return true
}

func (w interprocFactWriter) writeLiteralSignatures(
	graph *cfg.Graph,
	parent *scope.State,
	sigs map[*ast.FunctionExpr]*typ.Function,
) {
	if w.store == nil || graph == nil || len(sigs) == 0 {
		return
	}
	w.store.StoreLiteralSigs(graph.ID(), sigs)
	if key, ok := w.store.GraphKeyFor(graph, parent); ok {
		w.store.UpdateInterprocFactsNext(key, func(facts *api.Facts) {
			if facts.LiteralSigs == nil {
				facts.LiteralSigs = make(api.LiteralSigs, len(sigs))
			}
			for fnExpr, sig := range sigs {
				if fnExpr != nil && sig != nil {
					facts.LiteralSigs[fnExpr] = sig
				}
			}
		})
	}
}

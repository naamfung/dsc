package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

type factsWriteStoreStub struct {
	graphKeyFor         api.GraphKey
	graphKeyForOK       bool
	parentKeyBySymbol   map[cfg.SymbolID]api.GraphKey
	literalSigsByGraph  map[uint64]map[*ast.FunctionExpr]*typ.Function
	factsByGraphKeyNext map[api.GraphKey]api.Facts
}

func newFactsWriteStoreStub() *factsWriteStoreStub {
	return &factsWriteStoreStub{
		parentKeyBySymbol:   make(map[cfg.SymbolID]api.GraphKey),
		literalSigsByGraph:  make(map[uint64]map[*ast.FunctionExpr]*typ.Function),
		factsByGraphKeyNext: make(map[api.GraphKey]api.Facts),
	}
}

func (s *factsWriteStoreStub) UpdateInterprocFactsNext(key api.GraphKey, update func(*api.Facts)) {
	facts := s.factsByGraphKeyNext[key]
	update(&facts)
	s.factsByGraphKeyNext[key] = facts
}

func (s *factsWriteStoreStub) StoreLiteralSigs(graphID uint64, sigs map[*ast.FunctionExpr]*typ.Function) {
	s.literalSigsByGraph[graphID] = sigs
}

func (s *factsWriteStoreStub) GraphKeyFor(_ *cfg.Graph, _ *scope.State) (api.GraphKey, bool) {
	return s.graphKeyFor, s.graphKeyForOK
}

func (s *factsWriteStoreStub) ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool) {
	key, ok := s.parentKeyBySymbol[sym]
	return key, ok
}

func TestInterprocFactWriter_UpdateParentFactsForSymbol(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 7, ParentHash: 11}
	stub.parentKeyBySymbol[3] = key
	writer := newInterprocFactWriter(stub)

	ok := writer.updateParentFactsForSymbol(3, func(facts *api.Facts) {
		facts.ParamHints = map[cfg.SymbolID][]typ.Type{
			3: {typ.String},
		}
	})
	if !ok {
		t.Fatal("expected update to succeed")
	}
	got := stub.factsByGraphKeyNext[key]
	if len(got.ParamHints[3]) != 1 || !typ.TypeEquals(got.ParamHints[3][0], typ.String) {
		t.Fatalf("unexpected parent facts update: %#v", got.ParamHints)
	}

	if writer.updateParentFactsForSymbol(99, func(*api.Facts) {}) {
		t.Fatal("expected update to fail for unknown symbol")
	}
}

func TestInterprocFactWriter_WriteLiteralSignatures(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 5, ParentHash: 9}
	stub.graphKeyFor = key
	stub.graphKeyForOK = true
	writer := newInterprocFactWriter(stub)

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	sig := typ.Func().Returns(typ.String).Build()
	sigs := map[*ast.FunctionExpr]*typ.Function{fn: sig}

	writer.writeLiteralSignatures(graph, scope.New(), sigs)

	if len(stub.literalSigsByGraph[graph.ID()]) != 1 || stub.literalSigsByGraph[graph.ID()][fn] != sig {
		t.Fatalf("expected literal sigs stored for graph %d", graph.ID())
	}
	gotFacts := stub.factsByGraphKeyNext[key]
	if gotFacts.LiteralSigs == nil || gotFacts.LiteralSigs[fn] != sig {
		t.Fatalf("expected literal sig in facts update, got %#v", gotFacts.LiteralSigs)
	}
}

func TestInterprocFactWriter_WriteLiteralSignatures_StoresScratchWithoutGraphKey(t *testing.T) {
	stub := newFactsWriteStoreStub()
	stub.graphKeyForOK = false
	writer := newInterprocFactWriter(stub)

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	sig := typ.Func().Returns(typ.Number).Build()
	writer.writeLiteralSignatures(graph, scope.New(), map[*ast.FunctionExpr]*typ.Function{fn: sig})

	if len(stub.literalSigsByGraph[graph.ID()]) != 1 || stub.literalSigsByGraph[graph.ID()][fn] != sig {
		t.Fatalf("expected literal sigs stored even without graph key")
	}
	if len(stub.factsByGraphKeyNext) != 0 {
		t.Fatalf("expected no facts writes without graph key, got %#v", stub.factsByGraphKeyNext)
	}
}

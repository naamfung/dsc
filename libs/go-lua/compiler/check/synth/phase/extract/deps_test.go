package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/db"
)

func TestNewDeps(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	types := mockTypeQuerier{}
	scopes := make(api.ScopeMap)

	deps := NewDeps(ctx, types, scopes, nil, nil)

	if deps.Ctx != ctx {
		t.Fatal("context mismatch")
	}
	if deps.Types == nil {
		t.Fatal("types not set")
	}
	if deps.Scopes == nil {
		t.Fatal("scopes not set")
	}
	if deps.PreCache == nil {
		t.Fatal("preCache not initialized")
	}
	if deps.NarrowCache == nil {
		t.Fatal("narrowCache not initialized")
	}
}

func TestDeps_Graph_NilCheckCtx(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		CheckCtx: nil,
	}

	g := deps.Graph()
	if g != nil {
		t.Fatal("expected nil graph")
	}
}

func TestDeps_Entry_NilGraph(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		CheckCtx: nil,
	}

	entry := deps.Entry()
	if entry != 0 {
		t.Fatalf("got %v, want 0", entry)
	}
}

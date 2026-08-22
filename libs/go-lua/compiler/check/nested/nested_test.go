package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestGatherChildren_NilGraph(t *testing.T) {
	children := GatherChildren(nil, nil, nil)
	if len(children) != 0 {
		t.Errorf("expected empty children for nil graph, got %d", len(children))
	}
}

func TestGatherChildren_NilNestedFuncs(t *testing.T) {
	graph := &cfg.Graph{}
	children := GatherChildren(graph, nil, nil)
	if len(children) != 0 {
		t.Errorf("expected empty children for graph with no nested functions, got %d", len(children))
	}
}

func TestResolveNestedFuncIdentity_NilFuncDef(t *testing.T) {
	graph := &cfg.Graph{}
	nf := cfg.NestedFunc{Point: 0, Func: nil, Symbol: 0}
	name, sym, isLocal := ResolveNestedFuncIdentity(graph, nf, nil)
	if name != "" || sym != 0 || isLocal != false {
		t.Errorf("expected empty identity for nil funcDef and zero symbol, got (%s, %d, %v)", name, sym, isLocal)
	}
}

func TestResolveNestedFuncIdentity_WithSymbol(t *testing.T) {
	graph := &cfg.Graph{}
	nf := cfg.NestedFunc{Point: 0, Func: nil, Symbol: cfg.SymbolID(42)}
	name, sym, isLocal := ResolveNestedFuncIdentity(graph, nf, nil)
	if name != "" {
		t.Errorf("expected empty name, got %s", name)
	}
	if sym != 42 {
		t.Errorf("expected symbol 42, got %d", sym)
	}
	if !isLocal {
		t.Error("expected isLocal to be true")
	}
}

func TestChild_Struct(t *testing.T) {
	c := Child{
		FuncName: "test",
		FuncSym:  cfg.SymbolID(1),
		IsLocal:  true,
	}
	if c.FuncName != "test" {
		t.Errorf("expected FuncName 'test', got %s", c.FuncName)
	}
	if c.FuncSym != 1 {
		t.Errorf("expected FuncSym 1, got %d", c.FuncSym)
	}
	if !c.IsLocal {
		t.Error("expected IsLocal true")
	}
}

func TestFuncInfo_Struct(t *testing.T) {
	fi := FuncInfo{
		Child: Child{
			FuncName: "test",
			FuncSym:  cfg.SymbolID(1),
			IsLocal:  true,
		},
	}
	if fi.FuncName != "test" {
		t.Errorf("expected FuncName 'test', got %s", fi.FuncName)
	}
}

func TestScopeGroup_Struct(t *testing.T) {
	sg := ScopeGroup{
		Hash:     12345,
		Funcs:    nil,
		MinPoint: cfg.Point(10),
	}
	if sg.Hash != 12345 {
		t.Errorf("expected Hash 12345, got %d", sg.Hash)
	}
	if sg.MinPoint != 10 {
		t.Errorf("expected MinPoint 10, got %d", sg.MinPoint)
	}
}

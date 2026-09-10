package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexerInfo(t *testing.T) {
	info := IndexerInfo{
		KeyType: typ.String,
		ValType: typ.Integer,
	}
	if info.KeyType != typ.String {
		t.Errorf("expected KeyType to be String, got %v", info.KeyType)
	}
	if info.ValType != typ.Integer {
		t.Errorf("expected ValType to be Integer, got %v", info.ValType)
	}
}

func TestCollectTableInsertMutations_NilGraph(t *testing.T) {
	result := CollectTableInsertMutations(nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil map for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestCollectTableInsertOnDirect_NilGraph(t *testing.T) {
	result := CollectTableInsertOnDirect(nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil map for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestMergeIndexerMutations_EmptyInputs(t *testing.T) {
	indexers := make(map[cfg.SymbolID][]IndexerInfo)
	mutations := make(map[cfg.SymbolID][]IndexerInfo)

	MergeIndexerMutations(indexers, mutations)

	if len(indexers) != 0 {
		t.Errorf("expected empty indexers after merging empty mutations, got %d", len(indexers))
	}
}

func TestMergeIndexerMutations_MergesCorrectly(t *testing.T) {
	indexers := make(map[cfg.SymbolID][]IndexerInfo)
	indexers[1] = []IndexerInfo{{KeyType: typ.String, ValType: typ.Integer}}

	mutations := make(map[cfg.SymbolID][]IndexerInfo)
	mutations[1] = []IndexerInfo{{KeyType: typ.Integer, ValType: typ.String}}
	mutations[2] = []IndexerInfo{{KeyType: typ.Number, ValType: typ.Boolean}}

	MergeIndexerMutations(indexers, mutations)

	if len(indexers[1]) != 2 {
		t.Errorf("expected 2 infos for symbol 1, got %d", len(indexers[1]))
	}
	if len(indexers[2]) != 1 {
		t.Errorf("expected 1 info for symbol 2, got %d", len(indexers[2]))
	}
}

func TestCollectTableInsertOnDirect_AssignmentCallSite(t *testing.T) {
	code := `
		local t = {}
		local _ = table.insert(t, 1)
	`
	graph := buildGraph(t, code, "table")
	bindings := graph.Bindings()

	result := CollectTableInsertOnDirect(graph, tableInsertSynth(), bindings)
	if len(result) == 0 {
		t.Fatal("expected direct table mutation from assignment call site")
	}

	symT, ok := graph.SymbolAt(graph.Exit(), "t")
	if !ok || symT == 0 {
		t.Fatal("expected symbol for t")
	}
	got := result[symT]
	if got == nil || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected element type integer for t, got %v", got)
	}
}

func TestCollectTableInsertMutations_AssignmentCallSite(t *testing.T) {
	code := `
		local t = {}
		local k = "suite"
		local _ = table.insert(t[k], 1)
	`
	graph := buildGraph(t, code, "table")
	bindings := graph.Bindings()

	result := CollectTableInsertMutations(graph, tableInsertSynth(), bindings)
	if len(result) == 0 {
		t.Fatal("expected indexed table mutation from assignment call site")
	}

	symT, ok := graph.SymbolAt(graph.Exit(), "t")
	if !ok || symT == 0 {
		t.Fatal("expected symbol for t")
	}
	infos := result[symT]
	if len(infos) != 1 {
		t.Fatalf("expected 1 indexed mutation for t, got %d", len(infos))
	}
	if !typ.TypeEquals(infos[0].KeyType, typ.String) {
		t.Fatalf("expected key type string, got %v", infos[0].KeyType)
	}
	expectedVal := typ.NewArray(typ.Integer)
	if !typ.TypeEquals(infos[0].ValType, expectedVal) {
		t.Fatalf("expected value type %v, got %v", expectedVal, infos[0].ValType)
	}
}

func TestCollectTableInsertMutations_NestedBasePath(t *testing.T) {
	code := `
		local state = {}
		local k = "suite"
		local _ = table.insert(state.users[k], 1)
	`
	graph := buildGraph(t, code, "table")
	bindings := graph.Bindings()

	result := CollectTableInsertMutations(graph, tableInsertSynth(), bindings)
	if len(result) == 0 {
		t.Fatal("expected indexed table mutation from nested base path")
	}

	stateSym, ok := graph.SymbolAt(graph.Exit(), "state")
	if !ok || stateSym == 0 {
		t.Fatal("expected symbol for state")
	}
	usersSym := bindings.GetOrCreateFieldSymbol(stateSym, "users")
	infos := result[usersSym]
	if len(infos) != 1 {
		t.Fatalf("expected 1 indexed mutation for state.users, got %d", len(infos))
	}
	if !typ.TypeEquals(infos[0].KeyType, typ.String) {
		t.Fatalf("expected key type string, got %v", infos[0].KeyType)
	}
	expectedVal := typ.NewArray(typ.Integer)
	if !typ.TypeEquals(infos[0].ValType, expectedVal) {
		t.Fatalf("expected value type %v, got %v", expectedVal, infos[0].ValType)
	}
}

func TestCollectTableInsertOnDirect_NestedBasePath(t *testing.T) {
	code := `
		local state = {}
		local _ = table.insert(state.users, 1)
	`
	graph := buildGraph(t, code, "table")
	bindings := graph.Bindings()

	result := CollectTableInsertOnDirect(graph, tableInsertSynth(), bindings)
	if len(result) == 0 {
		t.Fatal("expected direct table mutation for nested base path")
	}

	stateSym, ok := graph.SymbolAt(graph.Exit(), "state")
	if !ok || stateSym == 0 {
		t.Fatal("expected symbol for state")
	}
	usersSym := bindings.GetOrCreateFieldSymbol(stateSym, "users")
	got := result[usersSym]
	if got == nil || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected element type integer for state.users, got %v", got)
	}
}

func buildGraph(t *testing.T, code string, globals ...string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: stmts}
	graph := cfg.Build(fn, globals...)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
	return graph
}

func tableInsertSynth() func(ast.Expr, cfg.Point) typ.Type {
	spec := contract.NewSpec().WithEffects(effect.TableMutator{
		Target: effect.ParamRef{Index: 0},
		Value:  effect.ParamRef{Index: 1},
	})
	tableInsert := typ.Func().
		Param("target", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Nil).
		Spec(spec).
		Build()

	return func(expr ast.Expr, _ cfg.Point) typ.Type {
		switch v := expr.(type) {
		case *ast.AttrGetExpr:
			obj, ok := v.Object.(*ast.IdentExpr)
			if !ok || obj.Value != "table" {
				return typ.Unknown
			}
			switch key := v.Key.(type) {
			case *ast.IdentExpr:
				if key.Value == "insert" {
					return tableInsert
				}
			case *ast.StringExpr:
				if key.Value == "insert" {
					return tableInsert
				}
			}
		case *ast.NumberExpr:
			return typ.Integer
		case *ast.IdentExpr:
			if v.Value == "k" {
				return typ.String
			}
		}
		return typ.Unknown
	}
}

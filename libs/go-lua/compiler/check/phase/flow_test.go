package phase

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractParams_NilParList(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: nil}
	result := ExtractParams(fn, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil ParList, got %v", result)
	}
}

func TestExtractParams_NilFunction(t *testing.T) {
	result := ExtractParams(nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil function, got %v", result)
	}
}

func TestExtractParams_EmptyParList(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{}}}
	result := ExtractParams(fn, nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty params for empty ParList, got %v", result)
	}
}

func TestExtractParams_WithNames(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"a", "b"}}}
	graph := cfg.Build(fn)
	result := ExtractParams(fn, nil, graph)
	if len(result) != 2 {
		t.Fatalf("expected 2 params, got %d", len(result))
	}
	if result[0].Name != "a" {
		t.Errorf("expected first param name 'a', got %q", result[0].Name)
	}
	if result[1].Name != "b" {
		t.Errorf("expected second param name 'b', got %q", result[1].Name)
	}
}

func TestExtractParams_WithTypes(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}}}
	graph := cfg.Build(fn)
	paramSymbols := graph.ParamSymbols()
	if len(paramSymbols) == 0 {
		t.Skip("no param symbols in graph")
	}
	paramTypes := map[cfg.SymbolID]typ.Type{paramSymbols[0]: typ.Number}
	result := ExtractParams(fn, paramTypes, graph)
	if len(result) != 1 {
		t.Fatalf("expected 1 param, got %d", len(result))
	}
	if !typ.TypeEquals(result[0].Type, typ.Number) {
		t.Errorf("expected Number type, got %v", result[0].Type)
	}
}

func TestInferRefinement_NilGraph(t *testing.T) {
	result := InferRefinement(nil, &flow.Solution{}, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil graph, got %v", result)
	}
}

func TestInferRefinement_NilSolution(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := InferRefinement(graph, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil solution, got %v", result)
	}
}

func TestEnrichWithKeysCollector_NilFn(t *testing.T) {
	result := EnrichWithKeysCollector(nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil fn, got %v", result)
	}
}

func TestEnrichWithKeysCollector_NonKeysCollector(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	result := EnrichWithKeysCollector(nil, fn)
	if result != nil {
		t.Errorf("expected nil for non-keys-collector fn, got %v", result)
	}
}

func TestEnrichWithKeysCollector_UsesDetectedReturnIndex(t *testing.T) {
	body, err := parse.ParseString(`
		local keys = {}
		for k in pairs(tbl) do
			table.insert(keys, k)
		end
		return nil, keys
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"tbl"}},
		Stmts:   body,
	}

	result := EnrichWithKeysCollector(nil, fn)
	if result == nil {
		t.Fatal("expected non-nil enriched effect")
	}

	want := constraint.KeyOf{
		Table: constraint.ParamPath(0),
		Key:   constraint.RetPath(1),
	}
	disjuncts := result.OnReturn.Disjuncts
	if len(disjuncts) != 1 {
		t.Fatalf("OnReturn disjunct count = %d, want 1", len(disjuncts))
	}
	if !constraint.ConjunctionContains(disjuncts[0], want) {
		t.Fatalf("expected OnReturn to contain %v, got %v", want, result.OnReturn)
	}
}

func TestEnrichWithKeysCollector_AppendsToExistingOnReturn(t *testing.T) {
	body, err := parse.ParseString(`
		local keys = {}
		for k in pairs(tbl) do
			table.insert(keys, k)
		end
		return nil, keys
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"tbl"}},
		Stmts:   body,
	}

	existing := &constraint.FunctionRefinement{
		OnReturn: constraint.FromConstraints(constraint.NotNil{Path: constraint.RetPath(0)}),
	}
	result := EnrichWithKeysCollector(existing, fn)
	if result == nil {
		t.Fatal("expected non-nil enriched effect")
	}

	disjuncts := result.OnReturn.Disjuncts
	if len(disjuncts) != 1 {
		t.Fatalf("OnReturn disjunct count = %d, want 1", len(disjuncts))
	}
	if !constraint.ConjunctionContains(disjuncts[0], constraint.NotNil{Path: constraint.RetPath(0)}) {
		t.Fatalf("expected existing NotNil constraint to be preserved: %v", result.OnReturn)
	}
	if !constraint.ConjunctionContains(disjuncts[0], constraint.KeyOf{
		Table: constraint.ParamPath(0),
		Key:   constraint.RetPath(1),
	}) {
		t.Fatalf("expected keys collector KeyOf to be appended: %v", result.OnReturn)
	}
}

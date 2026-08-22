package flowbuild_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/constprop"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/decl"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRootName_WithSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
		},
	}
	graph := cfg.Build(fn)

	// Graph provides NameOf for symbols
	result := resolve.RootName(graph, 1, "fallback")
	// Symbol 1 might not exist, so fallback is expected
	if result == "" {
		t.Error("expected non-empty root name")
	}
}

func TestRootName_NilGraph(t *testing.T) {
	result := resolve.RootName(nil, 1, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", result)
	}
}

func TestRootName_ZeroSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := resolve.RootName(graph, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", result)
	}
}

func TestRootNameFromBindings_NilBindings(t *testing.T) {
	result := resolve.RootNameFromBindings(nil, 1, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", result)
	}
}

func TestRootNameFromBindings_ZeroSymbol(t *testing.T) {
	result := resolve.RootNameFromBindings(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", result)
	}
}

func TestCollectConstAssignments_MinimalGraph(t *testing.T) {
	// Create a minimal graph
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := testCollectConstAssignments(graph)
	if result == nil {
		t.Error("expected non-nil map")
	}
}

func TestPropagateAllConstValues_NilGraph(t *testing.T) {
	result := testPropagateAllConstValues(nil, nil)
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestPropagateAllConstValues_EmptyAssigns(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := testPropagateAllConstValues(graph, nil)
	if result == nil {
		t.Error("expected non-nil map even for empty assigns")
	}
}

func TestBuildConstResolver_NilInputs(t *testing.T) {
	resolver := predicate.BuildConstResolver(nil, 0)
	if resolver != nil {
		t.Error("expected nil resolver for nil inputs")
	}
}

func TestBuildConstResolver_NilGraph(t *testing.T) {
	inputs := &flow.Inputs{
		ConstValues: map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue{},
		Graph:       nil,
	}
	resolver := predicate.BuildConstResolver(inputs, 0)
	if resolver != nil {
		t.Error("expected nil resolver when graph is nil")
	}
}

func TestLinkKey(t *testing.T) {
	key := predicate.LinkKey("isValid", 42)
	expected := "isValid@42"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestCollectInferredTypes_NilGraph(t *testing.T) {
	result := assign.CollectInferredTypes(&core.FlowContext{}, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil map")
	}
	if len(result) != 0 {
		t.Error("expected empty map for nil graph")
	}
}

func TestExtractIterSource_EmptyExprs(t *testing.T) {
	result := resolve.ExtractIteratorSource(nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for empty expressions")
	}
}

func TestExtractIterSource_NotCall(t *testing.T) {
	exprs := []ast.Expr{&ast.IdentExpr{Value: "x"}}
	result := resolve.ExtractIteratorSource(exprs, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for non-call expression")
	}
}

func TestBuildReceiverDependencies_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := assign.BuildReceiverDependencies(graph)
	if result == nil {
		t.Error("expected non-nil map")
	}
}

func TestExtractDeclaredTypes_NilGraph(t *testing.T) {
	decl.ExtractDeclaredTypes(&core.FlowContext{}, nil)
	// Should not panic
}

func TestExtractModuleAliases_NilGraph(t *testing.T) {
	decl.ExtractModuleAliases(&core.FlowContext{}, nil)
	// Should not panic
}

func TestExtractModuleAliases_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ModuleAliases: make(map[cfg.SymbolID]string),
	}
	decl.ExtractModuleAliases(&core.FlowContext{Graph: graph}, inputs)
	if len(inputs.ModuleAliases) != 0 {
		t.Error("expected no module aliases for empty graph")
	}
}

func TestNarrowReturnTypeBySpec_NilInputs(t *testing.T) {
	result := assign.NarrowReturnTypeBySpec(nil, nil, nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestSpecNarrowedTypes_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	scopes := map[cfg.Point]*scope.State{}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return nil
	}
	result := assign.CollectSpecNarrowedTypes(graph, scopes, synth, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil SpecTypes map")
	}
}

func TestExtractFuncDefAssignments_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	scopes := map[cfg.Point]*scope.State{}
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
		Assignments:   []flow.UnifiedAssignment{},
	}
	assign.ExtractFuncDefAssignments(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// Should not panic
}

func TestEdgeKey_Comparison(t *testing.T) {
	k1 := cond.EdgeKey{From: 1, To: 2}
	k2 := cond.EdgeKey{From: 1, To: 2}
	k3 := cond.EdgeKey{From: 1, To: 3}

	if k1 != k2 {
		t.Error("expected equal keys to be equal")
	}
	if k1 == k3 {
		t.Error("expected different keys to be different")
	}
}

func TestExtractAssignments_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		ConstValues:        make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		TypeKeys:           make(map[uint64]typ.Type),
		ReturnKinds:        make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints:  make(map[cfg.Point]flow.ReturnExprConstraints),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
		ModuleAliases:      make(map[cfg.SymbolID]string),
		Assignments:        []flow.UnifiedAssignment{},
	}
	scopes := map[cfg.Point]*scope.State{}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return nil, false
	}
	assign.ExtractAssignments(&core.FlowContext{
		Graph:   graph,
		Scopes:  scopes,
		Derived: &core.Derived{SymResolver: symResolver},
	}, inputs, nil)
	// Should not panic
}

func TestArgAtCall_Empty(t *testing.T) {
	result := callsite.PositionalArgAt(nil, 0)
	if result != nil {
		t.Error("expected nil for empty args")
	}
}

func TestArgAtCall_Positive(t *testing.T) {
	args := []ast.Expr{
		&ast.StringExpr{Value: "a"},
		&ast.StringExpr{Value: "b"},
	}
	result := callsite.PositionalArgAt(args, 1)
	str, ok := result.(*ast.StringExpr)
	if !ok || str.Value != "b" {
		t.Errorf("expected 'b', got %v", result)
	}
}

func TestArgAtCall_Negative(t *testing.T) {
	args := []ast.Expr{
		&ast.StringExpr{Value: "a"},
		&ast.StringExpr{Value: "b"},
	}
	result := callsite.PositionalArgAt(args, -1)
	str, ok := result.(*ast.StringExpr)
	if !ok || str.Value != "b" {
		t.Errorf("expected 'b', got %v", result)
	}
}

func TestArgAtCall_OutOfBounds(t *testing.T) {
	args := []ast.Expr{&ast.StringExpr{Value: "a"}}
	result := callsite.PositionalArgAt(args, 5)
	if result != nil {
		t.Error("expected nil for out of bounds")
	}
}

func TestIterSourceInfo_Fields(t *testing.T) {
	info := resolve.IteratorSourceInfo{
		Path: constraint.Path{Root: "arr", Symbol: 1},
		Kind: flow.IterateIndexed,
	}
	if info.Path.Root != "arr" {
		t.Errorf("expected root 'arr', got '%s'", info.Path.Root)
	}
	if info.Kind != flow.IterateIndexed {
		t.Errorf("expected IterateIndexed, got %v", info.Kind)
	}
}

func testCollectConstAssignments(graph *cfg.Graph) map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue {
	inputs := &flow.Inputs{ConstValues: make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue)}
	constprop.CollectConstAssignments(&core.FlowContext{Graph: graph}, inputs)
	return inputs.ConstValues
}

func testPropagateAllConstValues(graph *cfg.Graph, assigns map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue) map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue {
	if graph == nil || graph.CFG() == nil {
		return nil
	}
	inputs := &flow.Inputs{ConstValues: assigns}
	constprop.PropagateAllConstValues(&core.FlowContext{Graph: graph}, inputs)
	return inputs.ConstValues
}

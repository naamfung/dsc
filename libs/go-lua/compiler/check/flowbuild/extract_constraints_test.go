package flowbuild_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkeffects "github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestGetBindings_NilInputs(t *testing.T) {
	result := resolve.GetBindings(nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestGetBindings_NilGraph(t *testing.T) {
	inputs := &flow.Inputs{Graph: nil}
	result := resolve.GetBindings(inputs)
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestGetBindings_WithGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{Graph: graph}
	result := resolve.GetBindings(inputs)
	// Bindings should be available
	_ = result
}

func TestFindBranchEdges_NoEdges(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	trueEdge, falseEdge := cond.FindBranchEdges(graph, 0, nil)
	if trueEdge != 0 || falseEdge != 0 {
		t.Error("expected zero edges for empty successors")
	}
}

func TestExtractEdgeConstraints_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues: make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
	}
	scopes := map[cfg.Point]*scope.State{}
	cond.ExtractEdgeConstraints(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	// Should not panic
}

func TestExtractNumericConstraints_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues: make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
	}
	cond.ExtractNumericConstraints(&core.FlowContext{Graph: graph}, inputs)
	// Should not panic
}

func TestExtractCallOnReturnConstraints_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	inputs := &flow.Inputs{
		ConstValues: make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
	}
	scopes := map[cfg.Point]*scope.State{}
	result := cond.ExtractCallOnReturnConstraints(&core.FlowContext{Graph: graph, Scopes: scopes}, inputs)
	if result == nil {
		t.Error("expected non-nil map")
	}
}

func TestExtractEffectFromType_Nil(t *testing.T) {
	result := checkeffects.EffectFromType(nil)
	if result != nil {
		t.Error("expected nil for nil type")
	}
}

func TestExtractEffectFromType_NonFunction(t *testing.T) {
	result := checkeffects.EffectFromType(typ.String)
	if result != nil {
		t.Error("expected nil for non-function type")
	}
}

func TestExtractEffectFromType_FunctionWithoutRefinement(t *testing.T) {
	fn := &typ.Function{}
	result := checkeffects.EffectFromType(fn)
	if result != nil {
		t.Error("expected nil for function without refinement")
	}
}

func TestExtractEffectFromType_AliasedFunction(t *testing.T) {
	fn := &typ.Function{}
	alias := &typ.Alias{Name: "MyFunc", Target: fn}
	result := checkeffects.EffectFromType(alias)
	if result != nil {
		t.Error("expected nil for function without refinement")
	}
}

func TestResolveCalleeToFunctionLiteral_NilCallee(t *testing.T) {
	result := resolve.ResolveCalleeToFunctionLiteral(nil, nil)
	if result != nil {
		t.Error("expected nil for nil callee")
	}
}

func TestResolveCalleeToFunctionLiteral_DirectFunctionLiteral(t *testing.T) {
	fn := &ast.FunctionExpr{}
	result := resolve.ResolveCalleeToFunctionLiteral(fn, nil)
	if result != fn {
		t.Error("expected same function literal")
	}
}

func TestResolveCalleeToFunctionLiteral_Ident(t *testing.T) {
	ident := &ast.IdentExpr{Value: "fn"}
	result := resolve.ResolveCalleeToFunctionLiteral(ident, nil)
	if result != nil {
		t.Error("expected nil for ident without graph")
	}
}

func TestResolveExprToTableLiteral_NilExpr(t *testing.T) {
	result := resolve.ResolveExprToTableLiteral(nil, nil)
	if result != nil {
		t.Error("expected nil for nil expr")
	}
}

func TestResolveExprToTableLiteral_DirectTable(t *testing.T) {
	tbl := &ast.TableExpr{}
	// Function returns nil when graph is nil (guards against nil graph access)
	result := resolve.ResolveExprToTableLiteral(tbl, nil)
	if result != nil {
		t.Error("expected nil when graph is nil")
	}
}

func TestResolveExprToTableLiteral_IdentWithoutGraph(t *testing.T) {
	ident := &ast.IdentExpr{Value: "tbl"}
	result := resolve.ResolveExprToTableLiteral(ident, nil)
	if result != nil {
		t.Error("expected nil without graph")
	}
}

func TestCallTerminates_NilInfo(t *testing.T) {
	result := cond.CallTerminates(nil, 0, nil, nil, nil, nil, nil)
	if result {
		t.Error("expected false for nil info")
	}
}

func TestCallTerminates_NormalFunction(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "print"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return &typ.Function{}
	}
	result := cond.CallTerminates(info, 0, synth, nil, nil, nil, nil)
	if result {
		t.Error("expected false for normal function")
	}
}

func TestConstraintsFromCallOnReturn_NilInfo(t *testing.T) {
	result := cond.ConstraintsFromCallOnReturn(nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("expected no constraints for nil info")
	}
}

func TestConstraintsFromCallOnReturn_MethodCall(t *testing.T) {
	info := &cfg.CallInfo{
		Method:   "foo",
		Receiver: &ast.IdentExpr{Value: "obj"},
	}
	result := cond.ConstraintsFromCallOnReturn(info, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("expected no constraints for method call")
	}
}

func TestConstraintsFromCallOnReturn_NoArgs(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
		Args:   nil,
	}
	result := cond.ConstraintsFromCallOnReturn(info, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("expected no constraints for call without args")
	}
}

func TestConstraintsFromAssignOnReturn_NilInfo(t *testing.T) {
	result := cond.ConstraintsFromAssignOnReturn(nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("expected no constraints for nil info")
	}
}

func TestConstraintsFromAssignOnReturn_NoSourceCalls(t *testing.T) {
	info := &cfg.AssignInfo{
		SourceCalls: nil,
	}
	result := cond.ConstraintsFromAssignOnReturn(info, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("expected no constraints without source calls")
	}
}

func TestExtractPredicateLinkFromCallInfo_NilInfo(t *testing.T) {
	result := cond.ExtractPredicateLinkFromCallInfo(nil, 0, 0, nil, nil, nil, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}

func TestExtractPredicateLinkFromCallInfo_NoArgs(t *testing.T) {
	info := &cfg.CallInfo{
		Args: nil,
	}
	result := cond.ExtractPredicateLinkFromCallInfo(info, 0, 0, nil, nil, nil, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for call without args")
	}
}

func TestNumericForConstraints_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := cond.NumericForConstraints(graph, 0, "i", 0)
	if result != nil {
		t.Error("expected nil for non-numeric-for point")
	}
}

func TestExtractLenOfPath_NonLenOp(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := cond.ExtractLenOfPath(expr, 0, graph)
	if !result.IsEmpty() {
		t.Error("expected empty path for non-len expression")
	}
}

func TestExtractLenOfPath_LenOfNonIdent(t *testing.T) {
	expr := &ast.UnaryLenOpExpr{
		Expr: &ast.NumberExpr{Value: "1"},
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := cond.ExtractLenOfPath(expr, 0, graph)
	if !result.IsEmpty() {
		t.Error("expected empty path for len of non-ident")
	}
}

func TestExtractLenOfPath_LenOfIdent(t *testing.T) {
	expr := &ast.UnaryLenOpExpr{
		Expr: &ast.IdentExpr{Value: "arr"},
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := cond.ExtractLenOfPath(expr, 0, graph)
	if result.Root != "arr" {
		t.Errorf("expected root 'arr', got '%s'", result.Root)
	}
}

func TestNumericConstraintsFromExpr(t *testing.T) {
	// Test basic comparison extraction
	expr := &ast.RelationalOpExpr{
		Operator: "<",
		Lhs:      &ast.IdentExpr{Value: "x"},
		Rhs:      &ast.NumberExpr{Value: "10"},
	}
	inputs := &flow.Inputs{
		ConstValues: make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
	}
	result := cond.NumericConstraintsFromExpr(expr, 0, inputs)
	if len(result) == 0 {
		t.Error("expected numeric constraints from comparison")
	}
}

func TestNumericConstraintsFromExpr_NilExpr(t *testing.T) {
	inputs := &flow.Inputs{}
	result := cond.NumericConstraintsFromExpr(nil, 0, inputs)
	if len(result) != 0 {
		t.Error("expected no constraints for nil expr")
	}
}

func TestConstraintsFromBranch_NilInfo(t *testing.T) {
	result := (&cond.ConditionExtractor{}).ConstraintsFromBranch(nil)
	if result.OnTrue.HasConstraints() || result.OnFalse.HasConstraints() {
		t.Error("expected no constraints for nil info")
	}
}

func TestFindBranchEdges_WithEdges(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then:      []ast.Stmt{},
				Else:      []ast.Stmt{},
			},
		},
	}
	graph := cfg.Build(fn)
	// Check branch edges can be found
	var branchPoint cfg.Point
	graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		branchPoint = p
	})
	if branchPoint != 0 {
		succs := graph.Successors(branchPoint)
		trueEdge, falseEdge := cond.FindBranchEdges(graph, branchPoint, succs)
		// At least one edge should be found for if statement
		if trueEdge == 0 && falseEdge == 0 && len(succs) >= 2 {
			t.Error("expected at least one edge for if branch")
		}
	}
}

func TestExtractFunctionRefinement_EmptyInfo(t *testing.T) {
	info := &cfg.CallInfo{}
	result := cond.ExtractFunctionRefinement(info, 0, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for empty info")
	}
}

func TestExtractFunctionRefinement_WithCallee(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return &typ.Function{}
	}
	result := cond.ExtractFunctionRefinement(info, 0, synth, nil, nil, nil, nil)
	// No refinement, so nil expected
	if result != nil {
		t.Error("expected nil for function without refinement")
	}
}

func TestConstraintPath_Stability(t *testing.T) {
	// Verify paths with same root and symbol are equal
	p1 := constraint.Path{Root: "x", Symbol: 1}
	p2 := constraint.Path{Root: "x", Symbol: 1}
	if p1.Root != p2.Root || p1.Symbol != p2.Symbol {
		t.Error("expected equal paths")
	}
}

func TestConstraintPath_Segments(t *testing.T) {
	p := constraint.Path{
		Root:   "obj",
		Symbol: 1,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "field"},
		},
	}
	if len(p.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Name != "field" {
		t.Errorf("expected 'field', got '%s'", p.Segments[0].Name)
	}
}

package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// TestAddLinearEdge tests linear edge creation.
func TestAddLinearEdge(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Current = b.Cfg.Entry()
	b.CurrentLive = true

	next := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")
	b.AddLinearEdge(next)

	// Should have edge from entry to next
	succs := b.Cfg.Successors(b.Cfg.Entry())
	found := false
	for _, s := range succs {
		if s == next {
			found = true

			break
		}
	}

	if !found {
		t.Error("Should have edge from entry to next")
	}

	// Current should be updated
	if b.Current != next {
		t.Error("Current should be updated to next")
	}
}

// TestAddLinearEdge_DeadCode tests that dead code doesn't create edges.
func TestAddLinearEdge_DeadCode(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Current = b.Cfg.Entry()
	b.CurrentLive = false

	next := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")
	edgesBefore := len(b.Cfg.Edges())
	b.AddLinearEdge(next)

	// Should not add edge when not live
	if len(b.Cfg.Edges()) != edgesBefore {
		t.Error("Should not add edge when CurrentLive is false")
	}

	// Current should still be updated
	if b.Current != next {
		t.Error("Current should still be updated even when not live")
	}
}

// TestAddConditionEdges_Simple tests simple condition branching.
func TestAddConditionEdges_Simple(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	thenTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	elseTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")

	branch := b.AddConditionEdges(&ast.IdentExpr{Value: "x"}, thenTarget, elseTarget)

	// Branch should have two successors
	succs := b.Cfg.Successors(branch)
	if len(succs) != 2 {
		t.Errorf("Branch should have 2 successors, got %d", len(succs))
	}

	// Check edge conditions
	thenCond, thenOK := b.Cfg.EdgeCond(branch, thenTarget)
	elseCond, elseOK := b.Cfg.EdgeCond(branch, elseTarget)

	if !thenOK || !thenCond {
		t.Error("Then edge should have condition true")
	}
	if !elseOK || elseCond {
		t.Error("Else edge should have condition false")
	}
}

// TestAddConditionEdges_Nil tests nil condition.
func TestAddConditionEdges_Nil(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	thenTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	elseTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")

	branch := b.AddConditionEdges(nil, thenTarget, elseTarget)

	// Should still create branch with edges
	succs := b.Cfg.Successors(branch)
	if len(succs) != 2 {
		t.Errorf("Branch should have 2 successors, got %d", len(succs))
	}
}

// TestAddConditionEdges_And tests logical and expression.
func TestAddConditionEdges_And(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	thenTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	elseTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")

	expr := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      &ast.IdentExpr{Value: "a"},
		Rhs:      &ast.IdentExpr{Value: "b"},
	}

	branch := b.AddConditionEdges(expr, thenTarget, elseTarget)

	// Should create multiple branches for short-circuit evaluation
	branchCount := 0
	for _, n := range b.Cfg.Nodes {
		if n.Kind == basecfg.NodeBranch {
			branchCount++
		}
	}

	if branchCount < 2 {
		t.Errorf("Logical 'and' should create at least 2 branches, got %d", branchCount)
	}

	// First branch (lhs) should be the returned one
	if b.Cfg.Node(branch).Kind != basecfg.NodeBranch {
		t.Error("Returned point should be a branch")
	}
}

// TestAddConditionEdges_Or tests logical or expression.
func TestAddConditionEdges_Or(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	thenTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	elseTarget := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")

	expr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      &ast.IdentExpr{Value: "a"},
		Rhs:      &ast.IdentExpr{Value: "b"},
	}

	b.AddConditionEdges(expr, thenTarget, elseTarget)

	branchCount := 0
	for _, n := range b.Cfg.Nodes {
		if n.Kind == basecfg.NodeBranch {
			branchCount++
		}
	}

	if branchCount < 2 {
		t.Errorf("Logical 'or' should create at least 2 branches, got %d", branchCount)
	}
}

// TestAddCondBranch tests branch node creation.
func TestAddCondBranch(t *testing.T) {
	t.Parallel()

	b := NewBuilder()

	p := b.AddCondBranch(&ast.IdentExpr{Value: "cond"})

	node := b.Cfg.Node(p)
	if node.Kind != basecfg.NodeBranch {
		t.Error("Should create branch node")
	}

	info, ok := b.Info[p].(*BranchInfo)
	if !ok {
		t.Fatal("Should have BranchInfo")
	}
	if info.CondVar != "cond" {
		t.Errorf("CondVar should be 'cond', got %q", info.CondVar)
	}
	if info.CondCheck.Kind != basecfg.CheckTruthy {
		t.Errorf("CondCheck.Kind should be CheckTruthy, got %d", info.CondCheck.Kind)
	}
}

// TestResolvePendingGotos tests goto resolution.
func TestResolvePendingGotos(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	// Create a goto to a label that doesn't exist yet
	gotoPoint := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	b.Pending["target"] = []basecfg.Point{gotoPoint}

	// Create the label
	labelPoint := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	b.Labels["target"] = labelPoint

	// Resolve
	b.ResolvePendingGotos()

	// Should have edge from goto to label
	succs := b.Cfg.Successors(gotoPoint)
	found := false
	for _, s := range succs {
		if s == labelPoint {
			found = true

			break
		}
	}

	if !found {
		t.Error("Goto should be resolved to label")
	}
}

// TestProcessExprs tests expression processing.
func TestProcessExprs(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")

	exprs := []ast.Expr{
		&ast.IdentExpr{Value: "x"},
		&ast.NumberExpr{Value: "42"},
		&ast.IdentExpr{Value: "y"},
	}

	names := b.ProcessExprs(p, exprs)

	if len(names) != 3 {
		t.Fatalf("Expected 3 names, got %d", len(names))
	}
	if names[0] != "x" {
		t.Errorf("names[0] should be 'x', got %q", names[0])
	}
	if names[1] != "" {
		t.Errorf("names[1] should be empty for non-ident, got %q", names[1])
	}
	if names[2] != "y" {
		t.Errorf("names[2] should be 'y', got %q", names[2])
	}
}

// TestProcessExprs_Empty tests empty expression list.
func TestProcessExprs_Empty(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")

	names := b.ProcessExprs(p, nil)
	if names != nil {
		t.Error("Empty exprs should return nil")
	}

	names = b.ProcessExprs(p, []ast.Expr{})
	if names != nil {
		t.Error("Empty exprs should return nil")
	}
}

// TestScanExprForFuncsWithContext tests nested function detection.
func TestScanExprForFuncsWithContext(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")

	// Nested function in expression
	expr := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   []ast.Stmt{},
	}

	b.scanExprForFuncsWithContext(p, expr, nil, "")

	if len(b.Nested) != 1 {
		t.Errorf("Should find 1 nested function, got %d", len(b.Nested))
	}
	if b.Nested[0].Point != p {
		t.Error("Nested function point should match")
	}
}

// TestScanExprForFuncsWithContext_InTable tests function in table literal.
func TestScanExprForFuncsWithContext_InTable(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")

	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key: &ast.StringExpr{Value: "fn"},
				Value: &ast.FunctionExpr{
					ParList: &ast.ParList{},
					Stmts:   []ast.Stmt{},
				},
			},
		},
	}

	b.scanExprForFuncsWithContext(p, expr, nil, "")

	if len(b.Nested) != 1 {
		t.Errorf("Should find nested function in table, got %d", len(b.Nested))
	}
}

// TestScanExprForFuncsWithContext_InCall tests function in call arguments.
func TestScanExprForFuncsWithContext_InCall(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")

	expr := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "map"},
		Args: []ast.Expr{
			&ast.FunctionExpr{
				ParList: &ast.ParList{Names: []string{"x"}},
				Stmts:   []ast.Stmt{},
			},
		},
	}

	b.scanExprForFuncsWithContext(p, expr, nil, "")

	if len(b.Nested) != 1 {
		t.Errorf("Should find nested function in call args, got %d", len(b.Nested))
	}
}

// TestScanExprForFuncsWithContext_Nil tests nil handling.
func TestScanExprForFuncsWithContext_Nil(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")

	b.scanExprForFuncsWithContext(p, nil, nil, "")

	if len(b.Nested) != 0 {
		t.Error("Nil expr should not add nested functions")
	}
}

// TestGraph_GlobalsWithNameOf tests that seeded globals have correct names via NameOf.
func TestGraph_GlobalsWithNameOf(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   []ast.Stmt{},
	}

	globals := []string{"process", "print", "registry"}
	graph := Build(fn, globals...)
	if graph == nil {
		t.Fatal("graph should not be nil")
	}

	entry := graph.Entry()
	symbols := graph.AllSymbolsAt(entry)

	// All globals should be visible at entry
	for _, name := range globals {
		sym, ok := symbols[name]
		if !ok {
			t.Errorf("global %q should be visible at entry", name)

			continue
		}

		// NameOf should return the original name
		gotName := graph.NameOf(sym)
		if gotName != name {
			t.Errorf("NameOf(%d) = %q, want %q", sym, gotName, name)
		}
	}
}

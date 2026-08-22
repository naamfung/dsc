package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// TestBuild_NilFunction tests building with nil input.
func TestBuild_NilFunction(t *testing.T) {
	t.Parallel()

	g := Build(nil)
	if g != nil {
		t.Error("Build(nil) should return nil")
	}
}

// TestBuild_EmptyFunction tests building an empty function.
func TestBuild_EmptyFunction(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   []ast.Stmt{},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil for valid function")
	}

	if g.CFG() == nil {
		t.Error("Graph should have CFG")
	}
	if g.Size() < 2 {
		t.Error("Graph should have at least entry and exit nodes")
	}
	// Entry() == 0 is valid (first node), Exit() == 1 is valid (second node)
	if g.Node(g.Entry()) == nil || g.Node(g.Entry()).Kind != basecfg.NodeEntry {
		t.Error("Entry should point to an entry node")
	}
	if g.Node(g.Exit()) == nil || g.Node(g.Exit()).Kind != basecfg.NodeExit {
		t.Error("Exit should point to an exit node")
	}
}

func TestEachCallSite(t *testing.T) {
	t.Parallel()

	callF := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	callG := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "g"}}
	callH := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "h"}}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{
				Expr: callF,
			},
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{
					callG,
				},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					callH,
				},
			},
		},
	}

	g := Build(fn, "f", "g", "h")
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	var names []string

	g.EachCallSite(func(_ Point, info *CallInfo) {
		if info == nil {
			return
		}
		if info.CalleeName != "" {
			names = append(names, info.CalleeName)
		}
	})

	if len(names) != 3 {
		t.Fatalf("got %d call sites, want 3", len(names))
	}
	if names[0] != "f" || names[1] != "g" || names[2] != "h" {
		t.Fatalf("call order = %v, want [f g h]", names)
	}
}

func TestEachStmtCall(t *testing.T) {
	t.Parallel()

	callF := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	callG := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "g"}}
	callH := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "h"}}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{Expr: callF},
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{callG},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{callH},
			},
		},
	}

	g := Build(fn, "f", "g", "h")
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	var names []string
	g.EachStmtCall(func(_ Point, info *CallInfo) {
		if info == nil {
			return
		}
		if info.CalleeName != "" {
			names = append(names, info.CalleeName)
		}
	})

	if len(names) != 1 {
		t.Fatalf("got %d stmt calls, want 1", len(names))
	}
	if names[0] != "f" {
		t.Fatalf("stmt call order = %v, want [f]", names)
	}
}

func TestGraph_CallSitesAtAndCallSiteAt(t *testing.T) {
	t.Parallel()

	callF := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	callG := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "g"}}
	callH := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "h"}}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{Expr: callF},
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{callG},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{callH},
			},
		},
	}

	g := Build(fn, "f", "g", "h")
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	var callPoint Point
	var assignPoint Point
	var returnPoint Point

	g.EachCall(func(p Point, info *CallInfo) {
		if info != nil && info.Call == callF {
			callPoint = p
		}
	})
	g.EachAssign(func(p Point, _ *AssignInfo) {
		assignPoint = p
	})
	g.EachReturn(func(p Point, _ *ReturnInfo) {
		returnPoint = p
	})

	if got := g.CallSiteAt(callPoint, callF); got == nil || got.CalleeName != "f" {
		t.Fatalf("CallSiteAt direct call mismatch: got %+v", got)
	}
	if got := g.CallSiteAt(assignPoint, callG); got == nil || got.CalleeName != "g" {
		t.Fatalf("CallSiteAt assign source mismatch: got %+v", got)
	}
	if got := g.CallSiteAt(returnPoint, callH); got == nil || got.CalleeName != "h" {
		t.Fatalf("CallSiteAt return source mismatch: got %+v", got)
	}
	if got := g.CallSiteAt(assignPoint, callF); got != nil {
		t.Fatalf("CallSiteAt should not match unrelated call expression, got %+v", got)
	}

	directSites := g.CallSitesAt(callPoint)
	if len(directSites) != 1 || directSites[0].Call != callF {
		t.Fatalf("CallSitesAt direct call mismatch: %+v", directSites)
	}
	assignSites := g.CallSitesAt(assignPoint)
	if len(assignSites) != 1 || assignSites[0].Call != callG {
		t.Fatalf("CallSitesAt assign call mismatch: %+v", assignSites)
	}
	returnSites := g.CallSitesAt(returnPoint)
	if len(returnSites) != 1 || returnSites[0].Call != callH {
		t.Fatalf("CallSitesAt return call mismatch: %+v", returnSites)
	}
}

// TestBuild_WithParams tests parameter handling.
func TestBuild_WithParams(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
		},
		Stmts: []ast.Stmt{},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Check that parameters create assign nodes
	assignCount := 0

	g.EachAssign(func(_ Point, info *AssignInfo) {
		if info.IsLocal {
			assignCount++
		}
	})

	if assignCount != 2 {
		t.Errorf("Expected 2 parameter assigns, got %d", assignCount)
	}
}

// TestBuild_SSAVersions tests SSA version computation.
func TestBuild_SSAVersions(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// x should have SSA versions
	foundVersions := false
	for _, p := range g.RPO() {
		if sym, ok := g.SymbolAt(p, "x"); ok {
			if ver := g.VisibleVersion(p, sym); !ver.IsZero() {
				foundVersions = true

				break
			}
		}
	}

	if !foundVersions {
		t.Error("SSA versions should be computed for x")
	}
}

// TestBuild_PhiNodes tests phi node creation at join points.
func TestBuild_PhiNodes(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "cond"},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
				Else: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
					},
				},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	phis := g.PhiNodes()
	if len(phis) == 0 {
		t.Error("Should create phi node at join point")
	}

	foundXPhi := false

	for _, phi := range phis {
		if phi.Target.Root == "x" {
			foundXPhi = true

			if len(phi.Operands) < 2 {
				t.Errorf("Phi for x should have at least 2 operands, got %d", len(phi.Operands))
			}
		}
	}
	if !foundXPhi {
		t.Error("Should have phi node for x")
	}
}

// TestBuild_AllVisibleVersions ensures SSA visibility map is populated.
func TestBuild_AllVisibleVersions(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "cond"},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	var found bool
	for _, p := range g.RPO() {
		vis := g.AllVisibleVersions(p)
		if len(vis) == 0 {
			continue
		}
		if sym, ok := g.SymbolAt(p, "x"); ok {
			if ver := vis[sym]; !ver.IsZero() {
				found = true

				break
			}
		}
	}

	if !found {
		t.Error("AllVisibleVersions should include x at some point")
	}
}

// TestBuild_ScopeTracking tests scope visibility tracking.
func TestBuild_ScopeTracking(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"outer"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.DoBlockStmt{
				Stmts: []ast.Stmt{
					&ast.LocalAssignStmt{
						Names: []string{"inner"},
						Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
					},
				},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	if !g.HasScopeTracking() {
		t.Error("Graph should have scope tracking")
	}

	// Find scope exit - inner should not be visible there
	foundInnerAtExit := false
	for _, p := range g.RPO() {
		node := g.Node(p)
		if node != nil && node.Kind == basecfg.NodeScopeExit {
			if _, ok := g.SymbolAt(p, "inner"); ok {
				foundInnerAtExit = true
			}
		}
	}
	// After the do block's scope exit, inner should not be visible
	if foundInnerAtExit {
		t.Error("'inner' should not be visible after scope exit")
	}
}

// TestBuildBlock tests building from statement block.
func TestBuildBlock(t *testing.T) {
	t.Parallel()

	stmts := []ast.Stmt{
		&ast.LocalAssignStmt{
			Names: []string{"x"},
			Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
		},
		&ast.ReturnStmt{
			Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		},
	}

	g := BuildBlock(stmts)
	if g == nil {
		t.Fatal("BuildBlock should return graph")
	}

	// Should have assign and return
	hasAssign := false
	hasReturn := false

	g.EachAssign(func(_ Point, _ *AssignInfo) {
		hasAssign = true
	})
	g.EachReturn(func(_ Point, _ *ReturnInfo) {
		hasReturn = true
	})

	if !hasAssign {
		t.Error("Should have assign node")
	}
	if !hasReturn {
		t.Error("Should have return node")
	}
}

// TestGraph_NodeAccessors tests Graph node type accessors.
func TestGraph_NodeAccessors(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "print"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				},
			},
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "x"},
				Then:      []ast.Stmt{},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Test typed accessors
	foundAssign := false
	foundCall := false
	foundBranch := false
	foundReturn := false

	for _, p := range g.RPO() {
		if g.Assign(p) != nil {
			foundAssign = true
		}
		if g.Call(p) != nil {
			foundCall = true
		}
		if g.Branch(p) != nil {
			foundBranch = true
		}
		if g.Return(p) != nil {
			foundReturn = true
		}
	}

	if !foundAssign {
		t.Error("Assign accessor should find assign node")
	}
	if !foundCall {
		t.Error("Call accessor should find call node")
	}
	if !foundBranch {
		t.Error("Branch accessor should find branch node")
	}
	if !foundReturn {
		t.Error("Return accessor should find return node")
	}
}

// TestGraph_CFGMethods tests delegated CFG methods.
func TestGraph_CFGMethods(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Test Size
	if g.Size() < 3 {
		t.Errorf("Size should be at least 3 (entry, assign, exit), got %d", g.Size())
	}

	// Test RPO
	rpo := g.RPO()
	if len(rpo) < 3 {
		t.Errorf("RPO should have at least 3 nodes, got %d", len(rpo))
	}

	// Test Entry/Exit
	if g.Entry() != rpo[0] {
		t.Error("Entry should be first in RPO")
	}

	// Test Successors/Predecessors
	succs := g.Successors(g.Entry())
	if len(succs) == 0 {
		t.Error("Entry should have successors")
	}

	// Test ID
	if g.ID() == 0 {
		t.Error("Graph should have non-zero ID")
	}
}

// TestGraph_NestedFunctions tests nested function tracking.
func TestGraph_NestedFunctions(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"fn"},
				Exprs: []ast.Expr{
					&ast.FunctionExpr{
						ParList: &ast.ParList{Names: []string{"a"}},
						Stmts:   []ast.Stmt{},
					},
				},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	nested := g.NestedFunctions()
	if len(nested) != 1 {
		t.Errorf("Expected 1 nested function, got %d", len(nested))
	}
}

// TestGraph_PopulateSymbols tests symbol population.
func TestGraph_PopulateSymbols(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Create resolver that returns a fixed symbol
	var resolverCalled bool
	resolver := func(_ Point, name string) basecfg.SymbolID {
		if name == "x" {
			resolverCalled = true

			return 42
		}

		return 0
	}

	g.PopulateSymbols(resolver)

	if !resolverCalled {
		t.Error("Resolver should have been called")
	}
}

// TestGraph_DeclarationPoint tests declaration point lookup.
func TestGraph_DeclarationPoint(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts:   []ast.Stmt{},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Find the symbol for x
	var xSymbol basecfg.SymbolID
	for _, p := range g.RPO() {
		if sym, ok := g.SymbolAt(p, "x"); ok && sym != 0 {
			xSymbol = sym

			break
		}
	}

	if xSymbol == 0 {
		t.Fatal("Should find symbol for x")
	}

	// Get declaration point
	declPoint, ok := g.DeclarationPoint(xSymbol)
	if !ok {
		t.Error("Should find declaration point for x")
	}
	if declPoint == 0 {
		t.Error("Declaration point should not be zero")
	}
}

// TestGraph_ParamSymbols tests that param symbols are precomputed correctly.
func TestGraph_ParamSymbols(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x", "y", "z"}},
		Stmts:   []ast.Stmt{},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	names := g.ParamNames()
	symbols := g.ParamSymbols()
	declPoints := g.ParamDeclPoints()

	if len(names) != 3 {
		t.Errorf("ParamNames() len = %d, want 3", len(names))
	}
	if len(symbols) != 3 {
		t.Errorf("ParamSymbols() len = %d, want 3", len(symbols))
	}
	if len(declPoints) != 3 {
		t.Errorf("ParamDeclPoints() len = %d, want 3", len(declPoints))
	}

	// Check names match
	expected := []string{"x", "y", "z"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("ParamNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}

	// Check all symbols are non-zero and unique
	seen := make(map[basecfg.SymbolID]bool)
	for i, sym := range symbols {
		if sym == 0 {
			t.Errorf("ParamSymbols()[%d] = 0, want non-zero", i)
		}
		if seen[sym] {
			t.Errorf("ParamSymbols()[%d] = %d is duplicate", i, sym)
		}
		seen[sym] = true
	}

	// Check declaration points exist and align with ParamDeclPoints
	for i, sym := range symbols {
		declPoint, ok := g.DeclarationPoint(sym)
		if !ok {
			t.Errorf("DeclarationPoint for param %d not found", i)
		}
		if declPoint == 0 {
			t.Errorf("DeclarationPoint for param %d is zero", i)
		}
		if declPoint != declPoints[i] {
			t.Errorf("DeclarationPoint(%d) = %d, want %d (from ParamDeclPoints)", sym, declPoint, declPoints[i])
		}
	}

	// Check all decl points are non-zero and unique
	seenPoints := make(map[Point]bool)
	for i, p := range declPoints {
		if p == 0 {
			t.Errorf("ParamDeclPoints()[%d] = 0, want non-zero", i)
		}
		if seenPoints[p] {
			t.Errorf("ParamDeclPoints()[%d] = %d is duplicate", i, p)
		}
		seenPoints[p] = true
	}
}

// TestGraph_ParamSymbols_NoParams tests param symbols for functions with no parameters.
func TestGraph_ParamSymbols_NoParams(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: nil,
		Stmts:   []ast.Stmt{},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	if names := g.ParamNames(); names != nil {
		t.Errorf("ParamNames() = %v, want nil", names)
	}
	if symbols := g.ParamSymbols(); symbols != nil {
		t.Errorf("ParamSymbols() = %v, want nil", symbols)
	}
	if declPoints := g.ParamDeclPoints(); declPoints != nil {
		t.Errorf("ParamDeclPoints() = %v, want nil", declPoints)
	}
}

func TestGraph_ParamSlots_ImplicitSelfMethod(t *testing.T) {
	t.Parallel()

	source := `
local Runner = {}
function Runner:run(options: number?)
	return nil
end
`
	stmts, err := parse.ParseString(source, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	root := BuildBlock(stmts)
	if root == nil {
		t.Fatal("BuildBlock should return graph")
	}

	var methodGraph *Graph
	root.EachFuncDef(func(_ Point, info *FuncDefInfo) {
		if methodGraph != nil || info == nil || !info.IsMethod || info.FuncExpr == nil {
			return
		}
		methodGraph = BuildWithBindings(info.FuncExpr, root.Bindings())
	})
	if methodGraph == nil {
		t.Fatal("expected to find method graph")
	}

	slots := methodGraph.ParamSlots()
	if len(slots) != 2 {
		t.Fatalf("expected 2 param slots, got %d", len(slots))
	}
	if !slots[0].IsImplicitSelf || slots[0].SourceIndex != -1 || slots[0].Name != "self" {
		t.Fatalf("slot[0] should be implicit self, got %+v", slots[0])
	}
	if slots[1].IsImplicitSelf || slots[1].SourceIndex != 0 || slots[1].Name != "options" {
		t.Fatalf("slot[1] should map to explicit options param, got %+v", slots[1])
	}
	if slots[1].TypeAnnotation == nil {
		t.Fatal("slot[1] should carry source type annotation")
	}
}

// TestGraph_NameOf tests the NameOf method for symbol name lookup.
func TestGraph_NameOf(t *testing.T) {
	t.Parallel()

	// Test with parameters
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x", "y"}},
		Stmts:   []ast.Stmt{},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	symbols := g.ParamSymbols()
	if len(symbols) != 2 {
		t.Fatalf("ParamSymbols() len = %d, want 2", len(symbols))
	}

	// Verify param names via NameOf
	if name := g.NameOf(symbols[0]); name != "x" {
		t.Errorf("NameOf(param 0) = %q, want %q", name, "x")
	}
	if name := g.NameOf(symbols[1]); name != "y" {
		t.Errorf("NameOf(param 1) = %q, want %q", name, "y")
	}

	// Unknown symbol returns empty
	if name := g.NameOf(9999); name != "" {
		t.Errorf("NameOf(9999) = %q, want empty", name)
	}
}

// TestGraph_NameOf_Locals tests NameOf for local variables.
func TestGraph_NameOf_Locals(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: nil,
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"foo", "bar"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.NumberExpr{Value: "2"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Find symbols for foo and bar
	var fooSym, barSym basecfg.SymbolID
	for _, p := range g.RPO() {
		if sym, ok := g.SymbolAt(p, "foo"); ok && sym != 0 {
			fooSym = sym
		}
		if sym, ok := g.SymbolAt(p, "bar"); ok && sym != 0 {
			barSym = sym
		}
	}

	if fooSym == 0 {
		t.Fatal("Should find symbol for foo")
	}
	if barSym == 0 {
		t.Fatal("Should find symbol for bar")
	}

	if name := g.NameOf(fooSym); name != "foo" {
		t.Errorf("NameOf(fooSym) = %q, want %q", name, "foo")
	}
	if name := g.NameOf(barSym); name != "bar" {
		t.Errorf("NameOf(barSym) = %q, want %q", name, "bar")
	}
}

// TestGraph_NameOf_Globals tests NameOf for seeded globals.
func TestGraph_NameOf_Globals(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: nil,
		Stmts:   []ast.Stmt{},
	}

	g := Build(fn, "print", "error", "assert")
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Look up seeded global symbols
	entry := g.Entry()
	printSym, ok := g.SymbolAt(entry, "print")
	if !ok {
		t.Fatal("Should find symbol for print")
	}
	errorSym, ok := g.SymbolAt(entry, "error")
	if !ok {
		t.Fatal("Should find symbol for error")
	}
	assertSym, ok := g.SymbolAt(entry, "assert")
	if !ok {
		t.Fatal("Should find symbol for assert")
	}

	if name := g.NameOf(printSym); name != "print" {
		t.Errorf("NameOf(printSym) = %q, want %q", name, "print")
	}
	if name := g.NameOf(errorSym); name != "error" {
		t.Errorf("NameOf(errorSym) = %q, want %q", name, "error")
	}
	if name := g.NameOf(assertSym); name != "assert" {
		t.Errorf("NameOf(assertSym) = %q, want %q", name, "assert")
	}
}

// TestGraph_SymbolKind tests classification of symbols as Param, Local, or Global.
func TestGraph_SymbolKind(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"param1", "param2"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"localVar"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IdentExpr{Value: "globalVar"}},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Check parameter symbols are basecfg.SymbolParam
	paramSymbols := g.ParamSymbols()
	if len(paramSymbols) != 2 {
		t.Fatalf("ParamSymbols() len = %d, want 2", len(paramSymbols))
	}
	for i, sym := range paramSymbols {
		kind, ok := g.SymbolKind(sym)
		if !ok {
			t.Errorf("basecfg.SymbolKind(param %d) not found", i)

			continue
		}

		if kind != basecfg.SymbolParam {
			t.Errorf("basecfg.SymbolKind(param %d) = %d, want basecfg.SymbolParam (%d)", i, kind, basecfg.SymbolParam)
		}
	}

	// Find localVar symbol and check it's basecfg.SymbolLocal
	var localSym basecfg.SymbolID
	for _, p := range g.RPO() {
		if sym, ok := g.SymbolAt(p, "localVar"); ok && sym != 0 {
			localSym = sym

			break
		}
	}
	if localSym == 0 {
		t.Fatal("Should find symbol for localVar")
	}
	kind, ok := g.SymbolKind(localSym)
	if !ok {
		t.Error("basecfg.SymbolKind(localVar) not found")
	} else if kind != basecfg.SymbolLocal {
		t.Errorf("basecfg.SymbolKind(localVar) = %d, want basecfg.SymbolLocal (%d)", kind, basecfg.SymbolLocal)
	}

	// Find globalVar symbol and check it's basecfg.SymbolGlobal
	var globalSym basecfg.SymbolID
	for _, p := range g.RPO() {
		if sym, ok := g.SymbolAt(p, "globalVar"); ok && sym != 0 {
			globalSym = sym

			break
		}
	}
	if globalSym == 0 {
		t.Fatal("Should find symbol for globalVar")
	}
	kind, ok = g.SymbolKind(globalSym)
	if !ok {
		t.Error("basecfg.SymbolKind(globalVar) not found")
	} else if kind != basecfg.SymbolGlobal {
		t.Errorf("basecfg.SymbolKind(globalVar) = %d, want basecfg.SymbolGlobal (%d)", kind, basecfg.SymbolGlobal)
	}

	// Unknown symbol returns (basecfg.SymbolUnknown, false)
	kind, ok = g.SymbolKind(99999)
	if ok {
		t.Error("basecfg.SymbolKind(99999) should return false")
	}
	if kind != basecfg.SymbolUnknown {
		t.Errorf("basecfg.SymbolKind(99999) = %d, want basecfg.SymbolUnknown (%d)", kind, basecfg.SymbolUnknown)
	}
}

// TestGraph_SymbolKind_SeededGlobals tests that seeded globals are classified correctly.
func TestGraph_SymbolKind_SeededGlobals(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: nil,
		Stmts:   []ast.Stmt{},
	}

	globals := []string{"print", "pairs", "error"}
	g := Build(fn, globals...)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	for _, name := range globals {
		sym, ok := g.SymbolAt(g.Entry(), name)
		if !ok {
			t.Errorf("SymbolAt(entry, %q) not found", name)

			continue
		}

		kind, ok := g.SymbolKind(sym)
		if !ok {
			t.Errorf("basecfg.SymbolKind(%q) not found", name)

			continue
		}

		if kind != basecfg.SymbolGlobal {
			t.Errorf("basecfg.SymbolKind(%q) = %d, want basecfg.SymbolGlobal (%d)", name, kind, basecfg.SymbolGlobal)
		}
	}
}

// TestGraph_NilHandling tests that nil graph doesn't panic.
func TestGraph_NilHandling(t *testing.T) {
	t.Parallel()

	var g *Graph

	// All methods should handle nil gracefully
	if g.CFG() != nil {
		t.Error("nil.CFG() should return nil")
	}
	if g.Info(1) != nil {
		t.Error("nil.Info() should return nil")
	}
	if g.Entry() != 0 {
		t.Error("nil.Entry() should return 0")
	}
	if g.Exit() != 0 {
		t.Error("nil.Exit() should return 0")
	}
	if g.Size() != 0 {
		t.Error("nil.Size() should return 0")
	}
	if g.RPO() != nil {
		t.Error("nil.RPO() should return nil")
	}
	if g.PhiNodes() != nil {
		t.Error("nil.PhiNodes() should return nil")
	}
	if g.HasScopeTracking() {
		t.Error("nil.HasScopeTracking() should return false")
	}
	if kind, ok := g.SymbolKind(1); ok || kind != basecfg.SymbolUnknown {
		t.Error("nil.SymbolKind() should return (basecfg.SymbolUnknown, false)")
	}

	// Iteration methods should not panic
	g.EachAssign(func(_ Point, _ *AssignInfo) {
		t.Error("Should not iterate on nil")
	})
	g.EachCall(func(_ Point, _ *CallInfo) {
		t.Error("Should not iterate on nil")
	})
}

// TestGraph_Determinism tests that Build produces deterministic results.
func TestGraph_Determinism(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"a", "b"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x", "y"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.NumberExpr{Value: "2"}},
			},
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "a"},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "10"}},
					},
				},
				Else: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "y"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "20"}},
					},
				},
			},
		},
	}

	// Build first graph
	g1 := Build(fn)
	rpo1 := g1.RPO()
	phis1 := g1.PhiNodes()

	// Build multiple times and compare
	for i := range 10 {
		g := Build(fn)
		rpo := g.RPO()
		phis := g.PhiNodes()

		if len(rpo) != len(rpo1) {
			t.Errorf("Iteration %d: RPO length differs", i)
		}

		if len(phis) != len(phis1) {
			t.Errorf("Iteration %d: Phi count differs", i)
		}

		// Check phi determinism
		for j, phi := range phis {
			if j >= len(phis1) {
				break
			}
			if phi.Target.Root != phis1[j].Target.Root {
				t.Errorf("Iteration %d: Phi %d target differs", i, j)
			}
			if phi.Target.ID != phis1[j].Target.ID {
				t.Errorf("Iteration %d: Phi %d version ID differs", i, j)
			}
		}
	}
}

// TestBuild_WithGlobals tests that globals are seeded and visible.
func TestBuild_WithGlobals(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			// x = registry.get("key")
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				Rhs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: &ast.AttrGetExpr{
							Object: &ast.IdentExpr{Value: "registry"},
							Key:    &ast.StringExpr{Value: "get"},
						},
						Args: []ast.Expr{&ast.StringExpr{Value: "key"}},
					},
				},
			},
		},
	}

	globals := []string{"registry", "process"}
	g := Build(fn, globals...)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Globals should be visible at entry point
	registrySym, ok := g.SymbolAt(g.Entry(), "registry")
	if !ok {
		t.Error("registry should be visible at entry")
	}
	if registrySym == 0 {
		t.Error("registry should have non-zero basecfg.SymbolID")
	}

	processSym, ok := g.SymbolAt(g.Entry(), "process")
	if !ok {
		t.Error("process should be visible at entry")
	}
	if processSym == 0 {
		t.Error("process should have non-zero basecfg.SymbolID")
	}

	// Declaration point for globals should be 0
	declPoint, ok := g.DeclarationPoint(registrySym)
	if !ok {
		t.Error("DeclarationPoint should find registry")
	}
	if declPoint != 0 {
		t.Errorf("Global declaration point should be 0, got %d", declPoint)
	}
}

// TestBuild_GlobalsVisibleAtAllPoints tests globals are visible throughout CFG.
func TestBuild_GlobalsVisibleAtAllPoints(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "x"},
				Then: []ast.Stmt{
					&ast.LocalAssignStmt{
						Names: []string{"y"},
						Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
					},
				},
			},
		},
	}

	globals := []string{"print", "pairs"}
	g := Build(fn, globals...)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Check that globals are visible at all points
	for _, p := range g.RPO() {
		if _, ok := g.SymbolAt(p, "print"); !ok {
			t.Errorf("print should be visible at point %d", p)
		}
		if _, ok := g.SymbolAt(p, "pairs"); !ok {
			t.Errorf("pairs should be visible at point %d", p)
		}
	}
}

// TestBuild_GlobalsDeterminism tests that globals get consistent SymbolIDs.
func TestBuild_GlobalsDeterminism(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   []ast.Stmt{},
	}

	// Different orderings should produce same relative SymbolIDs
	globals1 := []string{"zebra", "apple", "mango"}
	globals2 := []string{"apple", "mango", "zebra"}

	g1 := Build(fn, globals1...)
	g2 := Build(fn, globals2...)

	// Get symbols at entry for both builds
	apple1, _ := g1.SymbolAt(g1.Entry(), "apple")
	mango1, _ := g1.SymbolAt(g1.Entry(), "mango")
	zebra1, _ := g1.SymbolAt(g1.Entry(), "zebra")

	apple2, _ := g2.SymbolAt(g2.Entry(), "apple")
	mango2, _ := g2.SymbolAt(g2.Entry(), "mango")
	zebra2, _ := g2.SymbolAt(g2.Entry(), "zebra")

	// Relative differences should be the same (since sorted internally)
	diff1AM := mango1 - apple1
	diff2AM := mango2 - apple2

	diff1MZ := zebra1 - mango1
	diff2MZ := zebra2 - mango2

	if diff1AM != diff2AM {
		t.Error("apple-mango difference should be consistent")
	}
	if diff1MZ != diff2MZ {
		t.Error("mango-zebra difference should be consistent")
	}
}

// TestBuildBlock_WithGlobals tests BuildBlock with globals.
func TestBuildBlock_WithGlobals(t *testing.T) {
	t.Parallel()

	stmts := []ast.Stmt{
		&ast.FuncCallStmt{
			Expr: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "print"},
				Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
			},
		},
	}

	globals := []string{"print"}
	g := BuildBlock(stmts, globals...)
	if g == nil {
		t.Fatal("BuildBlock should return graph")
	}

	printSym, ok := g.SymbolAt(g.Entry(), "print")
	if !ok {
		t.Error("print should be visible at entry")
	}
	if printSym == 0 {
		t.Error("print should have non-zero basecfg.SymbolID")
	}
}

// TestSSA_ParametersHaveVersions tests that function parameters have non-zero SSA versions.
func TestSSA_ParametersHaveVersions(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x", "y"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.IdentExpr{Value: "x"},
					&ast.IdentExpr{Value: "y"},
				},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Find the exit point where both params should be visible with versions
	exit := g.Exit()
	for _, pred := range g.Predecessors(exit) {
		xSym, xOk := g.SymbolAt(pred, "x")
		ySym, yOk := g.SymbolAt(pred, "y")

		if !xOk || !yOk {
			t.Fatal("x and y should be visible before exit")
		}

		xVer := g.VisibleVersion(pred, xSym)
		yVer := g.VisibleVersion(pred, ySym)

		if xVer.IsZero() {
			t.Errorf("parameter x should have non-zero version at point %d, got zero", pred)
		}
		if yVer.IsZero() {
			t.Errorf("parameter y should have non-zero version at point %d, got zero", pred)
		}
	}
}

// TestSSA_SeededGlobalsHaveVersions tests that seeded globals have non-zero SSA versions at entry.
func TestSSA_SeededGlobalsHaveVersions(t *testing.T) {
	t.Parallel()

	stmts := []ast.Stmt{
		&ast.FuncCallStmt{
			Expr: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "print"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "myGlobal"}},
			},
		},
	}

	globals := []string{"print", "myGlobal"}
	g := BuildBlock(stmts, globals...)
	if g == nil {
		t.Fatal("BuildBlock should return graph")
	}

	entry := g.Entry()

	printSym, printOk := g.SymbolAt(entry, "print")
	globalSym, globalOk := g.SymbolAt(entry, "myGlobal")

	if !printOk || printSym == 0 {
		t.Fatal("print should be visible at entry")
	}
	if !globalOk || globalSym == 0 {
		t.Fatal("myGlobal should be visible at entry")
	}

	printVer := g.VisibleVersion(entry, printSym)
	globalVer := g.VisibleVersion(entry, globalSym)

	if printVer.IsZero() {
		t.Error("seeded global 'print' should have non-zero version at entry")
	}
	if globalVer.IsZero() {
		t.Error("seeded global 'myGlobal' should have non-zero version at entry")
	}
}

// TestSSA_AllSymbolsAtEntryHaveVersions tests that all symbols visible at entry have versions.
func TestSSA_AllSymbolsAtEntryHaveVersions(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"a", "b", "c"}},
		Stmts:   []ast.Stmt{},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Get all symbols at the first point after entry where params are declared
	// Parameters are declared after entry, so we check after param declarations
	for _, p := range g.RPO() {
		allSyms := g.AllSymbolsAt(p)
		if len(allSyms) == 0 {
			continue
		}

		for name, sym := range allSyms {
			if sym == 0 {
				continue
			}
			ver := g.VisibleVersion(p, sym)
			if ver.IsZero() {
				t.Errorf("symbol %q (ID=%d) at point %d should have non-zero version", name, sym, p)
			}
		}
	}
}

// TestSSA_CapturedVariablesHaveVersions tests that captured variables from outer scope
// have non-zero SSA versions when seeded into nested function CFG.
func TestSSA_CapturedVariablesHaveVersions(t *testing.T) {
	t.Parallel()

	// Simulate a nested function that captures 'x' and 'y' from outer scope
	// by seeding them as globals
	innerFn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"z"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.ArithmeticOpExpr{
						Operator: "+",
						Lhs:      &ast.IdentExpr{Value: "x"},
						Rhs: &ast.ArithmeticOpExpr{
							Operator: "+",
							Lhs:      &ast.IdentExpr{Value: "y"},
							Rhs:      &ast.IdentExpr{Value: "z"},
						},
					},
				},
			},
		},
	}

	// Seed 'x' and 'y' as captured variables (like outer scope locals)
	capturedVars := []string{"x", "y"}
	g := Build(innerFn, capturedVars...)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	entry := g.Entry()

	// Check captured variables at entry
	xSym, xOk := g.SymbolAt(entry, "x")
	ySym, yOk := g.SymbolAt(entry, "y")

	if !xOk || xSym == 0 {
		t.Fatal("captured 'x' should be visible at entry")
	}
	if !yOk || ySym == 0 {
		t.Fatal("captured 'y' should be visible at entry")
	}

	xVer := g.VisibleVersion(entry, xSym)
	yVer := g.VisibleVersion(entry, ySym)

	if xVer.IsZero() {
		t.Error("captured 'x' should have non-zero version at entry")
	}
	if yVer.IsZero() {
		t.Error("captured 'y' should have non-zero version at entry")
	}

	// Also verify they're in AllSymbolsAt
	allSyms := g.AllSymbolsAt(entry)
	if _, ok := allSyms["x"]; !ok {
		t.Error("'x' should be in AllSymbolsAt(entry)")
	}
	if _, ok := allSyms["y"]; !ok {
		t.Error("'y' should be in AllSymbolsAt(entry)")
	}
}

// TestSSA_ParametersHaveVersionsAtAllPoints verifies that function parameters
// have non-zero SSA versions at every point where they're visible.
// This is critical for type synthesis during scope pass.
func TestSSA_ParametersHaveVersionsAtAllPoints(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
		},
		Stmts: []ast.Stmt{
			// Use parameters in expressions (don't reassign)
			&ast.LocalAssignStmt{
				Names: []string{"result"},
				Exprs: []ast.Expr{
					&ast.ArithmeticOpExpr{
						Operator: "+",
						Lhs:      &ast.IdentExpr{Value: "a"},
						Rhs:      &ast.IdentExpr{Value: "b"},
					},
				},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "result"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Parameters should be visible and have versions at all body points
	for _, p := range g.RPO() {
		// Skip entry/exit nodes
		node := g.Node(p)
		if node == nil {
			continue
		}
		if node.Kind == basecfg.NodeEntry || node.Kind == basecfg.NodeExit {
			continue
		}

		// Check parameter 'a'
		if aSym, ok := g.SymbolAt(p, "a"); ok {
			aVer := g.VisibleVersion(p, aSym)
			if aVer.IsZero() {
				t.Errorf("parameter 'a' should have non-zero version at point %d (node kind %d)", p, node.Kind)
			}
		}

		// Check parameter 'b'
		if bSym, ok := g.SymbolAt(p, "b"); ok {
			bVer := g.VisibleVersion(p, bSym)
			if bVer.IsZero() {
				t.Errorf("parameter 'b' should have non-zero version at point %d (node kind %d)", p, node.Kind)
			}
		}
	}
}

// TestSSA_VisibleSymbolsHaveVersions verifies that ALL visible symbols at a point
// have non-zero SSA versions - not just locally assigned ones.
func TestSSA_VisibleSymbolsHaveVersions(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"param"},
		},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"local_var"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			// Reference both param and local_var without reassigning
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.IdentExpr{Value: "param"},
					&ast.IdentExpr{Value: "local_var"},
				},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// At return point, both param and local_var should have versions
	var returnPoint Point

	g.EachReturn(func(p Point, _ *ReturnInfo) {
		returnPoint = p
	})

	if returnPoint == 0 {
		t.Fatal("should have return point")
	}

	// Check all visible symbols have versions
	allSyms := g.AllSymbolsAt(returnPoint)
	for name, sym := range allSyms {
		ver := g.VisibleVersion(returnPoint, sym)
		if ver.IsZero() {
			t.Errorf("visible symbol '%s' (sym=%d) should have non-zero version at return point", name, sym)
		}
	}
}

// TestSSA_Invariant_AllVisibleSymbolsHaveVersions is the definitive test for the CFG/SSA
// invariant: every visible symbol at every CFG point has a non-zero SSA version.
// This eliminates the need for scope fallback in type resolution.
func TestSSA_Invariant_AllVisibleSymbolsHaveVersions(t *testing.T) {
	t.Parallel()

	// Complex function with:
	// - multiple parameters
	// - locals that shadow parameters
	// - loops with reassignments
	// - conditionals with different assignments
	// - nested scopes
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x", "y", "z"}},
		Stmts: []ast.Stmt{
			// local a = x + y
			&ast.LocalAssignStmt{
				Names: []string{"a"},
				Exprs: []ast.Expr{
					&ast.ArithmeticOpExpr{
						Operator: "+",
						Lhs:      &ast.IdentExpr{Value: "x"},
						Rhs:      &ast.IdentExpr{Value: "y"},
					},
				},
			},
			// if z then ... else ... end
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "z"},
				Then: []ast.Stmt{
					// local b = a
					&ast.LocalAssignStmt{
						Names: []string{"b"},
						Exprs: []ast.Expr{&ast.IdentExpr{Value: "a"}},
					},
					// a = a + 1
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "a"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
				Else: []ast.Stmt{
					// a = a * 2
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "a"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
					},
				},
			},
			// while x > 0 do ... end
			&ast.WhileStmt{
				Condition: &ast.RelationalOpExpr{
					Operator: ">",
					Lhs:      &ast.IdentExpr{Value: "x"},
					Rhs:      &ast.NumberExpr{Value: "0"},
				},
				Stmts: []ast.Stmt{
					// x = x - 1
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
			},
			// return a
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "a"}},
			},
		},
	}

	// Seed globals to simulate captured variables and stdlib
	globals := []string{"print", "pairs", "captured_outer"}
	g := Build(fn, globals...)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Verify THE INVARIANT: every visible symbol at every point has a version
	violations := 0
	for _, p := range g.RPO() {
		allSyms := g.AllSymbolsAt(p)
		if allSyms == nil {
			continue
		}
		for name, sym := range allSyms {
			if sym == 0 {
				t.Errorf("point %d: symbol '%s' has zero basecfg.SymbolID", p, name)
				violations++

				continue
			}
			ver := g.VisibleVersion(p, sym)
			if ver.IsZero() {
				t.Errorf("INVARIANT VIOLATION: point %d, symbol '%s' (ID=%d) visible but has zero version", p, name, sym)
				violations++
			}
		}
	}

	if violations > 0 {
		t.Fatalf("CFG/SSA invariant violated %d times", violations)
	}
}

// TestSSA_Invariant_LoopVariablesHaveVersions verifies that loop control variables
// always have versions even at loop headers (join points).
func TestSSA_Invariant_LoopVariablesHaveVersions(t *testing.T) {
	t.Parallel()

	// for i = 1, 10 do ... end
	fn := &ast.FunctionExpr{
		ParList: nil,
		Stmts: []ast.Stmt{
			&ast.NumberForStmt{
				Name:  "i",
				Init:  &ast.NumberExpr{Value: "1"},
				Limit: &ast.NumberExpr{Value: "10"},
				Stmts: []ast.Stmt{
					&ast.FuncCallStmt{
						Expr: &ast.FuncCallExpr{
							Func: &ast.IdentExpr{Value: "print"},
							Args: []ast.Expr{&ast.IdentExpr{Value: "i"}},
						},
					},
				},
			},
		},
	}

	g := Build(fn, "print")
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Check that 'i' has a version wherever it's visible
	for _, p := range g.RPO() {
		if iSym, ok := g.SymbolAt(p, "i"); ok && iSym != 0 {
			ver := g.VisibleVersion(p, iSym)
			if ver.IsZero() {
				t.Errorf("loop variable 'i' at point %d visible but has zero version", p)
			}
		}
	}
}

// TestSSA_Invariant_GenericForVariablesHaveVersions verifies that generic for loop
// variables (k, v) have versions at all points where they're visible.
func TestSSA_Invariant_GenericForVariablesHaveVersions(t *testing.T) {
	t.Parallel()

	// for k, v in pairs(t) do ... end
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"t"}},
		Stmts: []ast.Stmt{
			&ast.GenericForStmt{
				Names: []string{"k", "v"},
				Exprs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: &ast.IdentExpr{Value: "pairs"},
						Args: []ast.Expr{&ast.IdentExpr{Value: "t"}},
					},
				},
				Stmts: []ast.Stmt{
					&ast.FuncCallStmt{
						Expr: &ast.FuncCallExpr{
							Func: &ast.IdentExpr{Value: "print"},
							Args: []ast.Expr{
								&ast.IdentExpr{Value: "k"},
								&ast.IdentExpr{Value: "v"},
							},
						},
					},
				},
			},
		},
	}

	g := Build(fn, "pairs", "print")
	if g == nil {
		t.Fatal("Build should return graph")
	}

	// Check that k, v, and t have versions wherever visible
	for _, p := range g.RPO() {
		for _, name := range []string{"k", "v", "t"} {
			if sym, ok := g.SymbolAt(p, name); ok && sym != 0 {
				ver := g.VisibleVersion(p, sym)
				if ver.IsZero() {
					t.Errorf("variable '%s' at point %d visible but has zero version", name, p)
				}
			}
		}
	}
}

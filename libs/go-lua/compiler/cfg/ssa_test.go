package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

func TestComputeSSAVersions_Empty(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}
	if len(g.PhiNodes()) != 0 {
		t.Errorf("Empty function should have no phi nodes, got %d", len(g.PhiNodes()))
	}
}

func TestComputeSSAVersions_SingleAssign(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}
	if len(g.PhiNodes()) != 0 {
		t.Errorf("Single assign should have no phi nodes, got %d", len(g.PhiNodes()))
	}
}

func TestComputeSSAVersions_IfBranch(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
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
		t.Fatal("Build should not return nil")
	}
	if len(g.PhiNodes()) == 0 {
		t.Error("If with assignments in both branches should have phi node")
	}
}

func TestComputeSSAVersions_WhileLoop(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"i"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.WhileStmt{
				Condition: &ast.TrueExpr{},
				Stmts: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "i"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}
	if len(g.PhiNodes()) == 0 {
		t.Error("While loop with assignment should have phi node")
	}
}

func TestComputeSSAVersions_MultipleVariables(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x", "y"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}, &ast.NumberExpr{Value: "0"}},
			},
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
				Else: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "y"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}
}

func TestCollectAssignedSymbols(t *testing.T) {
	b := NewBuilder()
	b.Cfg = basecfg.New()
	b.ScopeTracker = NewScopeTracker()

	syms := b.collectAssignedSymbols()
	if syms == nil {
		t.Error("collectAssignedSymbols should return empty map, not nil")
	}
	if len(syms) != 0 {
		t.Errorf("Empty builder should have no assigned symbols, got %d", len(syms))
	}
}

func TestCollectDefPoints(t *testing.T) {
	b := NewBuilder()
	b.Cfg = basecfg.New()
	b.ScopeTracker = NewScopeTracker()

	defPoints := b.collectDefPoints(map[basecfg.SymbolID]string{})
	if defPoints == nil {
		t.Error("collectDefPoints should return empty map, not nil")
	}
}

func TestPhiNodeOperands(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
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
		t.Fatal("Build should not return nil")
	}

	for _, phi := range g.PhiNodes() {
		if len(phi.Operands) < 2 {
			t.Errorf("Phi node should have at least 2 operands, got %d", len(phi.Operands))
		}
		for _, op := range phi.Operands {
			if op.From == 0 {
				t.Error("Phi operand should have valid From point")
			}
		}
	}
}

func TestVisibleVersions(t *testing.T) {
	fn := &ast.FunctionExpr{
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
		t.Fatal("Build should not return nil")
	}

	for _, p := range g.RPO() {
		versions := g.AllVisibleVersions(p)
		if len(versions) > 0 {
			return
		}
	}
	t.Error("Should have visible versions at some point")
}

func TestVisibleVersions_MapSharing(t *testing.T) {
	// Multiple statements with no new variables - should share VisibleVersion maps
	fn := &ast.FunctionExpr{
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
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "print"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				},
			},
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "print"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				},
			},
		},
	}
	g := Build(fn, "print")
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	// Collect all non-nil VisibleVersion maps
	var maps []map[basecfg.SymbolID]Version
	for _, p := range g.RPO() {
		if m := g.AllVisibleVersions(p); m != nil {
			maps = append(maps, m)
		}
	}

	// With sharing optimization, some maps should be identical (same pointer)
	if len(maps) < 2 {
		t.Skip("Not enough points with visible versions to test sharing")
	}

	// The call statements after the assignment should share the same map
	// since no new versions are pushed
	sharedCount := 0

	for i := 1; i < len(maps); i++ {
		// Check if maps[i] points to same underlying data as maps[i-1]
		// by checking if they have same length and same values
		if len(maps[i]) == len(maps[i-1]) {
			allSame := true
			for sym, ver := range maps[i] {
				if prevVer, ok := maps[i-1][sym]; !ok || prevVer != ver {
					allSame = false

					break
				}
			}
			if allSame {
				sharedCount++
			}
		}
	}

	if sharedCount == 0 {
		t.Log("No shared maps detected - optimization may not be triggering")
	}
}

func TestVisibleVersions_NoSharingAfterPush(t *testing.T) {
	// After an assignment, VisibleVersion should NOT be shared
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "3"}},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	// Each assignment creates a new version, so versions should differ
	var versions []Version
	for _, p := range g.RPO() {
		if m := g.AllVisibleVersions(p); m != nil {
			for _, ver := range m {
				if ver.Root == "x" && !ver.IsZero() {
					versions = append(versions, ver)
				}
			}
		}
	}

	// Should have different version IDs after each assignment
	seen := make(map[int]bool)
	for _, v := range versions {
		seen[v.ID] = true
	}
	if len(seen) < 2 {
		t.Errorf("Expected multiple different version IDs for x, got %d unique", len(seen))
	}
}

func TestVisibleVersions_StraightLineCode(t *testing.T) {
	// Straight-line code with no branching should work correctly
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"a", "b", "c"},
				Exprs: []ast.Expr{
					&ast.NumberExpr{Value: "1"},
					&ast.NumberExpr{Value: "2"},
					&ast.NumberExpr{Value: "3"},
				},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.IdentExpr{Value: "a"},
					&ast.IdentExpr{Value: "b"},
					&ast.IdentExpr{Value: "c"},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	// All three variables should have versions at the return point
	var returnPoint basecfg.Point

	g.EachReturn(func(p Point, _ *ReturnInfo) {
		returnPoint = p
	})

	versions := g.AllVisibleVersions(returnPoint)
	if versions == nil {
		t.Fatal("Return point should have visible versions")
	}

	// Check all three variables have non-zero versions
	for _, name := range []string{"a", "b", "c"} {
		found := false
		for _, ver := range versions {
			if ver.Root == name && !ver.IsZero() {
				found = true

				break
			}
		}
		if !found {
			t.Errorf("Variable %s should have a non-zero version at return point", name)
		}
	}
}

func TestVisibleVersions_NestedScopes(t *testing.T) {
	// Variables in nested scopes should have correct versions
	fn := &ast.FunctionExpr{
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
					&ast.FuncCallStmt{
						Expr: &ast.FuncCallExpr{
							Func: &ast.IdentExpr{Value: "print"},
							Args: []ast.Expr{
								&ast.IdentExpr{Value: "outer"},
								&ast.IdentExpr{Value: "inner"},
							},
						},
					},
				},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "outer"}},
			},
		},
	}
	g := Build(fn, "print")
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	// The function should complete without panics
	pointCount := 0
	for range g.RPO() {
		pointCount++
	}
	if pointCount == 0 {
		t.Error("Should have CFG points")
	}
}

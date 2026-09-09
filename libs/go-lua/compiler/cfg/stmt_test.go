package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

func TestStmts_Empty(_ *testing.T) {
	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	b.Stmts(nil)
	b.Stmts([]ast.Stmt{})
}

func TestStmt_LocalAssign(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x", "y"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.NumberExpr{Value: "2"}},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	assignCount := 0
	g.EachAssign(func(_ Point, _ *AssignInfo) {
		assignCount++
	})
	if assignCount == 0 {
		t.Error("Should have at least one assign")
	}
}

func TestStmt_Assign(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	assignCount := 0
	g.EachAssign(func(_ Point, _ *AssignInfo) {
		assignCount++
	})
	if assignCount < 2 {
		t.Error("Should have at least two assigns")
	}
}

func TestStmt_Return(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	returnCount := 0
	g.EachReturn(func(_ Point, _ *ReturnInfo) {
		returnCount++
	})
	if returnCount == 0 {
		t.Error("Should have at least one return")
	}
}

func TestStmt_Break(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.WhileStmt{
				Condition: &ast.TrueExpr{},
				Stmts: []ast.Stmt{
					&ast.BreakStmt{},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}
}

func TestStmt_If(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
					&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
				},
				Else: []ast.Stmt{
					&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}}},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	branchCount := 0
	g.EachBranch(func(_ Point, _ *BranchInfo) {
		branchCount++
	})
	if branchCount == 0 {
		t.Error("Should have at least one branch")
	}
}

func TestStmt_While(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.WhileStmt{
				Condition: &ast.TrueExpr{},
				Stmts: []ast.Stmt{
					&ast.BreakStmt{},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	branchCount := 0
	g.EachBranch(func(_ Point, _ *BranchInfo) {
		branchCount++
	})
	if branchCount == 0 {
		t.Error("While should have branch for condition")
	}
}

func TestStmt_Repeat(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.RepeatStmt{
				Condition: &ast.FalseExpr{},
				Stmts: []ast.Stmt{
					&ast.LocalAssignStmt{
						Names: []string{"x"},
						Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
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

func TestStmt_NumberFor(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.NumberForStmt{
				Name:  "i",
				Init:  &ast.NumberExpr{Value: "1"},
				Limit: &ast.NumberExpr{Value: "10"},
				Step:  &ast.NumberExpr{Value: "1"},
				Stmts: []ast.Stmt{},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}
}

func TestStmt_GenericFor(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.GenericForStmt{
				Names: []string{"k", "v"},
				Exprs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: &ast.IdentExpr{Value: "pairs"},
						Args: []ast.Expr{&ast.IdentExpr{Value: "t"}},
					},
				},
				Stmts: []ast.Stmt{},
			},
		},
	}
	g := Build(fn, "pairs", "t")
	if g == nil {
		t.Fatal("Build should not return nil")
	}
}

func TestStmt_Call(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "print"},
					Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
				},
			},
		},
	}
	g := Build(fn, "print")
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	callCount := 0
	g.EachCall(func(_ Point, info *CallInfo) {
		if info.CalleeName == "print" {
			callCount++
		}
	})
	if callCount == 0 {
		t.Error("Should have call to print")
	}
}

func TestStmt_FuncDef(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Func: &ast.IdentExpr{Value: "myFunc"},
				},
				Func: &ast.FunctionExpr{
					ParList: &ast.ParList{},
					Stmts:   []ast.Stmt{},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	funcDefCount := 0
	g.EachFuncDef(func(_ Point, info *FuncDefInfo) {
		if info.Name == "myFunc" {
			funcDefCount++
		}
	})
	if funcDefCount == 0 {
		t.Error("Should have FuncDefInfo for myFunc")
	}
}

func TestStmt_FuncDef_Method(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"T"},
				Exprs: []ast.Expr{&ast.TableExpr{}},
			},
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Receiver: &ast.IdentExpr{Value: "T"},
					Method:   "foo",
				},
				Func: &ast.FunctionExpr{
					ParList: &ast.ParList{},
					Stmts:   []ast.Stmt{},
				},
			},
		},
	}
	g := Build(fn)
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	found := false
	g.EachFuncDef(func(_ Point, info *FuncDefInfo) {
		if info.IsMethod {
			found = true
		}
	})
	if !found {
		t.Error("Should have method FuncDefInfo")
	}
}

func TestStmt_Label(t *testing.T) {
	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	stmt := &ast.LabelStmt{Name: "myLabel"}
	b.LabelStmt(stmt)

	if _, ok := b.Labels["myLabel"]; !ok {
		t.Error("Should register label")
	}
}

func TestStmt_Goto(t *testing.T) {
	b := NewBuilder()
	b.Current = b.Cfg.Entry()

	stmt := &ast.GotoStmt{Label: "myLabel"}
	b.GotoStmt(stmt)

	if len(b.Pending["myLabel"]) == 0 {
		t.Error("Should add to pending gotos")
	}
}

func TestStmt_ScopedBlock(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.DoBlockStmt{
				Stmts: []ast.Stmt{
					&ast.LocalAssignStmt{
						Names: []string{"x"},
						Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
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

func TestStmt_Dispatch(t *testing.T) {
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
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
		},
	}
	g := Build(fn, "print")
	if g == nil {
		t.Fatal("Build should not return nil")
	}

	// Should have assign, call, and return
	assignCount := 0
	g.EachAssign(func(_ Point, _ *AssignInfo) {
		assignCount++
	})

	callCount := 0
	g.EachCall(func(_ Point, _ *CallInfo) {
		callCount++
	})

	returnCount := 0
	g.EachReturn(func(_ Point, _ *ReturnInfo) {
		returnCount++
	})

	if assignCount == 0 {
		t.Error("Should have assigns")
	}
	if callCount == 0 {
		t.Error("Should have calls")
	}
	if returnCount == 0 {
		t.Error("Should have returns")
	}
}

// Unit test for Builder method directly.
func TestBuilder_LocalAssign_Direct(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	stmt := fn.Stmts[0].(*ast.LocalAssignStmt)
	b.LocalAssign(stmt)

	if len(b.Info) == 0 {
		t.Error("Should create node info")
	}
}

func TestBuilder_Assign_Direct(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	stmt := fn.Stmts[0].(*ast.AssignStmt)
	b.Assign(stmt)

	if len(b.Info) == 0 {
		t.Error("Should create node info")
	}
}

func TestBuilder_Break_Direct(t *testing.T) {
	b := NewBuilder()
	b.Current = b.Cfg.Entry()
	b.LoopExits = []basecfg.Point{5}

	stmt := &ast.BreakStmt{}
	b.BreakStmt(stmt)

	if b.CurrentLive {
		t.Error("Break should mark current as dead")
	}
}

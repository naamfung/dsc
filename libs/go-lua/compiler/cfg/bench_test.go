package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func BenchmarkBuild_Empty(b *testing.B) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}

	b.ResetTimer()

	for range b.N {
		Build(fn)
	}
}

func BenchmarkBuild_SimpleAssign(b *testing.B) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}

	b.ResetTimer()
	for range b.N {
		Build(fn)
	}
}

func BenchmarkBuild_MultipleAssigns(b *testing.B) {
	stmts := make([]ast.Stmt, 100)
	for i := range stmts {
		stmts[i] = &ast.LocalAssignStmt{
			Names: []string{"x"},
			Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
		}
	}

	fn := &ast.FunctionExpr{Stmts: stmts}

	b.ResetTimer()

	for range b.N {
		Build(fn)
	}
}

func BenchmarkBuild_NestedIf(b *testing.B) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
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
				Else: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "3"}},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for range b.N {
		Build(fn)
	}
}

func BenchmarkBuild_WhileLoop(b *testing.B) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"i"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}},
			},
			&ast.WhileStmt{
				Condition: &ast.RelationalOpExpr{
					Operator: "<",
					Lhs:      &ast.IdentExpr{Value: "i"},
					Rhs:      &ast.NumberExpr{Value: "10"},
				},
				Stmts: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "i"}},
						Rhs: []ast.Expr{
							&ast.ArithmeticOpExpr{
								Operator: "+",
								Lhs:      &ast.IdentExpr{Value: "i"},
								Rhs:      &ast.NumberExpr{Value: "1"},
							},
						},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for range b.N {
		Build(fn)
	}
}

func BenchmarkBuild_FunctionCalls(b *testing.B) {
	stmts := make([]ast.Stmt, 50)
	for i := range stmts {
		stmts[i] = &ast.FuncCallStmt{
			Expr: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "print"},
				Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		}
	}

	fn := &ast.FunctionExpr{Stmts: stmts}

	b.ResetTimer()

	for range b.N {
		Build(fn)
	}
}

func BenchmarkBuild_NestedFunctions(b *testing.B) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"f"},
				Exprs: []ast.Expr{
					&ast.FunctionExpr{
						Stmts: []ast.Stmt{
							&ast.LocalAssignStmt{
								Names: []string{"g"},
								Exprs: []ast.Expr{
									&ast.FunctionExpr{
										Stmts: []ast.Stmt{
											&ast.ReturnStmt{
												Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for range b.N {
		Build(fn)
	}
}

func BenchmarkBuild_WithGlobals(b *testing.B) {
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

	globals := []string{"print", "pairs", "ipairs", "error", "assert", "type", "tostring", "tonumber"}

	b.ResetTimer()

	for range b.N {
		Build(fn, globals...)
	}
}

func BenchmarkBuild_ComplexBranching(b *testing.B) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x", "y", "z"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}, &ast.NumberExpr{Value: "0"}, &ast.NumberExpr{Value: "0"}},
			},
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
					&ast.IfStmt{
						Condition: &ast.TrueExpr{},
						Then: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{&ast.IdentExpr{Value: "y"}},
								Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
							},
						},
						Else: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{&ast.IdentExpr{Value: "y"}},
								Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
							},
						},
					},
				},
				Else: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
					},
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "z"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for range b.N {
		Build(fn)
	}
}

func BenchmarkSSA_SimpleFunction(b *testing.B) {
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
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
		},
	}

	b.ResetTimer()
	for range b.N {
		Build(fn)
	}
}

func BenchmarkSSA_ManyPhis(b *testing.B) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"a", "b", "c", "d"},
				Exprs: []ast.Expr{
					&ast.NumberExpr{Value: "0"},
					&ast.NumberExpr{Value: "0"},
					&ast.NumberExpr{Value: "0"},
					&ast.NumberExpr{Value: "0"},
				},
			},
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "a"}, &ast.IdentExpr{Value: "b"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.NumberExpr{Value: "1"}},
					},
				},
				Else: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "c"}, &ast.IdentExpr{Value: "d"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.NumberExpr{Value: "1"}},
					},
				},
			},
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "a"}, &ast.IdentExpr{Value: "c"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}, &ast.NumberExpr{Value: "2"}},
					},
				},
				Else: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "b"}, &ast.IdentExpr{Value: "d"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}, &ast.NumberExpr{Value: "2"}},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for range b.N {
		Build(fn)
	}
}

func BenchmarkScopeTracker_DeepNesting(b *testing.B) {
	b.ResetTimer()
	for range b.N {
		t := NewScopeTracker()
		for j := range 20 {
			t.EnterScope()
			t.RegisterSymbol(SymbolID(j+1), "x", SymbolLocal, Point(j))
			t.SnapshotVisibility(Point(j))
		}
		for range 20 {
			t.ExitScope()
		}
	}
}

func BenchmarkScopeTracker_ManySymbols(b *testing.B) {
	b.ResetTimer()
	for range b.N {
		t := NewScopeTracker()
		for j := range 100 {
			name := string('a' + rune(j%26))
			t.RegisterSymbol(SymbolID(j+1), name, SymbolLocal, Point(j))
		}
		t.SnapshotVisibility(Point(100))
	}
}

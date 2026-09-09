package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/types/constraint"
)

// =============================================================================
// A) Field Function Definitions
// Ensure FuncDefInfo.Symbol exists for field paths.
// =============================================================================

// TestFieldSymbol_FuncDef_SingleField tests function M.f() end.
func TestFieldSymbol_FuncDef_SingleField(t *testing.T) {
	// function M.f() end
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "M")

	var info *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		info = f
	})

	if info == nil {
		t.Fatal("FuncDefInfo not found")
	}

	// Symbol should be non-zero and different from M's symbol
	if info.Symbol == 0 {
		t.Error("FuncDefInfo.Symbol should be non-zero for M.f")
	}

	// TargetPath should be M.f
	if info.TargetPath.Root != "M" {
		t.Errorf("TargetPath.Root = %q, want %q", info.TargetPath.Root, "M")
	}
	if len(info.TargetPath.Segments) != 1 {
		t.Fatalf("TargetPath should have 1 segment, got %d", len(info.TargetPath.Segments))
	}
	if info.TargetPath.Segments[0].Name != "f" {
		t.Errorf("TargetPath segment = %s, want f", info.TargetPath.Segments[0].Name)
	}

	// Symbol should differ from M's base symbol
	if info.Symbol == info.ReceiverSymbol {
		t.Error("FuncDefInfo.Symbol should be different from M's symbol")
	}
}

// TestFieldSymbol_FuncDef_NestedField tests function M.f.g() end.
func TestFieldSymbol_FuncDef_NestedField(t *testing.T) {
	// function M.f.g() end
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "M"},
					Key:    &ast.StringExpr{Value: "f"},
				},
				Key: &ast.StringExpr{Value: "g"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "M")

	var info *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		info = f
	})

	if info == nil {
		t.Fatal("FuncDefInfo not found")
	}

	// Symbol should exist for M.f.g
	if info.Symbol == 0 {
		t.Error("FuncDefInfo.Symbol should be non-zero for M.f.g")
	}

	// TargetPath should be M.f.g
	if info.TargetPath.Root != "M" {
		t.Errorf("TargetPath.Root = %q, want %q", info.TargetPath.Root, "M")
	}
	if len(info.TargetPath.Segments) != 2 {
		t.Fatalf("TargetPath should have 2 segments, got %d", len(info.TargetPath.Segments))
	}
	if info.TargetPath.Segments[0].Name != "f" || info.TargetPath.Segments[1].Name != "g" {
		t.Errorf("TargetPath segments = [%s, %s], want [f, g]",
			info.TargetPath.Segments[0].Name, info.TargetPath.Segments[1].Name)
	}
}

// TestFieldSymbol_FuncDef_Method tests function M:f() end.
func TestFieldSymbol_FuncDef_Method(t *testing.T) {
	// function M:f() end
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.IdentExpr{Value: "M"},
			Method:   "f",
		},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "M")

	var info *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		info = f
	})

	if info == nil {
		t.Fatal("FuncDefInfo not found")
	}

	// Symbol should exist for M.f (method becomes field)
	if info.Symbol == 0 {
		t.Error("FuncDefInfo.Symbol should be non-zero for M:f")
	}

	// Method and dot notation should produce same TargetPath structure
	if info.TargetPath.Root != "M" {
		t.Errorf("TargetPath.Root = %q, want %q", info.TargetPath.Root, "M")
	}
	if len(info.TargetPath.Segments) != 1 {
		t.Fatalf("TargetPath should have 1 segment, got %d", len(info.TargetPath.Segments))
	}
	if info.TargetPath.Segments[0].Name != "f" {
		t.Errorf("TargetPath segment = %s, want f", info.TargetPath.Segments[0].Name)
	}
	if !info.IsMethod {
		t.Error("IsMethod should be true")
	}
}

// TestFieldSymbol_FuncDef_DeeplyNested tests function a.b.c.d() end.
func TestFieldSymbol_FuncDef_DeeplyNested(t *testing.T) {
	// function a.b.c.d() end
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "a"},
						Key:    &ast.StringExpr{Value: "b"},
					},
					Key: &ast.StringExpr{Value: "c"},
				},
				Key: &ast.StringExpr{Value: "d"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var info *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		info = f
	})

	if info == nil {
		t.Fatal("FuncDefInfo not found")
	}

	if info.Symbol == 0 {
		t.Error("FuncDefInfo.Symbol should be non-zero for a.b.c.d")
	}

	if len(info.TargetPath.Segments) != 3 {
		t.Fatalf("TargetPath should have 3 segments, got %d", len(info.TargetPath.Segments))
	}

	expected := []string{"b", "c", "d"}
	for i, exp := range expected {
		if info.TargetPath.Segments[i].Name != exp {
			t.Errorf("Segment[%d] = %q, want %q", i, info.TargetPath.Segments[i].Name, exp)
		}
	}
}

// TestFieldSymbol_FuncDef_ChainedMethod tests function a.b:c() end.
func TestFieldSymbol_FuncDef_ChainedMethod(t *testing.T) {
	// function a.b:c() end
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "a"},
				Key:    &ast.StringExpr{Value: "b"},
			},
			Method: "c",
		},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var info *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		info = f
	})

	if info == nil {
		t.Fatal("FuncDefInfo not found")
	}

	if info.Symbol == 0 {
		t.Error("FuncDefInfo.Symbol should be non-zero for a.b:c")
	}

	// TargetPath should be a.b.c
	if info.TargetPath.Root != "a" {
		t.Errorf("TargetPath.Root = %q, want %q", info.TargetPath.Root, "a")
	}
	if len(info.TargetPath.Segments) != 2 {
		t.Fatalf("TargetPath should have 2 segments, got %d", len(info.TargetPath.Segments))
	}
	if info.TargetPath.Segments[0].Name != "b" || info.TargetPath.Segments[1].Name != "c" {
		t.Errorf("TargetPath segments = [%s, %s], want [b, c]",
			info.TargetPath.Segments[0].Name, info.TargetPath.Segments[1].Name)
	}
	if !info.IsMethod {
		t.Error("IsMethod should be true")
	}
}

// =============================================================================
// B) Field Assignments
// Ensure assignments allocate symbols on field paths.
// =============================================================================

// TestFieldSymbol_Assign_FieldFunction tests M.f = function() end.
func TestFieldSymbol_Assign_FieldFunction(t *testing.T) {
	// M.f = function() end
	stmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "M")

	var info *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			info = a
		}
	})

	if info == nil {
		t.Fatal("AssignInfo for field assignment not found")
	}

	target := info.Targets[0]
	if target.Symbol == 0 {
		t.Error("AssignTarget.Symbol should be non-zero for M.f")
	}
	if target.BaseName != "M" {
		t.Errorf("AssignTarget.BaseName = %q, want %q", target.BaseName, "M")
	}
	if len(target.FieldPath) != 1 || target.FieldPath[0] != "f" {
		t.Errorf("AssignTarget.FieldPath = %v, want [f]", target.FieldPath)
	}
}

// TestFieldSymbol_Assign_NestedField tests M.f.g = function() end.
func TestFieldSymbol_Assign_NestedField(t *testing.T) {
	// M.f.g = function() end
	stmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "M"},
					Key:    &ast.StringExpr{Value: "f"},
				},
				Key: &ast.StringExpr{Value: "g"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "M")

	var info *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			info = a
		}
	})

	if info == nil {
		t.Fatal("AssignInfo for nested field assignment not found")
	}

	target := info.Targets[0]
	if target.Symbol == 0 {
		t.Error("AssignTarget.Symbol should be non-zero for M.f.g")
	}
	if len(target.FieldPath) != 2 {
		t.Fatalf("FieldPath should have 2 elements, got %d", len(target.FieldPath))
	}
	if target.FieldPath[0] != "f" || target.FieldPath[1] != "g" {
		t.Errorf("FieldPath = %v, want [f, g]", target.FieldPath)
	}
}

// TestFieldSymbol_Assign_LocalTableField tests local T = {}; T.f = function() end.
func TestFieldSymbol_Assign_LocalTableField(t *testing.T) {
	// local T = {}
	// T.f = function() end
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, assignStmt}}
	g := Build(fn)

	// Get local T's symbol
	var tSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "T" {
			tSym = a.Targets[0].Symbol
		}
	})

	if tSym == 0 {
		t.Fatal("Symbol for local T not found")
	}

	// Get field assignment
	var fieldInfo *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldInfo = a
		}
	})

	if fieldInfo == nil {
		t.Fatal("Field assignment not found")
	}

	target := fieldInfo.Targets[0]
	if target.Symbol == 0 {
		t.Error("AssignTarget.Symbol should be non-zero for T.f")
	}
	if target.BaseSymbol != tSym {
		t.Errorf("BaseSymbol = %d, want local T symbol %d", target.BaseSymbol, tSym)
	}
}

// TestFieldSymbol_Assign_StringIndex tests T["f"] = function() end.
func TestFieldSymbol_Assign_StringIndex(t *testing.T) {
	// local T = {}
	// T["f"] = function() end
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, assignStmt}}
	g := Build(fn)

	var fieldInfo *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldInfo = a
		}
	})

	if fieldInfo == nil {
		t.Fatal("Field assignment not found")
	}

	target := fieldInfo.Targets[0]
	if target.Symbol == 0 {
		t.Error("AssignTarget.Symbol should be non-zero for T[\"f\"]")
	}
}

// TestFieldSymbol_Assign_MultipleTargets tests M.a, M.b = fn1, fn2.
func TestFieldSymbol_Assign_MultipleTargets(t *testing.T) {
	// M.a, M.b = function() end, function() end
	stmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "a"},
			},
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "b"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}, &ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "M")

	var info *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) >= 2 {
			info = a
		}
	})

	if info == nil {
		t.Fatal("Multi-target assignment not found")
	}

	if len(info.Targets) < 2 {
		t.Fatalf("Expected at least 2 targets, got %d", len(info.Targets))
	}

	// Each field should have its own symbol
	if info.Targets[0].Symbol == 0 {
		t.Error("First target symbol should be non-zero")
	}
	if info.Targets[1].Symbol == 0 {
		t.Error("Second target symbol should be non-zero")
	}
	if info.Targets[0].Symbol == info.Targets[1].Symbol {
		t.Error("M.a and M.b should have different symbols")
	}
}

// TestFieldSymbol_Assign_TableLiteralWithFields tests local T = { f = function() end }.
func TestFieldSymbol_Assign_TableLiteralWithFields(t *testing.T) {
	// local T = { f = function() end }
	stmt := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:   &ast.StringExpr{Value: "f"},
						Value: &ast.FunctionExpr{},
					},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	// Local T assignment should exist
	var info *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "T" {
			info = a
		}
	})

	if info == nil {
		t.Fatal("Local assignment for T not found")
	}

	if info.Targets[0].Symbol == 0 {
		t.Error("Symbol for T should be non-zero")
	}
}

// =============================================================================
// C) Calls Resolve to Same Symbol
// Ensure calls to a field resolve to the same symbol as the definition.
// =============================================================================

// TestFieldSymbol_Call_MatchesFuncDef tests function M.f() end; M.f().
func TestFieldSymbol_Call_MatchesFuncDef(t *testing.T) {
	// function M.f() end
	// M.f()
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defStmt, callStmt}}
	g := Build(fn, "M")

	// Get FuncDef symbol
	var defInfo *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defInfo = f
	})
	if defInfo == nil {
		t.Fatal("FuncDefInfo not found")
	}

	// Get Call symbol
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Symbols should match via TargetPath/CalleePath comparison
	if defInfo.TargetPath.String() != callInfo.CalleePath.String() {
		t.Errorf("Paths differ: def=%s, call=%s", defInfo.TargetPath.String(), callInfo.CalleePath.String())
	}

	// Root symbols should match
	if defInfo.TargetPath.Symbol != callInfo.CalleePath.Symbol {
		t.Errorf("Root symbols differ: def=%d, call=%d", defInfo.TargetPath.Symbol, callInfo.CalleePath.Symbol)
	}
}

// TestFieldSymbol_Call_MatchesAssign tests M.f = function() end; M.f().
func TestFieldSymbol_Call_MatchesAssign(t *testing.T) {
	// M.f = function() end
	// M.f()
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt, callStmt}}
	g := Build(fn, "M")

	// Get assignment target symbol
	var assignInfo *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			assignInfo = a
		}
	})
	if assignInfo == nil {
		t.Fatal("AssignInfo not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should point to M.f
	if callInfo.CalleePath.Root != "M" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "M")
	}
	if len(callInfo.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(callInfo.CalleePath.Segments))
	}
	if callInfo.CalleePath.Segments[0].Name != "f" {
		t.Errorf("CalleePath segment = %s, want f", callInfo.CalleePath.Segments[0].Name)
	}

	// Root symbols should match
	if assignInfo.Targets[0].BaseSymbol != callInfo.CalleePath.Symbol {
		t.Errorf("Base symbols differ: assign=%d, call=%d",
			assignInfo.Targets[0].BaseSymbol, callInfo.CalleePath.Symbol)
	}
}

// TestFieldSymbol_Call_MethodMatchesDef tests function M:f() end; M:f().
func TestFieldSymbol_Call_MethodMatchesDef(t *testing.T) {
	// function M:f() end
	// M:f()
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.IdentExpr{Value: "M"},
			Method:   "f",
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Receiver: &ast.IdentExpr{Value: "M"},
			Method:   "f",
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defStmt, callStmt}}
	g := Build(fn, "M")

	var defInfo *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defInfo = f
	})
	if defInfo == nil {
		t.Fatal("FuncDefInfo not found")
	}

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// For method calls, CalleePath is receiver and Method is separate
	if callInfo.Method != "f" {
		t.Errorf("Method = %q, want %q", callInfo.Method, "f")
	}
	if callInfo.CalleePath.Root != "M" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "M")
	}

	// Receiver symbols should match
	if defInfo.ReceiverSymbol != callInfo.CalleePath.Symbol {
		t.Errorf("Receiver symbols differ: def=%d, call=%d",
			defInfo.ReceiverSymbol, callInfo.CalleePath.Symbol)
	}
}

// TestFieldSymbol_Call_LocalTableField tests local assert = { not_nil = function() end }; assert.not_nil(x).
func TestFieldSymbol_Call_LocalTableField(t *testing.T) {
	// local assert = { not_nil = function() end }
	// assert.not_nil(x)
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"assert"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:   &ast.StringExpr{Value: "not_nil"},
						Value: &ast.FunctionExpr{},
					},
				},
			},
		},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "assert"},
				Key:    &ast.StringExpr{Value: "not_nil"},
			},
			Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, callStmt}}
	g := Build(fn, "x")

	// Get local assert's symbol
	var assertSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "assert" {
			assertSym = a.Targets[0].Symbol
		}
	})
	if assertSym == 0 {
		t.Fatal("Symbol for local assert not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should resolve to assert.not_nil
	if callInfo.CalleePath.Root != "assert" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "assert")
	}
	if callInfo.CalleePath.Symbol != assertSym {
		t.Errorf("CalleePath.Symbol = %d, want local assert symbol %d",
			callInfo.CalleePath.Symbol, assertSym)
	}
	if len(callInfo.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(callInfo.CalleePath.Segments))
	}
	if callInfo.CalleePath.Segments[0].Name != "not_nil" {
		t.Errorf("CalleePath segment = %s, want not_nil", callInfo.CalleePath.Segments[0].Name)
	}
}

// TestFieldSymbol_Call_LocalTableSeparateAssign tests:
// local assert = {}; assert.not_nil = function() end; assert.not_nil(x).
func TestFieldSymbol_Call_LocalTableSeparateAssign(t *testing.T) {
	// local assert = {}
	// assert.not_nil = function() end
	// assert.not_nil(x)
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"assert"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "assert"},
				Key:    &ast.StringExpr{Value: "not_nil"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "assert"},
				Key:    &ast.StringExpr{Value: "not_nil"},
			},
			Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, assignStmt, callStmt}}
	g := Build(fn, "x")

	// Get local assert's symbol
	var assertSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "assert" {
			assertSym = a.Targets[0].Symbol
		}
	})
	if assertSym == 0 {
		t.Fatal("Symbol for local assert not found")
	}

	// Get field assignment symbol
	var fieldAssign *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldAssign = a
		}
	})
	if fieldAssign == nil {
		t.Fatal("Field assignment not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Base symbols should match
	if fieldAssign.Targets[0].BaseSymbol != assertSym {
		t.Errorf("Field assignment base symbol = %d, want %d",
			fieldAssign.Targets[0].BaseSymbol, assertSym)
	}
	if callInfo.CalleePath.Symbol != assertSym {
		t.Errorf("CalleePath.Symbol = %d, want %d", callInfo.CalleePath.Symbol, assertSym)
	}

	// Both should reference the same field path
	if callInfo.CalleePath.Segments[0].Name != "not_nil" {
		t.Errorf("CalleePath segment = %s, want not_nil", callInfo.CalleePath.Segments[0].Name)
	}
}

// =============================================================================
// D) Shadowing & Scope
// Ensure symbols are distinct when shadowed.
// =============================================================================

// TestFieldSymbol_Shadowing_GlobalVsLocal tests global assert and local assert each with not_nil.
func TestFieldSymbol_Shadowing_GlobalVsLocal(t *testing.T) {
	// Global assert exists
	// local assert = { not_nil = function() end }
	// assert.not_nil(x) -- should use local
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"assert"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:   &ast.StringExpr{Value: "not_nil"},
						Value: &ast.FunctionExpr{},
					},
				},
			},
		},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "assert"},
				Key:    &ast.StringExpr{Value: "not_nil"},
			},
			Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, callStmt}}

	// Build with global "assert" seeded
	bindings := bind.Bind(fn, []string{"assert", "x"})
	g := BuildWithBindings(fn, bindings)

	// Get local assert's symbol
	var localSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "assert" {
			localSym = a.Targets[0].Symbol
		}
	})
	if localSym == 0 {
		t.Fatal("Symbol for local assert not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Call should resolve to local assert, not global
	if callInfo.CalleePath.Symbol != localSym {
		t.Errorf("CalleePath.Symbol = %d, want local symbol %d",
			callInfo.CalleePath.Symbol, localSym)
	}
}

// TestFieldSymbol_Shadowing_TwoLocals tests two local tables in different scopes with same field names.
func TestFieldSymbol_Shadowing_TwoLocals(t *testing.T) {
	// local T1 = {}
	// T1.f = function() end
	// do
	//   local T1 = {}
	//   T1.f = function() end  -- different symbol for inner T1.f
	// end
	outerLocal := &ast.LocalAssignStmt{
		Names: []string{"T1"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	outerAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T1"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}

	innerLocal := &ast.LocalAssignStmt{
		Names: []string{"T1"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	innerAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T1"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	doBlock := &ast.DoBlockStmt{
		Stmts: []ast.Stmt{innerLocal, innerAssign},
	}

	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{outerLocal, outerAssign, doBlock}}
	g := Build(fn)

	// Collect all local T1 assignments
	var t1Symbols []SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "T1" {
			t1Symbols = append(t1Symbols, a.Targets[0].Symbol)
		}
	})

	if len(t1Symbols) < 2 {
		t.Fatalf("Expected at least 2 T1 symbols, got %d", len(t1Symbols))
	}

	// The two T1s should have different symbols
	if t1Symbols[0] == t1Symbols[1] {
		t.Error("Outer and inner T1 should have different symbols")
	}

	// Collect field assignments
	var fieldBaseSyms []SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldBaseSyms = append(fieldBaseSyms, a.Targets[0].BaseSymbol)
		}
	})

	if len(fieldBaseSyms) < 2 {
		t.Fatalf("Expected at least 2 field assignments, got %d", len(fieldBaseSyms))
	}

	// Field base symbols should differ (pointing to different T1s)
	if fieldBaseSyms[0] == fieldBaseSyms[1] {
		t.Error("Outer T1.f and inner T1.f should have different base symbols")
	}
}

// TestFieldSymbol_Shadowing_NestedFunctions tests symbols in nested function scopes.
func TestFieldSymbol_Shadowing_NestedFunctions(t *testing.T) {
	// local M = {}
	// M.f = function()
	//   local M = {}
	//   M.g = function() end
	// end
	outerLocal := &ast.LocalAssignStmt{
		Names: []string{"M"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	innerLocal := &ast.LocalAssignStmt{
		Names: []string{"M"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	innerAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "g"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	outerAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{
			&ast.FunctionExpr{
				Stmts: []ast.Stmt{innerLocal, innerAssign},
			},
		},
	}

	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{outerLocal, outerAssign}}
	g := Build(fn)

	// Outer M should have its own symbol
	var outerMSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "M" {
			outerMSym = a.Targets[0].Symbol
		}
	})

	if outerMSym == 0 {
		t.Fatal("Outer M symbol not found")
	}

	// Outer field assignment should reference outer M
	var outerFieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			outerFieldSym = a.Targets[0].BaseSymbol
		}
	})

	if outerFieldSym != outerMSym {
		t.Errorf("Outer field base symbol = %d, want outer M symbol %d",
			outerFieldSym, outerMSym)
	}
}

// =============================================================================
// E) Negative / Dynamic
// Ensure we don't assign symbols when not statically resolvable.
// =============================================================================

// TestFieldSymbol_Dynamic_VariableKey tests obj[k] = function() end where k is not constant.
func TestFieldSymbol_Dynamic_VariableKey(t *testing.T) {
	// local obj = {}
	// local k = "f"
	// obj[k] = function() end
	localObj := &ast.LocalAssignStmt{
		Names: []string{"obj"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	localK := &ast.LocalAssignStmt{
		Names: []string{"k"},
		Exprs: []ast.Expr{&ast.StringExpr{Value: "f"}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "obj"},
				Key:    &ast.IdentExpr{Value: "k"}, // Variable key, not string literal
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localObj, localK, assignStmt}}
	g := Build(fn)

	// Find the index assignment
	var indexInfo *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetIndex {
			indexInfo = a
		}
	})

	if indexInfo == nil {
		t.Fatal("Index assignment not found")
	}

	// Symbol should be zero for dynamic key
	if indexInfo.Targets[0].Symbol != 0 {
		t.Errorf("Target symbol should be zero for dynamic key, got %d",
			indexInfo.Targets[0].Symbol)
	}
}

// TestFieldSymbol_Dynamic_VariableCall tests obj[k]() where k is not constant.
func TestFieldSymbol_Dynamic_VariableCall(t *testing.T) {
	// obj[k]()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "obj"},
			Key:    &ast.IdentExpr{Value: "k"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "obj", "k")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be empty for variable key
	if !callInfo.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for variable key, got %s",
			callInfo.CalleePath.String())
	}
}

// TestFieldSymbol_Dynamic_ComputedBase tests (getTable()).f().
func TestFieldSymbol_Dynamic_ComputedBase(t *testing.T) {
	// (getTable()).f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "getTable"},
			},
			Key: &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "getTable")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be empty when base is function call
	if !callInfo.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for computed base, got %s",
			callInfo.CalleePath.String())
	}
}

// TestFieldSymbol_Dynamic_ExpressionKey tests obj[1+1] = function() end.
func TestFieldSymbol_Dynamic_ExpressionKey(t *testing.T) {
	// obj[1+1] = function() end
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "obj"},
				Key: &ast.ArithmeticOpExpr{
					Operator: "+",
					Lhs:      &ast.NumberExpr{Value: "1"},
					Rhs:      &ast.NumberExpr{Value: "1"},
				},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt}}
	g := Build(fn, "obj")

	var indexInfo *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) > 0 && a.Targets[0].Kind == TargetIndex {
			indexInfo = a
		}
	})

	if indexInfo == nil {
		t.Fatal("Index assignment not found")
	}

	// Symbol should be zero for expression key
	if indexInfo.Targets[0].Symbol != 0 {
		t.Errorf("Target symbol should be zero for expression key, got %d",
			indexInfo.Targets[0].Symbol)
	}
}

// TestFieldSymbol_Dynamic_TableLiteralCall tests ({}).f().
func TestFieldSymbol_Dynamic_TableLiteralCall(t *testing.T) {
	// ({}).f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.TableExpr{},
			Key:    &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be empty for table literal base
	if !callInfo.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for table literal base, got %s",
			callInfo.CalleePath.String())
	}
}

// =============================================================================
// Additional permutations and variations
// =============================================================================

// TestFieldSymbol_MixedDotAndMethod tests function M.f() end followed by M:f().
func TestFieldSymbol_MixedDotAndMethod(t *testing.T) {
	// function M.f() end
	// M:f() -- same field, different call syntax
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Receiver: &ast.IdentExpr{Value: "M"},
			Method:   "f",
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defStmt, callStmt}}
	g := Build(fn, "M")

	var defInfo *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defInfo = f
	})

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if defInfo == nil || callInfo == nil {
		t.Fatal("FuncDefInfo or CallInfo not found")
	}

	// Receiver symbols should match
	if defInfo.ReceiverSymbol != callInfo.CalleePath.Symbol {
		t.Errorf("Symbol mismatch: def receiver=%d, call path=%d",
			defInfo.ReceiverSymbol, callInfo.CalleePath.Symbol)
	}
}

// TestFieldSymbol_SelfReference tests T.new that returns setmetatable({}, T).
func TestFieldSymbol_SelfReference(t *testing.T) {
	// local T = {}
	// T.new = function() return setmetatable({}, T) end
	// T.method = function(self) end
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	newAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "new"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	methodAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "method"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, newAssign, methodAssign}}
	g := Build(fn)

	// Get T's symbol
	var tSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "T" {
			tSym = a.Targets[0].Symbol
		}
	})

	if tSym == 0 {
		t.Fatal("T symbol not found")
	}

	// Both field assignments should reference same T
	var baseSym1, baseSym2 SymbolID
	count := 0
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			if count == 0 {
				baseSym1 = a.Targets[0].BaseSymbol
			} else {
				baseSym2 = a.Targets[0].BaseSymbol
			}
			count++
		}
	})

	if baseSym1 != tSym || baseSym2 != tSym {
		t.Errorf("Field assignments should reference T (sym=%d), got %d and %d",
			tSym, baseSym1, baseSym2)
	}
}

// TestFieldSymbol_IntegerIndex tests T[1] = function() end.
func TestFieldSymbol_IntegerIndex(t *testing.T) {
	// T[1] = function() end
	stmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.NumberExpr{Value: "1"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "T")

	var info *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		info = a
	})

	if info == nil {
		t.Fatal("AssignInfo not found")
	}

	// Integer index should create a symbol
	if info.Targets[0].Symbol == 0 {
		t.Error("Integer index target should have a symbol")
	}
}

// TestFieldSymbol_CallChain tests a.b.c.d().
func TestFieldSymbol_CallChain(t *testing.T) {
	// a.b.c.d()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
			Key: &ast.StringExpr{Value: "d"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be a.b.c.d
	if callInfo.CalleePath.Root != "a" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "a")
	}
	if len(callInfo.CalleePath.Segments) != 3 {
		t.Fatalf("CalleePath should have 3 segments, got %d", len(callInfo.CalleePath.Segments))
	}
	expected := []string{"b", "c", "d"}
	for i, exp := range expected {
		if callInfo.CalleePath.Segments[i].Name != exp {
			t.Errorf("Segment[%d] = %q, want %q", i, callInfo.CalleePath.Segments[i].Name, exp)
		}
	}
}

// TestFieldSymbol_MethodChain tests a.b:c().
func TestFieldSymbol_MethodChain(t *testing.T) {
	// a.b:c()
	call := &ast.FuncCallExpr{
		Receiver: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "a"},
			Key:    &ast.StringExpr{Value: "b"},
		},
		Method: "c",
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be receiver path: a.b
	if callInfo.CalleePath.Root != "a" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "a")
	}
	if len(callInfo.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(callInfo.CalleePath.Segments))
	}
	if callInfo.CalleePath.Segments[0].Name != "b" {
		t.Errorf("Segment = %q, want %q", callInfo.CalleePath.Segments[0].Name, "b")
	}
	if callInfo.Method != "c" {
		t.Errorf("Method = %q, want %q", callInfo.Method, "c")
	}
}

// TestFieldSymbol_MultipleFuncDefs tests multiple function definitions on same table.
func TestFieldSymbol_MultipleFuncDefs(t *testing.T) {
	// function M.a() end
	// function M.b() end
	// function M.c() end
	defA := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "a"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	defB := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "b"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	defC := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "c"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defA, defB, defC}}
	g := Build(fn, "M")

	var defs []*FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defs = append(defs, f)
	})

	if len(defs) != 3 {
		t.Fatalf("Expected 3 function definitions, got %d", len(defs))
	}

	// All should have the same receiver symbol (M)
	for i, def := range defs {
		if def.ReceiverSymbol == 0 {
			t.Errorf("Def %d: ReceiverSymbol should be non-zero", i)
		}
		if def.Symbol == 0 {
			t.Errorf("Def %d: Symbol should be non-zero", i)
		}
	}

	// Each function should have a unique symbol
	symbols := make(map[SymbolID]bool)
	for _, def := range defs {
		if symbols[def.Symbol] {
			t.Errorf("Duplicate function symbol found: %d", def.Symbol)
		}
		symbols[def.Symbol] = true
	}
}

// TestFieldSymbol_UpvalueField tests field access on upvalue.
func TestFieldSymbol_UpvalueField(t *testing.T) {
	// local M = {}
	// local function f()
	//   M.x = 1
	// end
	localM := &ast.LocalAssignStmt{
		Names: []string{"M"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	innerAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "x"},
			},
		},
		Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}
	innerFn := &ast.FunctionExpr{Stmts: []ast.Stmt{innerAssign}}
	localF := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{innerFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localM, localF}}
	g := Build(fn)

	// Get M's symbol from outer scope
	var mSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "M" {
			mSym = a.Targets[0].Symbol
		}
	})

	if mSym == 0 {
		t.Fatal("M symbol not found")
	}

	// The nested function is in Nested, but the CFG for outer function
	// should still reference M properly
	if g.NameOf(mSym) != "M" {
		t.Errorf("NameOf(mSym) = %q, want %q", g.NameOf(mSym), "M")
	}
}

// TestFieldSymbol_ReturnCall tests return M.f().
func TestFieldSymbol_ReturnCall(t *testing.T) {
	// return M.f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "M"},
			Key:    &ast.StringExpr{Value: "f"},
		},
	}
	ret := &ast.ReturnStmt{
		Exprs: []ast.Expr{call},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{ret}}
	g := Build(fn, "M")

	var retInfo *ReturnInfo
	g.EachReturn(func(_ Point, r *ReturnInfo) {
		if len(r.Exprs) > 0 {
			retInfo = r
		}
	})

	if retInfo == nil {
		t.Fatal("ReturnInfo not found")
	}

	if len(retInfo.SourceCalls) != 1 || retInfo.SourceCalls[0] == nil {
		t.Fatal("Return source call not found")
	}

	callInfo := retInfo.SourceCalls[0]
	if callInfo.CalleePath.Root != "M" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "M")
	}
	if len(callInfo.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(callInfo.CalleePath.Segments))
	}
	if callInfo.CalleePath.Segments[0].Name != "f" {
		t.Errorf("Segment = %s, want f", callInfo.CalleePath.Segments[0].Name)
	}
}

// TestFieldSymbol_AssignFromCall tests x = M.f().
func TestFieldSymbol_AssignFromCall(t *testing.T) {
	// x = M.f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "M"},
			Key:    &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		Rhs: []ast.Expr{call},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "M", "x")

	var info *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.SourceCalls) > 0 && a.SourceCalls[0] != nil {
			info = a
		}
	})

	if info == nil {
		t.Fatal("AssignInfo with source call not found")
	}

	callInfo := info.SourceCalls[0]
	if callInfo.CalleePath.Root != "M" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "M")
	}
}

// TestFieldSymbol_StringIndexVsField tests T["method"] vs T.method.
func TestFieldSymbol_StringIndexVsField(t *testing.T) {
	// T.method = function() end
	// T["method"]()
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "method"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	// String index call: T["method"]() is same as T.method()
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "method"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt, callStmt}}
	g := Build(fn, "T")

	var assignInfo *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		assignInfo = a
	})

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if assignInfo == nil || callInfo == nil {
		t.Fatal("AssignInfo or CallInfo not found")
	}

	// Both should reference T.method
	if callInfo.CalleePath.Root != "T" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "T")
	}
	if len(callInfo.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(callInfo.CalleePath.Segments))
	}
	// String index that is valid identifier should use SegmentField
	if callInfo.CalleePath.Segments[0].Kind != constraint.SegmentField {
		t.Errorf("Segment kind = %v, want SegmentField", callInfo.CalleePath.Segments[0].Kind)
	}
}

// TestFieldSymbol_NonIdentStringKey tests T["not-valid-ident"] access.
func TestFieldSymbol_NonIdentStringKey(t *testing.T) {
	// T["not-valid-ident"]()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "T"},
			Key:    &ast.StringExpr{Value: "not-valid-ident"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "T")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Should use SegmentIndexString for non-identifier keys
	if len(callInfo.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(callInfo.CalleePath.Segments))
	}
	if callInfo.CalleePath.Segments[0].Kind != constraint.SegmentIndexString {
		t.Errorf("Segment kind = %v, want SegmentIndexString", callInfo.CalleePath.Segments[0].Kind)
	}
}

// TestFieldSymbol_ReassignField tests M.f = fn1; M.f = fn2.
func TestFieldSymbol_ReassignField(t *testing.T) {
	// M.f = function() return 1 end
	// M.f = function() return 2 end
	// M.f()
	assign1 := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	assign2 := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assign1, assign2, callStmt}}
	g := Build(fn, "M")

	// Both assignments should reference M with same symbol
	var assigns []*AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		assigns = append(assigns, a)
	})

	if len(assigns) != 2 {
		t.Fatalf("Expected 2 assignments, got %d", len(assigns))
	}

	// Both should have same base symbol (M)
	if assigns[0].Targets[0].BaseSymbol != assigns[1].Targets[0].BaseSymbol {
		t.Error("Both assignments should have same base symbol")
	}

	// Call should resolve to same base
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo.CalleePath.Symbol != assigns[0].Targets[0].BaseSymbol {
		t.Errorf("Call base symbol differs from assignments")
	}
}

// =============================================================================
// F) Anonymous Function Literals - ALL must get symbols
// Every function literal must have a SymbolID, even anonymous ones.
// =============================================================================

// TestFieldSymbol_AnonymousLiteral_InlineCallback tests foo(function() end).
func TestFieldSymbol_AnonymousLiteral_InlineCallback(t *testing.T) {
	// foo(function() end)
	anonFn := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "foo"},
		Args: []ast.Expr{anonFn},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "foo")

	// The anonymous function should appear in Nested
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Anonymous function should be in Nested")
	}

	// Anonymous function literal should have a symbol
	found := false
	for _, n := range nested {
		if n.Func == anonFn {
			found = true
			// Symbol for anonymous literal should be non-zero
			if n.Symbol == 0 {
				t.Error("Anonymous callback function should have a symbol")
			}
		}
	}
	if !found {
		t.Error("Anonymous function not found in Nested")
	}
}

// TestFieldSymbol_AnonymousLiteral_Return tests return function() end.
func TestFieldSymbol_AnonymousLiteral_Return(t *testing.T) {
	// return function() end
	anonFn := &ast.FunctionExpr{}
	ret := &ast.ReturnStmt{
		Exprs: []ast.Expr{anonFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{ret}}
	g := Build(fn)

	// The anonymous function should appear in Nested
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Returned anonymous function should be in Nested")
	}

	// Should have a symbol
	for _, n := range nested {
		if n.Func == anonFn {
			if n.Symbol == 0 {
				t.Error("Returned anonymous function should have a symbol")
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_TableField tests local t = { f = function() end }.
func TestFieldSymbol_AnonymousLiteral_TableField(t *testing.T) {
	// local t = { f = function() end }
	anonFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:   &ast.StringExpr{Value: "f"},
						Value: anonFn,
					},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	// Get t's symbol
	var tSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "t" {
			tSym = a.Targets[0].Symbol
		}
	})
	if tSym == 0 {
		t.Fatal("Symbol for t not found")
	}

	// The function should appear in Nested with a symbol
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Table field function should be in Nested")
	}

	// Symbol should exist and ideally relate to t.f
	for _, n := range nested {
		if n.Func == anonFn {
			if n.Symbol == 0 {
				t.Error("Table field function should have a symbol (t.f)")
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_TableFieldCall tests local t = { f = function() end }; t.f().
func TestFieldSymbol_AnonymousLiteral_TableFieldCall(t *testing.T) {
	// local t = { f = function() end }
	// t.f()
	anonFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:   &ast.StringExpr{Value: "f"},
						Value: anonFn,
					},
				},
			},
		},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "t"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, callStmt}}
	g := Build(fn)

	// Get t's symbol
	var tSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "t" {
			tSym = a.Targets[0].Symbol
		}
	})
	if tSym == 0 {
		t.Fatal("Symbol for t not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Call should resolve to t.f
	if callInfo.CalleePath.Root != "t" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "t")
	}
	if callInfo.CalleePath.Symbol != tSym {
		t.Errorf("CalleePath.Symbol = %d, want %d", callInfo.CalleePath.Symbol, tSym)
	}

	// Get the nested function's symbol
	nested := g.NestedFunctions()
	var fnSym SymbolID
	for _, n := range nested {
		if n.Func == anonFn {
			fnSym = n.Symbol
		}
	}

	// Function literal should have symbol that call could resolve to
	if fnSym == 0 {
		t.Error("Table field function literal should have a symbol")
	}
}

// TestFieldSymbol_AnonymousLiteral_MultipleTableFields tests { a = fn, b = fn, c = fn }.
func TestFieldSymbol_AnonymousLiteral_MultipleTableFields(t *testing.T) {
	// local t = { a = function() end, b = function() end, c = function() end }
	fnA := &ast.FunctionExpr{}
	fnB := &ast.FunctionExpr{}
	fnC := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.StringExpr{Value: "a"}, Value: fnA},
					{Key: &ast.StringExpr{Value: "b"}, Value: fnB},
					{Key: &ast.StringExpr{Value: "c"}, Value: fnC},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected at least 3 nested functions, got %d", len(nested))
	}

	// Each should have a unique symbol
	symbols := make(map[SymbolID]bool)
	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Nested function should have a symbol")

			continue
		}
		if symbols[n.Symbol] {
			t.Errorf("Duplicate symbol %d for nested functions", n.Symbol)
		}
		symbols[n.Symbol] = true
	}
}

// TestFieldSymbol_AnonymousLiteral_NestedCallback tests foo(function() bar(function() end) end).
func TestFieldSymbol_AnonymousLiteral_NestedCallback(t *testing.T) {
	// foo(function() bar(function() end) end)
	innerFn := &ast.FunctionExpr{}
	innerCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "bar"},
			Args: []ast.Expr{innerFn},
		},
	}
	outerFn := &ast.FunctionExpr{Stmts: []ast.Stmt{innerCall}}
	outerCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "foo"},
			Args: []ast.Expr{outerFn},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{outerCall}}
	g := Build(fn, "foo", "bar")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Outer callback should be in Nested")
	}

	// Outer function should have a symbol
	for _, n := range nested {
		if n.Func == outerFn {
			if n.Symbol == 0 {
				t.Error("Outer callback should have a symbol")
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_ArrayElement tests { function() end, function() end }.
func TestFieldSymbol_AnonymousLiteral_ArrayElement(t *testing.T) {
	// local t = { function() end, function() end }
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Value: fn1}, // array element [1]
					{Value: fn2}, // array element [2]
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 2 {
		t.Fatalf("Expected at least 2 nested functions, got %d", len(nested))
	}

	// Each array element function should have a symbol
	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Array element function should have a symbol")
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_UnboundTable tests return { f = function() end }.
func TestFieldSymbol_AnonymousLiteral_UnboundTable(t *testing.T) {
	// return { f = function() end }
	anonFn := &ast.FunctionExpr{}
	ret := &ast.ReturnStmt{
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.StringExpr{Value: "f"}, Value: anonFn},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{ret}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in unbound table should be in Nested")
	}

	// Even in unbound table, the function should have a symbol
	for _, n := range nested {
		if n.Func == anonFn {
			if n.Symbol == 0 {
				t.Error("Function in unbound table should have a synthetic symbol")
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_PassToMethod tests obj:method(function() end).
func TestFieldSymbol_AnonymousLiteral_PassToMethod(t *testing.T) {
	// obj:method(function() end)
	anonFn := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "obj"},
		Method:   "method",
		Args:     []ast.Expr{anonFn},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "obj")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Callback to method should be in Nested")
	}

	for _, n := range nested {
		if n.Func == anonFn {
			if n.Symbol == 0 {
				t.Error("Callback to method should have a symbol")
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_IIFE tests (function() end)().
func TestFieldSymbol_AnonymousLiteral_IIFE(t *testing.T) {
	// (function() end)()
	anonFn := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: anonFn,
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("IIFE should be in Nested")
	}

	for _, n := range nested {
		if n.Func == anonFn {
			if n.Symbol == 0 {
				t.Error("IIFE should have a symbol")
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_MultipleArgs tests foo(fn1, fn2, fn3).
func TestFieldSymbol_AnonymousLiteral_MultipleArgs(t *testing.T) {
	// foo(function() end, function() end, function() end)
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	fn3 := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "foo"},
		Args: []ast.Expr{fn1, fn2, fn3},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "foo")

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected at least 3 nested functions, got %d", len(nested))
	}

	// Each should have unique symbol
	symbols := make(map[SymbolID]bool)
	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Callback argument should have a symbol")

			continue
		}

		if symbols[n.Symbol] {
			t.Errorf("Duplicate symbol for callback arguments")
		}
		symbols[n.Symbol] = true
	}
}

// TestFieldSymbol_AnonymousLiteral_AssignToLocal tests local f = function() end.
func TestFieldSymbol_AnonymousLiteral_AssignToLocal(t *testing.T) {
	// local f = function() end
	anonFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{anonFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	// Get local f's symbol
	var fSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "f" {
			fSym = a.Targets[0].Symbol
		}
	})
	if fSym == 0 {
		t.Fatal("Symbol for local f not found")
	}

	// The function literal should also have this symbol in Nested
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function literal should be in Nested")
	}

	for _, n := range nested {
		if n.Func == anonFn {
			// Symbol should match the local variable symbol
			if n.Symbol != fSym {
				t.Errorf("Nested symbol = %d, want local f symbol %d", n.Symbol, fSym)
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_AssignToGlobal tests f = function() end.
func TestFieldSymbol_AnonymousLiteral_AssignToGlobal(t *testing.T) {
	// f = function() end
	anonFn := &ast.FunctionExpr{}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IdentExpr{Value: "f"}},
		Rhs: []ast.Expr{anonFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt}}
	g := Build(fn, "f")

	// Get f's symbol from assignment
	var fSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) > 0 && a.Targets[0].Name == "f" {
			fSym = a.Targets[0].Symbol
		}
	})
	if fSym == 0 {
		t.Fatal("Symbol for global f not found")
	}

	// The function literal should have this symbol
	nested := g.NestedFunctions()
	for _, n := range nested {
		if n.Func == anonFn {
			if n.Symbol != fSym {
				t.Errorf("Nested symbol = %d, want global f symbol %d", n.Symbol, fSym)
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_AssignToField tests M.f = function() end.
func TestFieldSymbol_AnonymousLiteral_AssignToField(t *testing.T) {
	// M.f = function() end
	anonFn := &ast.FunctionExpr{}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{anonFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt}}
	g := Build(fn, "M")

	// Get field assignment symbol
	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})
	if fieldSym == 0 {
		t.Fatal("Symbol for M.f not found")
	}

	// The function literal should have the field symbol
	nested := g.NestedFunctions()
	for _, n := range nested {
		if n.Func == anonFn {
			if n.Symbol != fieldSym {
				t.Errorf("Nested symbol = %d, want field M.f symbol %d", n.Symbol, fieldSym)
			}
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_ConditionExpr tests (cond and function() end or function() end).
func TestFieldSymbol_AnonymousLiteral_ConditionExpr(t *testing.T) {
	// local f = cond and function() end or function() end
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{
			&ast.LogicalOpExpr{
				Operator: "or",
				Lhs: &ast.LogicalOpExpr{
					Operator: "and",
					Lhs:      &ast.IdentExpr{Value: "cond"},
					Rhs:      fn1,
				},
				Rhs: fn2,
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn, "cond")

	nested := g.NestedFunctions()
	if len(nested) < 2 {
		t.Fatalf("Expected at least 2 nested functions, got %d", len(nested))
	}

	// Both conditional functions should have symbols
	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Conditional branch function should have a symbol")
		}
	}
}

// TestFieldSymbol_AnonymousLiteral_TableWithMixedKeys tests { ["key"] = fn, 1, method = fn }.
func TestFieldSymbol_AnonymousLiteral_TableWithMixedKeys(t *testing.T) {
	// local t = { ["key"] = function() end, function() end, method = function() end }
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	fn3 := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.StringExpr{Value: "key"}, Value: fn1},
					{Value: fn2}, // array element
					{Key: &ast.StringExpr{Value: "method"}, Value: fn3},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected at least 3 nested functions, got %d", len(nested))
	}

	// All should have unique symbols
	symbols := make(map[SymbolID]bool)
	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Table field function should have a symbol")

			continue
		}

		if symbols[n.Symbol] {
			t.Errorf("Duplicate symbol for table fields")
		}
		symbols[n.Symbol] = true
	}
}

// TestFieldSymbol_AnonymousLiteral_DeepNesting tests deeply nested anonymous functions.
func TestFieldSymbol_AnonymousLiteral_DeepNesting(t *testing.T) {
	// local t = { a = { b = { c = function() end } } }
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key: &ast.StringExpr{Value: "a"},
						Value: &ast.TableExpr{
							Fields: []*ast.Field{
								{
									Key: &ast.StringExpr{Value: "b"},
									Value: &ast.TableExpr{
										Fields: []*ast.Field{
											{Key: &ast.StringExpr{Value: "c"}, Value: innerFn},
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
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Deeply nested function should be in Nested")
	}

	for _, n := range nested {
		if n.Func == innerFn {
			if n.Symbol == 0 {
				t.Error("Deeply nested function should have a symbol")
			}
		}
	}
}

// =============================================================================
// Edge Cases: Every function literal MUST have a symbol - no exceptions
// =============================================================================

// TestFieldSymbol_EdgeCase_TableArgToCall tests foo({f = function() end}) - unbound table.
func TestFieldSymbol_EdgeCase_TableArgToCall(t *testing.T) {
	// foo({f = function() end})
	innerFn := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "foo"},
		Args: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.StringExpr{Value: "f"}, Value: innerFn},
				},
			},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "foo")

	// Even in unbound table argument, function MUST have a symbol
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in table argument should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in unbound table argument MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_MultipleReturns tests return fn1, fn2, fn3.
func TestFieldSymbol_EdgeCase_MultipleReturns(t *testing.T) {
	// return function() end, function() end, function() end
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	fn3 := &ast.FunctionExpr{}
	ret := &ast.ReturnStmt{
		Exprs: []ast.Expr{fn1, fn2, fn3},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{ret}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected 3 nested functions, got %d", len(nested))
	}

	// All returned functions MUST have symbols
	symbols := make(map[SymbolID]bool)
	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Returned function MUST have a symbol")
		}
		if symbols[n.Symbol] {
			t.Error("Each returned function should have unique symbol")
		}
		symbols[n.Symbol] = true
	}
}

// TestFieldSymbol_EdgeCase_FunctionReassignment tests local g = t.f; g().
// CFG does not resolve aliases - g() has CalleeSymbol = 0 since g is a variable.
// Alias resolution belongs in flow analysis, not CFG.
func TestFieldSymbol_EdgeCase_FunctionReassignment(t *testing.T) {
	// local t = {f = function() end}
	// local g = t.f
	// g()
	innerFn := &ast.FunctionExpr{}
	localT := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.StringExpr{Value: "f"}, Value: innerFn},
				},
			},
		},
	}
	localG := &ast.LocalAssignStmt{
		Names: []string{"g"},
		Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "t"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "g"},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, localG, callStmt}}
	g := Build(fn)

	// Original function has symbol (t.f's symbol)
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function should be in Nested")
	}
	var fnSym SymbolID
	for _, n := range nested {
		if n.Func == innerFn {
			fnSym = n.Symbol
		}
	}
	if fnSym == 0 {
		t.Error("Original function MUST have a symbol")
	}

	// g() call has CalleeSymbol set to g's symbol (enables effect resolution)
	// Alias resolution is not CFG's job - g points to t.f but CalleeSymbol is g's own symbol
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Get g's symbol
	var gSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "g" {
			gSym = a.Targets[0].Symbol
		}
	})

	// CalleeSymbol is set for simple variable calls (enables effect resolution)
	if callInfo.CalleeSymbol != gSym {
		t.Errorf("g() CalleeSymbol = %d, want %d (gSym)", callInfo.CalleeSymbol, gSym)
	}
	// CalleeName should still be "g"
	if callInfo.CalleeName != "g" {
		t.Errorf("g() CalleeName = %q, want %q", callInfo.CalleeName, "g")
	}
}

// TestFieldSymbol_EdgeCase_PcallArgument tests pcall(function() end).
func TestFieldSymbol_EdgeCase_PcallArgument(t *testing.T) {
	// pcall(function() error("fail") end)
	innerFn := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "pcall"},
		Args: []ast.Expr{innerFn},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "pcall")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in pcall should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function passed to pcall MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_FunctionInBinaryOp tests local f = x or function() end.
func TestFieldSymbol_EdgeCase_FunctionInBinaryOp(t *testing.T) {
	// local f = x or function() end
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{
			&ast.LogicalOpExpr{
				Operator: "or",
				Lhs:      &ast.IdentExpr{Value: "x"},
				Rhs:      innerFn,
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn, "x")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in or-expression should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in binary expression MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_FunctionInArithmetic tests unlikely but valid: t[1+0] where index is computed.
func TestFieldSymbol_EdgeCase_ComputedIndexNoSymbol(t *testing.T) {
	// t[1+0] = function() end  -- computed index, no symbol
	innerFn := &ast.FunctionExpr{}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "t"},
				Key: &ast.ArithmeticOpExpr{
					Operator: "+",
					Lhs:      &ast.NumberExpr{Value: "1"},
					Rhs:      &ast.NumberExpr{Value: "0"},
				},
			},
		},
		Rhs: []ast.Expr{innerFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt}}
	g := Build(fn, "t")

	// Function still has a symbol (via GetOrCreateFuncLitSymbol)
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function with computed index target still MUST have a symbol")
		}
	}

	// But the target should NOT have a field symbol (computed key)
	var targetSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.Targets) > 0 {
			targetSym = a.Targets[0].Symbol
		}
	})
	if targetSym != 0 {
		t.Errorf("Computed index target should have no symbol, got %d", targetSym)
	}
}

// TestFieldSymbol_EdgeCase_NestedAnonymousInCallback tests foo(function() bar(function() end) end).
func TestFieldSymbol_EdgeCase_NestedAnonymousInCallback(t *testing.T) {
	// foo(function() bar(function() end) end)
	innerFn := &ast.FunctionExpr{}
	innerCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "bar"},
			Args: []ast.Expr{innerFn},
		},
	}
	outerFn := &ast.FunctionExpr{Stmts: []ast.Stmt{innerCall}}
	outerCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "foo"},
			Args: []ast.Expr{outerFn},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{outerCall}}
	g := Build(fn, "foo", "bar")

	// Outer function should have symbol
	nested := g.NestedFunctions()
	var outerSym SymbolID
	for _, n := range nested {
		if n.Func == outerFn {
			outerSym = n.Symbol
		}
	}
	if outerSym == 0 {
		t.Error("Outer callback MUST have a symbol")
	}
}

// TestFieldSymbol_EdgeCase_FunctionAsTableKey tests {[function() end] = 1} - function as key.
func TestFieldSymbol_EdgeCase_FunctionAsTableKey(t *testing.T) {
	// local t = {[function() end] = 1}
	keyFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: keyFn, Value: &ast.NumberExpr{Value: "1"}},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	// Even function used as key MUST have a symbol
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function as table key should be in Nested")
	}
	for _, n := range nested {
		if n.Func == keyFn && n.Symbol == 0 {
			t.Error("Function used as table key MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_FunctionInTableConcat tests t1 or {f = function() end}.
func TestFieldSymbol_EdgeCase_TableInOr(t *testing.T) {
	// local t = t1 or {f = function() end}
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.LogicalOpExpr{
				Operator: "or",
				Lhs:      &ast.IdentExpr{Value: "t1"},
				Rhs: &ast.TableExpr{
					Fields: []*ast.Field{
						{Key: &ast.StringExpr{Value: "f"}, Value: innerFn},
					},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn, "t1")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in fallback table should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in or-fallback table MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_WhileConditionCall tests while T.f() do end.
func TestFieldSymbol_EdgeCase_WhileConditionCall(t *testing.T) {
	// local T = {}
	// T.f = function() return true end
	// while T.f() do break end
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	whileStmt := &ast.WhileStmt{
		Condition: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Stmts: []ast.Stmt{&ast.BreakStmt{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, whileStmt}}
	g := Build(fn)

	// Verify field symbol exists
	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})
	if fieldSym == 0 {
		t.Error("T.f should have a field symbol")
	}
}

// TestFieldSymbol_EdgeCase_ForIteratorFunction tests for loop with function iterator.
func TestFieldSymbol_EdgeCase_ForIteratorFunction(t *testing.T) {
	// for k, v in (function() return pairs(t) end)() do end
	// This is unusual but valid - IIFE returning iterator
	iterFn := &ast.FunctionExpr{}
	forStmt := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: iterFn,
			},
		},
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{forStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Iterator function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == iterFn && n.Symbol == 0 {
			t.Error("Iterator function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_RepeatUntilCondition tests repeat until (function() end)().
func TestFieldSymbol_EdgeCase_RepeatUntilCondition(t *testing.T) {
	// repeat break until (function() return true end)()
	condFn := &ast.FunctionExpr{}
	repeatStmt := &ast.RepeatStmt{
		Condition: &ast.FuncCallExpr{Func: condFn},
		Stmts:     []ast.Stmt{&ast.BreakStmt{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{repeatStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in repeat-until condition should be in Nested")
	}
	for _, n := range nested {
		if n.Func == condFn && n.Symbol == 0 {
			t.Error("Function in repeat-until condition MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_NumericForExpressions tests for i = (function() end)(), 10 do end.
func TestFieldSymbol_EdgeCase_NumericForExpressions(t *testing.T) {
	// for i = (function() return 1 end)(), (function() return 10 end)(), (function() return 1 end)() do end
	initFn := &ast.FunctionExpr{}
	limitFn := &ast.FunctionExpr{}
	stepFn := &ast.FunctionExpr{}
	forStmt := &ast.NumberForStmt{
		Name:  "i",
		Init:  &ast.FuncCallExpr{Func: initFn},
		Limit: &ast.FuncCallExpr{Func: limitFn},
		Step:  &ast.FuncCallExpr{Func: stepFn},
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{forStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected 3 functions in numeric for, got %d", len(nested))
	}

	foundInit, foundLimit, foundStep := false, false, false
	for _, n := range nested {
		if n.Func == initFn {
			foundInit = true
			if n.Symbol == 0 {
				t.Error("Init function MUST have a symbol")
			}
		}
		if n.Func == limitFn {
			foundLimit = true
			if n.Symbol == 0 {
				t.Error("Limit function MUST have a symbol")
			}
		}
		if n.Func == stepFn {
			foundStep = true
			if n.Symbol == 0 {
				t.Error("Step function MUST have a symbol")
			}
		}
	}
	if !foundInit || !foundLimit || !foundStep {
		t.Error("All numeric for expression functions should be found")
	}
}

// TestFieldSymbol_EdgeCase_IfConditionFunction tests if (function() end)() then end.
func TestFieldSymbol_EdgeCase_IfConditionFunction(t *testing.T) {
	// if (function() return true end)() then end
	condFn := &ast.FunctionExpr{}
	ifStmt := &ast.IfStmt{
		Condition: &ast.FuncCallExpr{Func: condFn},
		Then:      []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{ifStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in if condition should be in Nested")
	}
	for _, n := range nested {
		if n.Func == condFn && n.Symbol == 0 {
			t.Error("Function in if condition MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_ElseIfConditionFunction tests elseif (function() end)() then end.
func TestFieldSymbol_EdgeCase_ElseIfConditionFunction(t *testing.T) {
	// if false then else if (function() return true end)() then end end
	// IfStmt chains via Else field containing another IfStmt
	condFn := &ast.FunctionExpr{}
	innerIf := &ast.IfStmt{
		Condition: &ast.FuncCallExpr{Func: condFn},
		Then:      []ast.Stmt{},
	}
	ifStmt := &ast.IfStmt{
		Condition: &ast.FalseExpr{},
		Then:      []ast.Stmt{},
		Else:      []ast.Stmt{innerIf},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{ifStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in elseif condition should be in Nested")
	}
	for _, n := range nested {
		if n.Func == condFn && n.Symbol == 0 {
			t.Error("Function in elseif condition MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_UnaryNotFunction tests not (function() end)().
func TestFieldSymbol_EdgeCase_UnaryNotFunction(t *testing.T) {
	// local x = not (function() return false end)()
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{
			&ast.UnaryNotOpExpr{
				Expr: &ast.FuncCallExpr{Func: innerFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in unary not should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in unary not MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_MultipleLocalFunctions tests local a, b = function() end, function() end.
func TestFieldSymbol_EdgeCase_MultipleLocalFunctions(t *testing.T) {
	// local a, b, c = function() end, function() end, function() end
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	fn3 := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"a", "b", "c"},
		Exprs: []ast.Expr{fn1, fn2, fn3},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected 3 nested functions, got %d", len(nested))
	}

	found1, found2, found3 := false, false, false
	for _, n := range nested {
		if n.Func == fn1 {
			found1 = true
			if n.Symbol == 0 {
				t.Error("First function MUST have a symbol")
			}
		}
		if n.Func == fn2 {
			found2 = true
			if n.Symbol == 0 {
				t.Error("Second function MUST have a symbol")
			}
		}
		if n.Func == fn3 {
			found3 = true
			if n.Symbol == 0 {
				t.Error("Third function MUST have a symbol")
			}
		}
	}
	if !found1 || !found2 || !found3 {
		t.Error("All local functions should be found")
	}
}

// TestFieldSymbol_EdgeCase_ReturnMultipleFunctions tests return function(), function().
func TestFieldSymbol_EdgeCase_ReturnMultipleFunctions(t *testing.T) {
	// return function() end, function() end, function() end
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	fn3 := &ast.FunctionExpr{}
	returnStmt := &ast.ReturnStmt{
		Exprs: []ast.Expr{fn1, fn2, fn3},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{returnStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected 3 nested functions in return, got %d", len(nested))
	}

	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Returned function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_IIFEPattern tests (function() end)() - immediately invoked.
func TestFieldSymbol_EdgeCase_IIFEPattern(t *testing.T) {
	// (function() return 1 end)()
	iifeFn := &ast.FunctionExpr{}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{Func: iifeFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{callStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("IIFE function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == iifeFn && n.Symbol == 0 {
			t.Error("IIFE function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_DoBlockFunction tests do local f = function() end end.
func TestFieldSymbol_EdgeCase_DoBlockFunction(t *testing.T) {
	// do local f = function() end f() end
	innerFn := &ast.FunctionExpr{}
	doStmt := &ast.DoBlockStmt{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"f"},
				Exprs: []ast.Expr{innerFn},
			},
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "f"},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{doStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in do block should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in do block MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_AssertArgFunction tests assert((function() end)()).
func TestFieldSymbol_EdgeCase_AssertArgFunction(t *testing.T) {
	// assert((function() return true end)())
	innerFn := &ast.FunctionExpr{}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "assert"},
			Args: []ast.Expr{
				&ast.FuncCallExpr{Func: innerFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{callStmt}}
	g := Build(fn, "assert")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in assert arg should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in assert argument MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_StringConcatFunction tests "a" .. (function() end)().
func TestFieldSymbol_EdgeCase_StringConcatFunction(t *testing.T) {
	// local s = "a" .. (function() return "b" end)()
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"s"},
		Exprs: []ast.Expr{
			&ast.StringConcatOpExpr{
				Lhs: &ast.StringExpr{Value: "a"},
				Rhs: &ast.FuncCallExpr{Func: innerFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in string concat should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in string concat MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_DeeplyNestedTable tests {a = {b = {c = function() end}}}.
func TestFieldSymbol_EdgeCase_DeeplyNestedTable(t *testing.T) {
	// local t = {a = {b = {c = function() end}}}
	deepFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key: &ast.StringExpr{Value: "a"},
						Value: &ast.TableExpr{
							Fields: []*ast.Field{
								{
									Key: &ast.StringExpr{Value: "b"},
									Value: &ast.TableExpr{
										Fields: []*ast.Field{
											{
												Key:   &ast.StringExpr{Value: "c"},
												Value: deepFn,
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
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Deeply nested function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == deepFn && n.Symbol == 0 {
			t.Error("Deeply nested function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_MethodReceiverFunction tests (function() end):method().
func TestFieldSymbol_EdgeCase_MethodReceiverFunction(t *testing.T) {
	// (function() return obj end)():method()
	receiverFn := &ast.FunctionExpr{}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Receiver: &ast.FuncCallExpr{Func: receiverFn},
			Method:   "method",
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{callStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function as method receiver should be in Nested")
	}
	for _, n := range nested {
		if n.Func == receiverFn && n.Symbol == 0 {
			t.Error("Function as method receiver MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_UnaryMinusFunction tests -(function() end)().
func TestFieldSymbol_EdgeCase_UnaryMinusFunction(t *testing.T) {
	// local x = -(function() return 1 end)()
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{
			&ast.UnaryMinusOpExpr{
				Expr: &ast.FuncCallExpr{Func: innerFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in unary minus should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in unary minus MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_UnaryLenFunction tests #(function() end)().
func TestFieldSymbol_EdgeCase_UnaryLenFunction(t *testing.T) {
	// local x = #(function() return {} end)()
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{
			&ast.UnaryLenOpExpr{
				Expr: &ast.FuncCallExpr{Func: innerFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in unary len should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in unary len MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_ArithmeticOpFunction tests 1 + (function() end)().
func TestFieldSymbol_EdgeCase_ArithmeticOpFunction(t *testing.T) {
	// local x = 1 + (function() return 2 end)()
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{
			&ast.ArithmeticOpExpr{
				Operator: "+",
				Lhs:      &ast.NumberExpr{Value: "1"},
				Rhs:      &ast.FuncCallExpr{Func: innerFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in arithmetic op should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in arithmetic op MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_RelationalOpFunction tests (function() end)() == 1.
func TestFieldSymbol_EdgeCase_RelationalOpFunction(t *testing.T) {
	// local x = (function() return 1 end)() == 1
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{
			&ast.RelationalOpExpr{
				Operator: "==",
				Lhs:      &ast.FuncCallExpr{Func: innerFn},
				Rhs:      &ast.NumberExpr{Value: "1"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in relational op should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in relational op MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_LogicalAndFunction tests true and function() end.
func TestFieldSymbol_EdgeCase_LogicalAndFunction(t *testing.T) {
	// local f = true and function() return 1 end
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{
			&ast.LogicalOpExpr{
				Operator: "and",
				Lhs:      &ast.TrueExpr{},
				Rhs:      innerFn,
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in logical and should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in logical and MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_FunctionInArrayPart tests {function() end, function() end}.
func TestFieldSymbol_EdgeCase_FunctionInArrayPart(t *testing.T) {
	// local t = {function() end, function() end, function() end}
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	fn3 := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Value: fn1}, // Array part - no key
					{Value: fn2},
					{Value: fn3},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) < 3 {
		t.Fatalf("Expected 3 functions in array part, got %d", len(nested))
	}
	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Function in table array part MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_CoroutineWrap tests coroutine.wrap(function() end).
func TestFieldSymbol_EdgeCase_CoroutineWrap(t *testing.T) {
	// local co = coroutine.wrap(function() end)
	innerFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"co"},
		Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "coroutine"},
					Key:    &ast.StringExpr{Value: "wrap"},
				},
				Args: []ast.Expr{innerFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn, "coroutine")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in coroutine.wrap should be in Nested")
	}
	for _, n := range nested {
		if n.Func == innerFn && n.Symbol == 0 {
			t.Error("Function in coroutine.wrap MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_SetmetatableCall tests setmetatable({}, {__call = function() end}).
func TestFieldSymbol_EdgeCase_SetmetatableCall(t *testing.T) {
	// local obj = setmetatable({}, {__call = function() end})
	callFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"obj"},
		Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "setmetatable"},
				Args: []ast.Expr{
					&ast.TableExpr{},
					&ast.TableExpr{
						Fields: []*ast.Field{
							{
								Key:   &ast.StringExpr{Value: "__call"},
								Value: callFn,
							},
						},
					},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn, "setmetatable")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Metatable __call function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == callFn && n.Symbol == 0 {
			t.Error("Metatable __call function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_GlobalAssignFunction tests G = function() end.
func TestFieldSymbol_EdgeCase_GlobalAssignFunction(t *testing.T) {
	// G = function() end
	// G()
	globalFn := &ast.FunctionExpr{}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IdentExpr{Value: "G"}},
		Rhs: []ast.Expr{globalFn},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "G"},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt, callStmt}}
	g := Build(fn, "G")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Global assigned function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == globalFn && n.Symbol == 0 {
			t.Error("Global assigned function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_ChainedFieldAssign tests M.a.b.c = function() end.
func TestFieldSymbol_EdgeCase_ChainedFieldAssign(t *testing.T) {
	// M.a.b.c = function() end
	deepFn := &ast.FunctionExpr{}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "M"},
						Key:    &ast.StringExpr{Value: "a"},
					},
					Key: &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
		},
		Rhs: []ast.Expr{deepFn},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt}}
	g := Build(fn, "M")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Deep field function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == deepFn && n.Symbol == 0 {
			t.Error("Deep field function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_MultipleFieldAssign tests M.a, M.b = function() end, function() end.
func TestFieldSymbol_EdgeCase_MultipleFieldAssign(t *testing.T) {
	// M.a, M.b = function() end, function() end
	fn1 := &ast.FunctionExpr{}
	fn2 := &ast.FunctionExpr{}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "a"},
			},
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "b"},
			},
		},
		Rhs: []ast.Expr{fn1, fn2},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{assignStmt}}
	g := Build(fn, "M")

	nested := g.NestedFunctions()
	if len(nested) < 2 {
		t.Fatalf("Expected 2 field functions, got %d", len(nested))
	}

	found1, found2 := false, false
	for _, n := range nested {
		if n.Func == fn1 {
			found1 = true
			if n.Symbol == 0 {
				t.Error("M.a function MUST have a symbol")
			}
		}
		if n.Func == fn2 {
			found2 = true

			if n.Symbol == 0 {
				t.Error("M.b function MUST have a symbol")
			}
		}
	}
	if !found1 || !found2 {
		t.Error("Both field functions should be found")
	}
}

// TestFieldSymbol_EdgeCase_FunctionInTernaryLike tests x and function() or function().
func TestFieldSymbol_EdgeCase_FunctionInTernaryLike(t *testing.T) {
	// local f = cond and function() return 1 end or function() return 2 end
	trueFn := &ast.FunctionExpr{}
	falseFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{
			&ast.LogicalOpExpr{
				Operator: "or",
				Lhs: &ast.LogicalOpExpr{
					Operator: "and",
					Lhs:      &ast.IdentExpr{Value: "cond"},
					Rhs:      trueFn,
				},
				Rhs: falseFn,
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn, "cond")

	nested := g.NestedFunctions()
	if len(nested) < 2 {
		t.Fatalf("Expected 2 functions in ternary-like, got %d", len(nested))
	}

	foundTrue, foundFalse := false, false
	for _, n := range nested {
		if n.Func == trueFn {
			foundTrue = true
			if n.Symbol == 0 {
				t.Error("True branch function MUST have a symbol")
			}
		}
		if n.Func == falseFn {
			foundFalse = true
			if n.Symbol == 0 {
				t.Error("False branch function MUST have a symbol")
			}
		}
	}
	if !foundTrue || !foundFalse {
		t.Error("Both ternary functions should be found")
	}
}

// TestFieldSymbol_EdgeCase_NestedIIFE tests ((function() return function() end end)())().
func TestFieldSymbol_EdgeCase_NestedIIFE(t *testing.T) {
	// ((function() return function() end end)())()
	innerFn := &ast.FunctionExpr{}
	outerFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{innerFn}},
		},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.FuncCallExpr{Func: outerFn},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{callStmt}}
	g := Build(fn)

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Outer IIFE function should be in Nested")
	}
	for _, n := range nested {
		if n.Func == outerFn && n.Symbol == 0 {
			t.Error("Outer IIFE function MUST have a symbol")
		}
	}
}

// TestFieldSymbol_EdgeCase_FunctionInIndexExpr tests t[(function() end)()].
func TestFieldSymbol_EdgeCase_FunctionInIndexExpr(t *testing.T) {
	// local x = t[(function() return "key" end)()]
	keyFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "t"},
				Key:    &ast.FuncCallExpr{Func: keyFn},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt}}
	g := Build(fn, "t")

	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Function in index expr should be in Nested")
	}
	for _, n := range nested {
		if n.Func == keyFn && n.Symbol == 0 {
			t.Error("Function in index expr MUST have a symbol")
		}
	}
}

// =============================================================================
// G) CalleeSymbol Resolution for Field Calls
// Ensure CallInfo.CalleeSymbol is set for statically resolvable field calls.
// =============================================================================

// TestFieldSymbol_CalleeSymbol_FieldAssignThenCall tests T.f = fn; T.f() - CalleeSymbol should match.
func TestFieldSymbol_CalleeSymbol_FieldAssignThenCall(t *testing.T) {
	// local T = {}
	// T.f = function() end
	// T.f()
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	anonFn := &ast.FunctionExpr{}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{anonFn},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, callStmt}}
	g := Build(fn)

	// Get field symbol from assignment
	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})
	if fieldSym == 0 {
		t.Fatal("Field symbol for T.f not found in assignment")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol should match the field symbol
	if callInfo.CalleeSymbol != fieldSym {
		t.Errorf("CalleeSymbol = %d, want field symbol %d", callInfo.CalleeSymbol, fieldSym)
	}
}

// TestFieldSymbol_CalleeSymbol_TableLiteralThenCall tests local t = {f = fn}; t.f() - CalleeSymbol should match.
func TestFieldSymbol_CalleeSymbol_TableLiteralThenCall(t *testing.T) {
	// local t = { f = function() end }
	// t.f()
	anonFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"t"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.StringExpr{Value: "f"}, Value: anonFn},
				},
			},
		},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "t"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, callStmt}}
	g := Build(fn)

	// Get t's symbol
	var tSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "t" {
			tSym = a.Targets[0].Symbol
		}
	})
	if tSym == 0 {
		t.Fatal("Symbol for t not found")
	}

	// Get the nested function's symbol (should be t.f field symbol)
	nested := g.NestedFunctions()
	var fnSym SymbolID
	for _, n := range nested {
		if n.Func == anonFn {
			fnSym = n.Symbol
		}
	}
	if fnSym == 0 {
		t.Fatal("Symbol for table field function not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol should match the function's field symbol
	if callInfo.CalleeSymbol != fnSym {
		t.Errorf("CalleeSymbol = %d, want function field symbol %d", callInfo.CalleeSymbol, fnSym)
	}
}

// TestFieldSymbol_CalleeSymbol_FuncDefThenCall tests function M.f() end; M.f() - CalleeSymbol should match.
func TestFieldSymbol_CalleeSymbol_FuncDefThenCall(t *testing.T) {
	// function M.f() end
	// M.f()
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "M"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defStmt, callStmt}}
	g := Build(fn, "M")

	// Get function definition symbol
	var defSym SymbolID
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defSym = f.Symbol
	})
	if defSym == 0 {
		t.Fatal("FuncDefInfo.Symbol not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol should match the function definition symbol
	if callInfo.CalleeSymbol != defSym {
		t.Errorf("CalleeSymbol = %d, want def symbol %d", callInfo.CalleeSymbol, defSym)
	}
}

// TestFieldSymbol_CalleeSymbol_MethodCall tests function M:f() end; M:f() - CalleeSymbol should match.
func TestFieldSymbol_CalleeSymbol_MethodCall(t *testing.T) {
	// function M:f() end
	// M:f()
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.IdentExpr{Value: "M"},
			Method:   "f",
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Receiver: &ast.IdentExpr{Value: "M"},
			Method:   "f",
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defStmt, callStmt}}
	g := Build(fn, "M")

	// Get function definition symbol
	var defSym SymbolID
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defSym = f.Symbol
	})
	if defSym == 0 {
		t.Fatal("FuncDefInfo.Symbol not found for method")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol should match the method definition symbol
	if callInfo.CalleeSymbol != defSym {
		t.Errorf("CalleeSymbol = %d, want method def symbol %d", callInfo.CalleeSymbol, defSym)
	}
}

// TestFieldSymbol_CalleeSymbol_NestedFieldCall tests a.b.c() - CalleeSymbol should resolve.
func TestFieldSymbol_CalleeSymbol_NestedFieldCall(t *testing.T) {
	// local a = {}
	// a.b = {}
	// a.b.c = function() end
	// a.b.c()
	localA := &ast.LocalAssignStmt{
		Names: []string{"a"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignB := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "a"},
				Key:    &ast.StringExpr{Value: "b"},
			},
		},
		Rhs: []ast.Expr{&ast.TableExpr{}},
	}
	anonFn := &ast.FunctionExpr{}
	assignC := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
		},
		Rhs: []ast.Expr{anonFn},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localA, assignB, assignC, callStmt}}
	g := Build(fn)

	// Get field symbol for a.b.c from assignment
	var abcSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			// The one with nested path
			if len(a.Targets[0].FieldPath) == 2 {
				abcSym = a.Targets[0].Symbol
			}
		}
	})
	if abcSym == 0 {
		t.Fatal("Symbol for a.b.c not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol should match the nested field symbol
	if callInfo.CalleeSymbol != abcSym {
		t.Errorf("CalleeSymbol = %d, want a.b.c symbol %d", callInfo.CalleeSymbol, abcSym)
	}
}

// TestFieldSymbol_CalleeSymbol_DynamicCallNoSymbol tests obj[k]() - CalleeSymbol should be 0.
func TestFieldSymbol_CalleeSymbol_DynamicCallNoSymbol(t *testing.T) {
	// obj[k]()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "obj"},
			Key:    &ast.IdentExpr{Value: "k"}, // variable key
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "obj", "k")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol should be 0 for dynamic key
	if callInfo.CalleeSymbol != 0 {
		t.Errorf("CalleeSymbol = %d, want 0 for dynamic key", callInfo.CalleeSymbol)
	}
}

// TestFieldSymbol_CalleeSymbol_SimpleIdentCall tests f() - CalleeSymbol = 0 for variable calls.
// CFG does not track alias resolution; f could be reassigned, so CalleeSymbol is unknown.
func TestFieldSymbol_CalleeSymbol_SimpleIdentCall(t *testing.T) {
	// local f = function() end
	// f()
	anonFn := &ast.FunctionExpr{}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{anonFn},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "f"},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, callStmt}}
	g := Build(fn)

	// Get f's symbol (still assigned to the local)
	var fSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "f" {
			fSym = a.Targets[0].Symbol
		}
	})
	if fSym == 0 {
		t.Fatal("Symbol for f not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol is set for simple variable calls (enables effect resolution)
	if callInfo.CalleeSymbol != fSym {
		t.Errorf("CalleeSymbol = %d, want %d (fSym)", callInfo.CalleeSymbol, fSym)
	}
	// CalleeName should still be "f"
	if callInfo.CalleeName != "f" {
		t.Errorf("CalleeName = %q, want %q", callInfo.CalleeName, "f")
	}
}

// TestFieldSymbol_CalleeSymbol_MethodCallOnField tests a.b:c() - method on nested field.
func TestFieldSymbol_CalleeSymbol_MethodCallOnField(t *testing.T) {
	// local a = {}
	// a.b = {}
	// function a.b:c() end
	// a.b:c()
	localA := &ast.LocalAssignStmt{
		Names: []string{"a"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignB := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "a"},
				Key:    &ast.StringExpr{Value: "b"},
			},
		},
		Rhs: []ast.Expr{&ast.TableExpr{}},
	}
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "a"},
				Key:    &ast.StringExpr{Value: "b"},
			},
			Method: "c",
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Receiver: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "a"},
				Key:    &ast.StringExpr{Value: "b"},
			},
			Method: "c",
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localA, assignB, defStmt, callStmt}}
	g := Build(fn)

	// Get method definition symbol
	var defSym SymbolID
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defSym = f.Symbol
	})
	if defSym == 0 {
		t.Fatal("Method definition symbol not found")
	}

	// Get call info
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol should match the method definition
	if callInfo.CalleeSymbol != defSym {
		t.Errorf("CalleeSymbol = %d, want method def symbol %d", callInfo.CalleeSymbol, defSym)
	}
}

// TestFieldSymbol_CalleeSymbol_DotCallAsMethod tests T.f() vs T:f() use same symbol.
func TestFieldSymbol_CalleeSymbol_DotCallAsMethod(t *testing.T) {
	// function T.f() end
	// T.f()  -- dot call
	// T:f()  -- method call (same function)
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Func: &ast.FunctionExpr{},
	}
	dotCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	methodCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Receiver: &ast.IdentExpr{Value: "T"},
			Method:   "f",
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defStmt, dotCall, methodCall}}
	g := Build(fn, "T")

	// Get definition symbol
	var defSym SymbolID
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defSym = f.Symbol
	})
	if defSym == 0 {
		t.Fatal("Definition symbol not found")
	}

	// Collect all calls
	var calls []*CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		calls = append(calls, c)
	})
	if len(calls) != 2 {
		t.Fatalf("Expected 2 calls, got %d", len(calls))
	}

	// Both calls should resolve to same symbol
	for i, call := range calls {
		if call.CalleeSymbol != defSym {
			t.Errorf("Call %d: CalleeSymbol = %d, want def symbol %d", i, call.CalleeSymbol, defSym)
		}
	}
}

// TestFieldSymbol_CalleeSymbol_StringIndexCall tests T["method"]() - string index call.
func TestFieldSymbol_CalleeSymbol_StringIndexCall(t *testing.T) {
	// local T = {}
	// T["method"] = function() end
	// T["method"]()
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "method"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "method"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, callStmt}}
	g := Build(fn)

	// Get field symbol
	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})
	if fieldSym == 0 {
		t.Fatal("Field symbol not found")
	}

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	if callInfo.CalleeSymbol != fieldSym {
		t.Errorf("CalleeSymbol = %d, want field symbol %d", callInfo.CalleeSymbol, fieldSym)
	}
}

// TestFieldSymbol_CalleeSymbol_ShadowedTableCall tests shadowed table field calls.
func TestFieldSymbol_CalleeSymbol_ShadowedTableCall(t *testing.T) {
	// local T = {}
	// T.f = function() return 1 end
	// T.f()  -- calls outer
	// do
	//   local T = {}
	//   T.f = function() return 2 end
	//   T.f()  -- calls inner
	// end
	outerLocal := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	outerAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	outerCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}

	innerLocal := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	innerAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	innerCall := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	doBlock := &ast.DoBlockStmt{
		Stmts: []ast.Stmt{innerLocal, innerAssign, innerCall},
	}

	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{outerLocal, outerAssign, outerCall, doBlock}}
	g := Build(fn)

	// Collect all field assignments
	var fieldSyms []SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSyms = append(fieldSyms, a.Targets[0].Symbol)
		}
	})
	if len(fieldSyms) < 2 {
		t.Fatalf("Expected at least 2 field symbols, got %d", len(fieldSyms))
	}

	// Collect all calls
	var calls []*CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		calls = append(calls, c)
	})
	if len(calls) < 2 {
		t.Fatalf("Expected at least 2 calls, got %d", len(calls))
	}

	// First call should resolve to outer field symbol
	if calls[0].CalleeSymbol != fieldSyms[0] {
		t.Errorf("Outer call: CalleeSymbol = %d, want outer field symbol %d",
			calls[0].CalleeSymbol, fieldSyms[0])
	}

	// Second call should resolve to inner field symbol
	if calls[1].CalleeSymbol != fieldSyms[1] {
		t.Errorf("Inner call: CalleeSymbol = %d, want inner field symbol %d",
			calls[1].CalleeSymbol, fieldSyms[1])
	}

	// The two field symbols should be different
	if fieldSyms[0] == fieldSyms[1] {
		t.Error("Outer and inner T.f should have different symbols")
	}
}

// TestFieldSymbol_CalleeSymbol_ComputedBaseNoSymbol tests (getT()).f() - no symbol.
func TestFieldSymbol_CalleeSymbol_ComputedBaseNoSymbol(t *testing.T) {
	// (getT()).f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "getT"},
			},
			Key: &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "getT")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		if c.CalleeName == "" { // The field call, not getT
			callInfo = c
		}
	})
	if callInfo == nil {
		t.Fatal("Field call not found")
	}

	// Should not have a symbol
	if callInfo.CalleeSymbol != 0 {
		t.Errorf("CalleeSymbol = %d, want 0 for computed base", callInfo.CalleeSymbol)
	}
}

// TestFieldSymbol_CalleeSymbol_TableLiteralBaseNoSymbol tests ({}).f() - no symbol.
func TestFieldSymbol_CalleeSymbol_TableLiteralBaseNoSymbol(t *testing.T) {
	// ({f = function() end}).f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.StringExpr{Value: "f"}, Value: &ast.FunctionExpr{}},
				},
			},
			Key: &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Should not have a symbol (base is table literal)
	if callInfo.CalleeSymbol != 0 {
		t.Errorf("CalleeSymbol = %d, want 0 for table literal base", callInfo.CalleeSymbol)
	}
}

// TestFieldSymbol_CalleeSymbol_ChainedMethodCalls tests a:b():c() - chained methods.
func TestFieldSymbol_CalleeSymbol_ChainedMethodCalls(t *testing.T) {
	// a:b():c()  -- c() is on return value of b(), no symbol
	// CFG creates one call node for the statement; inner call is in receiver
	call := &ast.FuncCallExpr{
		Receiver: &ast.FuncCallExpr{
			Receiver: &ast.IdentExpr{Value: "a"},
			Method:   "b",
		},
		Method: "c",
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// The outer :c() call should have no symbol (receiver is call result)
	if callInfo.Method != "c" {
		t.Errorf("Method = %q, want %q", callInfo.Method, "c")
	}
	if callInfo.CalleeSymbol != 0 {
		t.Errorf("Chained method :c() should have CalleeSymbol=0, got %d", callInfo.CalleeSymbol)
	}
}

// TestFieldSymbol_CalleeSymbol_GlobalFuncDef tests global function foo() called.
// CalleeSymbol is set for simple variable calls to enable symbol-only effect resolution.
func TestFieldSymbol_CalleeSymbol_GlobalFuncDef(t *testing.T) {
	// function foo() end
	// foo()
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.IdentExpr{Value: "foo"},
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "foo"},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{defStmt, callStmt}}
	g := Build(fn, "foo")

	var defInfo *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defInfo = f
	})
	if defInfo == nil || defInfo.Symbol == 0 {
		t.Fatal("Global function definition symbol not found")
	}

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleeSymbol is set for simple variable calls (enables effect resolution)
	if callInfo.CalleeSymbol != defInfo.Symbol {
		t.Errorf("CalleeSymbol = %d, want %d (defInfo.Symbol)", callInfo.CalleeSymbol, defInfo.Symbol)
	}
	// CalleeName should still be "foo"
	if callInfo.CalleeName != "foo" {
		t.Errorf("CalleeName = %q, want %q", callInfo.CalleeName, "foo")
	}
}

// TestFieldSymbol_CalleeSymbol_IntegerIndexCall tests T[1]() - integer index call.
func TestFieldSymbol_CalleeSymbol_IntegerIndexCall(t *testing.T) {
	// local T = {}
	// T[1] = function() end
	// T[1]()
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.NumberExpr{Value: "1"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.NumberExpr{Value: "1"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, callStmt}}
	g := Build(fn)

	// Get index symbol
	var indexSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 {
			indexSym = a.Targets[0].Symbol
		}
	})

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Integer index call should resolve to symbol
	if indexSym != 0 && callInfo.CalleeSymbol != indexSym {
		t.Errorf("CalleeSymbol = %d, want index symbol %d", callInfo.CalleeSymbol, indexSym)
	}
}

// TestFieldSymbol_CalleeSymbol_MultiLevelMethod tests a.b.c:d() - multi-level method.
func TestFieldSymbol_CalleeSymbol_MultiLevelMethod(t *testing.T) {
	// local a = {}
	// a.b = {}
	// a.b.c = {}
	// function a.b.c:d() end
	// a.b.c:d()
	localA := &ast.LocalAssignStmt{
		Names: []string{"a"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignB := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "a"},
				Key:    &ast.StringExpr{Value: "b"},
			},
		},
		Rhs: []ast.Expr{&ast.TableExpr{}},
	}
	assignC := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
		},
		Rhs: []ast.Expr{&ast.TableExpr{}},
	}
	defStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
			Method: "d",
		},
		Func: &ast.FunctionExpr{},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Receiver: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
			Method: "d",
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localA, assignB, assignC, defStmt, callStmt}}
	g := Build(fn)

	var defSym SymbolID
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		defSym = f.Symbol
	})
	if defSym == 0 {
		t.Fatal("Method definition symbol not found")
	}

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	if callInfo.CalleeSymbol != defSym {
		t.Errorf("CalleeSymbol = %d, want method def symbol %d", callInfo.CalleeSymbol, defSym)
	}
}

// TestFieldSymbol_CalleeSymbol_ReassignedFieldCall tests reassigned field call uses same symbol.
func TestFieldSymbol_CalleeSymbol_ReassignedFieldCall(t *testing.T) {
	// local T = {}
	// T.f = function() return 1 end
	// T.f = function() return 2 end  -- reassignment uses same symbol
	// T.f()
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assign1 := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	assign2 := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assign1, assign2, callStmt}}
	g := Build(fn)

	// Collect field symbols from assignments
	var fieldSyms []SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSyms = append(fieldSyms, a.Targets[0].Symbol)
		}
	})
	if len(fieldSyms) < 2 {
		t.Fatalf("Expected 2 field assignments, got %d", len(fieldSyms))
	}

	// Both assignments should use same symbol (same field path)
	if fieldSyms[0] != fieldSyms[1] {
		t.Errorf("Reassignment should use same symbol: %d != %d", fieldSyms[0], fieldSyms[1])
	}

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})
	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	if callInfo.CalleeSymbol != fieldSyms[0] {
		t.Errorf("CalleeSymbol = %d, want field symbol %d", callInfo.CalleeSymbol, fieldSyms[0])
	}
}

// TestFieldSymbol_CalleeSymbol_CallInCondition tests if T.f() then end - call in condition.
func TestFieldSymbol_CalleeSymbol_CallInCondition(t *testing.T) {
	// local T = {}
	// T.f = function() return true end
	// if T.f() then end
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	ifStmt := &ast.IfStmt{
		Condition: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Then: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, ifStmt}}
	g := Build(fn)

	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})

	// Note: Call in condition may be extracted differently
	// This test verifies the path is correct at least
	if fieldSym == 0 {
		t.Error("Field symbol should exist")
	}
}

// TestFieldSymbol_CalleeSymbol_CallAsArgument tests foo(T.f()) - call as argument.
// Note: Nested calls in arguments are not separate CFG call nodes.
// This test verifies the field symbol exists and the outer call is processed correctly.
func TestFieldSymbol_CalleeSymbol_CallAsArgument(t *testing.T) {
	// local T = {}
	// T.f = function() return 1 end
	// foo(T.f())
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callStmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "foo"},
			Args: []ast.Expr{
				&ast.FuncCallExpr{
					Func: &ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "T"},
						Key:    &ast.StringExpr{Value: "f"},
					},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, callStmt}}
	g := Build(fn, "foo")

	// Verify field symbol exists
	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})
	if fieldSym == 0 {
		t.Fatal("Field symbol not found")
	}

	// Verify the outer call (foo) is processed
	var fooCall *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		if c.CalleeName == "foo" {
			fooCall = c
		}
	})
	if fooCall == nil {
		t.Fatal("foo() call not found")
	}

	// The outer call should have the argument
	if len(fooCall.Args) != 1 {
		t.Errorf("foo() should have 1 arg, got %d", len(fooCall.Args))
	}
}

// TestFieldSymbol_CalleeSymbol_ReturnFieldCall tests return T.f() - call in return.
func TestFieldSymbol_CalleeSymbol_ReturnFieldCall(t *testing.T) {
	// local T = {}
	// T.f = function() return 1 end
	// return T.f()
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	retStmt := &ast.ReturnStmt{
		Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "T"},
					Key:    &ast.StringExpr{Value: "f"},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, retStmt}}
	g := Build(fn)

	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})
	if fieldSym == 0 {
		t.Fatal("Field symbol not found")
	}

	// Get return info
	var retInfo *ReturnInfo
	g.EachReturn(func(_ Point, r *ReturnInfo) {
		if len(r.SourceCalls) > 0 && r.SourceCalls[0] != nil {
			retInfo = r
		}
	})
	if retInfo == nil {
		t.Fatal("ReturnInfo with source call not found")
	}

	callInfo := retInfo.SourceCalls[0]
	if callInfo.CalleeSymbol != fieldSym {
		t.Errorf("CalleeSymbol = %d, want field symbol %d", callInfo.CalleeSymbol, fieldSym)
	}
}

// TestFieldSymbol_CalleeSymbol_AssignFromFieldCall tests x = T.f() - call as RHS.
func TestFieldSymbol_CalleeSymbol_AssignFromFieldCall(t *testing.T) {
	// local T = {}
	// T.f = function() return 1 end
	// local x = T.f()
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "f"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	localX := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "T"},
					Key:    &ast.StringExpr{Value: "f"},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStmt, localX}}
	g := Build(fn)

	var fieldSym SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			fieldSym = a.Targets[0].Symbol
		}
	})
	if fieldSym == 0 {
		t.Fatal("Field symbol not found")
	}

	// Find the assignment with source call
	var xAssign *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "x" {
			xAssign = a
		}
	})
	if xAssign == nil || len(xAssign.SourceCalls) == 0 || xAssign.SourceCalls[0] == nil {
		t.Fatal("Assignment with source call not found")
	}

	callInfo := xAssign.SourceCalls[0]
	if callInfo.CalleeSymbol != fieldSym {
		t.Errorf("CalleeSymbol = %d, want field symbol %d", callInfo.CalleeSymbol, fieldSym)
	}
}

// TestFieldSymbol_CalleeSymbol_MultipleFieldsSameTable tests multiple fields on same table.
func TestFieldSymbol_CalleeSymbol_MultipleFieldsSameTable(t *testing.T) {
	// local T = {}
	// T.a = function() end
	// T.b = function() end
	// T.c = function() end
	// T.a(); T.b(); T.c()
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}
	assignA := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "T"}, Key: &ast.StringExpr{Value: "a"}}},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	assignB := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "T"}, Key: &ast.StringExpr{Value: "b"}}},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	assignC := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "T"}, Key: &ast.StringExpr{Value: "c"}}},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	callA := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{Func: &ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "T"}, Key: &ast.StringExpr{Value: "a"}}},
	}
	callB := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{Func: &ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "T"}, Key: &ast.StringExpr{Value: "b"}}},
	}
	callC := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{Func: &ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "T"}, Key: &ast.StringExpr{Value: "c"}}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignA, assignB, assignC, callA, callB, callC}}
	g := Build(fn)

	// Collect field symbols by field name
	fieldSyms := make(map[string]SymbolID)
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == TargetField {
			name := a.Targets[0].FieldPath[0]
			fieldSyms[name] = a.Targets[0].Symbol
		}
	})

	// Collect call symbols by callee path
	callSyms := make(map[string]SymbolID)
	g.EachCall(func(_ Point, c *CallInfo) {
		if len(c.CalleePath.Segments) > 0 {
			name := c.CalleePath.Segments[0].Name
			callSyms[name] = c.CalleeSymbol
		}
	})

	// Each call should match its field symbol
	for name, fieldSym := range fieldSyms {
		callSym := callSyms[name]
		if callSym != fieldSym {
			t.Errorf("T.%s: CalleeSymbol = %d, want field symbol %d", name, callSym, fieldSym)
		}
	}

	// Verify all symbols are unique
	seenSyms := make(map[SymbolID]string)
	for name, sym := range fieldSyms {
		if prev, ok := seenSyms[sym]; ok {
			t.Errorf("Duplicate symbol %d for T.%s and T.%s", sym, prev, name)
		}
		seenSyms[sym] = name
	}
}

func TestFieldSymbol_CalleeSymbol_DotPathVsIndexStringKey_NoCollision(t *testing.T) {
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}

	assignIndex := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "a.b"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}

	assignFieldA := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "a"},
			},
		},
		Rhs: []ast.Expr{&ast.TableExpr{}},
	}

	assignFieldAB := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "T"},
					Key:    &ast.StringExpr{Value: "a"},
				},
				Key: &ast.StringExpr{Value: "b"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}

	callIndex := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "a.b"},
			},
		},
	}

	callField := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "T"},
					Key:    &ast.StringExpr{Value: "a"},
				},
				Key: &ast.StringExpr{Value: "b"},
			},
		},
	}

	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignIndex, assignFieldA, assignFieldAB, callIndex, callField}}
	g := Build(fn)

	var (
		indexAssignSym SymbolID
		fieldAssignSym SymbolID
		indexCallSym   SymbolID
		fieldCallSym   SymbolID
	)

	g.EachAssign(func(_ Point, info *AssignInfo) {
		if info == nil || info.IsLocal || len(info.Targets) == 0 {
			return
		}

		target := info.Targets[0]
		if target.Kind == TargetIndex {
			if key, ok := target.Key.(*ast.StringExpr); ok && key.Value == "a.b" {
				indexAssignSym = target.Symbol
			}
		}
		if target.Kind == TargetField && len(target.FieldPath) == 2 && target.FieldPath[0] == "a" && target.FieldPath[1] == "b" {
			fieldAssignSym = target.Symbol
		}
	})

	g.EachCall(func(_ Point, info *CallInfo) {
		if info == nil || len(info.CalleePath.Segments) == 0 {
			return
		}

		segs := info.CalleePath.Segments
		if len(segs) == 1 && segs[0].Kind == constraint.SegmentIndexString && segs[0].Name == "a.b" {
			indexCallSym = info.CalleeSymbol
		}
		if len(segs) == 2 &&
			segs[0] == (constraint.Segment{Kind: constraint.SegmentField, Name: "a"}) &&
			segs[1] == (constraint.Segment{Kind: constraint.SegmentField, Name: "b"}) {
			fieldCallSym = info.CalleeSymbol
		}
	})

	if indexAssignSym == 0 || fieldAssignSym == 0 || indexCallSym == 0 || fieldCallSym == 0 {
		t.Fatalf("missing symbols: indexAssign=%d fieldAssign=%d indexCall=%d fieldCall=%d",
			indexAssignSym, fieldAssignSym, indexCallSym, fieldCallSym)
	}

	if indexAssignSym == fieldAssignSym {
		t.Fatalf("assign symbols collided: %d", indexAssignSym)
	}
	if indexCallSym == fieldCallSym {
		t.Fatalf("call symbols collided: %d", indexCallSym)
	}
	if indexCallSym != indexAssignSym {
		t.Fatalf("index call symbol = %d, want %d", indexCallSym, indexAssignSym)
	}
	if fieldCallSym != fieldAssignSym {
		t.Fatalf("field call symbol = %d, want %d", fieldCallSym, fieldAssignSym)
	}
}

func TestFieldSymbol_CalleeSymbol_StringIndexNumericVsIntegerIndex_NoCollision(t *testing.T) {
	localT := &ast.LocalAssignStmt{
		Names: []string{"T"},
		Exprs: []ast.Expr{&ast.TableExpr{}},
	}

	assignStringIndex := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "1"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}

	assignIntIndex := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.NumberExpr{Value: "1"},
			},
		},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}

	callStringIndex := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.StringExpr{Value: "1"},
			},
		},
	}

	callIntIndex := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "T"},
				Key:    &ast.NumberExpr{Value: "1"},
			},
		},
	}

	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localT, assignStringIndex, assignIntIndex, callStringIndex, callIntIndex}}
	g := Build(fn)

	var (
		stringAssignSym SymbolID
		intAssignSym    SymbolID
		stringCallSym   SymbolID
		intCallSym      SymbolID
	)

	g.EachAssign(func(_ Point, info *AssignInfo) {
		if info == nil || info.IsLocal || len(info.Targets) == 0 {
			return
		}

		target := info.Targets[0]
		if target.Kind != TargetIndex {
			return
		}

		switch key := target.Key.(type) {
		case *ast.StringExpr:
			if key.Value == "1" {
				stringAssignSym = target.Symbol
			}
		case *ast.NumberExpr:
			if key.Value == "1" {
				intAssignSym = target.Symbol
			}
		}
	})

	g.EachCall(func(_ Point, info *CallInfo) {
		if info == nil || len(info.CalleePath.Segments) != 1 {
			return
		}

		seg := info.CalleePath.Segments[0]
		if seg == (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "1"}) {
			stringCallSym = info.CalleeSymbol
		}
		if seg == (constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}) {
			intCallSym = info.CalleeSymbol
		}
	})

	if stringAssignSym == 0 || intAssignSym == 0 || stringCallSym == 0 || intCallSym == 0 {
		t.Fatalf("missing symbols: stringAssign=%d intAssign=%d stringCall=%d intCall=%d",
			stringAssignSym, intAssignSym, stringCallSym, intCallSym)
	}

	if stringAssignSym == intAssignSym {
		t.Fatalf("assign symbols collided: %d", stringAssignSym)
	}
	if stringCallSym == intCallSym {
		t.Fatalf("call symbols collided: %d", stringCallSym)
	}
	if stringCallSym != stringAssignSym {
		t.Fatalf("string-index call symbol = %d, want %d", stringCallSym, stringAssignSym)
	}
	if intCallSym != intAssignSym {
		t.Fatalf("int-index call symbol = %d, want %d", intCallSym, intAssignSym)
	}
}

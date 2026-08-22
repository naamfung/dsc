package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// TestCallInfo_CalleePath_GlobalFunction tests CalleePath for global function calls.
func TestCallInfo_CalleePath_GlobalFunction(t *testing.T) {
	t.Parallel()

	// f()
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if info.CalleePath.Root != "f" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "f")
	}
	if info.CalleePath.Symbol == 0 {
		t.Error("CalleePath.Symbol should be non-zero for global")
	}
	if len(info.CalleePath.Segments) != 0 {
		t.Errorf("CalleePath should have no segments, got %d", len(info.CalleePath.Segments))
	}
	if info.Method != "" {
		t.Errorf("Method should be empty, got %q", info.Method)
	}
}

// TestCallInfo_CalleePath_LocalFunction tests CalleePath for local function calls.
func TestCallInfo_CalleePath_LocalFunction(t *testing.T) {
	t.Parallel()

	// local f = function() end; f()
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{&ast.FunctionExpr{}},
	}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	callStmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, callStmt}}
	g := Build(fn)

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if info.CalleePath.Root != "f" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "f")
	}
	if info.CalleePath.Symbol == 0 {
		t.Error("CalleePath.Symbol should be non-zero for local")
	}
	if len(info.CalleePath.Segments) != 0 {
		t.Errorf("CalleePath should have no segments, got %d", len(info.CalleePath.Segments))
	}
}

// TestCallInfo_CalleePath_FieldAccess tests CalleePath for obj.f() calls.
func TestCallInfo_CalleePath_FieldAccess(t *testing.T) {
	t.Parallel()

	// obj.f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "obj"},
			Key:    &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "obj")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if info.CalleePath.Root != "obj" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "obj")
	}
	if info.CalleePath.Symbol == 0 {
		t.Error("CalleePath.Symbol should be non-zero")
	}
	if len(info.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(info.CalleePath.Segments))
	}
	if info.CalleePath.Segments[0].Kind != constraint.SegmentField || info.CalleePath.Segments[0].Name != "f" {
		t.Errorf("CalleePath segment = %+v, want Field:f", info.CalleePath.Segments[0])
	}
	if info.Method != "" {
		t.Errorf("Method should be empty for dot call, got %q", info.Method)
	}
}

// TestCallInfo_CalleePath_MethodCall tests CalleePath for obj:f() calls.
func TestCallInfo_CalleePath_MethodCall(t *testing.T) {
	t.Parallel()

	// obj:f()
	call := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "obj"},
		Method:   "f",
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "obj")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// For method calls, CalleePath is the receiver path only
	if info.CalleePath.Root != "obj" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "obj")
	}
	if info.CalleePath.Symbol == 0 {
		t.Error("CalleePath.Symbol should be non-zero")
	}
	if len(info.CalleePath.Segments) != 0 {
		t.Errorf("CalleePath should have no segments for method call, got %d", len(info.CalleePath.Segments))
	}
	if info.Method != "f" {
		t.Errorf("Method = %q, want %q", info.Method, "f")
	}
}

// TestCallInfo_CalleePath_ChainedFieldAccess tests CalleePath for a.b.c() calls.
func TestCallInfo_CalleePath_ChainedFieldAccess(t *testing.T) {
	t.Parallel()

	// a.b.c()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "a"},
				Key:    &ast.StringExpr{Value: "b"},
			},
			Key: &ast.StringExpr{Value: "c"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if info.CalleePath.Root != "a" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "a")
	}
	if len(info.CalleePath.Segments) != 2 {
		t.Fatalf("CalleePath should have 2 segments, got %d", len(info.CalleePath.Segments))
	}
	if info.CalleePath.Segments[0].Name != "b" || info.CalleePath.Segments[1].Name != "c" {
		t.Errorf("CalleePath segments = [%s, %s], want [b, c]",
			info.CalleePath.Segments[0].Name, info.CalleePath.Segments[1].Name)
	}
}

// TestCallInfo_CalleePath_ChainedMethodCall tests CalleePath for a.b:c() calls.
func TestCallInfo_CalleePath_ChainedMethodCall(t *testing.T) {
	t.Parallel()

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

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// For method call, CalleePath is receiver path: a.b
	if info.CalleePath.Root != "a" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "a")
	}
	if len(info.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(info.CalleePath.Segments))
	}
	if info.CalleePath.Segments[0].Name != "b" {
		t.Errorf("CalleePath segment = %s, want b", info.CalleePath.Segments[0].Name)
	}
	if info.Method != "c" {
		t.Errorf("Method = %q, want %q", info.Method, "c")
	}
}

// TestCallInfo_CalleePath_TableLiteral tests CalleePath for ({}).f() calls.
func TestCallInfo_CalleePath_TableLiteral(t *testing.T) {
	t.Parallel()

	// ({}).f() - should have empty CalleePath
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.TableExpr{},
			Key:    &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if !info.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for table literal call, got %s", info.CalleePath.String())
	}
}

// TestCallInfo_CalleePath_Shadowing tests that local shadows global in CalleePath.
func TestCallInfo_CalleePath_Shadowing(t *testing.T) {
	t.Parallel()

	// Global f exists, local f shadows it
	// local f = function() end
	// f()
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{&ast.FunctionExpr{}},
	}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	callStmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localStmt, callStmt}}

	// Build with global "f" seeded
	bindings := bind.Bind(fn, []string{"f"})
	g := BuildWithBindings(fn, bindings)

	// Get the local symbol
	var localSym basecfg.SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "f" {
			localSym = a.Targets[0].Symbol
		}
	})

	if localSym == 0 {
		t.Fatal("Local symbol for f not found")
	}

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath.Symbol should be the local symbol, not global
	if callInfo.CalleePath.Symbol != localSym {
		t.Errorf("CalleePath.Symbol = %d, want local sym %d", callInfo.CalleePath.Symbol, localSym)
	}
}

// TestFuncDefInfo_TargetPath_Global tests TargetPath for global function definitions.
func TestFuncDefInfo_TargetPath_Global(t *testing.T) {
	t.Parallel()

	// function f() end
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: &ast.IdentExpr{Value: "f"}},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f")

	var info *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		info = f
	})

	if info == nil {
		t.Fatal("FuncDefInfo not found")
	}

	if info.TargetPath.Root != "f" {
		t.Errorf("TargetPath.Root = %q, want %q", info.TargetPath.Root, "f")
	}
	if info.TargetPath.Symbol == 0 {
		t.Error("TargetPath.Symbol should be non-zero")
	}
	if len(info.TargetPath.Segments) != 0 {
		t.Errorf("TargetPath should have no segments, got %d", len(info.TargetPath.Segments))
	}
}

// TestFuncDefInfo_TargetPath_Field tests TargetPath for function M.f() definitions.
func TestFuncDefInfo_TargetPath_Field(t *testing.T) {
	t.Parallel()

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

	if info.TargetPath.Root != "M" {
		t.Errorf("TargetPath.Root = %q, want %q", info.TargetPath.Root, "M")
	}
	if len(info.TargetPath.Segments) != 1 {
		t.Fatalf("TargetPath should have 1 segment, got %d", len(info.TargetPath.Segments))
	}
	if info.TargetPath.Segments[0].Name != "f" {
		t.Errorf("TargetPath segment = %s, want f", info.TargetPath.Segments[0].Name)
	}
}

// TestFuncDefInfo_TargetPath_Method tests TargetPath for function M:f() definitions.
func TestFuncDefInfo_TargetPath_Method(t *testing.T) {
	t.Parallel()

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

	// TargetPath for method is same as field: M.f
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

// TestFuncDefInfo_TargetPath_ChainedField tests TargetPath for function a.b.c() definitions.
func TestFuncDefInfo_TargetPath_ChainedField(t *testing.T) {
	t.Parallel()

	// function a.b.c() end
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
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
}

// TestGraph_CalleePathAt tests the Graph.CalleePathAt helper.
func TestGraph_CalleePathAt(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f")

	var callPoint Point
	g.EachCall(func(p Point, _ *CallInfo) {
		callPoint = p
	})

	path := g.CalleePathAt(callPoint)
	if path.Root != "f" {
		t.Errorf("CalleePathAt returned Root = %q, want %q", path.Root, "f")
	}

	// Non-call point should return empty path
	emptyPath := g.CalleePathAt(g.Entry())
	if !emptyPath.IsEmpty() {
		t.Errorf("CalleePathAt for non-call should be empty, got %s", emptyPath.String())
	}
}

// TestGraph_FuncDefPathAt tests the Graph.FuncDefPathAt helper.
func TestGraph_FuncDefPathAt(t *testing.T) {
	t.Parallel()

	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: &ast.IdentExpr{Value: "f"}},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f")

	var defPoint Point
	g.EachFuncDef(func(p Point, _ *FuncDefInfo) {
		defPoint = p
	})

	path := g.FuncDefPathAt(defPoint)
	if path.Root != "f" {
		t.Errorf("FuncDefPathAt returned Root = %q, want %q", path.Root, "f")
	}

	// Non-funcdef point should return empty path
	emptyPath := g.FuncDefPathAt(g.Entry())
	if !emptyPath.IsEmpty() {
		t.Errorf("FuncDefPathAt for non-funcdef should be empty, got %s", emptyPath.String())
	}
}

// TestLocalFunctionDefCall_SymbolMatch tests that local function def and call resolve to same symbol.
func TestLocalFunctionDefCall_SymbolMatch(t *testing.T) {
	t.Parallel()

	// local function f() end
	// f()
	localFn := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{&ast.FunctionExpr{}},
	}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	callStmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{localFn, callStmt}}
	g := Build(fn)

	// Get assignment symbol
	var assignSym basecfg.SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "f" {
			assignSym = a.Targets[0].Symbol
		}
	})

	if assignSym == 0 {
		t.Fatal("Assignment symbol not found")
	}

	// Get call symbol
	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	if callInfo.CalleePath.Symbol != assignSym {
		t.Errorf("Call symbol %d does not match assignment symbol %d",
			callInfo.CalleePath.Symbol, assignSym)
	}
}

// TestNestedLocalFunctions_SeparateSymbols tests nested local functions have separate symbols.
func TestNestedLocalFunctions_SeparateSymbols(t *testing.T) {
	t.Parallel()

	// local function outer()
	//   local function inner() end
	// end
	innerFn := &ast.LocalAssignStmt{
		Names: []string{"inner"},
		Exprs: []ast.Expr{&ast.FunctionExpr{}},
	}
	outerFn := &ast.LocalAssignStmt{
		Names: []string{"outer"},
		Exprs: []ast.Expr{&ast.FunctionExpr{Stmts: []ast.Stmt{innerFn}}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{outerFn}}
	g := Build(fn)

	// Get outer function symbol
	var outerSym basecfg.SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "outer" {
			outerSym = a.Targets[0].Symbol
		}
	})

	if outerSym == 0 {
		t.Fatal("Outer function symbol not found")
	}

	// Inner function is in nested graph, but outer should have its own symbol
	// This test verifies the outer graph has the outer symbol properly assigned
	if g.NameOf(outerSym) != "outer" {
		t.Errorf("NameOf(outerSym) = %q, want %q", g.NameOf(outerSym), "outer")
	}
}

// TestCallInfo_CalleePath_IndexAccess tests CalleePath for a[1]() and a["key"]() calls.
func TestCallInfo_CalleePath_IndexAccess(t *testing.T) {
	t.Parallel()

	// a[1]() - integer index
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "a"},
			Key:    &ast.NumberExpr{Value: "1"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if info.CalleePath.Root != "a" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "a")
	}
	if len(info.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(info.CalleePath.Segments))
	}
	if info.CalleePath.Segments[0].Kind != constraint.SegmentIndexInt {
		t.Errorf("segment kind = %v, want SegmentIndexInt", info.CalleePath.Segments[0].Kind)
	}
	if info.CalleePath.Segments[0].Index != 1 {
		t.Errorf("segment index = %d, want 1", info.CalleePath.Segments[0].Index)
	}
}

// TestCallInfo_CalleePath_StringIndex tests CalleePath for a["key"]() calls.
func TestCallInfo_CalleePath_StringIndex(t *testing.T) {
	t.Parallel()

	// a["non-ident-key"]() - string index that's not a valid identifier
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "a"},
			Key:    &ast.StringExpr{Value: "non-ident key"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if info.CalleePath.Root != "a" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "a")
	}
	if len(info.CalleePath.Segments) != 1 {
		t.Fatalf("CalleePath should have 1 segment, got %d", len(info.CalleePath.Segments))
	}
	if info.CalleePath.Segments[0].Kind != constraint.SegmentIndexString {
		t.Errorf("segment kind = %v, want SegmentIndexString", info.CalleePath.Segments[0].Kind)
	}
}

// TestCallInfo_CalleePath_NilCallee tests CalleePath when callee is nil.
func TestCallInfo_CalleePath_NilCallee(t *testing.T) {
	t.Parallel()

	// Method call has nil Callee but non-nil Receiver
	call := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "obj"},
		Method:   "m",
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "obj")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be receiver path for method calls
	if info.CalleePath.Root != "obj" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "obj")
	}
}

// TestCallInfo_CalleePath_DynamicCallee tests CalleePath for dynamic callees.
func TestCallInfo_CalleePath_DynamicCallee(t *testing.T) {
	t.Parallel()

	// (function() end)() - anonymous function call
	call := &ast.FuncCallExpr{
		Func: &ast.FunctionExpr{},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be empty for dynamic/anonymous callees
	if !info.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for anonymous function, got %s", info.CalleePath.String())
	}
}

// TestCallInfo_CalleePath_ReturnSourceCalls tests CalleePath in return statement calls.
func TestCallInfo_CalleePath_ReturnSourceCalls(t *testing.T) {
	t.Parallel()

	// return f()
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	ret := &ast.ReturnStmt{
		Exprs: []ast.Expr{call},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{ret}}
	g := Build(fn, "f")

	var retInfo *ReturnInfo
	g.EachReturn(func(_ Point, r *ReturnInfo) {
		if len(r.Exprs) > 0 {
			retInfo = r
		}
	})

	if retInfo == nil {
		t.Fatal("ReturnInfo not found")
	}

	if len(retInfo.SourceCalls) != 1 {
		t.Fatalf("SourceCalls len = %d, want 1", len(retInfo.SourceCalls))
	}

	callInfo := retInfo.SourceCalls[0]
	if callInfo.CalleePath.Root != "f" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "f")
	}
	if callInfo.CalleePath.Symbol == 0 {
		t.Error("CalleePath.Symbol should be non-zero")
	}
}

// TestCallInfo_CalleePath_AssignSourceCalls tests CalleePath in assignment source calls.
func TestCallInfo_CalleePath_AssignSourceCalls(t *testing.T) {
	t.Parallel()

	// local x = f()
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	stmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{call},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f")

	var assignInfo *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if len(a.SourceCalls) > 0 && a.SourceCalls[0] != nil {
			assignInfo = a
		}
	})

	if assignInfo == nil {
		t.Fatal("AssignInfo with source call not found")
	}

	callInfo := assignInfo.SourceCalls[0]
	if callInfo.CalleePath.Root != "f" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "f")
	}
}

// TestFuncDefInfo_TargetPath_EmptyReceiver tests TargetPath when receiver path is empty.
func TestFuncDefInfo_TargetPath_EmptyReceiver(t *testing.T) {
	t.Parallel()

	// function ({}):m() end - receiver is table literal, path should be empty
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.TableExpr{},
			Method:   "m",
		},
		Func: &ast.FunctionExpr{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	var info *FuncDefInfo
	g.EachFuncDef(func(_ Point, f *FuncDefInfo) {
		info = f
	})

	if info == nil {
		t.Fatal("FuncDefInfo not found")
	}

	// TargetPath should be empty when receiver is not resolvable
	if !info.TargetPath.IsEmpty() {
		t.Errorf("TargetPath should be empty for table literal receiver, got %s", info.TargetPath.String())
	}
}

// TestGraph_CalleePathAt_Nil tests CalleePathAt on nil graph.
func TestGraph_CalleePathAt_Nil(t *testing.T) {
	t.Parallel()

	var g *Graph
	path := g.CalleePathAt(0)
	if !path.IsEmpty() {
		t.Errorf("CalleePathAt on nil graph should return empty path, got %s", path.String())
	}
}

// TestGraph_FuncDefPathAt_Nil tests FuncDefPathAt on nil graph.
func TestGraph_FuncDefPathAt_Nil(t *testing.T) {
	t.Parallel()

	var g *Graph
	path := g.FuncDefPathAt(0)
	if !path.IsEmpty() {
		t.Errorf("FuncDefPathAt on nil graph should return empty path, got %s", path.String())
	}
}

// TestCallInfo_CalleePath_MultipleCallsInBlock tests multiple calls maintain correct paths.
func TestCallInfo_CalleePath_MultipleCallsInBlock(t *testing.T) {
	t.Parallel()

	// f(); g(); h.m()
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}},
			&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "g"}}},
			&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "h"},
					Key:    &ast.StringExpr{Value: "m"},
				},
			}},
		},
	}
	g := Build(fn, "f", "g", "h")

	calls := make([]*CallInfo, 0)
	g.EachCall(func(_ Point, c *CallInfo) {
		calls = append(calls, c)
	})

	if len(calls) != 3 {
		t.Fatalf("Expected 3 calls, got %d", len(calls))
	}

	// Verify each call has correct path
	paths := make([]string, len(calls))
	for i, c := range calls {
		paths[i] = c.CalleePath.String()
	}

	expected := []string{"f", "g", "h.m"}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("Call %d: CalleePath = %q, want %q", i, paths[i], exp)
		}
	}
}

// TestCallInfo_CalleePath_VariableKey tests CalleePath for a[x]() where x is variable.
func TestCallInfo_CalleePath_VariableKey(t *testing.T) {
	t.Parallel()

	// a[x]() - variable key, should result in empty path (not statically resolvable)
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "a"},
			Key:    &ast.IdentExpr{Value: "x"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a", "x")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// Variable key means path cannot be statically resolved
	if !info.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for variable key, got %s", info.CalleePath.String())
	}
}

// TestCallInfo_CalleePath_CallResultMethod tests f():m() pattern.
func TestCallInfo_CalleePath_CallResultMethod(t *testing.T) {
	t.Parallel()

	// f():m() - method call on result of function call
	// CFG creates one call node per statement; nested call is the receiver expression
	innerCall := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	call := &ast.FuncCallExpr{
		Receiver: innerCall,
		Method:   "m",
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// Method is set
	if info.Method != "m" {
		t.Errorf("Method = %q, want %q", info.Method, "m")
	}

	// CalleePath should be empty because receiver is a call expression, not a path
	if !info.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for call result receiver, got %s", info.CalleePath.String())
	}
}

// TestCallInfo_CalleePath_CallResultField tests f().x() pattern.
func TestCallInfo_CalleePath_CallResultField(t *testing.T) {
	t.Parallel()

	// f().x() - field access on result of function call
	// CFG creates one call node per statement; the callee is AttrGetExpr on call result
	innerCall := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: innerCall,
			Key:    &ast.StringExpr{Value: "x"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// CalleePath should be empty because base of AttrGetExpr is a call expression
	if !info.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for call result base, got %s", info.CalleePath.String())
	}
}

// TestCallInfo_CalleePath_DeeplyChained tests a.b.c.d.e.f() pattern.
func TestCallInfo_CalleePath_DeeplyChained(t *testing.T) {
	t.Parallel()

	// a.b.c.d.e.f()
	call := &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.AttrGetExpr{
						Object: &ast.AttrGetExpr{
							Object: &ast.IdentExpr{Value: "a"},
							Key:    &ast.StringExpr{Value: "b"},
						},
						Key: &ast.StringExpr{Value: "c"},
					},
					Key: &ast.StringExpr{Value: "d"},
				},
				Key: &ast.StringExpr{Value: "e"},
			},
			Key: &ast.StringExpr{Value: "f"},
		},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "a")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	if info.CalleePath.Root != "a" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "a")
	}
	if len(info.CalleePath.Segments) != 5 {
		t.Fatalf("CalleePath should have 5 segments, got %d", len(info.CalleePath.Segments))
	}

	expected := []string{"b", "c", "d", "e", "f"}
	for i, exp := range expected {
		if info.CalleePath.Segments[i].Name != exp {
			t.Errorf("Segment[%d] = %q, want %q", i, info.CalleePath.Segments[i].Name, exp)
		}
	}
}

// TestCallInfo_CalleePath_Reassignment tests local f; f = fn; f() pattern.
func TestCallInfo_CalleePath_Reassignment(t *testing.T) {
	t.Parallel()

	// local f
	// f = function() end
	// f()
	decl := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: nil,
	}
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IdentExpr{Value: "f"}},
		Rhs: []ast.Expr{&ast.FunctionExpr{}},
	}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
	}
	callStmt := &ast.FuncCallStmt{Expr: call}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{decl, assign, callStmt}}
	g := Build(fn)

	// Get the local symbol from declaration
	var declSym basecfg.SymbolID
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "f" {
			declSym = a.Targets[0].Symbol
		}
	})

	if declSym == 0 {
		t.Fatal("Declaration symbol not found")
	}

	var callInfo *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}

	// Call should resolve to the same symbol as declaration
	if callInfo.CalleePath.Symbol != declSym {
		t.Errorf("CalleePath.Symbol = %d, want %d", callInfo.CalleePath.Symbol, declSym)
	}
}

// TestCallInfo_CalleePath_NestedCall tests f(g()) pattern.
func TestCallInfo_CalleePath_NestedCall(t *testing.T) {
	t.Parallel()

	// f(g()) - CFG creates one call node for the statement
	// Inner call g() is an argument expression, not a separate CFG node
	innerCall := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "g"},
	}
	outerCall := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
		Args: []ast.Expr{innerCall},
	}
	stmt := &ast.FuncCallStmt{Expr: outerCall}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn, "f", "g")

	var info *CallInfo
	g.EachCall(func(_ Point, c *CallInfo) {
		info = c
	})

	if info == nil {
		t.Fatal("CallInfo not found")
	}

	// Outer call f() has the CalleePath
	if info.CalleePath.Root != "f" {
		t.Errorf("CalleePath.Root = %q, want %q", info.CalleePath.Root, "f")
	}

	// Args should contain the inner call expression
	if len(info.Args) != 1 {
		t.Fatalf("Expected 1 arg, got %d", len(info.Args))
	}
}

// TestFuncDefInfo_TargetPath_LocalFunction tests local function f() end.
func TestFuncDefInfo_TargetPath_LocalFunction(t *testing.T) {
	t.Parallel()

	// local function f() end (sugar for local f; f = function() end)
	// This is represented as LocalAssignStmt with function, not FuncDefStmt
	stmt := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{&ast.FunctionExpr{}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	g := Build(fn)

	// Local function is an AssignInfo, not FuncDefInfo
	var info *AssignInfo
	g.EachAssign(func(_ Point, a *AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "f" {
			info = a
		}
	})

	if info == nil {
		t.Fatal("AssignInfo for local function not found")
	}

	// Verify the target has a symbol
	if info.Targets[0].Symbol == 0 {
		t.Error("Local function target should have a symbol")
	}
}

// TestFuncDefInfo_TargetPath_ChainedMethod tests function a.b:c() end.
func TestFuncDefInfo_TargetPath_ChainedMethod(t *testing.T) {
	t.Parallel()

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

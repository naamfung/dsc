package bind

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
)

func TestNewBinder(t *testing.T) {
	b := NewBinder(nil)
	if b == nil {
		t.Fatal("NewBinder returned nil")
	}
	if b.table == nil {
		t.Error("table not initialized")
	}
	if len(b.stack) != 1 {
		t.Errorf("stack should have 1 frame, got %d", len(b.stack))
	}
	if b.globals == nil {
		t.Error("globals not initialized")
	}
}

func TestNewBinder_WithGlobals(t *testing.T) {
	globals := []string{"print", "pairs", "ipairs"}
	b := NewBinder(globals)

	for _, name := range globals {
		sym, ok := b.lookup(name)
		if !ok {
			t.Errorf("global %q not found in scope", name)
			continue
		}
		if _, exists := b.globals[name]; !exists {
			t.Errorf("global %q not in globals map", name)
		}
		kind, ok := b.table.Kind(sym)
		if !ok || kind != cfg.SymbolGlobal {
			t.Errorf("global %q should have kind SymbolGlobal", name)
		}
		if got := b.table.Name(sym); got != name {
			t.Errorf("global %q has name %q", name, got)
		}
	}
}

func TestBinder_EnterExitScope(t *testing.T) {
	b := NewBinder(nil)
	initial := len(b.stack)

	b.enterScope()
	if len(b.stack) != initial+1 {
		t.Errorf("enterScope: stack should have %d frames, got %d", initial+1, len(b.stack))
	}

	b.enterScope()
	if len(b.stack) != initial+2 {
		t.Errorf("enterScope: stack should have %d frames, got %d", initial+2, len(b.stack))
	}

	b.exitScope()
	if len(b.stack) != initial+1 {
		t.Errorf("exitScope: stack should have %d frames, got %d", initial+1, len(b.stack))
	}

	b.exitScope()
	if len(b.stack) != initial {
		t.Errorf("exitScope: stack should have %d frames, got %d", initial, len(b.stack))
	}

	// Exit beyond initial should not go below 1
	b.exitScope()
	if len(b.stack) < 1 {
		t.Error("exitScope should not reduce stack below 1")
	}
}

func TestBinder_Lookup(t *testing.T) {
	b := NewBinder([]string{"print"})

	// Lookup existing global
	sym, ok := b.lookup("print")
	if !ok {
		t.Error("lookup should find global")
	}
	if sym == 0 {
		t.Error("lookup returned zero symbol for global")
	}

	// Lookup non-existent
	if _, ok := b.lookup("nonexistent"); ok {
		t.Error("lookup should return false for non-existent")
	}

	// Declare local and verify shadowing
	b.enterScope()
	localSym := b.declareLocal("print")
	foundSym, ok := b.lookup("print")
	if !ok {
		t.Error("lookup should find shadowed local")
	}
	if foundSym != localSym {
		t.Error("lookup should return local symbol, not global")
	}

	// Exit scope and verify global is back
	b.exitScope()
	foundSym, ok = b.lookup("print")
	if !ok {
		t.Error("lookup should find global after exitScope")
	}
	if foundSym != sym {
		t.Error("lookup should return global after exitScope")
	}
}

func TestBinder_DeclareParam(t *testing.T) {
	b := NewBinder(nil)
	b.enterScope()
	sym := b.declareParam("x")

	if sym == 0 {
		t.Error("declareParam returned zero symbol")
	}
	foundSym, ok := b.lookup("x")
	if !ok || foundSym != sym {
		t.Error("declared param not found in scope")
	}
	kind, ok := b.table.Kind(sym)
	if !ok || kind != cfg.SymbolParam {
		t.Error("param should have kind SymbolParam")
	}
	if got := b.table.Name(sym); got != "x" {
		t.Errorf("param name = %q, want %q", got, "x")
	}
}

func TestBinder_DeclareLocal(t *testing.T) {
	b := NewBinder(nil)
	b.enterScope()
	sym := b.declareLocal("x")

	if sym == 0 {
		t.Error("declareLocal returned zero symbol")
	}
	foundSym, ok := b.lookup("x")
	if !ok || foundSym != sym {
		t.Error("declared local not found in scope")
	}
	kind, ok := b.table.Kind(sym)
	if !ok || kind != cfg.SymbolLocal {
		t.Error("local should have kind SymbolLocal")
	}
	if got := b.table.Name(sym); got != "x" {
		t.Errorf("local name = %q, want %q", got, "x")
	}
}

func TestBinder_DeclareGlobal(t *testing.T) {
	b := NewBinder(nil)
	sym := b.declareGlobal("g")

	if sym == 0 {
		t.Error("declareGlobal returned zero symbol")
	}
	if _, ok := b.globals["g"]; !ok {
		t.Error("global not in globals map")
	}
	kind, ok := b.table.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		t.Error("global should have kind SymbolGlobal")
	}

	// Redeclaring same global returns same symbol
	sym2 := b.declareGlobal("g")
	if sym2 != sym {
		t.Error("redeclaring global should return same symbol")
	}
}

func TestBind_SimpleFunction(t *testing.T) {
	// function(x, y) return x + y end
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x", "y"},
		},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.ArithmeticOpExpr{
						Operator: "+",
						Lhs:      &ast.IdentExpr{Value: "x"},
						Rhs:      &ast.IdentExpr{Value: "y"},
					},
				},
			},
		},
	}

	table := Bind(fn, nil)

	// Check param symbols
	paramSyms := table.ParamSymbols(fn)
	if len(paramSyms) != 2 {
		t.Fatalf("expected 2 param symbols, got %d", len(paramSyms))
	}

	// Check param kinds
	for i, sym := range paramSyms {
		kind, ok := table.Kind(sym)
		if !ok || kind != cfg.SymbolParam {
			t.Errorf("param %d should have kind SymbolParam", i)
		}
	}

	// Check names
	if got := table.Name(paramSyms[0]); got != "x" {
		t.Errorf("param 0 name = %q, want %q", got, "x")
	}
	if got := table.Name(paramSyms[1]); got != "y" {
		t.Errorf("param 1 name = %q, want %q", got, "y")
	}
}

func TestBind_LocalAssignment(t *testing.T) {
	// function() local a, b = 1, 2 end
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"a", "b"},
		Exprs: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.NumberExpr{Value: "2"},
		},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{localStmt},
	}

	table := Bind(fn, nil)

	localSyms := table.LocalSymbols(localStmt)
	if len(localSyms) != 2 {
		t.Fatalf("expected 2 local symbols, got %d", len(localSyms))
	}

	for i, sym := range localSyms {
		kind, ok := table.Kind(sym)
		if !ok || kind != cfg.SymbolLocal {
			t.Errorf("local %d should have kind SymbolLocal", i)
		}
	}
}

func TestBind_GlobalReference(t *testing.T) {
	// function() print("hello") end
	printIdent := &ast.IdentExpr{Value: "print"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: printIdent,
					Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
				},
			},
		},
	}

	table := Bind(fn, []string{"print"})

	sym, ok := table.SymbolOf(printIdent)
	if !ok {
		t.Fatal("print ident not bound")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		t.Error("print should have kind SymbolGlobal")
	}
}

func TestBind_ImplicitGlobal(t *testing.T) {
	// function() foo = 1 end
	fooIdent := &ast.IdentExpr{Value: "foo"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{fooIdent},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
		},
	}

	table := Bind(fn, nil)

	sym, ok := table.SymbolOf(fooIdent)
	if !ok {
		t.Fatal("foo ident not bound")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		t.Error("foo should have kind SymbolGlobal (implicit)")
	}
}

func TestBind_NumberFor(t *testing.T) {
	// function() for i = 1, 10 do end end
	forStmt := &ast.NumberForStmt{
		Name:  "i",
		Init:  &ast.NumberExpr{Value: "1"},
		Limit: &ast.NumberExpr{Value: "10"},
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{forStmt},
	}

	table := Bind(fn, nil)

	sym, ok := table.NumForSymbol(forStmt)
	if !ok {
		t.Fatal("for loop symbol not set")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolLocal {
		t.Error("for loop var should have kind SymbolLocal")
	}
	if got := table.Name(sym); got != "i" {
		t.Errorf("for loop var name = %q, want %q", got, "i")
	}
}

func TestBind_GenericFor(t *testing.T) {
	// function() for k, v in pairs(t) do end end
	pairsIdent := &ast.IdentExpr{Value: "pairs"}
	tIdent := &ast.IdentExpr{Value: "t"}
	forStmt := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: pairsIdent,
				Args: []ast.Expr{tIdent},
			},
		},
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{forStmt},
	}

	table := Bind(fn, []string{"pairs"})

	syms := table.GenericForSymbols(forStmt)
	if len(syms) != 2 {
		t.Fatalf("expected 2 for symbols, got %d", len(syms))
	}

	for i, sym := range syms {
		kind, ok := table.Kind(sym)
		if !ok || kind != cfg.SymbolLocal {
			t.Errorf("for var %d should have kind SymbolLocal", i)
		}
	}
	if got := table.Name(syms[0]); got != "k" {
		t.Errorf("for var 0 name = %q, want %q", got, "k")
	}
	if got := table.Name(syms[1]); got != "v" {
		t.Errorf("for var 1 name = %q, want %q", got, "v")
	}

	// t should be bound as global
	sym, ok := table.SymbolOf(tIdent)
	if !ok {
		t.Fatal("t ident not bound")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		t.Error("t should have kind SymbolGlobal")
	}
}

func TestBind_NestedScopes(t *testing.T) {
	// function()
	//   local x = 1
	//   do
	//     local x = 2  -- shadows outer x
	//   end
	// end
	outerLocal := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}
	innerLocal := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
	}
	// Reference outer x after do block
	outerRef := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			outerLocal,
			&ast.DoBlockStmt{
				Stmts: []ast.Stmt{innerLocal},
			},
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "print"},
					Args: []ast.Expr{outerRef},
				},
			},
		},
	}

	table := Bind(fn, []string{"print"})

	outerSyms := table.LocalSymbols(outerLocal)
	innerSyms := table.LocalSymbols(innerLocal)
	if len(outerSyms) != 1 || len(innerSyms) != 1 {
		t.Fatal("expected 1 symbol for each local stmt")
	}

	// Inner and outer x should have different symbols
	if outerSyms[0] == innerSyms[0] {
		t.Error("inner x should shadow outer x with different symbol")
	}

	// Reference after do block should bind to outer x
	refSym, ok := table.SymbolOf(outerRef)
	if !ok {
		t.Fatal("outerRef not bound")
	}
	if refSym != outerSyms[0] {
		t.Error("reference after do block should bind to outer x")
	}
}

func TestBind_WhileStmt(t *testing.T) {
	condIdent := &ast.IdentExpr{Value: "cond"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.WhileStmt{
				Condition: condIdent,
				Stmts:     []ast.Stmt{},
			},
		},
	}

	table := Bind(fn, nil)

	sym, ok := table.SymbolOf(condIdent)
	if !ok {
		t.Fatal("cond ident not bound")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		t.Error("cond should have kind SymbolGlobal")
	}
}

func TestBind_RepeatStmt(t *testing.T) {
	// repeat until x -- x is visible from inside the loop
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{&ast.TrueExpr{}},
	}
	condIdent := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.RepeatStmt{
				Condition: condIdent,
				Stmts:     []ast.Stmt{localStmt},
			},
		},
	}

	table := Bind(fn, nil)

	localSyms := table.LocalSymbols(localStmt)
	if len(localSyms) != 1 {
		t.Fatal("expected 1 local symbol")
	}

	// Condition should bind to the local declared inside
	condSym, ok := table.SymbolOf(condIdent)
	if !ok {
		t.Fatal("cond ident not bound")
	}
	if condSym != localSyms[0] {
		t.Error("repeat condition should bind to local declared inside loop")
	}
}

func TestBind_IfStmt(t *testing.T) {
	condIdent := &ast.IdentExpr{Value: "cond"}
	thenIdent := &ast.IdentExpr{Value: "thenVar"}
	elseIdent := &ast.IdentExpr{Value: "elseVar"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: condIdent,
				Then: []ast.Stmt{
					&ast.FuncCallStmt{
						Expr: &ast.FuncCallExpr{
							Func: &ast.IdentExpr{Value: "print"},
							Args: []ast.Expr{thenIdent},
						},
					},
				},
				Else: []ast.Stmt{
					&ast.FuncCallStmt{
						Expr: &ast.FuncCallExpr{
							Func: &ast.IdentExpr{Value: "print"},
							Args: []ast.Expr{elseIdent},
						},
					},
				},
			},
		},
	}

	table := Bind(fn, []string{"print"})

	for _, id := range []*ast.IdentExpr{condIdent, thenIdent, elseIdent} {
		if _, ok := table.SymbolOf(id); !ok {
			t.Errorf("ident %q not bound", id.Value)
		}
	}
}

func TestBind_FuncDefStmt(t *testing.T) {
	// function foo() end
	fooIdent := &ast.IdentExpr{Value: "foo"}
	innerFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Func: fooIdent,
				},
				Func: innerFn,
			},
		},
	}

	table := Bind(fn, nil)

	sym, ok := table.SymbolOf(fooIdent)
	if !ok {
		t.Fatal("foo ident not bound")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		t.Error("foo should have kind SymbolGlobal")
	}
}

func TestBind_MethodDef(t *testing.T) {
	// function obj:method() end
	objIdent := &ast.IdentExpr{Value: "obj"}
	innerFn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"self"},
		},
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Func:   objIdent,
					Method: "doThing",
				},
				Func: innerFn,
			},
		},
	}

	table := Bind(fn, nil)

	// obj should be bound as global
	sym, ok := table.SymbolOf(objIdent)
	if !ok {
		t.Fatal("obj ident not bound")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		t.Error("obj should have kind SymbolGlobal")
	}

	// self should be a parameter
	paramSyms := table.ParamSymbols(innerFn)
	if len(paramSyms) != 1 {
		t.Fatal("expected 1 param symbol for self")
	}
	kind, ok = table.Kind(paramSyms[0])
	if !ok || kind != cfg.SymbolParam {
		t.Error("self should have kind SymbolParam")
	}
}

func TestBind_TableExpr(t *testing.T) {
	// { x = y, [z] = w }
	yIdent := &ast.IdentExpr{Value: "y"}
	zIdent := &ast.IdentExpr{Value: "z"}
	wIdent := &ast.IdentExpr{Value: "w"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "use"},
					Args: []ast.Expr{
						&ast.TableExpr{
							Fields: []*ast.Field{
								{Key: &ast.StringExpr{Value: "x"}, Value: yIdent},
								{Key: zIdent, Value: wIdent},
							},
						},
					},
				},
			},
		},
	}

	table := Bind(fn, nil)

	for _, id := range []*ast.IdentExpr{yIdent, zIdent, wIdent} {
		if _, ok := table.SymbolOf(id); !ok {
			t.Errorf("ident %q not bound", id.Value)
		}
	}
}

func TestBind_AttrGetExpr(t *testing.T) {
	// obj.field or obj[key]
	objIdent := &ast.IdentExpr{Value: "obj"}
	keyIdent := &ast.IdentExpr{Value: "key"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "use"},
					Args: []ast.Expr{
						&ast.AttrGetExpr{
							Object: objIdent,
							Key:    keyIdent,
						},
					},
				},
			},
		},
	}

	table := Bind(fn, nil)

	for _, id := range []*ast.IdentExpr{objIdent, keyIdent} {
		if _, ok := table.SymbolOf(id); !ok {
			t.Errorf("ident %q not bound", id.Value)
		}
	}
}

func TestBind_UnaryOps(t *testing.T) {
	xIdent := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.UnaryMinusOpExpr{Expr: xIdent},
				},
			},
		},
	}

	table := Bind(fn, nil)

	if _, ok := table.SymbolOf(xIdent); !ok {
		t.Error("x ident not bound in unary op")
	}
}

func TestBind_LogicalOp(t *testing.T) {
	aIdent := &ast.IdentExpr{Value: "a"}
	bIdent := &ast.IdentExpr{Value: "b"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.LogicalOpExpr{
						Operator: "and",
						Lhs:      aIdent,
						Rhs:      bIdent,
					},
				},
			},
		},
	}

	table := Bind(fn, nil)

	for _, id := range []*ast.IdentExpr{aIdent, bIdent} {
		if _, ok := table.SymbolOf(id); !ok {
			t.Errorf("ident %q not bound", id.Value)
		}
	}
}

func TestBind_NilFunction(t *testing.T) {
	// Should not panic
	table := Bind(nil, nil)
	if table == nil {
		t.Error("Bind(nil) should return table")
	}
}

func TestBind_LocalsBeforeExprs(t *testing.T) {
	// local x = x  -- RHS x should bind to outer, LHS declares new local
	innerIdent := &ast.IdentExpr{Value: "x"}
	outerLocal := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}
	innerLocal := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{innerIdent},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			outerLocal,
			innerLocal,
		},
	}

	table := Bind(fn, nil)

	outerSyms := table.LocalSymbols(outerLocal)
	innerSyms := table.LocalSymbols(innerLocal)

	// The expression in innerLocal should bind to outerLocal's symbol
	refSym, ok := table.SymbolOf(innerIdent)
	if !ok {
		t.Fatal("innerIdent not bound")
	}
	if refSym != outerSyms[0] {
		t.Error("RHS of 'local x = x' should bind to outer x")
	}

	// But the declared local should be different
	if innerSyms[0] == outerSyms[0] {
		t.Error("LHS of 'local x = x' should create new symbol")
	}
}

func TestBind_AssignRhsBeforeLhs(t *testing.T) {
	// x = x + 1  -- both x's should bind to the same symbol
	lhsIdent := &ast.IdentExpr{Value: "x"}
	rhsIdent := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{lhsIdent},
				Rhs: []ast.Expr{
					&ast.ArithmeticOpExpr{
						Operator: "+",
						Lhs:      rhsIdent,
						Rhs:      &ast.NumberExpr{Value: "1"},
					},
				},
			},
		},
	}

	table := Bind(fn, nil)

	lhsSym, _ := table.SymbolOf(lhsIdent)
	rhsSym, _ := table.SymbolOf(rhsIdent)
	if lhsSym != rhsSym {
		t.Error("x = x + 1 should bind both x's to same symbol")
	}
}

func TestBind_NestedFunctions(t *testing.T) {
	// function()
	//   local x = 1
	//   return function() return x end
	// end
	outerLocal := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}
	innerXIdent := &ast.IdentExpr{Value: "x"}
	innerFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{innerXIdent},
			},
		},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			outerLocal,
			&ast.ReturnStmt{
				Exprs: []ast.Expr{innerFn},
			},
		},
	}

	table := Bind(fn, nil)

	outerSyms := table.LocalSymbols(outerLocal)
	innerSym, ok := table.SymbolOf(innerXIdent)
	if !ok {
		t.Fatal("inner x not bound")
	}

	// Inner function's x should refer to outer's x (closure)
	if innerSym != outerSyms[0] {
		t.Error("inner function should capture outer x")
	}
}

func TestBind_MethodDefImplicitSelf(t *testing.T) {
	// function obj:method(x) return self + x end
	// Method without explicit self in ParList - binder should add implicit self
	objIdent := &ast.IdentExpr{Value: "obj"}
	selfIdent := &ast.IdentExpr{Value: "self"}
	xIdent := &ast.IdentExpr{Value: "x"}
	innerFn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
		},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.ArithmeticOpExpr{
						Operator: "+",
						Lhs:      selfIdent,
						Rhs:      xIdent,
					},
				},
			},
		},
	}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Func:   objIdent,
					Method: "doThing",
				},
				Func: innerFn,
			},
		},
	}

	table := Bind(fn, nil)

	// Check implicit self was added as first param
	paramSyms := table.ParamSymbols(innerFn)
	if len(paramSyms) != 2 {
		t.Fatalf("expected 2 param symbols (self + x), got %d", len(paramSyms))
	}

	// First param should be self
	if got := table.Name(paramSyms[0]); got != "self" {
		t.Errorf("first param name = %q, want %q", got, "self")
	}
	// Second param should be x
	if got := table.Name(paramSyms[1]); got != "x" {
		t.Errorf("second param name = %q, want %q", got, "x")
	}

	// self reference in body should bind to the implicit self param
	selfSym, ok := table.SymbolOf(selfIdent)
	if !ok {
		t.Fatal("self ident not bound")
	}
	if selfSym != paramSyms[0] {
		t.Error("self reference should bind to implicit self param")
	}
}

func TestBind_CastExpr(t *testing.T) {
	xIdent := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.CastExpr{Expr: xIdent},
				},
			},
		},
	}

	table := Bind(fn, nil)

	if _, ok := table.SymbolOf(xIdent); !ok {
		t.Error("x ident not bound in cast expr")
	}
}

func TestBind_NonNilAssertExpr(t *testing.T) {
	xIdent := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.NonNilAssertExpr{Expr: xIdent},
				},
			},
		},
	}

	table := Bind(fn, nil)

	if _, ok := table.SymbolOf(xIdent); !ok {
		t.Error("x ident not bound in non-nil assert")
	}
}

func TestBind_RelationalOpExpr(t *testing.T) {
	aIdent := &ast.IdentExpr{Value: "a"}
	bIdent := &ast.IdentExpr{Value: "b"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.RelationalOpExpr{
						Operator: "<",
						Lhs:      aIdent,
						Rhs:      bIdent,
					},
				},
			},
		},
	}

	table := Bind(fn, nil)

	for _, id := range []*ast.IdentExpr{aIdent, bIdent} {
		if _, ok := table.SymbolOf(id); !ok {
			t.Errorf("ident %q not bound", id.Value)
		}
	}
}

func TestBind_StringConcatOpExpr(t *testing.T) {
	aIdent := &ast.IdentExpr{Value: "a"}
	bIdent := &ast.IdentExpr{Value: "b"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.StringConcatOpExpr{
						Lhs: aIdent,
						Rhs: bIdent,
					},
				},
			},
		},
	}

	table := Bind(fn, nil)

	for _, id := range []*ast.IdentExpr{aIdent, bIdent} {
		if _, ok := table.SymbolOf(id); !ok {
			t.Errorf("ident %q not bound", id.Value)
		}
	}
}

func TestBind_AllUnaryOps(t *testing.T) {
	tests := []struct {
		name string
		expr func(*ast.IdentExpr) ast.Expr
	}{
		{"UnaryMinus", func(id *ast.IdentExpr) ast.Expr { return &ast.UnaryMinusOpExpr{Expr: id} }},
		{"UnaryNot", func(id *ast.IdentExpr) ast.Expr { return &ast.UnaryNotOpExpr{Expr: id} }},
		{"UnaryLen", func(id *ast.IdentExpr) ast.Expr { return &ast.UnaryLenOpExpr{Expr: id} }},
		{"UnaryBNot", func(id *ast.IdentExpr) ast.Expr { return &ast.UnaryBNotOpExpr{Expr: id} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xIdent := &ast.IdentExpr{Value: "x"}
			fn := &ast.FunctionExpr{
				Stmts: []ast.Stmt{
					&ast.ReturnStmt{Exprs: []ast.Expr{tt.expr(xIdent)}},
				},
			}

			table := Bind(fn, nil)

			if _, ok := table.SymbolOf(xIdent); !ok {
				t.Errorf("x ident not bound in %s", tt.name)
			}
		})
	}
}

func TestBind_MutualRecursion(t *testing.T) {
	// local function even(n) return n == 0 or odd(n-1) end
	// local function odd(n) return n ~= 0 and even(n-1) end
	oddCallIdent := &ast.IdentExpr{Value: "odd"}
	evenCallIdent := &ast.IdentExpr{Value: "even"}

	evenLocal := &ast.LocalAssignStmt{
		Names: []string{"even"},
		Exprs: []ast.Expr{
			&ast.FunctionExpr{
				ParList: &ast.ParList{Names: []string{"n"}},
				Stmts: []ast.Stmt{
					&ast.ReturnStmt{
						Exprs: []ast.Expr{
							&ast.FuncCallExpr{Func: oddCallIdent},
						},
					},
				},
			},
		},
	}
	oddLocal := &ast.LocalAssignStmt{
		Names: []string{"odd"},
		Exprs: []ast.Expr{
			&ast.FunctionExpr{
				ParList: &ast.ParList{Names: []string{"n"}},
				Stmts: []ast.Stmt{
					&ast.ReturnStmt{
						Exprs: []ast.Expr{
							&ast.FuncCallExpr{Func: evenCallIdent},
						},
					},
				},
			},
		},
	}

	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{evenLocal, oddLocal},
	}

	table := Bind(fn, nil)

	evenSyms := table.LocalSymbols(evenLocal)
	oddSyms := table.LocalSymbols(oddLocal)
	if len(evenSyms) != 1 || len(oddSyms) != 1 {
		t.Fatal("expected 1 symbol for each local function")
	}

	// odd() call in even's body should bind to oddLocal's symbol
	oddCallSym, ok := table.SymbolOf(oddCallIdent)
	if !ok {
		t.Fatal("odd call not bound")
	}
	if oddCallSym != oddSyms[0] {
		t.Error("odd() in even should bind to local odd (mutual recursion)")
	}

	// even() call in odd's body should bind to evenLocal's symbol
	evenCallSym, ok := table.SymbolOf(evenCallIdent)
	if !ok {
		t.Fatal("even call not bound")
	}
	if evenCallSym != evenSyms[0] {
		t.Error("even() in odd should bind to local even (mutual recursion)")
	}
}

func TestNewBinder_DuplicateGlobals(t *testing.T) {
	b := NewBinder([]string{"print", "print", "pairs"})

	// Should not create duplicate symbols
	syms := b.table.Globals()
	names := make(map[string]bool)
	for _, sym := range syms {
		name := b.table.Name(sym)
		if names[name] {
			t.Errorf("duplicate global symbol for %q", name)
		}
		names[name] = true
	}
}

func TestNewBinder_EmptyGlobalName(t *testing.T) {
	b := NewBinder([]string{"", "print", ""})

	// Empty names should be skipped
	for _, sym := range b.table.Globals() {
		if name := b.table.Name(sym); name == "" {
			t.Error("empty global name should be skipped")
		}
	}
}

func TestBind_FuncCallReceiver(t *testing.T) {
	// obj:method(arg)
	objIdent := &ast.IdentExpr{Value: "obj"}
	argIdent := &ast.IdentExpr{Value: "arg"}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func:     objIdent,
					Receiver: objIdent,
					Method:   "method",
					Args:     []ast.Expr{argIdent},
				},
			},
		},
	}

	table := Bind(fn, nil)

	if _, ok := table.SymbolOf(objIdent); !ok {
		t.Error("obj ident not bound")
	}
	if _, ok := table.SymbolOf(argIdent); !ok {
		t.Error("arg ident not bound")
	}
}

func TestBind_FieldFuncDef(t *testing.T) {
	// function M.f() end
	mIdent := &ast.IdentExpr{Value: "M"}
	innerFn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Func:     mIdent,
					Receiver: &ast.AttrGetExpr{Object: mIdent, Key: &ast.StringExpr{Value: "f"}},
				},
				Func: innerFn,
			},
		},
	}

	table := Bind(fn, nil)

	if _, ok := table.SymbolOf(mIdent); !ok {
		t.Error("M ident not bound")
	}
}

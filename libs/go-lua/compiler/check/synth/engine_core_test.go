package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func newTestEngine() *Engine {
	return New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	})
}

func newTestEngineWithScope(sc *scope.State) *Engine {
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc
	return New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: scopes,
	})
}

func newTestEngineWithSymbol(name string, t typ.Type) (*Engine, *ast.IdentExpr) {
	ident := &ast.IdentExpr{Value: name}
	const sym = cfg.SymbolID(1)

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{name: sym}}
	declared := flow.DeclaredTypes{sym: t}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})
	return New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
	}), ident
}

func TestSynthExpr_Nil(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(nil, 0)
	if result != typ.Nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestSynthExpr_NilExpr(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.NilExpr{}, 0)
	if result != typ.Nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestSynthExpr_TrueExpr(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.TrueExpr{}, 0)
	if result != typ.True {
		t.Fatalf("got %v, want true", result)
	}
}

func TestSynthExpr_FalseExpr(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.FalseExpr{}, 0)
	if result != typ.False {
		t.Fatalf("got %v, want false", result)
	}
}

func TestSynthExpr_NumberExpr_Integer(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.NumberExpr{Value: "42"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("got %v, want integer literal", result)
	}
}

func TestSynthExpr_NumberExpr_Float(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.NumberExpr{Value: "3.14"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Number {
		t.Fatalf("got %v, want number literal", result)
	}
}

func TestSynthExpr_StringExpr(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.StringExpr{Value: "hello"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		t.Fatalf("got %v, want string literal", result)
	}
	if lit.Value != "hello" {
		t.Fatalf("got %q, want %q", lit.Value, "hello")
	}
}

func TestSynthExpr_IdentExpr_FromSymbolTypes(t *testing.T) {
	e, ident := newTestEngineWithSymbol("x", typ.Integer)

	result := e.TypeOf(ident, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthExpr_IdentExpr_Narrowed(t *testing.T) {
	const symX = cfg.SymbolID(100)
	ident := &ast.IdentExpr{Value: "x"}

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, symX)

	flowQ := mockFlowOps{narrowed: map[cfg.SymbolID]typ.Type{symX: typ.String}}
	graph := mockGraph{symbols: map[string]cfg.SymbolID{"x": symX}}
	declared := flow.DeclaredTypes{symX: typ.NewOptional(typ.String)}
	checkCtx := api.NewNarrowEnv(api.NarrowEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   flowQ,
		Env:    checkCtx,
		Phase:  api.PhaseNarrowing,
	})

	result := e.Narrow().TypeOf(ident, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string (narrowed)", result)
	}
}

func TestSynthExpr_IdentExpr_NarrowedFunctionDoesNotWidenDeclaredFunction(t *testing.T) {
	const symFn = cfg.SymbolID(101)
	ident := &ast.IdentExpr{Value: "f"}

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, symFn)

	declaredFn := typ.Func().Returns(typ.Integer).Build()
	widenedFn := typ.NewUnion(
		declaredFn,
		typ.Func().Returns(typ.NewOptional(typ.Integer)).Build(),
	)

	flowQ := mockFlowOps{narrowed: map[cfg.SymbolID]typ.Type{symFn: widenedFn}}
	graph := mockGraph{symbols: map[string]cfg.SymbolID{"f": symFn}}
	declared := flow.DeclaredTypes{symFn: declaredFn}
	checkCtx := api.NewNarrowEnv(api.NarrowEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   flowQ,
		Env:    checkCtx,
		Phase:  api.PhaseNarrowing,
	})

	result := e.Narrow().TypeOf(ident, 0)
	if !typ.TypeEquals(result, declaredFn) {
		t.Fatalf("got %v, want declared function %v", result, declaredFn)
	}
}

func TestSynthExpr_RelationalOp(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}, 0)
	if result != typ.Boolean {
		t.Fatalf("got %v, want boolean", result)
	}
}

func TestSynthExpr_StringConcatOp(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.StringConcatOpExpr{
		Lhs: &ast.StringExpr{Value: "a"},
		Rhs: &ast.StringExpr{Value: "b"},
	}, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestSynthExpr_UnaryNotOp(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.UnaryNotOpExpr{
		Expr: &ast.TrueExpr{},
	}, 0)
	if result != typ.Boolean {
		t.Fatalf("got %v, want boolean", result)
	}
}

func TestSynthExpr_UnaryLenOp(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.UnaryLenOpExpr{
		Expr: &ast.StringExpr{Value: "test"},
	}, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthExpr_UnaryLenOp_Any(t *testing.T) {
	e, ident := newTestEngineWithSymbol("x", typ.Any)
	result := e.TypeOf(&ast.UnaryLenOpExpr{
		Expr: ident,
	}, 0)
	// # operator always returns integer in Lua, even on any
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthExpr_UnaryLenOp_UnsupportedReturnsNil(t *testing.T) {
	e, ident := newTestEngineWithSymbol("b", typ.Boolean)
	result := e.TypeOf(&ast.UnaryLenOpExpr{
		Expr: ident,
	}, 0)
	if result != nil {
		t.Fatalf("got %v, want nil for unsupported operand", result)
	}
}

func TestSynthMulti_SingleExpr(t *testing.T) {
	e := newTestEngine()
	result := e.MultiTypeOf(&ast.NumberExpr{Value: "42"}, 0)
	if len(result) != 1 {
		t.Fatalf("got %d types, want 1", len(result))
	}
	lit, ok := result[0].(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("got %v, want integer literal", result[0])
	}
}

func TestSynthMulti_Nil(t *testing.T) {
	e := newTestEngine()
	result := e.MultiTypeOf(nil, 0)
	if result != nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestSynthMulti_Comma3WithVariadic(t *testing.T) {
	sc := scope.New().WithVariadic(typ.String)
	e := newTestEngineWithScope(sc)

	result := e.MultiTypeOf(&ast.Comma3Expr{}, 0)
	if len(result) != 1 || result[0] != typ.String {
		t.Fatalf("got %v, want [string]", result)
	}
}

func TestSynthIdentCore_FlowNarrowing(t *testing.T) {
	const symX = cfg.SymbolID(101)
	ident := &ast.IdentExpr{Value: "x"}

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, symX)

	flowQ := mockFlowOps{narrowed: map[cfg.SymbolID]typ.Type{symX: typ.Integer}}
	graph := mockGraph{symbols: map[string]cfg.SymbolID{"x": symX}}
	declared := flow.DeclaredTypes{symX: typ.NewOptional(typ.Integer)}
	checkCtx := api.NewNarrowEnv(api.NarrowEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   flowQ,
		Env:    checkCtx,
		Phase:  api.PhaseNarrowing,
	})

	result := e.Narrow().TypeOf(ident, 5)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer (narrowed)", result)
	}
}

func TestSynthIdentCore_FromSymbolTypes(t *testing.T) {
	e, ident := newTestEngineWithSymbol("y", typ.Boolean)

	result := e.TypeOf(ident, 0)
	if result != typ.Boolean {
		t.Fatalf("got %v, want boolean", result)
	}
}

func TestSynthIdentCore_ReturnsUnknown(t *testing.T) {
	sc := scope.New()
	e := newTestEngineWithScope(sc)

	result := e.TypeOf(&ast.IdentExpr{Value: "unknown"}, 0)
	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

func TestSynthComma3_WithVariadic(t *testing.T) {
	sc := scope.New().WithVariadic(typ.Number)
	e := newTestEngineWithScope(sc)

	result := e.TypeOf(&ast.Comma3Expr{}, 0)
	if result != typ.Number {
		t.Fatalf("got %v, want number", result)
	}
}

func TestSynthComma3_NoVariadic(t *testing.T) {
	e := newTestEngine()

	result := e.TypeOf(&ast.Comma3Expr{}, 0)
	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

func TestDeclaredView_TypeOf(t *testing.T) {
	e := newTestEngine()

	result := e.TypeOf(&ast.NumberExpr{Value: "100"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("got %v, want integer literal", result)
	}
}

func TestDeclaredView_Caching(t *testing.T) {
	e := newTestEngine()

	expr := &ast.StringExpr{Value: "cached"}
	t1 := e.TypeOf(expr, 0)
	t2 := e.TypeOf(expr, 0)

	if t1 != t2 {
		t.Fatal("cached results should be identical")
	}
}

func TestNarrowView_TypeOf(t *testing.T) {
	const symX = cfg.SymbolID(102)
	ident := &ast.IdentExpr{Value: "x"}

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, symX)

	flowQ := mockFlowOps{narrowed: map[cfg.SymbolID]typ.Type{symX: typ.String}}
	graph := mockGraph{symbols: map[string]cfg.SymbolID{"x": symX}}
	declared := flow.DeclaredTypes{symX: typ.NewOptional(typ.String)}
	checkCtx := api.NewNarrowEnv(api.NarrowEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Flow:   flowQ,
		Env:    checkCtx,
		Phase:  api.PhaseNarrowing,
	})

	narrow := e.Narrow()
	if narrow == nil {
		t.Fatal("Narrow() returned nil")
	}

	result := narrow.TypeOf(ident, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string (narrowed)", result)
	}
}

func TestSynthExpr_UnaryBNotOp(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.UnaryBNotOpExpr{
		Expr: &ast.NumberExpr{Value: "42"},
	}, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthExpr_CastExpr(t *testing.T) {
	sc := scope.New().WithType("MyType", typ.String)
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: scopes,
	})

	result := e.TypeOf(&ast.CastExpr{
		Expr: &ast.NumberExpr{Value: "42"},
		Type: &ast.TypeRefExpr{Path: []string{"MyType"}},
	}, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestSynthExpr_CastExpr_Primitive(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.CastExpr{
		Expr: &ast.NumberExpr{Value: "42"},
		Type: &ast.PrimitiveTypeExpr{Name: "number"},
	}, 0)
	if result != typ.Number {
		t.Fatalf("got %v, want number", result)
	}
}

func TestSynthExpr_NonNilAssertExpr(t *testing.T) {
	e, ident := newTestEngineWithSymbol("x", typ.NewOptional(typ.String))

	result := e.TypeOf(&ast.NonNilAssertExpr{
		Expr: ident,
	}, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string (nil removed)", result)
	}
}

func TestSynthExpr_NonNilAssertExpr_Union(t *testing.T) {
	unionType := typ.NewUnion(typ.Nil, typ.String, typ.Integer)
	e, ident := newTestEngineWithSymbol("x", unionType)

	result := e.TypeOf(&ast.NonNilAssertExpr{
		Expr: ident,
	}, 0)

	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("got %T, want union", result)
	}
	for _, m := range union.Members {
		if m.Kind() == kind.Nil {
			t.Fatal("nil should be removed from union")
		}
	}
}

func TestSynthExpr_NonNilAssertExpr_FieldAccess(t *testing.T) {
	// Create a record type with optional data field: {type: string, data: string?}
	msgType := typ.NewRecord().
		Field("type", typ.String).
		OptField("data", typ.String).
		Build()

	e, ident := newTestEngineWithSymbol("msg", msgType)

	// Test msg.data! should return string (nil removed from string?)
	result := e.TypeOf(&ast.NonNilAssertExpr{
		Expr: &ast.AttrGetExpr{
			Object: ident,
			Key:    &ast.StringExpr{Value: "data"},
		},
	}, 0)

	if result != typ.String {
		t.Fatalf("got %v, want string (nil removed from optional field)", result)
	}
}

func TestDeclaredView_ResolveReturnTypes(t *testing.T) {
	sc := scope.New()
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: scopes,
	})

	types := []ast.TypeExpr{
		&ast.PrimitiveTypeExpr{Name: "string"},
		&ast.PrimitiveTypeExpr{Name: "number"},
	}

	result := e.ResolveReturnTypes(types, sc)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
	if result[0] != typ.String {
		t.Fatalf("result[0]: got %v, want string", result[0])
	}
	if result[1] != typ.Number {
		t.Fatalf("result[1]: got %v, want number", result[1])
	}
}

func TestDeclaredView_ResolveReturnTypes_Tuple(t *testing.T) {
	sc := scope.New()
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: scopes,
	})

	types := []ast.TypeExpr{
		&ast.TupleTypeExpr{
			Elements: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.PrimitiveTypeExpr{Name: "number"},
			},
		},
	}

	result := e.ResolveReturnTypes(types, sc)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2 (tuple should be flattened)", len(result))
	}
}

func TestDeclaredView_ExpandValues(t *testing.T) {
	sc := scope.New()
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: scopes,
	})

	exprs := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.StringExpr{Value: "hello"},
	}

	result := e.ExpandValues(exprs, 3, 0)
	if len(result) != 3 {
		t.Fatalf("got %d, want 3", len(result))
	}
	if result[2] != typ.Nil {
		t.Fatalf("result[2]: got %v, want nil (padding)", result[2])
	}
}

func TestDeclaredView_ExpandValues_Empty(t *testing.T) {
	e := newTestEngine()

	result := e.ExpandValues(nil, 2, 0)
	if result != nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestSynthExprAt_GlobalFromCFG(t *testing.T) {
	// Create the ident first so we can use it for binding lookup
	moduleIdent := &ast.IdentExpr{Value: "mymodule"}

	// Build a simple function that references a global
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: &ast.AttrGetExpr{
							Object: moduleIdent,
							Key:    &ast.StringExpr{Value: "foo"},
						},
					},
				},
			},
		},
	}

	// Build CFG with the global seeded
	graph := ccfg.Build(fn, "mymodule")
	if graph == nil {
		t.Fatal("graph is nil")
	}

	entry := graph.Entry()
	t.Logf("Entry point: %d", entry)

	// Check if mymodule is visible at entry
	sym, ok := graph.SymbolAt(entry, "mymodule")
	t.Logf("SymbolAt(entry, 'mymodule'): sym=%d, ok=%v", sym, ok)

	if !ok {
		t.Fatal("mymodule should be visible at entry point")
	}

	// Check visibility at ALL points
	for p := cfg.Point(0); p < cfg.Point(graph.Size()); p++ {
		node := graph.Node(p)
		if node == nil {
			continue
		}
		psym, pok := graph.SymbolAt(p, "mymodule")
		t.Logf("Point %d (kind=%v): SymbolAt(p, 'mymodule')=%d, ok=%v", p, node.Kind, psym, pok)
		if !pok {
			t.Errorf("mymodule should be visible at point %d", p)
		}
	}

	// Create engine with CheckCtx using DeclaredTypes
	moduleType := typ.NewInterface("mymodule", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.String).Build()},
	})

	declared := flow.DeclaredTypes{sym: moduleType}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      graph.Bindings(),
		DeclaredTypes: declared,
	})

	engine := New(Config{
		Ctx: db.NewQueryContext(db.New()),
		Env: checkCtx,
	})

	// Test synthesizing 'mymodule' at entry point using the same ident instance
	result := engine.SynthExprAt(moduleIdent, entry, nil)
	t.Logf("SynthExprAt(mymodule, entry): %v", result)

	if result == nil || result.Kind() == kind.Unknown {
		t.Errorf("expected mymodule to resolve to interface type, got %v", result)
	}
}

func TestSynthIdentCore_UsesSymbolID(t *testing.T) {
	// Verify that ident lookup uses SymbolID from bindings
	const symX = cfg.SymbolID(200)
	ident := &ast.IdentExpr{Value: "x"}

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, symX)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{"x": symX}}
	declared := flow.DeclaredTypes{symX: typ.String}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
	})

	// Should resolve via binding lookup
	result := e.TypeOf(ident, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string (from binding-based lookup)", result)
	}
}

func TestSynthIdentCore_PreflowIgnoresFlowNarrowing(t *testing.T) {
	// Verify that preflow phase (DeclaredEngine) ignores flow narrowing
	const symX = cfg.SymbolID(201)
	ident := &ast.IdentExpr{Value: "x"}

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, symX)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{"x": symX}}

	declared := flow.DeclaredTypes{symX: typ.NewOptional(typ.Integer)}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
		Phase:  api.PhaseScopeCompute,
	})

	// DeclaredEngine should return declared type, not narrowed type
	result := e.TypeOf(ident, 0)
	opt, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("got %T, want *typ.Optional (DeclaredEngine ignores narrowing)", result)
	}
	if opt.Inner != typ.Integer {
		t.Fatalf("got optional<%v>, want optional<integer>", opt.Inner)
	}
}

package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSynthNumber_Integer(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.NumberExpr{Value: "42"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("got %v, want integer literal", result)
	}
}

func TestSynthNumber_Float(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.NumberExpr{Value: "3.14"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Number {
		t.Fatalf("got %v, want number literal", result)
	}
}

func TestSynthNumber_Hex(t *testing.T) {
	e := newTestEngine()
	result := e.TypeOf(&ast.NumberExpr{Value: "0xFF"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("got %v, want integer literal", result)
	}
}

func TestSynthAttrGetCore_StringKey(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	const sym = cfg.SymbolID(1)
	objIdent := &ast.IdentExpr{Value: "obj"}

	bindings := bind.NewBindingTable()
	bindings.Bind(objIdent, sym)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{"obj": sym}}
	declared := flow.DeclaredTypes{sym: rec}
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

	expr := &ast.AttrGetExpr{
		Object: objIdent,
		Key:    &ast.StringExpr{Value: "name"},
	}

	result := e.TypeOf(expr, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestSynthAttrGetCore_NumberKey(t *testing.T) {
	arr := typ.NewArray(typ.Integer)
	const sym = cfg.SymbolID(1)
	arrIdent := &ast.IdentExpr{Value: "arr"}

	bindings := bind.NewBindingTable()
	bindings.Bind(arrIdent, sym)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{"arr": sym}}
	declared := flow.DeclaredTypes{sym: arr}
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

	expr := &ast.AttrGetExpr{
		Object: arrIdent,
		Key:    &ast.NumberExpr{Value: "1"},
	}

	result := e.TypeOf(expr, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthAttrGetCore_UnknownField(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	const sym = cfg.SymbolID(1)
	objIdent := &ast.IdentExpr{Value: "obj"}

	bindings := bind.NewBindingTable()
	bindings.Bind(objIdent, sym)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{"obj": sym}}
	declared := flow.DeclaredTypes{sym: rec}
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

	expr := &ast.AttrGetExpr{
		Object: objIdent,
		Key:    &ast.StringExpr{Value: "unknown"},
	}

	result := e.TypeOf(expr, 0)
	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

func TestSynthLogicalOpCore_And(t *testing.T) {
	e := newTestEngine()
	expr := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      &ast.TrueExpr{},
		Rhs:      &ast.StringExpr{Value: "hello"},
	}

	result := e.TypeOf(expr, 0)
	if result == nil {
		t.Fatal("got nil")
	}
}

func TestSynthLogicalOpCore_Or(t *testing.T) {
	e := newTestEngine()
	expr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      &ast.NilExpr{},
		Rhs:      &ast.StringExpr{Value: "default"},
	}

	result := e.TypeOf(expr, 0)
	if result == nil {
		t.Fatal("got nil")
	}
}

func TestSynthArithmeticOpCore(t *testing.T) {
	e := newTestEngine()
	expr := &ast.ArithmeticOpExpr{
		Operator: "+",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}

	result := e.TypeOf(expr, 0)
	if result.Kind() != kind.Number {
		t.Fatalf("got %v, want number", result)
	}
}

func TestSynthUnaryMinusCore(t *testing.T) {
	e := newTestEngine()
	expr := &ast.UnaryMinusOpExpr{
		Expr: &ast.NumberExpr{Value: "5"},
	}

	result := e.TypeOf(expr, 0)
	if result.Kind() != kind.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestKeyName_StringExpr(t *testing.T) {
	result := ast.KeyName(&ast.StringExpr{Value: "field"})
	if result != "field" {
		t.Fatalf("got %q, want %q", result, "field")
	}
}

func TestKeyName_IdentExpr(t *testing.T) {
	result := ast.KeyName(&ast.IdentExpr{Value: "key"})
	if result != "key" {
		t.Fatalf("got %q, want %q", result, "key")
	}
}

func TestKeyName_Other(t *testing.T) {
	result := ast.KeyName(&ast.NumberExpr{Value: "1"})
	if result != "" {
		t.Fatalf("got %q, want empty", result)
	}
}

func TestExpandValues_Empty(t *testing.T) {
	e := newTestEngine()
	result := e.ExpandValues(nil, 3, 0)
	if result != nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestExpandValues_SingleExpr(t *testing.T) {
	e := newTestEngine()
	exprs := []ast.Expr{&ast.NumberExpr{Value: "42"}}

	result := e.ExpandValues(exprs, 1, 0)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	lit, ok := result[0].(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("got %v, want integer literal", result[0])
	}
}

func TestExpandValues_PadWithNil(t *testing.T) {
	e := newTestEngine()
	exprs := []ast.Expr{&ast.NumberExpr{Value: "42"}}

	result := e.ExpandValues(exprs, 3, 0)
	if len(result) != 3 {
		t.Fatalf("got %d, want 3", len(result))
	}
	if result[1] != typ.Nil {
		t.Fatalf("second element: got %v, want nil", result[1])
	}
	if result[2] != typ.Nil {
		t.Fatalf("third element: got %v, want nil", result[2])
	}
}

func TestExpandValues_MultipleExprs(t *testing.T) {
	e := newTestEngine()
	exprs := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.StringExpr{Value: "two"},
	}

	result := e.ExpandValues(exprs, 2, 0)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
	lit, ok := result[0].(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("first: got %v, want integer literal", result[0])
	}
}

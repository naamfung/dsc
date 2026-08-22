package guard_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/guard"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTruthyPathKey_Equality(t *testing.T) {
	key1 := guard.TruthyPathKey{Symbol: 1, Field: "foo"}
	key2 := guard.TruthyPathKey{Symbol: 1, Field: "foo"}
	key3 := guard.TruthyPathKey{Symbol: 2, Field: "foo"}
	key4 := guard.TruthyPathKey{Symbol: 1, Field: "bar"}

	if key1 != key2 {
		t.Error("expected identical keys to be equal")
	}
	if key1 == key3 {
		t.Error("expected different symbols to produce different keys")
	}
	if key1 == key4 {
		t.Error("expected different fields to produce different keys")
	}
}

func TestCollectTruthyGuards_NilGraph(t *testing.T) {
	result := guard.CollectTruthyGuards(nil, nil)
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestCollectTruthyGuards_NilBindings(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NilExpr{}}},
		},
	}
	graph := cfg.Build(fn)
	result := guard.CollectTruthyGuards(graph, nil)
	if result != nil {
		t.Error("expected nil for nil bindings")
	}
}

func TestExtractTruthyPathKeys_NilExpr(t *testing.T) {
	result := guard.ExtractTruthyPathKeys(nil, nil)
	if result != nil {
		t.Error("expected nil for nil expr")
	}
}

func TestExtractTruthyPathKeys_NilBindings(t *testing.T) {
	result := guard.ExtractTruthyPathKeys(&ast.IdentExpr{Value: "x"}, nil)
	if result != nil {
		t.Error("expected nil for nil bindings")
	}
}

func TestExtractTruthyPathKeys_IdentExpr(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "x"},
				Then:      []ast.Stmt{&ast.ReturnStmt{}},
			},
		},
	}
	bindings := bind.Bind(fn, nil)

	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	condIdent := ifStmt.Condition.(*ast.IdentExpr)

	result := guard.ExtractTruthyPathKeys(condIdent, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	if result[0].Field != "" {
		t.Errorf("expected empty field for simple ident, got %q", result[0].Field)
	}
}

func TestExtractTruthyPathKeys_NestedAttrExpr(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "event"},
			Key:    &ast.StringExpr{Value: "payload"},
		},
		Key: &ast.StringExpr{Value: "from"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: expr, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)

	result := guard.ExtractTruthyPathKeys(expr, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	base := expr.Object.(*ast.AttrGetExpr).Object.(*ast.IdentExpr)
	sym, _ := bindings.SymbolOf(base)
	if result[0].Symbol != sym {
		t.Fatalf("expected symbol %d, got %d", sym, result[0].Symbol)
	}
	if result[0].Field != "payload.from" {
		t.Fatalf("expected field payload.from, got %q", result[0].Field)
	}
}

func TestExtractTruthyPathKeys_StaticIndexIntExpr(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "event"},
		Key:    &ast.NumberExpr{Value: "1"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: expr, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	result := guard.ExtractTruthyPathKeys(expr, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key for static index-int path, got %d", len(result))
	}
	if result[0].Field != "[1]" {
		t.Fatalf("expected [1], got %q", result[0].Field)
	}
}

func TestExtractTruthyPathKeys_StaticIndexStringExpr(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "event"},
		Key:    &ast.StringExpr{Value: "x-y"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: expr, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)

	result := guard.ExtractTruthyPathKeys(expr, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	if result[0].Field != "[\"x-y\"]" {
		t.Fatalf("expected [\"x-y\"], got %q", result[0].Field)
	}
}

func TestTruthyKeyFromExpr_DynamicIdentKeyRejected(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "event"},
		Key:    &ast.IdentExpr{Value: "k"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event", "k"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)

	if _, ok := guard.TruthyKeyFromExpr(expr, bindings); ok {
		t.Fatal("expected dynamic ident key to be rejected")
	}
}

func TestNarrowTableFieldsByGuard_NonRecord(t *testing.T) {
	result := guard.NarrowTableFieldsByGuard(typ.String, nil, 0, nil, nil, nil)
	if result != typ.String {
		t.Error("expected string type returned unchanged")
	}
}

func TestNarrowTableFieldsByGuard_NilRecord(t *testing.T) {
	result := guard.NarrowTableFieldsByGuard(nil, nil, 0, nil, nil, nil)
	if result != nil {
		t.Error("expected nil returned")
	}
}

func TestNarrowTableFieldsByGuard_EmptyGuards(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.String).Build()
	result := guard.NarrowTableFieldsByGuard(rec, &ast.TableExpr{}, 1, nil, nil, nil)
	if result != rec {
		t.Error("expected original record when no guards")
	}
}

func TestNarrowTableFieldsByGuard_NoMatchingGuards(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.NewOptional(typ.String)).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "x"}, Value: &ast.StringExpr{Value: "val"}},
		},
	}
	guards := map[cfg.Point]map[guard.TruthyPathKey]bool{
		1: {},
	}
	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, nil, guards, nil)
	if result != rec {
		t.Error("expected original record when no matching guards")
	}
}

func TestNarrowTableFieldsByGuard_MatchingNestedPath(t *testing.T) {
	valueExpr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "event"},
			Key:    &ast.StringExpr{Value: "payload"},
		},
		Key: &ast.StringExpr{Value: "from"},
	}
	rec := typ.NewRecord().Field("from", typ.NewOptional(typ.String)).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "from"},
				Value: valueExpr,
			},
		},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{tbl},
			},
		},
	}
	bindings := bind.Bind(fn, nil)
	eventBase := valueExpr.Object.(*ast.AttrGetExpr).Object.(*ast.IdentExpr)
	eventSym, _ := bindings.SymbolOf(eventBase)

	guards := map[cfg.Point]map[guard.TruthyPathKey]bool{
		1: {
			{Symbol: eventSym, Field: "payload.from"}: true,
		},
	}

	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, bindings, guards, nil)
	out, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", result)
	}
	if len(out.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(out.Fields))
	}
	if !typ.TypeEquals(out.Fields[0].Type, typ.String) {
		t.Fatalf("expected narrowed string field, got %s", out.Fields[0].Type.String())
	}
}

func TestCollectTypeGuards_TypeNotEqReturnPropagatesFallthrough(t *testing.T) {
	condExpr := &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{
				&ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "payload"},
					Key:    &ast.StringExpr{Value: "respond_to"},
				},
			},
		},
		Rhs: &ast.StringExpr{Value: "string"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"payload"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: condExpr,
				Then:      []ast.Stmt{&ast.ReturnStmt{}},
			},
			&ast.LocalAssignStmt{
				Names: []string{"topic"},
				Exprs: []ast.Expr{
					&ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "payload"},
						Key:    &ast.StringExpr{Value: "respond_to"},
					},
				},
			},
		},
	}
	graph := cfg.Build(fn)
	bindings := bind.Bind(fn, nil)
	guards := guard.CollectTypeGuards(graph, bindings)

	payloadSym, ok := bindings.SymbolOf(condExpr.Lhs.(*ast.FuncCallExpr).Args[0].(*ast.AttrGetExpr).Object.(*ast.IdentExpr))
	if !ok || payloadSym == 0 {
		t.Fatal("expected payload symbol")
	}
	wantKey := guard.TruthyPathKey{Symbol: payloadSym, Field: "respond_to"}
	wantType := narrow.BuiltinTypeKey("string")

	found := false
	for _, atPoint := range guards {
		if tk, ok := atPoint[wantKey]; ok && tk == wantType {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected propagated type guard %v -> %v", wantKey, wantType)
	}
}

func TestNarrowTableFieldsByGuard_TypeGuardNarrowsAny(t *testing.T) {
	valueExpr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "payload"},
		Key:    &ast.StringExpr{Value: "respond_to"},
	}
	rec := typ.NewRecord().Field("respond_to", typ.Any).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "respond_to"}, Value: valueExpr},
		},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"payload"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{tbl},
			},
		},
	}
	bindings := bind.Bind(fn, nil)
	payloadSym, _ := bindings.SymbolOf(valueExpr.Object.(*ast.IdentExpr))
	typeGuards := map[cfg.Point]map[guard.TruthyPathKey]narrow.TypeKey{
		1: {
			{Symbol: payloadSym, Field: "respond_to"}: narrow.BuiltinTypeKey("string"),
		},
	}

	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, bindings, nil, typeGuards)
	out, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if len(out.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(out.Fields))
	}
	if !typ.TypeEquals(out.Fields[0].Type, typ.String) {
		t.Fatalf("expected narrowed string field, got %s", out.Fields[0].Type.String())
	}
}

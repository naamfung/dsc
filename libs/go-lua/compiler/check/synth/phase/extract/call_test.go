package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallQuery_NilSynthesizer(t *testing.T) {
	q := CallQuery{s: nil}

	if _, ok := q.Method(nil, typ.String, "foo"); ok {
		t.Fatal("expected false for nil synthesizer")
	}
	if _, ok := q.Field(nil, typ.String, "foo"); ok {
		t.Fatal("expected false for nil synthesizer")
	}
	if _, ok := q.Index(nil, typ.String, typ.Integer); ok {
		t.Fatal("expected false for nil synthesizer")
	}
	if q.BinaryOp(nil, typ.Integer, "+", typ.Integer) != typ.Unknown {
		t.Fatal("expected unknown for nil synthesizer")
	}
	if q.UnaryOp(nil, "-", typ.Integer) != typ.Unknown {
		t.Fatal("expected unknown for nil synthesizer")
	}
}

func TestCallQuery_NilTypes(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    nil,
		Scopes:   make(api.ScopeMap),
		PreCache: make(api.Cache),
	}
	s := NewSynthesizer(deps, api.PhaseTypeResolution)
	q := CallQuery{s: s}

	if _, ok := q.Method(deps.Ctx, typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
	if _, ok := q.Field(deps.Ctx, typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
	if _, ok := q.Index(deps.Ctx, typ.String, typ.Integer); ok {
		t.Fatal("expected false for nil types")
	}
}

func TestCallQuery_WithMock(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    mockTypeQuerier{},
		Scopes:   make(api.ScopeMap),
		PreCache: make(api.Cache),
	}
	s := NewSynthesizer(deps, api.PhaseTypeResolution)
	q := CallQuery{s: s}

	rec := typ.NewRecord().
		Field("name", typ.String).
		Build()

	ft, ok := q.Field(deps.Ctx, rec, "name")
	if !ok {
		t.Fatal("expected field lookup to succeed")
	}
	if ft != typ.String {
		t.Fatalf("got %v, want string", ft)
	}
}

func TestGetCallQuery(t *testing.T) {
	s := newTestSynthesizer()
	q := s.GetCallQuery()
	if q == nil {
		t.Fatal("expected non-nil call query")
	}
}

func TestSynthArgs(t *testing.T) {
	exprs := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.StringExpr{Value: "hello"},
	}
	recurse := func(ex ast.Expr) typ.Type {
		switch e := ex.(type) {
		case *ast.NumberExpr:
			return ops.ParseNumber(e.Value)
		case *ast.StringExpr:
			return typ.LiteralString(e.Value)
		}
		return typ.Unknown
	}

	args := synthArgs(exprs, recurse)
	if len(args) != 2 {
		t.Fatalf("got %d args, want 2", len(args))
	}
}

func TestCopyTypes(t *testing.T) {
	types := []typ.Type{typ.String, typ.Integer}
	copied := CopyTypes(types)

	if len(copied) != 2 {
		t.Fatalf("got %d types, want 2", len(copied))
	}
	if copied[0] != typ.String || copied[1] != typ.Integer {
		t.Fatal("copy mismatch")
	}

	types[0] = typ.Boolean
	if copied[0] != typ.String {
		t.Fatal("copy not independent")
	}
}

func TestCopyTypes_Empty(t *testing.T) {
	copied := CopyTypes(nil)
	if copied != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestResolveMethodCallee_Nil(t *testing.T) {
	s := newTestSynthesizer()
	result := s.resolveMethodCallee(nil, "foo")
	if result != nil {
		t.Fatal("expected nil for nil receiver")
	}
}

func TestMethod_NilTypes(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    nil,
		Scopes:   make(api.ScopeMap),
		PreCache: make(api.Cache),
	}
	s := NewSynthesizer(deps, api.PhaseTypeResolution)

	if _, ok := s.Method(typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
}

func TestField_NilTypes(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    nil,
		Scopes:   make(api.ScopeMap),
		PreCache: make(api.Cache),
	}
	s := NewSynthesizer(deps, api.PhaseTypeResolution)

	if _, ok := s.Field(typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
}

func TestField_WithRecord(t *testing.T) {
	s := newTestSynthesizer()
	rec := typ.NewRecord().
		Field("name", typ.String).
		Build()

	ft, ok := s.Field(rec, "name")
	if !ok {
		t.Fatal("expected field lookup to succeed")
	}
	if ft != typ.String {
		t.Fatalf("got %v, want string", ft)
	}
}

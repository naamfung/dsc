package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

type synthAPIStub struct {
	expanded []typ.Type

	called   bool
	needed   int
	point    cfg.Point
	specSize int
}

func (s *synthAPIStub) TypeOf(ast.Expr, cfg.Point) typ.Type { return typ.Unknown }

func (s *synthAPIStub) ExpandValues([]ast.Expr, int, cfg.Point) []typ.Type { return nil }

func (s *synthAPIStub) InferIterVars([]ast.Expr, int, cfg.Point) []typ.Type { return nil }

func (s *synthAPIStub) ExpandValuesWithSpecTypes(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	s.called = true
	s.needed = needed
	s.point = p
	s.specSize = len(specTypes)
	return s.expanded
}

func (s *synthAPIStub) InferIterVarsWithSpecTypes([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type {
	return nil
}

func TestExpandedAssignValues_Guards(t *testing.T) {
	info := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Name: "x"}},
		Sources: []ast.Expr{&ast.IdentExpr{Value: "v"}},
	}
	if got := expandedAssignValues(nil, info, 1, nil); got != nil {
		t.Fatalf("nil synthAPI should return nil, got %v", got)
	}
	if got := expandedAssignValues(&synthAPIStub{}, nil, 1, nil); got != nil {
		t.Fatalf("nil info should return nil, got %v", got)
	}
	if got := expandedAssignValues(&synthAPIStub{}, &cfg.AssignInfo{}, 1, nil); got != nil {
		t.Fatalf("empty targets/sources should return nil, got %v", got)
	}
}

func TestExpandedAssignValues_DelegatesToSynthAPI(t *testing.T) {
	stub := &synthAPIStub{expanded: []typ.Type{typ.String, typ.Number}}
	info := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{
			{Kind: cfg.TargetIdent, Name: "a"},
			{Kind: cfg.TargetIdent, Name: "b"},
		},
		Sources: []ast.Expr{&ast.IdentExpr{Value: "f"}},
	}
	spec := api.SpecTypes{1: typ.String}
	got := expandedAssignValues(stub, info, 42, spec)
	if !stub.called {
		t.Fatal("expected ExpandValuesWithSpecTypes to be called")
	}
	if stub.needed != 2 {
		t.Fatalf("needed=%d, want 2", stub.needed)
	}
	if stub.point != 42 {
		t.Fatalf("point=%d, want 42", stub.point)
	}
	if stub.specSize != 1 {
		t.Fatalf("spec size=%d, want 1", stub.specSize)
	}
	if len(got) != 2 || got[0] != typ.String || got[1] != typ.Number {
		t.Fatalf("unexpected expanded values: %v", got)
	}
}

func TestAssignValueAt_Bounds(t *testing.T) {
	values := []typ.Type{typ.String}
	if got := assignValueAt(values, -1); got != nil {
		t.Fatalf("index -1 should be nil, got %v", got)
	}
	if got := assignValueAt(values, 1); got != nil {
		t.Fatalf("index 1 should be nil, got %v", got)
	}
	if got := assignValueAt(values, 0); got != typ.String {
		t.Fatalf("index 0 should be string, got %v", got)
	}
}

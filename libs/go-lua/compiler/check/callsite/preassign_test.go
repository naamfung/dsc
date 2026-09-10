package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPreAssignmentTypeAtJoin_JoinsPredecessorsOnly(t *testing.T) {
	graph, joinPoint := buildGraphWithJoinPoint(t)
	sym := cfg.SymbolID(9001)
	preds := graph.Predecessors(joinPoint)

	byPred := make(map[cfg.Point]typ.Type, len(preds))
	var expected typ.Type
	for i, pred := range preds {
		var pt typ.Type
		if i%2 == 0 {
			pt = typ.String
		} else {
			pt = typ.Number
		}
		byPred[pred] = pt
		if expected == nil {
			expected = pt
		} else {
			expected = typ.JoinPreferNonSoft(expected, pt)
		}
	}

	got := PreAssignmentTypeAtJoin(graph, joinPoint, sym, func(p cfg.Point, id cfg.SymbolID) (typ.Type, bool) {
		if id != sym {
			return nil, false
		}
		if t, ok := byPred[p]; ok {
			return t, true
		}
		// Point fallback must be ignored by predecessor-only join.
		if p == joinPoint {
			return typ.Boolean, true
		}
		return nil, false
	})

	if !typ.TypeEquals(got, expected) {
		t.Fatalf("pre-assignment join mismatch: got %v want %v", got, expected)
	}
}

func TestPreAssignmentTypeAtJoinOrPoint_FallsBackToPoint(t *testing.T) {
	graph, joinPoint := buildGraphWithJoinPoint(t)
	sym := cfg.SymbolID(9002)

	got := PreAssignmentTypeAtJoinOrPoint(graph, joinPoint, sym, func(p cfg.Point, id cfg.SymbolID) (typ.Type, bool) {
		if id != sym {
			return nil, false
		}
		if p == joinPoint {
			return typ.Integer, true
		}
		return nil, false
	})

	if !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("fallback type mismatch: got %v want %v", got, typ.Integer)
	}
}

func buildGraphWithJoinPoint(t *testing.T) (*cfg.Graph, cfg.Point) {
	t.Helper()

	code := `
if cond then
    local x = "a"
else
    local x = 1
end
return 0
`
	stmts, err := parse.ParseString(code, "preassign_join.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, "cond")
	if graph == nil {
		t.Fatal("expected graph")
	}

	var joinPoint cfg.Point
	for _, p := range graph.RPO() {
		if len(graph.Predecessors(p)) >= 2 {
			joinPoint = p
			break
		}
	}
	if joinPoint == 0 {
		t.Fatal("expected graph point with multiple predecessors")
	}
	return graph, joinPoint
}

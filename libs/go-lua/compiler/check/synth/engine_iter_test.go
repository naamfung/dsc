package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferIterVars_CustomIteratorFunction(t *testing.T) {
	// Create engine with iter defined as:
	// fun(arr: {number}): fun(): (integer?, number?)
	iterReturnType := typ.Func().
		Returns(typ.NewOptional(typ.Integer), typ.NewOptional(typ.Number)).
		Build()
	iterType := typ.Func().
		Param("arr", typ.NewArray(typ.Number)).
		Returns(iterReturnType).
		Build()

	const symIter = cfg.SymbolID(1)
	iterIdent := &ast.IdentExpr{Value: "iter"}

	bindings := bind.NewBindingTable()
	bindings.Bind(iterIdent, symIter)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{"iter": symIter}}
	declared := flow.DeclaredTypes{symIter: iterType}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
	})

	// Simulate the expression iter(arr)
	iterCall := &ast.FuncCallExpr{
		Func: iterIdent,
		Args: []ast.Expr{&ast.IdentExpr{Value: "arr"}},
	}

	// Test InferIterVars
	types := e.InferIterVars([]ast.Expr{iterCall}, 2, cfg.Point(1))
	if len(types) != 2 {
		t.Fatalf("got %d types, want 2", len(types))
	}

	// First variable should be optional<integer>
	if opt, ok := types[0].(*typ.Optional); !ok {
		t.Errorf("types[0] = %T, want *typ.Optional", types[0])
	} else if opt.Inner != typ.Integer {
		t.Errorf("types[0].Inner = %v, want integer", opt.Inner)
	}

	// Second variable should be optional<number>
	if opt, ok := types[1].(*typ.Optional); !ok {
		t.Errorf("types[1] = %T, want *typ.Optional", types[1])
	} else if opt.Inner != typ.Number {
		t.Errorf("types[1].Inner = %v, want number", opt.Inner)
	}
}

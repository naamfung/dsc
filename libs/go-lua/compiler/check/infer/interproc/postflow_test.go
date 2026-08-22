package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestStoreFactsFromResult_NilStore(t *testing.T) {
	StoreFactsFromResult(nil, nil, nil, nil)
}

func TestStoreFactsFromResult_NilResult(t *testing.T) {
	StoreFactsFromResult(nil, nil, nil, nil)
}

func TestStoreFactsFromResult_NilGraph(t *testing.T) {
	result := &api.FuncResult{}
	StoreFactsFromResult(nil, nil, result, nil)
}

func TestExpectedFunctionFromResult_UnannotatedParamsRemainOptional(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
		},
	}
	graph := cfg.Build(fn)
	slots := graph.ParamSlots()
	if len(slots) != 2 {
		t.Fatalf("expected 2 param slots, got %d", len(slots))
	}
	declared := map[cfg.SymbolID]typ.Type{
		slots[0].Symbol: typ.String,
		slots[1].Symbol: typ.String,
	}
	result := &api.FuncResult{
		Graph: graph,
		FlowInputs: &flow.Inputs{
			DeclaredTypes: declared,
		},
	}

	got := expectedFunctionFromResult(result)
	if got == nil {
		t.Fatal("expected function type")
	}
	if len(got.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(got.Params))
	}
	if !got.Params[0].Optional || !got.Params[1].Optional {
		t.Fatalf("expected both params optional, got %+v", got.Params)
	}
	if got.Variadic == nil || !typ.TypeEquals(got.Variadic, typ.Any) {
		t.Fatalf("expected variadic any for unannotated expected function, got %v", got.Variadic)
	}
}

func TestExpectedFunctionFromResult_UnannotatedUndeclaredDefaultsToUnknown(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
		},
	}
	graph := cfg.Build(fn)
	result := &api.FuncResult{
		Graph: graph,
		FlowInputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{},
		},
	}

	got := expectedFunctionFromResult(result)
	if got == nil {
		t.Fatal("expected function type")
	}
	if len(got.Params) != 1 {
		t.Fatalf("expected one param, got %d", len(got.Params))
	}
	if !got.Params[0].Optional {
		t.Fatalf("expected param optional, got %+v", got.Params[0])
	}
	if !typ.TypeEquals(got.Params[0].Type, typ.Unknown) {
		t.Fatalf("expected undeclared param type unknown, got %v", got.Params[0].Type)
	}
}

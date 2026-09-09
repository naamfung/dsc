package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEnrichTableTypeWithFuncTypes_NilInputs(t *testing.T) {
	result := EnrichTableTypeWithFuncTypes(nil, nil, nil, nil)
	if rec, ok := result.(*typ.Record); !ok || rec != nil {
		t.Error("expected nil record for nil inputs")
	}
}

func TestEnrichTableTypeWithFuncTypes_NilRecord(t *testing.T) {
	result := EnrichTableTypeWithFuncTypes(nil, nil, &cfg.Graph{}, nil)
	if rec, ok := result.(*typ.Record); !ok || rec != nil {
		t.Error("expected nil record for nil record input")
	}
}

func TestCollectCapturedFieldAssignments_NilGraph(t *testing.T) {
	result := CollectCapturedFieldAssignments(nil, nil, nil)
	if result == nil {
		t.Error("expected empty map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestCollectCapturedFieldAssignments_EmptyCapturedSyms(t *testing.T) {
	result := CollectCapturedFieldAssignments(&cfg.Graph{}, map[cfg.SymbolID]bool{}, nil)
	if result == nil {
		t.Error("expected empty map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilInputs(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, 0, nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilSelfType(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, 1, nil)
	if result != nil {
		t.Error("expected nil for nil selfType")
	}
}

func TestMergeFieldsIntoSelfType_EmptyFields(t *testing.T) {
	selfType := typ.Number
	result := mergeFieldsIntoSelfType(selfType, nil)
	if result != selfType {
		t.Errorf("expected original selfType for empty fields, got %v", result)
	}
}

func TestMergeFieldsIntoSelfType_NonRecordNonInterface(t *testing.T) {
	selfType := typ.Number
	fields := map[string]typ.Type{"x": typ.String}
	result := mergeFieldsIntoSelfType(selfType, fields)
	if result != selfType {
		t.Errorf("expected original selfType for non-record/interface, got %v", result)
	}
}

func TestCollectCapturedContainerMutations_AssignmentCallSite(t *testing.T) {
	code := `
		local c = {}
		local _ = send(c, 1)
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, "send")
	if graph == nil {
		t.Fatal("expected graph")
	}
	symC, ok := graph.SymbolAt(graph.Exit(), "c")
	if !ok || symC == 0 {
		t.Fatal("expected symbol for c")
	}

	captured := map[cfg.SymbolID]bool{symC: true}
	result := CollectCapturedContainerMutations(graph, captured, nestedContainerSendSynth())
	muts := result[symC]
	if len(muts) != 1 {
		t.Fatalf("expected 1 container mutation for c, got %d", len(muts))
	}
	if !typ.TypeEquals(muts[0].ValueType, typ.Integer) {
		t.Fatalf("expected integer mutation value, got %v", muts[0].ValueType)
	}
}

func nestedContainerSendSynth() func(ast.Expr, cfg.Point) typ.Type {
	spec := contract.NewSpec().WithEffects(effect.Mutate{
		Target: effect.ParamRef{Index: 0},
		Transform: effect.ContainerElementUnion{
			Container: effect.ParamRef{Index: 0},
			Value:     effect.ParamRef{Index: 1},
		},
	})
	send := typ.Func().
		Param("container", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Nil).
		Spec(spec).
		Build()

	return func(expr ast.Expr, _ cfg.Point) typ.Type {
		switch v := expr.(type) {
		case *ast.IdentExpr:
			if v.Value == "send" {
				return send
			}
		case *ast.NumberExpr:
			return typ.Integer
		}
		return typ.Unknown
	}
}

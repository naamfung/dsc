package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSpecReturnOverride_NilFnType_ReturnsNil(t *testing.T) {
	s := &SpecReturnOverride{Phase: api.PhaseScopeCompute}
	result := s.Override(nil, nil)
	if result != nil {
		t.Fatal("expected nil for nil fn type")
	}
}

func TestSpecReturnOverride_WrongPhase_TypeResolution(t *testing.T) {
	s := &SpecReturnOverride{Phase: api.PhaseTypeResolution}
	fn := typ.Func().Build()
	result := s.Override(fn, nil)
	if result != nil {
		t.Fatal("expected nil for wrong phase")
	}
}

func TestSpecReturnOverride_PhaseScopeCompute(t *testing.T) {
	s := &SpecReturnOverride{Phase: api.PhaseScopeCompute}
	fn := typ.Func().Build()
	result := s.Override(fn, nil)
	// Without spec, returns nil
	if result != nil {
		t.Fatal("expected nil for function without spec")
	}
}

func TestSpecReturnOverride_PhaseNarrowing(t *testing.T) {
	s := &SpecReturnOverride{Phase: api.PhaseNarrowing}
	fn := typ.Func().Build()
	result := s.Override(fn, nil)
	// Without spec, returns nil
	if result != nil {
		t.Fatal("expected nil for function without spec")
	}
}

func TestApplyOverride_NilOverride_ReturnsUnchanged(t *testing.T) {
	types := []typ.Type{typ.String, typ.Integer}
	result := ApplyOverride(types, nil)
	if len(result) != 2 {
		t.Fatal("expected unchanged length")
	}
	if result[0] != typ.String {
		t.Fatal("expected first type unchanged")
	}
	if result[1] != typ.Integer {
		t.Fatal("expected second type unchanged")
	}
}

func TestApplyOverride_EmptyTypes_ReturnsNil(t *testing.T) {
	result := ApplyOverride(nil, typ.Integer)
	if result != nil {
		t.Fatal("expected nil for empty types")
	}
}

func TestApplyOverride_EmptySlice(t *testing.T) {
	result := ApplyOverride([]typ.Type{}, typ.Integer)
	if len(result) != 0 {
		t.Fatal("expected empty slice")
	}
}

func TestApplyOverride_ReplacesFirst(t *testing.T) {
	types := []typ.Type{typ.String, typ.Boolean, typ.Number}
	result := ApplyOverride(types, typ.Integer)
	if result[0] != typ.Integer {
		t.Fatal("expected first type to be replaced")
	}
	if result[1] != typ.Boolean {
		t.Fatal("expected second type unchanged")
	}
	if result[2] != typ.Number {
		t.Fatal("expected third type unchanged")
	}
}

func TestApplyOverride_SingleElement(t *testing.T) {
	types := []typ.Type{typ.String}
	result := ApplyOverride(types, typ.Integer)
	if len(result) != 1 {
		t.Fatal("expected single element")
	}
	if result[0] != typ.Integer {
		t.Fatal("expected replaced type")
	}
}

func TestApplyOverride_DoesNotModifyOriginal(t *testing.T) {
	types := []typ.Type{typ.String, typ.Boolean}
	_ = ApplyOverride(types, typ.Integer)
	if types[0] != typ.String {
		t.Fatal("original slice should not be modified")
	}
}

func TestResolveSpecFunction_Nil_ReturnsNil(t *testing.T) {
	result := ResolveSpecFunction(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestResolveSpecFunction_Function_ReturnsSame(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.Integer).Build()
	result := ResolveSpecFunction(fn)
	if result != fn {
		t.Fatal("expected same function back")
	}
}

func TestResolveSpecFunction_Alias_UnwrapsFunction(t *testing.T) {
	fn := typ.Func().Build()
	alias := typ.NewAlias("MyFunc", fn)
	result := ResolveSpecFunction(alias)
	if result != fn {
		t.Fatal("expected unwrapped function from alias")
	}
}

func TestResolveSpecFunction_Generic_ExtractsBody(t *testing.T) {
	fn := typ.Func().Build()
	gen := &typ.Generic{Body: fn}
	result := ResolveSpecFunction(gen)
	if result != fn {
		t.Fatal("expected function from generic body")
	}
}

func TestResolveSpecFunction_NonFunction_ReturnsNil(t *testing.T) {
	result := ResolveSpecFunction(typ.String)
	if result != nil {
		t.Fatal("expected nil for non-function type")
	}
}

func TestResolveSpecFunction_Record(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Integer).Build()
	result := ResolveSpecFunction(rec)
	if result != nil {
		t.Fatal("expected nil for record type")
	}
}

func TestResolveSpecFunction_Union(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Integer)
	result := ResolveSpecFunction(union)
	if result != nil {
		t.Fatal("expected nil for union type")
	}
}

func TestSpecReturnOverride_FunctionWithoutSpec(t *testing.T) {
	s := &SpecReturnOverride{Phase: api.PhaseScopeCompute}
	fn := typ.Func().
		Param("x", typ.Any).
		Returns(typ.String).
		Build()
	result := s.Override(fn, []ast.Expr{&ast.StringExpr{Value: "test"}})
	if result != nil {
		t.Fatal("expected nil for function without spec")
	}
}

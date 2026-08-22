package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferTypeArgs_ArrayParam(t *testing.T) {
	elem := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParam("T", nil).
		Param("list", typ.NewArray(elem)).
		Returns(typ.NewOptional(elem)).
		Build()

	args := []typ.Type{typ.NewArray(typ.String)}

	typeArgs, err := InferTypeArgsWithExpectedAndMode(fn, args, false, nil, nil, false)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}

	if len(typeArgs) != 1 {
		t.Fatalf("type args len = %d, want 1", len(typeArgs))
	}

	if typeArgs[0].Kind() != typ.String.Kind() {
		t.Fatalf("type arg = %v, want string", typeArgs[0])
	}
}

func TestInferTypeArgs_MapParam(t *testing.T) {
	keyParam := typ.NewTypeParam("K", nil)
	valParam := typ.NewTypeParam("V", nil)
	fn := typ.Func().
		TypeParam("K", nil).
		TypeParam("V", nil).
		Param("m", typ.NewMap(keyParam, valParam)).
		Returns(typ.NewArray(keyParam)).
		Build()

	args := []typ.Type{typ.NewMap(typ.String, typ.Number)}

	typeArgs, err := InferTypeArgsWithExpectedAndMode(fn, args, false, nil, nil, false)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}

	if len(typeArgs) != 2 {
		t.Fatalf("type args len = %d, want 2", len(typeArgs))
	}

	if typeArgs[0] != typ.String {
		t.Errorf("K = %v, want string", typeArgs[0])
	}

	if typeArgs[1] != typ.Number {
		t.Errorf("V = %v, want number", typeArgs[1])
	}
}

func TestInferTypeArgs_SimpleParam(t *testing.T) {
	paramT := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParam("T", nil).
		Param("value", paramT).
		Returns(paramT).
		Build()

	args := []typ.Type{typ.Integer}

	typeArgs, err := InferTypeArgsWithExpectedAndMode(fn, args, false, nil, nil, false)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}

	if len(typeArgs) != 1 {
		t.Fatalf("type args len = %d, want 1", len(typeArgs))
	}

	if typeArgs[0] != typ.Integer {
		t.Errorf("T = %v, want integer", typeArgs[0])
	}
}

func TestInferTypeArgs_LiteralWidens(t *testing.T) {
	paramT := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParam("T", nil).
		Param("value", paramT).
		Returns(paramT).
		Build()

	args := []typ.Type{typ.LiteralInt(42)}

	typeArgs, err := InferTypeArgsWithExpectedAndMode(fn, args, false, nil, nil, false)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}

	if len(typeArgs) != 1 {
		t.Fatalf("type args len = %d, want 1", len(typeArgs))
	}

	if typeArgs[0].Kind() != kind.Integer {
		t.Errorf("T = %v, want integer (widened from literal)", typeArgs[0])
	}
}

func TestInferTypeArgs_RecordField(t *testing.T) {
	paramT := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParam("T", nil).
		Param("rec", typ.NewRecord().Field("value", paramT).Build()).
		Returns(paramT).
		Build()

	args := []typ.Type{typ.NewRecord().Field("value", typ.String).Build()}

	typeArgs, err := InferTypeArgsWithExpectedAndMode(fn, args, false, nil, nil, false)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}

	if len(typeArgs) != 1 {
		t.Fatalf("type args len = %d, want 1", len(typeArgs))
	}

	if typeArgs[0] != typ.String {
		t.Errorf("T = %v, want string", typeArgs[0])
	}
}

func TestInferTypeArgs_NonGeneric(t *testing.T) {
	fn := typ.Func().
		Param("a", typ.String).
		Returns(typ.Boolean).
		Build()

	typeArgs, err := InferTypeArgsWithExpectedAndMode(fn, []typ.Type{typ.String}, false, nil, nil, false)
	if err != nil {
		t.Fatalf("InferTypeArgs error for non-generic: %v", err)
	}

	if typeArgs != nil {
		t.Errorf("expected nil type args for non-generic, got %v", typeArgs)
	}
}

func TestInferTypeArgs_CannotInfer(t *testing.T) {
	paramT := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParam("T", nil).
		Param("a", typ.String).
		Returns(paramT).
		Build()

	args, err := InferTypeArgsWithExpectedAndMode(fn, []typ.Type{typ.String}, false, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(args) != 1 {
		t.Fatalf("expected 1 type arg, got %d", len(args))
	}

	if args[0] != typ.Unknown {
		t.Errorf("expected unknown for unresolved T, got %v", args[0])
	}
}

func TestInferTypeArgs_ExpectedInstantiatedUnionReturn(t *testing.T) {
	tParam := typ.NewTypeParam("T", nil)
	user := typ.NewRecord().
		Field("id", typ.String).
		Field("email", typ.String).
		Build()
	resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{tParam},
		typ.NewUnion(
			typ.NewRecord().
				Field("ok", typ.True).
				Field("value", tParam).
				Build(),
			typ.NewRecord().
				Field("ok", typ.False).
				Field("error", typ.String).
				Build(),
		),
	)
	fn := typ.Func().
		TypeParam("T", nil).
		Param("message", typ.String).
		Returns(typ.Instantiate(resultGeneric, tParam)).
		Build()

	typeArgs, err := InferTypeArgsWithExpectedAndMode(
		fn,
		[]typ.Type{typ.String},
		false,
		nil,
		typ.Instantiate(resultGeneric, user),
		false,
	)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}
	if len(typeArgs) != 1 {
		t.Fatalf("type args len = %d, want 1", len(typeArgs))
	}
	if !typ.TypeEquals(typeArgs[0], user) {
		t.Fatalf("T = %v, want %v", typeArgs[0], user)
	}
}

func TestInferTypeArgs_FunctionParamWithInstantiatedUnionReturn(t *testing.T) {
	tParam := typ.NewTypeParam("T", nil)
	uParam := typ.NewTypeParam("U", nil)
	user := typ.NewRecord().
		Field("id", typ.String).
		Field("email", typ.String).
		Build()
	resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{tParam},
		typ.NewUnion(
			typ.NewRecord().
				Field("ok", typ.True).
				Field("value", tParam).
				Build(),
			typ.NewRecord().
				Field("ok", typ.False).
				Field("error", typ.String).
				Build(),
		),
	)
	fn := typ.Func().
		TypeParam("T", nil).
		TypeParam("U", nil).
		Param("r", typ.Instantiate(resultGeneric, tParam)).
		Param("mapper", typ.Func().
			Param("value", tParam).
			Returns(typ.Instantiate(resultGeneric, uParam)).
			Build()).
		Returns(typ.Instantiate(resultGeneric, uParam)).
		Build()
	callback := typ.Func().
		Param("value", user).
		Returns(typ.Instantiate(resultGeneric, user)).
		Build()

	typeArgs, err := InferTypeArgsWithExpectedAndMode(
		fn,
		[]typ.Type{
			typ.Instantiate(resultGeneric, user),
			callback,
		},
		false,
		nil,
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}
	if len(typeArgs) != 2 {
		t.Fatalf("type args len = %d, want 2", len(typeArgs))
	}
	if !typ.TypeEquals(typeArgs[0], user) {
		t.Fatalf("T = %v, want %v", typeArgs[0], user)
	}
	if !typ.TypeEquals(typeArgs[1], user) {
		t.Fatalf("U = %v, want %v", typeArgs[1], user)
	}
}

func TestInferTypeArgs_ExpectedExplicitUnionPrefersSpecificMember(t *testing.T) {
	tParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParam("T", nil).
		Returns(typ.NewUnion(tParam, typ.Nil)).
		Build()

	typeArgs, err := InferTypeArgsWithExpectedAndMode(
		fn,
		nil,
		false,
		nil,
		typ.NewUnion(typ.String, typ.Nil),
		false,
	)
	if err != nil {
		t.Fatalf("InferTypeArgs error: %v", err)
	}
	if len(typeArgs) != 1 {
		t.Fatalf("type args len = %d, want 1", len(typeArgs))
	}
	if !typ.TypeEquals(typeArgs[0], typ.String) {
		t.Fatalf("T = %v, want string", typeArgs[0])
	}
}

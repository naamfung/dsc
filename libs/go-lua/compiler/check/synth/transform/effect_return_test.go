package transform

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyEffectTransform_ErrorReturnOptionalizes(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	fn := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(spec).
		Build()

	got := ApplyEffectTransform(fn, nil, 0, typ.String)
	want := typ.NewOptional(typ.String)

	if !typ.TypeEquals(got, want) {
		t.Fatalf("ApplyEffectTransform error return: got %v, want %v", got, want)
	}
}

func TestBuildSelectResultUnion_ResolvesNegativeCasesIndex(t *testing.T) {
	chParam := typ.NewTypeParam("Ch", typ.Any)
	valParam := typ.NewTypeParam("T", typ.Any)
	selectCase := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{chParam, valParam}, typ.NewRecord().Build())

	args := []typ.Type{
		typ.String,
		typ.NewArray(typ.Instantiate(selectCase, typ.Integer, typ.Boolean)),
	}
	got := buildSelectResultUnion(args, effect.SelectResultOfCases{
		Cases:   effect.ParamRef{Index: -1},
		Default: effect.ParamRef{Index: -1},
	})

	want := typ.NewRecord().
		Field("__select_case_id", typ.LiteralInt(0)).
		Field("channel", typ.Integer).
		Field("ok", typ.Boolean).
		Field("value", typ.Boolean).
		Build()

	if !typ.TypeEquals(got, want) {
		t.Fatalf("buildSelectResultUnion negative cases index: got %v, want %v", got, want)
	}
}

func TestBuildSelectResultUnion_DefaultFieldRequiresInRangeDefaultArg(t *testing.T) {
	chParam := typ.NewTypeParam("Ch", typ.Any)
	valParam := typ.NewTypeParam("T", typ.Any)
	selectCase := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{chParam, valParam}, typ.NewRecord().Build())

	args := []typ.Type{
		typ.NewArray(typ.Instantiate(selectCase, typ.String, typ.Number)),
	}
	got := buildSelectResultUnion(args, effect.SelectResultOfCases{
		Cases:   effect.ParamRef{Index: 0},
		Default: effect.ParamRef{Index: 1},
	})

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", got)
	}
	if field := rec.GetField("default"); field != nil {
		t.Fatalf("did not expect default field when default arg is absent, got %+v", *field)
	}
}

func TestBuildSelectResultUnion_DefaultFieldFromCasesRecordDefaultKey(t *testing.T) {
	chParam := typ.NewTypeParam("Ch", typ.Any)
	valParam := typ.NewTypeParam("T", typ.Any)
	selectCase := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{chParam, valParam}, typ.NewRecord().Build())

	casesArg := typ.NewRecord().
		Field("1", typ.Instantiate(selectCase, typ.String, typ.Number)).
		Field("default", typ.LiteralBool(true)).
		Build()
	args := []typ.Type{casesArg}

	got := buildSelectResultUnion(args, effect.SelectResultOfCases{
		Cases:   effect.ParamRef{Index: 0},
		Default: effect.ParamRef{Index: 1}, // Out-of-range param index; default flag comes from cases record.
	})

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", got)
	}
	field := rec.GetField("default")
	if field == nil || !field.Optional || !typ.TypeEquals(field.Type, typ.Boolean) {
		t.Fatalf("expected optional boolean default field, got %+v", field)
	}
}

func TestBuildSelectResultUnion_SkipsNonCaseRecordFields(t *testing.T) {
	chParam := typ.NewTypeParam("Ch", typ.Any)
	valParam := typ.NewTypeParam("T", typ.Any)
	selectCase := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{chParam, valParam}, typ.NewRecord().Build())

	casesArg := typ.NewRecord().
		Field("1", typ.Instantiate(selectCase, typ.String, typ.Number)).
		Field("meta", typ.String).
		Build()
	args := []typ.Type{casesArg}

	got := buildSelectResultUnion(args, effect.SelectResultOfCases{
		Cases:   effect.ParamRef{Index: 0},
		Default: effect.ParamRef{Index: 1},
	})

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", got)
	}

	channelField := rec.GetField("channel")
	valueField := rec.GetField("value")
	if channelField == nil || valueField == nil {
		t.Fatalf("expected channel/value fields, got %v", rec)
	}

	if !typ.TypeEquals(channelField.Type, typ.String) {
		t.Fatalf("channel field should come from select case, got %v", channelField.Type)
	}
	if !typ.TypeEquals(valueField.Type, typ.Number) {
		t.Fatalf("value field should come from select case, got %v", valueField.Type)
	}
}

func TestApplyEffectTransform_CallbackReturn(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.Return{
		ReturnIndex: 1,
		Transform:   effect.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
	})
	fn := typ.Func().
		Param("f", typ.Any).
		Returns(typ.Boolean, typ.Any).
		Spec(spec).
		Build()
	args := []typ.Type{
		typ.Func().Returns(typ.String).Build(),
	}

	got := ApplyEffectTransform(fn, args, 1, typ.Any)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected callback return transform to produce string, got %v", got)
	}
}

func TestApplyEffectTransform_ArrayOfCallbackReturn(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.Return{
		ReturnIndex: 0,
		Transform:   effect.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 1}},
	})
	fn := typ.Func().
		Param("arr", typ.Any).
		Param("mapper", typ.Any).
		Returns(typ.Any).
		Spec(spec).
		Build()
	args := []typ.Type{
		typ.NewArray(typ.Integer),
		typ.Func().Param("x", typ.Integer).Returns(typ.String).Build(),
	}

	got := ApplyEffectTransform(fn, args, 0, typ.Any)
	want := typ.NewArray(typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected array(callback return) transform to produce %v, got %v", want, got)
	}
}

func TestApplyEffectTransform_StringUnpackValue_IntegerFormat(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.Return{
		ReturnIndex: 0,
		Transform:   effect.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
	})
	fn := typ.Func().
		Param("fmt", typ.String).
		Param("s", typ.String).
		OptParam("pos", typ.Integer).
		Returns(typ.Any).
		Spec(spec).
		Build()

	got := ApplyEffectTransform(fn, []typ.Type{typ.LiteralString(">I4"), typ.String, typ.Integer}, 0, typ.Any)
	if !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected string.unpack integer format to produce integer, got %v", got)
	}
}

func TestApplyEffectTransform_StringUnpackValue_StringFormat(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.Return{
		ReturnIndex: 0,
		Transform:   effect.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
	})
	fn := typ.Func().
		Param("fmt", typ.String).
		Param("s", typ.String).
		OptParam("pos", typ.Integer).
		Returns(typ.Any).
		Spec(spec).
		Build()

	got := ApplyEffectTransform(fn, []typ.Type{typ.LiteralString("z"), typ.String, typ.Integer}, 0, typ.Any)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected string.unpack string format to produce string, got %v", got)
	}
}

func TestApplyEffectTransform_StringUnpackValue_UnsupportedFormatFallsBack(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.Return{
		ReturnIndex: 0,
		Transform:   effect.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
	})
	fn := typ.Func().
		Param("fmt", typ.String).
		Param("s", typ.String).
		OptParam("pos", typ.Integer).
		Returns(typ.Any).
		Spec(spec).
		Build()

	got := ApplyEffectTransform(fn, []typ.Type{typ.LiteralString("X"), typ.String, typ.Integer}, 0, typ.Any)
	if !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("expected unsupported string.unpack format to fall back to any, got %v", got)
	}
}

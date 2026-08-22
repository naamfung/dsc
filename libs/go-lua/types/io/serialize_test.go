package io

import (
	"bytes"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEncodeDecode_Primitives(t *testing.T) {
	cases := []struct {
		name string
		typ  typ.Type
	}{
		{"Nil", typ.Nil},
		{"Boolean", typ.Boolean},
		{"Number", typ.Number},
		{"Integer", typ.Integer},
		{"String", typ.String},
		{"Any", typ.Any},
		{"Unknown", typ.Unknown},
		{"Never", typ.Never},
		{"Self", typ.Self},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Encode(tc.typ)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			decoded, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if decoded != tc.typ {
				t.Errorf("got %s, want %s", decoded, tc.typ)
			}
		})
	}
}

func TestEncodeDecode_Optional(t *testing.T) {
	cases := []typ.Type{
		typ.NewOptional(typ.String),
		typ.NewOptional(typ.Number),
		typ.NewOptional(typ.NewOptional(typ.Boolean)),
	}

	for _, original := range cases {
		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode(%s): %v", original, err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode(%s): %v", original, err)
		}

		if !original.Equals(decoded) {
			t.Errorf("got %s, want %s", decoded, original)
		}
	}
}

func TestEncodeDecode_Union(t *testing.T) {
	original := typ.NewUnion(typ.String, typ.Number, typ.Boolean)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	union, ok := decoded.(*typ.Union)
	if !ok {
		t.Fatalf("expected Union, got %T", decoded)
	}

	if len(union.Members) != 3 {
		t.Errorf("expected 3 members, got %d", len(union.Members))
	}
}

func TestEncodeDecode_Intersection(t *testing.T) {
	r1 := typ.NewRecord().Field("a", typ.String).Build()
	r2 := typ.NewRecord().Field("b", typ.Number).Build()
	original := typ.NewIntersection(r1, r2)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	inter, ok := decoded.(*typ.Intersection)
	if !ok {
		t.Fatalf("expected Intersection, got %T", decoded)
	}

	if len(inter.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(inter.Members))
	}
}

func TestEncodeDecode_Function(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		original := typ.Func().
			Param("x", typ.Number).
			OptParam("y", typ.String).
			Returns(typ.Boolean).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		fn, ok := decoded.(*typ.Function)
		if !ok {
			t.Fatalf("expected Function, got %T", decoded)
		}

		if len(fn.Params) != 2 {
			t.Errorf("expected 2 params, got %d", len(fn.Params))
		}

		if !fn.Params[1].Optional {
			t.Error("second param should be optional")
		}
	})

	t.Run("variadic", func(t *testing.T) {
		original := typ.Func().
			Param("fmt", typ.String).
			Variadic(typ.Any).
			Returns(typ.String).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		fn, ok := decoded.(*typ.Function)
		if !ok {
			t.Fatalf("expected Function, got %T", decoded)
		}
		if fn.Variadic == nil {
			t.Error("expected variadic")
		}
	})

	t.Run("typeparams", func(t *testing.T) {
		paramT := typ.NewTypeParam("T", typ.Number)
		original := typ.Func().
			TypeParam("T", typ.Number).
			Param("value", paramT).
			Returns(paramT).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		fn, ok := decoded.(*typ.Function)
		if !ok {
			t.Fatalf("expected Function, got %T", decoded)
		}

		if len(fn.TypeParams) != 1 {
			t.Fatalf("expected 1 type param, got %d", len(fn.TypeParams))
		}
		if fn.TypeParams[0].Name != "T" {
			t.Errorf("expected type param name T, got %s", fn.TypeParams[0].Name)
		}
		if fn.TypeParams[0].Constraint == nil || !fn.TypeParams[0].Constraint.Equals(typ.Number) {
			t.Errorf("expected type param constraint number, got %v", fn.TypeParams[0].Constraint)
		}
	})

	t.Run("multi-return", func(t *testing.T) {
		original := typ.Func().
			Returns(typ.String, typ.Number, typ.Boolean).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		fn, ok := decoded.(*typ.Function)
		if !ok {
			t.Fatalf("expected Function, got %T", decoded)
		}
		if len(fn.Returns) != 3 {
			t.Errorf("expected 3 returns, got %d", len(fn.Returns))
		}
	})

	t.Run("with-effects", func(t *testing.T) {
		row := effect.Row{Labels: []effect.Label{effect.IO{}, effect.Throw{}}}
		original := typ.Func().
			Param("path", typ.String).
			Returns(typ.String).
			Effects(row).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		fn, ok := decoded.(*typ.Function)
		if !ok {
			t.Fatalf("expected Function, got %T", decoded)
		}
		eff, ok := fn.Effects.(effect.Row)

		if !ok {
			t.Fatalf("expected effect.Row, got %T", fn.Effects)
		}

		if len(eff.Labels) != 2 {
			t.Errorf("expected 2 effect labels, got %d", len(eff.Labels))
		}
	})

	t.Run("with-spec", func(t *testing.T) {
		spec := contract.NewSpec().
			WithEnsures(constraint.NotNil{Path: constraint.Path{Root: "param[0]"}}).
			WithEffects(effect.Throw{}).
			WithCallback(1, contract.PredicateSpec(0))
		original := typ.Func().
			Param("x", typ.Any).
			Param("pred", typ.Any).
			Returns(typ.Any).
			Build()
		original.Spec = spec

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		fn, ok := decoded.(*typ.Function)
		if !ok {
			t.Fatalf("expected Function, got %T", decoded)
		}

		decodedSpec, ok := fn.Spec.(*contract.Spec)
		if !ok || decodedSpec == nil {
			t.Fatal("expected contract spec")
		}

		if len(decodedSpec.Ensures.AllConstraints()) != 1 {
			t.Fatalf("ensures len = %d, want 1", len(decodedSpec.Ensures.AllConstraints()))
		}

		if len(decodedSpec.Callbacks) != 1 {
			t.Fatalf("callbacks len = %d, want 1", len(decodedSpec.Callbacks))
		}
	})

	t.Run("with-refinement", func(t *testing.T) {
		refinement := &constraint.FunctionRefinement{
			OnReturn: constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "$0"}}),
			OnTrue:   constraint.FromConstraints(constraint.HasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}),
			OnFalse:  constraint.FromConstraints(constraint.NotHasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}),
		}
		original := typ.Func().
			Param("x", typ.Any).
			Returns(typ.Boolean).
			WithRefinement(refinement).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		fn, ok := decoded.(*typ.Function)
		if !ok {
			t.Fatalf("expected Function, got %T", decoded)
		}

		decodedRefinement, ok := fn.Refinement.(*constraint.FunctionRefinement)
		if !ok || decodedRefinement == nil {
			t.Fatal("expected refinement")
		}

		if len(decodedRefinement.OnReturn.MustConstraints()) != 1 {
			t.Errorf("OnReturn len = %d, want 1", len(decodedRefinement.OnReturn.MustConstraints()))
		}

		if len(decodedRefinement.OnTrue.MustConstraints()) != 1 {
			t.Errorf("OnTrue len = %d, want 1", len(decodedRefinement.OnTrue.MustConstraints()))
		}

		if len(decodedRefinement.OnFalse.MustConstraints()) != 1 {
			t.Errorf("OnFalse len = %d, want 1", len(decodedRefinement.OnFalse.MustConstraints()))
		}
	})
}

func TestEncodeDecode_Record(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		original := typ.NewRecord().
			Field("name", typ.String).
			OptField("age", typ.Integer).
			ReadonlyField("id", typ.Number).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		rec := decoded.(*typ.Record)
		if len(rec.Fields) != 3 {
			t.Errorf("expected 3 fields, got %d", len(rec.Fields))
		}
	})

	t.Run("with-metatable", func(t *testing.T) {
		meta := typ.NewRecord().
			Field("__index", typ.Any).
			Build()
		original := typ.NewRecord().
			Field("value", typ.Number).
			Build()
		original.Metatable = meta

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		rec := decoded.(*typ.Record)
		if rec.Metatable == nil {
			t.Error("metatable should not be nil")
		}
	})

	t.Run("open-and-map", func(t *testing.T) {
		original := typ.NewRecord().
			Field("id", typ.Integer).
			SetOpen(true).
			MapComponent(typ.String, typ.Number).
			Build()

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		rec := decoded.(*typ.Record)
		if !rec.Open {
			t.Error("expected Open to be true")
		}
		if !rec.HasMapComponent() {
			t.Fatal("expected map component")
		}
		if rec.MapKey == nil || rec.MapValue == nil {
			t.Fatal("expected map key/value types")
		}
		if !rec.MapKey.Equals(typ.String) {
			t.Errorf("expected map key string, got %s", rec.MapKey.String())
		}
		if !rec.MapValue.Equals(typ.Number) {
			t.Errorf("expected map value number, got %s", rec.MapValue.String())
		}
	})
}

func TestEncodeDecode_Array(t *testing.T) {
	original := typ.NewArray(typ.String)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	arr := decoded.(*typ.Array)
	if arr.Element != typ.String {
		t.Error("expected string element")
	}
}

func TestEncodeDecode_Map(t *testing.T) {
	original := typ.NewMap(typ.String, typ.Number)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	m := decoded.(*typ.Map)
	if m.Key != typ.String || m.Value != typ.Number {
		t.Error("map key/value mismatch")
	}
}

func TestEncodeDecode_Tuple(t *testing.T) {
	original := typ.NewTuple(typ.String, typ.Number, typ.Boolean)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	tup := decoded.(*typ.Tuple)
	if len(tup.Elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(tup.Elements))
	}
}

func TestEncodeDecode_Literal(t *testing.T) {
	cases := []struct {
		name string
		lit  *typ.Literal
	}{
		{"true", typ.LiteralBool(true)},
		{"false", typ.LiteralBool(false)},
		{"int-positive", typ.LiteralInt(42)},
		{"int-negative", typ.LiteralInt(-100)},
		{"int-zero", typ.LiteralInt(0)},
		{"number", typ.LiteralNumber(3.14159)},
		{"string", typ.LiteralString("hello")},
		{"string-empty", typ.LiteralString("")},
		{"string-unicode", typ.LiteralString("\u4e2d\u6587")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Encode(tc.lit)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			decoded, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if !tc.lit.Equals(decoded) {
				t.Errorf("got %s, want %s", decoded, tc.lit)
			}
		})
	}
}

func TestEncodeDecode_Generic(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)
		original := typ.NewGeneric("Box", []*typ.TypeParam{param}, typ.Any)

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		gen := decoded.(*typ.Generic)
		if gen.Name != "Box" {
			t.Errorf("expected name Box, got %s", gen.Name)
		}

		if len(gen.TypeParams) != 1 {
			t.Errorf("expected 1 type param, got %d", len(gen.TypeParams))
		}
	})

	t.Run("with-constraint", func(t *testing.T) {
		param := typ.NewTypeParam("T", typ.Number)
		original := typ.NewGeneric("NumBox", []*typ.TypeParam{param}, typ.Any)

		data, err := Encode(original)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		gen := decoded.(*typ.Generic)
		if gen.TypeParams[0].Constraint == nil {
			t.Error("expected constraint")
		}
	})
}

func TestEncodeDecode_Instantiated(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	generic := typ.NewGeneric("Box", []*typ.TypeParam{param}, typ.Any)
	original := typ.Instantiate(generic, typ.String)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	inst := decoded.(*typ.Instantiated)
	if len(inst.TypeArgs) != 1 {
		t.Errorf("expected 1 type arg, got %d", len(inst.TypeArgs))
	}
}

func TestEncodeDecode_TypeVar(t *testing.T) {
	original := typ.NewTypeVar(42)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	tv := decoded.(*typ.TypeVar)
	if tv.ID != 42 {
		t.Errorf("expected ID 42, got %d", tv.ID)
	}
}

func TestEncodeDecode_Ref(t *testing.T) {
	original := typ.NewRef("mymodule", "MyType")

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	ref := decoded.(*typ.Ref)
	if ref.Module != "mymodule" || ref.Name != "MyType" {
		t.Errorf("ref mismatch: got %s.%s", ref.Module, ref.Name)
	}
}

func TestEncodeDecode_Alias(t *testing.T) {
	original := typ.NewAlias("StringAlias", typ.String)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	alias := decoded.(*typ.Alias)
	if alias.Name != "StringAlias" {
		t.Errorf("expected name StringAlias, got %s", alias.Name)
	}

	if alias.Target != typ.String {
		t.Error("target mismatch")
	}
}

func TestEncodeDecode_Meta(t *testing.T) {
	original := typ.NewMeta(typ.String)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	meta := decoded.(*typ.Meta)
	if meta.Of != typ.String {
		t.Error("meta.Of mismatch")
	}
}

func TestEncodeDecode_Platform(t *testing.T) {
	original := typ.NewPlatform("userdata")

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	plat := decoded.(*typ.Platform)
	if plat.Name != "userdata" {
		t.Errorf("expected userdata, got %s", plat.Name)
	}
}

func TestEncodeDecode_Nil(t *testing.T) {
	data, _ := Encode(nil)

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded != typ.Nil {
		t.Errorf("expected Nil, got %v", decoded)
	}
}

func TestDecode_TruncatedData(t *testing.T) {
	original := typ.NewRecord().Field("name", typ.String).Build()
	data, _ := Encode(original)

	_, err := Decode(data[:len(data)/2])
	if err == nil {
		t.Error("expected error for truncated data")
	}
}

func TestDecode_EmptyData(t *testing.T) {
	_, err := Decode([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestManifest_RoundTrip(t *testing.T) {
	m := NewManifest("mymodule")
	m.Version = 42
	m.Export = typ.NewRecord().Field("foo", typ.String).Build()
	m.DefineType("MyType", typ.NewAlias("MyType", typ.Number))
	m.AddGlobal("globalFn", typ.Func().Returns(typ.Nil).Build())

	summary := NewSummary([]typ.Type{typ.String}, []typ.Type{typ.Boolean})
	summary.Effects = effect.Empty
	summary.Ensures = constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "result"}})
	summary.ParamEscapes[0] = true
	m.DefineSummary("checkString", summary)

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.Path != "mymodule" {
		t.Errorf("path mismatch: got %s", decoded.Path)
	}

	if decoded.Version != 42 {
		t.Errorf("version mismatch: got %d", decoded.Version)
	}

	if decoded.Export == nil {
		t.Error("export is nil")
	}

	if _, ok := decoded.Types["MyType"]; !ok {
		t.Error("MyType not found")
	}

	if _, ok := decoded.Globals["globalFn"]; !ok {
		t.Error("globalFn not found")
	}

	s, ok := decoded.Summaries["checkString"]
	if !ok {
		t.Fatal("checkString summary not found")
	}

	if len(s.Params) != 1 {
		t.Errorf("expected 1 param, got %d", len(s.Params))
	}

	if len(s.Ensures.MustConstraints()) != 1 {
		t.Errorf("expected 1 ensure, got %d", len(s.Ensures.MustConstraints()))
	}

	if !s.ParamEscapes[0] {
		t.Error("param escape not preserved")
	}
}

func TestManifest_Empty(t *testing.T) {
	m := NewManifest("empty")

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.Path != "empty" {
		t.Errorf("path mismatch: got %s", decoded.Path)
	}

	if decoded.Export != nil {
		t.Error("expected nil export")
	}

	if len(decoded.Types) != 0 {
		t.Error("expected no types")
	}
}

func TestManifest_InvalidHeader(t *testing.T) {
	_, err := DecodeManifest([]byte{0, 0, 0, 0})
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestManifest_VersionMismatch(t *testing.T) {
	var buf bytes.Buffer

	buf.Write([]byte{0x49, 0x4E, 0x41, 0x4D})
	buf.WriteByte(99)

	_, err := DecodeManifest(buf.Bytes())
	if !errors.Is(err, ErrVersionMismatch) {
		t.Errorf("expected ErrVersionMismatch, got %v", err)
	}
}

func TestManifest_Lookups(t *testing.T) {
	m := NewManifest("test")
	m.DefineType("Foo", typ.String)
	m.DefineSummary("bar", NewSummary(nil, nil))

	if ty, ok := m.LookupType("Foo"); !ok || ty != typ.String {
		t.Error("LookupType failed")
	}

	if _, ok := m.LookupType("Missing"); ok {
		t.Error("LookupType should return false for missing")
	}

	if _, ok := m.LookupSummary("bar"); !ok {
		t.Error("LookupSummary failed")
	}

	if _, ok := m.LookupSummary("Missing"); ok {
		t.Error("LookupSummary should return false for missing")
	}
}

func TestSummary_ReturnsParam(t *testing.T) {
	m := NewManifest("test")
	s := NewSummary([]typ.Type{typ.String}, []typ.Type{typ.String})
	s.ReturnsParam = 0
	m.DefineSummary("identity", s)

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	ds := decoded.Summaries["identity"]
	if ds.ReturnsParam != 0 {
		t.Errorf("expected ReturnsParam 0, got %d", ds.ReturnsParam)
	}
}

func TestEncodeManifest_Wrapper(t *testing.T) {
	m := NewManifest("test")
	data1, _ := m.Encode()
	data2, _ := EncodeManifest(m)

	if !bytes.Equal(data1, data2) {
		t.Error("EncodeManifest wrapper should produce same output")
	}
}

func TestManifest_SetExport(t *testing.T) {
	m := NewManifest("test")
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	m.SetExport(rec)

	if m.Export == nil {
		t.Fatal("export should not be nil")
	}

	if m.Export != rec {
		t.Error("export mismatch")
	}
}

func TestManifest_EnrichedExport(t *testing.T) {
	m := NewManifest("test")

	// Create a record with a function field
	fn := typ.Func().Param("x", typ.Any).Returns(typ.Boolean).Build()
	rec := typ.NewRecord().Field("isValid", fn).Build()
	m.SetExport(rec)

	// Add a summary for the function with constraints
	summary := NewSummary([]typ.Type{typ.Any}, []typ.Type{typ.Boolean})
	summary.Ensures = constraint.FromConstraints(constraint.HasType{
		Path: constraint.Path{Root: "$0"},
		Type: narrow.BuiltinTypeKey("string"),
	})
	m.DefineSummary("isValid", summary)

	// EnrichedExport should apply the summary
	enriched := m.EnrichedExport()
	if enriched == nil {
		t.Fatal("EnrichedExport should not return nil")
	}

	enrichedRec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("EnrichedExport should return a record, got %T", enriched)
	}

	// Find the isValid field
	var enrichedFn *typ.Function

	for _, f := range enrichedRec.Fields {
		if f.Name == "isValid" {
			enrichedFn, _ = f.Type.(*typ.Function)
			break
		}
	}

	if enrichedFn == nil {
		t.Fatal("isValid field should be a function")
	}

	// Verify the function has refinement from the summary
	if enrichedFn.Refinement == nil {
		t.Error("enriched function should have Refinement set from summary")
	}

	// Verify the function has spec from the summary
	if enrichedFn.Spec == nil {
		t.Error("enriched function should have Spec set from summary")
	}
}

func TestManifest_EnrichedExport_NoSummaries(t *testing.T) {
	m := NewManifest("test")
	fn := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	rec := typ.NewRecord().Field("convert", fn).Build()
	m.SetExport(rec)

	// No summaries defined - EnrichedExport should return Export unchanged
	enriched := m.EnrichedExport()
	if enriched != rec {
		t.Error("EnrichedExport with no summaries should return Export as-is")
	}
}

func TestManifest_EnrichedExport_Interface(t *testing.T) {
	m := NewManifest("test")

	// Create an interface with a method
	fn := typ.Func().Param("data", typ.Any).Returns(typ.Boolean).Build()
	iface := typ.NewInterface("Validator", []typ.Method{
		{Name: "validate", Type: fn},
	})
	m.SetExport(iface)

	// Add a summary for the method
	summary := NewSummary([]typ.Type{typ.Any}, []typ.Type{typ.Boolean})
	summary.Ensures = constraint.FromConstraints(constraint.Truthy{
		Path: constraint.Path{Root: "$0"},
	})
	m.DefineSummary("validate", summary)

	// EnrichedExport should apply the summary to interface methods
	enriched := m.EnrichedExport()
	if enriched == nil {
		t.Fatal("EnrichedExport should not return nil")
	}

	enrichedIface, ok := enriched.(*typ.Interface)
	if !ok {
		t.Fatalf("EnrichedExport should return an interface, got %T", enriched)
	}

	// Find the validate method
	var enrichedMethod *typ.Function

	for _, m := range enrichedIface.Methods {
		if m.Name == "validate" {
			enrichedMethod = m.Type
			break
		}
	}

	if enrichedMethod == nil {
		t.Fatal("validate method should exist")
	}

	// Verify the method has refinement from the summary
	if enrichedMethod.Refinement == nil {
		t.Error("enriched method should have Refinement set from summary")
	}
}

// TestDNFPreservation_SpecRequires verifies multi-disjunct Requires conditions
// are preserved through encode/decode without collapsing to MustConstraints.
func TestDNFPreservation_SpecRequires(t *testing.T) {
	// Build a condition with 2 disjuncts: (A AND B) OR (C AND D)
	disjunct1 := []constraint.Constraint{
		constraint.NotNil{Path: constraint.Path{Root: "$0"}},
		constraint.HasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")},
	}
	disjunct2 := []constraint.Constraint{
		constraint.NotNil{Path: constraint.Path{Root: "$0"}},
		constraint.HasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("number")},
	}
	multiDisjunct := constraint.Condition{Disjuncts: [][]constraint.Constraint{disjunct1, disjunct2}}

	spec := contract.NewSpec()
	spec.Requires = multiDisjunct

	original := typ.Func().
		Param("x", typ.Any).
		Returns(typ.Boolean).
		Build()
	original.Spec = spec

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	fn := decoded.(*typ.Function)
	decodedSpec := fn.Spec.(*contract.Spec)

	// Verify full DNF preservation
	if decodedSpec.Requires.NumDisjuncts() != 2 {
		t.Errorf("Requires: expected 2 disjuncts, got %d", decodedSpec.Requires.NumDisjuncts())
	}

	// Verify each disjunct has correct constraint count
	for i, d := range decodedSpec.Requires.Disjuncts {
		if len(d) != 2 {
			t.Errorf("disjunct %d: expected 2 constraints, got %d", i, len(d))
		}
	}
}

// TestDNFPreservation_SpecEnsures verifies multi-disjunct Ensures conditions
// are preserved through encode/decode without collapsing to MustConstraints.
func TestDNFPreservation_SpecEnsures(t *testing.T) {
	// Build a condition with 3 disjuncts
	disjuncts := [][]constraint.Constraint{
		{constraint.Truthy{Path: constraint.Path{Root: "result"}}},
		{constraint.IsNil{Path: constraint.Path{Root: "result"}}},
		{constraint.HasType{Path: constraint.Path{Root: "result"}, Type: narrow.BuiltinTypeKey("boolean")}},
	}
	multiDisjunct := constraint.Condition{Disjuncts: disjuncts}

	spec := contract.NewSpec()
	spec.Ensures = multiDisjunct

	original := typ.Func().
		Param("x", typ.Any).
		Returns(typ.Any).
		Build()
	original.Spec = spec

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	fn := decoded.(*typ.Function)
	decodedSpec := fn.Spec.(*contract.Spec)

	if decodedSpec.Ensures.NumDisjuncts() != 3 {
		t.Errorf("Ensures: expected 3 disjuncts, got %d", decodedSpec.Ensures.NumDisjuncts())
	}
}

// TestDNFPreservation_FunctionRefinement verifies multi-disjunct OnTrue/OnFalse
// conditions in FunctionRefinement are preserved through encode/decode.
func TestDNFPreservation_FunctionRefinement(t *testing.T) {
	// OnTrue: (string) OR (number)
	onTrueDisjuncts := [][]constraint.Constraint{
		{constraint.HasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		{constraint.HasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("number")}},
	}
	// OnFalse: (nil) OR (boolean AND falsy)
	onFalseDisjuncts := [][]constraint.Constraint{
		{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
		{
			constraint.HasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("boolean")},
			constraint.Falsy{Path: constraint.Path{Root: "$0"}},
		},
	}

	refinement := &constraint.FunctionRefinement{
		OnReturn: constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "$0"}}),
		OnTrue:   constraint.Condition{Disjuncts: onTrueDisjuncts},
		OnFalse:  constraint.Condition{Disjuncts: onFalseDisjuncts},
	}

	original := typ.Func().
		Param("x", typ.Any).
		Returns(typ.Boolean).
		WithRefinement(refinement).
		Build()

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	fn := decoded.(*typ.Function)
	decodedRef := fn.Refinement.(*constraint.FunctionRefinement)

	// Verify OnTrue DNF
	if decodedRef.OnTrue.NumDisjuncts() != 2 {
		t.Errorf("OnTrue: expected 2 disjuncts, got %d", decodedRef.OnTrue.NumDisjuncts())
	}

	// Verify OnFalse DNF
	if decodedRef.OnFalse.NumDisjuncts() != 2 {
		t.Errorf("OnFalse: expected 2 disjuncts, got %d", decodedRef.OnFalse.NumDisjuncts())
	}

	// Verify second OnFalse disjunct has 2 constraints
	if len(decodedRef.OnFalse.Disjuncts) >= 2 && len(decodedRef.OnFalse.Disjuncts[1]) != 2 {
		t.Errorf("OnFalse disjunct 1: expected 2 constraints, got %d", len(decodedRef.OnFalse.Disjuncts[1]))
	}

	// Verify OnReturn preserved
	if decodedRef.OnReturn.NumDisjuncts() != 1 {
		t.Errorf("OnReturn: expected 1 disjunct, got %d", decodedRef.OnReturn.NumDisjuncts())
	}
}

// TestDNFPreservation_ManifestSummary verifies DNF preservation in manifest summaries.
func TestDNFPreservation_ManifestSummary(t *testing.T) {
	m := NewManifest("test")

	// Multi-disjunct ensures
	ensures := constraint.Condition{
		Disjuncts: [][]constraint.Constraint{
			{constraint.NotNil{Path: constraint.Path{Root: "result"}}},
			{constraint.HasType{Path: constraint.Path{Root: "result"}, Type: narrow.BuiltinTypeKey("string")}},
			{constraint.Truthy{Path: constraint.Path{Root: "result"}}},
		},
	}

	summary := NewSummary([]typ.Type{typ.Any}, []typ.Type{typ.Any})
	summary.Ensures = ensures
	m.DefineSummary("multiDisjunctFn", summary)

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	s := decoded.Summaries["multiDisjunctFn"]
	if s.Ensures.NumDisjuncts() != 3 {
		t.Errorf("expected 3 disjuncts, got %d", s.Ensures.NumDisjuncts())
	}
}

// TestDNFPreservation_ReturnSpec verifies DNF preservation in ReturnSpec.When conditions.
func TestDNFPreservation_ReturnSpec(t *testing.T) {
	// Create spec with ReturnSpec containing multi-disjunct When conditions
	spec := contract.NewSpec()
	spec.Return = &contract.ReturnSpec{
		Cases: []contract.ReturnCase{
			{
				When: constraint.Condition{
					Disjuncts: [][]constraint.Constraint{
						{constraint.Truthy{Path: constraint.Path{Root: "$0"}}},
						{constraint.HasType{Path: constraint.Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
					},
				},
				Type: typ.String,
			},
			{
				When: constraint.Condition{
					Disjuncts: [][]constraint.Constraint{
						{constraint.Falsy{Path: constraint.Path{Root: "$0"}}},
						{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
					},
				},
				Type: typ.Nil,
			},
		},
		Default: typ.Any,
	}

	original := typ.Func().
		Param("x", typ.Any).
		Returns(typ.Any).
		Build()
	original.Spec = spec

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	fn := decoded.(*typ.Function)
	decodedSpec := fn.Spec.(*contract.Spec)

	if decodedSpec.Return == nil {
		t.Fatal("Return spec is nil")
	}

	if len(decodedSpec.Return.Cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(decodedSpec.Return.Cases))
	}

	// Verify first case has 2 disjuncts
	if decodedSpec.Return.Cases[0].When.NumDisjuncts() != 2 {
		t.Errorf("case 0: expected 2 disjuncts, got %d", decodedSpec.Return.Cases[0].When.NumDisjuncts())
	}

	// Verify second case has 2 disjuncts
	if decodedSpec.Return.Cases[1].When.NumDisjuncts() != 2 {
		t.Errorf("case 1: expected 2 disjuncts, got %d", decodedSpec.Return.Cases[1].When.NumDisjuncts())
	}
}

func TestWriterAdapter_Methods(t *testing.T) {
	var buf bytes.Buffer
	tw := &typeWriter{w: &buf}
	adapter := &writerAdapter{tw}

	if err := adapter.WriteByte(0x42); err != nil {
		t.Errorf("WriteByte: %v", err)
	}

	if err := adapter.WriteInt32(12345); err != nil {
		t.Errorf("WriteInt32: %v", err)
	}

	if err := adapter.WriteString("hello"); err != nil {
		t.Errorf("WriteString: %v", err)
	}

	if err := adapter.WriteType(typ.String); err != nil {
		t.Errorf("WriteType: %v", err)
	}

	// WriteType with non-Type should not error
	if err := adapter.WriteType("not a type"); err != nil {
		t.Errorf("WriteType with non-Type: %v", err)
	}
}

func TestReaderAdapter_Methods(t *testing.T) {
	original := typ.NewRecord().Field("x", typ.Number).Build()
	data, _ := Encode(original)

	r := &typeReader{r: bytes.NewReader(data)}
	adapter := &readerAdapter{r}

	// ReadByte
	b, err := adapter.ReadByte()
	if err != nil {
		t.Errorf("ReadByte: %v", err)
	}

	_ = b

	// ReadInt32 - skip to where an int might be, may error if not at right position
	_, _ = adapter.ReadInt32()

	// ReadString (may error if not at right position).
	_, _ = adapter.ReadString()

	// ReadType (may error if not at right position).
	_, _ = adapter.ReadType()
}

func TestDecode_CorruptedSliceLen(t *testing.T) {
	// Create a union with very large member count (corrupted)
	var buf bytes.Buffer
	tw := &typeWriter{w: &buf}
	tw.writeByte(byte(0x0A)) // kind.Union
	tw.writeUint32(0xFFFFFF) // very large count

	_, err := Decode(buf.Bytes())
	if err == nil {
		t.Error("expected error for corrupted slice length")
	}
}

func TestDecode_UnknownKind(t *testing.T) {
	_, err := Decode([]byte{0xFF})
	if err == nil {
		t.Error("expected error for unknown kind")
	}
}

func TestEncode_WriteError(t *testing.T) {
	// Create a writer that errors
	tw := &typeWriter{w: &errorWriter{}}
	tw.writeType(typ.NewRecord().Field("x", typ.Number).Build())

	if tw.err == nil {
		t.Error("expected error from errorWriter")
	}
}

type errorWriter struct{}

func (e *errorWriter) Write(_ []byte) (n int, err error) {
	return 0, ErrCorruptedData
}

type bogusType struct{}

func (bogusType) Kind() kind.Kind { return kind.Kind(255) }
func (bogusType) String() string  { return "bogus" }
func (bogusType) Hash() uint64    { return 0 }
func (bogusType) Equals(other typ.Type) bool {
	_, ok := other.(bogusType)
	return ok
}

func TestEncode_UnknownType(t *testing.T) {
	bogus := bogusType{}

	var buf bytes.Buffer
	tw := &typeWriter{w: &buf}
	tw.writeType(bogus)

	if !errors.Is(tw.err, ErrUnknownType) {
		t.Errorf("expected ErrUnknownType for bogus kind, got %v", tw.err)
	}
}

func TestDecode_ReadByte_Error(t *testing.T) {
	r := &typeReader{r: bytes.NewReader([]byte{})}
	b := r.readByte()

	if b != 0 || r.err == nil {
		t.Error("expected error reading from empty reader")
	}
}

func TestDecode_ReadString_Empty(t *testing.T) {
	var buf bytes.Buffer
	tw := &typeWriter{w: &buf}
	tw.writeString("")

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	s := r.readString()

	if s != "" || r.err != nil {
		t.Errorf("expected empty string, got %q, err: %v", s, r.err)
	}
}

func TestDecode_ReadString_RejectsLengthBeyondInput(t *testing.T) {
	var buf bytes.Buffer
	tw := &typeWriter{w: &buf}
	tw.writeUint32(1024)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	if got := r.readString(); got != "" || !errors.Is(r.err, ErrCorruptedData) {
		t.Fatalf("readString() = %q, err=%v", got, r.err)
	}
}

func TestDecode_Literal_UnknownBase(t *testing.T) {
	// kind.Literal is 23 (0x17)
	var buf bytes.Buffer
	tw := &typeWriter{w: &buf}
	tw.writeByte(23)        // kind.Literal
	tw.writeByte(byte(100)) // unknown base kind (not bool/int/number/string)

	decoded, err := Decode(buf.Bytes())
	// Should either error or return nil for invalid literal
	if err == nil && decoded != nil {
		if _, ok := decoded.(*typ.Literal); ok {
			t.Error("expected error or nil for unknown literal base")
		}
	}
}

func TestEncodeDecode_EffectWithTail(t *testing.T) {
	row := effect.Row{
		Labels: []effect.Label{effect.IO{}},
		Tail:   &effect.Var{Name: "E"},
	}
	original := typ.Func().
		Param("x", typ.String).
		Returns(typ.Number).
		Effects(row).
		Build()

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	fn := decoded.(*typ.Function)
	eff, ok := fn.Effects.(effect.Row)

	if !ok {
		t.Fatalf("expected effect.Row, got %T", fn.Effects)
	}

	if eff.Tail == nil {
		t.Error("expected effect tail")
	}

	if eff.Tail.Name != "E" {
		t.Errorf("expected tail name E, got %s", eff.Tail.Name)
	}
}

func TestEncodeDecode_ExtendedKinds(t *testing.T) {
	sum := typ.NewSum("Option", []typ.Variant{
		{Tag: "Some", Types: []typ.Type{typ.String}},
		{Tag: "None"},
	})

	iface := typ.NewInterface("Validator", []typ.Method{
		{
			Name: "validate",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("value", typ.Any).
				Returns(typ.Boolean).
				Build(),
		},
	})

	typeParam := typ.NewTypeParam("T", nil)
	fieldAccess := typ.NewFieldAccess(typeParam, "name")
	indexAccess := typ.NewIndexAccess(typ.NewArray(typeParam), typ.Integer)

	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", typ.NewOptional(self)).Build()
	})

	annotated := typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: int64(1)},
		{Name: "max", Arg: float64(10.5)},
	})

	cases := []struct {
		name string
		typ  typ.Type
	}{
		{"Sum", sum},
		{"Interface", iface},
		{"FieldAccess", fieldAccess},
		{"IndexAccess", indexAccess},
		{"Recursive", recursive},
		{"Annotated", annotated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Encode(tc.typ)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			decoded, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if !typ.TypeEquals(tc.typ, decoded) {
				t.Errorf("got %s, want %s", decoded, tc.typ)
			}
		})
	}
}

func TestEncodeDecode_GenericNoBody(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	original := typ.NewGeneric("NoBody", []*typ.TypeParam{param}, nil)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	gen := decoded.(*typ.Generic)
	if gen.Body != nil {
		t.Error("expected nil body")
	}
}

func TestEncodeDecode_TypeParam(t *testing.T) {
	original := typ.NewTypeParam("T", typ.Number)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	tp := decoded.(*typ.TypeParam)
	if tp.Name != "T" {
		t.Errorf("expected name T, got %s", tp.Name)
	}

	if tp.Constraint != typ.Number {
		t.Error("constraint mismatch")
	}
}

func TestEncodeDecode_InstantiatedMultipleArgs(t *testing.T) {
	// Test with multiple type arguments
	param1 := typ.NewTypeParam("K", nil)
	param2 := typ.NewTypeParam("V", nil)
	generic := typ.NewGeneric("Map", []*typ.TypeParam{param1, param2}, typ.Any)
	original := typ.Instantiate(generic, typ.String, typ.Number)

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	inst := decoded.(*typ.Instantiated)
	if len(inst.TypeArgs) != 2 {
		t.Errorf("expected 2 type args, got %d", len(inst.TypeArgs))
	}
}

func TestEncodeDecode_CallbackEnvOverlay(t *testing.T) {
	migrationFn := typ.Func().Param("name", typ.String).Returns(typ.Nil).Build()
	databaseFn := typ.Func().Returns(typ.Any).Build()

	spec := contract.NewSpec().
		WithCallback(0, (&contract.CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: contract.CardExactlyOnce,
		}).WithEnvOverlay(map[string]typ.Type{
			"migration": migrationFn,
			"database":  databaseFn,
		}))

	original := typ.Func().
		Param("cb", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Build()
	original.Spec = spec

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	fn := decoded.(*typ.Function)
	decodedSpec, ok := fn.Spec.(*contract.Spec)

	if !ok || decodedSpec == nil {
		t.Fatal("expected contract spec")
	}

	cb := decodedSpec.GetCallback(0)
	if cb == nil {
		t.Fatal("callback at index 0 should exist")
	}

	if len(cb.EnvOverlay) != 2 {
		t.Fatalf("EnvOverlay len = %d, want 2", len(cb.EnvOverlay))
	}

	if cb.EnvOverlay["migration"] == nil {
		t.Error("migration entry missing from EnvOverlay")
	}

	if cb.EnvOverlay["database"] == nil {
		t.Error("database entry missing from EnvOverlay")
	}

	// Verify the types round-tripped correctly
	mFn, ok := cb.EnvOverlay["migration"].(*typ.Function)
	if !ok {
		t.Fatal("migration should be a function type")
	}

	if len(mFn.Params) != 1 || mFn.Params[0].Name != "name" {
		t.Error("migration function params not preserved")
	}
}

func TestEncodeDecode_CallbackEmptyEnvOverlay(t *testing.T) {
	spec := contract.NewSpec().
		WithCallback(0, contract.PredicateSpec(0))

	original := typ.Func().
		Param("x", typ.Any).
		Param("pred", typ.Any).
		Returns(typ.Any).
		Build()
	original.Spec = spec

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	fn := decoded.(*typ.Function)
	decodedSpec := fn.Spec.(*contract.Spec)

	cb := decodedSpec.GetCallback(0)
	if cb == nil {
		t.Fatal("callback at index 0 should exist")
	}

	if len(cb.EnvOverlay) != 0 {
		t.Errorf("EnvOverlay should be empty, got %d entries", len(cb.EnvOverlay))
	}
}

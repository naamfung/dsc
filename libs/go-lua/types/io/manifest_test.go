package io

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewManifest(t *testing.T) {
	m := NewManifest("test/path")
	if m.Path != "test/path" {
		t.Errorf("Path = %q, want %q", m.Path, "test/path")
	}
	if m.Types == nil {
		t.Error("Types should be initialized")
	}
	if m.Summaries == nil {
		t.Error("Summaries should be initialized")
	}
	if m.Globals == nil {
		t.Error("Globals should be initialized")
	}
}

func TestNewSummary(t *testing.T) {
	params := []typ.Type{typ.String, typ.Number}
	returns := []typ.Type{typ.Boolean}
	s := NewSummary(params, returns)

	if len(s.Params) != 2 {
		t.Errorf("Params len = %d, want 2", len(s.Params))
	}
	if len(s.Returns) != 1 {
		t.Errorf("Returns len = %d, want 1", len(s.Returns))
	}
	if s.ReturnsParam != -1 {
		t.Errorf("ReturnsParam = %d, want -1", s.ReturnsParam)
	}
}

func TestManifest_DefineType(t *testing.T) {
	m := NewManifest("test")
	m.DefineType("MyType", typ.String)

	got, ok := m.LookupType("MyType")
	if !ok {
		t.Fatal("LookupType should find defined type")
	}
	if got != typ.String {
		t.Errorf("got %v, want string", got)
	}
}

func TestManifest_LookupType_ResolvesLocalRefs(t *testing.T) {
	m := NewManifest("test")
	customization := typ.NewRecord().
		Field("custom_css", typ.String).
		Field("css_variables", typ.NewMap(typ.String, typ.String)).
		Field("icons", typ.NewMap(typ.String, typ.String)).
		Build()
	facadeConfig := typ.NewRecord().
		Field("customization", typ.NewRef("", "Customization")).
		Build()
	m.DefineType("Customization", customization)
	m.DefineType("FacadeConfig", facadeConfig)

	got, ok := m.LookupType("FacadeConfig")
	if !ok || got == nil {
		t.Fatal("LookupType should resolve FacadeConfig")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", got)
	}
	field := rec.GetField("customization")
	if field == nil {
		t.Fatal("expected customization field")
	}
	if _, isRef := field.Type.(*typ.Ref); isRef {
		t.Fatalf("expected resolved type, still got ref: %v", field.Type)
	}
	if !typ.TypeEquals(field.Type, customization) {
		t.Fatalf("expected customization record, got %v", field.Type)
	}
}

func TestManifest_DefineSummary(t *testing.T) {
	m := NewManifest("test")
	s := NewSummary(nil, nil)
	m.DefineSummary("myFunc", s)

	got, ok := m.LookupSummary("myFunc")
	if !ok {
		t.Fatal("LookupSummary should find defined summary")
	}
	if got != s {
		t.Error("got different summary")
	}
}

func TestManifest_SetExport_Basic(t *testing.T) {
	m := NewManifest("test")
	m.SetExport(typ.String)
	if m.Export != typ.String {
		t.Error("Export should be set")
	}
}

func TestManifest_AddGlobal(t *testing.T) {
	m := NewManifest("test")
	m.AddGlobal("print", typ.Any)
	if m.Globals["print"] != typ.Any {
		t.Error("Global should be set")
	}
}

func TestManifest_AllTypes(t *testing.T) {
	m := NewManifest("test")
	m.DefineType("A", typ.String)
	m.DefineType("B", typ.Number)

	all := m.AllTypes()
	if len(all) != 2 {
		t.Errorf("AllTypes len = %d, want 2", len(all))
	}
}

func TestManifest_AllTypes_Nil(t *testing.T) {
	var m *Manifest
	all := m.AllTypes()
	if all != nil {
		t.Error("nil manifest AllTypes should return nil")
	}
}

func TestManifest_AllSummaries(t *testing.T) {
	m := NewManifest("test")
	m.DefineSummary("a", NewSummary(nil, nil))
	m.DefineSummary("b", NewSummary(nil, nil))

	all := m.AllSummaries()
	if len(all) != 2 {
		t.Errorf("AllSummaries len = %d, want 2", len(all))
	}
}

func TestManifest_AllGlobals(t *testing.T) {
	m := NewManifest("test")
	m.AddGlobal("x", typ.String)

	all := m.AllGlobals()
	if len(all) != 1 {
		t.Errorf("AllGlobals len = %d, want 1", len(all))
	}
}

func TestFunctionSummary_Clone(t *testing.T) {
	s := &FunctionSummary{
		Params:       []typ.Type{typ.String},
		Returns:      []typ.Type{typ.Boolean},
		ReturnsParam: 0,
	}
	clone := s.Clone()
	if clone == s {
		t.Error("Clone should return different instance")
	}
	if len(clone.Params) != 1 {
		t.Error("Clone should copy params")
	}
	if len(clone.Returns) != 1 {
		t.Error("Clone should copy returns")
	}
}

func TestFunctionSummary_Clone_Full(t *testing.T) {
	s := &FunctionSummary{
		Params:       []typ.Type{typ.String, typ.Number},
		Returns:      []typ.Type{typ.Boolean},
		ReturnsParam: 1,
		ParamEscapes: []bool{true, false},
	}
	clone := s.Clone()
	if len(clone.ParamEscapes) != 2 {
		t.Errorf("ParamEscapes len = %d, want 2", len(clone.ParamEscapes))
	}
	if !clone.ParamEscapes[0] || clone.ParamEscapes[1] {
		t.Error("ParamEscapes values not preserved")
	}
}

func TestFunctionSummary_Clone_Nil(t *testing.T) {
	var s *FunctionSummary
	clone := s.Clone()
	if clone != nil {
		t.Error("nil summary Clone should return nil")
	}
}

func TestFunctionSummary_Clone_WithConditions(t *testing.T) {
	s := &FunctionSummary{
		Params:  []typ.Type{typ.String},
		Returns: []typ.Type{typ.Boolean},
		Requires: constraint.FromConstraints(
			constraint.NotNil{Path: constraint.Path{Root: "$0"}},
		),
		Ensures: constraint.FromConstraints(
			constraint.Truthy{Path: constraint.Path{Root: "result"}},
		),
	}
	clone := s.Clone()
	if !clone.Requires.HasConstraints() {
		t.Error("Requires should be cloned")
	}
	if !clone.Ensures.HasConstraints() {
		t.Error("Ensures should be cloned")
	}
}

func TestManifest_Encode_Decode(t *testing.T) {
	m := NewManifest("test/module")
	m.DefineType("MyType", typ.String)
	m.SetExport(typ.Number)

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}
	if decoded.Path != "test/module" {
		t.Errorf("Path = %q, want %q", decoded.Path, "test/module")
	}
}

func TestManifest_Encode_Decode_LargeValidManifest(t *testing.T) {
	m := NewManifest(strings.Repeat("module/", 32))
	for i := 0; i < 2048; i++ {
		m.AddGlobal(fmt.Sprintf("global_%04d", i), typ.String)
	}

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}
	if decoded.Path != m.Path || len(decoded.Globals) != len(m.Globals) {
		t.Fatalf("decoded manifest differs: path=%q globals=%d", decoded.Path, len(decoded.Globals))
	}
}

func TestManifest_EnrichedExport_DoesNotApplySummaryToNestedSameName(t *testing.T) {
	m := NewManifest("test")

	topValidate := typ.Func().Param("x", typ.String).Returns(typ.Boolean).Build()
	nestedValidate := typ.Func().Param("x", typ.String).Returns(typ.Boolean).Build()
	nestedRecord := typ.NewRecord().Field("validate", nestedValidate).Build()
	exportRec := typ.NewRecord().
		Field("validate", topValidate).
		Field("nested", nestedRecord).
		Build()
	m.SetExport(exportRec)

	summary := NewSummary([]typ.Type{typ.String}, []typ.Type{typ.Boolean})
	summary.Ensures = constraint.FromConstraints(constraint.Truthy{
		Path: constraint.Path{Root: "result"},
	})
	m.DefineSummary("validate", summary)

	enriched := m.EnrichedExport()
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected record export, got %T", enriched)
	}

	topField := rec.GetField("validate")
	if topField == nil {
		t.Fatal("missing top-level validate field")
	}
	topFn, ok := topField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("top-level validate is not a function: %T", topField.Type)
	}
	if topFn.Refinement == nil {
		t.Fatal("expected top-level validate to be enriched")
	}

	nestedField := rec.GetField("nested")
	if nestedField == nil {
		t.Fatal("missing nested field")
	}
	nestedRec, ok := nestedField.Type.(*typ.Record)
	if !ok {
		t.Fatalf("nested field is not a record: %T", nestedField.Type)
	}
	nestedValidateField := nestedRec.GetField("validate")
	if nestedValidateField == nil {
		t.Fatal("missing nested validate field")
	}
	nestedFn, ok := nestedValidateField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("nested validate is not a function: %T", nestedValidateField.Type)
	}
	if nestedFn.Refinement != nil {
		t.Fatalf("nested validate should not be enriched, got refinement %#v", nestedFn.Refinement)
	}
}

func TestManifest_EnrichedExport_ResolvesLocalRefs(t *testing.T) {
	m := NewManifest("test")
	customization := typ.NewRecord().
		Field("custom_css", typ.String).
		Field("css_variables", typ.NewMap(typ.String, typ.String)).
		Field("icons", typ.NewMap(typ.String, typ.String)).
		Build()
	export := typ.NewRecord().
		Field("customization", typ.NewRef("", "Customization")).
		Build()
	m.DefineType("Customization", customization)
	m.SetExport(export)

	enriched := m.EnrichedExport()
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected record export, got %T", enriched)
	}
	field := rec.GetField("customization")
	if field == nil {
		t.Fatal("expected customization field")
	}
	if _, isRef := field.Type.(*typ.Ref); isRef {
		t.Fatalf("expected resolved export field type, got ref: %v", field.Type)
	}
	if !typ.TypeEquals(field.Type, customization) {
		t.Fatalf("expected customization record, got %v", field.Type)
	}
}

func TestDecodeManifest_InvalidMagic(t *testing.T) {
	_, err := DecodeManifest([]byte{0, 0, 0, 0})
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestDecodeManifest_RejectsCollectionLengthBeyondInput(t *testing.T) {
	var buf bytes.Buffer
	w := &manifestWriter{typeWriter: &typeWriter{w: &buf}}
	w.writeUint32(manifestMagic)
	w.writeByte(manifestVersion)
	w.writeUint64(0)
	w.writeString("test")
	w.writeBool(false)
	w.writeUint32(1024)

	if _, err := DecodeManifest(buf.Bytes()); !errors.Is(err, ErrCorruptedData) {
		t.Fatalf("DecodeManifest error = %v", err)
	}
}

func TestApplyFunctionSummary_Nil(t *testing.T) {
	fn := typ.Func().Build()
	result := ApplyFunctionSummary(fn, nil)
	if result != fn {
		t.Error("nil summary should return original function")
	}
}

func TestApplyFunctionSummary_EmptySummary(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	summary := NewSummary(nil, nil)
	result := ApplyFunctionSummary(fn, summary)
	if result != fn {
		t.Error("empty summary should return original function")
	}
}

func TestApplyFunctionSummary_WithOptionalAndVariadic(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.String).
		OptParam("y", typ.Number).
		Variadic(typ.Any).
		Returns(typ.Boolean).
		Build()
	summary := NewSummary([]typ.Type{typ.String, typ.Number}, []typ.Type{typ.Boolean})
	summary.Ensures = constraint.FromConstraints(constraint.NotNil{Path: constraint.Path{Root: "result"}})

	result := ApplyFunctionSummary(fn, summary)
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(result.Params))
	}
	if result.Variadic == nil {
		t.Error("variadic should be preserved")
	}
	if !result.Params[1].Optional {
		t.Error("second param should be optional")
	}

	result = ApplyFunctionSummary(nil, &FunctionSummary{})
	if result != nil {
		t.Error("nil function should return nil")
	}
}

func TestManifest_EnrichedExport_Empty(t *testing.T) {
	m := NewManifest("test")
	if m.EnrichedExport() != nil {
		t.Error("manifest with no export should return nil")
	}
}

func TestManifest_EnrichedExport_SummarySuffixFallback(t *testing.T) {
	m := NewManifest("test")

	baseFn := typ.Func().
		Param("v", typ.Any).
		OptParam("msg", typ.String).
		Returns(typ.Any).
		Build()
	m.SetExport(typ.NewRecord().Field("not_nil", baseFn).Build())

	summary := NewSummary([]typ.Type{typ.Any, typ.NewOptional(typ.String)}, []typ.Type{typ.Any})
	summary.Ensures = constraint.FromConstraints(constraint.NotNil{Path: constraint.ParamPath(0)})
	m.DefineSummary("test.not_nil", summary)

	enriched := m.EnrichedExport()
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected record export, got %T", enriched)
	}
	field := rec.GetField("not_nil")
	if field == nil {
		t.Fatal("missing not_nil field")
	}
	fn, ok := field.Type.(*typ.Function)
	if !ok {
		t.Fatalf("not_nil field is not function: %T", field.Type)
	}
	refinement, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok || refinement == nil || !refinement.OnReturn.HasConstraints() {
		t.Fatalf("expected suffix-matched summary refinement on not_nil, got %#v", fn.Refinement)
	}

	if _, found := m.LookupSummary("not_nil"); !found {
		t.Fatal("expected LookupSummary suffix fallback for not_nil")
	}
}

func TestManifest_EnrichedExport_SummarySuffixAmbiguityDoesNotGuess(t *testing.T) {
	m := NewManifest("test")
	baseFn := typ.Func().
		Param("actual", typ.Any).
		Param("expected", typ.Any).
		Build()
	m.SetExport(typ.NewRecord().Field("eq", baseFn).Build())

	s1 := NewSummary([]typ.Type{typ.Any, typ.Any}, nil)
	s1.Ensures = constraint.FromConstraints(constraint.Truthy{Path: constraint.ParamPath(0)})
	s2 := NewSummary([]typ.Type{typ.Any, typ.Any}, nil)
	s2.Ensures = constraint.FromConstraints(constraint.Truthy{Path: constraint.ParamPath(1)})
	m.DefineSummary("assert.eq", s1)
	m.DefineSummary("test.eq", s2)

	enriched := m.EnrichedExport()
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected record export, got %T", enriched)
	}
	field := rec.GetField("eq")
	if field == nil {
		t.Fatal("missing eq field")
	}
	fn, ok := field.Type.(*typ.Function)
	if !ok {
		t.Fatalf("eq field is not function: %T", field.Type)
	}
	if fn.Refinement != nil {
		t.Fatalf("ambiguous suffix summaries must not be applied, got refinement %#v", fn.Refinement)
	}

	if _, found := m.LookupSummary("eq"); found {
		t.Fatal("expected LookupSummary to reject ambiguous suffix match")
	}
}

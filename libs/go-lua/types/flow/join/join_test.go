package join

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestTypes_Empty(t *testing.T) {
	result := Types()
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestTypes_Single(t *testing.T) {
	result := Types(typ.String)
	if result != typ.String {
		t.Errorf("expected string, got %v", result)
	}
}

func TestTypes_AllEqual(t *testing.T) {
	result := Types(typ.String, typ.String, typ.String)
	if result != typ.String {
		t.Errorf("expected string, got %v", result)
	}
}

func TestTypes_Different(t *testing.T) {
	result := Types(typ.String, typ.Number)
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(u.Members))
	}
}

func TestCoalesceMaps_NoMaps(t *testing.T) {
	input := []typ.Type{typ.String, typ.Number}
	result := CoalesceMaps(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types, got %d", len(result))
	}
}

func TestCoalesceMaps_SingleMap(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	input := []typ.Type{m, typ.Boolean}
	result := CoalesceMaps(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types (no coalescing), got %d", len(result))
	}
}

func TestCoalesceMaps_MultipleMaps(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.Integer, typ.Boolean)
	input := []typ.Type{m1, m2}
	result := CoalesceMaps(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}
	m, ok := result[0].(*typ.Map)
	if !ok {
		t.Fatalf("expected map, got %T", result[0])
	}
	// Key should be string | integer
	if _, ok := m.Key.(*typ.Union); !ok {
		t.Errorf("expected union key type, got %T", m.Key)
	}
	// Value should be number | boolean
	if _, ok := m.Value.(*typ.Union); !ok {
		t.Errorf("expected union value type, got %T", m.Value)
	}
}

func TestCoalesceRecordMapComponents_MergesMatchingFieldShapes(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.String).Build()).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.String).Build()).
		MapComponent(typ.String, typ.Boolean).
		Build()

	result := CoalesceRecordMapComponents([]typ.Type{left, right})
	if len(result) != 1 {
		t.Fatalf("expected 1 merged record, got %d", len(result))
	}
	rec, ok := result[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", result[0])
	}
	if !rec.HasMapComponent() {
		t.Fatal("expected merged record to keep map component")
	}
	if _, ok := rec.MapValue.(*typ.Union); !ok {
		t.Fatalf("expected merged map value union, got %T", rec.MapValue)
	}
}

func TestCoalesceRecordMapComponents_DoesNotMergeDifferentFieldTypes(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.String).Build()).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.Integer).Build()).
		MapComponent(typ.String, typ.Boolean).
		Build()

	result := CoalesceRecordMapComponents([]typ.Type{left, right})
	if len(result) != 2 {
		t.Fatalf("expected distinct records to remain separate, got %d", len(result))
	}
}

func TestCoalesceEmptyRecordWithMap_NoEmptyRecord(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	input := []typ.Type{m, rec}
	result := CoalesceEmptyRecordWithMap(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types, got %d", len(result))
	}
}

func TestCoalesceEmptyRecordWithMap_NoMap(t *testing.T) {
	rec := typ.NewRecord().Build()
	input := []typ.Type{typ.String, rec}
	result := CoalesceEmptyRecordWithMap(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types, got %d", len(result))
	}
}

func TestCoalesceEmptyRecordWithMap_BothPresent(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	rec := typ.NewRecord().Build()
	input := []typ.Type{m, rec}
	result := CoalesceEmptyRecordWithMap(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 type (empty record removed), got %d", len(result))
	}
	if _, ok := result[0].(*typ.Map); !ok {
		t.Errorf("expected map to remain, got %T", result[0])
	}
}

func TestCoalesceEmptyRecordWithArray_BothPresent(t *testing.T) {
	arr := typ.NewArray(typ.Number)
	rec := typ.NewRecord().Build()
	input := []typ.Type{arr, rec}
	result := CoalesceEmptyRecordWithArray(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 type (empty record removed), got %d", len(result))
	}
	if _, ok := result[0].(*typ.Array); !ok {
		t.Errorf("expected array to remain, got %T", result[0])
	}
}

func TestIsEmptyRecord_Nil(t *testing.T) {
	if unwrap.IsEmptyRecord(nil) {
		t.Error("nil should not be empty record")
	}
}

func TestIsEmptyRecord_NonRecord(t *testing.T) {
	if unwrap.IsEmptyRecord(typ.String) {
		t.Error("string should not be empty record")
	}
}

func TestIsEmptyRecord_EmptyRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	if !unwrap.IsEmptyRecord(rec) {
		t.Error("empty record should be empty record")
	}
}

func TestIsEmptyRecord_NonEmptyRecord(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	if unwrap.IsEmptyRecord(rec) {
		t.Error("record with fields should not be empty record")
	}
}

func TestTypes_IntegrationWithCoalescing(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.String, typ.Boolean)
	emptyRec := typ.NewRecord().Build()

	result := Types(m1, m2, emptyRec)

	// Should coalesce maps and remove empty record
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("expected single map after coalescing, got %T", result)
	}
	if m.Key != typ.String {
		t.Errorf("expected string key, got %v", m.Key)
	}
	// Value should be number | boolean
	if _, ok := m.Value.(*typ.Union); !ok {
		t.Errorf("expected union value type, got %T", m.Value)
	}
}

func TestTypes_IntegrationWithArrayCoalescing(t *testing.T) {
	arr := typ.NewArray(typ.String)
	emptyRec := typ.NewRecord().Build()

	result := Types(arr, emptyRec)
	gotArr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected array after coalescing, got %T", result)
	}
	if !typ.TypeEquals(gotArr.Element, typ.String) {
		t.Fatalf("expected string[] element, got %v", gotArr.Element)
	}
}

func TestTypes_Idempotence(t *testing.T) {
	// join(x, y) repeated should yield TypeEquals true
	j1 := Types(typ.Number, typ.String)
	j2 := Types(typ.Number, typ.String)

	if !typ.TypeEquals(j1, j2) {
		t.Error("repeated joins should be equal")
	}

	if j1.Hash() != j2.Hash() {
		t.Error("repeated joins should have same hash")
	}
}

func TestTypes_IdempotenceNested(t *testing.T) {
	// join(join(A, B), A) == join(A, B)
	j1 := Types(typ.Number, typ.String)
	j2 := Types(j1, typ.Number)

	if !typ.TypeEquals(j1, j2) {
		t.Error("adding existing member via join should not change result")
	}
}

func TestTypes_OrderIndependence(t *testing.T) {
	j1 := Types(typ.Number, typ.String, typ.Boolean)
	j2 := Types(typ.Boolean, typ.Number, typ.String)
	j3 := Types(typ.String, typ.Boolean, typ.Number)

	if !typ.TypeEquals(j1, j2) {
		t.Error("j1 should equal j2")
	}
	if !typ.TypeEquals(j2, j3) {
		t.Error("j2 should equal j3")
	}
}

func TestTypes_WithUnionInput(t *testing.T) {
	// Joining with an existing union should flatten
	u := typ.NewUnion(typ.Number, typ.String)
	j := Types(u, typ.Boolean)

	result, ok := j.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", j)
	}

	if len(result.Members) != 3 {
		t.Errorf("expected 3 members, got %d", len(result.Members))
	}
}

func TestTypes_AllUnknown(t *testing.T) {
	result := Types(typ.Unknown, typ.Unknown, typ.Unknown)
	if result != typ.Unknown {
		t.Errorf("all unknown should return unknown, got %v", result)
	}
}

func TestTypes_MixedUnknown(t *testing.T) {
	result := Types(typ.String, typ.Unknown, typ.Number)
	// Unknown should be filtered out
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	// Should have string and number only
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members after filtering unknown, got %d", len(u.Members))
	}
}

func TestCoalesceCompatibleRecords_MergesExtraFieldsAsOptional(t *testing.T) {
	base := typ.NewRecord().
		Field("status_code", typ.Number).
		Field("message", typ.String).
		Build()
	withDetails := typ.NewRecord().
		Field("status_code", typ.Number).
		Field("message", typ.String).
		Field("code", typ.String).
		Field("type", typ.String).
		Build()

	result := CoalesceCompatibleRecords([]typ.Type{base, withDetails})
	if len(result) != 1 {
		t.Fatalf("expected one merged record, got %d", len(result))
	}

	rec, ok := result[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", result[0])
	}
	fields := map[string]typ.Field{}
	for _, f := range rec.Fields {
		fields[f.Name] = f
	}
	if !fields["code"].Optional || !typ.TypeEquals(fields["code"].Type, typ.String) {
		t.Fatalf("expected optional code:string, got %#v", fields["code"])
	}
	if !fields["type"].Optional || !typ.TypeEquals(fields["type"].Type, typ.String) {
		t.Fatalf("expected optional type:string, got %#v", fields["type"])
	}
}

func TestCoalesceCompatibleRecords_PreservesDiscriminatedUnion(t *testing.T) {
	a := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("x", typ.Number).
		Build()
	b := typ.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("y", typ.String).
		Build()

	result := CoalesceCompatibleRecords([]typ.Type{a, b})
	if len(result) != 2 {
		t.Fatalf("expected discriminated variants to remain separate, got %d", len(result))
	}
}

func TestTypes_WithNil(t *testing.T) {
	result := Types(typ.String, nil, typ.Number)
	// Nil should be filtered out
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members after filtering nil, got %d", len(u.Members))
	}
}

func TestTypes_AllNil(t *testing.T) {
	result := Types(nil, nil, nil)
	if result != typ.Unknown {
		t.Errorf("all nil should return unknown, got %v", result)
	}
}

func TestTypes_SingleUnknown(t *testing.T) {
	result := Types(typ.Unknown)
	if result != typ.Unknown {
		t.Errorf("single unknown should return unknown, got %v", result)
	}
}

func TestTypes_UnknownAndSingle(t *testing.T) {
	result := Types(typ.Unknown, typ.String)
	if result != typ.String {
		t.Errorf("unknown and string should return string, got %v", result)
	}
}

func TestCoalesceMaps_NilInput(t *testing.T) {
	result := CoalesceMaps(nil)
	if result != nil {
		t.Errorf("nil input should return nil, got %v", result)
	}
}

func TestCoalesceMaps_EmptyInput(t *testing.T) {
	result := CoalesceMaps([]typ.Type{})
	if len(result) != 0 {
		t.Errorf("empty input should return empty, got %d elements", len(result))
	}
}

func TestCoalesceMaps_WithNilElements(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.String, typ.Boolean)
	input := []typ.Type{m1, nil, m2}
	result := CoalesceMaps(input)
	// Should coalesce the two maps and skip nil
	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}
}

func TestCoalesceMaps_ThreeMaps(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.Integer, typ.Boolean)
	m3 := typ.NewMap(typ.Boolean, typ.String)
	input := []typ.Type{m1, m2, m3}
	result := CoalesceMaps(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}
	m, ok := result[0].(*typ.Map)
	if !ok {
		t.Fatalf("expected map, got %T", result[0])
	}
	// Key should be string | integer | boolean
	if _, ok := m.Key.(*typ.Union); !ok {
		t.Errorf("expected union key type, got %T", m.Key)
	}
}

func TestCoalesceEmptyRecordWithMap_MultipleEmptyRecords(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	rec1 := typ.NewRecord().Build()
	rec2 := typ.NewRecord().Build()
	input := []typ.Type{m, rec1, rec2, typ.String}
	result := CoalesceEmptyRecordWithMap(input)
	// Both empty records should be removed
	if len(result) != 2 {
		t.Errorf("expected 2 types (map and string), got %d", len(result))
	}
	for _, r := range result {
		if unwrap.IsEmptyRecord(r) {
			t.Error("empty record should have been removed")
		}
	}
}

func TestCoalesceEmptyRecordWithMap_NilInput(t *testing.T) {
	result := CoalesceEmptyRecordWithMap(nil)
	if result != nil {
		t.Errorf("nil input should return nil, got %v", result)
	}
}

func TestTypes_RecursiveTypes(t *testing.T) {
	// Join recursive types
	rec1 := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})
	rec2 := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})

	// Should deduplicate equivalent recursive types
	result := Types(rec1, rec2)

	// Result should be equivalent to rec1
	if !typ.TypeEquals(result, rec1) {
		t.Error("joining equivalent recursive types should yield single type")
	}
}

func TestTypes_RecursiveWithOther(t *testing.T) {
	rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})

	result := Types(rec, typ.Number)

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(u.Members))
	}
}

func TestTypes_NestedMaps(t *testing.T) {
	// Map<string, Map<string, number>> and Map<string, Map<string, boolean>>
	inner1 := typ.NewMap(typ.String, typ.Number)
	inner2 := typ.NewMap(typ.String, typ.Boolean)
	m1 := typ.NewMap(typ.String, inner1)
	m2 := typ.NewMap(typ.String, inner2)

	input := []typ.Type{m1, m2}
	result := CoalesceMaps(input)

	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}

	m, ok := result[0].(*typ.Map)
	if !ok {
		t.Fatalf("expected map, got %T", result[0])
	}

	// Value should be coalesced maps
	innerM, ok := m.Value.(*typ.Map)
	if !ok {
		t.Fatalf("expected inner map, got %T", m.Value)
	}

	// Inner value should be number | boolean
	if _, ok := innerM.Value.(*typ.Union); !ok {
		t.Errorf("expected union inner value, got %T", innerM.Value)
	}
}

func TestTypes_OptionalTypes(t *testing.T) {
	opt1 := typ.NewOptional(typ.String)
	opt2 := typ.NewOptional(typ.Number)

	result := Types(opt1, opt2)

	// Should create union of optionals
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) < 2 {
		t.Errorf("expected at least 2 members, got %d", len(u.Members))
	}
}

func TestTypes_SameTypeMultiple(t *testing.T) {
	// Repeated same type should return single type
	result := Types(typ.Number, typ.Number, typ.Number, typ.Number)
	if result != typ.Number {
		t.Errorf("repeated same type should return single type, got %v", result)
	}
}

func TestTypes_MixedSameAndDifferent(t *testing.T) {
	// Some same, some different
	result := Types(typ.Number, typ.String, typ.Number, typ.String)

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	// Should deduplicate
	if len(u.Members) != 2 {
		t.Errorf("expected 2 unique members, got %d", len(u.Members))
	}
}

func TestFilterUnknown(t *testing.T) {
	tests := []struct {
		name  string
		input []typ.Type
		want  int
	}{
		{"empty", []typ.Type{}, 0},
		{"all unknown", []typ.Type{typ.Unknown, typ.Unknown}, 0},
		{"all nil", []typ.Type{nil, nil}, 0},
		{"mixed", []typ.Type{typ.String, typ.Unknown, typ.Number, nil}, 2},
		{"no unknown", []typ.Type{typ.String, typ.Number}, 2},
	}
	for _, tt := range tests {
		result := filterUnknown(tt.input)
		if len(result) != tt.want {
			t.Errorf("%s: filterUnknown returned %d elements, want %d", tt.name, len(result), tt.want)
		}
	}
}

func TestCoalesceRecordOpenness_NoRecords(t *testing.T) {
	input := []typ.Type{typ.String, typ.Number}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types unchanged, got %d", len(result))
	}
}

func TestCoalesceRecordOpenness_AllOpen(t *testing.T) {
	r1 := typ.NewRecord().SetOpen(true).Field("x", typ.Number).Build()
	r2 := typ.NewRecord().SetOpen(true).Field("y", typ.String).Build()
	input := []typ.Type{r1, r2}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types unchanged, got %d", len(result))
	}
}

func TestCoalesceRecordOpenness_AllClosed(t *testing.T) {
	r1 := typ.NewRecord().Field("x", typ.Number).Build()
	r2 := typ.NewRecord().Field("y", typ.String).Build()
	input := []typ.Type{r1, r2}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types unchanged, got %d", len(result))
	}
}

func TestCoalesceRecordOpenness_MixedOpenClosed(t *testing.T) {
	open := typ.NewRecord().SetOpen(true).Field("x", typ.Number).Build()
	closed := typ.NewRecord().Field("y", typ.String).Build()
	input := []typ.Type{open, closed}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 types, got %d", len(result))
	}
	for _, r := range result {
		rec, ok := r.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", r)
		}
		if !rec.Open {
			t.Error("all records should be open after coalescing")
		}
	}
}

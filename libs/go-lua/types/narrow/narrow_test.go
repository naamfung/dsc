package narrow_test

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRemoveNil_Nil(t *testing.T) {
	got := narrow.RemoveNil(typ.Nil)
	if got != typ.Never {
		t.Errorf("RemoveNil(nil) = %v, want never", got)
	}
}

func TestRemoveNil_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.RemoveNil(opt)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("RemoveNil(string?) = %v, want string", got)
	}
}

func TestRemoveNil_Union(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Nil, typ.Number)
	got := narrow.RemoveNil(union)
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Errorf("RemoveNil(string|nil|number) = %v, want %v", got, want)
	}
}

func TestRemoveNil_UnionAllNil(t *testing.T) {
	union := typ.NewUnion(typ.Nil, typ.Nil)
	got := narrow.RemoveNil(union)
	if got != typ.Never {
		t.Errorf("RemoveNil(nil|nil) = %v, want never", got)
	}
}

func TestRemoveNil_NonNullable(t *testing.T) {
	got := narrow.RemoveNil(typ.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("RemoveNil(string) = %v, want string", got)
	}
}

func TestRemoveFalse_LiteralFalse(t *testing.T) {
	got := narrow.RemoveFalse(typ.LiteralBool(false))
	if got != typ.Never {
		t.Errorf("RemoveFalse(false) = %v, want never", got)
	}
}

func TestRemoveFalse_LiteralTrue(t *testing.T) {
	got := narrow.RemoveFalse(typ.LiteralBool(true))
	if !typ.TypeEquals(got, typ.True) {
		t.Errorf("RemoveFalse(true) = %v, want true", got)
	}
}

func TestRemoveFalse_Boolean(t *testing.T) {
	got := narrow.RemoveFalse(typ.Boolean)
	if !typ.TypeEquals(got, typ.True) {
		t.Errorf("RemoveFalse(boolean) = %v, want true", got)
	}
}

func TestRemoveFalse_Union(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.LiteralBool(false), typ.Number)
	got := narrow.RemoveFalse(union)
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Errorf("RemoveFalse(string|false|number) = %v, want %v", got, want)
	}
}

func TestRemoveFalse_NonBoolean(t *testing.T) {
	got := narrow.RemoveFalse(typ.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("RemoveFalse(string) = %v, want string", got)
	}
}

func TestToTruthy_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.ToTruthy(opt)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("ToTruthy(string?) = %v, want string", got)
	}
}

func TestToTruthy_UnionWithNilAndFalse(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Nil, typ.Boolean)
	got := narrow.ToTruthy(union)
	want := typ.NewUnion(typ.String, typ.True)
	if !typ.TypeEquals(got, want) {
		t.Errorf("ToTruthy(string|nil|boolean) = %v, want %v", got, want)
	}
}

func TestToTruthy_AllFalsy(t *testing.T) {
	union := typ.NewUnion(typ.Nil, typ.LiteralBool(false))
	got := narrow.ToTruthy(union)
	if got != typ.Never {
		t.Errorf("ToTruthy(nil|false) = %v, want never", got)
	}
}

func TestToFalsy_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.ToFalsy(opt)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Errorf("ToFalsy(string?) = %v, want nil", got)
	}
}

func TestToFalsy_UnionWithNilAndBoolean(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Nil, typ.Boolean)
	got := narrow.ToFalsy(union)
	want := typ.NewUnion(typ.Nil, typ.LiteralBool(false))
	if !typ.TypeEquals(got, want) {
		t.Errorf("ToFalsy(string|nil|boolean) = %v, want %v", got, want)
	}
}

func TestToFalsy_AllTruthy(t *testing.T) {
	got := narrow.ToFalsy(typ.String)
	if got != typ.Never {
		t.Errorf("ToFalsy(string) = %v, want never", got)
	}
}

func TestToFalsy_Boolean(t *testing.T) {
	got := narrow.ToFalsy(typ.Boolean)
	want := typ.LiteralBool(false)
	if !typ.TypeEquals(got, want) {
		t.Errorf("ToFalsy(boolean) = %v, want false", got)
	}
}

func TestToFalsy_Any(t *testing.T) {
	got := narrow.ToFalsy(typ.Any)
	want := typ.NewUnion(typ.Nil, typ.LiteralBool(false))
	if !typ.TypeEquals(got, want) {
		t.Errorf("ToFalsy(any) = %v, want nil|false", got)
	}
}

func TestToFalsy_FieldAccess(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	access := typ.NewFieldAccess(tp, "value")
	got := narrow.ToFalsy(access)
	want := typ.NewUnion(typ.Nil, typ.LiteralBool(false))
	if !typ.TypeEquals(got, want) {
		t.Errorf("ToFalsy(T.value) = %v, want nil|false", got)
	}
}

func TestToFalsy_IndexAccess(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	access := typ.NewIndexAccess(tp, typ.String)
	got := narrow.ToFalsy(access)
	want := typ.NewUnion(typ.Nil, typ.LiteralBool(false))
	if !typ.TypeEquals(got, want) {
		t.Errorf("ToFalsy(T[string]) = %v, want nil|false", got)
	}
}

func TestTypesOverlap_Same(t *testing.T) {
	if !narrow.TypesOverlap(typ.String, typ.String) {
		t.Error("TypesOverlap(string, string) = false, want true")
	}
}

func TestTypesOverlap_SubtypeAB(t *testing.T) {
	// string is subtype of string|number
	union := typ.NewUnion(typ.String, typ.Number)
	if !narrow.TypesOverlap(typ.String, union) {
		t.Error("TypesOverlap(string, string|number) = false, want true")
	}
}

func TestTypesOverlap_SubtypeBA(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	if !narrow.TypesOverlap(union, typ.String) {
		t.Error("TypesOverlap(string|number, string) = false, want true")
	}
}

func TestTypesOverlap_Disjoint(t *testing.T) {
	if narrow.TypesOverlap(typ.String, typ.Number) {
		t.Error("TypesOverlap(string, number) = true, want false")
	}
}

func TestTypesOverlap_Nil(t *testing.T) {
	if narrow.TypesOverlap(nil, typ.String) {
		t.Error("TypesOverlap(nil, string) = true, want false")
	}
}

func TestExcludeType_FromUnion(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	got := narrow.ExcludeType(union, typ.Number)
	want := typ.NewUnion(typ.String, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		t.Errorf("ExcludeType(string|number|boolean, number) = %v, want %v", got, want)
	}
}

func TestExcludeType_FromUnion_AllExcluded(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	superUnion := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	got := narrow.ExcludeType(union, superUnion)
	if got != typ.Never {
		t.Errorf("ExcludeType(string|number, string|number|boolean) = %v, want never", got)
	}
}

func TestExcludeType_NonUnion_Overlap(t *testing.T) {
	got := narrow.ExcludeType(typ.String, typ.String)
	if got != typ.Never {
		t.Errorf("ExcludeType(string, string) = %v, want never", got)
	}
}

func TestExcludeType_NonUnion_NoOverlap(t *testing.T) {
	got := narrow.ExcludeType(typ.String, typ.Number)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("ExcludeType(string, number) = %v, want string", got)
	}
}

func TestExcludeType_Any_PreservesAny(t *testing.T) {
	// ExcludeType(any, T) should return any unchanged
	// because we cannot narrow 'any' by excluding a specific type
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	got := narrow.ExcludeType(typ.Any, rec)
	if got.Kind() != kind.Any {
		t.Errorf("ExcludeType(any, Record) = %v (kind=%v), want any", got, got.Kind())
	}
}

func TestExcludeType_Unknown_PreservesUnknown(t *testing.T) {
	// ExcludeType(unknown, T) should return unknown unchanged
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	got := narrow.ExcludeType(typ.Unknown, rec)
	if got.Kind() != kind.Unknown {
		t.Errorf("ExcludeType(unknown, Record) = %v (kind=%v), want unknown", got, got.Kind())
	}
}

func TestExcludeKind_Any_PreservesAny(t *testing.T) {
	// ExcludeKind(any, kind) should return any unchanged
	got := narrow.ExcludeKind(typ.Any, kind.Record)
	if got.Kind() != kind.Any {
		t.Errorf("ExcludeKind(any, Record) = %v (kind=%v), want any", got, got.Kind())
	}
}

func TestExcludeKind_Unknown_PreservesUnknown(t *testing.T) {
	// ExcludeKind(unknown, kind) should return unknown unchanged
	got := narrow.ExcludeKind(typ.Unknown, kind.Record)
	if got.Kind() != kind.Unknown {
		t.Errorf("ExcludeKind(unknown, Record) = %v (kind=%v), want unknown", got, got.Kind())
	}
}

func TestFilterByKind_UnionPlaceholderAndNil_PreservesRuntimePossibility(t *testing.T) {
	got := narrow.FilterByKind(typ.NewUnion(typ.Unknown, typ.Nil), kind.Record)
	if got == nil || got.Kind().IsNever() {
		t.Fatalf("FilterByKind(unknown|nil, table) = %v, want table-like type", got)
	}
	if !narrow.KindMatches(got, kind.Record) {
		t.Fatalf("FilterByKind(unknown|nil, table) = %v, want table-like type", got)
	}
}

func TestFilterByKind_UnionAnyAndNil_Number(t *testing.T) {
	got := narrow.FilterByKind(typ.NewUnion(typ.Any, typ.Nil), kind.Number)
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("FilterByKind(any|nil, number) = %v, want number", got)
	}
}

func TestExcludeKind_OptionalUnknown_PreservesUnknownOptional(t *testing.T) {
	got := narrow.ExcludeKind(typ.NewOptional(typ.Unknown), kind.String)
	want := typ.NewOptional(typ.Unknown)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("ExcludeKind(unknown?, string) = %v, want %v", got, want)
	}
}

func TestExcludeType_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.NewUnion(typ.String, typ.Number))
	got := narrow.ExcludeType(opt, typ.Number)
	want := typ.NewOptional(typ.String)
	if !typ.TypeEquals(got, want) {
		t.Errorf("ExcludeType((string|number)?, number) = %v, want %v", got, want)
	}
}

func TestExcludeKind_FromUnion(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	got := narrow.ExcludeKind(union, kind.Number)
	want := typ.NewUnion(typ.String, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		t.Errorf("ExcludeKind(string|number|boolean, number) = %v, want %v", got, want)
	}
}

func TestExcludeKind_FromOptional_Nil(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.ExcludeKind(opt, kind.Nil)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("ExcludeKind(string?, nil) = %v, want string", got)
	}
}

func TestExcludeKind_NonUnion_Match(t *testing.T) {
	got := narrow.ExcludeKind(typ.String, kind.String)
	if got != typ.Never {
		t.Errorf("ExcludeKind(string, string) = %v, want never", got)
	}
}

func TestExcludeKind_NonUnion_NoMatch(t *testing.T) {
	got := narrow.ExcludeKind(typ.String, kind.Number)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("ExcludeKind(string, number) = %v, want string", got)
	}
}

func TestKindMatches_Exact(t *testing.T) {
	if !narrow.KindMatches(typ.String, kind.String) {
		t.Error("KindMatches(string, string) = false, want true")
	}
}

func TestKindMatches_TableRecord(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	if !narrow.KindMatches(rec, kind.Record) {
		t.Error("KindMatches(record, record) = false, want true")
	}
}

func TestKindMatches_IntegerNumber(t *testing.T) {
	if !narrow.KindMatches(typ.Integer, kind.Number) {
		t.Error("KindMatches(integer, number) = false, want true")
	}
}

func TestKindMatches_NoMatch(t *testing.T) {
	if narrow.KindMatches(typ.String, kind.Number) {
		t.Error("KindMatches(string, number) = true, want false")
	}
}

func TestIntersect_SubtypeAB(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	got := narrow.Intersect(typ.String, union)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(string, string|number) = %v, want string", got)
	}
}

func TestIntersect_SubtypeBA(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	got := narrow.Intersect(union, typ.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(string|number, string) = %v, want string", got)
	}
}

func TestIntersect_UnionFilter(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	other := typ.NewUnion(typ.String, typ.Boolean)
	got := narrow.Intersect(union, other)
	want := typ.NewUnion(typ.String, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		t.Errorf("Intersect(string|number|boolean, string|boolean) = %v, want %v", got, want)
	}
}

func TestIntersect_Any(t *testing.T) {
	got := narrow.Intersect(typ.Any, typ.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(any, string) = %v, want string", got)
	}
}

func TestIntersect_Unknown(t *testing.T) {
	got := narrow.Intersect(typ.String, typ.Unknown)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(string, unknown) = %v, want string", got)
	}
}

func TestIntersect_Disjoint(t *testing.T) {
	got := narrow.Intersect(typ.String, typ.Number)
	if got.Kind() != kind.Intersection {
		t.Errorf("Intersect(string, number) = %v, want intersection type", got)
	}
}

func TestRemoveNil_NestedOptionalInUnion(t *testing.T) {
	inner := typ.NewOptional(typ.Number)
	union := typ.NewUnion(typ.String, inner)
	got := narrow.RemoveNil(union)
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Errorf("RemoveNil(string|number?) = %v, want %v", got, want)
	}
}

func TestExcludeType_Records(t *testing.T) {
	rec1 := typ.NewRecord().Field("channel", typ.String).Field("value", typ.String).Build()
	rec2 := typ.NewRecord().Field("channel", typ.Number).Field("value", typ.Number).Build()
	union := typ.NewUnion(rec1, rec2)

	got := narrow.ExcludeType(union, rec2)
	if !typ.TypeEquals(got, rec1) {
		t.Errorf("ExcludeType(rec1|rec2, rec2) = %v, want %v", got, rec1)
	}
}

func TestRemoveNil_Intersection(t *testing.T) {
	inter := typ.NewIntersection(typ.NewOptional(typ.String), typ.NewOptional(typ.Number))
	got := narrow.RemoveNil(inter)
	if got.Kind() != kind.Intersection {
		t.Errorf("RemoveNil(string? & number?) kind = %v, want Intersection", got.Kind())
	}
}

func TestRemoveNil_IntersectionWithNil(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Nil)
	got := narrow.RemoveNil(inter)
	if got != typ.Never {
		t.Errorf("RemoveNil(string & nil) = %v, want never", got)
	}
}

func TestRemoveFalse_Intersection(t *testing.T) {
	inter := typ.NewIntersection(typ.Boolean, typ.String)
	got := narrow.RemoveFalse(inter)
	if got.Kind() != kind.Intersection {
		t.Errorf("RemoveFalse(boolean & string) kind = %v, want Intersection", got.Kind())
	}
}

func TestRemoveFalse_IntersectionWithFalse(t *testing.T) {
	inter := typ.NewIntersection(typ.LiteralBool(false), typ.String)
	got := narrow.RemoveFalse(inter)
	if got != typ.Never {
		t.Errorf("RemoveFalse(false & string) = %v, want never", got)
	}
}

func TestToFalsy_Intersection(t *testing.T) {
	inter := typ.NewIntersection(typ.NewOptional(typ.String), typ.NewOptional(typ.Number))
	got := narrow.ToFalsy(inter)
	if got != typ.Nil {
		t.Errorf("ToFalsy(string? & number?) = %v, want nil", got)
	}
}

func TestToFalsy_IntersectionAllTruthy(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	got := narrow.ToFalsy(inter)
	if got != typ.Never {
		t.Errorf("ToFalsy(string & number) = %v, want never", got)
	}
}

func TestExcludeType_Intersection(t *testing.T) {
	inter := typ.NewIntersection(typ.NewUnion(typ.String, typ.Number), typ.NewUnion(typ.String, typ.Boolean))
	got := narrow.ExcludeType(inter, typ.Number)
	if got.Kind() != kind.Intersection {
		t.Errorf("ExcludeType(intersection, number) kind = %v, want Intersection", got.Kind())
	}
}

func TestExcludeType_IntersectionToNever(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	got := narrow.ExcludeType(inter, typ.String)
	if got != typ.Never {
		t.Errorf("ExcludeType(string & number, string) = %v, want never", got)
	}
}

func TestExcludeKind_Intersection(t *testing.T) {
	inter := typ.NewIntersection(typ.NewUnion(typ.String, typ.Number), typ.NewUnion(typ.String, typ.Boolean))
	got := narrow.ExcludeKind(inter, kind.Number)
	if got.Kind() != kind.Intersection {
		t.Errorf("ExcludeKind(intersection, number) kind = %v, want Intersection", got.Kind())
	}
}

func TestExcludeKind_IntersectionToNever(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	got := narrow.ExcludeKind(inter, kind.String)
	if got != typ.Never {
		t.Errorf("ExcludeKind(string & number, string) = %v, want never", got)
	}
}

func TestIntersect_WithIntersection(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	got := narrow.Intersect(inter, typ.Boolean)
	if got.Kind() != kind.Intersection {
		t.Errorf("Intersect(string & number, boolean) kind = %v, want Intersection", got.Kind())
	}
	interResult := got.(*typ.Intersection)
	if len(interResult.Members) != 3 {
		t.Errorf("Intersect result has %d members, want 3", len(interResult.Members))
	}
}

func TestIntersect_WithAlias(t *testing.T) {
	alias := typ.NewAlias("MyString", typ.String)
	got := narrow.Intersect(alias, typ.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(alias(string), string) = %v, want string", got)
	}
}

func TestRemoveNil_NestedAlias(t *testing.T) {
	inner := typ.NewAlias("Inner", typ.NewOptional(typ.String))
	outer := typ.NewAlias("Outer", inner)
	got := narrow.RemoveNil(outer)
	if got.Kind() != kind.Alias {
		t.Errorf("RemoveNil(Outer->Inner->string?) kind = %v, want Alias", got.Kind())
	}
}

func TestExcludeType_AliasToIntersection(t *testing.T) {
	inter := typ.NewIntersection(typ.NewUnion(typ.String, typ.Number), typ.Boolean)
	alias := typ.NewAlias("MyType", inter)
	got := narrow.ExcludeType(alias, typ.Number)
	if got.Kind() != kind.Alias {
		t.Errorf("ExcludeType(alias(intersection), number) kind = %v, want Alias", got.Kind())
	}
}

func TestKindMatches_Instantiated_Record(t *testing.T) {
	// Generic<T> = { value: T }
	param := typ.NewTypeParam("T", nil)
	body := typ.NewRecord().Field("value", param).Build()
	generic := typ.NewGeneric("Container", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	// Instantiated type with Record body should match kind.Record
	if !narrow.KindMatches(inst, kind.Record) {
		t.Error("KindMatches(Instantiated<Record>, Record) = false, want true")
	}
}

func TestKindMatches_Instantiated_Interface(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewInterface("Container", []typ.Method{
		{Name: "get", Type: typ.Func().Returns(param).Build()},
	})
	generic := typ.NewGeneric("Container", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	// Instantiated type with Interface body should match kind.Record (table in Lua)
	if !narrow.KindMatches(inst, kind.Record) {
		t.Error("KindMatches(Instantiated<Interface>, Record) = false, want true")
	}
}

func TestKindMatches_Instantiated_NoMatch(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewRecord().Field("value", param).Build()
	generic := typ.NewGeneric("Container", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	// Instantiated Record should NOT match kind.String
	if narrow.KindMatches(inst, kind.String) {
		t.Error("KindMatches(Instantiated<Record>, String) = true, want false")
	}
}

func TestExcludeKind_UnionWithInstantiated(t *testing.T) {
	// Create an instantiated record type
	param := typ.NewTypeParam("T", nil)
	body := typ.NewRecord().Field("value", param).Build()
	generic := typ.NewGeneric("Container", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	// Union of Instantiated<Record> | string
	union := typ.NewUnion(inst, typ.String)

	// Exclude Record should remove the instantiated type
	got := narrow.ExcludeKind(union, kind.Record)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("ExcludeKind(Instantiated<Record>|string, Record) = %v, want string", got)
	}
}

// Interface narrowing tests

func TestTypesOverlap_TwoDistinctInterfaces(t *testing.T) {
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	overlap := narrow.TypesOverlap(eventType, timeType)
	if overlap {
		t.Errorf("TypesOverlap(Event, Time) = true, want false for distinct interfaces")
	}
}

func TestTypesOverlap_SameInterface(t *testing.T) {
	iface := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})

	overlap := narrow.TypesOverlap(iface, iface)
	if !overlap {
		t.Errorf("TypesOverlap(Event, Event) = false, want true for same interface")
	}
}

func TestTypesOverlap_InterfaceAndRecord(t *testing.T) {
	iface := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	rec := typ.NewRecord().Field("x", typ.Number).Build()

	overlap := narrow.TypesOverlap(iface, rec)
	if overlap {
		t.Errorf("TypesOverlap(Interface, Record) = true, want false")
	}
}

func TestTypesOverlap_InterfaceWithDifferentMethods(t *testing.T) {
	// Extended is a subtype of Base (has all methods of Base plus more).
	// Therefore they overlap via structural subtyping.
	base := typ.NewInterface("Base", []typ.Method{
		{Name: "get", Type: typ.Func().Returns(typ.String).Build()},
	})
	extended := typ.NewInterface("Extended", []typ.Method{
		{Name: "get", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "set", Type: typ.Func().Param("v", typ.String).Build()},
	})

	// Extended <: Base (structural subtyping), so they overlap
	overlap := narrow.TypesOverlap(base, extended)
	if !overlap {
		t.Errorf("TypesOverlap(Base, Extended) = false, want true (Extended <: Base)")
	}
}

func TestExcludeType_InterfaceFromUnion(t *testing.T) {
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	union := typ.NewUnion(eventType, timeType)
	result := narrow.ExcludeType(union, timeType)

	if result.Kind() == kind.Never {
		t.Errorf("ExcludeType(Event|Time, Time) = never, want Event interface")
	}
	if !typ.TypeEquals(result, eventType) {
		t.Errorf("ExcludeType(Event|Time, Time) = %v, want Event", result)
	}
}

func TestExcludeType_InterfaceFromThreeInterfaceUnion(t *testing.T) {
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Returns(typ.String).Build()},
	})

	union := typ.NewUnion(eventType, timeType, errorType)
	result := narrow.ExcludeType(union, timeType)

	want := typ.NewUnion(eventType, errorType)
	if !typ.TypeEquals(result, want) {
		t.Errorf("ExcludeType(Event|Time|Error, Time) = %v, want Event|Error", result)
	}
}

func TestExcludeType_InterfaceFromMixedUnion(t *testing.T) {
	iface := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	rec := typ.NewRecord().Field("channel", typ.String).Build()

	union := typ.NewUnion(iface, rec)
	result := narrow.ExcludeType(union, rec)

	if !typ.TypeEquals(result, iface) {
		t.Errorf("ExcludeType(Interface|Record, Record) = %v, want Interface", result)
	}
}

func TestExcludeType_RecordFromMixedUnion(t *testing.T) {
	iface := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})
	rec := typ.NewRecord().Field("channel", typ.String).Build()

	union := typ.NewUnion(iface, rec)
	result := narrow.ExcludeType(union, iface)

	if !typ.TypeEquals(result, rec) {
		t.Errorf("ExcludeType(Interface|Record, Interface) = %v, want Record", result)
	}
}

func TestIntersect_TwoDistinctInterfaces(t *testing.T) {
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	result := narrow.Intersect(eventType, timeType)

	// Two disjoint interfaces should form an intersection type
	if result.Kind() != kind.Intersection {
		t.Errorf("Intersect(Event, Time) kind = %v, want Intersection", result.Kind())
	}
}

func TestExcludeKind_InterfaceFromUnion(t *testing.T) {
	iface := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	union := typ.NewUnion(iface, typ.String, typ.Number)

	result := narrow.ExcludeKind(union, kind.Interface)
	want := typ.NewUnion(typ.String, typ.Number)

	if !typ.TypeEquals(result, want) {
		t.Errorf("ExcludeKind(Interface|string|number, Interface) = %v, want string|number", result)
	}
}

func TestExcludeKind_RecordFromMixedUnion(t *testing.T) {
	// In Lua, interfaces are tables, so excluding kind.Record also excludes interfaces
	iface := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	union := typ.NewUnion(iface, rec, typ.String)

	result := narrow.ExcludeKind(union, kind.Record)

	// Both interface and record are tables in Lua, so only string remains
	if !typ.TypeEquals(result, typ.String) {
		t.Errorf("ExcludeKind(Interface|Record|string, Record) = %v, want string", result)
	}
}

func TestTypesOverlap_UnionContainingInterface(t *testing.T) {
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	union := typ.NewUnion(eventType, timeType)

	// Union overlaps with its members
	if !narrow.TypesOverlap(union, eventType) {
		t.Errorf("TypesOverlap(Event|Time, Event) = false, want true")
	}
	if !narrow.TypesOverlap(union, timeType) {
		t.Errorf("TypesOverlap(Event|Time, Time) = false, want true")
	}

	// Members overlap with union
	if !narrow.TypesOverlap(eventType, union) {
		t.Errorf("TypesOverlap(Event, Event|Time) = false, want true")
	}
}

func TestToTruthy_Interface(t *testing.T) {
	// Interfaces are tables and always truthy in Lua
	iface := typ.NewInterface("Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})

	result := narrow.ToTruthy(iface)
	if !typ.TypeEquals(result, iface) {
		t.Errorf("ToTruthy(Interface) = %v, want Interface unchanged", result)
	}
}

func TestToTruthy_InterfaceUnion(t *testing.T) {
	eventType := typ.NewInterface("Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	// Union of interfaces with nil
	union := typ.NewUnion(eventType, timeType, typ.Nil)
	result := narrow.ToTruthy(union)

	want := typ.NewUnion(eventType, timeType)
	if !typ.TypeEquals(result, want) {
		t.Errorf("ToTruthy(Event|Time|nil) = %v, want Event|Time", result)
	}
}

func TestExcludeType_ChainedExclusion(t *testing.T) {
	// Exclude multiple interfaces one at a time
	eventType := typ.NewInterface("Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	timeType := typ.NewInterface("Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Returns(typ.Integer).Build()},
	})
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Returns(typ.String).Build()},
	})

	union := typ.NewUnion(eventType, timeType, errorType)

	// First exclude Time
	after1 := narrow.ExcludeType(union, timeType)
	want1 := typ.NewUnion(eventType, errorType)
	if !typ.TypeEquals(after1, want1) {
		t.Errorf("ExcludeType(Event|Time|Error, Time) = %v, want Event|Error", after1)
	}

	// Then exclude Error
	after2 := narrow.ExcludeType(after1, errorType)
	if !typ.TypeEquals(after2, eventType) {
		t.Errorf("ExcludeType(Event|Error, Error) = %v, want Event", after2)
	}
}

func TestExcludeType_SameInterface(t *testing.T) {
	iface := typ.NewInterface("Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})

	result := narrow.ExcludeType(iface, iface)
	if result != typ.Never {
		t.Errorf("ExcludeType(Event, Event) = %v, want never", result)
	}
}

// Regression: ExcludeType on a two-member union must return the remaining
// type directly, not wrapped in a single-element Union.
func TestExcludeType_SingleMemberUnionUnwrap(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	got := narrow.ExcludeType(union, typ.String)
	if got.Kind() == kind.Union {
		t.Fatalf("ExcludeType(string|number, string) returned a Union, want unwrapped Number")
	}
	if !typ.TypeEquals(got, typ.Number) {
		t.Errorf("ExcludeType(string|number, string) = %v, want number", got)
	}
}

// Regression: RemoveFalse(nil) must return Never, not Go nil.
func TestRemoveFalse_NilReturnsNever(t *testing.T) {
	got := narrow.RemoveFalse(nil)
	if got != typ.Never {
		t.Errorf("RemoveFalse(nil) = %v, want never", got)
	}
}

// Wrapper combination tests

func TestRemoveNil_Instantiated(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewOptional(param)
	generic := typ.NewGeneric("MaybeT", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	got := narrow.RemoveNil(inst)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("RemoveNil(Instantiated<string?>) = %v, want string", got)
	}
}

func TestRemoveFalse_Instantiated(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewUnion(param, typ.LiteralBool(false))
	generic := typ.NewGeneric("OrFalse", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	got := narrow.RemoveFalse(inst)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("RemoveFalse(Instantiated<string|false>) = %v, want string", got)
	}
}

func TestToTruthy_Instantiated(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewUnion(param, typ.Nil, typ.LiteralBool(false))
	generic := typ.NewGeneric("Falsy", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	got := narrow.ToTruthy(inst)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("ToTruthy(Instantiated<string|nil|false>) = %v, want string", got)
	}
}

func TestToFalsy_Instantiated(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewUnion(param, typ.Nil)
	generic := typ.NewGeneric("OrNil", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	got := narrow.ToFalsy(inst)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Errorf("ToFalsy(Instantiated<string|nil>) = %v, want nil", got)
	}
}

func TestExcludeType_Instantiated(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewUnion(param, typ.Number)
	generic := typ.NewGeneric("OrNumber", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	got := narrow.ExcludeType(inst, typ.Number)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("ExcludeType(Instantiated<string|number>, number) = %v, want string", got)
	}
}

func TestRemoveNil_AliasToOptional(t *testing.T) {
	alias := typ.NewAlias("MaybeString", typ.NewOptional(typ.String))
	got := narrow.RemoveNil(alias)
	if got.Kind() != kind.Alias {
		t.Errorf("RemoveNil(Alias<string?>) kind = %v, want Alias", got.Kind())
	}
	a := got.(*typ.Alias)
	if !typ.TypeEquals(a.Target, typ.String) {
		t.Errorf("RemoveNil(Alias<string?>) target = %v, want string", a.Target)
	}
}

func TestRemoveFalse_AliasToBoolean(t *testing.T) {
	alias := typ.NewAlias("Bool", typ.Boolean)
	got := narrow.RemoveFalse(alias)
	if got.Kind() != kind.Alias {
		t.Errorf("RemoveFalse(Alias<boolean>) kind = %v, want Alias", got.Kind())
	}
	a := got.(*typ.Alias)
	if !typ.TypeEquals(a.Target, typ.True) {
		t.Errorf("RemoveFalse(Alias<boolean>) target = %v, want true", a.Target)
	}
}

func TestIntersect_OptionalTypes(t *testing.T) {
	opt1 := typ.NewOptional(typ.String)
	opt2 := typ.NewOptional(typ.Number)

	got := narrow.Intersect(opt1, opt2)
	if got.Kind() != kind.Intersection {
		t.Errorf("Intersect(string?, number?) kind = %v, want Intersection", got.Kind())
	}
}

func TestIntersect_UnionWithOptional(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	opt := typ.NewOptional(typ.String)

	got := narrow.Intersect(union, opt)
	if got.Kind() == kind.Never {
		t.Errorf("Intersect(string|number, string?) = never, want overlap")
	}
}

func TestExcludeKind_OptionalInnerMatch(t *testing.T) {
	opt := typ.NewOptional(typ.NewUnion(typ.String, typ.Number))
	got := narrow.ExcludeKind(opt, kind.String)
	if got.Kind() != kind.Optional {
		t.Errorf("ExcludeKind(string|number?, string) kind = %v, want Optional", got.Kind())
	}
}

func TestFilterByKind_Union(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	got := narrow.FilterByKind(union, kind.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("FilterByKind(string|number|boolean, string) = %v, want string", got)
	}
}

func TestFilterByKind_UnionMultipleMatches(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Integer, typ.Number)
	got := narrow.FilterByKind(union, kind.Number)
	if !typ.TypeEquals(got, typ.Number) {
		t.Errorf("FilterByKind(string|integer|number, number) = %v, want number", got)
	}
}

func TestFilterByKind_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.FilterByKind(opt, kind.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("FilterByKind(string?, string) = %v, want string", got)
	}
}

func TestFilterByKind_OptionalNil(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.FilterByKind(opt, kind.Nil)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Errorf("FilterByKind(string?, nil) = %v, want nil", got)
	}
}

func TestFilterByKind_NoMatch(t *testing.T) {
	got := narrow.FilterByKind(typ.String, kind.Number)
	if got.Kind() != kind.Never {
		t.Errorf("FilterByKind(string, number) = %v, want never", got)
	}
}

func TestFilterByKind_Any(t *testing.T) {
	got := narrow.FilterByKind(typ.Any, kind.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("FilterByKind(any, string) = %v, want string", got)
	}
}

func TestFilterByKind_Alias(t *testing.T) {
	alias := typ.NewAlias("MyString", typ.String)
	got := narrow.FilterByKind(alias, kind.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("FilterByKind(Alias<string>, string) = %v, want string", got)
	}
}

func TestFilterByKind_Intersection(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.String)
	got := narrow.FilterByKind(inter, kind.String)
	if got.Kind() == kind.Never {
		t.Errorf("FilterByKind(string & string, string) = never, want non-never")
	}
}

func TestFilterByKind_IntersectionNoMatch(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	got := narrow.FilterByKind(inter, kind.Boolean)
	if got.Kind() != kind.Never {
		t.Errorf("FilterByKind(string & number, boolean) = %v, want never", got)
	}
}

func TestTypeForKind_Primitives(t *testing.T) {
	tests := []struct {
		k    kind.Kind
		want typ.Type
	}{
		{kind.Nil, typ.Nil},
		{kind.Boolean, typ.Boolean},
		{kind.Number, typ.Number},
		{kind.Integer, typ.Integer},
		{kind.String, typ.String},
		{kind.Any, typ.Any},
	}
	for _, tt := range tests {
		got := narrow.TypeForKind(tt.k)
		if !typ.TypeEquals(got, tt.want) {
			t.Errorf("TypeForKind(%v) = %v, want %v", tt.k, got, tt.want)
		}
	}
}

func TestTypeForKind_Record(t *testing.T) {
	got := narrow.TypeForKind(kind.Record)
	want := typ.NewInterface("table", nil)
	if !typ.TypeEquals(got, want) {
		t.Errorf("TypeForKind(Record) = %v, want %v", got, want)
	}
}

func TestTypeForKind_Function(t *testing.T) {
	got := narrow.TypeForKind(kind.Function)
	fn, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("TypeForKind(Function) kind = %v, want function", got.Kind())
	}
	if fn.Variadic == nil || !typ.TypeEquals(fn.Variadic, typ.Any) {
		t.Fatalf("TypeForKind(Function) variadic = %v, want any", fn.Variadic)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.Any) {
		t.Fatalf("TypeForKind(Function) returns = %v, want [any]", fn.Returns)
	}
}

func TestRemoveNil_GoNil(t *testing.T) {
	got := narrow.RemoveNil(nil)
	if got != typ.Never {
		t.Errorf("RemoveNil(nil) = %v, want never", got)
	}
}

func TestRemoveNil_UnionWithNilMember(t *testing.T) {
	union := typ.NewUnion(typ.String, nil, typ.Number)
	got := narrow.RemoveNil(union)
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Errorf("RemoveNil(string|nil|number) = %v, want %v", got, want)
	}
}

func TestRemoveFalse_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.Boolean)
	got := narrow.RemoveFalse(opt)
	if got.Kind() != kind.Optional {
		t.Errorf("RemoveFalse(boolean?) kind = %v, want Optional", got.Kind())
	}
}

func TestRemoveFalse_OptionalWithFalseOnly(t *testing.T) {
	opt := typ.NewOptional(typ.LiteralBool(false))
	got := narrow.RemoveFalse(opt)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Errorf("RemoveFalse(false?) = %v, want nil", got)
	}
}

func TestRemoveFalse_UnionAllFalse(t *testing.T) {
	union := typ.NewUnion(typ.LiteralBool(false), typ.LiteralBool(false))
	got := narrow.RemoveFalse(union)
	if got != typ.Never {
		t.Errorf("RemoveFalse(false|false) = %v, want never", got)
	}
}

func TestToTruthy_Nil(t *testing.T) {
	got := narrow.ToTruthy(nil)
	if got != nil {
		t.Errorf("ToTruthy(nil) = %v, want nil", got)
	}
}

func TestToTruthy_OnlyFalse(t *testing.T) {
	got := narrow.ToTruthy(typ.LiteralBool(false))
	if got != typ.Never {
		t.Errorf("ToTruthy(false) = %v, want never", got)
	}
}

func TestToFalsy_Nil(t *testing.T) {
	got := narrow.ToFalsy(nil)
	if got != nil {
		t.Errorf("ToFalsy(nil) = %v, want nil", got)
	}
}

func TestToFalsy_OptionalWithBoolean(t *testing.T) {
	opt := typ.NewOptional(typ.Boolean)
	got := narrow.ToFalsy(opt)
	// ToFalsy on boolean? keeps nil and adds false from boolean
	// Result could be nil | false (union) or false? (optional)
	if got.Kind() != kind.Union && got.Kind() != kind.Optional {
		t.Errorf("ToFalsy(boolean?) kind = %v, want Union or Optional", got.Kind())
	}
}

func TestToFalsy_LiteralTrue(t *testing.T) {
	got := narrow.ToFalsy(typ.LiteralBool(true))
	if got != typ.Never {
		t.Errorf("ToFalsy(true) = %v, want never", got)
	}
}

func TestToFalsy_UnionAllTruthy(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	got := narrow.ToFalsy(union)
	if got != typ.Never {
		t.Errorf("ToFalsy(string|number) = %v, want never", got)
	}
}

func TestExcludeType_Nil(t *testing.T) {
	got := narrow.ExcludeType(nil, typ.String)
	if got != nil {
		t.Errorf("ExcludeType(nil, string) = %v, want nil", got)
	}
}

func TestExcludeType_NilExcluded(t *testing.T) {
	got := narrow.ExcludeType(typ.String, nil)
	if got != typ.String {
		t.Errorf("ExcludeType(string, nil) = %v, want string", got)
	}
}

func TestExcludeType_UnionNoneExcluded(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	got := narrow.ExcludeType(union, typ.Boolean)
	if got != union {
		t.Errorf("ExcludeType(string|number, boolean) = %v, want original union", got)
	}
}

func TestExcludeType_OptionalInnerAllExcluded(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.ExcludeType(opt, typ.String)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Errorf("ExcludeType(string?, string) = %v, want nil", got)
	}
}

func TestExcludeType_OptionalUnchanged(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.ExcludeType(opt, typ.Number)
	if got != opt {
		t.Errorf("ExcludeType(string?, number) = %v, want original optional", got)
	}
}

func TestExcludeKind_Nil(t *testing.T) {
	got := narrow.ExcludeKind(nil, kind.String)
	if got != nil {
		t.Errorf("ExcludeKind(nil, string) = %v, want nil", got)
	}
}

func TestExcludeKind_OptionalInnerFullMatch(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	got := narrow.ExcludeKind(opt, kind.String)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Errorf("ExcludeKind(string?, string) = %v, want nil", got)
	}
}

func TestExcludeKind_UnionSingleRemaining(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	got := narrow.ExcludeKind(union, kind.String)
	if !typ.TypeEquals(got, typ.Number) {
		t.Errorf("ExcludeKind(string|number, string) = %v, want number", got)
	}
}

func TestExcludeKind_Interface(t *testing.T) {
	iface := typ.NewInterface("Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	got := narrow.ExcludeKind(iface, kind.String)
	if got != iface {
		t.Errorf("ExcludeKind(interface, string) = %v, want original interface", got)
	}
}

func TestKindMatches_Nil(t *testing.T) {
	if narrow.KindMatches(nil, kind.String) {
		t.Error("KindMatches(nil, string) = true, want false")
	}
}

func TestKindMatches_InstantiatedNilGeneric(t *testing.T) {
	inst := &typ.Instantiated{Generic: nil}
	if narrow.KindMatches(inst, kind.Record) {
		t.Error("KindMatches(instantiated with nil generic, record) should be false")
	}
}

func TestIntersect_Nil(t *testing.T) {
	got := narrow.Intersect(nil, typ.String)
	if got != nil {
		t.Errorf("Intersect(nil, string) = %v, want nil", got)
	}
	got = narrow.Intersect(typ.String, nil)
	if got != nil {
		t.Errorf("Intersect(string, nil) = %v, want nil", got)
	}
}

func TestIntersect_BothPlaceholder(t *testing.T) {
	got := narrow.Intersect(typ.Any, typ.Unknown)
	if got != typ.Unknown {
		t.Errorf("Intersect(any, unknown) = %v, want unknown", got)
	}
}

func TestIntersect_AliasOnBoth(t *testing.T) {
	aliasA := typ.NewAlias("A", typ.String)
	aliasB := typ.NewAlias("B", typ.String)
	got := narrow.Intersect(aliasA, aliasB)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(Alias<string>, Alias<string>) = %v, want string", got)
	}
}

func TestIntersect_IntersectionOnRight(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	got := narrow.Intersect(typ.Boolean, inter)
	if got.Kind() != kind.Intersection {
		t.Errorf("Intersect(boolean, string&number) kind = %v, want Intersection", got.Kind())
	}
	result := got.(*typ.Intersection)
	if len(result.Members) != 3 {
		t.Errorf("expected 3 members, got %d", len(result.Members))
	}
}

func TestIntersect_InstantiatedOnBoth(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewUnion(param, typ.Number)
	generic := typ.NewGeneric("OrNumber", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	got := narrow.Intersect(inst, typ.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(Instantiated<string|number>, string) = %v, want string", got)
	}
}

func TestIntersect_UnionOnRight(t *testing.T) {
	unionA := typ.NewUnion(typ.String, typ.Number)
	unionB := typ.NewUnion(typ.String, typ.Boolean)
	got := narrow.Intersect(unionA, unionB)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("Intersect(string|number, string|boolean) = %v, want string", got)
	}
}

func TestIntersect_UnionNoOverlap(t *testing.T) {
	unionA := typ.NewUnion(typ.String, typ.Number)
	unionB := typ.NewUnion(typ.Boolean, typ.Nil)
	got := narrow.Intersect(unionA, unionB)
	if got != typ.Never {
		t.Errorf("Intersect(string|number, boolean|nil) = %v, want never", got)
	}
}

func TestFilterUnionByOverlap_NilUnion(t *testing.T) {
	got := narrow.Intersect(typ.String, typ.Number)
	if got.Kind() != kind.Intersection {
		t.Errorf("non-overlapping should create intersection, got %v", got.Kind())
	}
}

func TestFilterByKind_OptionalNoMatch(t *testing.T) {
	opt := typ.NewOptional(typ.Number)
	got := narrow.FilterByKind(opt, kind.String)
	if got != typ.Never {
		t.Errorf("FilterByKind(number?, string) = %v, want never", got)
	}
}

func TestFilterByKind_UnionNoMatch(t *testing.T) {
	union := typ.NewUnion(typ.Number, typ.Boolean)
	got := narrow.FilterByKind(union, kind.String)
	if got != typ.Never {
		t.Errorf("FilterByKind(number|boolean, string) = %v, want never", got)
	}
}

func TestFilterByKind_Unknown(t *testing.T) {
	got := narrow.FilterByKind(typ.Unknown, kind.String)
	if !typ.TypeEquals(got, typ.String) {
		t.Errorf("FilterByKind(unknown, string) = %v, want string", got)
	}
}

func TestKindMatches_MapAsTable(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	if !narrow.KindMatches(m, kind.Record) {
		t.Error("KindMatches(map, record) = false, want true (maps are tables)")
	}
}

func TestKindMatches_ArrayAsTable(t *testing.T) {
	arr := typ.NewArray(typ.String)
	if !narrow.KindMatches(arr, kind.Record) {
		t.Error("KindMatches(array, record) = false, want true (arrays are tables)")
	}
}

func TestKindMatches_TupleAsTable(t *testing.T) {
	tup := typ.NewTuple(typ.String, typ.Number)
	if !narrow.KindMatches(tup, kind.Record) {
		t.Error("KindMatches(tuple, record) = false, want true (tuples are tables)")
	}
}

func TestKindMatches_IntersectionAsTable(t *testing.T) {
	inter := typ.NewIntersection(
		typ.NewRecord().Field("x", typ.Number).Build(),
		typ.NewRecord().Field("y", typ.String).Build(),
	)
	if !narrow.KindMatches(inter, kind.Record) {
		t.Error("KindMatches(intersection, record) = false, want true")
	}
}

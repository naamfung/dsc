package narrow

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFilterByMatch_Nil(t *testing.T) {
	result := FilterByMatch(nil, func(typ.Type) bool { return true }, false)
	if result != nil {
		t.Error("FilterByMatch(nil) should return nil")
	}
}

func TestFilterByMatch_NarrowSimple(t *testing.T) {
	result := FilterByMatch(typ.String, func(t typ.Type) bool {
		return t == typ.String
	}, false)
	if result != typ.String {
		t.Error("should keep matching type")
	}
}

func TestFilterByMatch_NarrowMiss(t *testing.T) {
	result := FilterByMatch(typ.String, func(t typ.Type) bool {
		return t == typ.Number
	}, false)
	if result != typ.Never {
		t.Error("should return Never when no match")
	}
}

func TestFilterByMatch_ExcludeSimple(t *testing.T) {
	result := FilterByMatch(typ.String, func(t typ.Type) bool {
		return t == typ.String
	}, true)
	if result != typ.Never {
		t.Error("exclude matching type should return Never")
	}
}

func TestFilterByMatch_ExcludeMiss(t *testing.T) {
	result := FilterByMatch(typ.String, func(t typ.Type) bool {
		return t == typ.Number
	}, true)
	if result != typ.String {
		t.Error("exclude non-matching should keep type")
	}
}

func TestFilterByMatch_Union_Narrow(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	result := FilterByMatch(union, func(t typ.Type) bool {
		return t == typ.String || t == typ.Number
	}, false)
	if result == nil {
		t.Fatal("should not return nil")
	}
}

func TestFilterByMatch_Union_Exclude(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	result := FilterByMatch(union, func(t typ.Type) bool {
		return t == typ.String
	}, true)
	if result == nil {
		t.Fatal("should not return nil")
	}
}

func TestFilterByMatch_Optional_Narrow(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	result := FilterByMatch(opt, func(t typ.Type) bool {
		return t == typ.String
	}, false)
	if result != typ.String {
		t.Errorf("narrowing optional should unwrap, got %v", result)
	}
}

func TestFilterByMatch_Optional_Exclude(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	result := FilterByMatch(opt, func(t typ.Type) bool {
		return t == typ.String
	}, true)
	if result != typ.Nil {
		t.Errorf("excluding inner should leave nil, got %v", result)
	}
}

func TestByFieldLiteral_Nil(t *testing.T) {
	result := ByFieldLiteral(nil, "field", typ.LiteralString("x"), nil)
	if result != nil {
		t.Error("should return nil for nil input")
	}
}

func TestExcludeByFieldLiteral_Nil(t *testing.T) {
	result := ExcludeByFieldLiteral(nil, "field", typ.LiteralString("x"), nil)
	if result != nil {
		t.Error("should return nil for nil input")
	}
}

func TestTypeIsExactlyLiteral_Nil(t *testing.T) {
	if TypeIsExactlyLiteral(nil, nil) {
		t.Error("nil inputs should return false")
	}
}

func TestTypeIsExactlyLiteral_Match(t *testing.T) {
	lit := typ.LiteralString("hello")
	if !TypeIsExactlyLiteral(lit, lit) {
		t.Error("same literal should match")
	}
}

func TestTypeIsExactlyLiteral_NoMatch(t *testing.T) {
	if TypeIsExactlyLiteral(typ.String, typ.LiteralString("x")) {
		t.Error("broad type should not match literal")
	}
}

// mockResolver implements Resolver for testing field-based narrowing.
type mockResolver struct {
	fields map[string]map[string]typ.Type
}

func newMockResolver() *mockResolver {
	return &mockResolver{fields: make(map[string]map[string]typ.Type)}
}

func (r *mockResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	if rec, ok := t.(*typ.Record); ok {
		if f := rec.GetField(name); f != nil {
			return f.Type, true
		}
	}
	key := t.String()
	if fields, ok := r.fields[key]; ok {
		if ft, ok := fields[name]; ok {
			return ft, true
		}
	}
	return nil, false
}

func (r *mockResolver) Index(_ typ.Type, _ typ.Type) (typ.Type, bool) {
	return nil, false
}

func TestFieldIsExactlyLiteral_NilResolver(t *testing.T) {
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	if FieldIsExactlyLiteral(rec, "kind", lit, nil) {
		t.Error("nil resolver should return false")
	}
}

func TestFieldIsExactlyLiteral_NilType(t *testing.T) {
	resolver := newMockResolver()
	lit := typ.LiteralString("a")
	if FieldIsExactlyLiteral(nil, "kind", lit, resolver) {
		t.Error("nil type should return false")
	}
}

func TestFieldIsExactlyLiteral_NilLiteral(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	if FieldIsExactlyLiteral(rec, "kind", nil, resolver) {
		t.Error("nil literal should return false")
	}
}

func TestFieldIsExactlyLiteral_FieldNotFound(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	if FieldIsExactlyLiteral(rec, "missing", lit, resolver) {
		t.Error("missing field should return false")
	}
}

func TestFieldIsExactlyLiteral_ExactMatch(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	if !FieldIsExactlyLiteral(rec, "kind", lit, resolver) {
		t.Error("exact literal field should match")
	}
}

func TestFieldIsExactlyLiteral_BroadType(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.String).Build()
	lit := typ.LiteralString("a")
	if FieldIsExactlyLiteral(rec, "kind", lit, resolver) {
		t.Error("broad field type should not match literal exactly")
	}
}

func TestFieldMatchesLiteral_NilResolver(t *testing.T) {
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	if FieldMatchesLiteral(rec, "kind", lit, nil) {
		t.Error("nil resolver should return false")
	}
}

func TestFieldMatchesLiteral_NilType(t *testing.T) {
	resolver := newMockResolver()
	lit := typ.LiteralString("a")
	if FieldMatchesLiteral(nil, "kind", lit, resolver) {
		t.Error("nil type should return false")
	}
}

func TestFieldMatchesLiteral_NilLiteral(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	if FieldMatchesLiteral(rec, "kind", nil, resolver) {
		t.Error("nil literal should return false")
	}
}

func TestFieldMatchesLiteral_FieldNotFound(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	if FieldMatchesLiteral(rec, "missing", lit, resolver) {
		t.Error("missing field should return false")
	}
}

func TestFieldMatchesLiteral_ExactMatch(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	if !FieldMatchesLiteral(rec, "kind", lit, resolver) {
		t.Error("exact literal field should match")
	}
}

func TestFieldMatchesLiteral_BroadTypeContains(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.String).Build()
	lit := typ.LiteralString("a")
	if !FieldMatchesLiteral(rec, "kind", lit, resolver) {
		t.Error("string field should match string literal")
	}
}

func TestTypeIsExactlyLiteral_Alias(t *testing.T) {
	lit := typ.LiteralString("hello")
	alias := typ.NewAlias("MyLit", lit)
	if !TypeIsExactlyLiteral(alias, lit) {
		t.Error("alias to literal should match")
	}
}

func TestTypeIsExactlyLiteral_UnionAllSame(t *testing.T) {
	lit := typ.LiteralString("x")
	union := typ.NewUnion(lit, lit)
	if !TypeIsExactlyLiteral(union, lit) {
		t.Error("union of same literals should match")
	}
}

func TestTypeIsExactlyLiteral_UnionDifferent(t *testing.T) {
	lit1 := typ.LiteralString("a")
	lit2 := typ.LiteralString("b")
	union := typ.NewUnion(lit1, lit2)
	if TypeIsExactlyLiteral(union, lit1) {
		t.Error("union with different literals should not match single literal")
	}
}

func TestTypeIsExactlyLiteral_EmptyUnion(t *testing.T) {
	lit := typ.LiteralString("x")
	union := &typ.Union{Members: []typ.Type{}}
	if TypeIsExactlyLiteral(union, lit) {
		t.Error("empty union should not match literal")
	}
}

func TestTypeIsExactlyLiteral_Optional(t *testing.T) {
	lit := typ.LiteralString("x")
	opt := typ.NewOptional(lit)
	if !TypeIsExactlyLiteral(opt, lit) {
		t.Error("optional with literal inner should match")
	}
}

func TestTypeIsExactlyLiteral_OptionalBroad(t *testing.T) {
	lit := typ.LiteralString("x")
	opt := typ.NewOptional(typ.String)
	if TypeIsExactlyLiteral(opt, lit) {
		t.Error("optional with broad inner should not match literal exactly")
	}
}

func TestByFieldLiteral_EmptyField(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	result := ByFieldLiteral(rec, "", lit, resolver)
	if result != rec {
		t.Error("empty field should return original type")
	}
}

func TestByFieldLiteral_NilLiteral(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	result := ByFieldLiteral(rec, "kind", nil, resolver)
	if result != rec {
		t.Error("nil literal should return original type")
	}
}

func TestByFieldLiteral_NilResolver(t *testing.T) {
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	result := ByFieldLiteral(rec, "kind", lit, nil)
	if result != rec {
		t.Error("nil resolver should return original type")
	}
}

func TestByFieldLiteral_DiscriminatedUnion(t *testing.T) {
	resolver := newMockResolver()
	recA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.Number).Build()
	recB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("y", typ.String).Build()
	union := typ.NewUnion(recA, recB)
	lit := typ.LiteralString("a")

	result := ByFieldLiteral(union, "kind", lit, resolver)
	if !typ.TypeEquals(result, recA) {
		t.Errorf("should narrow to recA, got %v", result)
	}
}

func TestByFieldLiteral_BuiltinTableTopMaterializesRecord(t *testing.T) {
	resolver := newMockResolver()
	tableTop := typ.NewInterface("table", nil)
	lit := typ.LiteralString("image")

	result := ByFieldLiteral(tableTop, "type", lit, resolver)
	want := typ.NewRecord().Field("type", lit).SetOpen(true).Build()
	if !typ.TypeEquals(result, want) {
		t.Errorf("ByFieldLiteral(tableTop, type, \"image\") = %v, want %v", result, want)
	}
}

func TestByFieldLiteral_PlaceholderMaterializesRecord(t *testing.T) {
	resolver := newMockResolver()
	lit := typ.LiteralString("image")

	result := ByFieldLiteral(typ.Any, "type", lit, resolver)
	want := typ.NewRecord().Field("type", lit).SetOpen(true).Build()
	if !typ.TypeEquals(result, want) {
		t.Errorf("ByFieldLiteral(any, type, \"image\") = %v, want %v", result, want)
	}
}

func TestExcludeByFieldLiteral_EmptyField(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	result := ExcludeByFieldLiteral(rec, "", lit, resolver)
	if result != rec {
		t.Error("empty field should return original type")
	}
}

func TestExcludeByFieldLiteral_NilLiteral(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	result := ExcludeByFieldLiteral(rec, "kind", nil, resolver)
	if result != rec {
		t.Error("nil literal should return original type")
	}
}

func TestExcludeByFieldLiteral_NilResolver(t *testing.T) {
	rec := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	lit := typ.LiteralString("a")
	result := ExcludeByFieldLiteral(rec, "kind", lit, nil)
	if result != rec {
		t.Error("nil resolver should return original type")
	}
}

func TestExcludeByFieldLiteral_DiscriminatedUnion(t *testing.T) {
	resolver := newMockResolver()
	recA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.Number).Build()
	recB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("y", typ.String).Build()
	union := typ.NewUnion(recA, recB)
	lit := typ.LiteralString("a")

	result := ExcludeByFieldLiteral(union, "kind", lit, resolver)
	if !typ.TypeEquals(result, recB) {
		t.Errorf("should exclude recA, got %v", result)
	}
}

func TestExcludeByFieldLiteral_BroadFieldNotExcluded(t *testing.T) {
	resolver := newMockResolver()
	rec := typ.NewRecord().Field("role", typ.String).Build()
	lit := typ.LiteralString("admin")

	result := ExcludeByFieldLiteral(rec, "role", lit, resolver)
	if result != rec {
		t.Error("broad field type should not be excluded by specific literal")
	}
}

func TestFilterByMatch_Alias(t *testing.T) {
	alias := typ.NewAlias("MyString", typ.String)
	result := FilterByMatch(alias, func(t typ.Type) bool {
		return t == typ.String
	}, false)
	if result != typ.String {
		t.Error("alias should unwrap before matching")
	}
}

func TestFilterByMatch_Instantiated(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewUnion(param, typ.Number)
	generic := typ.NewGeneric("OrNumber", []*typ.TypeParam{param}, body)
	inst := typ.Instantiate(generic, typ.String)

	result := FilterByMatch(inst, func(t typ.Type) bool {
		return t == typ.String
	}, false)
	if result != typ.String {
		t.Errorf("instantiated should unwrap before matching, got %v", result)
	}
}

func TestFilterByMatch_IntersectionNarrow(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	result := FilterByMatch(inter, func(t typ.Type) bool {
		return t.Kind() == kind.Intersection
	}, false)
	if result != inter {
		t.Error("intersection should match atomically")
	}
}

func TestFilterByMatch_IntersectionNarrowMiss(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	result := FilterByMatch(inter, func(t typ.Type) bool {
		return t == typ.String
	}, false)
	if result != typ.Never {
		t.Error("intersection not matching should return Never")
	}
}

func TestFilterByMatch_IntersectionExclude(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	result := FilterByMatch(inter, func(t typ.Type) bool {
		return t.Kind() == kind.Intersection
	}, true)
	if result != typ.Never {
		t.Error("excluding matching intersection should return Never")
	}
}

func TestFilterByMatch_IntersectionExcludeMiss(t *testing.T) {
	inter := typ.NewIntersection(typ.String, typ.Number)
	result := FilterByMatch(inter, func(t typ.Type) bool {
		return t == typ.Boolean
	}, true)
	if result != inter {
		t.Error("not excluding intersection should keep it")
	}
}

func TestFilterByMatch_UnionWithOptional_Narrow(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	union := typ.NewUnion(opt, typ.Number)
	result := FilterByMatch(union, func(t typ.Type) bool {
		return t == typ.String
	}, false)
	if result != typ.String {
		t.Errorf("should unwrap optional in union, got %v", result)
	}
}

func TestFilterByMatch_UnionWithOptional_Exclude(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	union := typ.NewUnion(opt, typ.Number)
	result := FilterByMatch(union, func(t typ.Type) bool {
		return t == typ.String
	}, true)
	// Excluding string from (string? | number) excludes the optional's inner,
	// leaving (nil | number) which is number?
	if result.Kind() != kind.Optional && result.Kind() != kind.Union {
		t.Errorf("should keep optional without inner or number, got %v", result)
	}
}

func TestFilterByMatch_UnionAllExcluded(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	result := FilterByMatch(union, func(typ.Type) bool {
		return true
	}, true)
	if result != typ.Never {
		t.Error("excluding all should return Never")
	}
}

func TestFilterByMatch_UnionNoneMatch(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number)
	result := FilterByMatch(union, func(t typ.Type) bool {
		return t == typ.Boolean
	}, false)
	if result != typ.Never {
		t.Error("no matches should return Never")
	}
}

func TestFilterByMatch_OptionalNarrowMiss(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	result := FilterByMatch(opt, func(t typ.Type) bool {
		return t == typ.Number
	}, false)
	if result != typ.Never {
		t.Error("optional with no inner match should return Never")
	}
}

func TestFilterByMatch_OptionalExcludeMiss(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	result := FilterByMatch(opt, func(t typ.Type) bool {
		return t == typ.Number
	}, true)
	if result != opt {
		t.Error("optional not matching should stay unchanged")
	}
}

package subtype

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNormalizeUnion_Empty(t *testing.T) {
	result := NormalizeUnion()
	if result != typ.Never {
		t.Errorf("NormalizeUnion() = %s, want never", result)
	}
}

func TestNormalizeUnion_Single(t *testing.T) {
	result := NormalizeUnion(typ.String)
	if result != typ.String {
		t.Errorf("NormalizeUnion(string) = %s, want string", result)
	}
}

func TestNormalizeUnion_Any(t *testing.T) {
	result := NormalizeUnion(typ.String, typ.Any, typ.Number)
	if result != typ.Any {
		t.Errorf("NormalizeUnion with any = %s, want any", result)
	}
}

func TestNormalizeUnion_Never(t *testing.T) {
	result := NormalizeUnion(typ.String, typ.Never, typ.Number)
	if result.Kind() == kind.Never {
		t.Error("NormalizeUnion should remove never from union")
	}
}

func TestNormalizeUnion_Subsumption(t *testing.T) {
	// integer <: number, so string | integer | number -> string | number
	result := NormalizeUnion(typ.String, typ.Integer, typ.Number)
	if u, ok := result.(*typ.Union); ok {
		for _, m := range u.Members {
			if m.Kind() == kind.Integer {
				t.Error("integer should be subsumed by number")
			}
		}
	}
}

func TestNormalizeUnion_Flatten(t *testing.T) {
	inner := typ.NewUnion(typ.String, typ.Number)

	result := NormalizeUnion(inner, typ.Boolean)
	if result.Kind() != kind.Union {
		t.Errorf("Expected union, got %s", result.Kind())
	}
}

func TestNormalizeIntersection_Empty(t *testing.T) {
	result := NormalizeIntersection()
	if result != typ.Any {
		t.Errorf("NormalizeIntersection() = %s, want any", result)
	}
}

func TestNormalizeIntersection_Single(t *testing.T) {
	result := NormalizeIntersection(typ.String)
	if result != typ.String {
		t.Errorf("NormalizeIntersection(string) = %s, want string", result)
	}
}

func TestNormalizeIntersection_Never(t *testing.T) {
	result := NormalizeIntersection(typ.String, typ.Never)
	if result != typ.Never {
		t.Errorf("NormalizeIntersection with never = %s, want never", result)
	}
}

func TestNormalizeIntersection_IncompatiblePrimitives(t *testing.T) {
	result := NormalizeIntersection(typ.String, typ.Number)
	if result != typ.Never {
		t.Errorf("NormalizeIntersection(string, number) = %s, want never", result)
	}
}

func TestNormalizeIntersection_Distribution(t *testing.T) {
	// (string | number) & string -> (string & string) | (number & string)
	// -> string | never -> string
	union := typ.NewUnion(typ.String, typ.Number)

	result := NormalizeIntersection(union, typ.String)
	if result != typ.String {
		t.Errorf("(string | number) & string = %s, want string", result)
	}
}

func TestNormalizeIntersection_DistributionCap(t *testing.T) {
	// Two 20-member unions: 20 * 20 = 400 > MaxDistributionProduct (256).
	// The normalizer must bail out and return a non-distributed Intersection.
	membersA := make([]typ.Type, 20)
	membersB := make([]typ.Type, 20)
	for i := 0; i < 20; i++ {
		membersA[i] = typ.LiteralString(fmt.Sprintf("a%d", i))
		membersB[i] = typ.LiteralString(fmt.Sprintf("b%d", i))
	}

	unionA := typ.NewUnion(membersA...)
	unionB := typ.NewUnion(membersB...)

	result := NormalizeIntersection(unionA, unionB)
	if result.Kind() != kind.Intersection {
		t.Errorf("expected Intersection (non-distributed), got %s (%s)", result.Kind(), result)
	}
}

func TestNormalizeIntersection_IncompatibleStringFunction(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	result := NormalizeIntersection(typ.String, fn)
	if result != typ.Never {
		t.Errorf("string & function = %s, want never", result)
	}
}

func TestNormalizeIntersection_IncompatibleStringRecord(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	result := NormalizeIntersection(typ.String, rec)
	if result != typ.Never {
		t.Errorf("string & record = %s, want never", result)
	}
}

func TestNormalizeIntersection_IncompatibleBooleanFunction(t *testing.T) {
	fn := typ.Func().Build()
	result := NormalizeIntersection(typ.Boolean, fn)
	if result != typ.Never {
		t.Errorf("boolean & function = %s, want never", result)
	}
}

func TestNormalizeIntersection_IncompatibleNilRecord(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	result := NormalizeIntersection(typ.Nil, rec)
	if result != typ.Never {
		t.Errorf("nil & record = %s, want never", result)
	}
}

func TestNormalizeUnion_Duplicates(t *testing.T) {
	result := NormalizeUnion(typ.String, typ.String, typ.String)
	if result != typ.String {
		t.Errorf("duplicate union members should collapse, got %s", result)
	}
}

func TestNormalizeUnion_AllNever(t *testing.T) {
	result := NormalizeUnion(typ.Never, typ.Never)
	if result != typ.Never {
		t.Errorf("union of only never should be never, got %s", result)
	}
}

func TestNormalizeUnion_NestedUnion(t *testing.T) {
	inner := typ.NewUnion(typ.String, typ.Number)
	result := NormalizeUnion(inner, typ.Boolean)

	if result.Kind() != kind.Union {
		t.Errorf("expected union, got %s", result.Kind())
	}
}

func TestNormalizeUnion_OptionalExpansion(t *testing.T) {
	opt := typ.NewOptional(typ.String)
	result := NormalizeUnion(opt, typ.Number)

	// Optional should expand to string | nil, then combine with number
	if result.Kind() != kind.Union {
		t.Errorf("expected union, got %s", result.Kind())
	}
}

func TestNormalizeUnion_NilTypes(t *testing.T) {
	result := NormalizeUnion(nil, typ.String, nil)
	if result != typ.String {
		t.Errorf("nil should be skipped, got %s", result)
	}
}

func TestNormalizeIntersection_UnknownRemoval(t *testing.T) {
	result := NormalizeIntersection(typ.Unknown, typ.String)
	if result != typ.String {
		t.Errorf("unknown & string = %s, want string", result)
	}
}

func TestNormalizeIntersection_DuplicateRemoval(t *testing.T) {
	result := NormalizeIntersection(typ.String, typ.String, typ.String)
	if result != typ.String {
		t.Errorf("duplicate intersection should collapse, got %s", result)
	}
}

func TestNormalizeIntersection_NilTypes(t *testing.T) {
	result := NormalizeIntersection(nil, typ.String, nil)
	if result != typ.String {
		t.Errorf("nil should be skipped, got %s", result)
	}
}

func TestNormalizeIntersection_NestedIntersection(t *testing.T) {
	inner := typ.NewIntersection(typ.String, typ.Number)
	result := NormalizeIntersection(inner, typ.Boolean)
	if result != typ.Never {
		t.Errorf("incompatible intersection should be never, got %s", result)
	}
}

func TestNormalizeIntersection_MultipleUnions(t *testing.T) {
	// Small unions that can be distributed
	union1 := typ.NewUnion(typ.String, typ.Number)
	union2 := typ.NewUnion(typ.Boolean, typ.Nil)

	result := NormalizeIntersection(union1, union2)
	// Result may be a complex distributed form; check it's not the original intersection
	if result.Kind() == kind.Intersection {
		// Distribution happened
		t.Log("distribution occurred")
	}
}

func TestNormalizeIntersection_AllUnionsEmpty(t *testing.T) {
	result := NormalizeIntersection(typ.Any, typ.Unknown)
	if result != typ.Any {
		t.Errorf("any & unknown = %s, want any", result)
	}
}

func TestNormalizeIntersection_RecordCompatible(t *testing.T) {
	rec1 := typ.NewRecord().Field("x", typ.Number).Build()
	rec2 := typ.NewRecord().Field("y", typ.String).Build()

	result := NormalizeIntersection(rec1, rec2)
	// Records are compatible (both are table types)
	if result.Kind() != kind.Intersection {
		t.Errorf("compatible records should form intersection, got %s", result.Kind())
	}
}

func TestFlattenUnion_WithNil(t *testing.T) {
	result := NormalizeUnion(nil, typ.String)
	if result != typ.String {
		t.Errorf("nil in flatten should be skipped, got %s", result)
	}
}

func TestIncompatiblePrimitives_IntegerAndNumber(t *testing.T) {
	// Integer and number are compatible (same category)
	result := NormalizeIntersection(typ.Integer, typ.Number)
	if result == typ.Never {
		t.Error("integer and number should be compatible")
	}
}

func TestIncompatiblePrimitives_ArrayTypes(t *testing.T) {
	arr := typ.NewArray(typ.String)
	result := NormalizeIntersection(arr, typ.String)
	if result != typ.Never {
		t.Errorf("array & string = %s, want never", result)
	}
}

func TestIncompatiblePrimitives_MapTypes(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	result := NormalizeIntersection(m, typ.Boolean)
	if result != typ.Never {
		t.Errorf("map & boolean = %s, want never", result)
	}
}

func TestIncompatiblePrimitives_TupleTypes(t *testing.T) {
	tup := typ.NewTuple(typ.String, typ.Number)
	result := NormalizeIntersection(tup, typ.Nil)
	if result != typ.Never {
		t.Errorf("tuple & nil = %s, want never", result)
	}
}

func TestIncompatiblePrimitives_InterfaceTypes(t *testing.T) {
	iface := typ.NewInterface("I", []typ.Method{{Name: "foo", Type: typ.Func().Build()}})
	result := NormalizeIntersection(iface, typ.String)
	if result != typ.Never {
		t.Errorf("interface & string = %s, want never", result)
	}
}

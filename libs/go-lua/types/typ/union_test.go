package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestUnionEmpty(t *testing.T) {
	u := NewUnion()
	if u != Never {
		t.Error("empty union should be Never")
	}
}

func TestUnionSingle(t *testing.T) {
	u := NewUnion(Number)
	if u != Number {
		t.Error("single-member union should unwrap to member")
	}
}

func TestUnionBasic(t *testing.T) {
	u := NewUnion(Number, String)

	if u.Kind() != kind.Union {
		t.Errorf("Kind: got %v, want Union", u.Kind())
	}

	union := u.(*Union)
	if len(union.Members) != 2 {
		t.Errorf("Members: got %d, want 2", len(union.Members))
	}
}

func TestUnionWithNil(t *testing.T) {
	u := NewUnion(Number, Nil)

	if u.Kind() != kind.Optional {
		t.Errorf("number | nil should become number?, got %v", u.Kind())
	}

	opt := u.(*Optional)
	if opt.Inner != Number {
		t.Errorf("Inner: got %v, want number", opt.Inner)
	}
}

func TestUnionWithAny(t *testing.T) {
	u := NewUnion(Number, String, Any)
	if u != Any {
		t.Error("union containing Any should collapse to Any")
	}
}

func TestUnionNeverIdentity(t *testing.T) {
	u := NewUnion(Number, Never, String)

	union := u.(*Union)
	for _, m := range union.Members {
		if m.Kind() == kind.Never {
			t.Error("Never should be filtered from union")
		}
	}
}

func TestUnionFlattening(t *testing.T) {
	inner := NewUnion(Number, String)
	outer := NewUnion(inner, Boolean)

	union := outer.(*Union)
	if len(union.Members) != 3 {
		t.Errorf("nested union should flatten, got %d members", len(union.Members))
	}
}

func TestUnionDeduplication(t *testing.T) {
	u := NewUnion(Number, String, Number)

	union := u.(*Union)
	if len(union.Members) != 2 {
		t.Errorf("duplicate should be removed, got %d members", len(union.Members))
	}
}

func TestUnionDedupHashCollision(t *testing.T) {
	a := &fakeType{id: "a", hash: 99}
	b := &fakeType{id: "b", hash: 99}

	u := NewUnion(a, b).(*Union)
	if len(u.Members) != 2 {
		t.Errorf("hash collision should keep both members, got %d", len(u.Members))
	}
}

func TestUnionEquality(t *testing.T) {
	u1 := NewUnion(Number, String)
	u2 := NewUnion(Number, String)
	u3 := NewUnion(Number, Boolean)

	if !u1.Equals(u2) {
		t.Error("number | string should equal number | string")
	}

	if u1.Equals(u3) {
		t.Error("number | string should not equal number | boolean")
	}

	if u1.Hash() != u2.Hash() {
		t.Error("equal unions should have same hash")
	}
}

func TestUnionOrderIndependence(t *testing.T) {
	u1 := NewUnion(Number, String)
	u2 := NewUnion(String, Number)

	if !u1.Equals(u2) {
		t.Error("union order should not affect equality")
	}

	if u1.Hash() != u2.Hash() {
		t.Error("union order should not affect hash")
	}
}

func TestUnionContains(t *testing.T) {
	u := NewUnion(Number, String, Boolean).(*Union)

	if !u.Contains(Number) {
		t.Error("union should contain Number")
	}

	if !u.Contains(String) {
		t.Error("union should contain String")
	}

	if u.Contains(Integer) {
		t.Error("union should not contain Integer")
	}
}

func TestUnionNotEqualToPrimitive(t *testing.T) {
	u := NewUnion(Number, String)
	if u.Equals(Number) {
		t.Error("union should not equal primitive")
	}
}

func TestUnionString(t *testing.T) {
	u := NewUnion(Number, String).(*Union)

	s := u.String()
	if s == "" {
		t.Error("union String() should not be empty")
	}
}

func TestUnionNestedDedup(t *testing.T) {
	// NewUnion(A, NewUnion(B, A)) should produce {A, B} once
	inner := NewUnion(String, Number)
	outer := NewUnion(Number, inner)

	u, ok := outer.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T", outer)
	}

	if len(u.Members) != 2 {
		t.Errorf("expected 2 members after dedup, got %d", len(u.Members))
	}

	hasNumber := false
	hasString := false
	for _, m := range u.Members {
		if m == Number {
			hasNumber = true
		}
		if m == String {
			hasString = true
		}
	}

	if !hasNumber || !hasString {
		t.Error("union should contain exactly number and string")
	}
}

func TestUnionCanonicalForm(t *testing.T) {
	// Different construction orders should yield equal results
	u1 := NewUnion(Number, NewUnion(String, Boolean))
	u2 := NewUnion(NewUnion(Boolean, Number), String)
	u3 := NewUnion(String, Boolean, Number)

	if !TypeEquals(u1, u2) {
		t.Error("u1 should equal u2")
	}
	if !TypeEquals(u2, u3) {
		t.Error("u2 should equal u3")
	}
	if u1.Hash() != u2.Hash() || u2.Hash() != u3.Hash() {
		t.Error("canonical unions should have same hash")
	}
}

func TestUnionIdempotence(t *testing.T) {
	// union(union(A, B), A) == union(A, B)
	base := NewUnion(Number, String)
	extended := NewUnion(base, Number)

	if !TypeEquals(base, extended) {
		t.Error("adding existing member should not change union")
	}
}

func TestUnionTripleFlatten(t *testing.T) {
	// Deeply nested unions should flatten completely
	inner := NewUnion(Number, String)
	mid := NewUnion(inner, Boolean)
	outer := NewUnion(mid, Integer)

	u, ok := outer.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T", outer)
	}

	if len(u.Members) != 3 {
		t.Errorf("expected 3 members after flattening/subsumption, got %d", len(u.Members))
	}
}

func TestUnionSubsumesStringLiteral(t *testing.T) {
	u := NewUnion(String, LiteralString(""))
	if u != String {
		t.Errorf("string | \"\" should collapse to string, got %v", u)
	}
}

func TestUnionSubsumesNumberLiteral(t *testing.T) {
	u := NewUnion(Number, LiteralNumber(42))
	if u != Number {
		t.Errorf("number | 42 should collapse to number, got %v", u)
	}
}

func TestUnionSubsumesBooleanLiteral(t *testing.T) {
	u := NewUnion(Boolean, True)
	if u != Boolean {
		t.Errorf("boolean | true should collapse to boolean, got %v", u)
	}
}

func TestUnionSubsumesIntegerLiteral(t *testing.T) {
	u := NewUnion(Integer, LiteralInt(7))
	if u != Integer {
		t.Errorf("integer | 7 should collapse to integer, got %v", u)
	}
}

func TestUnionSubsumesIntegerByNumber(t *testing.T) {
	u := NewUnion(Number, Integer)
	if u != Number {
		t.Errorf("number | integer should collapse to number, got %v", u)
	}
}

func TestUnionSubsumesIntegerLiteralByNumber(t *testing.T) {
	u := NewUnion(Number, LiteralInt(7))
	if u != Number {
		t.Errorf("number | 7 should collapse to number, got %v", u)
	}
}

func TestUnionSubsumesMultipleLiterals(t *testing.T) {
	u := NewUnion(String, LiteralString("a"), LiteralString("b"))
	if u != String {
		t.Errorf("string | \"a\" | \"b\" should collapse to string, got %v", u)
	}
}

func TestUnionSubsumesOptionalStringLiteral(t *testing.T) {
	// string? | "" => string? (nil + string, literal "" subsumed)
	u := NewUnion(NewOptional(String), LiteralString(""))
	if u.Kind() != kind.Optional {
		t.Errorf("string? | \"\" should be optional, got %v (%v)", u, u.Kind())
	}
	opt := u.(*Optional)
	if opt.Inner != String {
		t.Errorf("string? | \"\" inner should be string, got %v", opt.Inner)
	}
}

func TestUnionPreservesUnknownAlone(t *testing.T) {
	u := NewUnion(Unknown)
	if u != Unknown {
		t.Errorf("unknown alone should remain unknown, got %v", u)
	}
}

func TestUnionPreservesUnknownWithNil(t *testing.T) {
	u := NewUnion(Unknown, Nil)
	if u.Kind() != kind.Optional {
		t.Errorf("unknown | nil should be optional, got %v (%v)", u, u.Kind())
	}
	opt := u.(*Optional)
	if opt.Inner != Unknown {
		t.Errorf("unknown | nil inner should be unknown, got %v", opt.Inner)
	}
}

func TestUnionDropsUnknownWithConcrete(t *testing.T) {
	u := NewUnion(Unknown, String)
	if u != String {
		t.Errorf("unknown | string should collapse to string, got %v", u)
	}
}

func TestUnionNestedOptionalAndUnionNilDedups(t *testing.T) {
	inner := NewUnion(Nil, Number, String)
	outer := NewUnion(NewOptional(Boolean), inner)

	u, ok := outer.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T (%v)", outer, outer)
	}

	nilCount := 0
	for _, m := range u.Members {
		if m.Kind() == kind.Nil {
			nilCount++
		}
	}

	if nilCount != 1 {
		t.Fatalf("expected exactly one nil in union, got %d in %v", nilCount, u)
	}
}

func TestUnionAnnotatedOptionalMember(t *testing.T) {
	// Annotated wrapping Optional should not panic during union construction
	annotatedOpt := NewAnnotated(NewOptional(String), []Annotation{{Name: "min_len", Arg: int64(1)}})
	u := NewUnion(annotatedOpt, Number)

	if u == nil {
		t.Fatal("union should not be nil")
	}
}

func TestUnionAnnotatedUnionMember(t *testing.T) {
	// Annotated wrapping Union should not panic during union construction
	inner := NewUnion(String, Number)
	annotatedUnion := NewAnnotated(inner, []Annotation{{Name: "max_len", Arg: int64(255)}})
	u := NewUnion(annotatedUnion, Boolean)

	if u == nil {
		t.Fatal("union should not be nil")
	}
}

package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestOptionalBasic(t *testing.T) {
	o := NewOptional(Number)

	if o.Kind() != kind.Optional {
		t.Errorf("Kind: got %v, want Optional", o.Kind())
	}

	opt := o.(*Optional)
	if opt.Inner != Number {
		t.Error("Inner should be Number")
	}

	if o.String() != "number?" {
		t.Errorf("String: got %q, want %q", o.String(), "number?")
	}
}

func TestOptionalOfNil(t *testing.T) {
	o := NewOptional(Nil)
	if o != Nil {
		t.Error("optional of nil should be nil")
	}
}

func TestOptionalOfOptional(t *testing.T) {
	o1 := NewOptional(Number)
	o2 := NewOptional(o1)

	if o1 != o2 {
		t.Error("optional of optional should return same optional")
	}
}

func TestOptionalOfAny(t *testing.T) {
	o := NewOptional(Any)
	if o != Any {
		t.Error("optional of any should be any")
	}
}

func TestOptionalEquality(t *testing.T) {
	o1 := NewOptional(Number)
	o2 := NewOptional(Number)
	o3 := NewOptional(String)

	if !o1.Equals(o2) {
		t.Error("number? should equal number?")
	}

	if o1.Equals(o3) {
		t.Error("number? should not equal string?")
	}

	if o1.Hash() != o2.Hash() {
		t.Error("equal optionals should have same hash")
	}
}

func TestOptionalHashUniqueness(t *testing.T) {
	types := []Type{
		NewOptional(Number),
		NewOptional(String),
		NewOptional(Boolean),
		Number,
		String,
	}

	hashes := make(map[uint64]Type)

	for _, typ := range types {
		h := typ.Hash()
		if existing, ok := hashes[h]; ok {
			t.Errorf("Hash collision: %s and %s", existing.String(), typ.String())
		}

		hashes[h] = typ
	}
}

func TestOptionalNotEqualToPrimitive(t *testing.T) {
	o := NewOptional(Number)
	if o.Equals(Number) {
		t.Error("number? should not equal number")
	}
}

func TestOptionalNotEqualToNil(t *testing.T) {
	o := NewOptional(Number)
	if o.Equals(Nil) {
		t.Error("number? should not equal nil")
	}
}

func TestOptionalOfUnionWithNil(t *testing.T) {
	u := NewUnion(Number, String, Nil)
	o := NewOptional(u)

	// NewOptional normalizes through NewUnion, result should be equivalent
	if !TypeEquals(o, u) {
		t.Errorf("Optional(Union containing Nil) should equal the union, got %v vs %v", o, u)
	}
}

func TestOptionalOfUnionWithoutNil(t *testing.T) {
	u := NewUnion(Number, String)
	o := NewOptional(u)

	// For normalization symmetry, Optional(Union{T1, T2}) becomes Union{Nil, T1, T2}
	if o.Kind() != kind.Union {
		t.Errorf("Optional(Union without Nil) should normalize to Union, got %v", o.Kind())
	}

	union := o.(*Union)
	hasNil := false

	for _, m := range union.Members {
		if m.Kind() == kind.Nil {
			hasNil = true
			break
		}
	}

	if !hasNil {
		t.Error("Normalized union should contain Nil")
	}
}

func TestOptionalAnnotatedUnion(t *testing.T) {
	// NewOptional with Annotated wrapping Union should not panic
	inner := NewUnion(String, Number)
	annotated := NewAnnotated(inner, []Annotation{{Name: "max_len", Arg: int64(255)}})
	o := NewOptional(annotated)

	if o == nil {
		t.Fatal("optional should not be nil")
	}
}

package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestPrimitives(t *testing.T) {
	primitives := []struct {
		typ  Type
		k    kind.Kind
		name string
	}{
		{Nil, kind.Nil, "nil"},
		{Boolean, kind.Boolean, "boolean"},
		{Number, kind.Number, "number"},
		{Integer, kind.Integer, "integer"},
		{String, kind.String, "string"},
		{Any, kind.Any, "any"},
		{Unknown, kind.Unknown, "unknown"},
		{Never, kind.Never, "never"},
		{Self, kind.Self, "self"},
	}

	for _, tc := range primitives {
		t.Run(tc.name, func(t *testing.T) {
			if tc.typ.Kind() != tc.k {
				t.Errorf("Kind: got %v, want %v", tc.typ.Kind(), tc.k)
			}

			if tc.typ.String() != tc.name {
				t.Errorf("String: got %q, want %q", tc.typ.String(), tc.name)
			}

			if tc.typ.Hash() != uint64(tc.k) {
				t.Errorf("Hash: got %d, want %d", tc.typ.Hash(), tc.k)
			}
		})
	}
}

func TestPrimitiveEquality(t *testing.T) {
	if !Nil.Equals(Nil) {
		t.Error("Nil should equal Nil")
	}

	if Nil.Equals(Boolean) {
		t.Error("Nil should not equal Boolean")
	}

	if !Boolean.Equals(Boolean) {
		t.Error("Boolean should equal Boolean")
	}

	if Boolean.Equals(Number) {
		t.Error("Boolean should not equal Number")
	}

	if !Number.Equals(Number) {
		t.Error("Number should equal Number")
	}

	if Number.Equals(Integer) {
		t.Error("Number should not equal Integer")
	}

	if !Integer.Equals(Integer) {
		t.Error("Integer should equal Integer")
	}

	if !String.Equals(String) {
		t.Error("String should equal String")
	}

	if !Any.Equals(Any) {
		t.Error("Any should equal Any")
	}

	if !Unknown.Equals(Unknown) {
		t.Error("Unknown should equal Unknown")
	}

	if !Never.Equals(Never) {
		t.Error("Never should equal Never")
	}

	if !Self.Equals(Self) {
		t.Error("Self should equal Self")
	}
}

func TestPrimitivesAreSingletons(t *testing.T) {
	if Nil != Nil {
		t.Error("Nil should be singleton")
	}

	if Boolean != Boolean {
		t.Error("Boolean should be singleton")
	}

	if Any != Any {
		t.Error("Any should be singleton")
	}
}

func TestPrimitiveHashUniqueness(t *testing.T) {
	primitives := []Type{Nil, Boolean, Number, Integer, String, Any, Unknown, Never, Self}
	hashes := make(map[uint64]Type)

	for _, p := range primitives {
		h := p.Hash()
		if existing, ok := hashes[h]; ok {
			t.Errorf("Hash collision: %s and %s both have hash %d", existing.String(), p.String(), h)
		}

		hashes[h] = p
	}
}

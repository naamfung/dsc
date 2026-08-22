package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
)

func TestTypeKey(t *testing.T) {
	b := narrow.BuiltinTypeKey("string")
	if b.IsZero() {
		t.Fatalf("expected builtin key to be non-zero")
	}

	if b.Kind != narrow.TypeKeyBuiltin || b.Name != "string" {
		t.Fatalf("unexpected builtin key: %#v", b)
	}

	h := narrow.HashTypeKey(123)
	if h.IsZero() {
		t.Fatalf("expected hash key to be non-zero")
	}

	if h.Kind != narrow.TypeKeyHash || h.Hash != 123 {
		t.Fatalf("unexpected hash key: %#v", h)
	}

	if b.Hash64() == 0 || h.Hash64() == 0 {
		t.Fatalf("expected hash64 non-zero")
	}

	if b.Hash64() == h.Hash64() {
		t.Fatalf("expected distinct hashes for builtin vs hash key")
	}

	if !b.Equal(narrow.BuiltinTypeKey("string")) {
		t.Fatalf("expected builtin keys equal")
	}

	if h.Equal(narrow.HashTypeKey(0)) {
		t.Fatalf("zero hash key should not equal")
	}
}

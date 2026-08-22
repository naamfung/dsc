package pathkey

import "testing"

func TestSymbolRoot(t *testing.T) {
	if got := SymbolRoot(42); got != "sym42" {
		t.Fatalf("SymbolRoot(42) = %q, want %q", got, "sym42")
	}
}

func TestSymbolVersionRoot(t *testing.T) {
	if got := SymbolVersionRoot(42, 3); got != "sym42@3" {
		t.Fatalf("SymbolVersionRoot(42, 3) = %q, want %q", got, "sym42@3")
	}
}

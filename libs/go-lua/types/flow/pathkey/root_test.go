package pathkey

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestParseRootAndSuffix_SymbolVersioned(t *testing.T) {
	root, suffix, ok := ParseRootAndSuffix("sym42@3.field[0]")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if root != "sym42@3" {
		t.Fatalf("root=%q, want %q", root, "sym42@3")
	}
	if suffix != ".field[0]" {
		t.Fatalf("suffix=%q, want %q", suffix, ".field[0]")
	}
}

func TestParseRootAndSuffix_SymbolUnversioned(t *testing.T) {
	root, suffix, ok := ParseRootAndSuffix(`sym9["k"]`)
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if root != "sym9" {
		t.Fatalf("root=%q, want %q", root, "sym9")
	}
	if suffix != `["k"]` {
		t.Fatalf("suffix=%q, want %q", suffix, `["k"]`)
	}
}

func TestParseRootAndSuffix_PlaceholderAndReturn(t *testing.T) {
	tests := []struct {
		key    constraint.PathKey
		root   string
		suffix string
	}{
		{key: "$0.meta.ok", root: "$0", suffix: ".meta.ok"},
		{key: "ret[1].ok", root: "ret[1]", suffix: ".ok"},
	}
	for _, tt := range tests {
		root, suffix, ok := ParseRootAndSuffix(tt.key)
		if !ok {
			t.Fatalf("expected parse to succeed for %q", tt.key)
		}
		if root != tt.root {
			t.Fatalf("key %q: root=%q, want %q", tt.key, root, tt.root)
		}
		if suffix != tt.suffix {
			t.Fatalf("key %q: suffix=%q, want %q", tt.key, suffix, tt.suffix)
		}
	}
}

func TestParseRootAndSuffix_InvalidPlaceholderAndReturnRoots(t *testing.T) {
	invalid := []constraint.PathKey{
		"$x.a",
		"ret[x].a",
		"ret[].a",
	}
	for _, key := range invalid {
		if _, _, ok := ParseRootAndSuffix(key); ok {
			t.Fatalf("expected parse to fail for %q", key)
		}
	}
}

func TestParseRootAndSuffix_LegacyFallbackRoot(t *testing.T) {
	root, suffix, ok := ParseRootAndSuffix("#1.field")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if root != "#1" {
		t.Fatalf("root=%q, want %q", root, "#1")
	}
	if suffix != ".field" {
		t.Fatalf("suffix=%q, want %q", suffix, ".field")
	}
}

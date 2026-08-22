package pathkey

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

type mockGraph struct {
	versions map[cfg.Point]map[cfg.SymbolID]cfg.Version
}

func (m *mockGraph) VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	if m.versions == nil {
		return cfg.Version{}
	}
	if pointVersions, ok := m.versions[p]; ok {
		return pointVersions[sym]
	}
	return cfg.Version{}
}

func TestResolver_KeyAt_WithVersion(t *testing.T) {
	g := &mockGraph{
		versions: map[cfg.Point]map[cfg.SymbolID]cfg.Version{
			1: {100: {Root: "x", Symbol: 100, ID: 1}},
		},
	}
	r := NewResolver(g)

	path := constraint.Path{Root: "x", Symbol: 100}
	key := r.KeyAt(1, path)

	if key == "" {
		t.Fatal("expected non-empty key")
	}
	// Verify format: sym<SymbolID>@<VersionID>
	expected := "sym100@1"
	if string(key) != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}

func TestResolver_KeyAt_NoVersion_ReturnsEmpty(t *testing.T) {
	g := &mockGraph{versions: nil}
	r := NewResolver(g)

	path := constraint.Path{Root: "x", Symbol: 100}
	key := r.KeyAt(1, path)

	// No version = empty key (cannot create canonical key without version)
	if key != "" {
		t.Fatalf("expected empty key when no version, got %q", key)
	}
}

func TestResolver_KeyAt_EmptyPath(t *testing.T) {
	r := NewResolver(nil)

	path := constraint.Path{}
	key := r.KeyAt(1, path)

	if key != "" {
		t.Fatalf("expected empty key for empty path, got %q", key)
	}
}

func TestResolver_KeyAt_Placeholder(t *testing.T) {
	r := NewResolver(nil)

	path := constraint.Path{Root: "$0", Symbol: 0}
	key := r.KeyAt(1, path)

	// Placeholder uses Root directly
	if string(key) != "$0" {
		t.Fatalf("expected $0, got %q", key)
	}
}

func TestParseKey_WithVersionAndSuffix(t *testing.T) {
	sym, version, suffix, ok := ParseKey("sym42@3.field[0]")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if sym != 42 {
		t.Fatalf("sym=%d, want 42", sym)
	}
	if version != 3 {
		t.Fatalf("version=%d, want 3", version)
	}
	if suffix != ".field[0]" {
		t.Fatalf("suffix=%q, want .field[0]", suffix)
	}
}

func TestParseKey_Unversioned(t *testing.T) {
	sym, version, suffix, ok := ParseKey("sym9[\"k\"]")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if sym != 9 {
		t.Fatalf("sym=%d, want 9", sym)
	}
	if version != 0 {
		t.Fatalf("version=%d, want 0", version)
	}
	if suffix != "[\"k\"]" {
		t.Fatalf("suffix=%q, want [\"k\"]", suffix)
	}
}

func TestParseKey_InvalidVersionRejected(t *testing.T) {
	invalid := []constraint.PathKey{
		"sym1@.field",
		"sym1@x.field",
		"sym1@-1.field",
		"sym1@",
	}
	for _, key := range invalid {
		if _, _, _, ok := ParseKey(key); ok {
			t.Fatalf("expected ParseKey(%q) to fail", key)
		}
	}
}

func TestParseKey_InvalidSuffixRejected(t *testing.T) {
	invalid := []constraint.PathKey{
		"sym1@1.",
		"sym1@1..a",
		"sym1@1.a-b",
		"sym1@1[\"a\\nb\"]",
	}
	for _, key := range invalid {
		if _, _, _, ok := ParseKey(key); ok {
			t.Fatalf("expected ParseKey(%q) to fail", key)
		}
	}
}

func TestKeySymbolAndKeysShareSymbol(t *testing.T) {
	if got := KeySymbol("sym99@1.foo"); got != 99 {
		t.Fatalf("KeySymbol mismatch: got %d, want 99", got)
	}
	if !KeysShareSymbol("sym5@1.foo", "sym5@2.bar") {
		t.Fatal("expected KeysShareSymbol true for same symbol")
	}
	if KeysShareSymbol("sym5@1.foo", "sym6@1.foo") {
		t.Fatal("expected KeysShareSymbol false for different symbols")
	}
}

func TestParseKeyUnchecked_AllowsUnvalidatedSuffix(t *testing.T) {
	sym, version, suffix, ok := ParseKeyUnchecked("sym7@2.a-b")
	if !ok {
		t.Fatal("expected unchecked parse to succeed")
	}
	if sym != 7 || version != 2 || suffix != ".a-b" {
		t.Fatalf("unexpected unchecked parse result: sym=%d version=%d suffix=%q", sym, version, suffix)
	}

	if _, _, _, ok := ParseKey("sym7@2.a-b"); ok {
		t.Fatal("expected strict ParseKey to reject invalid suffix")
	}
}

func TestKeySymbolUnchecked(t *testing.T) {
	if got := KeySymbolUnchecked("sym123@99..bad"); got != 123 {
		t.Fatalf("KeySymbolUnchecked mismatch: got %d, want 123", got)
	}
	if got := KeySymbolUnchecked("x123@99"); got != 0 {
		t.Fatalf("expected non-symbol key to return 0, got %d", got)
	}
}

package domain

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestSplitPathKey_Simple(t *testing.T) {
	key := constraint.PathKey("sym1.field")
	parent, field, ok := SplitPathKey(key)
	if !ok {
		t.Fatal("expected split to succeed")
	}
	if parent != "sym1" {
		t.Fatalf("expected parent sym1, got %s", parent)
	}
	if field != "field" {
		t.Fatalf("expected field 'field', got %s", field)
	}
}

func TestSplitPathKey_Nested(t *testing.T) {
	key := constraint.PathKey("sym1.a.b")
	parent, field, ok := SplitPathKey(key)
	if !ok {
		t.Fatal("expected split to succeed")
	}
	if parent != "sym1.a" {
		t.Fatalf("expected parent sym1.a, got %s", parent)
	}
	if field != "b" {
		t.Fatalf("expected field 'b', got %s", field)
	}
}

func TestSplitPathKey_NoField(t *testing.T) {
	key := constraint.PathKey("sym1")
	_, _, ok := SplitPathKey(key)
	if ok {
		t.Fatal("expected split to fail for path without field")
	}
}

func TestSplitPathKey_Index(t *testing.T) {
	key := constraint.PathKey("sym1[0]")
	_, _, ok := SplitPathKey(key)
	if ok {
		t.Fatal("expected split to fail for indexed path")
	}
}

func TestSplitPathKey_StringIndexWithDot_NoField(t *testing.T) {
	key := constraint.PathKey(`sym1@2["a.b"]`)
	_, _, ok := SplitPathKey(key)
	if ok {
		t.Fatalf("expected split to fail for non-field terminal segment: %q", key)
	}
}

func TestSplitPathKey_FieldAfterStringIndexWithDot(t *testing.T) {
	key := constraint.PathKey(`sym1@2["a.b"].c`)
	parent, field, ok := SplitPathKey(key)
	if !ok {
		t.Fatalf("expected split to succeed: %q", key)
	}
	if parent != `sym1@2["a.b"]` {
		t.Fatalf("expected parent sym1@2[\"a.b\"], got %s", parent)
	}
	if field != "c" {
		t.Fatalf("expected field c, got %s", field)
	}
}

func TestSplitPathKey_FieldAfterEscapedStringIndex(t *testing.T) {
	key := constraint.PathKey(`sym1@2["a\"b"].c`)
	parent, field, ok := SplitPathKey(key)
	if !ok {
		t.Fatalf("expected split to succeed: %q", key)
	}
	if parent != `sym1@2["a\"b"]` {
		t.Fatalf("expected escaped parent, got %s", parent)
	}
	if field != "c" {
		t.Fatalf("expected field c, got %s", field)
	}
}

func TestSplitPathKey_PlaceholderAndReturnRoots(t *testing.T) {
	tests := []struct {
		key    constraint.PathKey
		parent constraint.PathKey
		field  string
	}{
		{key: `$0.meta.ok`, parent: `$0.meta`, field: "ok"},
		{key: `ret[0].ok`, parent: `ret[0]`, field: "ok"},
	}

	for _, tt := range tests {
		parent, field, ok := SplitPathKey(tt.key)
		if !ok {
			t.Fatalf("expected split to succeed for %q", tt.key)
		}
		if parent != tt.parent {
			t.Fatalf("key %q: parent=%q, want %q", tt.key, parent, tt.parent)
		}
		if field != tt.field {
			t.Fatalf("key %q: field=%q, want %q", tt.key, field, tt.field)
		}
	}
}

func TestIsChildPath(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{"#1", "#1.field", true},
		{"#1", "#1.a.b", true},
		{"#1.a", "#1.a.b", true},
		{"#1", "#1[0]", true},
		{"#1[0]", "#1[0].field", true},
		{`sym1@2["a.b"]`, `sym1@2["a.b"].c`, true},
		{`sym1@2["a\"b"]`, `sym1@2["a\"b"][0]`, true},
		{"#1", "#2.field", false},
		{"#1.a", "#1.b", false},
		{`sym1@2["a.b"]`, `sym1@2["a.c"].d`, false},
		{"#1", "#1", false},
		{"", "#1.field", false},
		{"#1", "", false},
	}

	for _, tt := range tests {
		got := IsChildPath(tt.parent, tt.child)
		if got != tt.want {
			t.Errorf("IsChildPath(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
		}
	}
}

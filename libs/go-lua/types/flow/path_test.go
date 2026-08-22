package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

func Test_parsePath_SimpleIdent(t *testing.T) {
	p, ok := parsePath("x")
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if p.Root != "x" {
		t.Errorf("expected root 'x', got %q", p.Root)
	}

	if len(p.Segments) != 0 {
		t.Errorf("expected no segments, got %d", len(p.Segments))
	}
}

func Test_parsePath_FieldAccess(t *testing.T) {
	p, ok := parsePath("x.foo")
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if p.Root != "x" {
		t.Errorf("expected root 'x', got %q", p.Root)
	}

	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}

	if p.Segments[0].Kind != constraint.SegmentField || p.Segments[0].Name != "foo" {
		t.Errorf("expected field 'foo', got %+v", p.Segments[0])
	}
}

func Test_parsePath_NestedFields(t *testing.T) {
	p, ok := parsePath("x.foo.bar.baz")
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if len(p.Segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(p.Segments))
	}

	names := []string{"foo", "bar", "baz"}
	for i, name := range names {
		if p.Segments[i].Kind != constraint.SegmentField || p.Segments[i].Name != name {
			t.Errorf("segment %d: expected field %q, got %+v", i, name, p.Segments[i])
		}
	}
}

func Test_parsePath_StringIndex(t *testing.T) {
	p, ok := parsePath(`x["key"]`)
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}

	if p.Segments[0].Kind != constraint.SegmentIndexString || p.Segments[0].Name != "key" {
		t.Errorf("expected string index 'key', got %+v", p.Segments[0])
	}
}

func Test_parsePath_IntIndex(t *testing.T) {
	p, ok := parsePath("arr[0]")
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}

	if p.Segments[0].Kind != constraint.SegmentIndexInt || p.Segments[0].Index != 0 {
		t.Errorf("expected int index 0, got %+v", p.Segments[0])
	}

	p2, ok := parsePath("arr[42]")
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if p2.Segments[0].Index != 42 {
		t.Errorf("expected int index 42, got %d", p2.Segments[0].Index)
	}
}

func Test_parsePath_MixedAccess(t *testing.T) {
	p, ok := parsePath(`x.foo["bar"][0].baz`)
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if len(p.Segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(p.Segments))
	}

	if p.Segments[0].Kind != constraint.SegmentField || p.Segments[0].Name != "foo" {
		t.Errorf("seg 0: expected field 'foo'")
	}

	if p.Segments[1].Kind != constraint.SegmentIndexString || p.Segments[1].Name != "bar" {
		t.Errorf("seg 1: expected string index 'bar'")
	}

	if p.Segments[2].Kind != constraint.SegmentIndexInt || p.Segments[2].Index != 0 {
		t.Errorf("seg 2: expected int index 0")
	}

	if p.Segments[3].Kind != constraint.SegmentField || p.Segments[3].Name != "baz" {
		t.Errorf("seg 3: expected field 'baz'")
	}
}

func Test_parsePath_EscapedString(t *testing.T) {
	p, ok := parsePath(`x["a\"b"]`)
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	if p.Segments[0].Name != `a"b` {
		t.Errorf("expected escaped string, got %q", p.Segments[0].Name)
	}
}

func Test_parsePath_Empty(t *testing.T) {
	_, ok := parsePath("")
	if ok {
		t.Error("empty path should fail")
	}
}

func Test_parsePath_InvalidStart(t *testing.T) {
	_, ok := parsePath("123abc")
	if ok {
		t.Error("path starting with digit should fail")
	}
}

func Test_parsePath_UnterminatedBracket(t *testing.T) {
	_, ok := parsePath("x[0")
	if ok {
		t.Error("unterminated bracket should fail")
	}
}

func Test_parsePath_UnterminatedString(t *testing.T) {
	_, ok := parsePath(`x["abc`)
	if ok {
		t.Error("unterminated string should fail")
	}
}

func Test_parsePath_EmptyBracket(t *testing.T) {
	_, ok := parsePath("x[]")
	if ok {
		t.Error("empty bracket should fail")
	}
}

func Test_parsePath_TrailingDot(t *testing.T) {
	_, ok := parsePath("x.")
	if ok {
		t.Error("trailing dot should fail")
	}
}

func Test_parsePath_UnquotedStringIndex(t *testing.T) {
	p, ok := parsePath("x[key]")
	if !ok {
		t.Fatal("unquoted string index should parse as string index")
	}

	if p.Segments[0].Kind != constraint.SegmentIndexString || p.Segments[0].Name != "key" {
		t.Errorf("expected string index 'key', got %+v", p.Segments[0])
	}
}

func TestIsIdentName(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"x", true},
		{"foo", true},
		{"_bar", true},
		{"Foo123", true},
		{"foo_bar", true},
		{"", false},
		{"123", false},
		{"foo-bar", false},
		{"foo.bar", false},
		{"foo bar", false},
	}
	for _, tt := range tests {
		got := pathkey.IsIdentName(tt.input)
		if got != tt.want {
			t.Errorf("IsIdentName(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPath_NonEmpty(t *testing.T) {
	p := constraint.Path{
		Root:     "x",
		Symbol:   5,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}},
	}

	if p.IsEmpty() {
		t.Error("path should not be empty")
	}
	if p.Root != "x" {
		t.Errorf("expected root 'x', got %q", p.Root)
	}
	if len(p.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(p.Segments))
	}
}

func TestPath_Empty(t *testing.T) {
	p := constraint.Path{}
	if !p.IsEmpty() {
		t.Error("empty path should be empty")
	}
}

func TestPath_Key(t *testing.T) {
	p := constraint.Path{
		Root:   "result",
		Symbol: 42,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "value"},
		},
	}

	if p.Root != "result" {
		t.Errorf("expected root 'result', got %q", p.Root)
	}
	if len(p.Segments) != 1 || p.Segments[0].Name != "value" {
		t.Errorf("expected segment .value, got %+v", p.Segments)
	}

	// Paths with Symbol use Symbol-only key (format: sym<SymbolID><segments>)
	expectedKey := "sym42.value"
	if string(p.Key()) != expectedKey {
		t.Errorf("expected key %q, got %q", expectedKey, p.Key())
	}
}

func TestIntToString(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{123456, "123456"},
		{-1, "-1"},
		{-42, "-42"},
	}
	for _, tt := range tests {
		got := pathkey.IntToString(tt.input)
		if got != tt.want {
			t.Errorf("IntToString(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseIntLiteral(t *testing.T) {
	tests := []struct {
		input   string
		wantVal int
		wantOk  bool
	}{
		{"0", 0, true},
		{"1", 1, true},
		{"42", 42, true},
		{"123", 123, true},
		{"", 0, false},
		{"abc", 0, false},
		{"-1", 0, false},
		{"1.5", 0, false},
	}
	for _, tt := range tests {
		got, ok := pathkey.ParseIntLiteral(tt.input)
		if ok != tt.wantOk {
			t.Errorf("ParseIntLiteral(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
		}

		if ok && got != tt.wantVal {
			t.Errorf("ParseIntLiteral(%q) = %d, want %d", tt.input, got, tt.wantVal)
		}
	}
}

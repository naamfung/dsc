package constraint

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
)

func TestPathPlaceholderIndex_OverflowRejected(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflowRoot := "$" + strconv.FormatInt(int64(maxInt), 10) + "0"
	p := Path{Root: overflowRoot}

	if got := p.PlaceholderIndex(); got != -1 {
		t.Fatalf("PlaceholderIndex() = %d, want -1 for overflow input", got)
	}
	if p.IsPlaceholder() {
		t.Fatal("IsPlaceholder() should be false for overflow placeholder root")
	}
}

func TestPathStringKeyHash(t *testing.T) {
	p := Path{
		Root: "x",
		Segments: []Segment{
			{Kind: SegmentField, Name: "y"},
			{Kind: SegmentIndexString, Name: "z"},
			{Kind: SegmentIndexInt, Index: 3},
		},
	}
	display := "x.y[z][3]"
	key := `x.y["z"][3]`

	if p.String() != display {
		t.Fatalf("expected %q, got %q", display, p.String())
	}

	if p.Key() != PathKey(key) {
		t.Fatalf("expected key %q, got %q", key, p.Key())
	}

	p2 := Path{Root: "x", Segments: []Segment{
		{Kind: SegmentField, Name: "y"},
		{Kind: SegmentIndexString, Name: "z"},
		{Kind: SegmentIndexInt, Index: 3},
	}}
	if p.Hash() != p2.Hash() {
		t.Fatalf("expected stable hash, got %d vs %d", p.Hash(), p2.Hash())
	}
}

func TestPathLessAndAppend(t *testing.T) {
	a := Path{Root: "a"}
	b := Path{Root: "b"}

	if !a.Less(b) {
		t.Fatalf("expected a < b")
	}

	seg := Segment{Kind: SegmentField, Name: "field"}

	ap := a.Append(seg)
	if ap.Root != "a" || len(ap.Segments) != 1 {
		t.Fatalf("expected appended path, got %#v", ap)
	}

	if ap.Segments[0] != seg {
		t.Fatalf("expected same segment, got %#v", ap.Segments[0])
	}
}

func TestPathSubstitute(t *testing.T) {
	placeholder := Path{Root: "$0"}
	arg := Path{Root: "vol"}

	result, ok := placeholder.Substitute([]Path{arg})
	if !ok {
		t.Fatal("Substitute failed")
	}
	if result.Root != "vol" {
		t.Fatalf("expected Root='vol', got %q", result.Root)
	}
}

func TestFormatSegments(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentField, Name: "foo"},
		{Kind: SegmentIndexString, Name: "bar"},
		{Kind: SegmentIndexInt, Index: 1},
	}

	expected := `.foo["bar"][1]`
	result := FormatSegments(segs)
	if result != expected {
		t.Fatalf("FormatSegments: expected %q, got %q", expected, result)
	}
}

func TestFormatSegmentsEscapesStringIndex(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentIndexString, Name: `a"b\c`},
	}
	expected := `["a\"b\\c"]`
	if got := FormatSegments(segs); got != expected {
		t.Fatalf("FormatSegments escaped index: expected %q, got %q", expected, got)
	}
}

func TestPathKeyUsesFormatSegments(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentField, Name: "foo"},
		{Kind: SegmentIndexString, Name: "bar"},
		{Kind: SegmentIndexInt, Index: 1},
	}

	p := Path{Symbol: 42, Segments: segs}
	expected := "sym42" + FormatSegments(segs)
	got := string(p.Key())
	if got != expected {
		t.Fatalf("Path.Key() should use FormatSegments: expected %q, got %q", expected, got)
	}
}

func TestPathKeyPlaceholderUsesCanonicalSegments(t *testing.T) {
	pStr := Path{
		Root: "$0",
		Segments: []Segment{
			{Kind: SegmentIndexString, Name: "1"},
		},
	}
	pInt := Path{
		Root: "$0",
		Segments: []Segment{
			{Kind: SegmentIndexInt, Index: 1},
		},
	}

	if got, want := string(pStr.Key()), `$0["1"]`; got != want {
		t.Fatalf("string index placeholder key: got %q, want %q", got, want)
	}
	if got, want := string(pInt.Key()), `$0[1]`; got != want {
		t.Fatalf("int index placeholder key: got %q, want %q", got, want)
	}
	if pStr.Key() == pInt.Key() {
		t.Fatalf("placeholder keys must not collide: %q", pStr.Key())
	}
}

func TestNewPath(t *testing.T) {
	p := NewPath(42, "x")
	if p.Symbol != 42 {
		t.Errorf("expected Symbol=42, got %d", p.Symbol)
	}
	if p.Root != "x" {
		t.Errorf("expected Root='x', got %q", p.Root)
	}
	if len(p.Segments) != 0 {
		t.Errorf("expected no segments, got %d", len(p.Segments))
	}
}

func TestNewPlaceholder(t *testing.T) {
	tests := []struct {
		index    int
		expected string
	}{
		{0, "$0"},
		{1, "$1"},
		{10, "$10"},
	}
	for _, tc := range tests {
		p := NewPlaceholder(tc.index)
		if p.Root != tc.expected {
			t.Errorf("NewPlaceholder(%d): expected Root=%q, got %q", tc.index, tc.expected, p.Root)
		}
		if p.Symbol != 0 {
			t.Errorf("NewPlaceholder(%d): expected Symbol=0, got %d", tc.index, p.Symbol)
		}
		if !p.IsPlaceholder() {
			t.Errorf("NewPlaceholder(%d): expected IsPlaceholder()=true", tc.index)
		}
		if p.PlaceholderIndex() != tc.index {
			t.Errorf("NewPlaceholder(%d): PlaceholderIndex()=%d", tc.index, p.PlaceholderIndex())
		}
	}
}

func TestNewPlaceholder_NegativeIndex(t *testing.T) {
	p := NewPlaceholder(-1)
	if !p.IsEmpty() {
		t.Fatalf("NewPlaceholder(-1) should return empty path, got %+v", p)
	}
	if p.IsPlaceholder() {
		t.Fatal("empty path must not be placeholder")
	}
}

func TestPathFluentBuilders(t *testing.T) {
	p := NewPath(1, "obj").Field("data").IndexStr("key").IndexInt(0)

	if p.String() != "obj.data[key][0]" {
		t.Errorf("expected 'obj.data[key][0]', got %q", p.String())
	}
	if len(p.Segments) != 3 {
		t.Errorf("expected 3 segments, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != SegmentField || p.Segments[0].Name != "data" {
		t.Errorf("segment 0: expected field 'data', got %+v", p.Segments[0])
	}
	if p.Segments[1].Kind != SegmentIndexString || p.Segments[1].Name != "key" {
		t.Errorf("segment 1: expected index string 'key', got %+v", p.Segments[1])
	}
	if p.Segments[2].Kind != SegmentIndexInt || p.Segments[2].Index != 0 {
		t.Errorf("segment 2: expected index int 0, got %+v", p.Segments[2])
	}
}

func TestPathParent(t *testing.T) {
	p := NewPath(1, "x").Field("a").Field("b")
	parent := p.Parent()

	if parent.String() != "x.a" {
		t.Errorf("expected 'x.a', got %q", parent.String())
	}

	grandparent := parent.Parent()
	if grandparent.String() != "x" {
		t.Errorf("expected 'x', got %q", grandparent.String())
	}

	empty := grandparent.Parent()
	if !empty.IsEmpty() {
		t.Errorf("expected empty path, got %q", empty.String())
	}
}

func TestPathLastSegment(t *testing.T) {
	p := NewPath(1, "x").Field("y").IndexInt(5)

	seg, ok := p.LastSegment()
	if !ok {
		t.Fatal("expected LastSegment to return ok=true")
	}
	if seg.Kind != SegmentIndexInt || seg.Index != 5 {
		t.Errorf("expected index int 5, got %+v", seg)
	}

	root := NewPath(1, "x")
	_, ok = root.LastSegment()
	if ok {
		t.Error("expected LastSegment to return ok=false for root path")
	}
}

func TestPathIsFieldAccess(t *testing.T) {
	field := NewPath(1, "x").Field("y")
	if !field.IsFieldAccess() {
		t.Error("expected IsFieldAccess=true for field access")
	}

	index := NewPath(1, "x").IndexInt(0)
	if index.IsFieldAccess() {
		t.Error("expected IsFieldAccess=false for index access")
	}

	root := NewPath(1, "x")
	if root.IsFieldAccess() {
		t.Error("expected IsFieldAccess=false for root path")
	}
}

func TestPathFieldName(t *testing.T) {
	p := NewPath(1, "x").Field("myField")
	if name := p.FieldName(); name != "myField" {
		t.Errorf("expected 'myField', got %q", name)
	}

	index := NewPath(1, "x").IndexInt(0)
	if name := index.FieldName(); name != "" {
		t.Errorf("expected empty string for index access, got %q", name)
	}
}

func TestPathLessComprehensive(t *testing.T) {
	tests := []struct {
		name   string
		a      Path
		b      Path
		expect bool
	}{
		{
			"symbol comparison a < b",
			NewPath(1, "x"),
			NewPath(2, "x"),
			true,
		},
		{
			"symbol comparison a > b",
			NewPath(2, "x"),
			NewPath(1, "x"),
			false,
		},
		{
			"root comparison a < b",
			Path{Root: "a"},
			Path{Root: "b"},
			true,
		},
		{
			"root comparison a > b",
			Path{Root: "b"},
			Path{Root: "a"},
			false,
		},
		{
			"segment length a < b",
			NewPath(1, "x"),
			NewPath(1, "x").Field("y"),
			true,
		},
		{
			"segment length a > b",
			NewPath(1, "x").Field("y"),
			NewPath(1, "x"),
			false,
		},
		{
			"segment kind comparison",
			NewPath(1, "x").Field("y"),
			NewPath(1, "x").IndexInt(0),
			true,
		},
		{
			"segment name comparison a < b",
			NewPath(1, "x").Field("a"),
			NewPath(1, "x").Field("b"),
			true,
		},
		{
			"segment name comparison a > b",
			NewPath(1, "x").Field("b"),
			NewPath(1, "x").Field("a"),
			false,
		},
		{
			"segment index comparison a < b",
			NewPath(1, "x").IndexInt(1),
			NewPath(1, "x").IndexInt(2),
			true,
		},
		{
			"segment index comparison a > b",
			NewPath(1, "x").IndexInt(2),
			NewPath(1, "x").IndexInt(1),
			false,
		},
		{
			"equal paths",
			NewPath(1, "x").Field("y"),
			NewPath(1, "x").Field("y"),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Less(tc.b); got != tc.expect {
				t.Errorf("Less() = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestPathHasSymbol(t *testing.T) {
	withSym := NewPath(1, "x")
	if !withSym.HasSymbol() {
		t.Error("Path with Symbol should return HasSymbol() = true")
	}

	withoutSym := Path{Root: "x"}
	if withoutSym.HasSymbol() {
		t.Error("Path without Symbol should return HasSymbol() = false")
	}
}

func TestPathDisplayRoot(t *testing.T) {
	resolver := func(id cfg.SymbolID) string {
		if id == 1 {
			return "myResolvedVar"
		}
		return ""
	}

	p := NewPath(1, "myVar")
	if got := p.DisplayRoot(resolver); got != "myResolvedVar" {
		t.Errorf("DisplayRoot() = %q, want %q", got, "myResolvedVar")
	}

	sym := NewPath(42, "original")
	if got := sym.DisplayRoot(nil); got != "original" {
		t.Errorf("DisplayRoot(nil) = %q, want %q", got, "original")
	}

	empty := Path{}
	if got := empty.DisplayRoot(nil); got != "" {
		t.Errorf("DisplayRoot() = %q, want empty", got)
	}
}

func TestPathValidateSymbol(t *testing.T) {
	valid := NewPath(1, "x")
	if msg := valid.ValidateSymbol(); msg != "" {
		t.Errorf("ValidateSymbol() returned error for valid path: %v", msg)
	}

	invalid := Path{Root: "x"}
	if msg := invalid.ValidateSymbol(); msg == "" {
		t.Error("ValidateSymbol() should return error for path without symbol")
	}

	empty := Path{}
	if msg := empty.ValidateSymbol(); msg != "" {
		t.Errorf("ValidateSymbol() should return empty for empty path: %v", msg)
	}

	placeholder := NewPlaceholder(0)
	if msg := placeholder.ValidateSymbol(); msg != "" {
		t.Errorf("ValidateSymbol() should return empty for placeholder: %v", msg)
	}
}

func TestPathEqual(t *testing.T) {
	a := NewPath(1, "x").Field("y")
	b := NewPath(1, "x").Field("y")
	c := NewPath(1, "x").Field("z")
	d := NewPath(2, "x").Field("y")
	e := Path{Root: "x"}.Field("y")
	f := Path{Root: "x"}.Field("y")
	g := Path{Root: "x"}.Field("z")

	if !a.Equal(b) {
		t.Error("Equal paths should be equal")
	}
	if a.Equal(c) {
		t.Error("Paths with different fields should not be equal")
	}
	if a.Equal(d) {
		t.Error("Paths with different symbols should not be equal")
	}
	if !e.Equal(f) {
		t.Error("Paths with same root should be equal")
	}
	if e.Equal(g) {
		t.Error("Paths with different fields should not be equal")
	}
	if a.Equal(e) {
		t.Error("Path with symbol should not equal path without symbol")
	}
}

func TestPathIsReturnPath(t *testing.T) {
	returnPath := Path{Root: "ret[0]"}
	if !IsReturnPath(returnPath) {
		t.Error("IsReturnPath should return true for ret[0] path")
	}

	returnPath2 := Path{Root: "ret[42]"}
	if !IsReturnPath(returnPath2) {
		t.Error("IsReturnPath should return true for ret[42] path")
	}

	notReturn := Path{Root: "x"}
	if IsReturnPath(notReturn) {
		t.Error("IsReturnPath should return false for non-return path")
	}

	partial := Path{Root: "ret["}
	if IsReturnPath(partial) {
		t.Error("IsReturnPath should return false for partial ret path")
	}

	invalid := []Path{
		{Root: "ret[-1]"},
		{Root: "ret[]"},
		{Root: "ret[abc]"},
		{Root: "ret[1x]"},
		{Root: "ret[1]]"},
		{Root: "ret[0]", Symbol: 7},
		{Root: "ret[0]", Segments: []Segment{{Kind: SegmentField, Name: "k"}}},
		{Root: "ret[0]", Segments: []Segment{{Kind: SegmentIndexInt, Index: 1}}},
	}
	for _, p := range invalid {
		if IsReturnPath(p) {
			t.Errorf("IsReturnPath should return false for invalid path %q (symbol=%d)", p.Root, p.Symbol)
		}
	}
}

// TestPathParentSliceAliasing tests for slice aliasing bugs in Parent().
// If Parent() shares the backing array with the original path's segments,
// modifications to one could corrupt the other.
func TestPathParentSliceAliasing(t *testing.T) {
	// Create a path with extra capacity in the segments slice
	segments := make([]Segment, 2, 4)
	segments[0] = Segment{Kind: SegmentField, Name: "a"}
	segments[1] = Segment{Kind: SegmentField, Name: "b"}
	original := Path{Root: "x", Symbol: 1, Segments: segments}

	parent := original.Parent()

	// Now append to parent's segments
	// If they share backing array, this would corrupt original
	if cap(parent.Segments) > len(parent.Segments) {
		// There's room in the backing array - this append would corrupt original
		// if slice aliasing exists
		extended := append(parent.Segments, Segment{Kind: SegmentField, Name: "CORRUPTED"})
		_ = extended

		// Check if original is corrupted
		if len(original.Segments) == 2 && original.Segments[1].Name != "b" {
			t.Error("BUG: Parent() slice aliasing corrupted original path")
		}
	}

	// Verify parent has correct content
	if len(parent.Segments) != 1 {
		t.Errorf("Parent should have 1 segment, got %d", len(parent.Segments))
	}
	if parent.Segments[0].Name != "a" {
		t.Errorf("Parent segment should be 'a', got %q", parent.Segments[0].Name)
	}
}

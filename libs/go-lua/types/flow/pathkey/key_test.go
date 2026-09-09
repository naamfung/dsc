package pathkey

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestSegmentsSuffix_Empty(t *testing.T) {
	result := SegmentsSuffix(nil)
	if result != "" {
		t.Fatalf("expected empty, got '%s'", result)
	}
}

func TestSegmentsSuffix_Field(t *testing.T) {
	segs := []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}}
	result := SegmentsSuffix(segs)
	if result != ".foo" {
		t.Fatalf("expected '.foo', got '%s'", result)
	}
}

func TestSegmentsSuffix_StringIndex(t *testing.T) {
	segs := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "key"}}
	result := SegmentsSuffix(segs)
	if result != "[\"key\"]" {
		t.Fatalf("expected '[\"key\"]', got '%s'", result)
	}
}

func TestSegmentsSuffix_IntIndex(t *testing.T) {
	segs := []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 42}}
	result := SegmentsSuffix(segs)
	if result != "[42]" {
		t.Fatalf("expected '[42]', got '%s'", result)
	}
}

func TestSegmentsSuffix_Mixed(t *testing.T) {
	segs := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "a"},
		{Kind: constraint.SegmentIndexInt, Index: 1},
		{Kind: constraint.SegmentField, Name: "b"},
	}
	result := SegmentsSuffix(segs)
	if result != ".a[1].b" {
		t.Fatalf("expected '.a[1].b', got '%s'", result)
	}
}

func TestParseSuffix_Field(t *testing.T) {
	segs := ParseSuffix(".foo")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].Kind != constraint.SegmentField || segs[0].Name != "foo" {
		t.Fatalf("expected field 'foo', got %+v", segs[0])
	}
}

func TestParseSuffix_IntIndex(t *testing.T) {
	segs := ParseSuffix("[42]")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].Kind != constraint.SegmentIndexInt || segs[0].Index != 42 {
		t.Fatalf("expected int index 42, got %+v", segs[0])
	}
}

func TestParseSuffix_StringIndexQuoted(t *testing.T) {
	segs := ParseSuffix("[\"x-y\"]")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].Kind != constraint.SegmentIndexString || segs[0].Name != "x-y" {
		t.Fatalf("expected string index x-y, got %+v", segs[0])
	}
}

func TestParseSuffix_StringIndexQuotedEscaped(t *testing.T) {
	segs := ParseSuffix("[\"a\\\"b\"]")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].Kind != constraint.SegmentIndexString || segs[0].Name != "a\"b" {
		t.Fatalf("expected string index a\\\"b, got %+v", segs[0])
	}
}

func TestParseSuffix_StringIndexQuotedEscapedBackslash(t *testing.T) {
	segs := ParseSuffix("[\"a\\\\b\"]")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].Kind != constraint.SegmentIndexString || segs[0].Name != `a\b` {
		t.Fatalf("expected string index a\\\\b, got %+v", segs[0])
	}
}

func TestParseSuffix_StringIndexQuotedInvalidEscapeRejected(t *testing.T) {
	segs := ParseSuffix("[\"a\\nb\"]")
	if segs != nil {
		t.Fatalf("expected invalid escape to be rejected, got %+v", segs)
	}
}

func TestParseSuffix_StringIndexQuotedDanglingEscapeRejected(t *testing.T) {
	segs := ParseSuffix("[\"a\\\"]")
	if segs != nil {
		t.Fatalf("expected dangling escape to be rejected, got %+v", segs)
	}
}

func TestParseSuffix_StringIndexVsInt_Disambiguated(t *testing.T) {
	stringSegs := ParseSuffix("[\"1\"]")
	if len(stringSegs) != 1 {
		t.Fatalf("expected 1 segment for string index, got %d", len(stringSegs))
	}
	if stringSegs[0].Kind != constraint.SegmentIndexString || stringSegs[0].Name != "1" {
		t.Fatalf("expected string index \"1\", got %+v", stringSegs[0])
	}

	intSegs := ParseSuffix("[1]")
	if len(intSegs) != 1 {
		t.Fatalf("expected 1 segment for int index, got %d", len(intSegs))
	}
	if intSegs[0].Kind != constraint.SegmentIndexInt || intSegs[0].Index != 1 {
		t.Fatalf("expected int index 1, got %+v", intSegs[0])
	}
}

func TestSegmentsSuffix_ParseSuffix_RoundTripEscapedStringIndex(t *testing.T) {
	original := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "meta"},
		{Kind: constraint.SegmentIndexString, Name: `a"b\c`},
	}
	suffix := SegmentsSuffix(original)
	parsed := ParseSuffix(suffix)
	if len(parsed) != len(original) {
		t.Fatalf("round-trip length mismatch: got %d, want %d (suffix=%q)", len(parsed), len(original), suffix)
	}
	for i := range original {
		if parsed[i] != original[i] {
			t.Fatalf("round-trip mismatch at %d: got %+v, want %+v (suffix=%q)", i, parsed[i], original[i], suffix)
		}
	}
}

func TestParseSuffix_Mixed(t *testing.T) {
	segs := ParseSuffix(".a[1].b")
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segs))
	}
	if segs[0].Kind != constraint.SegmentField || segs[0].Name != "a" {
		t.Fatalf("expected field 'a', got %+v", segs[0])
	}
	if segs[1].Kind != constraint.SegmentIndexInt || segs[1].Index != 1 {
		t.Fatalf("expected int index 1, got %+v", segs[1])
	}
	if segs[2].Kind != constraint.SegmentField || segs[2].Name != "b" {
		t.Fatalf("expected field 'b', got %+v", segs[2])
	}
}

func TestParseSuffix_RejectsEmptyFieldSegments(t *testing.T) {
	cases := []string{
		".",
		"..a",
		".a..b",
		".a.",
	}

	for _, suffix := range cases {
		if segs := ParseSuffix(suffix); segs != nil {
			t.Fatalf("expected %q to be rejected, got %+v", suffix, segs)
		}
	}
}

func TestParseSuffix_RejectsInvalidFieldIdentifiers(t *testing.T) {
	cases := []string{
		".1abc",
		".a-b",
		".a b",
	}

	for _, suffix := range cases {
		if segs := ParseSuffix(suffix); segs != nil {
			t.Fatalf("expected %q to be rejected, got %+v", suffix, segs)
		}
	}
}

func TestSegmentsPrefix_Empty(t *testing.T) {
	if !SegmentsPrefix(nil, nil) {
		t.Error("nil is prefix of nil")
	}
	if !SegmentsPrefix(nil, []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}}) {
		t.Error("nil is prefix of any")
	}
}

func TestSegmentsPrefix_Equal(t *testing.T) {
	a := []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}}
	b := []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}}
	if !SegmentsPrefix(a, b) {
		t.Error("equal segments should be prefix")
	}
}

func TestSegmentsPrefix_TruePrefix(t *testing.T) {
	a := []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}}
	b := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "x"},
		{Kind: constraint.SegmentField, Name: "y"},
	}
	if !SegmentsPrefix(a, b) {
		t.Error("a should be prefix of b")
	}
}

func TestSegmentsPrefix_Longer(t *testing.T) {
	a := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "x"},
		{Kind: constraint.SegmentField, Name: "y"},
	}
	b := []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}}
	if SegmentsPrefix(a, b) {
		t.Error("longer should not be prefix of shorter")
	}
}

func TestSegmentsPrefix_Different(t *testing.T) {
	a := []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}}
	b := []constraint.Segment{{Kind: constraint.SegmentField, Name: "y"}}
	if SegmentsPrefix(a, b) {
		t.Error("different segments should not be prefix")
	}
}

func TestPathRelated_SameSymbol(t *testing.T) {
	a := constraint.Path{Symbol: 100}
	b := constraint.Path{Symbol: 100}
	if !PathRelated(a, b) {
		t.Error("same symbol paths should be related")
	}
}

func TestPathRelated_DifferentVersion(t *testing.T) {
	a := constraint.Path{Symbol: 100, Version: 1}
	b := constraint.Path{Symbol: 100, Version: 2}
	if !PathRelated(a, b) {
		t.Error("different version paths should still be related by symbol")
	}
}

func TestPathRelated_VersionWildcard(t *testing.T) {
	a := constraint.Path{Symbol: 100, Version: 1}
	b := constraint.Path{Symbol: 100}
	if !PathRelated(a, b) {
		t.Error("versioned and unversioned paths should be related")
	}
}

func TestPathRelated_DifferentSymbol(t *testing.T) {
	a := constraint.Path{Symbol: 100}
	b := constraint.Path{Symbol: 200}
	if PathRelated(a, b) {
		t.Error("different symbol paths should not be related")
	}
}

func TestPathRelated_SameRoot(t *testing.T) {
	a := constraint.Path{Root: "x"}
	b := constraint.Path{Root: "x"}
	if !PathRelated(a, b) {
		t.Error("same root paths should be related")
	}
}

func TestPathRelated_DifferentRoot(t *testing.T) {
	a := constraint.Path{Root: "x"}
	b := constraint.Path{Root: "y"}
	if PathRelated(a, b) {
		t.Error("different root paths should not be related")
	}
}

func TestPathRelated_ParentChild(t *testing.T) {
	a := constraint.Path{Root: "x", Symbol: 100}
	b := constraint.Path{
		Root:     "x",
		Symbol:   100,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "y"}},
	}
	if !PathRelated(a, b) {
		t.Error("parent should be related to child")
	}
	if !PathRelated(b, a) {
		t.Error("child should be related to parent")
	}
}

func TestPathRelated_Siblings(t *testing.T) {
	a := constraint.Path{
		Root:     "x",
		Symbol:   100,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}},
	}
	b := constraint.Path{
		Root:     "x",
		Symbol:   100,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
	}
	if PathRelated(a, b) {
		t.Error("sibling paths should not be related")
	}
}

func TestPathRelated_Empty(t *testing.T) {
	a := constraint.Path{}
	b := constraint.Path{Root: "x"}
	if PathRelated(a, b) {
		t.Error("empty path should not be related")
	}
}

func TestFilterConstraintsForPath_Empty(t *testing.T) {
	result := FilterConstraintsForPath(nil, constraint.Path{Root: "x"})
	if len(result) != 0 {
		t.Error("expected empty result for nil input")
	}
}

func TestFilterConstraintsForPath_EmptyTarget(t *testing.T) {
	c := constraint.NotNil{Path: constraint.Path{Root: "x", Symbol: 100}}
	result := FilterConstraintsForPath([]constraint.Constraint{c}, constraint.Path{})
	if len(result) != 1 {
		t.Error("empty target should return all constraints")
	}
}

func TestFilterConstraintsForPath_Matching(t *testing.T) {
	target := constraint.Path{Root: "x", Symbol: 100}
	c1 := constraint.NotNil{Path: target}
	c2 := constraint.NotNil{Path: constraint.Path{Root: "y", Symbol: 200}}
	result := FilterConstraintsForPath([]constraint.Constraint{c1, c2}, target)
	if len(result) != 1 {
		t.Errorf("expected 1 matching constraint, got %d", len(result))
	}
}

func TestCollectEquivalentPaths_NoEqualities(t *testing.T) {
	target := constraint.Path{Root: "x", Symbol: 100}
	c := constraint.NotNil{Path: target}
	result := CollectEquivalentPaths([]constraint.Constraint{c}, target)
	if len(result) != 1 {
		t.Errorf("expected 1 path (target only), got %d", len(result))
	}
}

func TestCollectEquivalentPaths_WithEqPath(t *testing.T) {
	x := constraint.Path{Root: "x", Symbol: 100}
	y := constraint.Path{Root: "y", Symbol: 200}
	eq := constraint.NewEqPath(x, y)
	result := CollectEquivalentPaths([]constraint.Constraint{eq}, x)
	if len(result) != 2 {
		t.Errorf("expected 2 equivalent paths, got %d", len(result))
	}
	if !result[x.Key()] || !result[y.Key()] {
		t.Error("both x and y should be equivalent")
	}
}

func TestCollectEquivalentPaths_FieldEqualsPath(t *testing.T) {
	res := constraint.Path{Root: "result", Symbol: 100}
	ch := constraint.Path{Root: "ch", Symbol: 200}
	fieldPath := res.Append(constraint.Segment{Kind: constraint.SegmentField, Name: "channel"})
	eq := constraint.FieldEqualsPath{Target: res, Field: "channel", Value: ch}

	result := CollectEquivalentPaths([]constraint.Constraint{eq}, ch)
	if !result[ch.Key()] || !result[fieldPath.Key()] {
		t.Errorf("expected channel and result.channel to be equivalent")
	}
	if result[res.Key()] {
		t.Errorf("did not expect result root to be equivalent to channel")
	}
}

func TestFilterConstraintsForPath_FieldNotEqualsValueSideDropped(t *testing.T) {
	res := constraint.Path{Root: "result", Symbol: 100}
	ch := constraint.Path{Root: "ch", Symbol: 200}
	c := constraint.FieldNotEqualsPath{Target: res, Field: "channel", Value: ch}

	filtered := FilterConstraintsForPath([]constraint.Constraint{c}, ch)
	if len(filtered) != 0 {
		t.Fatalf("expected FieldNotEqualsPath to be dropped when filtering for value side, got %d", len(filtered))
	}
}

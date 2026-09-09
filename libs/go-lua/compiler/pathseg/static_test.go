package pathseg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestStaticTableFieldKeySegment_Identifier(t *testing.T) {
	seg, ok := StaticTableFieldKeySegment(&ast.IdentExpr{Value: "name"})
	if !ok {
		t.Fatal("expected static segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentField, Name: "name"}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticAttrKeySegment_StringNonIdentifier(t *testing.T) {
	seg, ok := StaticAttrKeySegment(&ast.StringExpr{Value: "x-y"})
	if !ok {
		t.Fatal("expected static segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticAttrKeySegment_Number(t *testing.T) {
	seg, ok := StaticAttrKeySegment(&ast.NumberExpr{Value: "7"})
	if !ok {
		t.Fatal("expected static segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 7}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticAttrKeySegment_RejectsDynamicIdent(t *testing.T) {
	if _, ok := StaticAttrKeySegment(&ast.IdentExpr{Value: "k"}); ok {
		t.Fatal("expected dynamic identifier key to be rejected for attr access")
	}
}

func TestStaticAttrKeySegment_InvalidNumber(t *testing.T) {
	if _, ok := StaticAttrKeySegment(&ast.NumberExpr{Value: "1.5"}); ok {
		t.Fatal("expected invalid numeric index to be rejected")
	}
}

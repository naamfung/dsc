package pathseg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

func segmentFromStringOrNumber(key ast.Expr) (constraint.Segment, bool) {
	switch k := key.(type) {
	case *ast.StringExpr:
		if k.Value == "" {
			return constraint.Segment{}, false
		}
		if pathkey.IsIdentName(k.Value) {
			return constraint.Segment{Kind: constraint.SegmentField, Name: k.Value}, true
		}
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: k.Value}, true
	case *ast.NumberExpr:
		if idx, ok := pathkey.ParseIntLiteral(k.Value); ok {
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
		}
		return constraint.Segment{}, false
	default:
		return constraint.Segment{}, false
	}
}

// StaticAttrKeySegment converts a static AttrGet key expression into a path segment.
//
// AttrGet semantics:
//   - static: obj["k"], obj[1], obj.k (parser string key)
//   - dynamic: obj[k] (parser identifier key) -> rejected
func StaticAttrKeySegment(key ast.Expr) (constraint.Segment, bool) {
	return segmentFromStringOrNumber(key)
}

// StaticTableFieldKeySegment converts a static table field key expression into a path segment.
//
// Table field semantics:
//   - identifier field: {name = v}
//   - string field: {["name"] = v}
//   - numeric field: {[1] = v}
func StaticTableFieldKeySegment(key ast.Expr) (constraint.Segment, bool) {
	if ident, ok := key.(*ast.IdentExpr); ok {
		if ident.Value == "" {
			return constraint.Segment{}, false
		}
		return constraint.Segment{Kind: constraint.SegmentField, Name: ident.Value}, true
	}
	return segmentFromStringOrNumber(key)
}

// Path extraction from AST expressions.
//
// IDENTITY MODEL:
// FromExprWithBindings resolves symbols from AST nodes via bindings, not by name lookup.
// This ensures paths have stable symbol identity across function boundaries.
//
// For constraint extraction, always use FromExprWithBindings.
package path

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

type versionedGraph interface {
	VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version
}

// StaticKeySegment converts a syntactically static key expression into a path segment.
//
// Supported static keys:
//   - identifier key: foo        -> SegmentField("foo")
//   - string key: "foo"          -> SegmentField("foo")
//   - string key: "x-y"          -> SegmentIndexString("x-y")
//   - number key: 1              -> SegmentIndexInt(1)
//
// Returns false for unsupported or empty keys.
func StaticKeySegment(key ast.Expr) (constraint.Segment, bool) {
	return pathseg.StaticTableFieldKeySegment(key)
}

// WithVersion binds a path to the SSA version visible at point p.
// If the path is unversioned or the version is unavailable, it is returned unchanged.
func WithVersion(path constraint.Path, graph versionedGraph, p cfg.Point) constraint.Path {
	if path.IsEmpty() || path.Symbol == 0 || graph == nil {
		return path
	}
	ver := graph.VisibleVersion(p, path.Symbol)
	if ver.IsZero() {
		return path
	}
	path.Version = ver.ID
	return path
}

// FromExprWithBindings extracts a flow path using bindings for symbol resolution.
// Resolves symbols from AST nodes directly.
func FromExprWithBindings(expr ast.Expr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable) constraint.Path {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		var sym cfg.SymbolID
		if bindings != nil {
			sym, _ = bindings.SymbolOf(e)
		}
		if sym != 0 {
			root := e.Value
			if bindings != nil {
				if name := bindings.Name(sym); name != "" {
					root = name
				}
			}
			return constraint.Path{Root: root, Symbol: sym}
		}
		return constraint.Path{Root: e.Value}
	case *ast.AttrGetExpr:
		base := FromExprWithBindings(e.Object, constResolver, bindings)
		if base.IsEmpty() {
			return constraint.Path{}
		}
		switch key := e.Key.(type) {
		case *ast.StringExpr:
			seg, ok := StaticKeySegment(key)
			if !ok {
				return constraint.Path{}
			}
			return base.Append(seg)
		case *ast.NumberExpr:
			if idx, ok := pathkey.ParseIntLiteral(key.Value); ok {
				return base.Append(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx})
			}
		case *ast.IdentExpr:
			if constResolver == nil {
				return constraint.Path{}
			}
			if val := constResolver(key.Value); val != nil {
				switch val.Kind {
				case flow.ConstString:
					if seg, ok := StaticKeySegment(&ast.StringExpr{Value: val.Str}); ok {
						return base.Append(seg)
					}
					return constraint.Path{}
				case flow.ConstInt:
					return base.Append(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(val.Int)})
				case flow.ConstFloat:
					if idx, ok := pathkey.FloatToSafeInt(val.Float); ok {
						return base.Append(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx})
					}
					return constraint.Path{}
				case flow.ConstBool, flow.ConstNil, flow.ConstUnknown:
					return constraint.Path{}
				}
			}
			return constraint.Path{}
		}
	}
	return constraint.Path{}
}

// FromExprWithBindingsAt extracts a flow path using bindings and binds it to the SSA version at point p.
func FromExprWithBindingsAt(expr ast.Expr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable, graph versionedGraph, p cfg.Point) constraint.Path {
	path := FromExprWithBindings(expr, constResolver, bindings)
	return WithVersion(path, graph, p)
}

// SplitIndexPath splits a path into base and index key.
func SplitIndexPath(path constraint.Path) (constraint.Path, typ.Type, bool) {
	if path.IsEmpty() || len(path.Segments) == 0 {
		return constraint.Path{}, nil, false
	}
	last := path.Segments[len(path.Segments)-1]
	var key typ.Type
	switch last.Kind {
	case constraint.SegmentIndexString:
		key = typ.LiteralString(last.Name)
	case constraint.SegmentIndexInt:
		key = typ.LiteralInt(int64(last.Index))
	default:
		return constraint.Path{}, nil, false
	}
	base := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	if len(path.Segments) > 1 {
		base.Segments = append(base.Segments, path.Segments[:len(path.Segments)-1]...)
	}
	return base, key, true
}

// TypeOfCallPathWithBindings extracts the path argument from a type() call using bindings.
func TypeOfCallPathWithBindings(expr ast.Expr, bindings *bind.BindingTable) (constraint.Path, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil {
		return constraint.Path{}, false
	}
	if callsite.IsMethodLikeExpr(call) {
		return constraint.Path{}, false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident.Value != "type" {
		return constraint.Path{}, false
	}
	if len(call.Args) != 1 {
		return constraint.Path{}, false
	}
	path := FromExprWithBindings(call.Args[0], nil, bindings)
	if path.IsEmpty() {
		return constraint.Path{}, false
	}
	return path, true
}

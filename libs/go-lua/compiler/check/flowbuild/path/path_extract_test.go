package path_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestPathFromExpr_Ident(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.Root != "x" {
		t.Errorf("expected root 'x', got '%s'", p.Root)
	}
	if p.Symbol != 0 {
		t.Errorf("expected symbol 0, got %d", p.Symbol)
	}
}

func TestPathFromExpr_NestedField(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "field"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.Root != "obj" {
		t.Errorf("expected root 'obj', got '%s'", p.Root)
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentField {
		t.Errorf("expected SegmentField, got %v", p.Segments[0].Kind)
	}
	if p.Segments[0].Name != "field" {
		t.Errorf("expected name 'field', got '%s'", p.Segments[0].Name)
	}
}

func TestPathFromExpr_NumberIndex(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "arr"},
		Key:    &ast.NumberExpr{Value: "1"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.Root != "arr" {
		t.Errorf("expected root 'arr', got '%s'", p.Root)
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentIndexInt {
		t.Errorf("expected SegmentIndexInt, got %v", p.Segments[0].Kind)
	}
	if p.Segments[0].Index != 1 {
		t.Errorf("expected index 1, got %d", p.Segments[0].Index)
	}
}

func TestPathFromExpr_NilExpr(t *testing.T) {
	p := path.FromExprWithBindings(nil, nil, nil)
	if !p.IsEmpty() {
		t.Error("expected empty path for nil expr")
	}
}

func TestPathFromExpr_UnsupportedExpr(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	p := path.FromExprWithBindings(expr, nil, nil)
	if !p.IsEmpty() {
		t.Error("expected empty path for unsupported expr")
	}
}

func TestPathFromExprWithConst_StringConst(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "key" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "myfield"}
		}
		return nil
	}

	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "key"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	if p.Root != "obj" {
		t.Errorf("expected root 'obj', got '%s'", p.Root)
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentField {
		t.Errorf("expected SegmentField, got %v", p.Segments[0].Kind)
	}
	if p.Segments[0].Name != "myfield" {
		t.Errorf("expected name 'myfield', got '%s'", p.Segments[0].Name)
	}
}

func TestPathFromExprWithConst_IntConst(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "idx" {
			return &flow.ConstValue{Kind: flow.ConstInt, Int: 5}
		}
		return nil
	}

	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "arr"},
		Key:    &ast.IdentExpr{Value: "idx"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentIndexInt {
		t.Errorf("expected SegmentIndexInt, got %v", p.Segments[0].Kind)
	}
	if p.Segments[0].Index != 5 {
		t.Errorf("expected index 5, got %d", p.Segments[0].Index)
	}
}

func TestPathFromExprWithConst_FloatConst(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "idx" {
			return &flow.ConstValue{Kind: flow.ConstFloat, Float: 3.0}
		}
		return nil
	}

	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "arr"},
		Key:    &ast.IdentExpr{Value: "idx"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentIndexInt {
		t.Errorf("expected SegmentIndexInt, got %v", p.Segments[0].Kind)
	}
	if p.Segments[0].Index != 3 {
		t.Errorf("expected index 3, got %d", p.Segments[0].Index)
	}
}

func TestPathFromExprWithConst_BoolConst(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "flag" {
			return &flow.ConstValue{Kind: flow.ConstBool, Bool: true}
		}
		return nil
	}

	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "flag"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	// Bool keys should result in empty path (invalid for indexing)
	if !p.IsEmpty() {
		t.Error("expected empty path for bool const key")
	}
}

func TestPathFromExprWithConst_NilConst(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "n" {
			return &flow.ConstValue{Kind: flow.ConstNil}
		}
		return nil
	}

	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "n"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	if !p.IsEmpty() {
		t.Error("expected empty path for nil const key")
	}
}

func TestPathFromExprWithConst_NoResolver(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "key"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if !p.IsEmpty() {
		t.Error("expected empty path with no resolver for ident key")
	}
}

func TestFromExprFull_DeepNesting(t *testing.T) {
	// a.b.c
	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "a"},
			Key:    &ast.StringExpr{Value: "b"},
		},
		Key: &ast.StringExpr{Value: "c"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.Root != "a" {
		t.Errorf("expected root 'a', got '%s'", p.Root)
	}
	if len(p.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(p.Segments))
	}
	if p.Segments[0].Name != "b" {
		t.Errorf("expected first segment 'b', got '%s'", p.Segments[0].Name)
	}
	if p.Segments[1].Name != "c" {
		t.Errorf("expected second segment 'c', got '%s'", p.Segments[1].Name)
	}
}

func TestFromExprWithBindings_NilBindings(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.Root != "x" {
		t.Errorf("expected root 'x', got '%s'", p.Root)
	}
	if p.Symbol != 0 {
		t.Errorf("expected symbol 0 with nil bindings, got %d", p.Symbol)
	}
}

func TestFromExprWithBindings_AttrWithStringKey(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "field"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.Root != "obj" {
		t.Errorf("expected root 'obj', got '%s'", p.Root)
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentField {
		t.Errorf("expected SegmentField, got %v", p.Segments[0].Kind)
	}
}

func TestFromExprWithBindings_NonIdentifierKey(t *testing.T) {
	// String key with non-identifier characters
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "my-field"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentIndexString {
		t.Errorf("expected SegmentIndexString for non-ident key, got %v", p.Segments[0].Kind)
	}
}

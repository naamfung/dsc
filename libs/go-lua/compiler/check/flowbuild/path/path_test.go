package path_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPathFromExpr_IdentExpr(t *testing.T) {
	expr := &ast.IdentExpr{Value: "myVar"}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if p.Root != "myVar" {
		t.Errorf("expected root 'myVar', got %q", p.Root)
	}
	if len(p.Segments) != 0 {
		t.Errorf("expected no segments, got %d", len(p.Segments))
	}
}

func TestPathFromExpr_AttrGetExpr_StringKey(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "field"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if p.Root != "obj" {
		t.Errorf("expected root 'obj', got %q", p.Root)
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentField {
		t.Error("expected field segment")
	}
	if p.Segments[0].Name != "field" {
		t.Errorf("expected name 'field', got %q", p.Segments[0].Name)
	}
}

func TestPathFromExpr_AttrGetExpr_NumberKey(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "arr"},
		Key:    &ast.NumberExpr{Value: "5"},
	}
	p := path.FromExprWithBindings(expr, nil, nil)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentIndexInt {
		t.Error("expected int index segment")
	}
	if p.Segments[0].Index != 5 {
		t.Errorf("expected index 5, got %d", p.Segments[0].Index)
	}
}

func TestStaticKeySegment_Ident(t *testing.T) {
	seg, ok := path.StaticKeySegment(&ast.IdentExpr{Value: "name"})
	if !ok {
		t.Fatal("expected static key segment")
	}
	if seg.Kind != constraint.SegmentField || seg.Name != "name" {
		t.Fatalf("unexpected segment: kind=%v name=%q", seg.Kind, seg.Name)
	}
}

func TestStaticKeySegment_StringIdentifier(t *testing.T) {
	seg, ok := path.StaticKeySegment(&ast.StringExpr{Value: "name"})
	if !ok {
		t.Fatal("expected static key segment")
	}
	if seg.Kind != constraint.SegmentField || seg.Name != "name" {
		t.Fatalf("unexpected segment: kind=%v name=%q", seg.Kind, seg.Name)
	}
}

func TestStaticKeySegment_StringNonIdentifier(t *testing.T) {
	seg, ok := path.StaticKeySegment(&ast.StringExpr{Value: "x-y"})
	if !ok {
		t.Fatal("expected static key segment")
	}
	if seg.Kind != constraint.SegmentIndexString || seg.Name != "x-y" {
		t.Fatalf("unexpected segment: kind=%v name=%q", seg.Kind, seg.Name)
	}
}

func TestStaticKeySegment_Number(t *testing.T) {
	seg, ok := path.StaticKeySegment(&ast.NumberExpr{Value: "1"})
	if !ok {
		t.Fatal("expected static key segment")
	}
	if seg.Kind != constraint.SegmentIndexInt || seg.Index != 1 {
		t.Fatalf("unexpected segment: kind=%v index=%d", seg.Kind, seg.Index)
	}
}

func TestPathFromExpr_Unsupported(t *testing.T) {
	p := path.FromExprWithBindings(&ast.StringExpr{Value: "hello"}, nil, nil)
	if !p.IsEmpty() {
		t.Error("expected empty path for string expr")
	}

	p = path.FromExprWithBindings(&ast.FuncCallExpr{}, nil, nil)
	if !p.IsEmpty() {
		t.Error("expected empty path for call expr")
	}
}

func TestPathFromExprWithConst_IntConstKey(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "IDX" {
			return &flow.ConstValue{Kind: flow.ConstInt, Int: 3}
		}
		return nil
	}
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "arr"},
		Key:    &ast.IdentExpr{Value: "IDX"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentIndexInt {
		t.Error("expected int index segment")
	}
	if p.Segments[0].Index != 3 {
		t.Errorf("expected index 3, got %d", p.Segments[0].Index)
	}
}

func TestPathFromExprWithConst_StringConstKey(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "KEY" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "myKey"}
		}
		return nil
	}
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "KEY"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentField {
		t.Error("expected field segment for identifier-like const string key")
	}
	if p.Segments[0].Name != "myKey" {
		t.Errorf("expected name 'myKey', got %q", p.Segments[0].Name)
	}
}

func TestPathFromExprWithConst_StringConstNonIdentifierKey(t *testing.T) {
	constResolver := func(name string) *flow.ConstValue {
		if name == "KEY" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "x-y"}
		}
		return nil
	}
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "KEY"},
	}
	p := path.FromExprWithBindings(expr, constResolver, nil)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if len(p.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != constraint.SegmentIndexString {
		t.Error("expected string index segment for non-identifier const string key")
	}
	if p.Segments[0].Name != "x-y" {
		t.Errorf("expected name 'x-y', got %q", p.Segments[0].Name)
	}
}

func TestFromExprWithBindings_WithSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	retStmt := fn.Stmts[0].(*ast.ReturnStmt)
	ident := retStmt.Exprs[0].(*ast.IdentExpr)

	p := path.FromExprWithBindings(ident, nil, bindings)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if p.Symbol == 0 {
		t.Error("expected path to have symbol")
	}
}

func TestFromExprWithBindings_NilBindingsParam(t *testing.T) {
	ident := &ast.IdentExpr{Value: "x"}
	p := path.FromExprWithBindings(ident, nil, nil)
	if p.IsEmpty() {
		t.Fatal("expected non-empty path")
	}
	if p.Root != "x" {
		t.Errorf("expected root 'x', got %q", p.Root)
	}
	if p.Symbol != 0 {
		t.Errorf("expected symbol 0, got %d", p.Symbol)
	}
}

func TestSplitIndexPath_EmptyPath(t *testing.T) {
	_, _, ok := path.SplitIndexPath(constraint.Path{})
	if ok {
		t.Error("expected ok=false for empty path")
	}
}

func TestSplitIndexPath_WithoutSegments(t *testing.T) {
	p := constraint.Path{Root: "x"}
	_, _, ok := path.SplitIndexPath(p)
	if ok {
		t.Error("expected ok=false for path without segments")
	}
}

func TestSplitIndexPath_FieldSegmentOnly(t *testing.T) {
	p := constraint.Path{
		Root:     "x",
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "field"}},
	}
	_, _, ok := path.SplitIndexPath(p)
	if ok {
		t.Error("expected ok=false for field segment")
	}
}

func TestSplitIndexPath_StringIndexKey(t *testing.T) {
	p := constraint.Path{
		Root:     "x",
		Segments: []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "key"}},
	}
	base, key, ok := path.SplitIndexPath(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if base.Root != "x" {
		t.Error("expected base root 'x'")
	}
	if len(base.Segments) != 0 {
		t.Error("expected no segments in base")
	}
	lit, isLit := key.(*typ.Literal)
	if !isLit {
		t.Fatal("expected literal key")
	}
	if lit.Value.(string) != "key" {
		t.Error("expected literal string 'key'")
	}
}

func TestSplitIndexPath_IntIndex(t *testing.T) {
	p := constraint.Path{
		Root:     "arr",
		Segments: []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 3}},
	}
	base, key, ok := path.SplitIndexPath(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if base.Root != "arr" {
		t.Error("expected base root 'arr'")
	}
	lit, isLit := key.(*typ.Literal)
	if !isLit {
		t.Fatal("expected literal key")
	}
	if lit.Value.(int64) != 3 {
		t.Error("expected literal int 3")
	}
}

func TestTypeOfCallPathWithBindings_NotACall(t *testing.T) {
	_, ok := path.TypeOfCallPathWithBindings(&ast.IdentExpr{Value: "x"}, nil)
	if ok {
		t.Error("expected ok=false for non-call expr")
	}
}

func TestTypeOfCallPathWithBindings_NilCall(t *testing.T) {
	_, ok := path.TypeOfCallPathWithBindings(nil, nil)
	if ok {
		t.Error("expected ok=false for nil expr")
	}
}

func TestTypeOfCallPathWithBindings_MethodCallExpr(t *testing.T) {
	call := &ast.FuncCallExpr{
		Method:   "myMethod",
		Receiver: &ast.IdentExpr{Value: "obj"},
	}
	_, ok := path.TypeOfCallPathWithBindings(call, nil)
	if ok {
		t.Error("expected ok=false for method call")
	}
}

func TestTypeOfCallPathWithBindings_NotTypeCall(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "other"},
		Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
	}
	_, ok := path.TypeOfCallPathWithBindings(call, nil)
	if ok {
		t.Error("expected ok=false for non-type call")
	}
}

func TestTypeOfCallPathWithBindings_ZeroArgs(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "type"},
		Args: []ast.Expr{},
	}
	_, ok := path.TypeOfCallPathWithBindings(call, nil)
	if ok {
		t.Error("expected ok=false for zero args")
	}
}

func TestTypeOfCallPathWithBindings_ValidTypeCall(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "type"},
		Args: []ast.Expr{&ast.IdentExpr{Value: "myVar"}},
	}
	p, ok := path.TypeOfCallPathWithBindings(call, nil)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Root != "myVar" {
		t.Errorf("expected root 'myVar', got %q", p.Root)
	}
}

func TestTypeOfCallPathWithBindings_NonPathArg(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "type"},
		Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
	}
	_, ok := path.TypeOfCallPathWithBindings(call, nil)
	if ok {
		t.Error("expected ok=false for non-path arg")
	}
}

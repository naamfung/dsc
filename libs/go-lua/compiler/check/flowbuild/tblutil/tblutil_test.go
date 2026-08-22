package tblutil_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/tblutil"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTableHasFunctionField_NilTable(t *testing.T) {
	result := tblutil.TableHasFunctionField(nil)
	if result {
		t.Error("expected false for nil table")
	}
}

func TestTableHasFunctionField_NilFields(t *testing.T) {
	tbl := &ast.TableExpr{Fields: nil}
	result := tblutil.TableHasFunctionField(tbl)
	if result {
		t.Error("expected false for nil fields")
	}
}

func TestTableHasFunctionField_EmptyFields(t *testing.T) {
	tbl := &ast.TableExpr{Fields: []*ast.Field{}}
	result := tblutil.TableHasFunctionField(tbl)
	if result {
		t.Error("expected false for empty fields")
	}
}

func TestTableHasFunctionField_NoFunctions(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "x"}, Value: &ast.NumberExpr{Value: "1"}},
			{Key: &ast.StringExpr{Value: "y"}, Value: &ast.StringExpr{Value: "hello"}},
		},
	}
	result := tblutil.TableHasFunctionField(tbl)
	if result {
		t.Error("expected false for table without functions")
	}
}

func TestTableHasFunctionField_WithFunction(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "x"}, Value: &ast.NumberExpr{Value: "1"}},
			{Key: &ast.StringExpr{Value: "fn"}, Value: &ast.FunctionExpr{}},
		},
	}
	result := tblutil.TableHasFunctionField(tbl)
	if !result {
		t.Error("expected true for table with function")
	}
}

func TestTableHasFunctionField_NilFieldEntry(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{nil},
	}
	result := tblutil.TableHasFunctionField(tbl)
	if result {
		t.Error("expected false when field entry is nil")
	}
}

func TestTableHasFunctionField_NilValue(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "x"}, Value: nil},
		},
	}
	result := tblutil.TableHasFunctionField(tbl)
	if result {
		t.Error("expected false when value is nil")
	}
}

func TestSynthTableLiteralWithWrapper_NilTable(t *testing.T) {
	result := tblutil.SynthTableLiteralWithWrapper(nil, 0, nil)
	if result != nil {
		t.Error("expected nil for nil table")
	}
}

func TestSynthTableLiteralWithWrapper_EmptyTable(t *testing.T) {
	tbl := &ast.TableExpr{Fields: nil}
	result := tblutil.SynthTableLiteralWithWrapper(tbl, 0, nil)
	if result == nil {
		t.Fatal("expected non-nil result for empty table")
	}
	if result.Kind() != kind.Record {
		t.Errorf("expected record kind, got %v", result.Kind())
	}
}

func TestSynthTableLiteralWithWrapper_RecordFields(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "test"}},
			{Key: &ast.IdentExpr{Value: "count"}, Value: &ast.NumberExpr{Value: "42"}},
		},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		switch expr.(type) {
		case *ast.StringExpr:
			return typ.String
		case *ast.NumberExpr:
			return typ.Integer
		default:
			return typ.Unknown
		}
	}
	result := tblutil.SynthTableLiteralWithWrapper(tbl, 0, synth)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Kind() != kind.Record {
		t.Errorf("expected record kind, got %v", result.Kind())
	}
	rec := result.(*typ.Record)
	if len(rec.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(rec.Fields))
	}
}

func TestSynthTableLiteralWithWrapper_ArrayElements(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: nil, Value: &ast.StringExpr{Value: "a"}},
			{Key: nil, Value: &ast.StringExpr{Value: "b"}},
		},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	result := tblutil.SynthTableLiteralWithWrapper(tbl, 0, synth)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Kind() != kind.Tuple {
		t.Errorf("expected tuple kind for array elements, got %v", result.Kind())
	}
}

func TestSynthTableLiteralWithWrapper_VarargArray(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: nil, Value: &ast.StringExpr{Value: "a"}},
			{Key: nil, Value: &ast.Comma3Expr{}},
		},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	result := tblutil.SynthTableLiteralWithWrapper(tbl, 0, synth)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Kind() != kind.Array {
		t.Errorf("expected array kind for vararg table, got %v", result.Kind())
	}
}

func TestFunctionHasAnnotations_NilFunction(t *testing.T) {
	result := tblutil.FunctionHasAnnotations(nil)
	if result {
		t.Error("expected false for nil function")
	}
}

func TestFunctionHasAnnotations_NoAnnotations(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList:     &ast.ParList{Names: []string{"x", "y"}},
		ReturnTypes: nil,
	}
	result := tblutil.FunctionHasAnnotations(fn)
	if result {
		t.Error("expected false for function without annotations")
	}
}

func TestFunctionHasAnnotations_WithReturnType(t *testing.T) {
	fn := &ast.FunctionExpr{
		ReturnTypes: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
	}
	result := tblutil.FunctionHasAnnotations(fn)
	if !result {
		t.Error("expected true for function with return type")
	}
}

func TestFunctionHasAnnotations_WithParamType(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
		},
	}
	result := tblutil.FunctionHasAnnotations(fn)
	if !result {
		t.Error("expected true for function with param type")
	}
}

func TestFunctionHasAnnotations_WithVarargType(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			HasVargs:   true,
			VarargType: &ast.PrimitiveTypeExpr{Name: "any"},
		},
	}
	result := tblutil.FunctionHasAnnotations(fn)
	if !result {
		t.Error("expected true for function with vararg type")
	}
}

func TestFunctionHasAnnotations_NilParList(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList:     nil,
		ReturnTypes: nil,
	}
	result := tblutil.FunctionHasAnnotations(fn)
	if result {
		t.Error("expected false for function with nil parlist")
	}
}

func TestFunctionHasAnnotations_NilTypesInParList(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x", "y"},
			Types: []ast.TypeExpr{nil, nil},
		},
	}
	result := tblutil.FunctionHasAnnotations(fn)
	if result {
		t.Error("expected false when all types are nil")
	}
}

func TestFunctionHasAnnotations_NilReturnTypes(t *testing.T) {
	fn := &ast.FunctionExpr{
		ReturnTypes: []ast.TypeExpr{nil},
	}
	result := tblutil.FunctionHasAnnotations(fn)
	if result {
		t.Error("expected false when return type is nil")
	}
}

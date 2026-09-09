package resolve

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestResolveType_PrimitiveWithAnnotations(t *testing.T) {
	resolver := New(Config{})
	sc := scope.New()

	// number @min(0)
	expr := &ast.PrimitiveTypeExpr{
		Name: "number",
		Annotations: []ast.AnnotationExpr{
			{Name: "min", Args: []ast.Expr{&ast.NumberExpr{Value: "0"}}},
		},
	}

	result := resolver.ResolveType(expr, sc)
	annotated, ok := result.(*typ.Annotated)
	if !ok {
		t.Fatalf("expected *typ.Annotated, got %T", result)
	}
	if annotated.Inner != typ.Number {
		t.Errorf("inner type should be Number")
	}
	if len(annotated.Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotated.Annotations))
	}
	if annotated.Annotations[0].Name != "min" {
		t.Errorf("annotation name = %q, want 'min'", annotated.Annotations[0].Name)
	}
	if annotated.Annotations[0].Arg != float64(0) {
		t.Errorf("annotation arg = %v, want 0", annotated.Annotations[0].Arg)
	}
}

func TestResolveType_MultipleAnnotations(t *testing.T) {
	resolver := New(Config{})
	sc := scope.New()

	// number @min(0) @max(100)
	expr := &ast.PrimitiveTypeExpr{
		Name: "number",
		Annotations: []ast.AnnotationExpr{
			{Name: "min", Args: []ast.Expr{&ast.NumberExpr{Value: "0"}}},
			{Name: "max", Args: []ast.Expr{&ast.NumberExpr{Value: "100"}}},
		},
	}

	result := resolver.ResolveType(expr, sc)
	annotated, ok := result.(*typ.Annotated)
	if !ok {
		t.Fatalf("expected *typ.Annotated, got %T", result)
	}
	if len(annotated.Annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(annotated.Annotations))
	}
	if annotated.Annotations[0].Name != "min" {
		t.Errorf("annotation 0 name = %q, want 'min'", annotated.Annotations[0].Name)
	}
	if annotated.Annotations[1].Name != "max" {
		t.Errorf("annotation 1 name = %q, want 'max'", annotated.Annotations[1].Name)
	}
}

func TestResolveType_NumberAnnotation_HexFloatArg(t *testing.T) {
	resolver := New(Config{})
	sc := scope.New()

	expr := &ast.PrimitiveTypeExpr{
		Name: "number",
		Annotations: []ast.AnnotationExpr{
			{Name: "min", Args: []ast.Expr{&ast.NumberExpr{Value: "0x1p2"}}},
		},
	}

	result := resolver.ResolveType(expr, sc)
	annotated, ok := result.(*typ.Annotated)
	if !ok {
		t.Fatalf("expected *typ.Annotated, got %T", result)
	}
	if got := annotated.Annotations[0].Arg; got != float64(4) {
		t.Fatalf("annotation arg = %v, want 4", got)
	}
}

func TestResolveType_StringAnnotation(t *testing.T) {
	resolver := New(Config{})
	sc := scope.New()

	// string @pattern("^[a-z]+$")
	expr := &ast.PrimitiveTypeExpr{
		Name: "string",
		Annotations: []ast.AnnotationExpr{
			{Name: "pattern", Args: []ast.Expr{&ast.StringExpr{Value: "^[a-z]+$"}}},
		},
	}

	result := resolver.ResolveType(expr, sc)
	annotated, ok := result.(*typ.Annotated)
	if !ok {
		t.Fatalf("expected *typ.Annotated, got %T", result)
	}
	if annotated.Annotations[0].Arg != "^[a-z]+$" {
		t.Errorf("annotation arg = %v, want '^[a-z]+$'", annotated.Annotations[0].Arg)
	}
}

func TestResolveType_NoArgAnnotation(t *testing.T) {
	resolver := New(Config{})
	sc := scope.New()

	// number @integer
	expr := &ast.PrimitiveTypeExpr{
		Name: "number",
		Annotations: []ast.AnnotationExpr{
			{Name: "integer"},
		},
	}

	result := resolver.ResolveType(expr, sc)
	annotated, ok := result.(*typ.Annotated)
	if !ok {
		t.Fatalf("expected *typ.Annotated, got %T", result)
	}
	if annotated.Annotations[0].Name != "integer" {
		t.Errorf("annotation name = %q, want 'integer'", annotated.Annotations[0].Name)
	}
	if annotated.Annotations[0].Arg != nil {
		t.Errorf("annotation arg = %v, want nil", annotated.Annotations[0].Arg)
	}
}

func TestResolveType_RecordFieldAnnotations(t *testing.T) {
	resolver := New(Config{})
	sc := scope.New()

	// {age: number @min(0), name: string @min_len(1)}
	expr := &ast.RecordTypeExpr{
		Fields: []ast.RecordFieldExpr{
			{
				Name: "age",
				Type: &ast.PrimitiveTypeExpr{Name: "number"},
				Annotations: []ast.AnnotationExpr{
					{Name: "min", Args: []ast.Expr{&ast.NumberExpr{Value: "0"}}},
				},
			},
			{
				Name: "name",
				Type: &ast.PrimitiveTypeExpr{Name: "string"},
				Annotations: []ast.AnnotationExpr{
					{Name: "min_len", Args: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
				},
			},
		},
	}

	result := resolver.ResolveType(expr, sc)
	record, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected *typ.Record, got %T", result)
	}

	ageField := record.GetField("age")
	if ageField == nil {
		t.Fatal("missing 'age' field")
	}
	ageType, ok := ageField.Type.(*typ.Annotated)
	if !ok {
		t.Fatalf("age field expected annotated type, got %T", ageField.Type)
	}
	if len(ageType.Annotations) != 1 {
		t.Errorf("age field expected 1 annotation, got %d", len(ageType.Annotations))
	}
	if ageType.Annotations[0].Name != "min" {
		t.Errorf("age annotation name = %q, want 'min'", ageType.Annotations[0].Name)
	}

	nameField := record.GetField("name")
	if nameField == nil {
		t.Fatal("missing 'name' field")
	}
	nameType, ok := nameField.Type.(*typ.Annotated)
	if !ok {
		t.Fatalf("name field expected annotated type, got %T", nameField.Type)
	}
	if len(nameType.Annotations) != 1 {
		t.Errorf("name field expected 1 annotation, got %d", len(nameType.Annotations))
	}
	if nameType.Annotations[0].Name != "min_len" {
		t.Errorf("name annotation name = %q, want 'min_len'", nameType.Annotations[0].Name)
	}
}

func TestResolveType_NoAnnotationsReturnsPlainType(t *testing.T) {
	resolver := New(Config{})
	sc := scope.New()

	expr := &ast.PrimitiveTypeExpr{Name: "number"}

	result := resolver.ResolveType(expr, sc)
	if result != typ.Number {
		t.Errorf("expected typ.Number, got %T", result)
	}
}

func TestParseAndResolve_AnnotatedType(t *testing.T) {
	input := `local x: number @min(0) @max(100) = 50`
	stmts, err := parse.Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("expected LocalAssignStmt, got %T", stmts[0])
	}
	if len(local.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(local.Types))
	}

	resolver := New(Config{})
	sc := scope.New()
	result := resolver.ResolveType(local.Types[0], sc)

	annotated, ok := result.(*typ.Annotated)
	if !ok {
		t.Fatalf("expected *typ.Annotated, got %T", result)
	}
	if annotated.Inner != typ.Number {
		t.Errorf("inner type should be Number")
	}
	if len(annotated.Annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(annotated.Annotations))
	}
	if annotated.Annotations[0].Name != "min" {
		t.Errorf("annotation 0 name = %q, want 'min'", annotated.Annotations[0].Name)
	}
	if annotated.Annotations[1].Name != "max" {
		t.Errorf("annotation 1 name = %q, want 'max'", annotated.Annotations[1].Name)
	}
}

func TestParseAndResolve_ArrayTypeAnnotation(t *testing.T) {
	input := `type Tags = {string} @min_len(1)`
	stmts, err := parse.Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	typedef, ok := stmts[0].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("expected TypeDefStmt, got %T", stmts[0])
	}

	resolver := New(Config{})
	sc := scope.New()
	result := resolver.ResolveType(typedef.Type, sc)

	annotated, ok := result.(*typ.Annotated)
	if !ok {
		t.Fatalf("expected *typ.Annotated, got %T", result)
	}
	if _, ok := annotated.Inner.(*typ.Array); !ok {
		t.Fatalf("expected annotated array, got %T", annotated.Inner)
	}
	if len(annotated.Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotated.Annotations))
	}
	if annotated.Annotations[0].Name != "min_len" {
		t.Errorf("annotation name = %q, want 'min_len'", annotated.Annotations[0].Name)
	}
}

func TestParseAndResolve_RecordFieldArrayAnnotation(t *testing.T) {
	input := `type Holder = {items: {number} @min_len(1)}`
	stmts, err := parse.Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	typedef, ok := stmts[0].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("expected TypeDefStmt, got %T", stmts[0])
	}

	resolver := New(Config{})
	sc := scope.New()
	result := resolver.ResolveType(typedef.Type, sc)

	record, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected *typ.Record, got %T", result)
	}
	field := record.GetField("items")
	if field == nil {
		t.Fatal("missing 'items' field")
	}
	annotated, ok := field.Type.(*typ.Annotated)
	if !ok {
		t.Fatalf("items field expected annotated type, got %T", field.Type)
	}
	if len(annotated.Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotated.Annotations))
	}
	if annotated.Annotations[0].Name != "min_len" {
		t.Errorf("annotation name = %q, want 'min_len'", annotated.Annotations[0].Name)
	}
}

func TestParseAndResolve_RecordFieldAnnotations(t *testing.T) {
	input := `type User = {
		age: number @min(0) @max(150),
		name: string @min_len(1) @max_len(100)
	}`
	stmts, err := parse.Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	typeDef, ok := stmts[0].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("expected TypeDefStmt, got %T", stmts[0])
	}

	resolver := New(Config{})
	sc := scope.New()
	result := resolver.ResolveType(typeDef.Type, sc)

	record, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected *typ.Record, got %T", result)
	}

	ageField := record.GetField("age")
	if ageField == nil {
		t.Fatal("missing 'age' field")
	}
	ageType, ok := ageField.Type.(*typ.Annotated)
	if !ok {
		t.Fatalf("age field expected annotated type, got %T", ageField.Type)
	}
	if len(ageType.Annotations) != 2 {
		t.Errorf("age field expected 2 annotations, got %d", len(ageType.Annotations))
	}

	nameField := record.GetField("name")
	if nameField == nil {
		t.Fatal("missing 'name' field")
	}
	nameType, ok := nameField.Type.(*typ.Annotated)
	if !ok {
		t.Fatalf("name field expected annotated type, got %T", nameField.Type)
	}
	if len(nameType.Annotations) != 2 {
		t.Errorf("name field expected 2 annotations, got %d", len(nameType.Annotations))
	}
}

package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEmitTableLiteralFieldAssignments_NilTable(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	EmitTableLiteralFieldAssignments(nil, 1, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for nil table, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_ZeroSymbol(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{}
	EmitTableLiteralFieldAssignments(table, 0, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for zero symbol, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_EmptyTable(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{Fields: []*ast.Field{}}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for empty table, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_NilField(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{Fields: []*ast.Field{nil}}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for nil field, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_NilFieldValue(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "key"}, Value: nil},
		},
	}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for nil field value, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_FunctionField(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "fn"}, Value: &ast.FunctionExpr{}},
		},
	}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for function field, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_StringKey(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "value"}},
		},
	}
	// No bindings, so source path will be empty and no assignment emitted
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
}

func TestEmitTableLiteralFieldAssignments_IdentKey(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "fieldname"}, Value: &ast.StringExpr{Value: "value"}},
		},
	}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
}

func TestEmitTableLiteralFieldAssignments_NumberKey(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.NumberExpr{Value: "1"}, Value: &ast.StringExpr{Value: "value"}},
		},
	}
	// Number keys are skipped (array elements)
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for number key, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_EmptyFieldName(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: ""}, Value: &ast.StringExpr{Value: "value"}},
		},
	}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments for empty field name, got %d", len(inputs.Assignments))
	}
}

func TestEmitTableLiteralFieldAssignments_WithBindings(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	bindings := &bind.BindingTable{}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.IdentExpr{Value: "src"}},
		},
	}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, bindings, nil, nil, nil, inputs)
}

func TestEmitTableLiteralFieldAssignments_WithSynth(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "value"}},
		},
	}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, nil, synth, nil, inputs)
}

func TestEmitTableLiteralFieldAssignments_WithConstResolver(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	constResolver := func(name string) *flow.ConstValue {
		return nil
	}
	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.IdentExpr{Value: "x"}},
		},
	}
	EmitTableLiteralFieldAssignments(table, 1, "t", 0, nil, constResolver, nil, nil, inputs)
}

func TestEmitTableLiteralFieldAssignments_StringNonIdentifierKey_UsesIndexStringSegment(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	bindings := bind.NewBindingTable()
	src := &ast.IdentExpr{Value: "src"}
	bindings.Bind(src, 42)
	bindings.SetName(42, "src")

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "x-y"}, Value: src},
		},
	}

	EmitTableLiteralFieldAssignments(table, 1, "t", 0, bindings, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(inputs.Assignments))
	}
	got := inputs.Assignments[0]
	if len(got.TargetPath.Segments) != 1 {
		t.Fatalf("expected 1 target segment, got %d", len(got.TargetPath.Segments))
	}
	if got.TargetPath.Segments[0].Kind != constraint.SegmentIndexString {
		t.Fatalf("expected SegmentIndexString, got %v", got.TargetPath.Segments[0].Kind)
	}
	if got.TargetPath.Segments[0].Name != "x-y" {
		t.Fatalf("expected index key x-y, got %q", got.TargetPath.Segments[0].Name)
	}
}

func TestEmitTableLiteralFieldAssignments_IdentKey_UsesFieldSegment(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	bindings := bind.NewBindingTable()
	src := &ast.IdentExpr{Value: "src"}
	bindings.Bind(src, 99)
	bindings.SetName(99, "src")

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "name"}, Value: src},
		},
	}

	EmitTableLiteralFieldAssignments(table, 1, "t", 0, bindings, nil, nil, nil, inputs)
	if len(inputs.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(inputs.Assignments))
	}
	got := inputs.Assignments[0]
	if len(got.TargetPath.Segments) != 1 {
		t.Fatalf("expected 1 target segment, got %d", len(got.TargetPath.Segments))
	}
	if got.TargetPath.Segments[0].Kind != constraint.SegmentField {
		t.Fatalf("expected SegmentField, got %v", got.TargetPath.Segments[0].Kind)
	}
	if got.TargetPath.Segments[0].Name != "name" {
		t.Fatalf("expected field name, got %q", got.TargetPath.Segments[0].Name)
	}
}

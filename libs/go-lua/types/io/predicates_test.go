package io

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// roundTripConstraint encodes then decodes a single constraint.
func roundTripConstraint(t *testing.T, c constraint.Constraint) constraint.Constraint {
	t.Helper()
	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeConstraint(c)
	if w.err != nil {
		t.Fatalf("writeConstraint: %v", w.err)
	}

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readConstraint()
	if r.err != nil {
		t.Fatalf("readConstraint: %v", r.err)
	}
	return got
}

// path builds a constraint.Path matching readPath output (nil Segments -> empty slice).
func path(root string) constraint.Path {
	return constraint.Path{Root: root, Segments: []constraint.Segment{}}
}

func TestPredicateCodec_Truthy(t *testing.T) {
	orig := constraint.Truthy{Path: path("x")}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_Falsy(t *testing.T) {
	orig := constraint.Falsy{Path: path("y")}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_IsNil(t *testing.T) {
	orig := constraint.IsNil{Path: path("val")}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_NotNil(t *testing.T) {
	orig := constraint.NotNil{Path: path("result")}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_HasType(t *testing.T) {
	orig := constraint.HasType{
		Path: path("$0"),
		Type: narrow.BuiltinTypeKey("string"),
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_NotHasType(t *testing.T) {
	orig := constraint.NotHasType{
		Path: path("$0"),
		Type: narrow.BuiltinTypeKey("number"),
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_HasType_HashKey(t *testing.T) {
	orig := constraint.HasType{
		Path: path("$0"),
		Type: narrow.HashTypeKey(0xDEADBEEF),
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_FieldEquals(t *testing.T) {
	orig := constraint.FieldEquals{
		Target: path("obj"),
		Field:  "status",
		Value:  typ.LiteralString("ok"),
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.FieldEquals)
	if !ok {
		t.Fatalf("expected FieldEquals, got %T", result)
	}
	if !reflect.DeepEqual(got.Target, orig.Target) {
		t.Errorf("target: got %+v, want %+v", got.Target, orig.Target)
	}
	if got.Field != orig.Field {
		t.Errorf("field: got %q, want %q", got.Field, orig.Field)
	}
	if !got.Value.Equals(orig.Value) {
		t.Errorf("value: got %v, want %v", got.Value, orig.Value)
	}
}

func TestPredicateCodec_FieldEquals_NilValue(t *testing.T) {
	orig := constraint.FieldEquals{
		Target: path("obj"),
		Field:  "tag",
		Value:  nil,
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.FieldEquals)
	if !ok {
		t.Fatalf("expected FieldEquals, got %T", result)
	}
	if got.Value != nil {
		t.Errorf("value: got %v, want nil", got.Value)
	}
}

func TestPredicateCodec_IndexEquals(t *testing.T) {
	orig := constraint.IndexEquals{
		Target: path("tbl"),
		Key:    typ.String,
		Value:  typ.LiteralInt(42),
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.IndexEquals)
	if !ok {
		t.Fatalf("expected IndexEquals, got %T", result)
	}
	if !reflect.DeepEqual(got.Target, orig.Target) {
		t.Errorf("target mismatch")
	}
	if !typ.TypeEquals(got.Key, orig.Key) {
		t.Errorf("key: got %v, want %v", got.Key, orig.Key)
	}
	if !got.Value.Equals(orig.Value) {
		t.Errorf("value: got %v, want %v", got.Value, orig.Value)
	}
}

func TestPredicateCodec_IndexEquals_NilKeyValue(t *testing.T) {
	orig := constraint.IndexEquals{
		Target: path("tbl"),
		Key:    nil,
		Value:  nil,
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.IndexEquals)
	if !ok {
		t.Fatalf("expected IndexEquals, got %T", result)
	}
	if got.Key != nil {
		t.Errorf("key: got %v, want nil", got.Key)
	}
	if got.Value != nil {
		t.Errorf("value: got %v, want nil", got.Value)
	}
}

func TestPredicateCodec_EqPath(t *testing.T) {
	orig := constraint.EqPath{
		Left:  path("a"),
		Right: path("b"),
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_NotEqPath(t *testing.T) {
	orig := constraint.NotEqPath{
		Left:  path("a"),
		Right: path("b"),
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_FieldEqualsPath(t *testing.T) {
	orig := constraint.FieldEqualsPath{
		Target: path("obj"),
		Field:  "name",
		Value:  path("expected"),
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_IndexEqualsPath(t *testing.T) {
	orig := constraint.IndexEqualsPath{
		Target: path("tbl"),
		Key:    typ.Integer,
		Value:  path("val"),
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.IndexEqualsPath)
	if !ok {
		t.Fatalf("expected IndexEqualsPath, got %T", result)
	}
	if !reflect.DeepEqual(got.Target, orig.Target) || !reflect.DeepEqual(got.Value, orig.Value) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
	if !typ.TypeEquals(got.Key, orig.Key) {
		t.Errorf("key: got %v, want %v", got.Key, orig.Key)
	}
}

func TestPredicateCodec_PathWithSegments(t *testing.T) {
	orig := constraint.Truthy{
		Path: constraint.Path{
			Root: "obj",
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "inner"},
				{Kind: constraint.SegmentIndexInt, Index: 3},
				{Kind: constraint.SegmentIndexString, Name: "key"},
			},
		},
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_Condition_Empty(t *testing.T) {
	orig := constraint.Condition{}
	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeCondition(orig)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readCondition()
	if r.err != nil {
		t.Fatalf("readCondition: %v", r.err)
	}
	if got.HasConstraints() {
		t.Error("empty condition should have no constraints")
	}
}

func TestPredicateCodec_Condition_MultiDisjunct(t *testing.T) {
	orig := constraint.Condition{
		Disjuncts: [][]constraint.Constraint{
			{
				constraint.NotNil{Path: path("$0")},
				constraint.HasType{Path: path("$0"), Type: narrow.BuiltinTypeKey("string")},
			},
			{
				constraint.IsNil{Path: path("$0")},
			},
		},
	}

	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeCondition(orig)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readCondition()
	if r.err != nil {
		t.Fatalf("readCondition: %v", r.err)
	}
	if got.NumDisjuncts() != 2 {
		t.Fatalf("disjuncts: got %d, want 2", got.NumDisjuncts())
	}
	if len(got.Disjuncts[0]) != 2 {
		t.Errorf("disjunct 0: got %d constraints, want 2", len(got.Disjuncts[0]))
	}
	if len(got.Disjuncts[1]) != 1 {
		t.Errorf("disjunct 1: got %d constraints, want 1", len(got.Disjuncts[1]))
	}
}

func TestPredicateCodec_FunctionRefinement(t *testing.T) {
	orig := &constraint.FunctionRefinement{
		OnReturn: constraint.FromConstraints(constraint.NotNil{Path: path("$0")}),
		OnTrue:   constraint.FromConstraints(constraint.HasType{Path: path("$0"), Type: narrow.BuiltinTypeKey("string")}),
		OnFalse:  constraint.FromConstraints(constraint.NotHasType{Path: path("$0"), Type: narrow.BuiltinTypeKey("string")}),
	}

	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeFunctionRefinement(orig)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readFunctionRefinement()
	if r.err != nil {
		t.Fatalf("readFunctionRefinement: %v", r.err)
	}
	if got == nil {
		t.Fatal("expected non-nil FunctionRefinement")
	}
	if len(got.OnReturn.MustConstraints()) != 1 {
		t.Errorf("OnReturn: got %d, want 1", len(got.OnReturn.MustConstraints()))
	}
	if len(got.OnTrue.MustConstraints()) != 1 {
		t.Errorf("OnTrue: got %d, want 1", len(got.OnTrue.MustConstraints()))
	}
	if len(got.OnFalse.MustConstraints()) != 1 {
		t.Errorf("OnFalse: got %d, want 1", len(got.OnFalse.MustConstraints()))
	}
}

func TestPredicateCodec_FunctionRefinement_Nil(t *testing.T) {
	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeFunctionRefinement(nil)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readFunctionRefinement()
	if r.err != nil {
		t.Fatalf("readFunctionRefinement: %v", r.err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestPredicateCodec_Expr_AllTypes(t *testing.T) {
	cases := []struct {
		name string
		expr constraint.Expr
	}{
		{"nil", nil},
		{"var", constraint.Var{Name: "count"}},
		{"const", constraint.Const{Value: -99}},
		{"len", constraint.Len{Of: "arr"}},
		{"param", constraint.Param{Index: 2}},
		{"ret", constraint.Ret{Index: 1}},
		{"param_len", constraint.ParamLen{Index: 0}},
		{"ret_len", constraint.RetLen{Index: 3}},
		{"add", constraint.BinOp{Op: constraint.OpAdd, Left: constraint.Var{Name: "x"}, Right: constraint.Const{Value: 1}}},
		{"sub", constraint.BinOp{Op: constraint.OpSub, Left: constraint.Param{Index: 0}, Right: constraint.Const{Value: 1}}},
		{"mul", constraint.BinOp{Op: constraint.OpMul, Left: constraint.Const{Value: 2}, Right: constraint.Var{Name: "n"}}},
		{"div", constraint.BinOp{Op: constraint.OpDiv, Left: constraint.Var{Name: "total"}, Right: constraint.Const{Value: 4}}},
		{"mod", constraint.BinOp{Op: constraint.OpMod, Left: constraint.Var{Name: "i"}, Right: constraint.Const{Value: 2}}},
		{"min", constraint.Min{Left: constraint.Var{Name: "a"}, Right: constraint.Var{Name: "b"}}},
		{"max", constraint.Max{Left: constraint.Const{Value: 0}, Right: constraint.Var{Name: "len"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := &typeWriter{w: &buf}
			w.writeExpr(tc.expr)
			if w.err != nil {
				t.Fatalf("writeExpr: %v", w.err)
			}

			r := &typeReader{r: bytes.NewReader(buf.Bytes())}
			got := r.readExpr()
			if r.err != nil {
				t.Fatalf("readExpr: %v", r.err)
			}

			if !constraint.ExprEquals(tc.expr, got) {
				t.Errorf("got %v, want %v", got, tc.expr)
			}
		})
	}
}

func TestPredicateCodec_Expr_Nested(t *testing.T) {
	// min(max(param[0], 0), len(param[1]) - 1)
	orig := constraint.Min{
		Left: constraint.Max{
			Left:  constraint.Param{Index: 0},
			Right: constraint.Const{Value: 0},
		},
		Right: constraint.BinOp{
			Op:    constraint.OpSub,
			Left:  constraint.ParamLen{Index: 1},
			Right: constraint.Const{Value: 1},
		},
	}

	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeExpr(orig)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readExpr()
	if r.err != nil {
		t.Fatalf("readExpr: %v", r.err)
	}
	if !constraint.ExprEquals(orig, got) {
		t.Errorf("got %v, want %v", got, orig)
	}
}

func TestPredicateCodec_ExprCompares(t *testing.T) {
	orig := []constraint.ExprCompare{
		{Rel: constraint.ExprEq, Left: constraint.ParamLen{Index: 0}, Right: constraint.RetLen{Index: 0}},
		{Rel: constraint.ExprLe, Left: constraint.Param{Index: 1}, Right: constraint.ParamLen{Index: 0}},
		{Rel: constraint.ExprGt, Left: constraint.Var{Name: "n"}, Right: constraint.Const{Value: 0}},
		{Rel: constraint.ExprNe, Left: constraint.Ret{Index: 0}, Right: constraint.Const{Value: -1}},
		{Rel: constraint.ExprLt, Left: constraint.Const{Value: 0}, Right: constraint.Var{Name: "x"}},
		{Rel: constraint.ExprGe, Left: constraint.Var{Name: "i"}, Right: constraint.Const{Value: 1}},
	}

	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeExprCompares(orig)
	if w.err != nil {
		t.Fatalf("writeExprCompares: %v", w.err)
	}

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readExprCompares()
	if r.err != nil {
		t.Fatalf("readExprCompares: %v", r.err)
	}

	if len(got) != len(orig) {
		t.Fatalf("len: got %d, want %d", len(got), len(orig))
	}
	for i := range orig {
		if !orig[i].Equals(got[i]) {
			t.Errorf("[%d] got %v, want %v", i, got[i], orig[i])
		}
	}
}

func TestPredicateCodec_ExprCompares_Empty(t *testing.T) {
	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeExprCompares(nil)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readExprCompares()
	if r.err != nil {
		t.Fatalf("readExprCompares: %v", r.err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestPredicateCodec_NilConstraint(t *testing.T) {
	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeConstraint(nil)

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readConstraint()
	if r.err != nil {
		t.Fatalf("readConstraint: %v", r.err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestPredicateCodec_KeyOf(t *testing.T) {
	orig := constraint.KeyOf{
		Table: constraint.ParamPath(0),
		Key:   constraint.RetPath(0),
	}

	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeConstraint(orig)
	if w.err != nil {
		t.Fatalf("writeConstraint: %v", w.err)
	}

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readConstraint()
	if r.err != nil {
		t.Fatalf("readConstraint: %v", r.err)
	}

	keyOf, ok := got.(constraint.KeyOf)
	if !ok {
		t.Fatalf("expected KeyOf, got %T", got)
	}
	if !keyOf.Table.Equal(orig.Table) {
		t.Errorf("Table: got %v, want %v", keyOf.Table, orig.Table)
	}
	if !keyOf.Key.Equal(orig.Key) {
		t.Errorf("Key: got %v, want %v", keyOf.Key, orig.Key)
	}
}

func TestPredicateCodec_FunctionRefinement_WithKeyOf(t *testing.T) {
	keyOf := constraint.KeyOf{
		Table: constraint.ParamPath(0),
		Key:   constraint.RetPath(0),
	}
	orig := &constraint.FunctionRefinement{
		OnReturn: constraint.FromConstraints(keyOf),
	}

	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeFunctionRefinement(orig)
	if w.err != nil {
		t.Fatalf("writeFunctionRefinement: %v", w.err)
	}

	r := &typeReader{r: bytes.NewReader(buf.Bytes())}
	got := r.readFunctionRefinement()
	if r.err != nil {
		t.Fatalf("readFunctionRefinement: %v", r.err)
	}
	if got == nil {
		t.Fatal("expected non-nil FunctionRefinement")
	}
	if !got.OnReturn.HasConstraints() {
		t.Fatal("OnReturn should have constraints")
	}

	constraints := got.OnReturn.MustConstraints()
	if len(constraints) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(constraints))
	}
	if _, ok := constraints[0].(constraint.KeyOf); !ok {
		t.Fatalf("expected KeyOf, got %T", constraints[0])
	}
	if paramIdx := got.KeysCollectorParamIndex(); paramIdx != 0 {
		t.Errorf("KeysCollectorParamIndex: got %d, want 0", paramIdx)
	}
}

func TestPredicateCodec_HasField(t *testing.T) {
	orig := constraint.HasField{
		Path:  path("obj"),
		Field: "name",
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_FieldNotEquals(t *testing.T) {
	orig := constraint.FieldNotEquals{
		Target: path("obj"),
		Field:  "status",
		Value:  typ.LiteralString("error"),
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.FieldNotEquals)
	if !ok {
		t.Fatalf("expected FieldNotEquals, got %T", result)
	}
	if !reflect.DeepEqual(got.Target, orig.Target) {
		t.Errorf("target mismatch")
	}
	if got.Field != orig.Field {
		t.Errorf("field: got %q, want %q", got.Field, orig.Field)
	}
	if !got.Value.Equals(orig.Value) {
		t.Errorf("value: got %v, want %v", got.Value, orig.Value)
	}
}

func TestPredicateCodec_IndexNotEquals(t *testing.T) {
	orig := constraint.IndexNotEquals{
		Target: path("tbl"),
		Key:    typ.String,
		Value:  typ.LiteralInt(0),
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.IndexNotEquals)
	if !ok {
		t.Fatalf("expected IndexNotEquals, got %T", result)
	}
	if !reflect.DeepEqual(got.Target, orig.Target) {
		t.Errorf("target mismatch")
	}
	if !typ.TypeEquals(got.Key, orig.Key) {
		t.Errorf("key: got %v, want %v", got.Key, orig.Key)
	}
	if !got.Value.Equals(orig.Value) {
		t.Errorf("value: got %v, want %v", got.Value, orig.Value)
	}
}

func TestPredicateCodec_FieldNotEqualsPath(t *testing.T) {
	orig := constraint.FieldNotEqualsPath{
		Target: path("obj"),
		Field:  "name",
		Value:  path("expected"),
	}
	got := roundTripConstraint(t, orig)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPredicateCodec_IndexNotEqualsPath(t *testing.T) {
	orig := constraint.IndexNotEqualsPath{
		Target: path("tbl"),
		Key:    typ.Integer,
		Value:  path("val"),
	}
	result := roundTripConstraint(t, orig)
	got, ok := result.(constraint.IndexNotEqualsPath)
	if !ok {
		t.Fatalf("expected IndexNotEqualsPath, got %T", result)
	}
	if !reflect.DeepEqual(got.Target, orig.Target) || !reflect.DeepEqual(got.Value, orig.Value) {
		t.Errorf("got %+v, want %+v", got, orig)
	}
	if !typ.TypeEquals(got.Key, orig.Key) {
		t.Errorf("key: got %v, want %v", got.Key, orig.Key)
	}
}

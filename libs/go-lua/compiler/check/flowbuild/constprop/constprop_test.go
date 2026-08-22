package constprop_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/constprop"
	"github.com/wippyai/go-lua/types/flow"
)

func TestConstValueFromExpr_String(t *testing.T) {
	expr := &ast.StringExpr{Value: "hello"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(string) returned nil")
	}
	if val.Kind != flow.ConstString {
		t.Errorf("Kind = %v, want ConstString", val.Kind)
	}
	if val.Str != "hello" {
		t.Errorf("Str = %q, want %q", val.Str, "hello")
	}
}

func TestConstValueFromExpr_Number(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(number) returned nil")
	}
	if val.Kind != flow.ConstInt {
		t.Errorf("Kind = %v, want ConstInt", val.Kind)
	}
	if val.Int != 42 {
		t.Errorf("Int = %d, want %d", val.Int, 42)
	}
}

func TestConstValueFromExpr_Float(t *testing.T) {
	expr := &ast.NumberExpr{Value: "3.14"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(float) returned nil")
	}
	if val.Kind != flow.ConstFloat {
		t.Errorf("Kind = %v, want ConstFloat", val.Kind)
	}
	if val.Float != 3.14 {
		t.Errorf("Float = %f, want %f", val.Float, 3.14)
	}
}

func TestConstValueFromExpr_NumberLeadingZeroDecimal(t *testing.T) {
	expr := &ast.NumberExpr{Value: "08"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(08) returned nil")
	}
	if val.Kind != flow.ConstInt {
		t.Errorf("Kind = %v, want ConstInt", val.Kind)
	}
	if val.Int != 8 {
		t.Errorf("Int = %d, want %d", val.Int, 8)
	}
}

func TestConstValueFromExpr_HexFloat(t *testing.T) {
	expr := &ast.NumberExpr{Value: "0x1p2"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(0x1p2) returned nil")
	}
	if val.Kind != flow.ConstFloat {
		t.Errorf("Kind = %v, want ConstFloat", val.Kind)
	}
	if val.Float != 4 {
		t.Errorf("Float = %f, want 4", val.Float)
	}
}

func TestConstValueFromExpr_Nil(t *testing.T) {
	val := constprop.ConstValueFromExpr(nil)
	if val != nil {
		t.Errorf("constprop.ConstValueFromExpr(nil) = %v, want nil", val)
	}
}

func TestConstValueFromExpr_True(t *testing.T) {
	expr := &ast.TrueExpr{}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(true) returned nil")
	}
	if val.Kind != flow.ConstBool {
		t.Errorf("Kind = %v, want ConstBool", val.Kind)
	}
	if !val.Bool {
		t.Errorf("Bool = %v, want true", val.Bool)
	}
}

func TestConstValueFromExpr_False(t *testing.T) {
	expr := &ast.FalseExpr{}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(false) returned nil")
	}
	if val.Kind != flow.ConstBool {
		t.Errorf("Kind = %v, want ConstBool", val.Kind)
	}
	if val.Bool {
		t.Errorf("Bool = %v, want false", val.Bool)
	}
}

func TestConstValueFromExpr_NilExpr(t *testing.T) {
	expr := &ast.NilExpr{}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(nil expr) returned nil")
	}
	if val.Kind != flow.ConstNil {
		t.Errorf("Kind = %v, want ConstNil", val.Kind)
	}
}

func TestConstValueFromExpr_Other(t *testing.T) {
	expr := &ast.IdentExpr{Value: "foo"}
	val := constprop.ConstValueFromExpr(expr)
	if val != nil {
		t.Errorf("constprop.ConstValueFromExpr(ident) = %v, want nil", val)
	}
}

func TestConstValueFromExpr_NumberFormats(t *testing.T) {
	tests := []struct {
		input   string
		wantNil bool
		kind    flow.ConstKind
		intVal  int64
		floatV  float64
	}{
		{input: "-1", kind: flow.ConstInt, intVal: -1},
		{input: "0xDEAD", kind: flow.ConstInt, intVal: 0xDEAD},
		{input: "1e10", kind: flow.ConstFloat, floatV: 1e10},
		{input: "3.14", kind: flow.ConstFloat, floatV: 3.14},
		{input: "abc", wantNil: true},
		{input: "", wantNil: true},
	}

	for _, tc := range tests {
		val := constprop.ConstValueFromExpr(&ast.NumberExpr{Value: tc.input})
		if tc.wantNil {
			if val != nil {
				t.Errorf("ConstValueFromExpr(%q) = %v, want nil", tc.input, val)
			}
			continue
		}
		if val == nil {
			t.Errorf("ConstValueFromExpr(%q) returned nil", tc.input)
			continue
		}
		if val.Kind != tc.kind {
			t.Errorf("ConstValueFromExpr(%q) kind = %v, want %v", tc.input, val.Kind, tc.kind)
			continue
		}
		if tc.kind == flow.ConstInt && val.Int != tc.intVal {
			t.Errorf("ConstValueFromExpr(%q) int = %d, want %d", tc.input, val.Int, tc.intVal)
		}
		if tc.kind == flow.ConstFloat && val.Float != tc.floatV {
			t.Errorf("ConstValueFromExpr(%q) float = %f, want %f", tc.input, val.Float, tc.floatV)
		}
	}
}

func TestConstValueEqual_Float(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstFloat, Float: 3.14}
	b := &flow.ConstValue{Kind: flow.ConstFloat, Float: 3.14}
	c := &flow.ConstValue{Kind: flow.ConstFloat, Float: 2.71}

	if !constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(3.14, 3.14) = false, want true")
	}
	if constprop.ConstValueEqual(a, c) {
		t.Error("constprop.ConstValueEqual(3.14, 2.71) = true, want false")
	}
}

func TestConstValueEqual_Bool(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstBool, Bool: true}
	b := &flow.ConstValue{Kind: flow.ConstBool, Bool: true}
	c := &flow.ConstValue{Kind: flow.ConstBool, Bool: false}

	if !constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(true, true) = false, want true")
	}
	if constprop.ConstValueEqual(a, c) {
		t.Error("constprop.ConstValueEqual(true, false) = true, want false")
	}
}

func TestConstValueEqual_Nil(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstNil}
	b := &flow.ConstValue{Kind: flow.ConstNil}

	if !constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(nil, nil) = false, want true")
	}
}

func TestConstValueEqual_DifferentKinds(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstInt, Int: 42}
	b := &flow.ConstValue{Kind: flow.ConstFloat, Float: 42.0}

	if constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(int 42, float 42.0) = true, want false")
	}
}

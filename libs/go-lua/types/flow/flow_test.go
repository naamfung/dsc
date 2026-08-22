package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnKind_Values(t *testing.T) {
	tests := []struct {
		kind ReturnKind
		want int
	}{
		{ReturnUnknown, 0},
		{ReturnTrue, 1},
		{ReturnFalse, 2},
	}
	for _, tt := range tests {
		if int(tt.kind) != tt.want {
			t.Errorf("ReturnKind %v = %d, want %d", tt.kind, tt.kind, tt.want)
		}
	}
}

func TestConstKind_Values(t *testing.T) {
	tests := []struct {
		kind ConstKind
		want int
	}{
		{ConstUnknown, 0},
		{ConstString, 1},
		{ConstInt, 2},
		{ConstFloat, 3},
		{ConstBool, 4},
		{ConstNil, 5},
	}
	for _, tt := range tests {
		if int(tt.kind) != tt.want {
			t.Errorf("ConstKind %v = %d, want %d", tt.kind, tt.kind, tt.want)
		}
	}
}

func TestConstValue_ToLiteralType_Nil(t *testing.T) {
	var cv *ConstValue
	if cv.ToLiteralType() != nil {
		t.Error("nil ConstValue should return nil")
	}
}

func TestConstValue_ToLiteralType_String(t *testing.T) {
	cv := &ConstValue{Kind: ConstString, Str: "hello"}
	lt := cv.ToLiteralType()
	if lt == nil {
		t.Fatal("expected literal type")
	}
	lit, ok := lt.(*typ.Literal)
	if !ok {
		t.Fatal("expected *typ.Literal")
	}
	if lit.Value != "hello" {
		t.Errorf("got %v, want hello", lit.Value)
	}
}

func TestConstValue_ToLiteralType_Int(t *testing.T) {
	cv := &ConstValue{Kind: ConstInt, Int: 42}
	lt := cv.ToLiteralType()
	if lt == nil {
		t.Fatal("expected literal type")
	}
	lit, ok := lt.(*typ.Literal)
	if !ok {
		t.Fatal("expected *typ.Literal")
	}
	if lit.Value != int64(42) {
		t.Errorf("got %v, want 42", lit.Value)
	}
}

func TestConstValue_ToLiteralType_Float(t *testing.T) {
	cv := &ConstValue{Kind: ConstFloat, Float: 3.14}
	lt := cv.ToLiteralType()
	if lt == nil {
		t.Fatal("expected literal type")
	}
}

func TestConstValue_ToLiteralType_Bool(t *testing.T) {
	cv := &ConstValue{Kind: ConstBool, Bool: true}
	lt := cv.ToLiteralType()
	if lt == nil {
		t.Fatal("expected literal type")
	}
}

func TestConstValue_ToLiteralType_NilKind(t *testing.T) {
	cv := &ConstValue{Kind: ConstNil}
	lt := cv.ToLiteralType()
	if lt != typ.Nil {
		t.Error("expected typ.Nil")
	}
}

func TestConstValue_ToLiteralType_Unknown(t *testing.T) {
	cv := &ConstValue{Kind: ConstUnknown}
	lt := cv.ToLiteralType()
	if lt != nil {
		t.Error("expected nil for unknown const kind")
	}
}

func TestIteratorKind_Values(t *testing.T) {
	if IterateIndexed != 0 {
		t.Errorf("IterateIndexed = %d, want 0", IterateIndexed)
	}
	if IterateKeyed != 1 {
		t.Errorf("IterateKeyed = %d, want 1", IterateKeyed)
	}
}

func TestTypeState_Values(t *testing.T) {
	tests := []struct {
		state TypeState
		want  int
	}{
		{StateUnknown, 0},
		{StateResolved, 1},
		{StatePending, 2},
		{StateConflicted, 3},
	}
	for _, tt := range tests {
		if int(tt.state) != tt.want {
			t.Errorf("TypeState %v = %d, want %d", tt.state, tt.state, tt.want)
		}
	}
}

func TestTypedValue(t *testing.T) {
	tv := TypedValue{
		Type:  typ.String,
		State: StateResolved,
	}
	if tv.Type != typ.String {
		t.Error("unexpected type")
	}
	if tv.State != StateResolved {
		t.Error("unexpected state")
	}
}

func TestInputs_Zero(t *testing.T) {
	inputs := Inputs{}
	if inputs.Graph != nil {
		t.Error("zero Inputs should have nil graph")
	}
}

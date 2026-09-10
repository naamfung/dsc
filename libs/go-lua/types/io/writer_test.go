package io

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestEncode_Nil(t *testing.T) {
	data, err := Encode(nil)
	if err != nil {
		t.Fatalf("Encode(nil) returned error: %v", err)
	}
	if len(data) == 0 {
		t.Error("Encode(nil) should return non-empty data")
	}
}

func TestEncode_Primitives(t *testing.T) {
	types := []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.Integer,
		typ.String,
		typ.Any,
		typ.Unknown,
		typ.Never,
	}
	for _, ty := range types {
		data, err := Encode(ty)
		if err != nil {
			t.Errorf("Encode(%v) failed: %v", ty, err)
		}
		if len(data) == 0 {
			t.Errorf("Encode(%v) returned empty data", ty)
		}
	}
}

func TestEncode_Function(t *testing.T) {
	fn := typ.Func().Param("x", typ.Number).Returns(typ.String).Build()
	data, err := Encode(fn)
	if err != nil {
		t.Fatalf("Encode(function) failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("Encode(function) returned empty data")
	}
}

func TestEncode_Record(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	data, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode(record) failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("Encode(record) returned empty data")
	}
}

func TestEncode_Literal(t *testing.T) {
	tests := []typ.Type{
		typ.LiteralBool(true),
		typ.LiteralInt(42),
		typ.LiteralNumber(3.14),
		typ.LiteralString("hello"),
	}
	for _, ty := range tests {
		data, err := Encode(ty)
		if err != nil {
			t.Errorf("Encode(%v) failed: %v", ty, err)
		}
		if len(data) == 0 {
			t.Errorf("Encode(%v) returned empty data", ty)
		}
	}
}

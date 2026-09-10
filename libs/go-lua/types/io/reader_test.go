package io

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestDecode_Nil(t *testing.T) {
	_, err := Decode(nil)
	if err == nil {
		t.Error("Decode(nil) should return error")
	}
}

func TestDecode_RoundTrip_Primitives(t *testing.T) {
	tests := []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.Integer,
		typ.String,
		typ.Any,
		typ.Unknown,
		typ.Never,
	}
	for _, want := range tests {
		data, err := Encode(want)
		if err != nil {
			t.Errorf("Encode(%v) failed: %v", want, err)
			continue
		}
		got, err := Decode(data)
		if err != nil {
			t.Errorf("Decode failed for %v: %v", want, err)
			continue
		}
		if !typ.TypeEquals(got, want) {
			t.Errorf("round-trip failed: got %v, want %v", got, want)
		}
	}
}

func TestDecode_RoundTrip_Optional(t *testing.T) {
	want := typ.NewOptional(typ.String)
	data, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !typ.TypeEquals(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDecode_RoundTrip_Union(t *testing.T) {
	want := typ.NewUnion(typ.String, typ.Number)
	data, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if got == nil {
		t.Fatal("Decode returned nil")
	}
}

func TestDecode_RoundTrip_Array(t *testing.T) {
	want := typ.NewArray(typ.Integer)
	data, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !typ.TypeEquals(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDecode_RoundTrip_Map(t *testing.T) {
	want := typ.NewMap(typ.String, typ.Number)
	data, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !typ.TypeEquals(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

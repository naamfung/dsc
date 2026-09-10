package numparse

import (
	"strconv"
	"testing"
)

func TestParseNonNegativeDecimalInt(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := strconv.FormatInt(int64(maxInt), 10) + "0"

	tests := []struct {
		input string
		want  int
		ok    bool
	}{
		{"0", 0, true},
		{"42", 42, true},
		{"001", 1, true},
		{"", 0, false},
		{"-1", 0, false},
		{"+1", 0, false},
		{"1e3", 0, false},
		{"abc", 0, false},
		{overflow, 0, false},
	}

	for _, tt := range tests {
		got, ok := ParseNonNegativeDecimalInt(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseNonNegativeDecimalInt(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseNonNegativeDecimalInt(%q)=%d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseIntegerLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"42", 42, true},
		{"08", 8, true},
		{"0xDEAD", 0xDEAD, true},
		{"0x1p2", 0, false},
		{"1e3", 0, false},
		{"0.0", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseIntegerLiteral(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseIntegerLiteral(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseIntegerLiteral(%q)=%d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseNumberLiteral(t *testing.T) {
	tests := []struct {
		input   string
		wantInt int64
		wantFlt float64
		isInt   bool
		parseOK bool
	}{
		{"08", 8, 0, true, true},
		{"0xDEAD", 0xDEAD, 0, true, true},
		{"3.14", 0, 3.14, false, true},
		{"0x1p2", 0, 4, false, true},
		{"nope", 0, 0, false, false},
	}
	for _, tt := range tests {
		i, f, ok := ParseNumberLiteral(tt.input)
		if ok != tt.parseOK {
			t.Fatalf("ParseNumberLiteral(%q) ok=%v, want %v", tt.input, ok, tt.parseOK)
		}
		if !ok {
			continue
		}
		if tt.isInt {
			if i != tt.wantInt || f != 0 {
				t.Fatalf("ParseNumberLiteral(%q) got int=%d float=%f, want int=%d float=0", tt.input, i, f, tt.wantInt)
			}
			continue
		}
		if i != 0 || f != tt.wantFlt {
			t.Fatalf("ParseNumberLiteral(%q) got int=%d float=%f, want int=0 float=%f", tt.input, i, f, tt.wantFlt)
		}
	}
}

func TestParseFloatLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"42", 42, true},
		{"0.0", 0, true},
		{"0x1p2", 4, true},
		{"nope", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseFloatLiteral(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseFloatLiteral(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseFloatLiteral(%q)=%v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseIntegralLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"1", 1, true},
		{"1.0", 1, true},
		{"1e0", 1, true},
		{"0x1p0", 1, true},
		{"-1.0", -1, true},
		{"1.5", 0, false},
		{"0x1.8p1", 3, true},
		{"nope", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseIntegralLiteral(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseIntegralLiteral(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseIntegralLiteral(%q)=%v, want %v", tt.input, got, tt.want)
		}
	}
}

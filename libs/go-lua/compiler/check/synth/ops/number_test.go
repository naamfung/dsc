package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestParseNumber_Integer(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"123456789", 123456789},
	}
	for _, tt := range tests {
		result := ParseNumber(tt.input)
		lit, ok := result.(*typ.Literal)

		if !ok {
			t.Errorf("ParseNumber(%q) should return Literal, got %T", tt.input, result)
			continue
		}

		if lit.Base != kind.Integer {
			t.Errorf("ParseNumber(%q) should have Base Integer, got %v", tt.input, lit.Base)
			continue
		}

		val, ok := lit.Value.(int64)
		if !ok {
			t.Errorf("ParseNumber(%q) value should be int64, got %T", tt.input, lit.Value)
			continue
		}

		if val != tt.want {
			t.Errorf("ParseNumber(%q) = %d, want %d", tt.input, val, tt.want)
		}
	}
}

func TestParseNumber_Float(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"3.14", 3.14},
		{"0.5", 0.5},
		{"1.0", 1.0},
		{"1e10", 1e10},
		{"1.5e-3", 1.5e-3},
		{"2E5", 2e5},
	}
	for _, tt := range tests {
		result := ParseNumber(tt.input)
		lit, ok := result.(*typ.Literal)

		if !ok {
			t.Errorf("ParseNumber(%q) should return Literal, got %T", tt.input, result)
			continue
		}

		if lit.Base != kind.Number {
			t.Errorf("ParseNumber(%q) should have Base Number, got %v", tt.input, lit.Base)
			continue
		}

		val, ok := lit.Value.(float64)
		if !ok {
			t.Errorf("ParseNumber(%q) value should be float64, got %T", tt.input, lit.Value)
			continue
		}

		if val != tt.want {
			t.Errorf("ParseNumber(%q) = %f, want %f", tt.input, val, tt.want)
		}
	}
}

func TestParseNumber_Hex(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0x0", 0},
		{"0x10", 16},
		{"0xFF", 255},
		{"0Xff", 255},
		{"0X1A", 26},
		{"0xDEADBEEF", 0xDEADBEEF},
	}
	for _, tt := range tests {
		result := ParseNumber(tt.input)
		lit, ok := result.(*typ.Literal)

		if !ok {
			t.Errorf("ParseNumber(%q) should return Literal, got %T", tt.input, result)
			continue
		}

		if lit.Base != kind.Integer {
			t.Errorf("ParseNumber(%q) should have Base Integer, got %v", tt.input, lit.Base)
			continue
		}

		val, ok := lit.Value.(int64)
		if !ok {
			t.Errorf("ParseNumber(%q) value should be int64, got %T", tt.input, lit.Value)
			continue
		}

		if val != tt.want {
			t.Errorf("ParseNumber(%q) = %d, want %d", tt.input, val, tt.want)
		}
	}
}

func TestParseNumber_Invalid(t *testing.T) {
	result := ParseNumber("not a number")
	if result != typ.Number {
		t.Error("invalid number should return typ.Number")
	}
}

func TestParseNumber_InvalidHex(t *testing.T) {
	result := ParseNumber("0xGGG")
	if result != typ.Number {
		t.Error("invalid hex should return typ.Number")
	}
}

func TestParseNumber_HexFloat(t *testing.T) {
	result := ParseNumber("0x1p2")
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("ParseNumber(hex float) should return Literal, got %T", result)
	}
	if lit.Base != kind.Number {
		t.Fatalf("ParseNumber(hex float) should have Base Number, got %v", lit.Base)
	}
	val, ok := lit.Value.(float64)
	if !ok || val != 4 {
		t.Fatalf("ParseNumber(hex float) value = %v, want 4.0", lit.Value)
	}
}

func TestIsIntegerLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"0", true},
		{"42", true},
		{"0x10", true},
		{"0XFF", true},
		{"08", true},
		{"0x1p2", false},
		{"3.14", false},
		{"1.0", false},
		{"1e5", false},
		{"1E10", false},
		{"abc", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsIntegerLiteral(tt.input)
		if got != tt.want {
			t.Errorf("IsIntegerLiteral(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseNumberValue_Integer(t *testing.T) {
	tests := []struct {
		input   string
		wantVal float64
		wantInt bool
	}{
		{"0", 0, true},
		{"42", 42, true},
		{"0x10", 16, true},
		{"0xFF", 255, true},
		{"08", 8, true},
	}
	for _, tt := range tests {
		val, isInt := ParseNumberValue(tt.input)
		if val != tt.wantVal {
			t.Errorf("ParseNumberValue(%q) value = %f, want %f", tt.input, val, tt.wantVal)
		}

		if isInt != tt.wantInt {
			t.Errorf("ParseNumberValue(%q) isInt = %v, want %v", tt.input, isInt, tt.wantInt)
		}
	}
}

func TestParseNumberValue_Float(t *testing.T) {
	tests := []struct {
		input   string
		wantVal float64
	}{
		{"3.14", 3.14},
		{"0.5", 0.5},
		{"1e10", 1e10},
		{"0x1p2", 4},
	}
	for _, tt := range tests {
		val, isInt := ParseNumberValue(tt.input)
		if val != tt.wantVal {
			t.Errorf("ParseNumberValue(%q) value = %f, want %f", tt.input, val, tt.wantVal)
		}

		if isInt {
			t.Errorf("ParseNumberValue(%q) should not be int", tt.input)
		}
	}
}

func TestParseNumberValue_Invalid(t *testing.T) {
	val, isInt := ParseNumberValue("not a number")
	if val != 0 {
		t.Error("invalid should return 0")
	}

	if isInt {
		t.Error("invalid should return false for isInt")
	}
}

func TestParseNumber_WholeFloatIsInt(t *testing.T) {
	// "5" should be integer
	result := ParseNumber("5")

	lit, ok := result.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Error("'5' should be parsed as integer literal")
	}

	// "5.0" should be float (has decimal point)
	result = ParseNumber("5.0")

	lit, ok = result.(*typ.Literal)
	if !ok || lit.Base != kind.Number {
		t.Error("'5.0' should be parsed as float literal")
	}

	// "5e0" should be float (has exponent)
	result = ParseNumber("5e0")

	lit, ok = result.(*typ.Literal)
	if !ok || lit.Base != kind.Number {
		t.Error("'5e0' should be parsed as float literal")
	}
}

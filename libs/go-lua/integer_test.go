package lua

import (
	"testing"
)

func TestLInteger_Type(t *testing.T) {
	var v LValue = LInteger(42)
	if v.Type() != LTInteger {
		t.Errorf("expected LTInteger, got %v", v.Type())
	}
}

func TestLInteger_String(t *testing.T) {
	i := LInteger(12345)
	if i.String() != "12345" {
		t.Errorf("expected '12345', got '%s'", i.String())
	}

	i = LInteger(-999)
	if i.String() != "-999" {
		t.Errorf("expected '-999', got '%s'", i.String())
	}
}

func TestLInteger_TypeName(t *testing.T) {
	if LTInteger.String() != "number" {
		t.Errorf("expected 'number', got '%s'", LTInteger.String())
	}
}

func TestLInteger_Preloads(t *testing.T) {
	// Values in [-256, 256) should be preloaded (zero alloc)
	for i := int64(-256); i < 256; i++ {
		v := lintegerToValue(LInteger(i))
		if v.Type() != LTInteger {
			t.Errorf("LInteger(%d) should have type LTInteger", i)
		}
		if int64(v.(LInteger)) != i {
			t.Errorf("expected %d, got %d", i, v.(LInteger))
		}
	}
}

func TestLInteger_LargeValues(t *testing.T) {
	// Values outside preload range should also work
	large := []int64{1000, -1000, 1 << 62, -(1 << 62)}
	for _, i := range large {
		v := lintegerToValue(LInteger(i))
		if v.Type() != LTInteger {
			t.Errorf("LInteger(%d) should have type LTInteger", i)
		}
		if int64(v.(LInteger)) != i {
			t.Errorf("expected %d, got %d", i, v.(LInteger))
		}
	}
}

func TestLVAsNumber_Integer(t *testing.T) {
	// LVAsNumber should convert LInteger to LNumber
	v := LInteger(42)
	n := LVAsNumber(v)
	if n != LNumber(42) {
		t.Errorf("expected 42, got %v", n)
	}
}

func TestLVCanConvToString_Integer(t *testing.T) {
	if !LVCanConvToString(LInteger(42)) {
		t.Error("LInteger should be convertible to string")
	}
}

func TestLVAsString_Integer(t *testing.T) {
	s := LVAsString(LInteger(42))
	if s != "42" {
		t.Errorf("expected '42', got '%s'", s)
	}
}

func TestParseNumberValue_Integers(t *testing.T) {
	tests := []struct {
		input    string
		wantType LValueType
		wantVal  int64
	}{
		{"123", LTInteger, 123},
		{"-456", LTInteger, -456},
		{"0", LTInteger, 0},
		{"0xff", LTInteger, 255},
		{"0xFF", LTInteger, 255},
		{"0x10", LTInteger, 16},
	}

	for _, tc := range tests {
		v, err := parseNumberValue(tc.input)
		if err != nil {
			t.Errorf("parseNumberValue(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if v.Type() != tc.wantType {
			t.Errorf("parseNumberValue(%q): got type %v, want %v", tc.input, v.Type(), tc.wantType)
		}
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.wantVal {
				t.Errorf("parseNumberValue(%q): got %d, want %d", tc.input, i, tc.wantVal)
			}
		}
	}
}

func TestParseNumberValue_Floats(t *testing.T) {
	tests := []struct {
		input   string
		wantVal float64
	}{
		{"123.0", 123.0},
		{"123.456", 123.456},
		{".5", 0.5},
		{"1e10", 1e10},
		{"1E10", 1e10},
		{"1.5e2", 150.0},
	}

	for _, tc := range tests {
		v, err := parseNumberValue(tc.input)
		if err != nil {
			t.Errorf("parseNumberValue(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if v.Type() != LTNumber {
			t.Errorf("parseNumberValue(%q): got type %v, want LTNumber", tc.input, v.Type())
		}
		if n, ok := v.(LNumber); ok {
			if float64(n) != tc.wantVal {
				t.Errorf("parseNumberValue(%q): got %v, want %v", tc.input, n, tc.wantVal)
			}
		}
	}
}

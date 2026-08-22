package pathkey

import (
	"math"
	"strconv"
	"testing"
)

func TestParseIntLiteral_Empty(t *testing.T) {
	_, ok := ParseIntLiteral("")
	if ok {
		t.Error("expected false for empty string")
	}
}

func TestParseIntLiteral_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"123", 123},
		{"9999", 9999},
	}
	for _, tc := range tests {
		got, ok := ParseIntLiteral(tc.input)
		if !ok {
			t.Errorf("ParseIntLiteral(%q) returned false", tc.input)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseIntLiteral(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestParseIntLiteral_Invalid(t *testing.T) {
	tests := []string{
		"-1",
		"abc",
		"12a",
		"a12",
		" 1",
		"1 ",
		"1.0",
	}
	for _, input := range tests {
		_, ok := ParseIntLiteral(input)
		if ok {
			t.Errorf("ParseIntLiteral(%q) should return false", input)
		}
	}
}

func TestParseIntLiteral_Overflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	maxStr := strconv.FormatInt(int64(maxInt), 10)
	if got, ok := ParseIntLiteral(maxStr); !ok || got != maxInt {
		t.Fatalf("ParseIntLiteral(maxInt) = (%d, %v), want (%d, true)", got, ok, maxInt)
	}

	overflow := maxStr + "0"
	if _, ok := ParseIntLiteral(overflow); ok {
		t.Fatalf("ParseIntLiteral(%q) should return false on overflow", overflow)
	}
}

func TestIsIdentStart(t *testing.T) {
	valid := []byte{'_', 'a', 'z', 'A', 'Z'}
	for _, ch := range valid {
		if !IsIdentStart(ch) {
			t.Errorf("IsIdentStart(%q) should be true", ch)
		}
	}

	invalid := []byte{'0', '9', '-', ' ', '.'}
	for _, ch := range invalid {
		if IsIdentStart(ch) {
			t.Errorf("IsIdentStart(%q) should be false", ch)
		}
	}
}

func TestIsIdentPart(t *testing.T) {
	valid := []byte{'_', 'a', 'z', 'A', 'Z', '0', '9'}
	for _, ch := range valid {
		if !IsIdentPart(ch) {
			t.Errorf("IsIdentPart(%q) should be true", ch)
		}
	}

	invalid := []byte{'-', ' ', '.', '@'}
	for _, ch := range invalid {
		if IsIdentPart(ch) {
			t.Errorf("IsIdentPart(%q) should be false", ch)
		}
	}
}

func TestReadIdent_NilIdx(t *testing.T) {
	result := ReadIdent("foo", nil)
	if result != "" {
		t.Errorf("expected empty for nil idx, got %q", result)
	}
}

func TestReadIdent_Valid(t *testing.T) {
	tests := []struct {
		s       string
		start   int
		want    string
		wantIdx int
	}{
		{"foo", 0, "foo", 3},
		{"_bar", 0, "_bar", 4},
		{"foo123", 0, "foo123", 6},
		{"x.y", 0, "x", 1},
		{"abc.def", 4, "def", 7},
	}
	for _, tc := range tests {
		idx := tc.start
		got := ReadIdent(tc.s, &idx)
		if got != tc.want {
			t.Errorf("ReadIdent(%q, %d) = %q, want %q", tc.s, tc.start, got, tc.want)
		}
		if idx != tc.wantIdx {
			t.Errorf("ReadIdent(%q, %d) idx = %d, want %d", tc.s, tc.start, idx, tc.wantIdx)
		}
	}
}

func TestReadIdent_Invalid(t *testing.T) {
	idx := 0
	result := ReadIdent("123abc", &idx)
	if result != "" {
		t.Errorf("expected empty for numeric start, got %q", result)
	}
	if idx != 0 {
		t.Errorf("idx should not advance, got %d", idx)
	}
}

func TestReadIdent_BeyondLength(t *testing.T) {
	idx := 10
	result := ReadIdent("foo", &idx)
	if result != "" {
		t.Errorf("expected empty for idx beyond length, got %q", result)
	}
}

func TestIsIdentName(t *testing.T) {
	valid := []string{"foo", "_bar", "A1", "camelCase", "snake_case"}
	for _, s := range valid {
		if !IsIdentName(s) {
			t.Errorf("IsIdentName(%q) should be true", s)
		}
	}

	invalid := []string{"", "1abc", "-foo", "a-b", "foo.bar"}
	for _, s := range invalid {
		if IsIdentName(s) {
			t.Errorf("IsIdentName(%q) should be false", s)
		}
	}
}

func TestIntToString(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-1, "-1"},
		{-42, "-42"},
		{12345, "12345"},
		{maxInt, strconv.FormatInt(int64(maxInt), 10)},
		{minInt, strconv.FormatInt(int64(minInt), 10)},
	}
	for _, tc := range tests {
		got := IntToString(tc.input)
		if got != tc.want {
			t.Errorf("IntToString(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFloatToSafeInt_WholeNumbers(t *testing.T) {
	tests := []struct {
		input float64
		want  int
	}{
		{0.0, 0},
		{1.0, 1},
		{-1.0, -1},
		{42.0, 42},
		{-42.0, -42},
	}
	for _, tc := range tests {
		got, ok := FloatToSafeInt(tc.input)
		if !ok {
			t.Errorf("FloatToSafeInt(%v) returned false", tc.input)
			continue
		}
		if got != tc.want {
			t.Errorf("FloatToSafeInt(%v) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestFloatToSafeInt_Fractional(t *testing.T) {
	tests := []float64{0.5, 1.1, -1.5, 3.14}
	for _, f := range tests {
		_, ok := FloatToSafeInt(f)
		if ok {
			t.Errorf("FloatToSafeInt(%v) should return false for fractional", f)
		}
	}
}

func TestFloatToSafeInt_BeyondPrecision(t *testing.T) {
	huge := float64(MaxSafeFloat64Int + 1)
	_, ok := FloatToSafeInt(huge)
	if ok {
		t.Error("FloatToSafeInt should return false for values beyond safe precision")
	}
}

func TestFloatToSafeInt_NaNOrInf(t *testing.T) {
	tests := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, f := range tests {
		if _, ok := FloatToSafeInt(f); ok {
			t.Errorf("FloatToSafeInt(%v) should return false", f)
		}
	}
}

func TestFloatToSafeInt_IntRange(t *testing.T) {
	if strconv.IntSize == 32 {
		if _, ok := FloatToSafeInt(2147483648.0); ok {
			t.Fatal("FloatToSafeInt should reject values outside int32 range")
		}
	}
}

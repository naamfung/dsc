package validate

import "testing"

func TestValidateMinLen(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		arg     any
		wantErr bool
	}{
		{"string above min", "hello", 3, false},
		{"string at min", "abc", 3, false},
		{"string below min", "ab", 3, true},
		{"empty string at zero", "", 0, false},
		{"empty string below min", "", 1, true},
		{"int64 arg", "hello", int64(3), false},
		{"float arg", "hello", 3.0, false},
		{"number skipped", 123, 3, false},
		{"nil skipped", nil, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMinLen(tt.val, tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMinLen() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && err.Constraint != "min_len" {
				t.Errorf("constraint = %q, want 'min_len'", err.Constraint)
			}
		})
	}
}

func TestValidateMaxLen(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		arg     any
		wantErr bool
	}{
		{"string below max", "ab", 3, false},
		{"string at max", "abc", 3, false},
		{"string above max", "abcd", 3, true},
		{"empty string at zero", "", 0, false},
		{"int64 arg", "ab", int64(3), false},
		{"number skipped", 123, 3, false},
		{"nil skipped", nil, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMaxLen(tt.val, tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMaxLen() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && err.Constraint != "max_len" {
				t.Errorf("constraint = %q, want 'max_len'", err.Constraint)
			}
		})
	}
}

// Mock types for interface testing
type mockLen int

func (m mockLen) Len() int { return int(m) }

type mockStr string

func (m mockStr) String() string { return string(m) }

func TestGetLength(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   int
		wantOk bool
	}{
		{"string", "hello", 5, true},
		{"empty string", "", 0, true},
		{"lengther", mockLen(10), 10, true},
		{"stringer", mockStr("abc"), 3, true},
		{"int", 42, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := getLength(tt.val)
			if ok != tt.wantOk {
				t.Errorf("getLength() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && got != tt.want {
				t.Errorf("getLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateMinLenWithInterfaces(t *testing.T) {
	err := validateMinLen(mockLen(5), 3)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	err = validateMinLen(mockLen(2), 3)
	if err == nil {
		t.Error("expected error")
	}

	err = validateMinLen(mockStr("hello"), 3)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func BenchmarkValidateMinLen(b *testing.B) {
	s := "hello world"
	for i := 0; i < b.N; i++ {
		_ = validateMinLen(s, 5)
	}
}

func BenchmarkValidateMaxLen(b *testing.B) {
	s := "hello world"
	for i := 0; i < b.N; i++ {
		_ = validateMaxLen(s, 100)
	}
}

func TestValidateMinLenNegativeConstraint(t *testing.T) {
	err := validateMinLen("test", -1)
	if err != nil {
		t.Error("expected nil for negative constraint")
	}
}

func TestValidateMaxLenNegativeConstraint(t *testing.T) {
	err := validateMaxLen("test", -1)
	if err != nil {
		t.Error("expected nil for negative constraint")
	}
}

func TestToIntEdgeCases(t *testing.T) {
	tests := []struct {
		val  any
		want int
	}{
		{42, 42},
		{int64(100), 100},
		{int32(50), 50},
		{3.9, 3},
		{float32(2.9), 2},
		{"string", 0},
		{nil, 0},
		{true, 0},
		{[]int{1, 2}, 0},
	}

	for _, tt := range tests {
		got := toInt(tt.val)
		if got != tt.want {
			t.Errorf("toInt(%v) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestValidateMinLenErrorFields(t *testing.T) {
	err := validateMinLen("ab", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, ok := err.Got.(string); !ok || got != "ab" {
		t.Errorf("Got = %v, want 'ab'", err.Got)
	}
	if err.Expected != 5 {
		t.Errorf("Expected = %v, want 5", err.Expected)
	}
}

func TestValidateMaxLenErrorFields(t *testing.T) {
	err := validateMaxLen("abcdefgh", 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, ok := err.Got.(string); !ok || got != "abcdefgh" {
		t.Errorf("Got = %v, want 'abcdefgh'", err.Got)
	}
	if err.Expected != 3 {
		t.Errorf("Expected = %v, want 3", err.Expected)
	}
}

func TestValidateMaxLenWithInterfaces(t *testing.T) {
	err := validateMaxLen(mockLen(2), 5)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	err = validateMaxLen(mockLen(10), 5)
	if err == nil {
		t.Error("expected error")
	}

	err = validateMaxLen(mockStr("ab"), 5)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	err = validateMaxLen(mockStr("abcdefghij"), 5)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetLengthZeroLen(t *testing.T) {
	got, ok := getLength(mockLen(0))
	if !ok {
		t.Error("expected ok for zero length")
	}
	if got != 0 {
		t.Errorf("got = %d, want 0", got)
	}
}

func TestValidateLenWithFloatArg(t *testing.T) {
	err := validateMinLen("hello", 3.7)
	if err != nil {
		t.Error("expected no error for float arg truncated to 3")
	}

	err = validateMinLen("ab", 3.7)
	if err == nil {
		t.Error("expected error")
	}
}

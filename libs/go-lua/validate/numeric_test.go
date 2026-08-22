package validate

import "testing"

func TestValidateMin(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		arg     any
		wantErr bool
	}{
		{"float above min", 10.0, 5.0, false},
		{"float at min", 5.0, 5.0, false},
		{"float below min", 3.0, 5.0, true},
		{"int above min", 10, 5, false},
		{"int below min", 3, 5, true},
		{"int64 above min", int64(10), int64(5), false},
		{"int64 below min", int64(3), int64(5), true},
		{"negative ok", -3.0, -5.0, false},
		{"negative fail", -10.0, -5.0, true},
		{"zero at min", 0.0, 0.0, false},
		{"string skipped", "hello", 5, false},
		{"nil skipped", nil, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMin(tt.val, tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMin() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && err.Constraint != "min" {
				t.Errorf("constraint = %q, want 'min'", err.Constraint)
			}
		})
	}
}

func TestValidateMax(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		arg     any
		wantErr bool
	}{
		{"float below max", 3.0, 5.0, false},
		{"float at max", 5.0, 5.0, false},
		{"float above max", 10.0, 5.0, true},
		{"int below max", 3, 5, false},
		{"int above max", 10, 5, true},
		{"int64 below max", int64(3), int64(5), false},
		{"int64 above max", int64(10), int64(5), true},
		{"negative ok", -10.0, -5.0, false},
		{"negative fail", -3.0, -5.0, true},
		{"string skipped", "hello", 5, false},
		{"nil skipped", nil, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMax(tt.val, tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMax() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && err.Constraint != "max" {
				t.Errorf("constraint = %q, want 'max'", err.Constraint)
			}
		})
	}
}

func TestToNumber(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   float64
		wantOk bool
	}{
		{"float64", 3.14, 3.14, true},
		{"int", 42, 42.0, true},
		{"int64", int64(100), 100.0, true},
		{"string", "hello", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toNumber(tt.val)
			if ok != tt.wantOk {
				t.Errorf("toNumber() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && got != tt.want {
				t.Errorf("toNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		val  any
		want float64
	}{
		{3.14, 3.14},
		{float32(2.5), 2.5},
		{42, 42.0},
		{int64(100), 100.0},
		{int32(50), 50.0},
		{"string", 0},
		{nil, 0},
	}

	for _, tt := range tests {
		got := toFloat(tt.val)
		if got != tt.want {
			t.Errorf("toFloat(%v) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

// Mock types to test interface-based conversion
type mockFloat float64

func (m mockFloat) Float64() float64 { return float64(m) }

type mockInt int64

func (m mockInt) Int64() int64 { return int64(m) }

func TestValidateMinWithInterfaces(t *testing.T) {
	err := validateMin(mockFloat(10), 5.0)
	if err != nil {
		t.Errorf("expected no error for mockFloat(10) >= 5, got %v", err)
	}

	err = validateMin(mockFloat(3), 5.0)
	if err == nil {
		t.Error("expected error for mockFloat(3) < 5")
	}

	err = validateMin(mockInt(10), 5.0)
	if err != nil {
		t.Errorf("expected no error for mockInt(10) >= 5, got %v", err)
	}

	err = validateMin(mockInt(3), 5.0)
	if err == nil {
		t.Error("expected error for mockInt(3) < 5")
	}
}

func TestValidateMaxWithInterfaces(t *testing.T) {
	err := validateMax(mockFloat(3), 5.0)
	if err != nil {
		t.Errorf("expected no error for mockFloat(3) <= 5, got %v", err)
	}

	err = validateMax(mockFloat(10), 5.0)
	if err == nil {
		t.Error("expected error for mockFloat(10) > 5")
	}
}

func BenchmarkValidateMin(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = validateMin(42.0, 10.0)
	}
}

func BenchmarkValidateMax(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = validateMax(42.0, 100.0)
	}
}

func TestAsFloat64(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   float64
		wantOk bool
	}{
		{"float64er", mockFloat(3.14), 3.14, true},
		{"string", "test", 0, false},
		{"int", 42, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asFloat64(tt.val)
			if ok != tt.wantOk {
				t.Errorf("asFloat64() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && got != tt.want {
				t.Errorf("asFloat64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAsInt64(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   int64
		wantOk bool
	}{
		{"int64er", mockInt(100), 100, true},
		{"string", "test", 0, false},
		{"float", 3.14, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asInt64(tt.val)
			if ok != tt.wantOk {
				t.Errorf("asInt64() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && got != tt.want {
				t.Errorf("asInt64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateMinErrorFields(t *testing.T) {
	err := validateMin(3.0, 5.0)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Got != 3.0 {
		t.Errorf("Got = %v, want 3.0", err.Got)
	}
	if err.Expected != 5.0 {
		t.Errorf("Expected = %v, want 5.0", err.Expected)
	}
}

func TestValidateMaxErrorFields(t *testing.T) {
	err := validateMax(10.0, 5.0)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Got != 10.0 {
		t.Errorf("Got = %v, want 10.0", err.Got)
	}
	if err.Expected != 5.0 {
		t.Errorf("Expected = %v, want 5.0", err.Expected)
	}
}

func TestValidateMinZeroArg(t *testing.T) {
	err := validateMin(0.0, 0.0)
	if err != nil {
		t.Error("expected no error for value at zero min")
	}

	err = validateMin(-0.001, 0.0)
	if err == nil {
		t.Error("expected error for negative value with zero min")
	}
}

func TestValidateMaxZeroArg(t *testing.T) {
	err := validateMax(0.0, 0.0)
	if err != nil {
		t.Error("expected no error for value at zero max")
	}

	err = validateMax(0.001, 0.0)
	if err == nil {
		t.Error("expected error for positive value with zero max")
	}
}

func TestToFloatEdgeCases(t *testing.T) {
	tests := []struct {
		val  any
		want float64
	}{
		{uint(10), 0},
		{uint64(10), 0},
		{true, 0},
		{complex(1, 2), 0},
	}

	for _, tt := range tests {
		got := toFloat(tt.val)
		if got != tt.want {
			t.Errorf("toFloat(%T) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestToNumberWithBothInterfaces(t *testing.T) {
	got, ok := toNumber(mockFloat(3.14))
	if !ok {
		t.Error("expected ok for mockFloat")
	}
	if got != 3.14 {
		t.Errorf("got = %v, want 3.14", got)
	}

	got, ok = toNumber(mockInt(42))
	if !ok {
		t.Error("expected ok for mockInt")
	}
	if got != 42.0 {
		t.Errorf("got = %v, want 42.0", got)
	}
}

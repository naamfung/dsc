package lua

import (
	"math"
	"testing"
)

func TestIsIntegerValue(t *testing.T) {
	tests := []struct {
		input    LNumber
		expected bool
	}{
		{0, true},
		{1, true},
		{-1, true},
		{100, true},
		{255, true},
		{256, true},
		{1000, true},
		{math.MaxInt32, true},
		{math.MinInt32, true},
		{0.5, false},
		{1.1, false},
		{-0.5, false},
		{math.Pi, false},
		{1.0000001, false},
	}

	for _, tt := range tests {
		result := IsIntegerValue(tt.input)
		if result != tt.expected {
			t.Errorf("IsIntegerValue(%v) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestIsArrayKey(t *testing.T) {
	tests := []struct {
		input    LNumber
		expected bool
	}{
		{1, true},
		{2, true},
		{100, true},
		{LNumber(MaxArrayIndex - 1), true},
		{0, false},
		{-1, false},
		{-100, false},
		{0.5, false},
		{1.5, false},
		{LNumber(MaxArrayIndex), false},
		{LNumber(MaxArrayIndex + 1), false},
	}

	for _, tt := range tests {
		result := isArrayKey(tt.input)
		if result != tt.expected {
			t.Errorf("isArrayKey(%v) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestLNumber2IPreload(t *testing.T) {
	for i := 0; i < preloadLimit; i++ {
		v := lnumberToValue(LNumber(i))
		if v != preloadedNumbers[i] {
			t.Errorf("lnumberToValue(%d) did not return preloaded value", i)
		}
	}

	v := lnumberToValue(LNumber(preloadLimit))
	if v == preloadedNumbers[int(preloadLimit)-1] {
		t.Errorf("lnumberToValue(%d) should not return preloaded value", preloadLimit)
	}
}

func TestLNumber2INonInteger(t *testing.T) {
	tests := []LNumber{0.5, 1.5, -0.5, math.Pi, 127.5}
	for _, v := range tests {
		result := lnumberToValue(v)
		if n, ok := result.(LNumber); !ok || n != v {
			t.Errorf("lnumberToValue(%v) returned incorrect value", v)
		}
	}
}

func BenchmarkIsIntegerValue(b *testing.B) {
	values := []LNumber{0, 1, 100, 1000, 0.5, 1.5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range values {
			IsIntegerValue(v)
		}
	}
}

func BenchmarkIsArrayKey(b *testing.B) {
	values := []LNumber{1, 100, 1000, 0, -1, 0.5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range values {
			isArrayKey(v)
		}
	}
}

func TestStrCmp(t *testing.T) {
	tests := []struct {
		s1, s2   string
		expected int
	}{
		{"", "", 0},
		{"a", "a", 0},
		{"abc", "abc", 0},
		{"a", "b", -1},
		{"b", "a", 1},
		{"abc", "abd", -1},
		{"abd", "abc", 1},
		{"abc", "ab", 1},  // s1 longer
		{"ab", "abc", -1}, // s2 longer
		{"a", "abc", -1},  // s2 much longer
		{"abc", "a", 1},   // s1 much longer
		{"", "a", -1},     // s1 empty
		{"a", "", 1},      // s2 empty
	}

	for _, tt := range tests {
		result := strCmp(tt.s1, tt.s2)
		if result != tt.expected {
			t.Errorf("strCmp(%q, %q) = %d, expected %d", tt.s1, tt.s2, result, tt.expected)
		}
	}
}

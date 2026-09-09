package internal

import (
	"math"
	"testing"
)

func TestSafeAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b   int64
		want   int64
		wantOk bool
	}{
		{1, 2, 3, true},
		{0, 0, 0, true},
		{-1, 1, 0, true},
		{math.MaxInt64, 0, math.MaxInt64, true},
		{math.MaxInt64, 1, 0, false},
		{math.MaxInt64, math.MaxInt64, 0, false},
		{math.MinInt64, 0, math.MinInt64, true},
		{math.MinInt64, -1, 0, false},
		{math.MinInt64, 1, math.MinInt64 + 1, true},
	}

	for _, testCase := range tests {
		got, ok := SafeAdd(testCase.a, testCase.b)
		if ok != testCase.wantOk || (ok && got != testCase.want) {
			t.Errorf("SafeAdd(%d, %d) = (%d, %v), want (%d, %v)",
				testCase.a, testCase.b, got, ok, testCase.want, testCase.wantOk)
		}
	}
}

func TestSafeSub(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b   int64
		want   int64
		wantOk bool
	}{
		{3, 2, 1, true},
		{0, 0, 0, true},
		{-1, -1, 0, true},
		{math.MaxInt64, 0, math.MaxInt64, true},
		{math.MaxInt64, -1, 0, false},
		{math.MinInt64, 0, math.MinInt64, true},
		{math.MinInt64, 1, 0, false},
		{math.MinInt64, -1, math.MinInt64 + 1, true},
	}

	for _, testCase := range tests {
		got, ok := SafeSub(testCase.a, testCase.b)
		if ok != testCase.wantOk || (ok && got != testCase.want) {
			t.Errorf("SafeSub(%d, %d) = (%d, %v), want (%d, %v)",
				testCase.a, testCase.b, got, ok, testCase.want, testCase.wantOk)
		}
	}
}

func TestSafeMul(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b   int64
		want   int64
		wantOk bool
	}{
		{2, 3, 6, true},
		{0, math.MaxInt64, 0, true},
		{math.MaxInt64, 0, 0, true},
		{1, math.MaxInt64, math.MaxInt64, true},
		{2, math.MaxInt64, 0, false},
		{math.MaxInt64, 2, 0, false},
		{-1, math.MinInt64, 0, false},
		{math.MinInt64, -1, 0, false},
		{-1, -1, 1, true},
		{-2, 3, -6, true},
	}

	for _, testCase := range tests {
		got, ok := SafeMul(testCase.a, testCase.b)
		if ok != testCase.wantOk || (ok && got != testCase.want) {
			t.Errorf("SafeMul(%d, %d) = (%d, %v), want (%d, %v)",
				testCase.a, testCase.b, got, ok, testCase.want, testCase.wantOk)
		}
	}
}

func TestSafeNeg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a      int64
		want   int64
		wantOk bool
	}{
		{0, 0, true},
		{1, -1, true},
		{-1, 1, true},
		{math.MaxInt64, -math.MaxInt64, true},
		{math.MinInt64, 0, false},
	}

	for _, testCase := range tests {
		got, ok := SafeNeg(testCase.a)
		if ok != testCase.wantOk || (ok && got != testCase.want) {
			t.Errorf("SafeNeg(%d) = (%d, %v), want (%d, %v)",
				testCase.a, got, ok, testCase.want, testCase.wantOk)
		}
	}
}

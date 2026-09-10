package internal

import "math"

// Int64 bounds for overflow detection.
const (
	MinInt64 = math.MinInt64
	MaxInt64 = math.MaxInt64
)

// SafeAdd returns (a + b, true) if no overflow, (0, false) otherwise.
func SafeAdd(a, b int64) (int64, bool) {
	if b > 0 && a > MaxInt64-b {
		return 0, false
	}

	if b < 0 && a < MinInt64-b {
		return 0, false
	}

	return a + b, true
}

// SafeSub returns (a - b, true) if no overflow, (0, false) otherwise.
func SafeSub(a, b int64) (int64, bool) {
	if b < 0 && a > MaxInt64+b {
		return 0, false
	}

	if b > 0 && a < MinInt64+b {
		return 0, false
	}

	return a - b, true
}

// SafeMul returns (a * b, true) if no overflow, (0, false) otherwise.
func SafeMul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}

	if a == -1 && b == MinInt64 {
		return 0, false
	}

	if b == -1 && a == MinInt64 {
		return 0, false
	}

	result := a * b

	if result/a != b {
		return 0, false
	}

	return result, true
}

// SafeNeg returns (-a, true) if no overflow, (0, false) otherwise.
func SafeNeg(a int64) (int64, bool) {
	if a == MinInt64 {
		return 0, false
	}

	return -a, true
}

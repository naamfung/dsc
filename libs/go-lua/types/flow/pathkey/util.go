// util.go provides parsing utilities for path key construction.
//
// These functions handle low-level parsing of identifiers and numbers
// in path syntax, enabling path key parsing and construction.
package pathkey

import (
	"math"

	"github.com/wippyai/go-lua/types/numparse"
)

// ParseIntLiteral parses a string as a non-negative integer literal.
//
// Returns the integer value and true if the string contains only ASCII digits.
// Returns (0, false) for empty strings, negative numbers, or non-digit characters.
//
// This is used to distinguish integer indices ([0], [1]) from string indices
// ([key]) in path suffix parsing.
func ParseIntLiteral(s string) (int, bool) {
	return numparse.ParseNonNegativeDecimalInt(s)
}

// IsIdentStart reports whether ch can start a Lua identifier.
//
// In Lua, identifiers must start with a letter (A-Z, a-z) or underscore (_).
// This matches Lua's lexical rules for variable names.
func IsIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

// IsIdentPart reports whether ch can appear in a Lua identifier (after the first character).
//
// In Lua, identifiers can contain letters, digits (0-9), and underscores.
// The first character must satisfy IsIdentStart; subsequent characters satisfy IsIdentPart.
func IsIdentPart(ch byte) bool {
	return IsIdentStart(ch) || (ch >= '0' && ch <= '9')
}

// ReadIdent reads a Lua identifier from s starting at *idx, advancing *idx past it.
//
// An identifier starts with IsIdentStart and continues with IsIdentPart characters.
// The function modifies *idx in place to point past the consumed identifier.
//
// Returns the identifier string, or empty string if no valid identifier starts at *idx.
// If idx is nil or out of bounds, returns empty string without modifying idx.
func ReadIdent(s string, idx *int) string {
	if idx == nil || *idx >= len(s) {
		return ""
	}

	start := *idx

	ch := s[*idx]
	if !IsIdentStart(ch) {
		return ""
	}

	*idx++
	for *idx < len(s) {
		ch = s[*idx]
		if !IsIdentPart(ch) {
			break
		}
		*idx++
	}

	return s[start:*idx]
}

// IsIdentName reports whether s is a valid Lua identifier name.
//
// A valid identifier is non-empty, starts with IsIdentStart, and all subsequent
// characters satisfy IsIdentPart. This validates variable names, field names, etc.
func IsIdentName(s string) bool {
	if s == "" {
		return false
	}

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if i == 0 {
			if !IsIdentStart(ch) {
				return false
			}
		} else if !IsIdentPart(ch) {
			return false
		}
	}

	return true
}

// IntToString converts an integer to its string representation.
//
// This is a simple implementation without strconv for hot paths.
func IntToString(v int) string {
	if v == 0 {
		return "0"
	}

	neg := v < 0
	var u uint
	if neg {
		// Two's-complement absolute value; safe for MinInt.
		u = uint(^v) + 1
	} else {
		u = uint(v)
	}

	var buf [20]byte

	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// MaxSafeFloat64Int is the largest integer magnitude that can be exactly represented in float64.
//
// IEEE 754 double precision has a 53-bit significand, so integers up to 2^53 - 1
// can be represented exactly. Beyond this, consecutive integers may round to the
// same float64 value, causing precision loss.
//
// This constant is used to validate integer conversions from Lua numbers, which
// are all represented as float64 internally.
const MaxSafeFloat64Int = (1 << 53) - 1

// FloatToSafeInt converts a float64 to int if it represents a whole number safely.
//
// Returns (value, true) if:
//   - The float has no fractional part (float64(int64(f)) == f)
//   - The magnitude is within MaxSafeFloat64Int
//
// Returns (0, false) if the conversion would lose precision or the value is
// not an exact integer. This is used for array index validation where only
// exact integers are valid indices.
func FloatToSafeInt(f float64) (int, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if f > MaxSafeFloat64Int || f < -MaxSafeFloat64Int {
		return 0, false
	}
	if math.Trunc(f) != f {
		return 0, false
	}
	i := int64(f)
	maxInt := int64(int(^uint(0) >> 1))
	minInt := -maxInt - 1
	if i > maxInt || i < minInt {
		return 0, false
	}
	return int(i), true
}

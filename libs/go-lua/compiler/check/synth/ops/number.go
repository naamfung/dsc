package ops

import (
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// ParseNumber parses a Lua numeric literal and returns its type.
//
// Distinguishes between integer and float literals to enable integer-specific
// operations (bitwise operators, integer division).
//
// Recognized formats:
//   - Hex: 0x1A, 0XFF -> LiteralInt
//   - Integer: 42, -7 -> LiteralInt
//   - Float: 3.14, 1e10, 2.5e-3 -> LiteralNumber
//
// Returns typ.Number if parsing fails.
func ParseNumber(value string) typ.Type {
	if i, ok := numparse.ParseIntegerLiteral(value); ok {
		return typ.LiteralInt(i)
	}

	f, ok := numparse.ParseFloatLiteral(value)
	if !ok {
		return typ.Number
	}

	return typ.LiteralNumber(f)
}

// IsIntegerLiteral checks if the string represents an integer literal.
func IsIntegerLiteral(value string) bool {
	_, ok := numparse.ParseIntegerLiteral(value)
	return ok
}

// ParseNumberValue extracts the numeric value from a literal string.
// Returns the value and true for integers, or the value and false for floats.
func ParseNumberValue(value string) (float64, bool) {
	if i, ok := numparse.ParseIntegerLiteral(value); ok {
		return float64(i), true
	}

	f, ok := numparse.ParseFloatLiteral(value)
	if !ok {
		return 0, false
	}

	return f, false
}

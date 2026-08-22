package constraint

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/numparse"
)

// ParamPath returns the path for a parameter value.
// Uses the $N format which is recognized by IsPlaceholder() and PlaceholderIndex().
func ParamPath(index int) Path {
	return NewPlaceholder(index)
}

// PlaceholderIndexFromString extracts a placeholder index from canonical forms.
//
// Accepted forms:
//   - "$N" (constraint path placeholder)
//   - "param[N]" (legacy predicate placeholder)
//
// Returns -1 for invalid syntax, negative values, and overflow.
func PlaceholderIndexFromString(s string) int {
	if len(s) >= 2 && s[0] == '$' {
		return parseNonNegativeDecimal(s[1:])
	}
	if strings.HasPrefix(s, "param[") && strings.HasSuffix(s, "]") {
		return parseNonNegativeDecimal(s[6 : len(s)-1])
	}
	return -1
}

func parseNonNegativeDecimal(s string) int {
	value, ok := numparse.ParseNonNegativeDecimalInt(s)
	if !ok {
		return -1
	}
	return value
}

// PlaceholderArgIndex resolves a placeholder path to an argument index.
//
// Returns false when the path is not a placeholder or index is out of bounds.
func PlaceholderArgIndex(path Path, argCount int) (int, bool) {
	if !path.IsPlaceholder() {
		return 0, false
	}
	idx := path.PlaceholderIndex()
	if idx < 0 || idx >= argCount {
		return 0, false
	}
	return idx, true
}

// RetPath returns the path for a return value.
func RetPath(index int) Path {
	if index < 0 {
		return Path{}
	}
	return Path{Root: "ret[" + strconv.Itoa(index) + "]"}
}

// ReturnIndexFromString extracts a return slot index from "ret[N]".
//
// Returns -1 for invalid syntax, negative values, and overflow.
func ReturnIndexFromString(s string) int {
	if strings.HasPrefix(s, "ret[") && strings.HasSuffix(s, "]") {
		return parseNonNegativeDecimal(s[4 : len(s)-1])
	}
	return -1
}

// IsReturnPath checks if the path represents a return value (ret[N] format).
func IsReturnPath(p Path) bool {
	return p.Symbol == 0 && len(p.Segments) == 0 && ReturnIndexFromString(p.Root) >= 0
}

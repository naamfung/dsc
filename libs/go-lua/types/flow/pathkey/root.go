package pathkey

import (
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
)

// ParseRootAndSuffix splits a key into its canonical root and suffix.
//
// Canonical roots are:
//   - Symbol keys: sym<ID> or sym<ID>@<version>
//   - Placeholder roots: $<index>
//   - Return roots: ret[<index>]
//
// For legacy keys that don't match the canonical forms, the function falls
// back to splitting at the first '.' or '[' and returns that prefix as root.
func ParseRootAndSuffix(key constraint.PathKey) (root string, suffix string, ok bool) {
	if sym, version, suffix, ok := ParseKey(key); ok {
		if version != 0 {
			return SymbolVersionRoot(sym, version), suffix, true
		}
		return SymbolRoot(sym), suffix, true
	}

	s := string(key)
	if s == "" {
		return "", "", false
	}

	if s[0] == '$' {
		end := 1
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end > 1 {
			return s[:end], s[end:], true
		}
		return "", "", false
	}

	if strings.HasPrefix(s, "ret[") {
		end := 4
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end > 4 && end < len(s) && s[end] == ']' {
			end++
			return s[:end], s[end:], true
		}
		return "", "", false
	}

	end := 0
	for end < len(s) && s[end] != '.' && s[end] != '[' {
		end++
	}
	if end == 0 {
		return "", "", false
	}
	return s[:end], s[end:], true
}

package cfg

import (
	"sort"

	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// SortedSymbolIDs returns the SymbolIDs from m in ascending order.
func SortedSymbolIDs[T any](m map[basecfg.SymbolID]T) []basecfg.SymbolID {
	if len(m) == 0 {
		return nil
	}
	keys := make([]basecfg.SymbolID, 0, len(m))
	for sym := range m {
		keys = append(keys, sym)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// SortedFieldNames returns the field names from m in ascending order.
func SortedFieldNames[T any](m map[string]T) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for name := range m {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

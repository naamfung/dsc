package constraint

import "sort"

// SortedPathKeys returns PathKeys in deterministic order for map iteration.
func SortedPathKeys[T any](m map[PathKey]T) []PathKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]PathKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

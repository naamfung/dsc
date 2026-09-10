package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// ContainerMutationMerger merges an incoming mutation with an existing mutation
// on the same canonical path key. When prev is nil, next is a new key.
type ContainerMutationMerger func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation

// MergeContainerMutationSlices merges mutation slices by canonical path key.
// Output ordering is deterministic by key.
func MergeContainerMutationSlices(
	existing []api.ContainerMutation,
	next []api.ContainerMutation,
	merge ContainerMutationMerger,
) []api.ContainerMutation {
	if len(existing) == 0 {
		return next
	}
	if len(next) == 0 {
		return existing
	}

	mergeFn := merge
	if mergeFn == nil {
		mergeFn = func(_ *api.ContainerMutation, n api.ContainerMutation) api.ContainerMutation { return n }
	}

	byKey := make(map[string]api.ContainerMutation, len(existing)+len(next))
	for _, m := range existing {
		byKey[api.ContainerMutationKey(m)] = m
	}
	for _, m := range next {
		key := api.ContainerMutationKey(m)
		if prev, ok := byKey[key]; ok {
			merged := mergeFn(&prev, m)
			byKey[key] = merged
			continue
		}
		byKey[key] = mergeFn(nil, m)
	}

	out := make([]api.ContainerMutation, 0, len(byKey))
	for _, key := range cfg.SortedFieldNames(byKey) {
		out = append(out, byKey[key])
	}
	return out
}

// MergeCapturedContainerMutationMaps merges per-symbol captured mutation maps.
func MergeCapturedContainerMutationMaps(
	existing map[cfg.SymbolID][]api.ContainerMutation,
	next map[cfg.SymbolID][]api.ContainerMutation,
	merge ContainerMutationMerger,
) map[cfg.SymbolID][]api.ContainerMutation {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	merged := make(map[cfg.SymbolID][]api.ContainerMutation, len(existing)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(existing) {
		merged[sym] = existing[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		merged[sym] = MergeContainerMutationSlices(merged[sym], next[sym], merge)
	}
	return merged
}

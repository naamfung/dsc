package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldTypeMerger merges an incoming field type with an existing field type.
// When prev is nil, next is a new field value.
type FieldTypeMerger func(prev typ.Type, next typ.Type) typ.Type

// MergeCapturedFieldSymbolMaps merges captured-field maps keyed by captured symbol.
// Structure: capturedSymbol -> fieldName -> fieldType.
func MergeCapturedFieldSymbolMaps(
	existing map[cfg.SymbolID]map[string]typ.Type,
	next map[cfg.SymbolID]map[string]typ.Type,
	merge FieldTypeMerger,
) map[cfg.SymbolID]map[string]typ.Type {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}

	mergeFn := merge
	if mergeFn == nil {
		mergeFn = func(prev typ.Type, n typ.Type) typ.Type {
			if prev != nil {
				return prev
			}
			return n
		}
	}

	merged := make(map[cfg.SymbolID]map[string]typ.Type, len(existing)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(existing) {
		merged[sym] = existing[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		fields := next[sym]
		existingFields := merged[sym]
		if existingFields == nil {
			merged[sym] = fields
			continue
		}
		out := make(map[string]typ.Type, len(existingFields)+len(fields))
		for _, name := range cfg.SortedFieldNames(existingFields) {
			out[name] = existingFields[name]
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			t := fields[name]
			out[name] = mergeFn(out[name], t)
		}
		merged[sym] = out
	}
	return merged
}

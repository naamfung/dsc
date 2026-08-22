// Package join provides type joining operations for phi node merging.
//
// When control flow merges at phi nodes, the join package computes the
// resulting type from multiple incoming branches. The join operation
// produces a type that is a supertype of all incoming types.
//
// # Coalescing Rules
//
// The join package applies several coalescing rules before creating unions:
//
//   - Empty records are removed when maps are present
//   - Open and closed records are unified to open records
//   - Records with identical field signatures merge their map components
//   - Multiple maps merge into a single map with joined key/value types
//   - Unknown types are filtered out (they don't contribute information)
//
// These rules ensure that join results are as precise as possible while
// remaining sound supertypes of all inputs.
//
// # Usage
//
// Call Types with the types from each incoming branch:
//
//	result := join.Types(branchA, branchB, branchC)
//
// For single-type or identical inputs, Types returns the input directly
// without creating unnecessary unions.
package join

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Types computes the union of multiple types for phi node joining.
// Coalesces empty records with container shapes and merges multiple maps into one.
// Filters out unknown types since they don't contribute information to the join.
func Types(types ...typ.Type) typ.Type {
	if len(types) == 0 {
		return nil
	}
	if len(types) == 1 {
		return types[0]
	}

	// Filter out nil and unknown types before processing
	filtered := filterUnknown(types)
	if len(filtered) == 0 {
		// All types were unknown/nil, return unknown
		return typ.Unknown
	}
	if len(filtered) == 1 {
		return filtered[0]
	}

	first := filtered[0]
	allEqual := true
	for i := 1; i < len(filtered); i++ {
		if !typ.TypeEquals(first, filtered[i]) {
			allEqual = false
			break
		}
	}
	if allEqual {
		return first
	}
	filtered = CoalesceEmptyRecordWithMap(filtered)
	filtered = CoalesceEmptyRecordWithArray(filtered)
	filtered = CoalesceRecordOpenness(filtered)
	filtered = CoalesceRecordMapComponents(filtered)
	filtered = CoalesceCompatibleRecords(filtered)
	filtered = CoalesceMaps(filtered)
	if len(filtered) == 1 {
		return filtered[0]
	}
	return typ.PruneSoftUnionMembers(typ.NewUnion(filtered...))
}

// CoalesceEmptyRecordWithArray removes empty records when arrays are present.
//
// Lua uses the same table runtime value for map-like and list-like tables. During
// flow joins, a common pattern is {} on one path and array growth on another
// (table.insert). Keeping {} in the union loses sequence intent and creates
// downstream nil-index noise. When an array shape is present, prefer it.
func CoalesceEmptyRecordWithArray(types []typ.Type) []typ.Type {
	hasEmptyRecord := false
	hasArray := false
	for _, t := range types {
		if unwrap.IsEmptyRecord(t) {
			hasEmptyRecord = true
			continue
		}
		if _, ok := unwrap.Alias(t).(*typ.Array); ok {
			hasArray = true
		}
	}
	if !hasEmptyRecord || !hasArray {
		return types
	}
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if !unwrap.IsEmptyRecord(t) {
			result = append(result, t)
		}
	}
	return result
}

// CoalesceCompatibleRecords merges structurally compatible record variants into
// one optional-field record. This keeps flow joins precise for mutation-style
// code paths while preserving discriminated unions.
func CoalesceCompatibleRecords(types []typ.Type) []typ.Type {
	if len(types) < 2 {
		return types
	}

	used := make([]bool, len(types))
	out := make([]typ.Type, 0, len(types))
	for i := 0; i < len(types); i++ {
		if used[i] {
			continue
		}
		current := types[i]
		for j := i + 1; j < len(types); j++ {
			if used[j] {
				continue
			}
			merged, ok := typ.JoinCompatibleRecords(current, types[j])
			if !ok {
				continue
			}
			current = merged
			used[j] = true
		}
		out = append(out, current)
	}
	return out
}

// filterUnknown removes nil and unknown types from the slice.
//
// Unknown types represent complete lack of information and don't contribute
// to a join. Including them would make the join result imprecise (Unknown
// unions with anything stays Unknown-ish).
//
// Nil types can appear from uninitialized paths and are similarly filtered.
func filterUnknown(types []typ.Type) []typ.Type {
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if typ.IsAbsentOrUnknown(t) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// CoalesceMaps merges multiple map types into a single map with unioned key/value types.
//
// When joining branches that each produce a map type, rather than creating
// a union of maps, we create a single map with the union of key types and
// union of value types. This is more precise and matches Lua semantics.
//
// Example: Joining {[string]: number} and {[string]: boolean} produces
// {[string]: number|boolean}, not {[string]: number} | {[string]: boolean}.
//
// Non-map types in the input are preserved unchanged in the result.
func CoalesceMaps(types []typ.Type) []typ.Type {
	if len(types) < 2 {
		return types
	}

	var maps []*typ.Map
	rest := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if t == nil {
			continue
		}
		if m, ok := t.(*typ.Map); ok {
			maps = append(maps, m)
			continue
		}
		rest = append(rest, t)
	}

	if len(maps) <= 1 {
		return types
	}

	key := maps[0].Key
	val := maps[0].Value
	for i := 1; i < len(maps); i++ {
		key = Types(key, maps[i].Key)
		val = Types(val, maps[i].Value)
	}
	rest = append(rest, typ.NewMap(key, val))
	return rest
}

// CoalesceRecordOpenness converts closed records to open when joining with open records.
//
// In Lua, a closed record {x: number, y: string} can flow to code expecting an
// open record {x: number, ...}. When branches produce different record variants,
// we open all closed records to allow the join to represent "at least these fields."
//
// This prevents spurious errors when one branch produces a known-complete record
// and another produces a record that might have additional fields.
//
// If all records are closed or all are open, no transformation is needed.
func CoalesceRecordOpenness(types []typ.Type) []typ.Type {
	hasOpen := false
	hasClosed := false
	for _, t := range types {
		if r, ok := t.(*typ.Record); ok {
			if r.Open {
				hasOpen = true
			} else {
				hasClosed = true
			}
		}
	}
	if !hasOpen || !hasClosed {
		return types
	}
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		r, ok := t.(*typ.Record)
		if !ok || r.Open {
			result = append(result, t)
			continue
		}
		builder := typ.NewRecord().SetOpen(true)
		for _, f := range r.Fields {
			switch {
			case f.Optional && f.Readonly:
				builder.OptReadonlyField(f.Name, f.Type)
			case f.Optional:
				builder.OptField(f.Name, f.Type)
			case f.Readonly:
				builder.ReadonlyField(f.Name, f.Type)
			default:
				builder.Field(f.Name, f.Type)
			}
		}
		if r.Metatable != nil {
			builder.Metatable(r.Metatable)
		}
		if r.HasMapComponent() {
			builder.MapComponent(r.MapKey, r.MapValue)
		}
		result = append(result, builder.Build())
	}
	return result
}

// CoalesceRecordMapComponents merges map components across records with identical fields.
//
// Lua tables can have both named fields (record component) and dynamic keys
// (map component). When joining records with the same field signature but
// different map components, this function merges the map components.
//
// Example: Joining {foo: string, [string]: number} and {foo: string, [string]: boolean}
// produces {foo: string, [string]: number|boolean}.
//
// Records are grouped by field signature (name, type, optional/readonly flags).
// Only groups where at least one record has a map component are merged.
// Records with different field signatures are not affected.
func CoalesceRecordMapComponents(types []typ.Type) []typ.Type {
	if len(types) < 2 {
		return types
	}

	// Group records by field signature
	type recGroup struct {
		template *typ.Record
		indices  []int
		records  []*typ.Record
	}
	groups := make(map[uint64][]*recGroup)
	for i, t := range types {
		rec, ok := t.(*typ.Record)
		if !ok || len(rec.Fields) == 0 {
			continue
		}
		sigHash := recordFieldSignatureHash(rec)
		var group *recGroup
		for _, candidate := range groups[sigHash] {
			if sameRecordFieldSignature(candidate.template, rec) {
				group = candidate
				break
			}
		}
		if group == nil {
			group = &recGroup{template: rec}
			groups[sigHash] = append(groups[sigHash], group)
		}
		group.indices = append(group.indices, i)
		group.records = append(group.records, rec)
	}

	// Check if any group has records with map components to merge
	needsMerge := false
	for _, bucket := range groups {
		for _, g := range bucket {
			if len(g.records) < 2 {
				continue
			}
			hasMap := false
			for _, r := range g.records {
				if r.HasMapComponent() {
					hasMap = true
					break
				}
			}
			if hasMap {
				needsMerge = true
				break
			}
		}
		if needsMerge {
			break
		}
	}
	if !needsMerge {
		return types
	}

	// Build result replacing merged groups
	skip := make(map[int]bool)
	result := make([]typ.Type, 0, len(types))
	for _, bucket := range groups {
		for _, g := range bucket {
			if len(g.records) < 2 {
				continue
			}
			hasMap := false
			for _, r := range g.records {
				if r.HasMapComponent() {
					hasMap = true
					break
				}
			}
			if !hasMap {
				continue
			}

			// Merge map components
			var mapKey, mapValue typ.Type
			for _, r := range g.records {
				if !r.HasMapComponent() {
					continue
				}
				if mapKey == nil {
					mapKey = r.MapKey
					mapValue = r.MapValue
				} else {
					mapKey = Types(mapKey, r.MapKey)
					mapValue = Types(mapValue, r.MapValue)
				}
			}
			// Use the first record as the template
			template := g.template
			builder := typ.NewRecord()
			if template.Open {
				builder.SetOpen(true)
			}
			for _, f := range template.Fields {
				switch {
				case f.Optional && f.Readonly:
					builder.OptReadonlyField(f.Name, f.Type)
				case f.Optional:
					builder.OptField(f.Name, f.Type)
				case f.Readonly:
					builder.ReadonlyField(f.Name, f.Type)
				default:
					builder.Field(f.Name, f.Type)
				}
			}
			if template.Metatable != nil {
				builder.Metatable(template.Metatable)
			}
			if mapKey != nil && mapValue != nil {
				builder.MapComponent(mapKey, mapValue)
			}
			merged := builder.Build()

			// Mark all indices in this group for replacement
			for _, idx := range g.indices {
				skip[idx] = true
			}
			// Add merged record at the position of the first occurrence
			result = append(result, merged)
		}
	}

	if len(skip) == 0 {
		return types
	}

	// Build final result preserving order for non-merged types
	final := make([]typ.Type, 0, len(types))
	for i, t := range types {
		if skip[i] {
			continue
		}
		final = append(final, t)
	}
	final = append(final, result...)
	return final
}

func sameRecordFieldSignature(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Open != b.Open || len(a.Fields) != len(b.Fields) {
		return false
	}
	for i, af := range a.Fields {
		bf := b.Fields[i]
		if af.Name != bf.Name || af.Optional != bf.Optional || af.Readonly != bf.Readonly {
			return false
		}
		if !typ.TypeEquals(af.Type, bf.Type) {
			return false
		}
	}
	return true
}

func recordFieldSignatureHash(r *typ.Record) uint64 {
	if r == nil {
		return 0
	}
	h := internal.HashCombine(uint64(kind.Record), uint64(len(r.Fields)))
	if r.Open {
		h = internal.HashCombine(h, 1)
	}
	for _, f := range r.Fields {
		h = internal.HashCombine(h, internal.FnvString(f.Name))
		if f.Optional {
			h = internal.HashCombine(h, 2)
		}
		if f.Readonly {
			h = internal.HashCombine(h, 3)
		}
		h = internal.HashCombine(h, f.Type.Hash())
	}
	return h
}

// CoalesceEmptyRecordWithMap removes empty records when maps are present.
//
// In Lua, {} (empty table) is a common initial value that gets populated later.
// When joining {} with a map type, the empty record is redundant since the map
// already represents "table with dynamic keys." Keeping the empty record would
// create an imprecise union.
//
// Example: Joining {} and {[string]: number} produces {[string]: number}, not
// {} | {[string]: number}.
func CoalesceEmptyRecordWithMap(types []typ.Type) []typ.Type {
	hasEmptyRecord := false
	hasMap := false
	for _, t := range types {
		if unwrap.IsEmptyRecord(t) {
			hasEmptyRecord = true
		}
		if t != nil && t.Kind() == kind.Map {
			hasMap = true
		}
	}
	if !hasEmptyRecord || !hasMap {
		return types
	}
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if !unwrap.IsEmptyRecord(t) {
			result = append(result, t)
		}
	}
	return result
}

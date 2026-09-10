package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/typ"
)

// This file provides utilities for applying type mutations (field assignments,
// indexer assignments, array mutations) to type overlays during return inference.
//
// When analyzing nested functions, field assignments and mutations performed
// by called functions must be reflected in the types visible to the caller.
// These utilities merge mutation information into type overlays.

// MergeFieldAssignments merges src into dst.
//
// Field assignments from different sources (different called functions,
// different branches) are merged using join.Two to produce union types.
// This ensures that all possible field types are captured.
func MergeFieldAssignments(
	dst map[cfg.SymbolID]map[string]typ.Type,
	src map[cfg.SymbolID]map[string]typ.Type,
) {
	overlaymut.MergeFieldAssignments(dst, src)
}

// ApplyFieldMergeToOverlay merges collected field assignments into symbol types in the overlay.
//
// For each symbol with collected field assignments, this function:
//  1. Looks up the symbol's current type in the overlay
//  2. Merges the assigned fields into that type using MergeFieldsIntoType
//  3. Updates the overlay with the enriched type
//
// This enables field assignments from called nested functions to be reflected
// in the types visible during parent function analysis.
func ApplyFieldMergeToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	fieldAssignments map[cfg.SymbolID]map[string]typ.Type,
) {
	overlaymut.ApplyFieldMergeToOverlay(overlay, fieldAssignments)
}

// MergeFieldsIntoType merges a set of field types into a base type.
//
// The merge strategy depends on the base type:
//   - nil base: Creates an open record with the given fields
//   - Map base: Creates an open record with map component plus fields
//   - Record base: Adds new fields, preserving existing fields and metadata
//   - Other base: Creates an open record with just the fields
//
// Field names are sorted for deterministic output. Existing record fields
// are preserved (not overwritten) since they represent more precise type info.
func MergeFieldsIntoType(baseType typ.Type, fields map[string]typ.Type) typ.Type {
	return overlaymut.MergeFieldsIntoType(baseType, fields)
}

// ApplyIndexerMergeToOverlay adds map components to symbol types based on dynamic index assignments.
//
// Dynamic index assignments (t[k] = v where k is not a literal) indicate
// map-like behavior. This function collects all indexer assignments for each
// symbol, joins the key and value types, and adds a map component to the
// symbol's type.
//
// Key types are joined across all assignments; if all keys are numbers, the
// result is a numeric map. Value types are joined with special handling:
// empty records {} are replaced by arrays when array elements are assigned.
func ApplyIndexerMergeToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	indexerAssignments map[cfg.SymbolID][]mutator.IndexerInfo,
) {
	overlaymut.ApplyIndexerMergeToOverlay(overlay, indexerAssignments)
}

// JoinValueTypes joins two value types, preferring arrays over empty records.
//
// When {} (empty record) and T[] (array) are joined, the result is T[].
// This models the common Lua pattern of initializing a variable as {} and
// then using it as an array via table.insert or indexed assignment.
// The array type takes precedence because it carries more specific information.
func JoinValueTypes(a, b typ.Type) typ.Type {
	return overlaymut.JoinValueTypes(a, b)
}

// MergeMapComponentIntoType adds a map component to a base type.
//
// The merge strategy depends on the base type:
//   - nil base: Creates a new Map type
//   - Map base: Joins key and value types with existing map types
//   - Record base: Adds/updates the map component while preserving fields
//   - Other base: Creates a new Map type
//
// This is used when dynamic index assignments are detected, indicating the
// variable is used as a map or has map-like access patterns.
func MergeMapComponentIntoType(baseType, keyType, valType typ.Type) typ.Type {
	return overlaymut.MergeMapComponentIntoType(baseType, keyType, valType)
}

// ApplyDirectMutationsToOverlay widens array element types based on table.insert mutations.
//
// When table.insert(t, v) is called, the array element type of t should include
// the type of v. This function applies such mutations by widening the element
// type of each affected symbol's type.
//
// This is separate from field assignments because table.insert modifies the
// array portion of a table, not named fields.
func ApplyDirectMutationsToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	mutations map[cfg.SymbolID]typ.Type,
) {
	overlaymut.ApplyDirectMutationsToOverlay(overlay, mutations)
}

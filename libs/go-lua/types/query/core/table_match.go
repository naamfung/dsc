package core

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TableMatchResult holds the result of matching a table literal against a union type.
//
// When a table literal is assigned to a union type, this result identifies
// which specific union member the table matches, enabling precise type inference.
type TableMatchResult struct {
	// Member is the specific union member type that the table matches.
	Member typ.Type

	// MemberIndex is the index of the matching member in the union's Members slice.
	MemberIndex int
}

// TryDiscriminatedUnionMember finds the single matching union member for a table literal.
//
// Given a table literal expression and an expected union type, this function
// examines the literal string fields in the table and matches them against
// the union members' discriminator fields.
//
// Example:
//
//	type Event = {kind: "click", x: number} | {kind: "key", code: string}
//	local e: Event = {kind = "click", x = 10}  -- matches first member
//
// Returns nil if:
//   - The expected type is not a union
//   - The table has no literal string fields
//   - Multiple union members match
//   - No union members match
func TryDiscriminatedUnionMember(table *ast.TableExpr, expected typ.Type) *TableMatchResult {
	union, ok := unwrap.Alias(expected).(*typ.Union)
	if !ok {
		return nil
	}

	literalFields := extractLiteralStringFields(table)
	if len(literalFields) == 0 {
		return nil
	}

	var matchingMembers []typ.Type
	matchingIdx := -1

	for i, member := range union.Members {
		if matchesMemberFields(member, literalFields) {
			matchingMembers = append(matchingMembers, member)
			matchingIdx = i
		}
	}

	if len(matchingMembers) == 1 {
		return &TableMatchResult{
			Member:      matchingMembers[0],
			MemberIndex: matchingIdx,
		}
	}

	return nil
}

// extractLiteralStringFields extracts field name to literal string value mappings.
// Only includes fields with string or identifier keys and string literal values.
func extractLiteralStringFields(table *ast.TableExpr) map[string]string {
	result := make(map[string]string)
	for _, field := range table.Fields {
		if field.Key == nil {
			continue
		}
		var fieldName string
		switch k := field.Key.(type) {
		case *ast.StringExpr:
			fieldName = k.Value
		case *ast.IdentExpr:
			fieldName = k.Value
		default:
			continue
		}

		if strVal, ok := field.Value.(*ast.StringExpr); ok {
			result[fieldName] = strVal.Value
		}
	}
	return result
}

// matchesMemberFields checks if a union member's literal fields match the table's values.
// A member matches if it has at least one literal string field and all its literal
// string fields have matching values in the table.
func matchesMemberFields(member typ.Type, literalFields map[string]string) bool {
	record := unwrap.Record(member)
	if record == nil {
		return false
	}

	hasDiscriminant := false
	for _, fieldDef := range record.Fields {
		if !unwrap.IsLiteralString(fieldDef.Type) {
			continue
		}
		hasDiscriminant = true

		tableValue, hasField := literalFields[fieldDef.Name]
		if !hasField {
			return false
		}

		if !matchesLiteralString(fieldDef.Type, tableValue) {
			return false
		}
	}

	return hasDiscriminant
}

// matchesLiteralString checks if a type matches a specific string literal value.
// Handles literal types directly and unions of literals recursively.
func matchesLiteralString(t typ.Type, value string) bool {
	unwrapped := unwrap.Alias(t)

	if lit, ok := unwrapped.(*typ.Literal); ok {
		if strVal, ok := lit.Value.(string); ok {
			return strVal == value
		}
	}

	if union, ok := unwrapped.(*typ.Union); ok {
		for _, m := range union.Members {
			if matchesLiteralString(m, value) {
				return true
			}
		}
	}

	return false
}

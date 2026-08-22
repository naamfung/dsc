package core

import (
	"github.com/wippyai/go-lua/types/typ"
)

// DiscriminatorInfo contains information about a discriminator field in a union type.
//
// A discriminated union (also called tagged union or sum type) uses a field
// with unique literal values to distinguish between union members at runtime.
// This enables precise type narrowing in control flow analysis.
//
// Example:
//
//	type Result = {ok: true, value: T} | {ok: false, error: string}
//
// Here "ok" is the discriminator field with values true and false.
type DiscriminatorInfo struct {
	// FieldName is the name of the discriminating field (e.g., "kind", "type").
	FieldName string

	// Values contains the unique literal value for each union member,
	// in the same order as the union's Members slice.
	Values []interface{}
}

// preferredDiscriminatorNames lists common discriminator field names in priority order.
// When multiple valid discriminators exist, these names are preferred.
var preferredDiscriminatorNames = []string{"tag", "type", "kind", "status", "action"}

// InferDiscriminator analyzes a union type to find a discriminator field.
//
// A valid discriminator must:
//  1. Exist as a field in ALL union members
//  2. Have a literal type in each member
//  3. Have a UNIQUE literal value for each member
//
// If multiple valid discriminators exist, the function prefers common names
// (tag, type, kind, status, action) and falls back to field declaration order.
//
// Returns nil if the union has fewer than 2 members, contains non-record types,
// or no valid discriminator field exists.
func InferDiscriminator(union *typ.Union) *DiscriminatorInfo {
	if union == nil {
		return nil
	}

	members := AllMembers(union)
	if len(members) < 2 {
		return nil
	}

	records := make([]*typ.Record, 0, len(members))

	for _, m := range members {
		rec, ok := m.(*typ.Record)
		if !ok {
			return nil
		}

		records = append(records, rec)
	}

	candidates := findDiscriminatorCandidates(records)
	if len(candidates) == 0 {
		return nil
	}

	for _, name := range preferredDiscriminatorNames {
		if info, ok := candidates[name]; ok {
			return info
		}
	}

	for _, field := range records[0].Fields {
		if info, ok := candidates[field.Name]; ok {
			return info
		}
	}

	return nil
}

// findDiscriminatorCandidates finds all fields that could serve as discriminators.
// A candidate must be a literal type field in the first record that has unique
// literal values across all records.
func findDiscriminatorCandidates(records []*typ.Record) map[string]*DiscriminatorInfo {
	if len(records) == 0 {
		return nil
	}

	firstRecord := records[0]
	candidates := make(map[string]*DiscriminatorInfo)

	for _, field := range firstRecord.Fields {
		if field.Type == nil {
			continue
		}

		lit, ok := field.Type.(*typ.Literal)
		if !ok {
			continue
		}

		values := []interface{}{lit.Value}
		valueSet := map[interface{}]bool{lit.Value: true}
		valid := true

		for i := 1; i < len(records); i++ {
			rec := records[i]

			fieldLit := getRecordFieldLiteral(rec, field.Name)
			if fieldLit == nil {
				valid = false
				break
			}

			if valueSet[fieldLit.Value] {
				valid = false
				break
			}

			values = append(values, fieldLit.Value)
			valueSet[fieldLit.Value] = true
		}

		if valid {
			candidates[field.Name] = &DiscriminatorInfo{
				FieldName: field.Name,
				Values:    values,
			}
		}
	}

	return candidates
}

// getRecordFieldLiteral returns the literal type of a field, or nil if not literal.
func getRecordFieldLiteral(rec *typ.Record, fieldName string) *typ.Literal {
	for _, f := range rec.Fields {
		if f.Name == fieldName {
			if lit, ok := f.Type.(*typ.Literal); ok {
				return lit
			}

			return nil
		}
	}

	return nil
}

// HasDiscriminator returns true if the union is a discriminated union.
//
// This is a convenience wrapper around InferDiscriminator that discards the
// discriminator details.
func HasDiscriminator(union *typ.Union) bool {
	return InferDiscriminator(union) != nil
}

// GetDiscriminatorValue returns the discriminator value for a specific record type.
//
// Given a record type and a known discriminator field name, extracts the literal
// value that identifies this record within a discriminated union.
//
// Example: For {kind: "success", value: T}, GetDiscriminatorValue(rec, "kind")
// returns ("success", true).
//
// Returns (nil, false) if the field doesn't exist or isn't a literal type.
func GetDiscriminatorValue(rec *typ.Record, fieldName string) (interface{}, bool) {
	if rec == nil {
		return nil, false
	}

	lit := getRecordFieldLiteral(rec, fieldName)
	if lit == nil {
		return nil, false
	}

	return lit.Value, true
}

package ops

import (
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CheckResult contains the result of bidirectional type checking.
//
// Type is the checked type (may be the expected type if compatible, or
// synthesized type if checking fails). Errors contains field mismatches,
// missing required fields, and type incompatibilities.
type CheckResult struct {
	Type   typ.Type     // The checked/inferred type
	Errors []CheckError // Type errors detected
}

// CheckError describes a type error during checking.
type CheckError struct {
	Message  string
	Expected typ.Type
	Got      typ.Type
	Field    string // For field-specific errors
}

// CheckTable performs bidirectional type checking for table constructors.
//
// Given a table constructor with named fields and array elements, checks
// compatibility with an expected type and infers the result type.
//
// Behavior varies by expected type:
//   - nil: Pure synthesis mode, builds type from fields/elements
//   - Record: Checks field names/types match, reports missing/extra fields
//   - Array: Checks all elements are subtypes of element type
//   - Map: Checks keys and values against map types
//   - Tuple: Checks element count and position types
//   - Union: Finds best matching member, checks against it
//   - Intersection: Checks against all members
//   - Optional: Unwraps and checks inner type
//
// Returns the checked type and any errors found.
func CheckTable(fields []FieldDef, arrayElems []typ.Type, expected typ.Type) CheckResult {
	if expected == nil {
		// Pure synthesis mode
		return CheckResult{Type: tableConstructor(fields, arrayElems)}
	}

	if rec := unwrap.Record(expected); rec != nil {
		return checkTableAsRecord(fields, arrayElems, rec)
	}

	if alias, ok := expected.(*typ.Alias); ok {
		return CheckTable(fields, arrayElems, alias.Target)
	}

	if opt, ok := expected.(*typ.Optional); ok {
		inner := opt.Inner
		if inner == nil {
			inner = typ.Unknown
		}

		result := CheckTable(fields, arrayElems, inner)
		if result.Type == nil {
			result.Type = inner
		}

		return result
	}

	if inst, ok := expected.(*typ.Instantiated); ok {
		if resolved, err := querycore.ResolveInstantiated(inst); err == nil {
			return CheckTable(fields, arrayElems, resolved)
		}
	}

	// Handle any/unknown
	if expected.Kind().IsPlaceholder() {
		return CheckResult{Type: tableConstructor(fields, arrayElems)}
	}

	if inter, ok := expected.(*typ.Intersection); ok {
		var errors []CheckError

		for _, member := range inter.Members {
			result := CheckTable(fields, arrayElems, member)
			if rec := unwrap.Record(member); rec != nil {
				result = checkTableAsRecordAllowExtra(fields, arrayElems, rec)
			}

			if len(result.Errors) > 0 {
				errors = append(errors, result.Errors...)
			}
		}

		if len(errors) == 0 {
			return CheckResult{Type: expected}
		}

		return CheckResult{Type: expected, Errors: errors}
	}

	unwrapped := typ.UnwrapAnnotated(expected)

	switch unwrapped.Kind() {
	case kind.Array:
		return checkTableAsArray(fields, arrayElems, unwrapped.(*typ.Array))
	case kind.Map:
		return checkTableAsMap(fields, arrayElems, unwrapped.(*typ.Map))
	case kind.Record:
		return checkTableAsRecord(fields, arrayElems, unwrapped.(*typ.Record))
	case kind.Tuple:
		return checkTableAsTuple(arrayElems, unwrapped.(*typ.Tuple))
	case kind.Union:
		return checkTableAsUnion(fields, arrayElems, unwrapped.(*typ.Union))
	default:
		// Try synthesis and check compatibility
		synthesized := tableConstructor(fields, arrayElems)
		if subtype.IsSubtype(synthesized, expected) {
			return CheckResult{Type: synthesized}
		}

		return CheckResult{
			Type: synthesized,
			Errors: []CheckError{{
				Message:  "table not compatible with expected type",
				Expected: expected,
				Got:      synthesized,
			}},
		}
	}
}

func checkTableAsRecordAllowExtra(fields []FieldDef, elems []typ.Type, expected *typ.Record) CheckResult {
	result := checkTableAsRecord(fields, elems, expected)
	if len(result.Errors) == 0 {
		return result
	}

	filtered := result.Errors[:0]

	for _, err := range result.Errors {
		if err.Message == "unexpected field" {
			continue
		}

		filtered = append(filtered, err)
	}

	result.Errors = filtered

	return result
}

// checkTableAsArray checks table against array type.
func checkTableAsArray(fields []FieldDef, elems []typ.Type, expected *typ.Array) CheckResult {
	var errors []CheckError

	// Named fields not allowed in array context
	if len(fields) > 0 {
		errors = append(errors, CheckError{
			Message: "named fields not allowed in array context",
		})
	}

	// Check each element against expected element type
	for i, elem := range elems {
		if !subtype.IsSubtype(elem, expected.Element) {
			errors = append(errors, CheckError{
				Message:  "element type mismatch",
				Expected: expected.Element,
				Got:      elem,
				Field:    string(rune('0' + i + 1)), // "1", "2", etc.
			})
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}

// checkTableAsMap checks table against map type.
func checkTableAsMap(fields []FieldDef, elems []typ.Type, expected *typ.Map) CheckResult {
	var errors []CheckError

	// Check named fields (string keys)
	if expected.Key.Kind() == kind.String {
		for _, f := range fields {
			if !subtype.IsSubtype(f.Type, expected.Value) {
				errors = append(errors, CheckError{
					Message:  "field value type mismatch",
					Expected: expected.Value,
					Got:      f.Type,
					Field:    f.Name,
				})
			}
		}
	}

	// Check array elements (integer keys)
	if expected.Key.Kind() == kind.Integer || expected.Key.Kind() == kind.Number {
		for i, elem := range elems {
			if !subtype.IsSubtype(elem, expected.Value) {
				errors = append(errors, CheckError{
					Message:  "element type mismatch",
					Expected: expected.Value,
					Got:      elem,
					Field:    string(rune('0' + i + 1)),
				})
			}
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}

// checkTableAsUnion checks table against union type by finding the best-matching member.
func checkTableAsUnion(fields []FieldDef, arrayElems []typ.Type, expected *typ.Union) CheckResult {
	if len(expected.Members) == 0 {
		return CheckResult{
			Type:   tableConstructor(fields, arrayElems),
			Errors: []CheckError{{Message: "empty union type"}},
		}
	}

	// Find the union member with the best field match
	bestMember := findBestUnionMember(fields, expected.Members)
	if bestMember != nil {
		result := CheckTable(fields, arrayElems, bestMember)
		if len(result.Errors) == 0 {
			return result
		}
	}

	// Try each member and return first success
	for _, member := range expected.Members {
		result := CheckTable(fields, arrayElems, member)
		if len(result.Errors) == 0 {
			return result
		}
	}

	// No member matched - synthesize and report error
	synthesized := tableConstructor(fields, arrayElems)

	return CheckResult{
		Type: synthesized,
		Errors: []CheckError{{
			Message:  "table not compatible with any union member",
			Expected: expected,
			Got:      synthesized,
		}},
	}
}

// findBestUnionMember finds the union member whose record fields best match the provided fields.
func findBestUnionMember(fields []FieldDef, members []typ.Type) typ.Type {
	if len(fields) == 0 {
		return nil
	}

	fieldNames := make(map[string]bool)
	for _, f := range fields {
		fieldNames[f.Name] = true
	}

	var bestMember typ.Type

	bestScore := -1

	for _, member := range members {
		rec := unwrap.Record(member)
		if rec == nil {
			continue
		}

		score := 0

		for _, rf := range rec.Fields {
			if fieldNames[rf.Name] {
				score++
			}
		}

		if score > bestScore {
			bestScore = score
			bestMember = member
		}
	}

	return bestMember
}

// checkTableAsRecord checks table against record type.
func checkTableAsRecord(fields []FieldDef, elems []typ.Type, expected *typ.Record) CheckResult {
	var errors []CheckError

	// Array elements not allowed in record context (unless record has integer fields)
	if len(elems) > 0 {
		errors = append(errors, CheckError{
			Message: "array elements not allowed in record context",
		})
	}

	// Build a map of provided fields
	provided := make(map[string]typ.Type)
	for _, f := range fields {
		provided[f.Name] = f.Type
	}

	// Check each expected field
	for _, ef := range expected.Fields {
		pf, ok := provided[ef.Name]
		if !ok {
			if !ef.Optional {
				errors = append(errors, CheckError{
					Message:  "missing required field",
					Expected: ef.Type,
					Field:    ef.Name,
				})
			}

			continue
		}

		if !subtype.IsSubtype(pf, ef.Type) {
			errors = append(errors, CheckError{
				Message:  "field type mismatch",
				Expected: ef.Type,
				Got:      pf,
				Field:    ef.Name,
			})
		}
	}

	// Check for extra fields not in expected
	for _, f := range fields {
		if expected.GetField(f.Name) == nil {
			errors = append(errors, CheckError{
				Message: "unexpected field",
				Got:     f.Type,
				Field:   f.Name,
			})
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}

// checkTableAsTuple checks table against tuple type.
func checkTableAsTuple(elems []typ.Type, expected *typ.Tuple) CheckResult {
	var errors []CheckError

	// Check element count
	if len(elems) < len(expected.Elements) {
		errors = append(errors, CheckError{
			Message: "not enough elements for tuple",
		})
	} else if len(elems) > len(expected.Elements) {
		errors = append(errors, CheckError{
			Message: "too many elements for tuple",
		})
	}

	// Check each element
	for i := 0; i < len(elems) && i < len(expected.Elements); i++ {
		if !subtype.IsSubtype(elems[i], expected.Elements[i]) {
			errors = append(errors, CheckError{
				Message:  "element type mismatch",
				Expected: expected.Elements[i],
				Got:      elems[i],
				Field:    string(rune('0' + i + 1)),
			})
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}

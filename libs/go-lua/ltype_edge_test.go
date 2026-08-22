package lua

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// errMessage extracts the message string from a validation error.
func errMessage(errVal LValue) string {
	if errVal == nil || errVal == LNil {
		return ""
	}
	if e, ok := errVal.(*Error); ok {
		return e.Message
	}
	return errVal.String()
}

func errDetail(errVal LValue, key string) string {
	if e, ok := errVal.(*Error); ok {
		if d := e.Details(); d != nil {
			if v, ok := d[key]; ok {
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

// errField extracts the field path from a validation error.
func errField(errVal LValue) string {
	return errDetail(errVal, "field")
}

// errExpected extracts the expected type from a validation error.
func errExpected(errVal LValue) string {
	return errDetail(errVal, "expected")
}

// errGot extracts the actual type from a validation error.
func errGot(errVal LValue) string {
	return errDetail(errVal, "got")
}

// errConstraint extracts the constraint name from a validation error.
func errConstraint(errVal LValue) string {
	return errDetail(errVal, "constraint")
}

// ---------------------------------------------------------------------------
// Group 1: Optional + every inner type kind
// ---------------------------------------------------------------------------

func TestOptionalInterface_Table(t *testing.T) {
	L := NewState()
	defer L.Close()

	// table? — the exact pattern that fails in production
	optTable := &LType{inner: typ.NewOptional(typ.NewInterface("table", nil))}

	tbl := L.NewTable()
	tbl.RawSetString("key", LString("value"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"table passes", tbl, true},
		{"empty table passes", L.NewTable(), true},
		{"nil passes", LNil, true},
		{"string fails", LString("hello"), false},
		{"number fails", LNumber(42), false},
		{"boolean fails", LTrue, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optTable.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalInterface_TableIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	// table? validated via :is() — the exact call path that fails
	optTable := &LType{inner: typ.NewOptional(typ.NewInterface("table", nil))}

	tbl := L.NewTable()
	tbl.RawSetString("key", LString("value"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"table passes", tbl, true},
		{"empty table passes", L.NewTable(), true},
		{"nil passes", LNil, true},
		{"string fails", LString("hello"), false},
		{"number fails", LNumber(42), false},
	}

	isMethod := L.typeGetField(optTable, "is")
	if isMethod == LNil {
		t.Fatal(":is method should not be nil")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L.Push(isMethod)
			L.Push(tt.value)
			L.Call(1, 2)
			val := L.Get(-2)
			errVal := L.Get(-1)
			L.Pop(2)

			if tt.ok {
				// Success is indicated by errVal == nil, not val ~= nil.
				// For optional types, nil is a valid value so val can be nil on success.
				if errVal != LNil {
					t.Errorf("expected nil error, got %v", errVal)
				}
				if val != tt.value {
					t.Errorf("expected value %v, got %v", tt.value, val)
				}
			} else {
				if val != LNil {
					t.Errorf("expected nil value, got %v", val)
				}
				if errVal == LNil {
					t.Error("expected error, got nil")
				}
			}
		})
	}
}

func TestOptionalRecord(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {x: number}?
	inner := typ.NewRecord().Field("x", typ.Number).Build()
	optRecord := &LType{inner: typ.NewOptional(inner)}

	valid := L.NewTable()
	valid.RawSetString("x", LNumber(1))

	invalid := L.NewTable()
	invalid.RawSetString("x", LString("bad"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid record passes", valid, true},
		{"nil passes", LNil, true},
		{"empty table fails (missing x)", L.NewTable(), false},
		{"invalid field type fails", invalid, false},
		{"string fails", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optRecord.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalArray(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {number}?
	optArray := &LType{inner: typ.NewOptional(typ.NewArray(typ.Number))}

	valid := L.NewTable()
	valid.Append(LNumber(1))
	valid.Append(LNumber(2))

	invalid := L.NewTable()
	invalid.Append(LString("bad"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid array passes", valid, true},
		{"nil passes", LNil, true},
		{"empty table passes", L.NewTable(), true},
		{"invalid element type fails", invalid, false},
		{"number fails", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optArray.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalMap(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {[string]: number}?
	optMap := &LType{inner: typ.NewOptional(typ.NewMap(typ.String, typ.Number))}

	valid := L.NewTable()
	valid.RawSetString("a", LNumber(1))

	invalid := L.NewTable()
	invalid.RawSetString("a", LString("bad"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid map passes", valid, true},
		{"nil passes", LNil, true},
		{"empty table passes", L.NewTable(), true},
		{"invalid value type fails", invalid, false},
		{"string fails", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optMap.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalUnion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// (number | string)?
	optUnion := &LType{inner: typ.NewOptional(typ.NewUnion(typ.Number, typ.String))}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"number passes", LNumber(42), true},
		{"string passes", LString("hello"), true},
		{"nil passes", LNil, true},
		{"boolean fails", LTrue, false},
		{"table fails", &LTable{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optUnion.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalLiteral(t *testing.T) {
	L := NewState()
	defer L.Close()

	// "active"?
	optLiteral := &LType{inner: typ.NewOptional(typ.LiteralString("active"))}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"matching literal passes", LString("active"), true},
		{"nil passes", LNil, true},
		{"non-matching string fails", LString("inactive"), false},
		{"number fails", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optLiteral.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalAnnotated(t *testing.T) {
	L := NewState()
	defer L.Close()

	// (number @min(0))?
	annotated := typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
	})
	optAnnotated := &LType{inner: typ.NewOptional(annotated)}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid number passes", LNumber(5), true},
		{"nil passes", LNil, true},
		{"negative fails annotation", LNumber(-1), false},
		{"string fails type", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optAnnotated.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	// function?
	optFunc := &LType{inner: typ.NewOptional(typ.Func().Param("x", typ.Number).Returns(typ.String).Build())}

	luaFn := L.NewFunction(func(L *LState) int { return 0 })
	goFn := LGoFunc(func(L *LState) int { return 0 })

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"lua function passes", luaFn, true},
		{"go function passes", goFn, true},
		{"nil passes", LNil, true},
		{"number fails", LNumber(42), false},
		{"table fails", L.NewTable(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optFunc.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 2: Record with optional table-typed fields (exact production scenario)
// ---------------------------------------------------------------------------

func TestRecordWithOptionalTableFields(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Mirrors the UpdateInput type from the user's binding
	tableIface := typ.NewInterface("table", nil)
	updateInput := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("name", typ.String).
			OptField("meta", tableIface).
			OptField("content", tableIface).
			OptField("tags", typ.NewArray(typ.String)).
			Build(),
		name: "UpdateInput",
	}

	// Full valid input
	full := L.NewTable()
	full.RawSetString("id", LString("abc"))
	full.RawSetString("name", LString("test"))
	meta := L.NewTable()
	meta.RawSetString("key", LString("val"))
	full.RawSetString("meta", meta)
	content := L.NewTable()
	content.RawSetString("body", LString("text"))
	full.RawSetString("content", content)
	tags := L.NewTable()
	tags.Append(LString("tag1"))
	full.RawSetString("tags", tags)

	// Minimal valid input (only required fields)
	minimal := L.NewTable()
	minimal.RawSetString("id", LString("abc"))

	// Missing required field
	missingID := L.NewTable()
	missingID.RawSetString("name", LString("test"))

	// Wrong type for optional table field
	wrongContent := L.NewTable()
	wrongContent.RawSetString("id", LString("abc"))
	wrongContent.RawSetString("content", LString("not a table"))

	// Empty table as content (should pass)
	emptyContent := L.NewTable()
	emptyContent.RawSetString("id", LString("abc"))
	emptyContent.RawSetString("content", L.NewTable())

	// Nested table with arrays as content
	nestedContent := L.NewTable()
	nestedContent.RawSetString("id", LString("abc"))
	inner := L.NewTable()
	inner.RawSetString("items", L.NewTable())
	inner.RawGet(LString("items")).(*LTable).Append(LNumber(1))
	nestedContent.RawSetString("content", inner)

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"full valid input", full, true},
		{"minimal valid input", minimal, true},
		{"missing required field fails", missingID, false},
		{"wrong type for optional table field fails", wrongContent, false},
		{"empty table as content passes", emptyContent, true},
		{"nested table as content passes", nestedContent, true},
		{"not a table fails", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateInput.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRecordWithOptionalTableFieldsIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Same structure but test via :is() for error message quality
	tableIface := typ.NewInterface("table", nil)
	updateInput := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("content", tableIface).
			Build(),
		name: "UpdateInput",
	}

	isMethod := L.typeGetField(updateInput, "is")

	// content is a table — must pass
	input := L.NewTable()
	input.RawSetString("id", LString("abc"))
	input.RawSetString("content", L.NewTable())

	L.Push(isMethod)
	L.Push(input)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Errorf("table field should validate; error: %v", errVal)
	}

	// content is string — must fail with useful error
	badInput := L.NewTable()
	badInput.RawSetString("id", LString("abc"))
	badInput.RawSetString("content", LString("not a table"))

	L.Push(isMethod)
	L.Push(badInput)
	L.Call(1, 2)
	val = L.Get(-2)
	errVal = L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Error("string content should fail validation")
	}
	if errVal == LNil {
		t.Fatal("expected error message")
	}

	errStr := errMessage(errVal)
	if !strings.Contains(errStr, "content") {
		t.Errorf("error should mention field name 'content', got: %s", errStr)
	}
	if strings.Contains(errStr, "expected table, got table") {
		t.Errorf("error should not say 'expected table, got table', got: %s", errStr)
	}
}

// Record with type-level optional (field.Optional=false, field.Type=Optional(T))
func TestRecordWithTypeLevelOptional(t *testing.T) {
	L := NewState()
	defer L.Close()

	// content field is non-optional, but type is table?
	tableIface := typ.NewInterface("table", nil)
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			Field("content", typ.NewOptional(tableIface)).
			Build(),
	}

	// content present as table
	withContent := L.NewTable()
	withContent.RawSetString("id", LString("abc"))
	withContent.RawSetString("content", L.NewTable())

	// content absent (nil)
	withoutContent := L.NewTable()
	withoutContent.RawSetString("id", LString("abc"))

	// content is wrong type
	wrongContent := L.NewTable()
	wrongContent.RawSetString("id", LString("abc"))
	wrongContent.RawSetString("content", LNumber(42))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"content present as table", withContent, true},
		{"content absent (nil passes optional)", withoutContent, true},
		{"content is wrong type", wrongContent, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 3: Interface type validation
// ---------------------------------------------------------------------------

func TestInterfaceValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	tableType := &LType{inner: typ.NewInterface("table", nil)}

	plain := L.NewTable()
	nested := L.NewTable()
	nested.RawSetString("inner", L.NewTable())

	arrayTable := L.NewTable()
	arrayTable.Append(LNumber(1))
	arrayTable.Append(LNumber(2))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"plain table", plain, true},
		{"nested table", nested, true},
		{"array-like table", arrayTable, true},
		{"empty table", L.NewTable(), true},
		{"string fails", LString("hello"), false},
		{"number fails", LNumber(42), false},
		{"nil fails", LNil, false},
		{"boolean fails", LTrue, false},
		{"function fails", LGoFunc(func(L *LState) int { return 0 }), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tableType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestInterfaceValidationIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	tableType := &LType{inner: typ.NewInterface("table", nil)}
	isMethod := L.typeGetField(tableType, "is")

	// table passes
	L.Push(isMethod)
	L.Push(L.NewTable())
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)
	if val == LNil {
		t.Errorf("table should pass Interface validation; error: %v", errVal)
	}

	// string fails with useful error
	L.Push(isMethod)
	L.Push(LString("not a table"))
	L.Call(1, 2)
	val = L.Get(-2)
	errVal = L.Get(-1)
	L.Pop(2)
	if val != LNil {
		t.Error("string should fail Interface validation")
	}
	if errVal == LNil {
		t.Fatal("expected error")
	}
	errStr := errMessage(errVal)
	if !strings.Contains(errStr, "table") {
		t.Errorf("error should mention 'table', got: %s", errStr)
	}
	if !strings.Contains(errStr, "string") {
		t.Errorf("error should mention 'string', got: %s", errStr)
	}
}

// ---------------------------------------------------------------------------
// Group 4: Error message quality
// ---------------------------------------------------------------------------

func TestErrorMessages_FieldPath(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Nested: {a: {b: {c: number}}}
	inner := typ.NewRecord().Field("c", typ.Number).Build()
	mid := typ.NewRecord().Field("b", inner).Build()
	outer := &LType{inner: typ.NewRecord().Field("a", mid).Build()}

	bad := L.NewTable()
	aTable := L.NewTable()
	bTable := L.NewTable()
	bTable.RawSetString("c", LString("not a number"))
	aTable.RawSetString("b", bTable)
	bad.RawSetString("a", aTable)

	isMethod := L.typeGetField(outer, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail validation")
	}
	errStr := errMessage(errVal)
	if !strings.Contains(errStr, "a.b.c") {
		t.Errorf("error should contain full path 'a.b.c', got: %s", errStr)
	}
}

func TestErrorMessages_ArrayElement(t *testing.T) {
	L := NewState()
	defer L.Close()

	arrType := &LType{inner: typ.NewArray(typ.Number)}

	bad := L.NewTable()
	bad.Append(LNumber(1))
	bad.Append(LString("bad"))
	bad.Append(LNumber(3))

	isMethod := L.typeGetField(arrType, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail validation")
	}
	errStr := errMessage(errVal)
	if !strings.Contains(errStr, "[2]") {
		t.Errorf("error should contain element index '[2]', got: %s", errStr)
	}
}

func TestErrorMessages_ExpectedVsGot(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		name        string
		typ         *LType
		value       LValue
		expectInErr string
		rejectInErr string
	}{
		{
			"number rejects string",
			LTypeNumber, LString("x"),
			"expected number, got string", "",
		},
		{
			"string rejects number",
			LTypeString, LNumber(1),
			"expected string, got number", "",
		},
		{
			"boolean rejects number",
			LTypeBoolean, LNumber(1),
			"expected boolean, got number", "",
		},
		{
			"table rejects string",
			&LType{inner: typ.NewInterface("table", nil)},
			LString("x"),
			"expected table, got string", "",
		},
		{
			"record rejects number",
			&LType{inner: typ.NewRecord().Field("x", typ.Number).Build()},
			LNumber(42),
			"expected", "expected table, got table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMethod := L.typeGetField(tt.typ, "is")
			L.Push(isMethod)
			L.Push(tt.value)
			L.Call(1, 2)
			val := L.Get(-2)
			errVal := L.Get(-1)
			L.Pop(2)

			if val != LNil {
				t.Fatal("should fail validation")
			}
			errStr := errMessage(errVal)
			if tt.expectInErr != "" && !strings.Contains(errStr, tt.expectInErr) {
				t.Errorf("error should contain %q, got: %s", tt.expectInErr, errStr)
			}
			if tt.rejectInErr != "" && strings.Contains(errStr, tt.rejectInErr) {
				t.Errorf("error should NOT contain %q, got: %s", tt.rejectInErr, errStr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 5: Complex nested types
// ---------------------------------------------------------------------------

func TestRecordWithOptionalNestedRecord(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {name: string, address: {street: string, zip: string}?}
	address := typ.NewRecord().
		Field("street", typ.String).
		Field("zip", typ.String).
		Build()
	person := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			OptField("address", address).
			Build(),
	}

	withAddr := L.NewTable()
	withAddr.RawSetString("name", LString("John"))
	addr := L.NewTable()
	addr.RawSetString("street", LString("Main St"))
	addr.RawSetString("zip", LString("12345"))
	withAddr.RawSetString("address", addr)

	withoutAddr := L.NewTable()
	withoutAddr.RawSetString("name", LString("John"))

	badAddr := L.NewTable()
	badAddr.RawSetString("name", LString("John"))
	bad := L.NewTable()
	bad.RawSetString("street", LNumber(123))
	badAddr.RawSetString("address", bad)

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"with valid address", withAddr, true},
		{"without address", withoutAddr, true},
		{"with invalid address field", badAddr, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := person.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestArrayOfOptionalElements(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {(number?)}  — array where elements can be number or nil
	arrOfOpt := &LType{inner: typ.NewArray(typ.NewOptional(typ.Number))}

	valid := L.NewTable()
	valid.Append(LNumber(1))
	valid.Append(LNil)
	valid.Append(LNumber(3))

	// Array with string element should fail
	invalid := L.NewTable()
	invalid.Append(LNumber(1))
	invalid.Append(LString("bad"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"array with nils passes", valid, true},
		{"array with string fails", invalid, false},
		{"empty array passes", L.NewTable(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arrOfOpt.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestMapWithOptionalValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {[string]: number?}
	mapOfOpt := &LType{inner: typ.NewMap(typ.String, typ.NewOptional(typ.Number))}

	valid := L.NewTable()
	valid.RawSetString("a", LNumber(1))
	// Note: setting nil in strdict doesn't store it, so we test with present values

	invalid := L.NewTable()
	invalid.RawSetString("a", LString("bad"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid map passes", valid, true},
		{"map with wrong value type fails", invalid, false},
		{"empty map passes", L.NewTable(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapOfOpt.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestUnionOfRecords(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {kind: "a", x: number} | {kind: "b", y: string}
	recA := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("x", typ.Number).
		Build()
	recB := typ.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("y", typ.String).
		Build()
	unionRec := &LType{inner: typ.NewUnion(recA, recB)}

	validA := L.NewTable()
	validA.RawSetString("kind", LString("a"))
	validA.RawSetString("x", LNumber(42))

	validB := L.NewTable()
	validB.RawSetString("kind", LString("b"))
	validB.RawSetString("y", LString("hello"))

	invalidKind := L.NewTable()
	invalidKind.RawSetString("kind", LString("c"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"record A passes", validA, true},
		{"record B passes", validB, true},
		{"unknown kind fails", invalidKind, false},
		{"non-table fails", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unionRec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRecordWithArrayField(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {name: string, tags: {string}}
	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			Field("tags", typ.NewArray(typ.String)).
			Build(),
	}

	valid := L.NewTable()
	valid.RawSetString("name", LString("test"))
	tags := L.NewTable()
	tags.Append(LString("a"))
	tags.Append(LString("b"))
	valid.RawSetString("tags", tags)

	invalidTag := L.NewTable()
	invalidTag.RawSetString("name", LString("test"))
	badTags := L.NewTable()
	badTags.Append(LNumber(1))
	invalidTag.RawSetString("tags", badTags)

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid record with array", valid, true},
		{"invalid array element type", invalidTag, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRecordWithMapField(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {config: {[string]: string}}
	rec := &LType{
		inner: typ.NewRecord().
			Field("config", typ.NewMap(typ.String, typ.String)).
			Build(),
	}

	valid := L.NewTable()
	config := L.NewTable()
	config.RawSetString("key", LString("value"))
	valid.RawSetString("config", config)

	invalid := L.NewTable()
	badConfig := L.NewTable()
	badConfig.RawSetString("key", LNumber(42))
	invalid.RawSetString("config", badConfig)

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid map field", valid, true},
		{"invalid map value type", invalid, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 6: Record required vs optional field semantics
// ---------------------------------------------------------------------------

func TestRecordRequiredFieldMissing(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("a", typ.String).
			Field("b", typ.Number).
			OptField("c", typ.Boolean).
			Build(),
	}

	// Only a provided — b missing
	missing := L.NewTable()
	missing.RawSetString("a", LString("hello"))

	if rec.Validate(L, missing) {
		t.Error("missing required field 'b' should fail")
	}

	// a and b provided, c optional
	valid := L.NewTable()
	valid.RawSetString("a", LString("hello"))
	valid.RawSetString("b", LNumber(42))

	if !rec.Validate(L, valid) {
		t.Error("all required fields present should pass")
	}
}

func TestRecordOptionalFieldWrongType(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("count", typ.Number).
			Build(),
	}

	// count present but wrong type
	bad := L.NewTable()
	bad.RawSetString("id", LString("abc"))
	bad.RawSetString("count", LString("not a number"))

	if rec.Validate(L, bad) {
		t.Error("optional field with wrong type should fail")
	}

	// Verify :is() gives useful error
	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Error("should fail")
	}
	errStr := errMessage(errVal)
	if !strings.Contains(errStr, "count") {
		t.Errorf("error should reference field 'count', got: %s", errStr)
	}
}

func TestRecordAllFieldsOptional(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			OptField("a", typ.String).
			OptField("b", typ.Number).
			Build(),
	}

	empty := L.NewTable()
	if !rec.Validate(L, empty) {
		t.Error("empty table should pass when all fields are optional")
	}
}

// ---------------------------------------------------------------------------
// Group 7: Tuple validation
// ---------------------------------------------------------------------------

func TestTupleValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// (number, string, boolean)
	tupleType := &LType{inner: typ.NewTuple(typ.Number, typ.String, typ.Boolean)}

	valid := L.NewTable()
	valid.Append(LNumber(1))
	valid.Append(LString("hello"))
	valid.Append(LTrue)

	wrongOrder := L.NewTable()
	wrongOrder.Append(LString("hello"))
	wrongOrder.Append(LNumber(1))
	wrongOrder.Append(LTrue)

	tooShort := L.NewTable()
	tooShort.Append(LNumber(1))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid tuple", valid, true},
		{"wrong element order", wrongOrder, false},
		{"missing elements", tooShort, false},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tupleType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 8: Integer edge cases
// ---------------------------------------------------------------------------

func TestIntegerEdgeCases(t *testing.T) {
	L := NewState()
	defer L.Close()

	intType := LTypeInteger

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"LInteger passes", LInteger(42), true},
		{"whole LNumber passes", LNumber(42.0), true},
		{"fractional LNumber fails", LNumber(42.5), false},
		{"zero passes", LNumber(0), true},
		{"negative integer passes", LInteger(-1), true},
		{"negative whole number passes", LNumber(-5.0), true},
		{"string fails", LString("42"), false},
		{"boolean fails", LTrue, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 9: Alias and Ref type handling at runtime
// ---------------------------------------------------------------------------

func TestAliasTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// type Score = number
	alias := typ.NewAlias("Score", typ.Number)
	scoreType := &LType{inner: alias}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"number passes alias", LNumber(42), true},
		{"integer passes alias", LInteger(42), true},
		{"string fails alias", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scoreType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAliasTypeValidationIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	alias := typ.NewAlias("Score", typ.Number)
	scoreType := &LType{inner: alias}

	isMethod := L.typeGetField(scoreType, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Errorf("alias to number should pass; error: %v", errVal)
	}

	L.Push(isMethod)
	L.Push(LString("bad"))
	L.Call(1, 2)
	val = L.Get(-2)
	errVal = L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Error("string should fail alias-to-number validation")
	}
	if errVal == LNil {
		t.Fatal("expected error")
	}
}

func TestOptionalAliasType(t *testing.T) {
	L := NewState()
	defer L.Close()

	// type Score = number; Score?
	alias := typ.NewAlias("Score", typ.Number)
	optAlias := &LType{inner: typ.NewOptional(alias)}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"number passes", LNumber(42), true},
		{"nil passes", LNil, true},
		{"string fails", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optAlias.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRefTypeWithResolver(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Simulate a Ref that resolves to a known type
	ref := typ.NewRef("", "Score")
	refType := &LType{
		inner: ref,
		resolver: &typeResolver{
			types: map[string]typ.Type{
				"Score": typ.Number,
			},
		},
	}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"number passes resolved ref", LNumber(42), true},
		{"string fails resolved ref", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRefTypeWithResolverIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	ref := typ.NewRef("", "Score")
	refType := &LType{
		inner: ref,
		resolver: &typeResolver{
			types: map[string]typ.Type{
				"Score": typ.Number,
			},
		},
	}

	isMethod := L.typeGetField(refType, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Errorf("resolved ref should pass; error: %v", errVal)
	}
}

func TestRefTypeWithoutResolver(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Unresolved Ref — should not silently accept everything
	ref := typ.NewRef("", "Unknown")
	refType := &LType{inner: ref}

	isMethod := L.typeGetField(refType, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	// An unresolved ref should fail validation with a useful error,
	// not produce "expected Unknown, got number"
	if val != LNil && errVal != LNil {
		// Either it passes (permissive) or fails (strict) — but error must be useful
		errStr := errMessage(errVal)
		if strings.Contains(errStr, "expected "+ref.String()+", got") {
			// This is a poor error — it means the ref was not resolved
			t.Logf("unresolved ref produced fallthrough error: %s", errStr)
		}
	}
}

// ---------------------------------------------------------------------------
// Group 10: Intersection type handling
// ---------------------------------------------------------------------------

func TestIntersectionTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {x: number} & {y: string} — value must satisfy both
	recA := typ.NewRecord().Field("x", typ.Number).Build()
	recB := typ.NewRecord().Field("y", typ.String).Build()
	intersection := &LType{inner: typ.NewIntersection(recA, recB)}

	// Has both x and y
	valid := L.NewTable()
	valid.RawSetString("x", LNumber(1))
	valid.RawSetString("y", LString("hello"))

	// Only has x
	onlyX := L.NewTable()
	onlyX.RawSetString("x", LNumber(1))

	// Only has y
	onlyY := L.NewTable()
	onlyY.RawSetString("y", LString("hello"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"has both fields", valid, true},
		{"missing y", onlyX, false},
		{"missing x", onlyY, false},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intersection.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestIntersectionTypeIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	recA := typ.NewRecord().Field("x", typ.Number).Build()
	recB := typ.NewRecord().Field("y", typ.String).Build()
	intersection := &LType{inner: typ.NewIntersection(recA, recB)}

	valid := L.NewTable()
	valid.RawSetString("x", LNumber(1))
	valid.RawSetString("y", LString("hello"))

	isMethod := L.typeGetField(intersection, "is")
	L.Push(isMethod)
	L.Push(valid)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Errorf("intersection validation should pass; error: %v", errVal)
	}
}

// ---------------------------------------------------------------------------
// Group 11: Annotated type inside optional inside record field
// ---------------------------------------------------------------------------

func TestRecordWithAnnotatedOptionalField(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {name: string, score: (number @min(0) @max(100))?}
	annotatedNum := typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
		{Name: "max", Arg: float64(100)},
	})
	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			OptField("score", annotatedNum).
			Build(),
	}

	valid := L.NewTable()
	valid.RawSetString("name", LString("test"))
	valid.RawSetString("score", LNumber(50))

	noScore := L.NewTable()
	noScore.RawSetString("name", LString("test"))

	tooHigh := L.NewTable()
	tooHigh.RawSetString("name", LString("test"))
	tooHigh.RawSetString("score", LNumber(150))

	wrongType := L.NewTable()
	wrongType.RawSetString("name", LString("test"))
	wrongType.RawSetString("score", LString("fifty"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid score", valid, true},
		{"missing optional score", noScore, true},
		{"score exceeds max", tooHigh, false},
		{"score wrong type", wrongType, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 12: Literal type validation edge cases
// ---------------------------------------------------------------------------

func TestLiteralTypeEdgeCases(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		name  string
		typ   *LType
		value LValue
		ok    bool
	}{
		// String literals
		{"empty string literal matches", &LType{inner: typ.LiteralString("")}, LString(""), true},
		{"empty string literal rejects non-empty", &LType{inner: typ.LiteralString("")}, LString("x"), false},

		// Number literals
		{"zero literal matches", &LType{inner: typ.LiteralNumber(0)}, LNumber(0), true},
		{"negative literal matches", &LType{inner: typ.LiteralNumber(-1)}, LNumber(-1), true},

		// Bool literals
		{"true literal matches true", &LType{inner: typ.LiteralBool(true)}, LTrue, true},
		{"true literal rejects false", &LType{inner: typ.LiteralBool(true)}, LFalse, false},
		{"false literal matches false", &LType{inner: typ.LiteralBool(false)}, LFalse, true},

		// Int64 literals with cross-type matching
		{"int literal matches LInteger", &LType{inner: typ.LiteralInt(42)}, LInteger(42), true},
		{"int literal matches equivalent LNumber", &LType{inner: typ.LiteralInt(42)}, LNumber(42), true},
		{"int literal rejects different value", &LType{inner: typ.LiteralInt(42)}, LInteger(43), false},

		// Type mismatches
		{"string literal rejects number", &LType{inner: typ.LiteralString("42")}, LNumber(42), false},
		{"number literal rejects string", &LType{inner: typ.LiteralNumber(42)}, LString("42"), false},
		{"bool literal rejects number", &LType{inner: typ.LiteralBool(true)}, LNumber(1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 13: UserData handling
// ---------------------------------------------------------------------------

func TestUserDataValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// UserData should not pass as table, number, string, etc.
	ud := &LUserData{Value: "some data"}

	tests := []struct {
		name string
		typ  *LType
		ok   bool
	}{
		{"not a table", &LType{inner: typ.NewInterface("table", nil)}, false},
		{"not a number", LTypeNumber, false},
		{"not a string", LTypeString, false},
		{"passes any", LTypeAny, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.Validate(L, ud); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 14: Never type
// ---------------------------------------------------------------------------

func TestNeverType(t *testing.T) {
	L := NewState()
	defer L.Close()

	neverType := LTypeNever

	values := []LValue{LNil, LTrue, LNumber(0), LString(""), L.NewTable()}
	for _, v := range values {
		if neverType.Validate(L, v) {
			t.Errorf("never should reject %v", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Group 15: Record with Ref fields (manifest scenario)
// Ref fields inside records simulate how types come from manifests where
// inter-type references are stored as typ.Ref.
// ---------------------------------------------------------------------------

func TestRecordWithRefField_Resolved(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Simulates: type Status = "active" | "draft"
	// type Input = {id: string, status: Status?}
	// At runtime, the Record stores status as Ref("Status") which the resolver maps.
	statusType := typ.NewUnion(typ.LiteralString("active"), typ.LiteralString("draft"))

	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("status", typ.NewRef("", "Status")).
			Build(),
		resolver: &typeResolver{
			types: map[string]typ.Type{
				"Status": statusType,
			},
		},
	}

	valid := L.NewTable()
	valid.RawSetString("id", LString("abc"))
	valid.RawSetString("status", LString("active"))

	noStatus := L.NewTable()
	noStatus.RawSetString("id", LString("abc"))

	badStatus := L.NewTable()
	badStatus.RawSetString("id", LString("abc"))
	badStatus.RawSetString("status", LString("unknown"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid status", valid, true},
		{"missing optional status", noStatus, true},
		{"invalid status value", badStatus, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRecordWithRefField_ResolvedIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	statusType := typ.NewUnion(typ.LiteralString("active"), typ.LiteralString("draft"))

	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("status", typ.NewRef("", "Status")).
			Build(),
		resolver: &typeResolver{
			types: map[string]typ.Type{
				"Status": statusType,
			},
		},
	}

	valid := L.NewTable()
	valid.RawSetString("id", LString("abc"))
	valid.RawSetString("status", LString("active"))

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(valid)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Errorf("resolved Ref field should pass; error: %v", errVal)
	}
}

// Ref to "table" — the exact scenario where table? fails if resolver
// doesn't have "table" as a builtin
func TestRecordWithRefToTable_NoBuiltin(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Record with content: Ref("table") — simulates what happens when
	// the manifest stores table as a Ref instead of inline Interface
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("content", typ.NewRef("", "table")).
			Build(),
		// resolver does NOT have "table" — simulates missing builtin
		resolver: &typeResolver{
			types: map[string]typ.Type{},
		},
	}

	input := L.NewTable()
	input.RawSetString("id", LString("abc"))
	input.RawSetString("content", L.NewTable())

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(input)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	// This is the exact scenario that produces "content: expected table, got table"
	// The Ref("table") can't be resolved, falls through, and both type name and
	// value name are "table".
	if val == LNil {
		errStr := ""
		if errVal != LNil {
			errStr = string(errVal.(LString))
		}
		t.Errorf("unresolved Ref('table') should not reject a table; error: %s", errStr)
	}
}

// Same scenario but with the resolver containing the builtin
func TestRecordWithRefToTable_WithBuiltin(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("content", typ.NewRef("", "table")).
			Build(),
		resolver: &typeResolver{
			types: map[string]typ.Type{
				"table": typ.NewInterface("table", nil),
			},
		},
	}

	input := L.NewTable()
	input.RawSetString("id", LString("abc"))
	input.RawSetString("content", L.NewTable())

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(input)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Errorf("resolved Ref('table') should pass; error: %v", errVal)
	}
}

// ---------------------------------------------------------------------------
// Group 16: Sum type handling
// ---------------------------------------------------------------------------

func TestSumTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Sum types at runtime — currently unhandled
	sumType := &LType{
		inner: typ.NewSum("Status", []typ.Variant{
			{Tag: "Active", Types: nil},
			{Tag: "Suspended", Types: nil},
		}),
	}

	// Sum types use tables for runtime representation
	tbl := L.NewTable()
	tbl.RawSetString("tag", LString("Active"))

	// At minimum, sum validation should not panic and should
	// produce a useful error or accept tables
	result := sumType.Validate(L, tbl)
	_ = result // Document behavior, don't assert specific outcome yet

	// Non-table should definitely fail
	if sumType.Validate(L, LString("Active")) {
		t.Error("sum type should reject non-table values")
	}
}

// ---------------------------------------------------------------------------
// Group 17: Unresolved Ref error messages
// ---------------------------------------------------------------------------

func TestUnresolvedRefErrorMessage(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Unresolved Ref should produce a clear error, not "expected X, got X"
	refType := &LType{inner: typ.NewRef("", "SomeType")}

	isMethod := L.typeGetField(refType, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil && errVal == LNil {
		// If it passes, that's one valid approach (permissive for unresolved)
		return
	}

	if errVal != LNil {
		errStr := errMessage(errVal)
		// The error should indicate the type is unresolved, not just
		// "expected SomeType, got number" which is confusing
		if !strings.Contains(errStr, "unresolved") {
			t.Errorf("unresolved Ref error should mention 'unresolved', got: %s", errStr)
		}
	}
}

// ===========================================================================
// STRUCTURED ERROR TESTS
// ===========================================================================

// ---------------------------------------------------------------------------
// Group 18: Structured error fields
// ---------------------------------------------------------------------------

func TestStructuredError_TypeMismatch(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			Field("age", typ.Number).
			Build(),
	}

	bad := L.NewTable()
	bad.RawSetString("name", LNumber(123))
	bad.RawSetString("age", LNumber(25))

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}

	// Verify structured fields
	if errField(errVal) != "name" {
		t.Errorf("field should be 'name', got %q", errField(errVal))
	}
	if errExpected(errVal) != "string" {
		t.Errorf("expected should be 'string', got %q", errExpected(errVal))
	}
	if errGot(errVal) != "number" {
		t.Errorf("got should be 'number', got %q", errGot(errVal))
	}
}

func TestStructuredError_MissingRequired(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			Field("name", typ.String).
			Build(),
	}

	missing := L.NewTable()
	missing.RawSetString("id", LString("abc"))

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(missing)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}

	msg := errMessage(errVal)
	if !strings.Contains(msg, "required") {
		t.Errorf("error should mention 'required', got: %s", msg)
	}
	if errField(errVal) != "name" {
		t.Errorf("field should be 'name', got %q", errField(errVal))
	}
	if errExpected(errVal) != "string" {
		t.Errorf("expected should be 'string', got %q", errExpected(errVal))
	}
}

func TestStructuredError_NestedField(t *testing.T) {
	L := NewState()
	defer L.Close()

	inner := typ.NewRecord().Field("zip", typ.String).Build()
	outer := &LType{inner: typ.NewRecord().Field("addr", inner).Build()}

	bad := L.NewTable()
	a := L.NewTable()
	a.RawSetString("zip", LNumber(12345))
	bad.RawSetString("addr", a)

	isMethod := L.typeGetField(outer, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}

	if errField(errVal) != "addr.zip" {
		t.Errorf("field should be 'addr.zip', got %q", errField(errVal))
	}
	if errExpected(errVal) != "string" {
		t.Errorf("expected should be 'string', got %q", errExpected(errVal))
	}
	if errGot(errVal) != "number" {
		t.Errorf("got should be 'number', got %q", errGot(errVal))
	}
}

func TestStructuredError_ArrayElement(t *testing.T) {
	L := NewState()
	defer L.Close()

	arrType := &LType{inner: typ.NewArray(typ.Number)}

	bad := L.NewTable()
	bad.Append(LNumber(1))
	bad.Append(LString("bad"))

	isMethod := L.typeGetField(arrType, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}

	if errField(errVal) != "[2]" {
		t.Errorf("field should be '[2]', got %q", errField(errVal))
	}
	if errExpected(errVal) != "number" {
		t.Errorf("expected should be 'number', got %q", errExpected(errVal))
	}
}

func TestStructuredError_ConstraintViolation(t *testing.T) {
	L := NewState()
	defer L.Close()

	annotated := &LType{
		inner: typ.NewAnnotated(typ.Number, []typ.Annotation{
			{Name: "min", Arg: float64(0)},
		}),
	}

	isMethod := L.typeGetField(annotated, "is")
	L.Push(isMethod)
	L.Push(LNumber(-5))
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}

	if errConstraint(errVal) != "min" {
		t.Errorf("constraint should be 'min', got %q", errConstraint(errVal))
	}
}

func TestStructuredError_NestedConstraint(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("score", typ.Number, false, []typ.Annotation{
				{Name: "max", Arg: float64(100)},
			}).
			Build(),
	}

	bad := L.NewTable()
	bad.RawSetString("score", LNumber(150))

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}

	if errField(errVal) != "score" {
		t.Errorf("field should be 'score', got %q", errField(errVal))
	}
	if errConstraint(errVal) != "max" {
		t.Errorf("constraint should be 'max', got %q", errConstraint(errVal))
	}
}

func TestStructuredError_TostringMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenErrors(L)

	L.SetGlobal("Number", LTypeNumber)

	err := L.DoString(`
		local val, err = Number:is("hello")
		assert(val == nil, "should fail")
		assert(err ~= nil, "should have error")
		local msg = tostring(err)
		assert(type(msg) == "string", "tostring should return string, got " .. type(msg))
		assert(msg:find("expected"), "message should contain 'expected', got: " .. msg)
	`)
	if err != nil {
		t.Fatalf("tostring test failed: %v", err)
	}
}

func TestStructuredError_ConcatMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenErrors(L)

	L.SetGlobal("Number", LTypeNumber)

	err := L.DoString(`
		local val, err = Number:is("hello")
		local msg1 = "validation failed: " .. err
		assert(type(msg1) == "string", "concat should produce string")
		assert(msg1:find("expected"), "concat should contain error message")

		local msg2 = err .. " (fatal)"
		assert(type(msg2) == "string", "concat should produce string")
	`)
	if err != nil {
		t.Fatalf("concat test failed: %v", err)
	}
}

func TestStructuredError_ErrorMethods(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenErrors(L)

	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			Build(),
		name: "TestRec",
	}
	L.SetGlobal("TestRec", rec)

	err := L.DoString(`
		local val, err = TestRec:is({name = 123})
		assert(val == nil)
		assert(err ~= nil)

		-- kind should be Invalid
		assert(err:kind() == "Invalid", "kind should be 'Invalid', got: " .. tostring(err:kind()))

		-- message method
		local msg = err:message()
		assert(msg:find("expected string"), "message should mention 'expected string'")

		-- details method returns structured info
		local d = err:details()
		assert(d ~= nil, "details should not be nil")
		assert(d.field == "name", "details.field should be 'name', got: " .. tostring(d.field))
		assert(d.expected == "string", "details.expected should be 'string', got: " .. tostring(d.expected))
		assert(d.got == "number", "details.got should be 'number', got: " .. tostring(d.got))
	`)
	if err != nil {
		t.Fatalf("error methods test failed: %v", err)
	}
}

func TestStructuredError_UnresolvedRef(t *testing.T) {
	L := NewState()
	defer L.Close()

	refType := &LType{inner: typ.NewRef("", "Missing")}

	isMethod := L.typeGetField(refType, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	_ = L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	msg := errMessage(errVal)
	if !strings.Contains(msg, "unresolved") {
		t.Errorf("should mention 'unresolved', got: %s", msg)
	}
	if !strings.Contains(msg, "Missing") {
		t.Errorf("should mention type name 'Missing', got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// Group 19: Record with map component
// ---------------------------------------------------------------------------

func TestRecordWithMapComponent(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {name: string, [string]: number} — record with known fields AND dynamic map
	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			MapComponent(typ.String, typ.Number).
			Build(),
	}

	valid := L.NewTable()
	valid.RawSetString("name", LString("test"))
	valid.RawSetString("score", LNumber(42))
	valid.RawSetString("count", LNumber(7))

	badMapVal := L.NewTable()
	badMapVal.RawSetString("name", LString("test"))
	badMapVal.RawSetString("extra", LString("not a number"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid with extra number fields", valid, true},
		{"invalid extra field type", badMapVal, false},
		{"only required fields", func() LValue {
			t := L.NewTable()
			t.RawSetString("name", LString("x"))
			return t
		}(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRecordWithMapComponentIs(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			MapComponent(typ.String, typ.Number).
			Build(),
	}

	bad := L.NewTable()
	bad.RawSetString("name", LString("test"))
	bad.RawSetString("extra", LString("not a number"))

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Error("should fail")
	}
	if errField(errVal) == "" {
		t.Error("error should have a field path")
	}
}

// ---------------------------------------------------------------------------
// Group 20: Edge cases for number/integer cross-type validation
// ---------------------------------------------------------------------------

func TestNumberIntegerCrossType(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		name  string
		typ   *LType
		value LValue
		ok    bool
	}{
		// number accepts both
		{"number accepts LNumber", LTypeNumber, LNumber(42.5), true},
		{"number accepts LInteger", LTypeNumber, LInteger(42), true},
		{"number accepts LNumber zero", LTypeNumber, LNumber(0), true},
		{"number accepts LInteger zero", LTypeNumber, LInteger(0), true},

		// integer is strict
		{"integer accepts LInteger", LTypeInteger, LInteger(42), true},
		{"integer accepts whole LNumber", LTypeInteger, LNumber(42.0), true},
		{"integer rejects fractional", LTypeInteger, LNumber(42.5), false},
		{"integer rejects NaN-like", LTypeInteger, LNumber(0.1 + 0.2), false}, // 0.30000000000000004

		// edge: very large numbers
		{"integer accepts max int", LTypeInteger, LInteger(9223372036854775807), true},
		{"integer accepts min int", LTypeInteger, LInteger(-9223372036854775808), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 21: Union type with literals (discriminated union pattern)
// ---------------------------------------------------------------------------

func TestUnionLiterals(t *testing.T) {
	L := NewState()
	defer L.Close()

	// "active" | "draft" | "archived"
	statusUnion := &LType{
		inner: typ.NewUnion(
			typ.LiteralString("active"),
			typ.LiteralString("draft"),
			typ.LiteralString("archived"),
		),
	}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"active passes", LString("active"), true},
		{"draft passes", LString("draft"), true},
		{"archived passes", LString("archived"), true},
		{"unknown fails", LString("deleted"), false},
		{"number fails", LNumber(1), false},
		{"empty string fails", LString(""), false},
		{"nil fails", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusUnion.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestUnionLiteralsErrorDetail(t *testing.T) {
	L := NewState()
	defer L.Close()

	statusUnion := &LType{
		inner: typ.NewUnion(
			typ.LiteralString("active"),
			typ.LiteralString("draft"),
		),
	}

	isMethod := L.typeGetField(statusUnion, "is")
	L.Push(isMethod)
	L.Push(LString("invalid"))
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}

	msg := errMessage(errVal)
	// Error should show the union type, not individual members
	if !strings.Contains(msg, "expected") {
		t.Errorf("should contain 'expected', got: %s", msg)
	}
	if errGot(errVal) != "string" {
		t.Errorf("got should be 'string', got: %q", errGot(errVal))
	}
}

// ---------------------------------------------------------------------------
// Group 22: Optional(Optional(T)) normalization
// ---------------------------------------------------------------------------

func TestDoubleOptional(t *testing.T) {
	L := NewState()
	defer L.Close()

	// number?? should normalize to number? (NewOptional already handles this)
	inner := typ.NewOptional(typ.Number)
	outer := typ.NewOptional(inner)

	doubleOpt := &LType{inner: outer}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"number passes", LNumber(42), true},
		{"nil passes", LNil, true},
		{"string fails", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := doubleOpt.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 23: Ref chain resolution
// ---------------------------------------------------------------------------

func TestRefChainResolution(t *testing.T) {
	L := NewState()
	defer L.Close()

	// A -> B -> number (alias chain through refs)
	resolver := &typeResolver{
		types: map[string]typ.Type{
			"A": typ.NewAlias("A", typ.NewRef("", "B")),
			"B": typ.NewAlias("B", typ.Number),
		},
	}

	refType := &LType{
		inner:    typ.NewRef("", "A"),
		resolver: resolver,
	}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"number passes through chain", LNumber(42), true},
		{"string fails through chain", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 24: Full Lua integration with structured errors
// ---------------------------------------------------------------------------

func TestLuaIntegration_StructuredErrors(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenErrors(L)

	personType := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			Field("age", typ.Number).
			OptField("email", typ.String).
			Build(),
		name: "Person",
	}
	L.SetGlobal("Person", personType)

	err := L.DoString(`
		-- Valid input
		local p, err = Person:is({name = "Alice", age = 30})
		assert(p ~= nil, "valid person should pass")
		assert(err == nil, "valid person should have no error")

		-- Missing required field
		local p2, err2 = Person:is({name = "Bob"})
		assert(p2 == nil, "missing age should fail")
		assert(err2 ~= nil, "should have error")
		local d2 = err2:details()
		assert(d2.field == "age", "field should be 'age', got: " .. tostring(d2.field))
		assert(err2:message():find("required"), "should mention 'required': " .. err2:message())

		-- Wrong type
		local p3, err3 = Person:is({name = 123, age = 30})
		assert(p3 == nil, "wrong name type should fail")
		local d3 = err3:details()
		assert(d3.field == "name", "field should be 'name'")
		assert(d3.expected == "string", "expected should be 'string'")
		assert(d3.got == "number", "got should be 'number'")

		-- Optional field present but wrong type
		local p4, err4 = Person:is({name = "Carol", age = 25, email = 123})
		assert(p4 == nil, "wrong email type should fail")
		local d4 = err4:details()
		assert(d4.field == "email", "field should be 'email'")

		-- Not a table
		local p5, err5 = Person:is("not a table")
		assert(p5 == nil, "string should fail")
		local d5 = err5:details()
		assert(d5.got == "string", "got should be 'string'")

		-- Error kind is Invalid
		assert(err5:kind() == "Invalid", "kind should be Invalid")
	`)
	if err != nil {
		t.Fatalf("Lua integration test failed: %v", err)
	}
}

func TestLuaIntegration_NestedStructuredErrors(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenErrors(L)

	addrType := typ.NewRecord().
		Field("street", typ.String).
		Field("zip", typ.String).
		Build()
	personType := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			Field("address", addrType).
			Build(),
		name: "Person",
	}
	L.SetGlobal("Person", personType)

	err := L.DoString(`
		local p, err = Person:is({
			name = "Alice",
			address = { street = "Main St", zip = 12345 }
		})
		assert(p == nil, "should fail")
		local d = err:details()
		assert(d.field == "address.zip", "field should be 'address.zip', got: " .. tostring(d.field))
		assert(d.expected == "string", "expected should be 'string'")
		assert(d.got == "number", "got should be 'number'")
	`)
	if err != nil {
		t.Fatalf("nested structured errors test failed: %v", err)
	}
}

func TestLuaIntegration_AnnotationErrors(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenErrors(L)

	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("score", typ.Number, false, []typ.Annotation{
				{Name: "min", Arg: float64(0)},
				{Name: "max", Arg: float64(100)},
			}).
			AnnotatedField("name", typ.String, false, []typ.Annotation{
				{Name: "min_len", Arg: float64(1)},
			}).
			Build(),
		name: "Input",
	}
	L.SetGlobal("Input", rec)

	err := L.DoString(`
		-- Score below min
		local v, err = Input:is({score = -5, name = "test"})
		assert(v == nil)
		local d = err:details()
		assert(d.field == "score", "field should be 'score', got: " .. tostring(d.field))
		assert(d.constraint == "min", "constraint should be 'min', got: " .. tostring(d.constraint))

		-- Name too short
		local v2, err2 = Input:is({score = 50, name = ""})
		assert(v2 == nil)
		local d2 = err2:details()
		assert(d2.field == "name", "field should be 'name', got: " .. tostring(d2.field))
		assert(d2.constraint == "min_len", "constraint should be 'min_len', got: " .. tostring(d2.constraint))

		-- All valid
		local v3, err3 = Input:is({score = 50, name = "test"})
		assert(v3 ~= nil, "should pass")
		assert(err3 == nil, "should have no error")
	`)
	if err != nil {
		t.Fatalf("annotation errors test failed: %v", err)
	}
}

// ===========================================================================
// REMAINING TYPE COVERAGE TESTS
// ===========================================================================

// ---------------------------------------------------------------------------
// Group 25: Recursive type validation
// ---------------------------------------------------------------------------

func TestRecursiveTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// type Node = { value: number, next: Node? }
	nodeType := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.Number).
			OptField("next", self).
			Build()
	})

	recType := &LType{inner: nodeType, name: "Node"}

	// Single node
	single := L.NewTable()
	single.RawSetString("value", LNumber(1))

	// Two-node chain
	second := L.NewTable()
	second.RawSetString("value", LNumber(2))
	chain := L.NewTable()
	chain.RawSetString("value", LNumber(1))
	chain.RawSetString("next", second)

	// Invalid node (value is string)
	invalid := L.NewTable()
	invalid.RawSetString("value", LString("bad"))

	// Invalid next (not a table)
	badNext := L.NewTable()
	badNext.RawSetString("value", LNumber(1))
	badNext.RawSetString("next", LString("not a table"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"single node", single, true},
		{"two-node chain", chain, true},
		{"invalid value type", invalid, false},
		{"invalid next type", badNext, false},
		{"not a table", LNumber(42), false},
		{"nil fails", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRecursiveTypeValidationIs(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	nodeType := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.Number).
			OptField("next", self).
			Build()
	})
	recType := &LType{inner: nodeType, name: "Node"}

	chain := L.NewTable()
	chain.RawSetString("value", LNumber(1))
	next := L.NewTable()
	next.RawSetString("value", LNumber(2))
	chain.RawSetString("next", next)

	isMethod := L.typeGetField(recType, "is")
	L.Push(isMethod)
	L.Push(chain)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Errorf("recursive type should validate chain; error: %v", errMessage(errVal))
	}
}

func TestRecursiveOptionalType(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Node? — optional recursive
	nodeType := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.Number).
			OptField("next", self).
			Build()
	})
	optNode := &LType{inner: typ.NewOptional(nodeType)}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"nil passes", LNil, true},
		{"valid node passes", func() LValue {
			t := L.NewTable()
			t.RawSetString("value", LNumber(1))
			return t
		}(), true},
		{"string fails", LString("nope"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optNode.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 26: Sum type validation
// ---------------------------------------------------------------------------

func TestSumTypeValidationFull(t *testing.T) {
	L := NewState()
	defer L.Close()

	sumType := &LType{
		inner: typ.NewSum("Color", []typ.Variant{
			{Tag: "Red", Types: nil},
			{Tag: "Green", Types: nil},
			{Tag: "RGB", Types: []typ.Type{typ.Number, typ.Number, typ.Number}},
		}),
		name: "Color",
	}

	tbl := L.NewTable()
	tbl.RawSetString("tag", LString("Red"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"table passes", tbl, true},
		{"empty table passes", L.NewTable(), true},
		{"string fails", LString("Red"), false},
		{"number fails", LNumber(1), false},
		{"nil fails", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sumType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestSumTypeIs(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	sumType := &LType{
		inner: typ.NewSum("Result", []typ.Variant{
			{Tag: "Ok", Types: []typ.Type{typ.String}},
			{Tag: "Err", Types: []typ.Type{typ.String}},
		}),
		name: "Result",
	}

	isMethod := L.typeGetField(sumType, "is")

	// Table passes
	tbl := L.NewTable()
	L.Push(isMethod)
	L.Push(tbl)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)
	if val == LNil {
		t.Errorf("sum should accept table; error: %v", errMessage(errVal))
	}

	// Non-table fails with structured error
	L.Push(isMethod)
	L.Push(LString("not a table"))
	L.Call(1, 2)
	val = L.Get(-2)
	errVal = L.Get(-1)
	L.Pop(2)
	if val != LNil {
		t.Error("sum should reject string")
	}
	if errGot(errVal) != "string" {
		t.Errorf("got should be 'string', got %q", errGot(errVal))
	}
}

// ---------------------------------------------------------------------------
// Group 27: Platform type validation
// ---------------------------------------------------------------------------

func TestPlatformTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	fileType := &LType{inner: typ.NewPlatform("File"), name: "File"}

	ud := &LUserData{Value: "file handle"}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"userdata passes", ud, true},
		{"table fails", L.NewTable(), false},
		{"string fails", LString("file"), false},
		{"number fails", LNumber(0), false},
		{"nil fails", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestOptionalPlatformType(t *testing.T) {
	L := NewState()
	defer L.Close()

	optFile := &LType{inner: typ.NewOptional(typ.NewPlatform("File"))}
	ud := &LUserData{Value: "file handle"}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"userdata passes", ud, true},
		{"nil passes", LNil, true},
		{"string fails", LString("file"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optFile.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 28: Annotation edge cases
// ---------------------------------------------------------------------------

func TestAnnotation_OnOptionalField(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {name: string @min_len(1)?} — optional field with annotated type
	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("name", typ.String, true, []typ.Annotation{
				{Name: "min_len", Arg: float64(1)},
			}).
			Build(),
	}

	// Missing optional field — should pass
	empty := L.NewTable()
	if !rec.Validate(L, empty) {
		t.Error("missing optional annotated field should pass")
	}

	// Present but valid
	valid := L.NewTable()
	valid.RawSetString("name", LString("hello"))
	if !rec.Validate(L, valid) {
		t.Error("valid annotated field should pass")
	}

	// Present but fails annotation
	tooShort := L.NewTable()
	tooShort.RawSetString("name", LString(""))
	if rec.Validate(L, tooShort) {
		t.Error("empty name should fail min_len annotation")
	}
}

func TestAnnotation_PatternOnField(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("email", typ.String, false, []typ.Annotation{
				{Name: "pattern", Arg: "^[^@]+@[^@]+$"},
			}).
			Build(),
	}

	valid := L.NewTable()
	valid.RawSetString("email", LString("user@example.com"))

	bad := L.NewTable()
	bad.RawSetString("email", LString("invalid"))

	isMethod := L.typeGetField(rec, "is")

	// Valid
	L.Push(isMethod)
	L.Push(valid)
	L.Call(1, 2)
	val := L.Get(-2)
	L.Pop(2)
	if val == LNil {
		t.Error("valid email should pass")
	}

	// Invalid
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val = L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)
	if val != LNil {
		t.Error("invalid email should fail")
	}
	if errConstraint(errVal) != "pattern" {
		t.Errorf("constraint should be 'pattern', got %q", errConstraint(errVal))
	}
	if errField(errVal) != "email" {
		t.Errorf("field should be 'email', got %q", errField(errVal))
	}
}

func TestAnnotation_MultipleOnSameField(t *testing.T) {
	L := NewState()
	defer L.Close()

	// score: number @min(0) @max(100) — multiple annotations
	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("score", typ.Number, false, []typ.Annotation{
				{Name: "min", Arg: float64(0)},
				{Name: "max", Arg: float64(100)},
			}).
			Build(),
	}

	tests := []struct {
		name  string
		score float64
		ok    bool
	}{
		{"in range", 50, true},
		{"at min", 0, true},
		{"at max", 100, true},
		{"below min", -1, false},
		{"above max", 101, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tbl := L.NewTable()
			tbl.RawSetString("score", LNumber(tt.score))
			if got := rec.Validate(L, tbl); got != tt.ok {
				t.Errorf("Validate(%v) = %v, want %v", tt.score, got, tt.ok)
			}
		})
	}
}

func TestAnnotation_MinLenOnArray(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {string} @min_len(1) — array must have at least 1 element
	arrType := &LType{
		inner: typ.NewAnnotated(typ.NewArray(typ.String), []typ.Annotation{
			{Name: "min_len", Arg: float64(1)},
		}),
	}

	empty := L.NewTable()
	nonempty := L.NewTable()
	nonempty.Append(LString("hello"))

	if arrType.Validate(L, empty) {
		t.Error("empty array should fail min_len(1)")
	}
	if !arrType.Validate(L, nonempty) {
		t.Error("non-empty array should pass min_len(1)")
	}
}

// ---------------------------------------------------------------------------
// Group 29: Map with number keys — Array part
// ---------------------------------------------------------------------------

func TestMapNumberKeys_ArrayPart(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {[number]: string} — number-keyed map
	numMap := &LType{inner: typ.NewMap(typ.Number, typ.String)}

	// Table with entries in Array part (via Append)
	tbl := L.NewTable()
	tbl.Append(LString("first"))
	tbl.Append(LString("second"))

	// Array part entries have implicit integer keys 1, 2, ...
	// For a {[number]: string} map, these should be valid
	// Currently only Dict and Strdict are checked — Array is missed
	if !numMap.Validate(L, tbl) {
		t.Error("{[number]: string} should accept table with array-part string values")
	}
}

// ---------------------------------------------------------------------------
// Group 30: Record field lookup in Dict (non-Strdict)
// ---------------------------------------------------------------------------

func TestRecordFieldLookupDict(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			Build(),
	}

	// Table with "name" stored via RawSet with LString key (goes to Dict, not Strdict)
	tbl := L.NewTable()
	tbl.RawSet(LString("name"), LString("hello"))

	// This entry is in Strdict because RawSet with LString puts it in Strdict
	// (at least in this implementation). Verify the table works:
	if !rec.Validate(L, tbl) {
		t.Error("record should find field in table set via RawSet(LString)")
	}
}

// ---------------------------------------------------------------------------
// Group 31: Open record accepts unknown fields
// ---------------------------------------------------------------------------

func TestOpenRecord(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Open record: {name: string, ...}
	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			SetOpen(true).
			Build(),
	}

	// Table with extra fields
	tbl := L.NewTable()
	tbl.RawSetString("name", LString("test"))
	tbl.RawSetString("extra", LNumber(42))
	tbl.RawSetString("another", LTrue)

	if !rec.Validate(L, tbl) {
		t.Error("open record should accept extra fields")
	}
}

// ---------------------------------------------------------------------------
// Group 32: Empty record
// ---------------------------------------------------------------------------

func TestEmptyRecord(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {} — empty record accepts any table
	rec := &LType{inner: typ.NewRecord().Build()}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"empty table", L.NewTable(), true},
		{"table with fields", func() LValue {
			t := L.NewTable()
			t.RawSetString("x", LNumber(1))
			return t
		}(), true},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 33: Instantiated generic at runtime
// ---------------------------------------------------------------------------

func TestInstantiatedGenericValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Generic Array<T> instantiated as Array<number>
	generic := typ.NewGeneric("Array", []*typ.TypeParam{
		typ.NewTypeParam("T", nil),
	}, typ.NewArray(typ.NewTypeParam("T", nil)))

	instantiated := typ.Instantiate(generic, typ.Number)
	instType := &LType{inner: instantiated}

	valid := L.NewTable()
	valid.Append(LNumber(1))
	valid.Append(LNumber(2))

	invalid := L.NewTable()
	invalid.Append(LString("bad"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"valid number array", valid, true},
		{"invalid string element", invalid, false},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instType.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 34: Annotation validator edge cases
// ---------------------------------------------------------------------------

func TestAnnotation_MinOnInteger(t *testing.T) {
	L := NewState()
	defer L.Close()

	ann := &LType{inner: typ.NewAnnotated(typ.Integer, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
	})}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"positive LInteger", LInteger(42), true},
		{"zero LInteger", LInteger(0), true},
		{"negative LInteger", LInteger(-1), false},
		{"positive LNumber whole", LNumber(5.0), true},
		{"negative LNumber whole", LNumber(-5.0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ann.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAnnotation_MaxOnInteger(t *testing.T) {
	L := NewState()
	defer L.Close()

	ann := &LType{inner: typ.NewAnnotated(typ.Integer, []typ.Annotation{
		{Name: "max", Arg: float64(100)},
	})}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"within range", LInteger(50), true},
		{"at max", LInteger(100), true},
		{"above max", LInteger(101), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ann.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAnnotation_MinLenOnString(t *testing.T) {
	L := NewState()
	defer L.Close()

	ann := &LType{inner: typ.NewAnnotated(typ.String, []typ.Annotation{
		{Name: "min_len", Arg: float64(3)},
	})}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"exact length", LString("abc"), true},
		{"longer", LString("abcdef"), true},
		{"too short", LString("ab"), false},
		{"empty", LString(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ann.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAnnotation_MaxLenZero(t *testing.T) {
	L := NewState()
	defer L.Close()

	ann := &LType{inner: typ.NewAnnotated(typ.String, []typ.Annotation{
		{Name: "max_len", Arg: float64(0)},
	})}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"empty string passes", LString(""), true},
		{"non-empty fails", LString("a"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ann.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAnnotation_MinLenOnTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	// @min_len on table checks LTable.Len() which counts the array part
	ann := &LType{inner: typ.NewAnnotated(typ.NewInterface("table", nil), []typ.Annotation{
		{Name: "min_len", Arg: float64(1)},
	})}

	empty := L.NewTable()
	withArray := L.NewTable()
	withArray.Append(LString("a"))

	// Table with only strdict entries — Len() returns 0
	strdictOnly := L.NewTable()
	strdictOnly.RawSetString("key", LString("val"))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"empty table fails", empty, false},
		{"table with array element passes", withArray, true},
		// Len() only counts array part, strdict-only table has Len()=0
		{"table with only strdict fails min_len", strdictOnly, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ann.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAnnotation_PatternEdgeCases(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	tests := []struct {
		name    string
		pattern string
		value   string
		ok      bool
	}{
		{"dot star matches anything", ".*", "anything", true},
		{"dot star matches empty", ".*", "", true},
		{"caret dollar exact", "^hello$", "hello", true},
		{"caret dollar rejects extra", "^hello$", "hello world", false},
		{"email-like pattern", "^[^@]+@[^@]+\\.[^@]+$", "user@example.com", true},
		{"email-like rejects bare", "^[^@]+@[^@]+\\.[^@]+$", "notanemail", false},
		{"uuid pattern", "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$",
			"550e8400-e29b-41d4-a716-446655440000", true},
		{"uuid pattern rejects", "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$",
			"not-a-uuid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ann := &LType{inner: typ.NewAnnotated(typ.String, []typ.Annotation{
				{Name: "pattern", Arg: tt.pattern},
			})}
			if got := ann.Validate(L, LString(tt.value)); got != tt.ok {
				t.Errorf("Validate(%q) = %v, want %v", tt.value, got, tt.ok)
			}
		})
	}
}

func TestAnnotation_PatternStructuredError(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("code", typ.String, false, []typ.Annotation{
				{Name: "pattern", Arg: "^[A-Z]{3}$"},
			}).
			Build(),
	}

	bad := L.NewTable()
	bad.RawSetString("code", LString("lowercase"))

	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(bad)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Fatal("should fail")
	}
	if errField(errVal) != "code" {
		t.Errorf("field=%q, want 'code'", errField(errVal))
	}
	if errConstraint(errVal) != "pattern" {
		t.Errorf("constraint=%q, want 'pattern'", errConstraint(errVal))
	}
}

func TestAnnotation_WrongBaseType(t *testing.T) {
	L := NewState()
	defer L.Close()

	// @min on a string type — the min validator silently passes
	// because toNumber fails on string. The TYPE check catches the error.
	ann := &LType{inner: typ.NewAnnotated(typ.String, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
	})}

	// String value: type check passes, min annotation silently passes (can't extract number)
	if !ann.Validate(L, LString("hello")) {
		t.Error("string @min(0) should pass for string values (min is a no-op on strings)")
	}

	// Number value: type check (string) fails before annotations run
	if ann.Validate(L, LNumber(42)) {
		t.Error("string @min(0) should fail for number values (type mismatch)")
	}
}

func TestAnnotation_MinLenOnWrongType(t *testing.T) {
	L := NewState()
	defer L.Close()

	// @min_len on number type — length check silently passes
	ann := &LType{inner: typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min_len", Arg: float64(1)},
	})}

	// Number passes type check, min_len silently passes (can't get length of number)
	if !ann.Validate(L, LNumber(42)) {
		t.Error("number @min_len(1) should pass (min_len is a no-op on numbers)")
	}
}

func TestAnnotation_CombinedRecord(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	// Full real-world-like record with multiple annotated fields
	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("name", typ.String, false, []typ.Annotation{
				{Name: "min_len", Arg: float64(1)},
				{Name: "max_len", Arg: float64(255)},
			}).
			AnnotatedField("email", typ.String, false, []typ.Annotation{
				{Name: "pattern", Arg: "^[^@]+@[^@]+$"},
			}).
			AnnotatedField("age", typ.Number, false, []typ.Annotation{
				{Name: "min", Arg: float64(0)},
				{Name: "max", Arg: float64(200)},
			}).
			AnnotatedField("bio", typ.String, true, []typ.Annotation{
				{Name: "max_len", Arg: float64(1000)},
			}).
			Build(),
		name: "UserInput",
	}
	L.SetGlobal("UserInput", rec)

	err := L.DoString(`
		-- Valid input
		local v, e = UserInput:is({name = "Alice", email = "alice@example.com", age = 30})
		assert(v ~= nil, "valid input should pass")

		-- With optional bio
		local v2, e2 = UserInput:is({name = "Bob", email = "bob@example.com", age = 25, bio = "Hi there"})
		assert(v2 ~= nil, "valid input with bio should pass")

		-- Without optional bio
		local v3, e3 = UserInput:is({name = "Carol", email = "carol@example.com", age = 28})
		assert(v3 ~= nil, "valid input without bio should pass")

		-- Empty name fails min_len
		local v4, e4 = UserInput:is({name = "", email = "d@e.com", age = 20})
		assert(v4 == nil, "empty name should fail")
		local d4 = e4:details()
		assert(d4.field == "name", "field should be 'name'")
		assert(d4.constraint == "min_len", "constraint should be 'min_len'")

		-- Bad email fails pattern
		local v5, e5 = UserInput:is({name = "Eve", email = "notanemail", age = 20})
		assert(v5 == nil, "bad email should fail")
		local d5 = e5:details()
		assert(d5.field == "email", "field should be 'email'")
		assert(d5.constraint == "pattern", "constraint should be 'pattern'")

		-- Negative age fails min
		local v6, e6 = UserInput:is({name = "Frank", email = "f@g.com", age = -1})
		assert(v6 == nil, "negative age should fail")
		local d6 = e6:details()
		assert(d6.field == "age", "field should be 'age'")
		assert(d6.constraint == "min", "constraint should be 'min'")

		-- Age above max
		local v7, e7 = UserInput:is({name = "Grace", email = "g@h.com", age = 300})
		assert(v7 == nil, "age 300 should fail")
		local d7 = e7:details()
		assert(d7.constraint == "max", "constraint should be 'max'")
	`)
	if err != nil {
		t.Fatalf("combined annotation test failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Group 35: Map Array-part rejection
// ---------------------------------------------------------------------------

func TestMapNumberKeys_ArrayPart_BadValue(t *testing.T) {
	L := NewState()
	defer L.Close()

	numMap := &LType{inner: typ.NewMap(typ.Number, typ.String)}

	tbl := L.NewTable()
	tbl.Append(LString("ok"))
	tbl.Append(LNumber(42)) // wrong: value should be string

	if numMap.Validate(L, tbl) {
		t.Error("{[number]: string} should reject number values in Array part")
	}
}

func TestMapNumberKeys_ArrayPart_Is(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	numMap := &LType{inner: typ.NewMap(typ.Number, typ.String)}

	tbl := L.NewTable()
	tbl.Append(LNumber(42)) // wrong value type

	isMethod := L.typeGetField(numMap, "is")
	L.Push(isMethod)
	L.Push(tbl)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Error("should fail")
	}
	if errField(errVal) != "[1]" {
		t.Errorf("field should be '[1]', got %q", errField(errVal))
	}
}

func TestMapStringKeys_IgnoresArrayPart(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {[string]: number} — Array part should be ignored for string-keyed maps
	strMap := &LType{inner: typ.NewMap(typ.String, typ.Number)}

	tbl := L.NewTable()
	tbl.RawSetString("a", LNumber(1))
	tbl.Append(LString("stray")) // in Array part, should be ignored

	if !strMap.Validate(L, tbl) {
		t.Error("{[string]: number} should ignore Array part entries")
	}
}

// ---------------------------------------------------------------------------
// Group 35: Recursive depth limit
// ---------------------------------------------------------------------------

func TestRecursiveDepthLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	// A recursive type that would infinite loop without depth limit.
	// Body is the recursive type itself (no Optional wrapper to break recursion).
	rec := typ.NewRecursivePlaceholder("Loop")
	rec.SetBody(rec) // self-referential body

	loopType := &LType{inner: rec}

	// Should not hang — depth limit should kick in and return false
	result := loopType.Validate(L, L.NewTable())
	_ = result // We don't care about the result, just that it terminates
}

// ---------------------------------------------------------------------------
// Group 36: typeCall vs :is() consistency
// ---------------------------------------------------------------------------

func TestTypeCall_ErrorFormat(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	rec := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", rec)

	// Type(value) should error on invalid input
	err := L.DoString(`
		local ok, err = pcall(function()
			Point("not a table")
		end)
		assert(not ok, "should error")
	`)
	if err != nil {
		t.Fatalf("typeCall error test failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Group 37: Tuple edge cases
// ---------------------------------------------------------------------------

func TestTupleEmpty(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Empty tuple () — should accept empty table
	emptyTuple := &LType{inner: typ.NewTuple()}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"empty table passes", L.NewTable(), true},
		{"non-empty table passes", func() LValue {
			t := L.NewTable()
			t.Append(LNumber(1))
			return t
		}(), true},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emptyTuple.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestTupleExtraElements(t *testing.T) {
	L := NewState()
	defer L.Close()

	// (number, string) — table with 3 elements should still pass (extra ignored)
	tuple := &LType{inner: typ.NewTuple(typ.Number, typ.String)}

	tbl := L.NewTable()
	tbl.Append(LNumber(1))
	tbl.Append(LString("hello"))
	tbl.Append(LTrue) // extra, should be ignored

	if !tuple.Validate(L, tbl) {
		t.Error("tuple with extra elements should pass")
	}
}

func TestTupleNilHoles(t *testing.T) {
	L := NewState()
	defer L.Close()

	// (number, string) — table with nil in position 2
	tuple := &LType{inner: typ.NewTuple(typ.Number, typ.NewOptional(typ.String))}

	tbl := L.NewTable()
	tbl.Append(LNumber(1))
	// Position 2 is missing (nil) — optional so should pass

	if !tuple.Validate(L, tbl) {
		t.Error("tuple with optional nil element should pass")
	}
}

// ---------------------------------------------------------------------------
// Group 38: Both field.Optional and Optional(T) type simultaneously
// ---------------------------------------------------------------------------

func TestRecordFieldOptionalAndTypeOptional(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Field is optional AND type is Optional(number) — double optional
	rec := &LType{
		inner: typ.NewRecord().
			OptField("count", typ.NewOptional(typ.Number)).
			Build(),
	}

	tests := []struct {
		name  string
		value *LTable
		ok    bool
	}{
		{"field absent", &LTable{}, true},
		{"field present with number", func() *LTable {
			t := &LTable{Strdict: map[string]LValue{"count": LNumber(5)}}
			return t
		}(), true},
		{"field present with nil", func() *LTable {
			// Setting nil in strdict doesn't actually store, so field is absent
			return &LTable{}
		}(), true},
		{"field present with wrong type", func() *LTable {
			return &LTable{Strdict: map[string]LValue{"count": LString("bad")}}
		}(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 39: Generic type rejection
// ---------------------------------------------------------------------------

func TestGenericTypeRejectsAll(t *testing.T) {
	L := NewState()
	defer L.Close()

	generic := &LType{inner: typ.NewGeneric("Container", []*typ.TypeParam{
		typ.NewTypeParam("T", nil),
	}, typ.NewArray(typ.NewTypeParam("T", nil)))}

	// Uninstantiated generic should reject everything
	values := []LValue{LNil, LTrue, LNumber(0), LString(""), L.NewTable()}
	for _, v := range values {
		if generic.Validate(L, v) {
			t.Errorf("uninstantiated generic should reject %v", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Group 40: Sparse arrays
// ---------------------------------------------------------------------------

func TestArraySparse(t *testing.T) {
	L := NewState()
	defer L.Close()

	arrType := &LType{inner: typ.NewArray(typ.Number)}

	// Array with nil holes — nils should be skipped
	tbl := &LTable{Array: []LValue{LNumber(1), LNil, LNumber(3), LNil, LNumber(5)}}

	if !arrType.Validate(L, tbl) {
		t.Error("sparse array with nil holes should pass (nils are skipped)")
	}
}

func TestArraySparseInvalid(t *testing.T) {
	L := NewState()
	defer L.Close()

	arrType := &LType{inner: typ.NewArray(typ.Number)}

	// Array with nil holes AND a wrong-type element
	tbl := &LTable{Array: []LValue{LNumber(1), LNil, LString("bad"), LNil, LNumber(5)}}

	if arrType.Validate(L, tbl) {
		t.Error("sparse array with wrong-type non-nil element should fail")
	}
}

// ---------------------------------------------------------------------------
// Group 41: Validate and Is consistency
// ---------------------------------------------------------------------------

func TestValidateAndIsConsistency(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	types := []struct {
		name string
		typ  *LType
	}{
		{"number", LTypeNumber},
		{"string", LTypeString},
		{"boolean", LTypeBoolean},
		{"optional number", &LType{inner: typ.NewOptional(typ.Number)}},
		{"table", &LType{inner: typ.NewInterface("table", nil)}},
		{"array", &LType{inner: typ.NewArray(typ.Number)}},
	}

	values := []LValue{
		LNil, LTrue, LFalse, LNumber(42), LInteger(42), LString("hello"),
		L.NewTable(), &LUserData{Value: "x"},
	}

	for _, tt := range types {
		for _, v := range values {
			validateResult := tt.typ.Validate(L, v)

			isMethod := L.typeGetField(tt.typ, "is")
			L.Push(isMethod)
			L.Push(v)
			L.Call(1, 2)
			isVal := L.Get(-2)
			isErr := L.Get(-1)
			L.Pop(2)

			isResult := isVal != LNil || (isVal == LNil && isErr == LNil)

			// For optional types, nil is valid — :is(nil) returns (nil, nil)
			if v == LNil && isErr == LNil {
				isResult = true
			}

			if validateResult != isResult {
				t.Errorf("%s / %v: Validate()=%v but :is() returned val=%v err=%v",
					tt.name, v, validateResult, isVal, errMessage(isErr))
			}
		}
	}
}

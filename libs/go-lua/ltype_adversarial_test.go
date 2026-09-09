package lua

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// ---------------------------------------------------------------------------
// Adversarial: NaN, Inf, negative zero
// ---------------------------------------------------------------------------

func TestAdversarial_NaN(t *testing.T) {
	L := NewState()
	defer L.Close()

	nan := LNumber(math.NaN())

	// NaN should pass number type check (it IS a number)
	if !LTypeNumber.Validate(L, nan) {
		t.Error("NaN is a number")
	}

	// NaN should fail integer check (NaN is not an integer)
	if LTypeInteger.Validate(L, nan) {
		t.Error("NaN should not be an integer")
	}

	// NaN in @min/@max: NaN < anything is false, NaN > anything is false
	// So @min(0) should pass for NaN (n < minVal is false)
	// This is a known IEEE 754 behavior — NaN comparisons are always false
	minType := &LType{inner: typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
	})}
	// NaN is a number, and `NaN < 0` is false so min validator passes
	if !minType.Validate(L, nan) {
		t.Error("NaN passes @min(0) due to IEEE 754 NaN comparison semantics")
	}

	// NaN literal matching — NaN != NaN by IEEE 754
	nanLiteral := &LType{inner: typ.LiteralNumber(math.NaN())}
	if nanLiteral.Validate(L, nan) {
		t.Error("NaN literal should not match NaN value (NaN != NaN)")
	}
}

func TestAdversarial_Inf(t *testing.T) {
	L := NewState()
	defer L.Close()

	posInf := LNumber(math.Inf(1))
	negInf := LNumber(math.Inf(-1))

	// Inf is a number
	if !LTypeNumber.Validate(L, posInf) {
		t.Error("+Inf is a number")
	}
	if !LTypeNumber.Validate(L, negInf) {
		t.Error("-Inf is a number")
	}

	// Inf is not an integer
	if LTypeInteger.Validate(L, posInf) {
		t.Error("+Inf should not be an integer")
	}

	// @max(100) should reject +Inf
	maxType := &LType{inner: typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "max", Arg: float64(100)},
	})}
	if maxType.Validate(L, posInf) {
		t.Error("+Inf should fail @max(100)")
	}

	// @min(0) should reject -Inf
	minType := &LType{inner: typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
	})}
	if minType.Validate(L, negInf) {
		t.Error("-Inf should fail @min(0)")
	}
}

func TestAdversarial_NegativeZero(t *testing.T) {
	L := NewState()
	defer L.Close()

	negZero := LNumber(math.Copysign(0, -1))

	if !LTypeNumber.Validate(L, negZero) {
		t.Error("-0 is a number")
	}

	// -0 == 0 in IEEE 754, so integer check should pass
	if !LTypeInteger.Validate(L, negZero) {
		t.Error("-0 should pass integer check (IsIntegerValue)")
	}

	// Literal 0 should match -0 (they're equal in IEEE 754)
	zeroLit := &LType{inner: typ.LiteralNumber(0)}
	if !zeroLit.Validate(L, negZero) {
		t.Error("literal 0 should match -0")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: huge tables
// ---------------------------------------------------------------------------

func TestAdversarial_HugeArray(t *testing.T) {
	L := NewState()
	defer L.Close()

	arrType := &LType{inner: typ.NewArray(typ.Number)}

	tbl := &LTable{Array: make([]LValue, 10000)}
	for i := range tbl.Array {
		tbl.Array[i] = LNumber(float64(i))
	}

	if !arrType.Validate(L, tbl) {
		t.Error("10k element array should pass")
	}

	// Corrupt one element
	tbl.Array[9999] = LString("bad")
	if arrType.Validate(L, tbl) {
		t.Error("array with bad last element should fail")
	}
}

func TestAdversarial_HugeRecord(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Record with 100 fields
	rb := typ.NewRecord()
	for i := 0; i < 100; i++ {
		rb.Field(fmt.Sprintf("f%d", i), typ.Number)
	}
	rec := &LType{inner: rb.Build()}

	tbl := &LTable{Strdict: make(map[string]LValue, 100)}
	for i := 0; i < 100; i++ {
		tbl.Strdict[fmt.Sprintf("f%d", i)] = LNumber(float64(i))
	}

	if !rec.Validate(L, tbl) {
		t.Error("100-field record should pass")
	}

	// Miss one field
	delete(tbl.Strdict, "f99")
	if rec.Validate(L, tbl) {
		t.Error("record missing f99 should fail")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: deeply nested types
// ---------------------------------------------------------------------------

func TestAdversarial_DeeplyNestedOptional(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Optional(Optional(Optional(... number ...)))  50 levels deep
	// NewOptional normalizes Optional(Optional(X)) -> Optional(X)
	// So this should all collapse to just Optional(Number)
	inner := typ.Type(typ.Number)
	for i := 0; i < 50; i++ {
		inner = typ.NewOptional(inner)
	}

	deepOpt := &LType{inner: inner}
	if !deepOpt.Validate(L, LNumber(42)) {
		t.Error("deeply nested optional should pass for number")
	}
	if !deepOpt.Validate(L, LNil) {
		t.Error("deeply nested optional should pass for nil")
	}
}

func TestAdversarial_DeeplyNestedRecord(t *testing.T) {
	L := NewState()
	defer L.Close()

	// type A = {inner: {inner: {inner: ... {value: number} ...}}}  20 levels
	var inner typ.Type = typ.NewRecord().Field("value", typ.Number).Build()
	for i := 0; i < 20; i++ {
		inner = typ.NewRecord().Field("inner", inner).Build()
	}

	deepRec := &LType{inner: inner}

	// Build matching table 20 levels deep
	var tbl LValue = &LTable{Strdict: map[string]LValue{"value": LNumber(42)}}
	for i := 0; i < 20; i++ {
		tbl = &LTable{Strdict: map[string]LValue{"inner": tbl}}
	}

	if !deepRec.Validate(L, tbl) {
		t.Error("20-level nested record should pass")
	}
}

func TestAdversarial_DeeplyNestedUnion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Union(Union(Union(... number | string ...)))
	// NewUnion flattens, so this collapses to Union(number, string)
	inner := typ.Type(typ.NewUnion(typ.Number, typ.String))
	for i := 0; i < 30; i++ {
		inner = typ.NewUnion(inner, typ.Boolean)
	}

	deepUnion := &LType{inner: inner}
	if !deepUnion.Validate(L, LNumber(42)) {
		t.Error("deep union should accept number")
	}
	if !deepUnion.Validate(L, LString("hi")) {
		t.Error("deep union should accept string")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: circular / self-referential types
// ---------------------------------------------------------------------------

func TestAdversarial_MutualRecursion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// type A = { b: B? }
	// type B = { a: A? }
	a := typ.NewRecursivePlaceholder("A")
	b := typ.NewRecursivePlaceholder("B")
	a.SetBody(typ.NewRecord().OptField("b", b).Build())
	b.SetBody(typ.NewRecord().OptField("a", a).Build())

	aType := &LType{inner: a}

	// Simple value — no cycle in data
	tbl := L.NewTable()
	if !aType.Validate(L, tbl) {
		t.Error("empty table should pass A (all fields optional)")
	}

	// One level of nesting
	inner := L.NewTable()
	tbl2 := L.NewTable()
	tbl2.RawSetString("b", inner)
	if !aType.Validate(L, tbl2) {
		t.Error("A{b: {}} should pass")
	}

	// Two levels: A -> B -> A
	innerA := L.NewTable()
	innerB := L.NewTable()
	innerB.RawSetString("a", innerA)
	tbl3 := L.NewTable()
	tbl3.RawSetString("b", innerB)
	if !aType.Validate(L, tbl3) {
		t.Error("A{b: B{a: A{}}} should pass")
	}
}

func TestAdversarial_DirectSelfReference(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Malformed: Body = itself (no Optional to break recursion)
	rec := typ.NewRecursivePlaceholder("Loop")
	rec.SetBody(rec)

	loopType := &LType{inner: rec}

	// Must not hang — depth limit kicks in
	loopType.Validate(L, L.NewTable())
}

func TestAdversarial_SelfReferenceIs(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	rec := typ.NewRecursivePlaceholder("Loop")
	rec.SetBody(rec)

	loopType := &LType{inner: rec}
	isMethod := L.typeGetField(loopType, "is")
	L.Push(isMethod)
	L.Push(L.NewTable())
	L.Call(1, 2)
	// Just verify it doesn't hang — result doesn't matter
	L.Pop(2)
}

// ---------------------------------------------------------------------------
// Adversarial: nil / empty in every position
// ---------------------------------------------------------------------------

func TestAdversarial_NilInEveryPosition(t *testing.T) {
	L := NewState()
	defer L.Close()

	types := []*LType{
		LTypeNumber,
		LTypeString,
		LTypeBoolean,
		LTypeInteger,
		LTypeNil,
		LTypeAny,
		LTypeNever,
		{inner: typ.NewOptional(typ.Number)},
		{inner: typ.NewArray(typ.Number)},
		{inner: typ.NewMap(typ.String, typ.Number)},
		{inner: typ.NewRecord().Field("x", typ.Number).Build()},
		{inner: typ.NewInterface("table", nil)},
		{inner: typ.NewUnion(typ.Number, typ.String)},
		{inner: typ.LiteralString("x")},
		{inner: typ.NewTuple(typ.Number)},
	}

	for _, lt := range types {
		name := lt.String()
		// Should not panic on nil value
		_ = lt.Validate(L, LNil)

		// Should not panic via :is()
		isMethod := L.typeGetField(lt, "is")
		L.Push(isMethod)
		L.Push(LNil)
		L.Call(1, 2)
		L.Pop(2)

		_ = name // used for debugging if needed
	}
}

func TestAdversarial_EmptyTableAgainstAll(t *testing.T) {
	L := NewState()
	defer L.Close()

	empty := L.NewTable()

	tests := []struct {
		name string
		typ  *LType
		ok   bool
	}{
		{"number", LTypeNumber, false},
		{"string", LTypeString, false},
		{"boolean", LTypeBoolean, false},
		{"nil", LTypeNil, false},
		{"any", LTypeAny, true},
		{"table", &LType{inner: typ.NewInterface("table", nil)}, true},
		{"empty record", &LType{inner: typ.NewRecord().Build()}, true},
		{"record with required field", &LType{inner: typ.NewRecord().Field("x", typ.Number).Build()}, false},
		{"record all optional", &LType{inner: typ.NewRecord().OptField("x", typ.Number).Build()}, true},
		{"empty array", &LType{inner: typ.NewArray(typ.Number)}, true},
		{"empty map", &LType{inner: typ.NewMap(typ.String, typ.Number)}, true},
		{"function", &LType{inner: typ.Func().Returns(typ.Number).Build()}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.Validate(L, empty); got != tt.ok {
				t.Errorf("Validate(empty table) = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Adversarial: type confusion / mixed table shapes
// ---------------------------------------------------------------------------

func TestAdversarial_TableWithBothArrayAndDict(t *testing.T) {
	L := NewState()
	defer L.Close()

	// A table that has BOTH array entries and string keys
	// This is valid Lua — tables can be mixed
	mixed := L.NewTable()
	mixed.Append(LNumber(1))                    // Array[0] = 1
	mixed.Append(LNumber(2))                    // Array[1] = 2
	mixed.RawSetString("name", LString("test")) // Strdict["name"] = "test"

	// Record should see "name" field
	rec := &LType{inner: typ.NewRecord().Field("name", typ.String).Build()}
	if !rec.Validate(L, mixed) {
		t.Error("record should find 'name' in mixed table")
	}

	// Array should see array part
	arr := &LType{inner: typ.NewArray(typ.Number)}
	if !arr.Validate(L, mixed) {
		t.Error("array should validate array part of mixed table")
	}

	// Map {[string]: string} only checks Strdict — Strdict has name="test" (valid)
	// Array part is not checked for string-keyed maps (correct behavior)
	strMap := &LType{inner: typ.NewMap(typ.String, typ.String)}
	if !strMap.Validate(L, mixed) {
		t.Error("string map should pass — only Strdict is checked, and name=test is valid")
	}

	// Map {[string]: number} should reject because Strdict has name="test" (string, not number)
	strNumMap := &LType{inner: typ.NewMap(typ.String, typ.Number)}
	if strNumMap.Validate(L, mixed) {
		t.Error("string->number map should reject string value in Strdict")
	}
}

func TestAdversarial_TableWithNonStringKeys(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Table keyed by booleans — goes into Dict
	tbl := L.NewTable()
	tbl.RawSet(LTrue, LString("yes"))
	tbl.RawSet(LFalse, LString("no"))

	// Map {[boolean]: string} — check Dict
	boolMap := &LType{inner: typ.NewMap(typ.Boolean, typ.String)}
	if !boolMap.Validate(L, tbl) {
		t.Error("boolean-keyed map should pass")
	}

	// Map {[string]: string} — boolean keys should fail
	strMap := &LType{inner: typ.NewMap(typ.String, typ.String)}
	if strMap.Validate(L, tbl) {
		t.Error("string-keyed map should reject boolean keys")
	}
}

func TestAdversarial_TableWithTableKeys(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Table keyed by other tables — goes into Dict
	key1 := L.NewTable()
	key2 := L.NewTable()
	tbl := L.NewTable()
	tbl.RawSet(key1, LNumber(1))
	tbl.RawSet(key2, LNumber(2))

	// Map {[table]: number}
	tableMap := &LType{inner: typ.NewMap(typ.NewInterface("table", nil), typ.Number)}
	if !tableMap.Validate(L, tbl) {
		t.Error("table-keyed map should pass")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: very long strings and patterns
// ---------------------------------------------------------------------------

func TestAdversarial_LongString(t *testing.T) {
	L := NewState()
	defer L.Close()

	longStr := LString(strings.Repeat("a", 100000))

	if !LTypeString.Validate(L, longStr) {
		t.Error("100k char string should pass string validation")
	}

	// @max_len(100) should reject
	maxLen := &LType{inner: typ.NewAnnotated(typ.String, []typ.Annotation{
		{Name: "max_len", Arg: float64(100)},
	})}
	if maxLen.Validate(L, longStr) {
		t.Error("100k string should fail @max_len(100)")
	}

	// @pattern should work on long strings
	pattern := &LType{inner: typ.NewAnnotated(typ.String, []typ.Annotation{
		{Name: "pattern", Arg: "^a+$"},
	})}
	if !pattern.Validate(L, longStr) {
		t.Error("100k 'a' string should match ^a+$")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: union with overlapping types
// ---------------------------------------------------------------------------

func TestAdversarial_UnionOverlap(t *testing.T) {
	L := NewState()
	defer L.Close()

	// number | integer — integer is a subtype of number, both should match
	u := &LType{inner: typ.NewUnion(typ.Number, typ.Integer)}

	if !u.Validate(L, LNumber(42.5)) {
		t.Error("float should pass number|integer")
	}
	if !u.Validate(L, LInteger(42)) {
		t.Error("integer should pass number|integer")
	}
}

func TestAdversarial_UnionWithNever(t *testing.T) {
	L := NewState()
	defer L.Close()

	// number | never — never contributes nothing
	u := &LType{inner: typ.NewUnion(typ.Number, typ.Never)}
	if !u.Validate(L, LNumber(42)) {
		t.Error("number should pass number|never")
	}
	if u.Validate(L, LString("x")) {
		t.Error("string should fail number|never")
	}
}

func TestAdversarial_UnionWithAny(t *testing.T) {
	L := NewState()
	defer L.Close()

	// string | any — any swallows everything
	u := &LType{inner: typ.NewUnion(typ.String, typ.Any)}
	if !u.Validate(L, LNumber(42)) {
		t.Error("number should pass string|any")
	}
	if !u.Validate(L, LNil) {
		t.Error("nil should pass string|any")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: resolver attacks
// ---------------------------------------------------------------------------

func TestAdversarial_ResolverCircularRef(t *testing.T) {
	L := NewState()
	defer L.Close()

	// A resolves to Ref(B), B resolves to Ref(A) — circular
	resolver := &typeResolver{
		types: map[string]typ.Type{
			"A": typ.NewRef("", "B"),
			"B": typ.NewRef("", "A"),
		},
	}

	refType := &LType{inner: typ.NewRef("", "A"), resolver: resolver}

	// Should not hang — resolveRuntimeType has depth limit of 32
	result := refType.Validate(L, LNumber(42))
	_ = result // just verify termination
}

func TestAdversarial_ResolverDeepChain(t *testing.T) {
	L := NewState()
	defer L.Close()

	// A -> B -> C -> ... -> Z -> number (26-level alias chain)
	types := make(map[string]typ.Type)
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("T%d", i)
		next := fmt.Sprintf("T%d", i+1)
		types[name] = typ.NewAlias(name, typ.NewRef("", next))
	}
	types["T25"] = typ.Number

	resolver := &typeResolver{types: types}
	refType := &LType{inner: typ.NewRef("", "T0"), resolver: resolver}

	if !refType.Validate(L, LNumber(42)) {
		t.Error("26-level alias chain should resolve to number")
	}
}

func TestAdversarial_ResolverChainExceedsDepth(t *testing.T) {
	L := NewState()
	defer L.Close()

	// 40-level chain — exceeds the 32-depth limit
	types := make(map[string]typ.Type)
	for i := 0; i < 39; i++ {
		name := fmt.Sprintf("T%d", i)
		next := fmt.Sprintf("T%d", i+1)
		types[name] = typ.NewAlias(name, typ.NewRef("", next))
	}
	types["T39"] = typ.Number

	resolver := &typeResolver{types: types}
	refType := &LType{inner: typ.NewRef("", "T0"), resolver: resolver}

	// Should not hang, but may not resolve fully
	_ = refType.Validate(L, LNumber(42))
}

// ---------------------------------------------------------------------------
// Adversarial: record field name edge cases
// ---------------------------------------------------------------------------

func TestAdversarial_RecordEmptyFieldName(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Field with empty string name
	rec := &LType{inner: typ.NewRecord().Field("", typ.Number).Build()}

	tbl := L.NewTable()
	tbl.RawSetString("", LNumber(42))

	if !rec.Validate(L, tbl) {
		t.Error("record with empty-string field should work")
	}
}

func TestAdversarial_RecordSpecialFieldNames(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{inner: typ.NewRecord().
		Field("__index", typ.String).
		Field("__tostring", typ.String).
		Field("is", typ.Number). // shadows the method name
		Build(),
	}

	tbl := L.NewTable()
	tbl.RawSetString("__index", LString("test"))
	tbl.RawSetString("__tostring", LString("test"))
	tbl.RawSetString("is", LNumber(42))

	if !rec.Validate(L, tbl) {
		t.Error("record with metamethod-like field names should work")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: annotation with extreme values
// ---------------------------------------------------------------------------

func TestAdversarial_AnnotationExtremeValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		name       string
		annotation typ.Annotation
		value      LValue
		ok         bool
	}{
		{"min MaxFloat64", typ.Annotation{Name: "min", Arg: math.MaxFloat64}, LNumber(math.MaxFloat64), true},
		{"min MaxFloat64 rejects less", typ.Annotation{Name: "min", Arg: math.MaxFloat64}, LNumber(0), false},
		{"max -MaxFloat64", typ.Annotation{Name: "max", Arg: -math.MaxFloat64}, LNumber(-math.MaxFloat64), true},
		{"max -MaxFloat64 rejects more", typ.Annotation{Name: "max", Arg: -math.MaxFloat64}, LNumber(0), false},
		{"min_len very large", typ.Annotation{Name: "min_len", Arg: float64(999999)}, LString("short"), false},
		{"max_len negative", typ.Annotation{Name: "max_len", Arg: float64(-1)}, LString("anything"), true}, // negative max_len returns nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ann := &LType{inner: typ.NewAnnotated(typ.Number, []typ.Annotation{tt.annotation})}
			// For string annotations, use string type
			if _, ok := tt.value.(LString); ok {
				ann = &LType{inner: typ.NewAnnotated(typ.String, []typ.Annotation{tt.annotation})}
			}
			if got := ann.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Adversarial: :is() with wrong number of arguments
// ---------------------------------------------------------------------------

func TestAdversarial_IsNoArgs(t *testing.T) {
	L := NewState()
	defer L.Close()

	isMethod := L.typeGetField(LTypeNumber, "is")
	L.Push(isMethod)
	L.Call(0, 2)
	val := L.Get(-2)
	L.Pop(2)

	// With 0 args, idx=1, L.Get(1) returns LNil
	// Should validate LNil against Number type — fails
	if val != LNil {
		t.Error("is() with no args should return nil (number doesn't accept nil)")
	}
}

func TestAdversarial_IsManyArgs(t *testing.T) {
	L := NewState()
	defer L.Close()

	// :is() with 5 args — should only look at arg 2 (colon syntax)
	isMethod := L.typeGetField(LTypeNumber, "is")
	L.Push(isMethod)
	L.Push(LString("self"))  // arg 1 (self in colon syntax)
	L.Push(LNumber(42))      // arg 2 (the value)
	L.Push(LString("extra")) // arg 3 (ignored)
	L.Push(LTrue)            // arg 4 (ignored)
	L.Push(LNil)             // arg 5 (ignored)
	L.Call(5, 2)
	val := L.Get(-2)
	L.Pop(2)

	if val == LNil {
		t.Error("is() should validate arg 2 (the number)")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: record with many optional fields, one required at the end
// ---------------------------------------------------------------------------

func TestAdversarial_RecordLastFieldRequired(t *testing.T) {
	L := NewState()
	defer L.Close()

	// 20 optional fields, then one required at the end
	rb := typ.NewRecord()
	for i := 0; i < 20; i++ {
		rb.OptField(fmt.Sprintf("opt%d", i), typ.String)
	}
	rb.Field("required", typ.Number)
	rec := &LType{inner: rb.Build()}

	// Empty table — missing required field
	if rec.Validate(L, L.NewTable()) {
		t.Error("should fail — required field missing")
	}

	// Table with only the required field
	tbl := L.NewTable()
	tbl.RawSetString("required", LNumber(42))
	if !rec.Validate(L, tbl) {
		t.Error("should pass — only required field present")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: Validate vs :is() consistency under adversarial input
// ---------------------------------------------------------------------------

func TestAdversarial_ValidateIsConsistency(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("tags", typ.NewArray(typ.String)).
			OptField("meta", typ.NewInterface("table", nil)).
			Build(),
	}

	adversarialValues := []LValue{
		LNil,
		LTrue,
		LFalse,
		LNumber(0),
		LNumber(math.NaN()),
		LNumber(math.Inf(1)),
		LString(""),
		LString("not a table"),
		L.NewTable(),               // empty table
		&LUserData{Value: "x"},     // userdata
		&LTable{Array: []LValue{}}, // empty array table
		func() LValue { // table with wrong types
			t := L.NewTable()
			t.RawSetString("id", LNumber(123))
			return t
		}(),
		func() LValue { // valid table
			t := L.NewTable()
			t.RawSetString("id", LString("abc"))
			return t
		}(),
		func() LValue { // valid with optionals
			t := L.NewTable()
			t.RawSetString("id", LString("abc"))
			tags := L.NewTable()
			tags.Append(LString("x"))
			t.RawSetString("tags", tags)
			t.RawSetString("meta", L.NewTable())
			return t
		}(),
	}

	isMethod := L.typeGetField(rec, "is")
	for i, v := range adversarialValues {
		validateResult := rec.Validate(L, v)

		L.Push(isMethod)
		L.Push(v)
		L.Call(1, 2)
		isVal := L.Get(-2)
		isErr := L.Get(-1)
		L.Pop(2)

		// Determine :is() result
		isSuccess := isVal != LNil || isErr == LNil

		if validateResult != isSuccess {
			t.Errorf("case %d (%T): Validate()=%v but is() val=%v err=%v",
				i, v, validateResult, isVal, errMessage(isErr))
		}
	}
}

// ---------------------------------------------------------------------------
// Adversarial: empty/nil type internals
// ---------------------------------------------------------------------------

func TestAdversarial_NilTypeInner(t *testing.T) {
	L := NewState()
	defer L.Close()

	// LType with nil inner — should not panic
	lt := &LType{inner: nil}
	if lt.Validate(L, LNumber(42)) {
		t.Error("nil inner should fail")
	}
	if lt.Validate(L, LNil) {
		t.Error("nil inner should fail for nil too")
	}
}

func TestAdversarial_NilTypeInnerIs(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	lt := &LType{inner: nil}
	isMethod := L.typeGetField(lt, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	val := L.Get(-2)
	L.Pop(2)

	if val != LNil {
		t.Error("nil inner type should reject everything")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: Annotated wrapping Annotated
// ---------------------------------------------------------------------------

func TestAdversarial_DoubleAnnotated(t *testing.T) {
	L := NewState()
	defer L.Close()

	// number @min(0) @max(100) — then wrap that in another @pattern (nonsensical but shouldn't crash)
	inner := typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
		{Name: "max", Arg: float64(100)},
	})
	outer := &LType{inner: typ.NewAnnotated(inner, []typ.Annotation{
		{Name: "min", Arg: float64(10)}, // stricter min on top
	})}

	if !outer.Validate(L, LNumber(50)) {
		t.Error("50 should pass both annotation layers")
	}
	if outer.Validate(L, LNumber(5)) {
		t.Error("5 should fail outer @min(10)")
	}
	if outer.Validate(L, LNumber(105)) {
		t.Error("105 should fail inner @max(100)")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: LType as a value being validated
// ---------------------------------------------------------------------------

// ===========================================================================
// EXPLOIT: nil pointer dereference attacks
// These would cause segfaults in JIT if not handled
// ===========================================================================

func TestExploit_NilFieldType_MissingField(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	// Record with nil field type — corrupted manifest
	rec := &LType{inner: &typ.Record{Fields: []typ.Field{
		{Name: "x", Type: nil, Optional: false},
	}}}

	// Validate path — fieldVal is LNil, field not optional, field.Type is nil
	// validateValueDepth: nil type → returns false. No crash.
	if rec.Validate(L, L.NewTable()) {
		t.Error("should fail")
	}

	// :is() path — the danger zone. validateValue returns false,
	// then validateWithError calls field.Type.String() → nil deref → PANIC
	// This must not crash.
	isMethod := L.typeGetField(rec, "is")
	L.Push(isMethod)
	L.Push(L.NewTable())
	L.Call(1, 2)
	val := L.Get(-2)
	L.Pop(2)
	if val != LNil {
		t.Error("should fail")
	}
}

func TestExploit_NilMapKey(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	// Map with nil Key — corrupted manifest
	m := &LType{inner: &typ.Map{Key: nil, Value: typ.Number}}

	tbl := L.NewTable()
	tbl.RawSetString("a", LNumber(1))

	// Validate path — nil key type → returns false. Safe.
	if m.Validate(L, tbl) {
		t.Error("should fail")
	}

	// :is() path — t.String() calls m.Key.String() → nil deref
	isMethod := L.typeGetField(m, "is")
	L.Push(isMethod)
	L.Push(tbl)
	L.Call(1, 2)
	val := L.Get(-2)
	L.Pop(2)
	if val != LNil {
		t.Error("should fail")
	}
}

func TestExploit_NilMapValue(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	m := &LType{inner: &typ.Map{Key: typ.String, Value: nil}}

	tbl := L.NewTable()
	tbl.RawSetString("a", LNumber(1))

	isMethod := L.typeGetField(m, "is")
	L.Push(isMethod)
	L.Push(tbl)
	L.Call(1, 2)
	L.Pop(2)
	// Must not crash
}

func TestExploit_NilArrayElement(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	arr := &LType{inner: &typ.Array{Element: nil}}

	tbl := L.NewTable()
	tbl.Append(LNumber(1))

	isMethod := L.typeGetField(arr, "is")
	L.Push(isMethod)
	L.Push(tbl)
	L.Call(1, 2)
	L.Pop(2)
}

func TestExploit_NilOptionalInner(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	opt := &LType{inner: &typ.Optional{Inner: nil}}

	isMethod := L.typeGetField(opt, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	L.Pop(2)
}

func TestExploit_NilAnnotatedInner(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	ann := &LType{inner: &typ.Annotated{Inner: nil, Annotations: []typ.Annotation{
		{Name: "min", Arg: float64(0)},
	}}}

	isMethod := L.typeGetField(ann, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	L.Pop(2)
}

func TestExploit_NilUnionMember(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	u := &LType{inner: &typ.Union{Members: []typ.Type{nil, typ.Number}}}

	isMethod := L.typeGetField(u, "is")
	L.Push(isMethod)
	L.Push(LString("bad"))
	L.Call(1, 2)
	L.Pop(2)
}

func TestExploit_NilIntersectionMember(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	inter := &LType{inner: &typ.Intersection{Members: []typ.Type{nil, typ.Number}}}

	isMethod := L.typeGetField(inter, "is")
	L.Push(isMethod)
	L.Push(LNumber(42))
	L.Call(1, 2)
	L.Pop(2)
}

func TestExploit_NilTupleElement(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	tuple := &LType{inner: &typ.Tuple{Elements: []typ.Type{typ.Number, nil}}}

	tbl := L.NewTable()
	tbl.Append(LNumber(1))
	tbl.Append(LString("x"))

	isMethod := L.typeGetField(tuple, "is")
	L.Push(isMethod)
	L.Push(tbl)
	L.Call(1, 2)
	L.Pop(2)
}

func TestExploit_ValidateIsDisagreement(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	// If validateValue returns false but validateWithError returns true,
	// verr would be nil → toValidationLuaError panics.
	// Verify these never disagree.
	types := []typ.Type{
		typ.Number, typ.String, typ.Boolean, typ.Integer,
		typ.NewOptional(typ.Number),
		typ.NewArray(typ.Number),
		typ.NewMap(typ.String, typ.Number),
		typ.NewRecord().Field("x", typ.Number).Build(),
		typ.NewUnion(typ.Number, typ.String),
		typ.NewInterface("table", nil),
		typ.LiteralString("hello"),
		typ.LiteralInt(42),
		typ.NewIntersection(
			typ.NewRecord().Field("x", typ.Number).Build(),
			typ.NewRecord().Field("y", typ.String).Build(),
		),
	}

	values := []LValue{
		LNil, LTrue, LFalse, LNumber(42), LNumber(42.5), LInteger(42),
		LString("hello"), LString(""), &LTable{}, &LUserData{},
	}

	for ti, tp := range types {
		_ = &LType{inner: tp}
		for vi, v := range values {
			boolResult := validateValue(v, tp, nil)
			_, verr := validateWithError(v, tp, nil, "")
			errResult := verr == nil

			if boolResult != errResult {
				t.Errorf("DISAGREEMENT type[%d] val[%d]: validateValue=%v validateWithError=%v (verr=%v)",
					ti, vi, boolResult, errResult, verr)
			}
		}
	}
}

func TestAdversarial_TypeAsValue(t *testing.T) {
	L := NewState()
	defer L.Close()

	typeVal := LTypeNumber

	if LTypeNumber.Validate(L, typeVal) {
		t.Error("LType should not pass as number")
	}
	if LTypeString.Validate(L, typeVal) {
		t.Error("LType should not pass as string")
	}
	if LTypeAny.Validate(L, typeVal) != true {
		t.Error("LType should pass as any")
	}

	tblType := &LType{inner: typ.NewInterface("table", nil)}
	if tblType.Validate(L, typeVal) {
		t.Error("LType should not pass as table")
	}
}

// ===========================================================================
// LITERAL ADVERSARIAL
// ===========================================================================

func TestAdversarial_LiteralNumberVsInteger(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Literal int64(42) should match both LInteger(42) and LNumber(42.0)
	intLit := &LType{inner: typ.LiteralInt(42)}
	if !intLit.Validate(L, LInteger(42)) {
		t.Error("int64 literal should match LInteger")
	}
	if !intLit.Validate(L, LNumber(42.0)) {
		t.Error("int64 literal should match whole LNumber")
	}
	if intLit.Validate(L, LNumber(42.5)) {
		t.Error("int64 literal should NOT match fractional LNumber")
	}
	if intLit.Validate(L, LNumber(42.0000001)) {
		t.Error("int64 literal should NOT match near-miss LNumber")
	}

	// Literal float64(42.0) should match LNumber(42.0) but NOT LInteger(42)
	floatLit := &LType{inner: typ.LiteralNumber(42.0)}
	if !floatLit.Validate(L, LNumber(42.0)) {
		t.Error("float64 literal should match LNumber")
	}
	if floatLit.Validate(L, LInteger(42)) {
		t.Error("float64 literal should NOT match LInteger (different Go type)")
	}

	// Literal float64(42.1) should not match LNumber(42.2) — different values
	diffLit := &LType{inner: typ.LiteralNumber(42.1)}
	if diffLit.Validate(L, LNumber(42.2)) {
		t.Error("42.1 literal should NOT match 42.2")
	}
}

func TestAdversarial_LiteralBoolInUnion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// true | "yes" | 1 — mixed literal union
	u := &LType{inner: typ.NewUnion(
		typ.LiteralBool(true),
		typ.LiteralString("yes"),
		typ.LiteralInt(1),
	)}

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"true matches", LTrue, true},
		{"false fails", LFalse, false},
		{"yes matches", LString("yes"), true},
		{"no fails", LString("no"), false},
		{"1 as integer matches", LInteger(1), true},
		{"1 as number matches", LNumber(1.0), true},
		{"2 fails", LInteger(2), false},
		{"nil fails", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := u.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate(%v) = %v, want %v", tt.value, got, tt.ok)
			}
		})
	}
}

func TestAdversarial_LiteralEmptyString(t *testing.T) {
	L := NewState()
	defer L.Close()

	// "" literal — must match empty string, reject everything else
	emptyLit := &LType{inner: typ.LiteralString("")}

	if !emptyLit.Validate(L, LString("")) {
		t.Error("empty string literal should match empty string")
	}
	if emptyLit.Validate(L, LString(" ")) {
		t.Error("empty string literal should reject space")
	}
	if emptyLit.Validate(L, LNil) {
		t.Error("empty string literal should reject nil")
	}
}

// ===========================================================================
// UNION ADVERSARIAL
// ===========================================================================

func TestAdversarial_UnionOfRecordsDiscriminant(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)

	// Discriminated union pattern:
	// {type: "circle", radius: number} | {type: "rect", width: number, height: number}
	circle := typ.NewRecord().
		Field("type", typ.LiteralString("circle")).
		Field("radius", typ.Number).
		Build()
	rect := typ.NewRecord().
		Field("type", typ.LiteralString("rect")).
		Field("width", typ.Number).
		Field("height", typ.Number).
		Build()
	shape := &LType{inner: typ.NewUnion(circle, rect), name: "Shape"}
	L.SetGlobal("Shape", shape)

	err := L.DoString(`
		local c, e = Shape:is({type = "circle", radius = 5})
		assert(c ~= nil, "circle should pass: " .. tostring(e))

		local r, e = Shape:is({type = "rect", width = 10, height = 20})
		assert(r ~= nil, "rect should pass: " .. tostring(e))

		-- Wrong discriminant
		local x, e = Shape:is({type = "triangle", sides = 3})
		assert(x == nil, "triangle should fail")

		-- Missing discriminant
		local y, e = Shape:is({radius = 5})
		assert(y == nil, "missing type field should fail")

		-- Right discriminant, wrong payload
		local z, e = Shape:is({type = "circle", radius = "big"})
		assert(z == nil, "circle with string radius should fail")
	`)
	if err != nil {
		t.Fatalf("discriminated union test failed: %v", err)
	}
}

func TestAdversarial_UnionRecordVsPrimitive(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {x: number} | string — table or string
	u := &LType{inner: typ.NewUnion(
		typ.NewRecord().Field("x", typ.Number).Build(),
		typ.String,
	)}

	tbl := L.NewTable()
	tbl.RawSetString("x", LNumber(42))

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"record passes", tbl, true},
		{"string passes", LString("hello"), true},
		{"number fails", LNumber(42), false},
		{"empty table fails (missing x)", L.NewTable(), false},
		{"nil fails", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := u.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAdversarial_UnionSingleMember(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Single-member union normalizes to the member itself
	u := typ.NewUnion(typ.Number)
	lt := &LType{inner: u}

	if !lt.Validate(L, LNumber(42)) {
		t.Error("single-member union should pass for member type")
	}
	if lt.Validate(L, LString("x")) {
		t.Error("single-member union should fail for non-member")
	}
}

func TestAdversarial_UnionEmpty(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Empty union normalizes to Never
	u := typ.NewUnion()
	lt := &LType{inner: u}

	// Never rejects everything
	if lt.Validate(L, LNumber(42)) {
		t.Error("empty union (Never) should reject number")
	}
	if lt.Validate(L, LNil) {
		t.Error("empty union (Never) should reject nil")
	}
}

// ===========================================================================
// REFLECTION METHOD TESTS
// ===========================================================================

func TestReflection_KindMethod(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	types := map[string]*LType{
		"number":   LTypeNumber,
		"string":   LTypeString,
		"boolean":  LTypeBoolean,
		"integer":  LTypeInteger,
		"any":      LTypeAny,
		"never":    LTypeNever,
		"record":   {inner: typ.NewRecord().Field("x", typ.Number).Build()},
		"array":    {inner: typ.NewArray(typ.Number)},
		"map":      {inner: typ.NewMap(typ.String, typ.Number)},
		"optional": {inner: typ.NewOptional(typ.Number)},
		"union":    {inner: typ.NewUnion(typ.Number, typ.String)},
		"function": {inner: typ.Func().Returns(typ.Number).Build()},
	}

	for expected, lt := range types {
		k := lt.KindString()
		if k != expected {
			t.Errorf("KindString() for %s = %q, want %q", lt.String(), k, expected)
		}
	}
}

func TestReflection_FieldsIterator(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	rec := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			OptField("age", typ.Number).
			Field("active", typ.Boolean).
			Build(),
		name: "User",
	}
	L.SetGlobal("User", rec)

	err := L.DoString(`
		local names = {}
		local types = {}
		for name, fieldType in User:fields() do
			names[#names+1] = name
			types[name] = fieldType:kind()
		end
		-- Fields should be present (order may vary due to sorting)
		assert(types["name"] == "string", "name should be string")
		assert(types["age"] == "number", "age should be number")
		assert(types["active"] == "boolean", "active should be boolean")
		assert(#names == 3, "should have 3 fields, got " .. #names)
	`)
	if err != nil {
		t.Fatalf("fields iterator test failed: %v", err)
	}
}

func TestReflection_VariantsIterator(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	u := &LType{inner: typ.NewUnion(typ.Number, typ.String, typ.Boolean)}
	L.SetGlobal("MyUnion", u)

	err := L.DoString(`
		local count = 0
		for variant in MyUnion:variants() do
			count = count + 1
			assert(variant:kind() ~= nil, "variant should have a kind")
		end
		assert(count == 3, "should have 3 variants, got " .. count)
	`)
	if err != nil {
		t.Fatalf("variants iterator test failed: %v", err)
	}
}

func TestReflection_InnerMethod(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	opt := &LType{inner: typ.NewOptional(typ.Number)}
	L.SetGlobal("OptNum", opt)

	err := L.DoString(`
		local inner = OptNum:inner()
		assert(inner ~= nil, "inner should not be nil")
		assert(inner:kind() == "number", "inner kind should be 'number', got: " .. inner:kind())
	`)
	if err != nil {
		t.Fatalf("inner method test failed: %v", err)
	}
}

func TestReflection_ElemKeyVal(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	arr := &LType{inner: typ.NewArray(typ.String)}
	m := &LType{inner: typ.NewMap(typ.String, typ.Number)}
	L.SetGlobal("StringArray", arr)
	L.SetGlobal("StringNumMap", m)

	err := L.DoString(`
		local elem = StringArray:elem()
		assert(elem:kind() == "string", "array elem should be string")

		local key = StringNumMap:key()
		assert(key:kind() == "string", "map key should be string")

		local val = StringNumMap:val()
		assert(val:kind() == "number", "map val should be number")
	`)
	if err != nil {
		t.Fatalf("elem/key/val test failed: %v", err)
	}
}

func TestReflection_ParamsAndRet(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	fn := &LType{inner: typ.Func().
		Param("x", typ.Number).
		Param("y", typ.String).
		Returns(typ.Boolean).
		Build()}
	L.SetGlobal("MyFunc", fn)

	err := L.DoString(`
		local count = 0
		for param in MyFunc:params() do
			count = count + 1
		end
		assert(count == 2, "should have 2 params, got " .. count)

		local ret = MyFunc:ret()
		assert(ret:kind() == "boolean", "return type should be boolean")
	`)
	if err != nil {
		t.Fatalf("params/ret test failed: %v", err)
	}
}

// ===========================================================================
// JIT-CRITICAL: concurrent validation (data races)
// ===========================================================================

func TestAdversarial_ConcurrentValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("name", typ.String).
			OptField("count", typ.Number).
			Build(),
	}

	// Validate from multiple goroutines against the same type
	// Types are immutable so this should be safe
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			ls := NewState()
			defer ls.Close()

			tbl := ls.NewTable()
			tbl.RawSetString("id", LString(fmt.Sprintf("id-%d", n)))
			if n%2 == 0 {
				tbl.RawSetString("name", LString("test"))
			}
			if n%3 == 0 {
				tbl.RawSetString("count", LNumber(float64(n)))
			}

			result := rec.Validate(ls, tbl)
			if !result {
				panic(fmt.Sprintf("goroutine %d: valid table rejected", n))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// ===========================================================================
// Record field types that are themselves complex
// ===========================================================================

func TestAdversarial_RecordFieldIsUnionOfRecords(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {payload: ({kind: "text", body: string} | {kind: "image", url: string})}
	textRec := typ.NewRecord().
		Field("kind", typ.LiteralString("text")).
		Field("body", typ.String).
		Build()
	imageRec := typ.NewRecord().
		Field("kind", typ.LiteralString("image")).
		Field("url", typ.String).
		Build()
	outer := &LType{inner: typ.NewRecord().
		Field("payload", typ.NewUnion(textRec, imageRec)).
		Build(),
	}

	validText := L.NewTable()
	payload := L.NewTable()
	payload.RawSetString("kind", LString("text"))
	payload.RawSetString("body", LString("hello"))
	validText.RawSetString("payload", payload)

	validImage := L.NewTable()
	imgPayload := L.NewTable()
	imgPayload.RawSetString("kind", LString("image"))
	imgPayload.RawSetString("url", LString("https://example.com/img.png"))
	validImage.RawSetString("payload", imgPayload)

	invalidPayload := L.NewTable()
	badPayload := L.NewTable()
	badPayload.RawSetString("kind", LString("video"))
	invalidPayload.RawSetString("payload", badPayload)

	tests := []struct {
		name  string
		value LValue
		ok    bool
	}{
		{"text payload", validText, true},
		{"image payload", validImage, true},
		{"video payload (unknown kind)", invalidPayload, false},
		{"payload is string", func() LValue {
			t := L.NewTable()
			t.RawSetString("payload", LString("not a record"))
			return t
		}(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outer.Validate(L, tt.value); got != tt.ok {
				t.Errorf("Validate() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestAdversarial_RecordFieldIsArrayOfRecords(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {items: {name: string, price: number}[]}
	itemRec := typ.NewRecord().
		Field("name", typ.String).
		Field("price", typ.Number).
		Build()
	outer := &LType{inner: typ.NewRecord().
		Field("items", typ.NewArray(itemRec)).
		Build(),
	}

	items := L.NewTable()
	item1 := L.NewTable()
	item1.RawSetString("name", LString("Widget"))
	item1.RawSetString("price", LNumber(9.99))
	item2 := L.NewTable()
	item2.RawSetString("name", LString("Gadget"))
	item2.RawSetString("price", LNumber(19.99))
	items.Append(item1)
	items.Append(item2)

	valid := L.NewTable()
	valid.RawSetString("items", items)

	if !outer.Validate(L, valid) {
		t.Error("array of records should pass")
	}

	// Corrupt one item
	item2.RawSetString("price", LString("expensive"))
	if outer.Validate(L, valid) {
		t.Error("array with bad record should fail")
	}
}

func TestAdversarial_RecordFieldIsMapOfArrays(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {groups: {[string]: {number}[]}} — map of string to array of numbers
	outer := &LType{inner: typ.NewRecord().
		Field("groups", typ.NewMap(typ.String, typ.NewArray(typ.Number))).
		Build(),
	}

	groups := L.NewTable()
	g1 := L.NewTable()
	g1.Append(LNumber(1))
	g1.Append(LNumber(2))
	groups.RawSetString("alpha", g1)
	g2 := L.NewTable()
	g2.Append(LNumber(3))
	groups.RawSetString("beta", g2)

	valid := L.NewTable()
	valid.RawSetString("groups", groups)

	if !outer.Validate(L, valid) {
		t.Error("map of arrays should pass")
	}

	// Corrupt: put string in number array
	g2.Append(LString("bad"))
	if outer.Validate(L, valid) {
		t.Error("map of arrays with bad element should fail")
	}
}

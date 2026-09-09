package lua

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// Test toString behavior for different type kinds
func TestLTypeStringRepresentation(t *testing.T) {
	// Record type uses its name
	userType := NewLType(typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		Build())

	// Anonymous records don't have a name, so we expect the kind description
	if userType.KindString() != "record" {
		t.Errorf("expected 'record', got %q", userType.KindString())
	}

	// Named type uses name
	namedType := NewNamedLType(typ.Number, "Score")
	if namedType.String() != "Score" {
		t.Errorf("expected 'Score', got %q", namedType.String())
	}
}

// Test basic type validation without annotations
func TestBasicTypeValidation(t *testing.T) {
	ctx := NewValidationContext()

	userType := typ.NewRecord().
		Field("age", typ.Number).
		Field("name", typ.String).
		Build()

	lt := NewLType(userType)

	// Valid user
	validUser := &LTable{Strdict: map[string]LValue{"age": LNumber(25), "name": LString("John")}}
	errs := ctx.Validate(validUser, lt)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// Invalid user (wrong type for age)
	invalidUser := &LTable{Strdict: map[string]LValue{"age": LString("not a number"), "name": LString("John")}}
	errs = ctx.Validate(invalidUser, lt)
	if len(errs) == 0 {
		t.Error("expected error for wrong type")
	}
}

// Test nested record validation
func TestNestedRecordBasicValidation(t *testing.T) {
	ctx := NewValidationContext()

	addressType := typ.NewRecord().
		Field("zip", typ.String).
		Build()

	personType := typ.NewRecord().
		Field("age", typ.Number).
		Field("address", addressType).
		Build()

	lt := NewLType(personType)

	// Valid nested
	addr := &LTable{Strdict: map[string]LValue{"zip": LString("12345")}}
	person := &LTable{Strdict: map[string]LValue{
		"age":     LNumber(30),
		"address": addr,
	}}
	errs := ctx.Validate(person, lt)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// Invalid nested (wrong type for address)
	invalidPerson := &LTable{Strdict: map[string]LValue{
		"age":     LNumber(30),
		"address": LString("not a table"),
	}}
	errs = ctx.Validate(invalidPerson, lt)
	if len(errs) == 0 {
		t.Error("expected error for invalid address type")
	}
}

// Test array validation
func TestArrayBasicValidation(t *testing.T) {
	ctx := NewValidationContext()

	numArrayType := typ.NewArray(typ.Number)
	lt := NewLType(numArrayType)

	// Valid array
	validArr := &LTable{Array: []LValue{LNumber(1), LNumber(2), LNumber(3)}}
	errs := ctx.Validate(validArr, lt)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// Invalid array (contains string)
	invalidArr := &LTable{Array: []LValue{LNumber(1), LString("two"), LNumber(3)}}
	errs = ctx.Validate(invalidArr, lt)
	if len(errs) == 0 {
		t.Error("expected error for string element in number array")
	}
}

func TestValidationIntegration_PrimitiveTypes(t *testing.T) {
	ctx := DefaultValidationContext()

	tests := []struct {
		name    string
		val     LValue
		typ     typ.Type
		wantErr bool
	}{
		// Numbers
		{"number accepts LNumber", LNumber(42), typ.Number, false},
		{"number accepts LInteger", LInteger(42), typ.Number, false},
		{"number rejects string", LString("42"), typ.Number, true},
		{"number rejects nil", LNil, typ.Number, true},
		{"number rejects bool", LTrue, typ.Number, true},

		// Integers
		{"integer accepts LInteger", LInteger(42), typ.Integer, false},
		{"integer accepts whole LNumber", LNumber(42.0), typ.Integer, false},
		{"integer rejects fractional", LNumber(42.5), typ.Integer, true},
		{"integer rejects string", LString("42"), typ.Integer, true},

		// Strings
		{"string accepts LString", LString("hello"), typ.String, false},
		{"string rejects number", LNumber(42), typ.String, true},
		{"string rejects nil", LNil, typ.String, true},

		// Booleans
		{"boolean accepts true", LTrue, typ.Boolean, false},
		{"boolean accepts false", LFalse, typ.Boolean, false},
		{"boolean rejects number", LNumber(1), typ.Boolean, true},
		{"boolean rejects string", LString("true"), typ.Boolean, true},

		// Nil
		{"nil accepts LNil", LNil, typ.Nil, false},
		{"nil rejects number", LNumber(0), typ.Nil, true},
		{"nil rejects false", LFalse, typ.Nil, true},

		// Any
		{"any accepts number", LNumber(42), typ.Any, false},
		{"any accepts string", LString("hello"), typ.Any, false},
		{"any accepts nil", LNil, typ.Any, false},
		{"any accepts bool", LTrue, typ.Any, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lt := NewLType(tt.typ)
			errs := ctx.Validate(tt.val, lt)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

func TestValidationIntegration_OptionalTypes(t *testing.T) {
	ctx := DefaultValidationContext()

	optNum := typ.NewOptional(typ.Number)
	lt := NewLType(optNum)

	tests := []struct {
		name    string
		val     LValue
		wantErr bool
	}{
		{"optional number accepts number", LNumber(42), false},
		{"optional number accepts nil", LNil, false},
		{"optional number rejects string", LString("42"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

func TestValidationIntegration_UnionTypes(t *testing.T) {
	ctx := DefaultValidationContext()

	strOrNum := typ.NewUnion(typ.String, typ.Number)
	lt := NewLType(strOrNum)

	tests := []struct {
		name    string
		val     LValue
		wantErr bool
	}{
		{"union accepts string", LString("hello"), false},
		{"union accepts number", LNumber(42), false},
		{"union rejects bool", LTrue, true},
		{"union rejects nil", LNil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

func TestValidationIntegration_SimpleRecord(t *testing.T) {
	ctx := DefaultValidationContext()

	userType := typ.NewRecord().
		Field("age", typ.Number).
		Field("email", typ.String).
		Field("name", typ.String).
		Build()

	lt := NewLType(userType)

	tests := []struct {
		name       string
		val        *LTable
		wantErrors int
	}{
		{
			"all valid",
			&LTable{Strdict: map[string]LValue{
				"age":   LNumber(25),
				"email": LString("test@example.com"),
				"name":  LString("John"),
			}},
			0,
		},
		{
			"wrong type for age",
			&LTable{Strdict: map[string]LValue{
				"age":   LString("not a number"),
				"email": LString("test@example.com"),
				"name":  LString("John"),
			}},
			1,
		},
		{
			"wrong type for email",
			&LTable{Strdict: map[string]LValue{
				"age":   LNumber(25),
				"email": LNumber(123),
				"name":  LString("John"),
			}},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("Validate() got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s: %s", e.Field, e.Message)
				}
			}
		})
	}
}

func TestValidationIntegration_NestedRecords(t *testing.T) {
	ctx := DefaultValidationContext()

	addressType := typ.NewRecord().
		Field("zip", typ.String).
		Field("street", typ.String).
		Build()

	personType := typ.NewRecord().
		Field("name", typ.String).
		Field("address", addressType).
		Build()

	lt := NewLType(personType)

	tests := []struct {
		name       string
		val        *LTable
		wantErrors int
	}{
		{
			"valid nested",
			&LTable{Strdict: map[string]LValue{
				"name": LString("John"),
				"address": &LTable{Strdict: map[string]LValue{
					"zip":    LString("12345"),
					"street": LString("Main St"),
				}},
			}},
			0,
		},
		{
			"wrong type for nested field",
			&LTable{Strdict: map[string]LValue{
				"name": LString("John"),
				"address": &LTable{Strdict: map[string]LValue{
					"zip":    LNumber(12345),
					"street": LString("Main St"),
				}},
			}},
			1,
		},
		{
			"address is not a table",
			&LTable{Strdict: map[string]LValue{
				"name":    LString("John"),
				"address": LString("not a table"),
			}},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("Validate() got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s: %s", e.Field, e.Message)
				}
			}
		})
	}
}

func TestValidationIntegration_ArrayType(t *testing.T) {
	ctx := DefaultValidationContext()

	scoresType := typ.NewArray(typ.Number)
	lt := NewLType(scoresType)

	tests := []struct {
		name       string
		val        *LTable
		wantErrors int
	}{
		{
			"all valid scores",
			&LTable{Array: []LValue{LNumber(85), LNumber(92), LNumber(78)}},
			0,
		},
		{
			"contains string",
			&LTable{Array: []LValue{LNumber(85), LString("bad"), LNumber(78)}},
			1,
		},
		{
			"empty array",
			&LTable{Array: []LValue{}},
			0,
		},
		{
			"array with nil elements",
			&LTable{Array: []LValue{LNumber(85), LNil, LNumber(78)}},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("Validate() got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s: %s", e.Field, e.Message)
				}
			}
		})
	}
}

func TestValidationIntegration_ErrorFieldPaths(t *testing.T) {
	ctx := DefaultValidationContext()

	// Deep nesting to test field paths
	level3 := typ.NewRecord().
		Field("value", typ.Number).
		Build()

	level2 := typ.NewRecord().
		Field("level3", level3).
		Build()

	level1 := typ.NewRecord().
		Field("level2", level2).
		Build()

	lt := NewLType(level1)

	val := &LTable{Strdict: map[string]LValue{
		"level2": &LTable{Strdict: map[string]LValue{
			"level3": &LTable{Strdict: map[string]LValue{
				"value": LString("not a number"),
			}},
		}},
	}}

	errs := ctx.Validate(val, lt)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}

	expectedPath := "level2.level3.value"
	if errs[0].Field != expectedPath {
		t.Errorf("expected field path %q, got %q", expectedPath, errs[0].Field)
	}
}

func TestValidationIntegration_TypeMismatch(t *testing.T) {
	ctx := DefaultValidationContext()

	recordType := typ.NewRecord().
		Field("num", typ.Number).
		Field("str", typ.String).
		Build()

	lt := NewLType(recordType)

	// Wrong types for fields
	val := &LTable{Strdict: map[string]LValue{
		"num": LString("not a number"),
		"str": LNumber(42),
	}}

	errs := ctx.Validate(val, lt)
	if len(errs) != 2 {
		t.Errorf("expected 2 type mismatch errors, got %d", len(errs))
	}
}

func TestValidationIntegration_LiteralTypes(t *testing.T) {
	ctx := DefaultValidationContext()

	tests := []struct {
		name    string
		typ     typ.Type
		val     LValue
		wantErr bool
	}{
		{"string literal match", typ.LiteralString("hello"), LString("hello"), false},
		{"string literal mismatch", typ.LiteralString("hello"), LString("world"), true},
		{"number literal match", typ.LiteralNumber(42), LNumber(42), false},
		{"number literal mismatch", typ.LiteralNumber(42), LNumber(43), true},
		{"bool literal match true", typ.LiteralBool(true), LTrue, false},
		{"bool literal mismatch", typ.LiteralBool(true), LFalse, true},
		{"int64 literal match", typ.LiteralInt(42), LInteger(42), false},
		{"int64 literal mismatch", typ.LiteralInt(42), LInteger(43), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lt := NewLType(tt.typ)
			errs := ctx.Validate(tt.val, lt)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

func TestValidationIntegration_MapType(t *testing.T) {
	ctx := DefaultValidationContext()

	mapType := typ.NewMap(typ.String, typ.Number)
	lt := NewLType(mapType)

	tests := []struct {
		name    string
		val     *LTable
		wantErr bool
	}{
		{
			"valid string->number map",
			&LTable{Strdict: map[string]LValue{
				"a": LNumber(1),
				"b": LNumber(2),
			}},
			false,
		},
		{
			"invalid value type",
			&LTable{Strdict: map[string]LValue{
				"a": LString("not a number"),
			}},
			true,
		},
		{
			"empty map",
			&LTable{Strdict: map[string]LValue{}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

// Benchmark for integration testing
func BenchmarkValidation_SimpleRecord(b *testing.B) {
	ctx := DefaultValidationContext()

	userType := typ.NewRecord().
		Field("age", typ.Number).
		Field("name", typ.String).
		Build()

	lt := NewLType(userType)
	val := &LTable{Strdict: map[string]LValue{
		"age":  LNumber(25),
		"name": LString("John"),
	}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Validate(val, lt)
	}
}

func BenchmarkValidation_NestedRecord(b *testing.B) {
	ctx := DefaultValidationContext()

	addressType := typ.NewRecord().
		Field("zip", typ.String).
		Build()

	personType := typ.NewRecord().
		Field("name", typ.String).
		Field("address", addressType).
		Build()

	lt := NewLType(personType)
	val := &LTable{Strdict: map[string]LValue{
		"name": LString("John"),
		"address": &LTable{Strdict: map[string]LValue{
			"zip": LString("12345"),
		}},
	}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Validate(val, lt)
	}
}

func BenchmarkValidation_LargeArray(b *testing.B) {
	ctx := DefaultValidationContext()

	arrayType := typ.NewArray(typ.Number)
	lt := NewLType(arrayType)

	arr := make([]LValue, 100)
	for i := 0; i < 100; i++ {
		arr[i] = LNumber(float64(i))
	}
	val := &LTable{Array: arr}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Validate(val, lt)
	}
}

// Annotation validation tests

func TestAnnotation_MinMax(t *testing.T) {
	ctx := DefaultValidationContext()

	// number @min(0) @max(100)
	annotatedType := typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
		{Name: "max", Arg: float64(100)},
	})
	lt := NewLType(annotatedType)

	tests := []struct {
		name       string
		val        LValue
		wantErrors int
	}{
		{"valid in range", LNumber(50), 0},
		{"valid at min", LNumber(0), 0},
		{"valid at max", LNumber(100), 0},
		{"below min", LNumber(-1), 1},
		{"above max", LNumber(101), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s", e.Message)
				}
			}
		})
	}
}

func TestAnnotation_MinLen_MaxLen(t *testing.T) {
	ctx := DefaultValidationContext()

	// string @min_len(2) @max_len(10)
	annotatedType := typ.NewAnnotated(typ.String, []typ.Annotation{
		{Name: "min_len", Arg: float64(2)},
		{Name: "max_len", Arg: float64(10)},
	})
	lt := NewLType(annotatedType)

	tests := []struct {
		name       string
		val        LValue
		wantErrors int
	}{
		{"valid length", LString("hello"), 0},
		{"at min", LString("ab"), 0},
		{"at max", LString("abcdefghij"), 0},
		{"too short", LString("a"), 1},
		{"too long", LString("abcdefghijk"), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s", e.Message)
				}
			}
		})
	}
}

func TestAnnotation_Pattern(t *testing.T) {
	ctx := DefaultValidationContext()

	// string @pattern("^[a-z]+$")
	annotatedType := typ.NewAnnotated(typ.String, []typ.Annotation{
		{Name: "pattern", Arg: "^[a-z]+$"},
	})
	lt := NewLType(annotatedType)

	tests := []struct {
		name       string
		val        LValue
		wantErrors int
	}{
		{"matches", LString("hello"), 0},
		{"no match uppercase", LString("Hello"), 1},
		{"no match numbers", LString("hello123"), 1},
		{"empty no match", LString(""), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s", e.Message)
				}
			}
		})
	}
}

func TestAnnotation_RecordFieldAnnotations(t *testing.T) {
	ctx := DefaultValidationContext()

	// type User = {age: number @min(0) @max(150), name: string @min_len(1)}
	userType := typ.NewRecord().
		AnnotatedField("age", typ.Number, false, []typ.Annotation{
			{Name: "min", Arg: float64(0)},
			{Name: "max", Arg: float64(150)},
		}).
		AnnotatedField("name", typ.String, false, []typ.Annotation{
			{Name: "min_len", Arg: float64(1)},
		}).
		Build()
	lt := NewLType(userType)

	tests := []struct {
		name       string
		val        *LTable
		wantErrors int
	}{
		{
			"all valid",
			&LTable{Strdict: map[string]LValue{
				"age":  LNumber(30),
				"name": LString("John"),
			}},
			0,
		},
		{
			"age below min",
			&LTable{Strdict: map[string]LValue{
				"age":  LNumber(-5),
				"name": LString("John"),
			}},
			1,
		},
		{
			"age above max",
			&LTable{Strdict: map[string]LValue{
				"age":  LNumber(200),
				"name": LString("John"),
			}},
			1,
		},
		{
			"name too short",
			&LTable{Strdict: map[string]LValue{
				"age":  LNumber(30),
				"name": LString(""),
			}},
			1,
		},
		{
			"both invalid",
			&LTable{Strdict: map[string]LValue{
				"age":  LNumber(-1),
				"name": LString(""),
			}},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s: %s", e.Field, e.Message)
				}
			}
		})
	}
}

func TestAnnotation_NestedRecordAnnotations(t *testing.T) {
	ctx := DefaultValidationContext()

	// type Address = {zip: string @pattern("^[0-9]{5}$")}
	// type Person = {name: string, address: Address}
	addressType := typ.NewRecord().
		AnnotatedField("zip", typ.String, false, []typ.Annotation{
			{Name: "pattern", Arg: "^[0-9]{5}$"},
		}).
		Build()

	personType := typ.NewRecord().
		Field("name", typ.String).
		Field("address", addressType).
		Build()
	lt := NewLType(personType)

	tests := []struct {
		name       string
		val        *LTable
		wantErrors int
	}{
		{
			"valid zip",
			&LTable{Strdict: map[string]LValue{
				"name": LString("John"),
				"address": &LTable{Strdict: map[string]LValue{
					"zip": LString("12345"),
				}},
			}},
			0,
		},
		{
			"invalid zip format",
			&LTable{Strdict: map[string]LValue{
				"name": LString("John"),
				"address": &LTable{Strdict: map[string]LValue{
					"zip": LString("1234"),
				}},
			}},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ctx.Validate(tt.val, lt)
			if len(errs) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(errs), tt.wantErrors)
				for _, e := range errs {
					t.Logf("  error: %s: %s", e.Field, e.Message)
				}
			}
		})
	}
}

func TestAnnotation_ErrorPath(t *testing.T) {
	ctx := DefaultValidationContext()

	// Test that error paths are correct for nested structures
	addressType := typ.NewRecord().
		AnnotatedField("zip", typ.String, false, []typ.Annotation{
			{Name: "min_len", Arg: float64(5)},
		}).
		Build()

	personType := typ.NewRecord().
		Field("address", addressType).
		Build()
	lt := NewLType(personType)

	val := &LTable{Strdict: map[string]LValue{
		"address": &LTable{Strdict: map[string]LValue{
			"zip": LString("123"),
		}},
	}}

	errs := ctx.Validate(val, lt)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "address.zip" {
		t.Errorf("expected field 'address.zip', got %q", errs[0].Field)
	}
}

func TestAnnotation_TypeString(t *testing.T) {
	// Verify annotated types render correctly
	annotatedType := typ.NewAnnotated(typ.Number, []typ.Annotation{
		{Name: "min", Arg: float64(0)},
		{Name: "max", Arg: float64(100)},
	})

	expected := "number @min(0) @max(100)"
	if annotatedType.String() != expected {
		t.Errorf("got %q, want %q", annotatedType.String(), expected)
	}
}

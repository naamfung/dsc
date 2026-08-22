package lua

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestLTypeBasic(t *testing.T) {
	// Test primitive singletons
	if LTypeNumber.Type() != LTType {
		t.Error("LTypeNumber should have LTType")
	}
	if LTypeNumber.String() != "number" {
		t.Errorf("LTypeNumber.String() = %q, want %q", LTypeNumber.String(), "number")
	}
	if LTypeNumber.KindString() != "number" {
		t.Errorf("LTypeNumber.KindString() = %q, want %q", LTypeNumber.KindString(), "number")
	}
}

func TestLTypeNilInner(t *testing.T) {
	L := NewState()
	defer L.Close()

	lt := NewLType(nil)
	if lt.String() != "unknown" {
		t.Errorf("nil inner String() = %q, want %q", lt.String(), "unknown")
	}
	if lt.KindString() != "unknown" {
		t.Errorf("nil inner KindString() = %q, want %q", lt.KindString(), "unknown")
	}
	if lt.Validate(L, LNumber(42)) {
		t.Error("nil inner Validate() should return false")
	}

	L.SetGlobal("NilType", lt)
	err := L.DoString(`
		local val, err = NilType:is(42)
		assert(val == nil, "nil type should not validate")
		assert(err ~= nil, "nil type should return error")
	`)
	if err != nil {
		t.Fatalf("nil inner Type:is failed: %v", err)
	}
}

func TestLTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		name     string
		typ      *LType
		value    LValue
		expected bool
	}{
		{"number validates number", LTypeNumber, LNumber(42), true},
		{"number validates integer", LTypeNumber, LInteger(42), true},
		{"number rejects string", LTypeNumber, LString("hello"), false},
		{"string validates string", LTypeString, LString("hello"), true},
		{"string rejects number", LTypeString, LNumber(42), false},
		{"boolean validates true", LTypeBoolean, LTrue, true},
		{"boolean validates false", LTypeBoolean, LFalse, true},
		{"boolean rejects number", LTypeBoolean, LNumber(1), false},
		{"nil validates nil", LTypeNil, LNil, true},
		{"nil rejects number", LTypeNil, LNumber(0), false},
		{"any validates anything", LTypeAny, LNumber(42), true},
		{"any validates string", LTypeAny, LString("hello"), true},
		{"any validates nil", LTypeAny, LNil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.typ.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeOptional(t *testing.T) {
	L := NewState()
	defer L.Close()

	optNumber := &LType{inner: typ.NewOptional(typ.Number)}

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"optional number accepts number", LNumber(42), true},
		{"optional number accepts nil", LNil, true},
		{"optional number rejects string", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := optNumber.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeRecord(t *testing.T) {
	L := NewState()
	defer L.Close()

	// type Point = {x: number, y: number}
	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}

	// Valid point
	validPoint := L.NewTable()
	validPoint.RawSetString("x", LNumber(1))
	validPoint.RawSetString("y", LNumber(2))

	// Invalid point (missing y)
	invalidPoint := L.NewTable()
	invalidPoint.RawSetString("x", LNumber(1))

	// Invalid point (wrong type)
	wrongType := L.NewTable()
	wrongType.RawSetString("x", LString("not a number"))
	wrongType.RawSetString("y", LNumber(2))

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"valid point", validPoint, true},
		{"missing field (y is nil)", invalidPoint, false},
		{"wrong field type", wrongType, false},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pointType.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeArray(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {number}
	numberArray := &LType{inner: typ.NewArray(typ.Number)}

	// Valid array
	validArray := L.NewTable()
	validArray.Append(LNumber(1))
	validArray.Append(LNumber(2))
	validArray.Append(LNumber(3))

	// Invalid array (contains string)
	invalidArray := L.NewTable()
	invalidArray.Append(LNumber(1))
	invalidArray.Append(LString("two"))

	// Empty array is valid
	emptyArray := L.NewTable()

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"valid number array", validArray, true},
		{"invalid mixed array", invalidArray, false},
		{"empty array", emptyArray, true},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := numberArray.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeUnion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// number | string
	numOrStr := &LType{inner: typ.NewUnion(typ.Number, typ.String)}

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"number in union", LNumber(42), true},
		{"string in union", LString("hello"), true},
		{"boolean not in union", LTrue, false},
		{"nil not in union", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := numOrStr.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeGetField(t *testing.T) {
	L := NewState()
	defer L.Close()

	// type Point = {x: number, y: number}
	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}

	// Test field access
	xType := L.typeGetField(pointType, "x")
	if xType == LNil {
		t.Error("Point.x should not be nil")
	}
	if lt, ok := xType.(*LType); ok {
		if lt.KindString() != "number" {
			t.Errorf("Point.x kind = %q, want %q", lt.KindString(), "number")
		}
	} else {
		t.Error("Point.x should be LType")
	}

	// Test method access
	kindMethod := L.typeGetField(pointType, "kind")
	if kindMethod == LNil {
		t.Error("Point:kind should not be nil")
	}
	if _, ok := kindMethod.(LGoFunc); !ok {
		t.Error("Point:kind should be LGoFunc")
	}

	// Test nonexistent field
	noField := L.typeGetField(pointType, "z")
	if noField != LNil {
		t.Error("Point.z should be nil")
	}
}

func TestTypeComparison(t *testing.T) {
	// Test TypeEquals
	if !TypeEquals(LTypeNumber, LTypeNumber) {
		t.Error("number should equal number")
	}
	if TypeEquals(LTypeNumber, LTypeString) {
		t.Error("number should not equal string")
	}

	// Test TypeIsSubtype
	if !TypeIsSubtype(LTypeInteger, LTypeNumber) {
		t.Error("integer should be subtype of number")
	}
	if TypeIsSubtype(LTypeNumber, LTypeInteger) {
		t.Error("number should not be subtype of integer")
	}
}

func TestLTypeMap(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {[string]: number}
	strToNum := &LType{inner: typ.NewMap(typ.String, typ.Number)}

	// Valid map
	validMap := L.NewTable()
	validMap.RawSetString("a", LNumber(1))
	validMap.RawSetString("b", LNumber(2))

	// Invalid map (wrong value type)
	invalidMap := L.NewTable()
	invalidMap.RawSetString("a", LString("not a number"))

	// Empty map
	emptyMap := L.NewTable()

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"valid string->number map", validMap, true},
		{"invalid value type", invalidMap, false},
		{"empty map", emptyMap, true},
		{"not a table", LNumber(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strToNum.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	// (number) -> string
	fnType := &LType{inner: typ.Func().Param("a", typ.Number).Returns(typ.String).Build()}

	// Create a Lua function
	luaFn := L.NewFunction(func(L *LState) int {
		return 0
	})

	// Create a Go function
	goFn := LGoFunc(func(L *LState) int {
		return 0
	})

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"Lua function", luaFn, true},
		{"Go function", goFn, true},
		{"not a function", LNumber(42), false},
		{"table is not function", L.NewTable(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fnType.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeMethods(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create test types
	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}

	arrayType := &LType{inner: typ.NewArray(typ.Number)}
	mapType := &LType{inner: typ.NewMap(typ.String, typ.Number)}
	optType := &LType{inner: typ.NewOptional(typ.Number)}
	fnType := &LType{inner: typ.Func().Param("a", typ.Number).Returns(typ.String).Build()}

	// Test :kind()
	t.Run("kind method", func(t *testing.T) {
		tests := []struct {
			typ      *LType
			expected string
		}{
			{LTypeNumber, "number"},
			{LTypeString, "string"},
			{pointType, "record"},
			{arrayType, "array"},
			{mapType, "map"},
			{optType, "optional"},
			{fnType, "function"},
		}
		for _, tt := range tests {
			if tt.typ.KindString() != tt.expected {
				t.Errorf("%s.KindString() = %q, want %q", tt.typ.String(), tt.typ.KindString(), tt.expected)
			}
		}
	})

	// Test :name()
	t.Run("name method", func(t *testing.T) {
		if pointType.Name() != "Point" {
			t.Errorf("pointType.Name() = %q, want %q", pointType.Name(), "Point")
		}
		if arrayType.Name() != "" {
			t.Errorf("arrayType.Name() = %q, want empty", arrayType.Name())
		}
	})

	// Test :elem()
	t.Run("elem method", func(t *testing.T) {
		elemMethod := L.typeGetField(arrayType, "elem")
		if elemMethod == LNil {
			t.Fatal("elem method should not be nil")
		}
		// Call the method
		L.Push(elemMethod)
		L.Call(0, 1)
		result := L.Get(-1)
		L.Pop(1)
		if lt, ok := result.(*LType); ok {
			if lt.KindString() != "number" {
				t.Errorf("elem() returned %s, want number", lt.KindString())
			}
		} else {
			t.Errorf("elem() returned %T, want *LType", result)
		}
	})

	// Test :key() and :val()
	t.Run("key and val methods", func(t *testing.T) {
		keyMethod := L.typeGetField(mapType, "key")
		valMethod := L.typeGetField(mapType, "val")
		if keyMethod == LNil || valMethod == LNil {
			t.Fatal("key/val methods should not be nil")
		}

		L.Push(keyMethod)
		L.Call(0, 1)
		keyResult := L.Get(-1).(*LType)
		L.Pop(1)

		L.Push(valMethod)
		L.Call(0, 1)
		valResult := L.Get(-1).(*LType)
		L.Pop(1)

		if keyResult.KindString() != "string" {
			t.Errorf("key() returned %s, want string", keyResult.KindString())
		}
		if valResult.KindString() != "number" {
			t.Errorf("val() returned %s, want number", valResult.KindString())
		}
	})

	// Test :inner()
	t.Run("inner method", func(t *testing.T) {
		innerMethod := L.typeGetField(optType, "inner")
		if innerMethod == LNil {
			t.Fatal("inner method should not be nil")
		}
		L.Push(innerMethod)
		L.Call(0, 1)
		result := L.Get(-1).(*LType)
		L.Pop(1)
		if result.KindString() != "number" {
			t.Errorf("inner() returned %s, want number", result.KindString())
		}
	})

	// Test :ret()
	t.Run("ret method", func(t *testing.T) {
		retMethod := L.typeGetField(fnType, "ret")
		if retMethod == LNil {
			t.Fatal("ret method should not be nil")
		}
		L.Push(retMethod)
		L.Call(0, 1)
		result := L.Get(-1).(*LType)
		L.Pop(1)
		if result.KindString() != "string" {
			t.Errorf("ret() returned %s, want string", result.KindString())
		}
	})
}

func TestLTypeIterators(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test :fields() iterator
	t.Run("fields iterator", func(t *testing.T) {
		pointType := &LType{
			inner: typ.NewRecord().
				Field("x", typ.Number).
				Field("y", typ.Number).
				Build(),
			name: "Point",
		}

		fieldsMethod := L.typeGetField(pointType, "fields")
		if fieldsMethod == LNil {
			t.Fatal("fields method should not be nil")
		}

		// Call fields() to get iterator
		L.Push(fieldsMethod)
		L.Call(0, 1)
		iter := L.Get(-1)
		L.Pop(1)

		// Collect all fields
		var fieldNames []string
		for {
			L.Push(iter)
			L.Call(0, 2)
			name := L.Get(-2)
			L.Pop(2)
			if name == LNil {
				break
			}
			fieldNames = append(fieldNames, string(name.(LString)))
		}

		if len(fieldNames) != 2 {
			t.Errorf("expected 2 fields, got %d", len(fieldNames))
		}
		if fieldNames[0] != "x" || fieldNames[1] != "y" {
			t.Errorf("expected [x, y], got %v", fieldNames)
		}
	})

	// Test :variants() iterator
	t.Run("variants iterator", func(t *testing.T) {
		unionType := &LType{
			inner: typ.NewUnion(typ.Number, typ.String, typ.Boolean),
		}

		variantsMethod := L.typeGetField(unionType, "variants")
		if variantsMethod == LNil {
			t.Fatal("variants method should not be nil")
		}

		L.Push(variantsMethod)
		L.Call(0, 1)
		iter := L.Get(-1)
		L.Pop(1)

		// Count variants
		count := 0
		for {
			L.Push(iter)
			L.Call(0, 1)
			variant := L.Get(-1)
			L.Pop(1)
			if variant == LNil {
				break
			}
			count++
		}

		if count != 3 {
			t.Errorf("expected 3 variants, got %d", count)
		}
	})

	// Test :params() iterator
	t.Run("params iterator", func(t *testing.T) {
		fnType := &LType{
			inner: typ.Func().
				Param("a", typ.Number).
				Param("b", typ.String).
				Returns(typ.Boolean).
				Build(),
		}

		paramsMethod := L.typeGetField(fnType, "params")
		if paramsMethod == LNil {
			t.Fatal("params method should not be nil")
		}

		L.Push(paramsMethod)
		L.Call(0, 1)
		iter := L.Get(-1)
		L.Pop(1)

		// Count params
		count := 0
		for {
			L.Push(iter)
			L.Call(0, 1)
			param := L.Get(-1)
			L.Pop(1)
			if param == LNil {
				break
			}
			count++
		}

		if count != 2 {
			t.Errorf("expected 2 params, got %d", count)
		}
	})
}

func TestLTypeIsMethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}

	// Valid point
	validPoint := L.NewTable()
	validPoint.RawSetString("x", LNumber(1))
	validPoint.RawSetString("y", LNumber(2))

	// Invalid point
	invalidPoint := L.NewTable()
	invalidPoint.RawSetString("x", LString("not a number"))

	// Get :is method
	isMethod := L.typeGetField(pointType, "is")
	if isMethod == LNil {
		t.Fatal(":is method should not be nil")
	}

	// Test with valid point - returns (value, nil)
	L.Push(isMethod)
	L.Push(validPoint)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)
	if val == LNil {
		t.Error(":is(validPoint) should return value, got nil")
	}
	if errVal != LNil {
		t.Errorf(":is(validPoint) should return nil error, got %v", errVal)
	}

	// Test with invalid point - returns (nil, error)
	L.Push(isMethod)
	L.Push(invalidPoint)
	L.Call(1, 2)
	val = L.Get(-2)
	errVal = L.Get(-1)
	L.Pop(2)
	if val != LNil {
		t.Errorf(":is(invalidPoint) should return nil, got %v", val)
	}
	if errVal == LNil {
		t.Error(":is(invalidPoint) should return error message")
	}
}

func TestLTypeVMCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a type as global
	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)

	// Test calling type for validation (valid)
	err := L.DoString(`
		local p = Point({x = 1, y = 2})
		assert(p.x == 1)
		assert(p.y == 2)
	`)
	if err != nil {
		t.Errorf("valid Point validation failed: %v", err)
	}

	// Test calling type for validation (invalid) - should error
	err = L.DoString(`
		local p = Point({x = "bad", y = 2})
	`)
	if err == nil {
		t.Error("invalid Point validation should have failed")
	}
}

func TestLTypeVMCall_ArityAndMixedArgs(t *testing.T) {
	L := NewState()
	defer L.Close()

	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)
	L.SetGlobal("Number", LTypeNumber)

	// Mixed type/value args should error.
	err := L.DoString(`local _ = Point(Number, {x = 1, y = 2})`)
	if err == nil {
		t.Error("mixed type/value args should have failed")
	}

	// Multiple value args should error.
	err = L.DoString(`local _ = Point({x = 1, y = 2}, {x = 3, y = 4})`)
	if err == nil {
		t.Error("multiple value args should have failed")
	}

	// Non-generic type with type args should error.
	err = L.DoString(`local _ = Point(Number)`)
	if err == nil {
		t.Error("non-generic type with type args should have failed")
	}
}

func TestLTypeVMFieldAccess(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register types
	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)

	// Test field access
	err := L.DoString(`
		local xType = Point.x
		assert(xType:kind() == "number", "Point.x should be number")
	`)
	if err != nil {
		t.Errorf("field access failed: %v", err)
	}

	// Test method access
	err = L.DoString(`
		assert(Point:kind() == "record", "Point:kind() should be record")
		assert(Point:name() == "Point", "Point:name() should be Point")
	`)
	if err != nil {
		t.Errorf("method access failed: %v", err)
	}

	// Test :is() method - returns (value, nil) on success, (nil, error) on failure
	err = L.DoString(`
		local valid = {x = 1, y = 2}
		local invalid = {x = "bad"}
		local val, err = Point:is(valid)
		assert(val ~= nil, "valid point should return value")
		assert(err == nil, "valid point should return nil error")
		val, err = Point:is(invalid)
		assert(val == nil, "invalid point should return nil")
		assert(err ~= nil, "invalid point should return error")
	`)
	if err != nil {
		t.Errorf(":is() method failed: %v", err)
	}
}

func TestLTypeVMFieldAccess_MethodPrecedence(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Record field named "is" should not shadow the Type:is method.
	pointType := &LType{
		inner: typ.NewRecord().
			Field("is", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)

	err := L.DoString(`
		assert(type(Point.is) == "function", "Point.is should resolve to method")
		local val, err = Point:is({is = 1})
		assert(val ~= nil, "Point:is should succeed even with field named is")
		assert(err == nil, "Point:is should return nil error on success")

		local iter = Point:fields()
		local name, t = iter()
		assert(name == "is", "Point:fields should expose field name")
		assert(t:kind() == "number", "Point:fields should expose field type")
	`)
	if err != nil {
		t.Fatalf("method precedence failed: %v", err)
	}
}

func TestLTypeVMComparison(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register types
	L.SetGlobal("Number", LTypeNumber)
	L.SetGlobal("Integer", LTypeInteger)
	L.SetGlobal("String", LTypeString)

	// Test equality
	err := L.DoString(`
		assert(Number == Number, "Number should equal Number")
		assert(not (Number == String), "Number should not equal String")
	`)
	if err != nil {
		t.Errorf("equality test failed: %v", err)
	}

	// Test subtype (<=)
	err = L.DoString(`
		assert(Integer <= Number, "Integer should be subtype of Number")
		assert(Number <= Number, "Number should be subtype of itself")
	`)
	if err != nil {
		t.Errorf("subtype test failed: %v", err)
	}

	// Test strict subtype (<)
	err = L.DoString(`
		assert(Integer < Number, "Integer should be strict subtype of Number")
		assert(not (Number < Number), "Number should not be strict subtype of itself")
	`)
	if err != nil {
		t.Errorf("strict subtype test failed: %v", err)
	}
}

func TestLTypeVMIterators(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register type
	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)

	// Test :fields() iterator in Lua
	err := L.DoString(`
		local fields = {}
		for name, typ in Point:fields() do
			fields[name] = typ:kind()
		end
		assert(fields.x == "number", "x should be number")
		assert(fields.y == "number", "y should be number")
	`)
	if err != nil {
		t.Errorf("fields iterator failed: %v", err)
	}
}

func TestLTypeToString(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("Number", LTypeNumber)

	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)

	// Test tostring
	err := L.DoString(`
		assert(tostring(Number) == "number", "tostring(Number) should be 'number'")
		assert(tostring(Point) == "Point", "tostring(Point) should be 'Point'")
	`)
	if err != nil {
		t.Errorf("tostring test failed: %v", err)
	}
}

func TestLTypeAsTableKey(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("Number", LTypeNumber)
	L.SetGlobal("String", LTypeString)

	// Test using types as table keys
	err := L.DoString(`
		local handlers = {}
		handlers[Number] = function() return "number handler" end
		handlers[String] = function() return "string handler" end

		assert(handlers[Number]() == "number handler")
		assert(handlers[String]() == "string handler")
	`)
	if err != nil {
		t.Errorf("type as table key failed: %v", err)
	}
}

func TestLTypeLuaType(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("Number", LTypeNumber)

	// Test that type() returns "type" for LType values
	err := L.DoString(`
		assert(type(Number) == "type", "type(Number) should be 'type'")
		assert(type(123) == "number", "type(123) should be 'number'")
	`)
	if err != nil {
		t.Errorf("type() test failed: %v", err)
	}
}

// TestTypeMethodIs_ReturnsValueOnSuccess tests that Type:is(val) returns (val, nil) on success.
func TestTypeMethodIs_ReturnsValueOnSuccess(t *testing.T) {
	L := NewState()
	defer L.Close()

	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}

	validPoint := L.NewTable()
	validPoint.RawSetString("x", LNumber(1))
	validPoint.RawSetString("y", LNumber(2))

	isMethod := L.typeGetField(pointType, "is")
	if isMethod == LNil {
		t.Fatal(":is method should not be nil")
	}

	// Call Type:is(validPoint)
	L.Push(isMethod)
	L.Push(validPoint)
	L.Call(1, 2) // Expect 2 return values

	val := L.Get(-2)
	err := L.Get(-1)
	L.Pop(2)

	// On success: (value, nil)
	if val == LNil {
		t.Error("Type:is(validValue) should return the value as first return, got nil")
	}
	if val != validPoint {
		t.Errorf("Type:is(validValue) should return the same value, got %v", val)
	}
	if err != LNil {
		t.Errorf("Type:is(validValue) should return nil as error, got %v", err)
	}
}

// TestTypeMethodIs_ReturnsErrorOnFailure tests that Type:is(val) returns (nil, error) on failure.
func TestTypeMethodIs_ReturnsErrorOnFailure(t *testing.T) {
	L := NewState()
	defer L.Close()

	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}

	invalidPoint := L.NewTable()
	invalidPoint.RawSetString("x", LString("not a number"))
	invalidPoint.RawSetString("y", LNumber(2))

	isMethod := L.typeGetField(pointType, "is")
	if isMethod == LNil {
		t.Fatal(":is method should not be nil")
	}

	// Call Type:is(invalidPoint)
	L.Push(isMethod)
	L.Push(invalidPoint)
	L.Call(1, 2) // Expect 2 return values

	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	// On failure: (nil, error)
	if val != LNil {
		t.Errorf("Type:is(invalidValue) should return nil as first return, got %v", val)
	}
	if errVal == LNil {
		t.Error("Type:is(invalidValue) should return an error message, got nil")
	}
}

func TestTypeMethodIs_OptionalFieldMissing(t *testing.T) {
	L := NewState()
	defer L.Close()

	personType := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			OptField("age", typ.Number).
			Build(),
		name: "Person",
	}

	isMethod := L.typeGetField(personType, "is")
	if isMethod == LNil {
		t.Fatal(":is method should not be nil")
	}

	valTable := L.NewTable()
	valTable.RawSetString("name", LString("Ada"))

	L.Push(isMethod)
	L.Push(valTable)
	L.Call(1, 2)

	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Error("Type:is should succeed when optional field is missing")
	}
	if errVal != LNil {
		t.Errorf("Type:is should return nil error when optional field is missing, got %v", errVal)
	}
}

func TestTypeMethodIs_AnnotatedType(t *testing.T) {
	L := NewState()
	defer L.Close()

	annotated := &LType{
		inner: typ.NewAnnotated(typ.Number, []typ.Annotation{
			{Name: "min", Arg: float64(0)},
		}),
		name: "NonNegative",
	}

	isMethod := L.typeGetField(annotated, "is")
	if isMethod == LNil {
		t.Fatal(":is method should not be nil")
	}

	L.Push(isMethod)
	L.Push(LNumber(5))
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Error("Type:is should succeed for valid annotated value")
	}
	if errVal != LNil {
		t.Errorf("Type:is should return nil error for valid annotated value, got %v", errVal)
	}

	L.Push(isMethod)
	L.Push(LNumber(-1))
	L.Call(1, 2)
	val = L.Get(-2)
	errVal = L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Errorf("Type:is should return nil value for invalid annotated value, got %v", val)
	}
	if errVal == LNil {
		t.Error("Type:is should return error for invalid annotated value")
	}
}

func TestTypeMethodIs_AnnotatedArrayMinLen(t *testing.T) {
	L := NewState()
	defer L.Close()

	annotated := &LType{
		inner: typ.NewAnnotated(typ.NewArray(typ.Number), []typ.Annotation{
			{Name: "min_len", Arg: float64(1)},
		}),
		name: "NumList",
	}

	isMethod := L.typeGetField(annotated, "is")
	if isMethod == LNil {
		t.Fatal(":is method should not be nil")
	}

	empty := L.NewTable()
	L.Push(isMethod)
	L.Push(empty)
	L.Call(1, 2)
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Errorf("Type:is should fail for empty array, got %v", val)
	}
	if errVal == LNil {
		t.Error("Type:is should return error for empty array")
	}

	nonEmpty := L.NewTable()
	nonEmpty.Append(LNumber(1))
	L.Push(isMethod)
	L.Push(nonEmpty)
	L.Call(1, 2)
	val = L.Get(-2)
	errVal = L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Error("Type:is should pass for non-empty array")
	}
	if errVal != LNil {
		t.Errorf("Type:is should return nil error for non-empty array, got %v", errVal)
	}
}

// TestTypeMethodIs_PrimitiveTypes tests Type:is with primitive types.
func TestTypeMethodIs_PrimitiveTypes(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		name        string
		typ         *LType
		value       LValue
		shouldMatch bool
	}{
		{"number matches number", LTypeNumber, LNumber(42), true},
		{"number matches integer", LTypeNumber, LInteger(42), true},
		{"number rejects string", LTypeNumber, LString("hello"), false},
		{"string matches string", LTypeString, LString("hello"), true},
		{"string rejects number", LTypeString, LNumber(42), false},
		{"boolean matches true", LTypeBoolean, LTrue, true},
		{"boolean matches false", LTypeBoolean, LFalse, true},
		{"boolean rejects number", LTypeBoolean, LNumber(1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMethod := L.typeGetField(tt.typ, "is")
			if isMethod == LNil {
				t.Fatal(":is method should not be nil")
			}

			L.Push(isMethod)
			L.Push(tt.value)
			L.Call(1, 2)

			val := L.Get(-2)
			errVal := L.Get(-1)
			L.Pop(2)

			if tt.shouldMatch {
				if val == LNil {
					t.Error("expected value to be returned, got nil")
				}
				if errVal != LNil {
					t.Errorf("expected nil error, got %v", errVal)
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

// TestTypeMethodIs_UnionType tests Type:is with union types.
func TestTypeMethodIs_UnionType(t *testing.T) {
	L := NewState()
	defer L.Close()

	// number | string
	unionType := &LType{
		inner: typ.NewUnion(typ.Number, typ.String),
	}

	tests := []struct {
		name        string
		value       LValue
		shouldMatch bool
	}{
		{"number in union", LNumber(42), true},
		{"string in union", LString("hello"), true},
		{"boolean not in union", LTrue, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMethod := L.typeGetField(unionType, "is")
			if isMethod == LNil {
				t.Fatal(":is method should not be nil")
			}

			L.Push(isMethod)
			L.Push(tt.value)
			L.Call(1, 2)

			val := L.Get(-2)
			errVal := L.Get(-1)
			L.Pop(2)

			if tt.shouldMatch {
				if val == LNil {
					t.Error("expected value to be returned, got nil")
				}
				if errVal != LNil {
					t.Errorf("expected nil error, got %v", errVal)
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

// TestTypeMethodIs_OptionalType tests Type:is with optional types.
func TestTypeMethodIs_OptionalType(t *testing.T) {
	L := NewState()
	defer L.Close()

	// number?
	optionalType := &LType{
		inner: typ.NewOptional(typ.Number),
	}

	tests := []struct {
		name        string
		value       LValue
		shouldMatch bool
	}{
		{"number matches optional", LNumber(42), true},
		{"nil matches optional", LNil, true},
		{"string rejects optional", LString("hello"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMethod := L.typeGetField(optionalType, "is")
			if isMethod == LNil {
				t.Fatal(":is method should not be nil")
			}

			L.Push(isMethod)
			L.Push(tt.value)
			L.Call(1, 2)

			val := L.Get(-2)
			errVal := L.Get(-1)
			L.Pop(2)

			if tt.shouldMatch {
				// For optional types, even nil is a valid match
				if val != tt.value {
					t.Errorf("expected value %v to be returned, got %v", tt.value, val)
				}
				if errVal != LNil {
					t.Errorf("expected nil error, got %v", errVal)
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

// TestTypeMethodIs_VMLuaUsage tests using Type:is in actual Lua code.
func TestTypeMethodIs_VMLuaUsage(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a type
	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)

	// Test successful validation - value should be returned
	err := L.DoString(`
		local data = {x = 1, y = 2}
		local val, err = Point:is(data)
		assert(val ~= nil, "val should not be nil for valid data")
		assert(val.x == 1, "val.x should be 1")
		assert(val.y == 2, "val.y should be 2")
		assert(err == nil, "err should be nil for valid data")
	`)
	if err != nil {
		t.Errorf("valid Point:is() test failed: %v", err)
	}

	// Test failed validation - error should be returned
	err = L.DoString(`
		local data = {x = "bad", y = 2}
		local val, err = Point:is(data)
		assert(val == nil, "val should be nil for invalid data")
		assert(err ~= nil, "err should not be nil for invalid data")
	`)
	if err != nil {
		t.Errorf("invalid Point:is() test failed: %v", err)
	}
}

// TestTypeMethodIs_FlowNarrowingPattern tests the idiomatic flow narrowing pattern.
func TestTypeMethodIs_FlowNarrowingPattern(t *testing.T) {
	L := NewState()
	defer L.Close()

	pointType := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
		name: "Point",
	}
	L.SetGlobal("Point", pointType)

	// Test the idiomatic pattern: if val, err = Type:is(data); val then ... end
	err := L.DoString(`
		local function processPoint(data)
			local val, err = Point:is(data)
			if val then
				-- In this branch, val is guaranteed to be a valid Point
				return val.x + val.y
			else
				-- In this branch, err contains the error message
				return nil, err
			end
		end

		-- Test with valid data
		local result = processPoint({x = 10, y = 20})
		assert(result == 30, "expected 30, got " .. tostring(result))

		-- Test with invalid data
		local result2, err2 = processPoint({x = "bad"})
		assert(result2 == nil, "expected nil for invalid data")
		assert(err2 ~= nil, "expected error for invalid data")
	`)
	if err != nil {
		t.Errorf("flow narrowing pattern test failed: %v", err)
	}
}

// TestLTypeStringLibraryMethods tests that LTypeString supports string library methods.
// When registered as "string", it should support both string(x) typecast and string.upper().
func TestLTypeStringLibraryMethods(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenString(L)

	// Register LTypeString as "string" - replacing the library table
	L.SetGlobal("string", LTypeString)

	// Test typecast: string(x) should validate and return the value
	err := L.DoString(`
		local s = string("hello")
		assert(s == "hello", "string typecast should return the value")
	`)
	if err != nil {
		t.Errorf("string typecast failed: %v", err)
	}

	// Test library methods: string.upper, string.lower, etc.
	err = L.DoString(`
		local upper = string.upper("hello")
		assert(upper == "HELLO", "string.upper should work")

		local lower = string.lower("WORLD")
		assert(lower == "world", "string.lower should work")

		local len = string.len("test")
		assert(len == 4, "string.len should work")
	`)
	if err != nil {
		t.Errorf("string library methods failed: %v", err)
	}

	// Test both together
	err = L.DoString(`
		local x = string("hello")
		local result = string.upper(x)
		assert(result == "HELLO", "combined typecast and method should work")
	`)
	if err != nil {
		t.Errorf("combined typecast and method failed: %v", err)
	}
}

// TestLTypeStringTypecastError tests that string(x) raises error for non-strings.
func TestLTypeStringTypecastError(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenString(L)

	L.SetGlobal("string", LTypeString)

	// Calling string(123) should fail
	err := L.DoString(`string(123)`)
	if err == nil {
		t.Error("expected error when typecasting number to string")
	}
}

// TestLTypeCallAsLastArg tests that type validation calls used as the last
// argument to a function pass exactly 1 result, not stale register values.
func TestLTypeCallAsLastArg(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenString(L)

	L.SetGlobal("string", LTypeString)

	// Register a Go function that reports its argument count
	L.SetGlobal("argcount", L.NewFunction(func(l *LState) int {
		l.Push(LNumber(l.GetTop()))
		return 1
	}))

	// Register a Go function that checks argument count is exactly expected
	L.SetGlobal("check_args", L.NewFunction(func(l *LState) int {
		expected := l.CheckInt(1)
		val := l.CheckString(2)
		got := l.GetTop()
		if got != expected {
			l.RaiseError("expected %d args, got %d", expected, got)
		}
		l.Push(LString(val))
		return 1
	}))

	// string(x) as last argument should pass exactly 2 args to check_args
	err := L.DoString(`
		local result = check_args(2, string("hello"))
		assert(result == "hello", "value should pass through")
	`)
	if err != nil {
		t.Errorf("typeCall as last arg passed wrong number of args: %v", err)
	}

	// argcount with string(x) as the only argument should see 1 arg
	err = L.DoString(`
		local n = argcount(string("test"))
		assert(n == 1, "argcount should be 1, got " .. tostring(n))
	`)
	if err != nil {
		t.Errorf("typeCall as sole last arg: %v", err)
	}

	// Verify with a non-last-arg position (should always work)
	err = L.DoString(`
		local n = argcount(string("a"), "b")
		assert(n == 2, "argcount should be 2, got " .. tostring(n))
	`)
	if err != nil {
		t.Errorf("typeCall as non-last arg: %v", err)
	}
}

func TestLTypeInterfaceTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	tableType := &LType{inner: typ.NewInterface("table", nil)}

	plainTable := L.NewTable()
	plainTable.RawSetString("key", LString("value"))

	tableWithArray := L.NewTable()
	tableWithArray.RawSetString("tags", L.NewTable())
	tableWithArray.RawGet(LString("tags")).(*LTable).Append(LString("a"))
	tableWithArray.RawGet(LString("tags")).(*LTable).Append(LString("b"))

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"table accepts plain table", plainTable, true},
		{"table accepts table with nested arrays", tableWithArray, true},
		{"table accepts empty table", L.NewTable(), true},
		{"table rejects string", LString("hello"), false},
		{"table rejects number", LNumber(42), false},
		{"table rejects nil", LNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tableType.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLTypeRecordWithTableField(t *testing.T) {
	L := NewState()
	defer L.Close()

	// {kind: string, data: table}
	commandType := &LType{
		inner: typ.NewRecord().
			Field("kind", typ.String).
			Field("data", typ.NewInterface("table", nil)).
			Build(),
	}

	valid := L.NewTable()
	valid.RawSetString("kind", LString("create"))
	data := L.NewTable()
	data.RawSetString("name", LString("test"))
	tags := L.NewTable()
	tags.Append(LString("a"))
	tags.Append(LString("b"))
	data.RawSetString("tags", tags)
	valid.RawSetString("data", data)

	invalid := L.NewTable()
	invalid.RawSetString("kind", LString("create"))
	invalid.RawSetString("data", LString("not a table"))

	tests := []struct {
		name     string
		value    LValue
		expected bool
	}{
		{"record with table field accepts nested arrays", valid, true},
		{"record with table field rejects non-table data", invalid, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := commandType.Validate(L, tt.value)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

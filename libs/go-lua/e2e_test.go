package lua

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// TestE2E_ParseTypeCheckCompileRun tests the full pipeline:
// parse → typecheck → compile → execute.
func TestE2E_ParseTypeCheckCompileRun(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		typeErrors  int
		runtimeFail bool
		expected    LValue
	}{
		{
			name: "typed function returns correct type",
			code: `
				local function add(a: number, b: number): number
					return a + b
				end
				return add(10, 20)
			`,
			expected: LNumber(30),
		},
		{
			name: "integer arithmetic preserves type",
			code: `
				local x: integer = 5
				local y: integer = 3
				return x + y
			`,
			expected: LNumber(8),
		},
		{
			name: "string concat",
			code: `
				local s: string = "hello"
				local t: string = " world"
				return s .. t
			`,
			expected: LString("hello world"),
		},
		{
			name: "table with typed fields",
			code: `
				type Point = {x: number, y: number}
				local p: Point = {x = 10.0, y = 20.0}
				return p.x + p.y
			`,
			expected: LNumber(30),
		},
		{
			name: "array access",
			code: `
				local arr: {number} = {1.0, 2.0, 3.0, 4.0, 5.0}
				local sum = 0.0
				for i = 1, 5 do
					sum = sum + (arr[i] or 0.0)
				end
				return sum
			`,
			expected: LNumber(15),
		},
		{
			name: "optional nil handling",
			code: `
				local function find(arr: {number}, val: number): number?
					for i = 1, #arr do
						if arr[i] == val then return i end
					end
					return nil
				end
				local idx = find({1.0, 2.0, 3.0}, 2)
				return idx or -1
			`,
			expected: LNumber(2),
		},
		{
			name: "union type narrowing",
			code: `
				local function process(x: number | string): string
					if type(x) == "string" then
						return x
					end
					return tostring(x)
				end
				return process(42)
			`,
			expected: LString("42"),
		},
		{
			name: "generic function pattern",
			code: `
				local function identity(x: any): any
					return x
				end
				return identity("test")
			`,
			expected: LString("test"),
		},
		{
			name: "method definition",
			code: `
				local Counter = {}
				function Counter:new()
					local obj = {count = 0}
					setmetatable(obj, {__index = Counter})
					return obj
				end
				function Counter:inc()
					self.count = self.count + 1
				end
				function Counter:get()
					return self.count
				end
				local c = Counter:new()
				c:inc()
				c:inc()
				return c:get()
			`,
			typeErrors: 0, // type system now handles metatable OOP patterns
			expected:   LNumber(2),
		},
		{
			name: "type error detected",
			code: `
				local x: number = "not a number"
				return x
			`,
			typeErrors: 1,
			expected:   LString("not a number"), // still runs
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Phase 1: Type check
			stmts, err := parse.ParseString(tc.code, tc.name)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			diagnostics := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign()).CheckChunk(stmts, tc.name).Diagnostics
			if len(diagnostics) != tc.typeErrors {
				t.Errorf("expected %d type errors, got %d:", tc.typeErrors, len(diagnostics))
				for _, d := range diagnostics {
					t.Errorf("  %s", d.Message)
				}
			}

			// Phase 2: Execute
			L := NewState()
			defer L.Close()
			OpenBase(L)
			OpenString(L)
			OpenTable(L)
			OpenMath(L)

			if err := L.DoString(tc.code); err != nil {
				if !tc.runtimeFail {
					t.Fatalf("runtime error: %v", err)
				}
				return
			}

			// Phase 3: Check result
			if tc.expected != nil {
				result := L.Get(-1)
				if tc.expected.Type() == LTNumber && result.Type() == LTNumber {
					if float64(tc.expected.(LNumber)) != float64(result.(LNumber)) {
						t.Errorf("expected %v, got %v", tc.expected, result)
					}
				} else if tc.expected.String() != result.String() {
					t.Errorf("expected %v, got %v", tc.expected, result)
				}
			}
		})
	}
}

// TestE2E_RuntimeTypeValidation tests LType runtime validation.
func TestE2E_RuntimeTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// Register types as globals
	L.SetGlobal("NumberType", LTypeNumber)
	L.SetGlobal("StringType", LTypeString)
	L.SetGlobal("IntegerType", LTypeInteger)
	L.SetGlobal("BooleanType", LTypeBoolean)
	L.SetGlobal("AnyType", LTypeAny)

	tests := []struct {
		name        string
		code        string
		shouldMatch bool
	}{
		{
			name:        "NumberType:is(number) matches",
			code:        `local val, err = NumberType:is(42); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "NumberType:is(string) fails",
			code:        `local val, err = NumberType:is("hello"); return val == nil, err`,
			shouldMatch: true,
		},
		{
			name:        "StringType:is(string) matches",
			code:        `local val, err = StringType:is("hello"); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "BooleanType:is(boolean) matches",
			code:        `local val, err = BooleanType:is(true); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "AnyType:is(anything) matches",
			code:        `local v1 = (AnyType:is("anything")); local v2 = (AnyType:is(123)); local v3 = (AnyType:is({})); return v1 ~= nil and v2 ~= nil and v3 ~= nil, nil`,
			shouldMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-2)
			L.Pop(2)
			if result != LTrue {
				t.Errorf("test condition failed")
			}
		})
	}

	// Test kind method separately
	if err := L.DoString(`return NumberType:kind()`); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	result := L.Get(-1)
	L.Pop(1)
	if result.String() != "number" {
		t.Errorf("expected 'number', got %v", result)
	}
}

// TestE2E_RuntimeTypeCall tests Type(value) validation.
func TestE2E_RuntimeTypeCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	L.SetGlobal("NumberType", LTypeNumber)
	L.SetGlobal("StringType", LTypeString)

	// Valid calls should pass through
	err := L.DoString(`local x = NumberType(42.0); return x`)
	if err != nil {
		t.Fatalf("valid type call failed: %v", err)
	}
	result := L.Get(-1)
	switch v := result.(type) {
	case LNumber:
		if v != 42 {
			t.Error("type call should return the validated value")
		}
	case LInteger:
		if v != 42 {
			t.Error("type call should return the validated value")
		}
	default:
		t.Errorf("unexpected type: %T", result)
	}
	L.Pop(1)

	// Invalid calls should raise error
	err = L.DoString(`local x = NumberType("hello")`)
	if err == nil {
		t.Fatal("expected type validation error")
	}
	if !strings.Contains(err.Error(), "expected") {
		t.Errorf("error should mention expected type: %v", err)
	}
}

// TestE2E_RuntimeTypeCall_Record tests Type(value) validation for record types.
func TestE2E_RuntimeTypeCall_Record(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	pointType := NewLType(typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build())
	L.SetGlobal("PointType", pointType)

	// Valid call should pass through and return the table
	err := L.DoString(`local p = PointType({x = 1, y = 2}); return p`)
	if err != nil {
		t.Fatalf("valid record type call failed: %v", err)
	}
	result := L.Get(-1)
	L.Pop(1)
	if result.Type() != LTTable {
		t.Fatalf("expected table result, got %v", result.Type())
	}
	if tbl, ok := result.(*LTable); ok {
		if xv := tbl.RawGetString("x"); !numberEquals(xv, 1) {
			t.Errorf("expected x=1, got %v", xv)
		}
		if yv := tbl.RawGetString("y"); !numberEquals(yv, 2) {
			t.Errorf("expected y=2, got %v", yv)
		}
	}

	// Invalid call should raise error
	err = L.DoString(`PointType({x = "bad"})`)
	if err == nil {
		t.Fatal("expected record type validation error")
	}
}

func numberEquals(v LValue, want LNumber) bool {
	switch n := v.(type) {
	case LNumber:
		return n == want
	case LInteger:
		return LNumber(n) == want
	default:
		return false
	}
}

// TestE2E_RecordTypeValidation tests record type validation at runtime.
func TestE2E_RecordTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// Create a Point type
	pointType := NewLType(typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build())
	L.SetGlobal("PointType", pointType)

	tests := []struct {
		name        string
		code        string
		shouldMatch bool
	}{
		{
			name:        "valid record passes",
			code:        `local val, err = PointType:is({x = 10, y = 20}); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "missing field fails",
			code:        `local val, err = PointType:is({x = 10}); return val == nil, err`,
			shouldMatch: true,
		},
		{
			name:        "wrong field type fails",
			code:        `local val, err = PointType:is({x = "not a number", y = 20}); return val == nil, err`,
			shouldMatch: true,
		},
		{
			name:        "non-table fails",
			code:        `local val, err = PointType:is(42); return val == nil, err`,
			shouldMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-2)
			L.Pop(2)
			if result != LTrue {
				t.Errorf("test condition failed")
			}
		})
	}
}

// TestE2E_ArrayTypeValidation tests array type validation.
func TestE2E_ArrayTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// Create {number} type
	numArrayType := NewLType(typ.NewArray(typ.Number))
	L.SetGlobal("NumArray", numArrayType)

	tests := []struct {
		name        string
		code        string
		shouldMatch bool
	}{
		{
			name:        "valid number array",
			code:        `local val, err = NumArray:is({1, 2, 3}); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "empty array valid",
			code:        `local val, err = NumArray:is({}); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "mixed types fail",
			code:        `local val, err = NumArray:is({1, "two", 3}); return val == nil, err`,
			shouldMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-2)
			L.Pop(2)
			if result != LTrue {
				t.Errorf("test condition failed")
			}
		})
	}
}

// TestE2E_UnionTypeValidation tests union type validation.
func TestE2E_UnionTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// Create number | string type
	unionType := NewLType(typ.NewUnion(typ.Number, typ.String))
	L.SetGlobal("NumOrString", unionType)

	tests := []struct {
		name        string
		code        string
		shouldMatch bool
	}{
		{
			name:        "number matches",
			code:        `local val, err = NumOrString:is(42); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "string matches",
			code:        `local val, err = NumOrString:is("hello"); return val ~= nil, err`,
			shouldMatch: true,
		},
		{
			name:        "boolean fails",
			code:        `local val, err = NumOrString:is(true); return val == nil, err`,
			shouldMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-2)
			L.Pop(2)
			if result != LTrue {
				t.Errorf("test condition failed")
			}
		})
	}
}

// TestE2E_OptionalTypeValidation tests optional type validation.
func TestE2E_OptionalTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// Create number? type
	optionalType := NewLType(typ.NewOptional(typ.Number))
	L.SetGlobal("OptNumber", optionalType)

	tests := []struct {
		name        string
		code        string
		shouldMatch bool
	}{
		{
			name:        "number matches",
			code:        `local val, err = OptNumber:is(42); return val ~= nil and err == nil`,
			shouldMatch: true,
		},
		{
			name:        "nil matches",
			code:        `local val, err = OptNumber:is(nil); return err == nil`,
			shouldMatch: true,
		},
		{
			name:        "string fails",
			code:        `local val, err = OptNumber:is("hello"); return val == nil and err ~= nil`,
			shouldMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-1)
			L.Pop(1)
			if result != LTrue {
				t.Errorf("test condition failed")
			}
		})
	}
}

// TestE2E_TypeIntrospection tests type introspection methods.
func TestE2E_TypeIntrospection(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// Array type for elem() test
	arrType := NewLType(typ.NewArray(typ.String))
	L.SetGlobal("StringArray", arrType)

	// Map type for key()/val() test
	mapType := NewLType(typ.NewMap(typ.String, typ.Number))
	L.SetGlobal("StrNumMap", mapType)

	// Optional type for inner() test
	optType := NewLType(typ.NewOptional(typ.Boolean))
	L.SetGlobal("OptBool", optType)

	// Function type for ret()/params() test
	fnType := NewLType(typ.Func().
		Param("a", typ.Number).
		Param("b", typ.String).
		Returns(typ.Boolean).
		Build())
	L.SetGlobal("MyFunc", fnType)

	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "array elem kind",
			code:     `return StringArray:elem():kind()`,
			expected: "string",
		},
		{
			name:     "map key kind",
			code:     `return StrNumMap:key():kind()`,
			expected: "string",
		},
		{
			name:     "map val kind",
			code:     `return StrNumMap:val():kind()`,
			expected: "number",
		},
		{
			name:     "optional inner kind",
			code:     `return OptBool:inner():kind()`,
			expected: "boolean",
		},
		{
			name:     "function ret kind",
			code:     `return MyFunc:ret():kind()`,
			expected: "boolean",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-1)
			L.Pop(1)
			if result.String() != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result.String())
			}
		})
	}
}

// TestE2E_RecordFieldIteration tests iterating over record fields.
func TestE2E_RecordFieldIteration(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenTable(L)

	personType := NewLType(typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build())
	L.SetGlobal("PersonType", personType)

	code := `
		local fields = {}
		for name, fieldType in PersonType:fields() do
			fields[name] = true
		end
		local count = 0
		for _ in pairs(fields) do count = count + 1 end
		if not fields.name then return "missing name" end
		if not fields.age then return "missing age" end
		return "ok:" .. count
	`

	if err := L.DoString(code); err != nil {
		t.Fatalf("runtime error: %v", err)
	}

	result := L.Get(-1).String()
	if result != "ok:2" {
		t.Errorf("expected 'ok:2', got '%s'", result)
	}
}

// TestE2E_TypecheckAndExecuteWithErrors verifies type errors don't prevent execution.
func TestE2E_TypecheckAndExecuteWithErrors(t *testing.T) {
	code := `
		local x: number = "not a number"  -- type error
		local y: string = 42              -- type error
		return "executed"
	`

	// Type check should find errors
	stmts, err := parse.ParseString(code, "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	diagnostics := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign()).CheckChunk(stmts, "test").Diagnostics
	if len(diagnostics) < 2 {
		t.Errorf("expected at least 2 type errors, got %d", len(diagnostics))
	}

	// But code should still execute
	L := NewState()
	defer L.Close()
	OpenBase(L)

	if err := L.DoString(code); err != nil {
		t.Fatalf("execution should succeed: %v", err)
	}

	result := L.Get(-1)
	if result.String() != "executed" {
		t.Errorf("expected 'executed', got '%s'", result.String())
	}
}

// TestE2E_CrossModuleTypes tests type checking across module boundaries.
func TestE2E_CrossModuleTypes(t *testing.T) {

	// Module A exports a Point type
	moduleA := `
		type Point = {x: number, y: number}
		local M = {}
		function M.origin(): Point
			return {x = 0, y = 0}
		end
		return M
	`

	// Module B uses Point type
	moduleB := `
		local A = require("moduleA")
		local p: moduleA.Point = A.origin()
		return p.x + p.y
	`

	// Check module A
	stmtsA, err := parse.ParseString(moduleA, "moduleA")
	if err != nil {
		t.Fatalf("parse error in moduleA: %v", err)
	}
	diagsA := check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign()).CheckChunk(stmtsA, "moduleA").Diagnostics
	if len(diagsA) > 0 {
		t.Errorf("moduleA type errors: %v", diagsA)
	}

	// TODO: Need manifest extraction and cross-module import support
	_ = moduleB
}

// TestE2E_LiteralTypes tests literal type validation.
func TestE2E_LiteralTypes(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// Create literal types
	okLiteral := NewLType(typ.LiteralString("ok"))
	L.SetGlobal("OkLiteral", okLiteral)

	num42 := NewLType(typ.LiteralInt(42))
	L.SetGlobal("Num42", num42)

	tests := []struct {
		name        string
		code        string
		shouldMatch bool
	}{
		{
			name:        "exact string literal matches",
			code:        `local val, err = OkLiteral:is("ok"); return val ~= nil and err == nil`,
			shouldMatch: true,
		},
		{
			name:        "different string fails",
			code:        `local val, err = OkLiteral:is("error"); return val == nil and err ~= nil`,
			shouldMatch: true,
		},
		{
			name:        "exact number literal matches",
			code:        `local val, err = Num42:is(42); return val ~= nil and err == nil`,
			shouldMatch: true,
		},
		{
			name:        "different number fails",
			code:        `local val, err = Num42:is(43); return val == nil and err ~= nil`,
			shouldMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-1)
			L.Pop(1)
			if result != LTrue {
				t.Errorf("test condition failed")
			}
		})
	}
}

// TestE2E_FunctionTypeValidation tests function type validation.
func TestE2E_FunctionTypeValidation(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	fnType := NewLType(typ.Func().
		Param("x", typ.Number).
		Returns(typ.String).
		Build())
	L.SetGlobal("FnType", fnType)

	// Functions should match function types
	err := L.DoString(`
		local function myFunc(x) return tostring(x) end
		local val, err = FnType:is(myFunc)
		return val ~= nil, err
	`)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	if L.Get(-2) != LTrue {
		t.Error("function should match function type")
	}
	L.Pop(2)

	// Non-functions should fail
	err = L.DoString(`local val, err = FnType:is(42); return val == nil, err`)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	if L.Get(-2) != LTrue {
		t.Error("number should not match function type")
	}
	L.Pop(2)
}

// TestE2E_ComplexNestedTypes tests deeply nested type validation.
func TestE2E_ComplexNestedTypes(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	// {items: {x: number, y: number}[]}
	pointType := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()
	pointsArrayType := typ.NewArray(pointType)
	containerType := NewLType(typ.NewRecord().
		Field("items", pointsArrayType).
		Build())
	L.SetGlobal("Container", containerType)

	tests := []struct {
		name        string
		code        string
		shouldMatch bool
	}{
		{
			name: "valid nested structure",
			code: `
				local val, err = Container:is({
					items = {
						{x = 1, y = 2},
						{x = 3, y = 4}
					}
				})
				return val ~= nil, err
			`,
			shouldMatch: true,
		},
		{
			name: "empty items valid",
			code: `
				local val, err = Container:is({items = {}})
				return val ~= nil, err
			`,
			shouldMatch: true,
		},
		{
			name: "invalid nested item fails",
			code: `
				local val, err = Container:is({
					items = {
						{x = 1, y = "wrong"}
					}
				})
				return val == nil, err
			`,
			shouldMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := L.DoString(tc.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			result := L.Get(-2)
			errMsg := L.Get(-1)
			L.Pop(2)
			if result != LTrue {
				t.Errorf("expected true, got %v (error: %v)", result, errMsg)
			}
		})
	}
}

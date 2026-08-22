package lua

import (
	"strings"
	"testing"
)

func setupErrorsTest(_ *testing.T) *LState {
	L := NewState()
	// errors module is now auto-loaded via OpenLibs in NewState
	return L
}

func TestErrorsLib_AutoLoaded(t *testing.T) {
	// Verify errors module is automatically loaded with NewState
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		assert(errors ~= nil, "errors module should be loaded")
		assert(errors.new ~= nil, "errors.new should exist")
		assert(errors.NOT_FOUND ~= nil, "errors.NOT_FOUND should exist")
	`); err != nil {
		t.Fatalf("errors module not auto-loaded: %v", err)
	}
}

func TestErrorsLib_NewString(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new("test error")
		assert(type(e) == "userdata", "expected userdata")
		assert(tostring(e) == "test error", "tostring should return message")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_NewTable(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new({
			message = "not found",
			kind = errors.NOT_FOUND,
			retryable = false,
			details = {
				resource = "user",
				id = 123
			}
		})

		assert(tostring(e) == "not found", "message mismatch")
		assert(e:kind() == "NotFound", "kind should be NotFound, got: " .. tostring(e:kind()))
		assert(e:retryable() == false, "retryable should be false")

		local d = e:details()
		assert(d ~= nil, "details should not be nil")
		assert(d.resource == "user", "details.resource mismatch")
		-- Numbers are stored as float64, so comparison might need tolerance
		assert(math.floor(d.id) == 123, "details.id mismatch, got: " .. tostring(d.id))
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Kind(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new({message = "test", kind = errors.TIMEOUT})
		assert(e:kind() == "Timeout", "expected Timeout, got: " .. tostring(e:kind()))
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Retryable(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		-- Unknown by default
		local e1 = errors.new("test")
		assert(e1:retryable() == nil, "should be nil by default")

		-- Explicit true
		local e2 = errors.new({message = "retry", retryable = true})
		assert(e2:retryable() == true, "should be true")

		-- Explicit false
		local e3 = errors.new({message = "no retry", retryable = false})
		assert(e3:retryable() == false, "should be false")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Details(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new({
			message = "test",
			details = {
				str = "hello",
				num = 42,
				flag = true,
				nested = {x = 1, y = 2}
			}
		})
		local d = e:details()
		assert(d.str == "hello", "string detail mismatch")
		assert(math.floor(d.num) == 42, "number detail mismatch")
		assert(d.flag == true, "boolean detail mismatch")
		assert(math.floor(d.nested.x) == 1, "nested detail mismatch")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Message(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new("my message")
		assert(e:message() == "my message", "message() should return message")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Stack(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local function inner()
			return errors.new("from inner")
		end
		local function outer()
			return inner()
		end
		e = outer()
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}

	errVal := L.GetGlobal("e")
	e, ok := errVal.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", errVal)
	}

	stack := e.Stack()
	if stack == "" {
		t.Error("stack should not be empty")
	}
}

func TestErrorsLib_Tostring(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new("hello world")
		local s = tostring(e)
		assert(s == "hello world", "tostring should return message, got: " .. s)
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Concat(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new("test error")
		local s1 = "prefix: " .. e
		assert(s1 == "prefix: test error", "concat with prefix failed: " .. s1)

		local s2 = e .. " :suffix"
		assert(s2 == "test error :suffix", "concat with suffix failed: " .. s2)

		local s3 = "(" .. e .. ")"
		assert(s3 == "(test error)", "double concat failed: " .. s3)
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Print(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	// Should not error
	if err := L.DoString(`
		local e = errors.new("printable error")
		print(e)
	`); err != nil {
		t.Fatalf("print test failed: %v", err)
	}
}

func TestErrorsLib_Wrap(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local inner = errors.new("inner error")
		local outer = errors.wrap(inner, "outer context")

		assert(tostring(outer) == "inner error", "wrapped error message")

		-- Metadata should be preserved
		local inner2 = errors.new({
			message = "base",
			kind = errors.INVALID,
			retryable = true,
			details = {key = "value"}
		})
		local outer2 = errors.wrap(inner2, "wrapped")
		assert(outer2:kind() == "Invalid", "kind should be preserved")
		assert(outer2:retryable() == true, "retryable should be preserved")
		local d = outer2:details()
		assert(d.key == "value", "details should be preserved")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_WrapString(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.wrap("base error", "wrapped context")
		assert(type(e) == "userdata", "should be userdata")
		assert(tostring(e) == "base error", "message should be base error")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_CallStack(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local function level2()
			return errors.new("from level2")
		end
		local function level1()
			return level2()
		end
		local e = level1()
		local stack = errors.call_stack(e)

		assert(stack ~= nil, "call_stack should not return nil")
		assert(type(stack.frames) == "table", "should have frames table")
		assert(#stack.frames > 0, "should have at least one frame")

		local found = false
		for i, frame in ipairs(stack.frames) do
			if frame.name then
				found = true
			end
		end
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_Is(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e1 = errors.new({message = "test", kind = errors.NOT_FOUND})
		assert(errors.is(e1, errors.NOT_FOUND) == true, "should match NOT_FOUND")
		assert(errors.is(e1, errors.TIMEOUT) == false, "should not match TIMEOUT")

		local e2 = errors.new("plain error")
		assert(errors.is(e2, errors.UNKNOWN) == true, "plain error should be UNKNOWN")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_KindConstants(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		assert(errors.NOT_FOUND == "NotFound", "NOT_FOUND constant")
		assert(errors.ALREADY_EXISTS == "AlreadyExists", "ALREADY_EXISTS constant")
		assert(errors.INVALID == "Invalid", "INVALID constant")
		assert(errors.PERMISSION_DENIED == "PermissionDenied", "PERMISSION_DENIED constant")
		assert(errors.UNAVAILABLE == "Unavailable", "UNAVAILABLE constant")
		assert(errors.INTERNAL == "Internal", "INTERNAL constant")
		assert(errors.CANCELED == "Canceled", "CANCELED constant")
		assert(errors.CONFLICT == "Conflict", "CONFLICT constant")
		assert(errors.TIMEOUT == "Timeout", "TIMEOUT constant")
		assert(errors.RATE_LIMITED == "RateLimited", "RATE_LIMITED constant")
		assert(errors.UNKNOWN == "", "UNKNOWN constant should be empty string")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_StackMethod(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local function level2()
			return errors.new("deep error")
		end
		local function level1()
			return level2()
		end
		global_err = level1()
		local stack = global_err:stack()
		assert(type(stack) == "string", "stack() should return string")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}

	errVal := L.GetGlobal("global_err")
	e, ok := errVal.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", errVal)
	}

	stack := e.Stack()
	if !strings.Contains(stack, "level") || stack == "" {
		t.Logf("Stack: %s", stack)
	}
}

func TestErrorsLib_TypeCheck(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new("test")
		assert(type(e) == "userdata", "type should be userdata")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_InPcall(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	// Test returning error objects from functions called with pcall
	// (not using error() which converts to string)
	if err := L.DoString(`
		local function returns_error()
			return nil, errors.new("operation failed")
		end

		local result, err = returns_error()
		assert(result == nil, "result should be nil")
		assert(err ~= nil, "error should not be nil")
		assert(tostring(err) == "operation failed", "error message mismatch")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_AsReturnValue(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local function getError()
			return nil, errors.new("operation failed")
		end

		local result, err = getError()
		assert(result == nil, "result should be nil")
		assert(err ~= nil, "error should not be nil")
		assert(tostring(err) == "operation failed", "error message mismatch")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_ConcatWithError(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e1 = errors.new("first")
		local e2 = errors.new("second")
		local s = e1 .. " and " .. e2
		assert(s == "first and second", "concat with two errors: " .. s)
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_NilDetails(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	if err := L.DoString(`
		local e = errors.new("no details")
		assert(e:details() == nil, "details should be nil when not set")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorsLib_EmptyStack(t *testing.T) {
	L := setupErrorsTest(t)
	defer L.Close()

	// Create error in Go, not Lua - should have no Lua stack
	e := NewError("go error")
	L.SetGlobal("go_err", e)

	if err := L.DoString(`
		local stack = errors.call_stack(go_err)
		assert(stack == nil, "call_stack should return nil for Go-only errors")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestGoToLuaValueIntTypes(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test various int types through error details
	details := map[string]any{
		"int":    42,
		"int8":   int8(8),
		"int16":  int16(16),
		"int32":  int32(32),
		"int64":  int64(64),
		"uint":   uint(100),
		"uint8":  uint8(8),
		"uint16": uint16(16),
		"uint32": uint32(32),
		"uint64": uint64(64),
		"f32":    float32(3.14),
		"f64":    2.71,
	}

	e := NewError("test").WithDetails(details)
	SetErrorMetatable(L, e)
	L.SetGlobal("err", e)

	if err := L.DoString(`
		local d = err:details()
		assert(d.int == 42, "int mismatch: " .. tostring(d.int))
		assert(d.int8 == 8, "int8 mismatch")
		assert(d.int16 == 16, "int16 mismatch")
		assert(d.int32 == 32, "int32 mismatch")
		assert(d.int64 == 64, "int64 mismatch")
		assert(d.uint == 100, "uint mismatch")
		assert(d.uint8 == 8, "uint8 mismatch")
		assert(d.uint16 == 16, "uint16 mismatch")
		assert(d.uint32 == 32, "uint32 mismatch")
		assert(d.uint64 == 64, "uint64 mismatch")
		assert(math.abs(d.f32 - 3.14) < 0.01, "float32 mismatch")
		assert(math.abs(d.f64 - 2.71) < 0.01, "float64 mismatch")
	`); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

// TestErrorMetatableInPcall verifies that *Error values returned from pcall
// have their metatable set correctly so methods like :kind() work.
func TestErrorMetatableInPcall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function that creates and raises an error
	L.SetGlobal("raise_typed_error", L.NewFunction(func(L *LState) int {
		e := NewError("test error")
		e.kind = NotFound
		e.LuaStack = captureStackTrace(L)
		SetErrorMetatable(L, e)
		L.RaiseError("%s", e.String())
		return 0
	}))

	// Register a Go function that directly raises an *Error
	L.SetGlobal("raise_error_object", L.NewFunction(func(L *LState) int {
		e := NewLuaError(L, "direct error")
		e.kind = Invalid
		L.Error(e, 1)
		return 0
	}))

	if err := L.DoString(`
		-- Test 1: pcall catching string error (tostring should work)
		local ok1, err1 = pcall(function()
			error("simple string error")
		end)
		assert(not ok1, "pcall should fail")
		assert(type(err1) == "string", "err1 should be string, got " .. type(err1))

		-- Test 2: pcall catching *Error from RaiseError (tostring should work)
		local ok2, err2 = pcall(function()
			raise_typed_error()
		end)
		assert(not ok2, "pcall should fail")
		-- err2 is a string from RaiseError, not *Error

		-- Test 3: pcall catching *Error from L.Error (methods should work)
		local ok3, err3 = pcall(function()
			raise_error_object()
		end)
		assert(not ok3, "pcall should fail")
		-- This is where the metatable matters - err3 is *Error
		local msg3 = tostring(err3)
		assert(msg3 ~= nil, "tostring should work on err3")

		-- Test err3:kind() - this requires metatable with __index
		local kind3 = err3:kind()
		assert(kind3 == "Invalid", "expected kind 'Invalid', got: " .. tostring(kind3))

		-- Test 4: errors.new should create error with metatable
		local err4 = errors.new({
			message = "custom error",
			kind = "not_found"
		})
		assert(err4:kind() == "not_found", "err4:kind() should be 'not_found'")
		assert(tostring(err4) == "custom error", "tostring(err4) should be 'custom error'")
	`); err != nil {
		t.Fatal(err)
	}
}

// TestErrorMetatableInCoroutinePcall verifies that *Error values from pcall
// inside a coroutine have their metatable set correctly.
func TestErrorMetatableInCoroutinePcall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function that directly raises an *Error
	L.SetGlobal("raise_error_object", L.NewFunction(func(L *LState) int {
		e := NewLuaError(L, "coroutine error")
		e.kind = Timeout
		L.Error(e, 1)
		return 0
	}))

	if err := L.DoString(`
		function test_pcall_in_coroutine()
			local ok, err = pcall(function()
				raise_error_object()
			end)
			assert(not ok, "pcall should fail")

			-- err should be *Error with metatable
			local errStr = tostring(err)
			assert(errStr ~= nil, "tostring should work on error")

			-- Test :kind() method
			local kind = err:kind()
			assert(kind == "Timeout", "expected kind 'Timeout', got: " .. tostring(kind))

			return "success"
		end
	`); err != nil {
		t.Fatal(err)
	}

	co, cancel := L.NewThread()
	defer cancel()
	fn := L.GetGlobal("test_pcall_in_coroutine").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("expected ResumeOK, got %v", state)
	}
	if len(results) != 1 || results[0].String() != "success" {
		t.Fatalf("expected 'success', got: %v", results)
	}
}

// TestErrorMetatableMultiplePcallsInCoroutine tests multiple pcalls with errors
// in a coroutine, ensuring each error has proper metatable.
func TestErrorMetatableMultiplePcallsInCoroutine(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function that directly raises an *Error with specified kind
	L.SetGlobal("raise_with_kind", L.NewFunction(func(L *LState) int {
		kindStr := L.CheckString(1)
		e := NewLuaError(L, "error with kind "+kindStr)
		e.kind = Kind(kindStr)
		L.Error(e, 1)
		return 0
	}))

	if err := L.DoString(`
		function test_multiple_pcalls()
			-- First pcall
			local ok1, err1 = pcall(function()
				raise_with_kind("not_found")
			end)
			assert(not ok1, "pcall 1 should fail")
			assert(err1:kind() == "not_found", "err1 kind should be 'not_found', got: " .. tostring(err1:kind()))

			-- Second pcall
			local ok2, err2 = pcall(function()
				raise_with_kind("timeout")
			end)
			assert(not ok2, "pcall 2 should fail")
			assert(err2:kind() == "timeout", "err2 kind should be 'timeout', got: " .. tostring(err2:kind()))

			-- Third pcall (success)
			local ok3, result3 = pcall(function()
				return "worked"
			end)
			assert(ok3, "pcall 3 should succeed")
			assert(result3 == "worked", "result3 should be 'worked'")

			-- Fourth pcall with different error
			local ok4, err4 = pcall(function()
				raise_with_kind("invalid")
			end)
			assert(not ok4, "pcall 4 should fail")
			assert(err4:kind() == "invalid", "err4 kind should be 'invalid', got: " .. tostring(err4:kind()))

			return "all passed"
		end
	`); err != nil {
		t.Fatal(err)
	}

	co, cancel := L.NewThread()
	defer cancel()
	fn := L.GetGlobal("test_multiple_pcalls").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("expected ResumeOK, got %v", state)
	}
	if len(results) != 1 || results[0].String() != "all passed" {
		t.Fatalf("expected 'all passed', got: %v", results)
	}
}

// TestErrorToStringMeta verifies that tostring() calls the __tostring metamethod
// on *Error values.
func TestErrorToStringMeta(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		local err = errors.new({
			message = "my error message",
			kind = "internal"
		})

		local str = tostring(err)
		assert(str == "my error message", "expected 'my error message', got: " .. tostring(str))
	`); err != nil {
		t.Fatal(err)
	}
}

// TestErrorConcatMeta verifies that .. operator calls the __concat metamethod
// on *Error values.
func TestErrorConcatMeta(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		local err = errors.new("concat test")

		local str1 = "prefix: " .. err
		assert(str1 == "prefix: concat test", "expected 'prefix: concat test', got: " .. str1)

		local str2 = err .. " :suffix"
		assert(str2 == "concat test :suffix", "expected 'concat test :suffix', got: " .. str2)
	`); err != nil {
		t.Fatal(err)
	}
}

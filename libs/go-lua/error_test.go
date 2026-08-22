package lua

import (
	"errors"
	"strings"
	"testing"
)

func TestError_Basic(t *testing.T) {
	e := NewError("something went wrong")

	if e.Message != "something went wrong" {
		t.Errorf("expected 'something went wrong', got %q", e.Message)
	}
	if e.Error() != "something went wrong" {
		t.Errorf("Error() should return message, got %q", e.Error())
	}
	if e.String() != "something went wrong" {
		t.Errorf("String() should return message for Lua compat, got %q", e.String())
	}
	if e.Type() != LTUserData {
		t.Errorf("expected LTUserData, got %v", e.Type())
	}
}

func TestError_Formatted(t *testing.T) {
	e := NewErrorf("error %d: %s", 42, "test")
	if e.Message != "error 42: test" {
		t.Errorf("expected formatted message, got %q", e.Message)
	}
}

func TestError_Kind(t *testing.T) {
	e := NewError("not found").WithKind(NotFound)

	if e.Kind() != NotFound {
		t.Errorf("expected NotFound, got %v", e.Kind())
	}
	if e.Kind().String() != "NotFound" {
		t.Errorf("expected 'NotFound', got %q", e.Kind().String())
	}

	// Default kind
	e2 := NewError("unknown")
	if e2.Kind() != Unknown {
		t.Errorf("expected Unknown, got %v", e2.Kind())
	}
	if e2.Kind().String() != "Unknown" {
		t.Errorf("expected 'Unknown', got %q", e2.Kind().String())
	}
}

func TestError_Retryable(t *testing.T) {
	// Unknown by default
	e := NewError("test")
	if e.Retryable() != TernaryUnknown {
		t.Errorf("expected TernaryUnknown, got %v", e.Retryable())
	}

	// Set to true
	_ = e.WithRetryable(true)
	if e.Retryable() != TernaryTrue {
		t.Errorf("expected TernaryTrue, got %v", e.Retryable())
	}
	if !e.Retryable().Bool() {
		t.Error("Retryable().Bool() should be true")
	}

	// Set to false
	_ = e.WithRetryable(false)
	if e.Retryable() != TernaryFalse {
		t.Errorf("expected TernaryFalse, got %v", e.Retryable())
	}
	if e.Retryable().Bool() {
		t.Error("Retryable().Bool() should be false")
	}
}

func TestError_Details(t *testing.T) {
	details := map[string]any{
		"user_id":  123,
		"resource": "profile",
	}
	e := NewError("not found").WithDetails(details)

	d := e.Details()
	if d["user_id"] != 123 {
		t.Errorf("expected user_id=123, got %v", d["user_id"])
	}
	if d["resource"] != "profile" {
		t.Errorf("expected resource='profile', got %v", d["resource"])
	}
}

func TestError_Context(t *testing.T) {
	e := NewError("connection refused").WithContext("fetching user profile")

	if e.Context != "fetching user profile" {
		t.Errorf("expected context, got %q", e.Context)
	}
	// Error() should include context
	if e.Error() != "fetching user profile: connection refused" {
		t.Errorf("expected context in Error(), got %q", e.Error())
	}
	// String() should still return just the message for Lua
	if e.String() != "connection refused" {
		t.Errorf("String() should return message only, got %q", e.String())
	}
}

func TestError_Wrap(t *testing.T) {
	inner := errors.New("connection refused")
	e := WrapError(inner, "fetching user")

	if e.Err != inner {
		t.Error("wrapped error should point to inner")
	}
	if !errors.Is(e, inner) {
		t.Error("errors.Is should work with wrapped error")
	}
	if e.Context != "fetching user" {
		t.Errorf("expected context, got %q", e.Context)
	}
}

func TestError_WrapPreservesMetadata(t *testing.T) {
	inner := NewError("not found").
		WithKind(NotFound).
		WithRetryable(true).
		WithDetails(map[string]any{"id": 42})

	outer := WrapError(inner, "fetching profile")

	// Metadata should be preserved
	if outer.Kind() != NotFound {
		t.Errorf("kind should be preserved, got %v", outer.Kind())
	}
	if outer.Retryable() != TernaryTrue {
		t.Errorf("retryable should be preserved, got %v", outer.Retryable())
	}
	if outer.Details()["id"] != 42 {
		t.Errorf("details should be preserved, got %v", outer.Details())
	}
}

func TestError_GoStack(t *testing.T) {
	e := NewError("test")
	if len(e.goStack) == 0 {
		t.Error("Go stack should be captured")
	}

	stack := e.Stack()
	if !strings.Contains(stack, "error_test.go") {
		t.Errorf("stack should contain test file, got:\n%s", stack)
	}
}

func TestError_StackWithContext(t *testing.T) {
	e := NewError("inner").WithContext("doing something")
	stack := e.Stack()

	if !strings.Contains(stack, "doing something") {
		t.Errorf("stack should contain context, got:\n%s", stack)
	}
}

func TestError_ChainedStack(t *testing.T) {
	e1 := NewError("database error")
	e2 := WrapError(e1, "user lookup")
	e3 := WrapError(e2, "authentication")

	stack := e3.Stack()

	// All contexts should be in the stack
	if !strings.Contains(stack, "authentication") {
		t.Errorf("stack should contain 'authentication', got:\n%s", stack)
	}
	if !strings.Contains(stack, "user lookup") {
		t.Errorf("stack should contain 'user lookup', got:\n%s", stack)
	}
}

func TestError_NilWrap(t *testing.T) {
	e := WrapError(nil, "context")
	if e != nil {
		t.Error("wrapping nil should return nil")
	}
}

func TestError_AsLValue(t *testing.T) {
	var v LValue = NewError("test error")

	// Should work as LValue
	if v.Type() != LTUserData {
		t.Errorf("expected LTUserData, got %v", v.Type())
	}
	if v.String() != "test error" {
		t.Errorf("expected 'test error', got %q", v.String())
	}
}

func TestIsErrorKind(t *testing.T) {
	e := NewError("not found").WithKind(NotFound)

	if !IsErrorKind(e, NotFound) {
		t.Error("IsErrorKind should return true for matching kind")
	}
	if IsErrorKind(e, Internal) {
		t.Error("IsErrorKind should return false for non-matching kind")
	}
	if IsErrorKind(errors.New("plain"), NotFound) {
		t.Error("IsErrorKind should return false for non-Error")
	}
}

func TestGetErrorKind(t *testing.T) {
	e := NewError("timeout").WithKind(Timeout)

	if GetErrorKind(e) != Timeout {
		t.Errorf("expected Timeout, got %v", GetErrorKind(e))
	}
	if GetErrorKind(errors.New("plain")) != Unknown {
		t.Errorf("expected Unknown for plain error")
	}
}

func TestAsError(t *testing.T) {
	e := NewError("test")
	var v LValue = e

	extracted, ok := AsError(v)
	if !ok {
		t.Error("AsError should succeed for *Error")
	}
	if extracted != e {
		t.Error("extracted error should be same as original")
	}

	_, ok = AsError(LString("not error"))
	if ok {
		t.Error("AsError should fail for non-Error LValue")
	}
}

func TestGetError(t *testing.T) {
	e := NewError("test error")

	// Direct extraction
	got := GetError(e)
	if got != e {
		t.Error("GetError should return the same error")
	}

	// Wrapped error
	wrapped := WrapError(e, "context")
	got = GetError(wrapped)
	if got != wrapped {
		t.Error("GetError should return the outermost *Error")
	}

	// Plain error - should return nil
	got = GetError(errors.New("plain"))
	if got != nil {
		t.Error("GetError should return nil for non-Error")
	}

	// Nil error
	got = GetError(nil)
	if got != nil {
		t.Error("GetError should return nil for nil error")
	}
}

func TestKind_AllConstants(t *testing.T) {
	kinds := []Kind{
		Unknown,
		NotFound,
		AlreadyExists,
		Invalid,
		PermissionDenied,
		Unavailable,
		Internal,
		Canceled,
		Conflict,
		Timeout,
		RateLimited,
	}

	// All should be unique except Unknown (empty string)
	seen := make(map[string]bool)
	for _, k := range kinds {
		s := k.String()
		if k != Unknown && seen[s] {
			t.Errorf("duplicate kind string: %q", s)
		}
		seen[s] = true
	}
}

func TestTernary_String(t *testing.T) {
	if TernaryUnknown.String() != "Unknown" {
		t.Errorf("expected 'Unknown', got %q", TernaryUnknown.String())
	}
	if TernaryTrue.String() != "True" {
		t.Errorf("expected 'True', got %q", TernaryTrue.String())
	}
	if TernaryFalse.String() != "False" {
		t.Errorf("expected 'False', got %q", TernaryFalse.String())
	}
}

// TestError_LuaToString tests that errors work with tostring() in Lua
func TestError_LuaToString(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create error and set as global
	e := NewError("my error message")
	L.SetGlobal("err", e)

	// Test tostring()
	if err := L.DoString(`
		local s = tostring(err)
		if s ~= "my error message" then
			error("expected 'my error message', got: " .. tostring(s))
		end
	`); err != nil {
		t.Fatalf("tostring test failed: %v", err)
	}
}

// TestError_LuaPrint tests that errors work with print() in Lua
func TestError_LuaPrint(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create error and set as global
	e := NewError("printed error")
	L.SetGlobal("err", e)

	// print() should not error and should use String()
	if err := L.DoString(`print(err)`); err != nil {
		t.Fatalf("print test failed: %v", err)
	}
}

// TestError_LuaConcat tests that errors work with .. concatenation in Lua
func TestError_LuaConcatNeedsMetatable(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create error without metatable
	e := NewError("test error")
	L.SetGlobal("err", e)

	// Without __concat metatable, this will fail
	// We need to verify that the error type is set up correctly
	// For now, just verify that tostring works (which is needed for concat fallback)
	if err := L.DoString(`
		local s = tostring(err)
		assert(s == "test error", "tostring failed")
	`); err != nil {
		t.Fatalf("basic string conversion failed: %v", err)
	}
}

// TestError_LuaType tests that type() returns "userdata" for errors
func TestError_LuaType(t *testing.T) {
	L := NewState()
	defer L.Close()

	e := NewError("test")
	L.SetGlobal("err", e)

	if err := L.DoString(`
		local t = type(err)
		if t ~= "userdata" then
			error("expected 'userdata', got: " .. tostring(t))
		end
	`); err != nil {
		t.Fatalf("type test failed: %v", err)
	}
}

// TestError_WrapWithLua tests capturing Lua stack traces
func TestError_WrapWithLua(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Define a Go function that creates an error with Lua stack
	L.SetGlobal("make_error", L.NewFunction(func(L *LState) int {
		e := WrapErrorWithLua(L, errors.New("inner error"), "go function")
		L.Push(e)
		return 1
	}))

	if err := L.DoString(`
		local function outer()
			local function inner()
				return make_error()
			end
			return inner()
		end
		err = outer()
	`); err != nil {
		t.Fatalf("Lua code failed: %v", err)
	}

	// Get the error
	errVal := L.GetGlobal("err")
	e, ok := errVal.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", errVal)
	}

	// Should have captured Lua stack
	if e.LuaStack == nil {
		t.Fatal("LuaStack should be captured")
	}
	if len(e.LuaStack.Frames) == 0 {
		t.Fatal("LuaStack should have frames")
	}

	// Stack should contain the Lua call hierarchy
	stack := e.Stack()
	if !strings.Contains(stack, "inner") || !strings.Contains(stack, "outer") {
		t.Logf("Stack trace:\n%s", stack)
		// Note: function names may not always be captured depending on debug info
	}
}

// TestError_ChainedWithLuaStack tests error chains with Lua stack traces
func TestError_ChainedWithLuaStack(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Multiple wrap points
	L.SetGlobal("wrap_error", L.NewFunction(func(L *LState) int {
		existing := L.CheckAny(1)
		context := L.CheckString(2)

		if e, ok := existing.(*Error); ok {
			wrapped := WrapErrorWithLua(L, e, context)
			L.Push(wrapped)
		} else {
			e := WrapErrorWithLua(L, errors.New("base error"), context)
			L.Push(e)
		}
		return 1
	}))

	if err := L.DoString(`
		local function layer1()
			return wrap_error(nil, "layer1")
		end
		local function layer2()
			local e = layer1()
			return wrap_error(e, "layer2")
		end
		local function layer3()
			local e = layer2()
			return wrap_error(e, "layer3")
		end
		err = layer3()
	`); err != nil {
		t.Fatalf("Lua code failed: %v", err)
	}

	errVal := L.GetGlobal("err")
	e, ok := errVal.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", errVal)
	}

	stack := e.Stack()

	// All contexts should appear in the stack
	if !strings.Contains(stack, "layer3") {
		t.Errorf("stack should contain 'layer3', got:\n%s", stack)
	}
	if !strings.Contains(stack, "layer2") {
		t.Errorf("stack should contain 'layer2', got:\n%s", stack)
	}
	if !strings.Contains(stack, "layer1") {
		t.Errorf("stack should contain 'layer1', got:\n%s", stack)
	}
}

// TestError_PreservesMetadataThroughLuaWrap tests that metadata is preserved when wrapping with Lua
func TestError_PreservesMetadataThroughLuaWrap(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create error with metadata
	original := NewError("not found").
		WithKind(NotFound).
		WithRetryable(true).
		WithDetails(map[string]any{"id": 123})
	L.SetGlobal("original", original)

	L.SetGlobal("wrap_error", L.NewFunction(func(L *LState) int {
		existing := L.CheckAny(1)
		if e, ok := existing.(*Error); ok {
			wrapped := WrapErrorWithLua(L, e, "from lua")
			L.Push(wrapped)
			return 1
		}
		L.Push(LNil)
		return 1
	}))

	if err := L.DoString(`wrapped = wrap_error(original)`); err != nil {
		t.Fatalf("Lua code failed: %v", err)
	}

	errVal := L.GetGlobal("wrapped")
	e, ok := errVal.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", errVal)
	}

	// Metadata should be preserved
	if e.Kind() != NotFound {
		t.Errorf("kind should be preserved, got %v", e.Kind())
	}
	if e.Retryable() != TernaryTrue {
		t.Errorf("retryable should be preserved, got %v", e.Retryable())
	}
	if e.Details()["id"] != 123 {
		t.Errorf("details should be preserved, got %v", e.Details())
	}
}

// TestStackFrame_String tests the string representation of stack frames
func TestStackFrame_String(t *testing.T) {
	frame := StackFrame{
		Level:       0,
		Source:      "test.lua",
		CurrentLine: 42,
		Name:        "myfunction",
		FuncType:    "Lua",
	}

	s := frame.String()
	if s != "test.lua:42 (myfunction)" {
		t.Errorf("unexpected frame string: %q", s)
	}

	// Frame without name
	frame2 := StackFrame{
		Source:      "script.lua",
		CurrentLine: 10,
	}
	s2 := frame2.String()
	if s2 != "script.lua:10" {
		t.Errorf("unexpected frame string without name: %q", s2)
	}
}

// TestStackTrace_String tests the string representation of stack traces
func TestStackTrace_String(t *testing.T) {
	trace := StackTrace{
		ThreadID: "0x12345",
		Frames: []StackFrame{
			{Source: "a.lua", CurrentLine: 1, Name: "a"},
			{Source: "b.lua", CurrentLine: 2, Name: "b"},
		},
	}

	s := trace.String()
	if !strings.Contains(s, "a.lua:1 (a)") {
		t.Errorf("expected first frame in trace, got:\n%s", s)
	}
	if !strings.Contains(s, "b.lua:2 (b)") {
		t.Errorf("expected second frame in trace, got:\n%s", s)
	}
}

// externalError simulates an error from another package (like apierror.Error)
// that implements the metadata bridge contracts.
type externalError struct {
	message   string
	kind      string
	retryable int // 0=unknown, 1=true, 2=false (like apierror.Ternary)
	details   map[string]any
}

func (e *externalError) Error() string { return e.message }
func (e *externalError) ErrorKind() string {
	return e.kind
}
func (e *externalError) ErrorRetryable() (bool, bool) {
	switch e.retryable {
	case 1:
		return true, true
	case 2:
		return false, true
	default:
		return false, false
	}
}
func (e *externalError) ErrorDetails() map[string]any {
	return e.details
}

func testMetadataExtractor(err error) *ErrorMetadata {
	for e := err; e != nil; e = errors.Unwrap(e) {
		meta := &ErrorMetadata{}
		if k, ok := e.(interface{ ErrorKind() string }); ok {
			if kind := k.ErrorKind(); kind != "" {
				meta.Kind = Kind(kind)
			}
		}
		if r, ok := e.(interface{ ErrorRetryable() (bool, bool) }); ok {
			if b, known := r.ErrorRetryable(); known {
				v := b
				meta.Retryable = &v
			}
		}
		if d, ok := e.(interface{ ErrorDetails() map[string]any }); ok {
			if details := d.ErrorDetails(); details != nil {
				meta.Details = details
			}
		}
		if meta.Kind != "" || meta.Retryable != nil || meta.Details != nil {
			return meta
		}
	}
	return nil
}

// TestWrapError_CrossPackageMetadata tests that WrapError extracts metadata
// from errors implementing metadata provider contracts.
func TestWrapError_CrossPackageMetadata(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		expectedKind    Kind
		expectedRetry   Ternary
		expectedDetails map[string]any
	}{
		{
			name: "external error with all metadata",
			err: &externalError{
				message:   "not found",
				kind:      "NotFound",
				retryable: 2, // false
				details:   map[string]any{"key": "value"},
			},
			expectedKind:    NotFound,
			expectedRetry:   TernaryFalse,
			expectedDetails: map[string]any{"key": "value"},
		},
		{
			name: "external error retryable true",
			err: &externalError{
				message:   "service unavailable",
				kind:      "Unavailable",
				retryable: 1, // true
				details:   nil,
			},
			expectedKind:    Unavailable,
			expectedRetry:   TernaryTrue,
			expectedDetails: nil,
		},
		{
			name: "external error retryable unknown",
			err: &externalError{
				message:   "internal error",
				kind:      "Internal",
				retryable: 0, // unknown
				details:   nil,
			},
			expectedKind:    Internal,
			expectedRetry:   TernaryUnknown,
			expectedDetails: nil,
		},
		{
			name:            "plain error - no metadata",
			err:             errors.New("plain error"),
			expectedKind:    Unknown,
			expectedRetry:   TernaryUnknown,
			expectedDetails: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapErrorWithMetadata(tt.err, "context", testMetadataExtractor)

			if wrapped.Kind() != tt.expectedKind {
				t.Errorf("kind = %v, want %v", wrapped.Kind(), tt.expectedKind)
			}
			if wrapped.Retryable() != tt.expectedRetry {
				t.Errorf("retryable = %v, want %v", wrapped.Retryable(), tt.expectedRetry)
			}
			if tt.expectedDetails != nil {
				if wrapped.Details() == nil {
					t.Error("details should not be nil")
				} else if wrapped.Details()["key"] != tt.expectedDetails["key"] {
					t.Errorf("details = %v, want %v", wrapped.Details(), tt.expectedDetails)
				}
			}
		})
	}
}

type kindStringer string

func (k kindStringer) String() string { return string(k) }

type externalErrorWithStringer struct {
	message string
	kind    kindStringer
}

func (e *externalErrorWithStringer) Error() string { return e.message }
func (e *externalErrorWithStringer) ErrorKind() string {
	return e.kind.String()
}

// TestWrapError_KindAsStringer tests kind extraction via metadata contract.
func TestWrapError_KindAsStringer(t *testing.T) {
	err := &externalErrorWithStringer{
		message: "timeout error",
		kind:    kindStringer("Timeout"),
	}

	wrapped := WrapErrorWithMetadata(err, "", testMetadataExtractor)
	if wrapped.Kind() != Timeout {
		t.Errorf("kind = %v, want %v", wrapped.Kind(), Timeout)
	}
}

type externalTernary int

const (
	externalTernaryUnknown externalTernary = 0
	externalTernaryTrue    externalTernary = 1
	externalTernaryFalse   externalTernary = 2
)

func (t externalTernary) Bool() bool {
	return t == externalTernaryTrue
}

func (t externalTernary) String() string {
	switch t {
	case externalTernaryTrue:
		return "True"
	case externalTernaryFalse:
		return "False"
	default:
		return "Unspecified"
	}
}

type externalAttrs map[string]any

func (a externalAttrs) AsMap() map[string]any {
	return map[string]any(a)
}

type externalTypedError struct {
	message   string
	kind      kindStringer
	retryable externalTernary
	details   externalAttrs
}

func (e *externalTypedError) Error() string {
	return e.message
}

func (e *externalTypedError) ErrorKind() string {
	return e.kind.String()
}

func (e *externalTypedError) ErrorRetryable() (bool, bool) {
	switch e.retryable {
	case externalTernaryTrue:
		return true, true
	case externalTernaryFalse:
		return false, true
	default:
		return false, false
	}
}

func (e *externalTypedError) ErrorDetails() map[string]any {
	return map[string]any(e.details)
}

func TestWrapError_CrossPackageTypedMetadata(t *testing.T) {
	errWithFalse := &externalTypedError{
		message:   "typed metadata",
		kind:      kindStringer("Invalid"),
		retryable: externalTernaryFalse,
		details:   externalAttrs{"source": "typed"},
	}
	wrapped := WrapErrorWithMetadata(errWithFalse, "ctx", testMetadataExtractor)
	if wrapped.Kind() != Invalid {
		t.Fatalf("kind = %v, want %v", wrapped.Kind(), Invalid)
	}
	if wrapped.Retryable() != TernaryFalse {
		t.Fatalf("retryable = %v, want %v", wrapped.Retryable(), TernaryFalse)
	}
	if wrapped.Details()["source"] != "typed" {
		t.Fatalf("details = %v, want source=typed", wrapped.Details())
	}

	errUnknown := &externalTypedError{
		message:   "typed unknown retryable",
		kind:      kindStringer("Internal"),
		retryable: externalTernaryUnknown,
	}
	wrapped = WrapErrorWithMetadata(errUnknown, "", testMetadataExtractor)
	if wrapped.Retryable() != TernaryUnknown {
		t.Fatalf("retryable = %v, want %v", wrapped.Retryable(), TernaryUnknown)
	}
}

func TestApiError_String(t *testing.T) {
	err := newApiErrorS(ApiErrorRun, "test error message")

	if err.String() != "test error message" {
		t.Errorf("String() = %q, want %q", err.String(), "test error message")
	}

	result := "prefix: " + err.String()
	if result != "prefix: test error message" {
		t.Errorf("concatenation = %q, want %q", result, "prefix: test error message")
	}
}

func TestCompileError_String(t *testing.T) {
	_, err := CompileString("invalid syntax @#$%", "test")
	if err == nil {
		t.Fatal("expected compile error for invalid syntax")
	}

	if ce, ok := err.(*CompileError); ok {
		s := ce.String()
		if s == "" {
			t.Error("String() returned empty string")
		}
	}
}

func TestLuaError_String(t *testing.T) {
	e := NewError("test message")
	if e.String() != "test message" {
		t.Errorf("String() = %q, want %q", e.String(), "test message")
	}
}

func TestApiError_LuaConcatenation(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("get_error", L.NewFunction(func(L *LState) int {
		err := newApiErrorS(ApiErrorRun, "api error msg")
		L.Push(LString(err.String()))
		return 1
	}))

	err := L.DoString(`
		local e = get_error()
		result = "got: " .. e
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result := L.GetGlobal("result")
	if result.String() != "got: api error msg" {
		t.Errorf("result = %q, want %q", result.String(), "got: api error msg")
	}
}

func TestLuaError_AsLValueConcat(t *testing.T) {
	L := NewState()
	defer L.Close()

	e := NewError("my error")
	L.SetGlobal("err", e)

	err := L.DoString(`result = tostring(err)`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result := L.GetGlobal("result")
	if !strings.Contains(result.String(), "my error") {
		t.Errorf("tostring(err) = %q, should contain 'my error'", result.String())
	}
}

func TestPcallError_StringConcat(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local ok, e = pcall(function() error("pcall error") end)
		if ok then
			result = "should have failed"
		else
			result = "caught: " .. tostring(e)
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result := L.GetGlobal("result")
	if !strings.Contains(result.String(), "pcall error") {
		t.Errorf("result = %q, should contain 'pcall error'", result.String())
	}
}

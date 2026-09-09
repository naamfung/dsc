package lua

import (
	"fmt"
	"sync"
)

const (
	errorMetatableName = "error"
)

// OpenErrors registers the errors module and error metatable.
func OpenErrors(L *LState) int {
	// Register error metatable
	RegisterErrorMetatable(L)

	// Create errors module table
	mod := L.CreateTable(0, 14)

	// Add functions
	mod.RawSetString("new", LGoFunc(errorsNew))
	mod.RawSetString("wrap", LGoFunc(errorsWrap))
	mod.RawSetString("call_stack", LGoFunc(errorsCallStack))
	mod.RawSetString("is", LGoFunc(errorsIs))

	// Add kind constants (UPPERCASE convention)
	mod.RawSetString("NOT_FOUND", LString(NotFound))
	mod.RawSetString("ALREADY_EXISTS", LString(AlreadyExists))
	mod.RawSetString("INVALID", LString(Invalid))
	mod.RawSetString("PERMISSION_DENIED", LString(PermissionDenied))
	mod.RawSetString("UNAVAILABLE", LString(Unavailable))
	mod.RawSetString("INTERNAL", LString(Internal))
	mod.RawSetString("CANCELED", LString(Canceled))
	mod.RawSetString("CONFLICT", LString(Conflict))
	mod.RawSetString("TIMEOUT", LString(Timeout))
	mod.RawSetString("RATE_LIMITED", LString(RateLimited))
	mod.RawSetString("UNKNOWN", LString(Unknown))
	mod.Immutable = true

	// Set as global
	L.SetGlobal("errors", mod)

	return 1
}

var (
	cachedErrorMt     *LTable
	cachedErrorMtOnce sync.Once
)

// buildErrorMetatable creates the error metatable once
func buildErrorMetatable() *LTable {
	cachedErrorMtOnce.Do(func() {
		mt := newLTable(0, 4)

		mt.RawSetString("__tostring", LGoFunc(errorToString))
		mt.RawSetString("__concat", LGoFunc(errorConcat))

		methods := newLTable(0, 8)
		methods.RawSetString("kind", LGoFunc(errorKind))
		methods.RawSetString("retryable", LGoFunc(errorRetryable))
		methods.RawSetString("details", LGoFunc(errorDetails))
		methods.RawSetString("message", LGoFunc(errorMessage))
		methods.RawSetString("stack", LGoFunc(errorStack))
		methods.Immutable = true

		mt.RawSetString("__index", methods)
		mt.Immutable = true

		cachedErrorMt = mt
	})
	return cachedErrorMt
}

// RegisterErrorMetatable registers the cached error metatable for this LState.
// This must be called once per LState to enable error methods like :kind().
func RegisterErrorMetatable(L *LState) {
	mt := buildErrorMetatable()

	// Register as builtin metatable for *Error type
	dummyErr := &Error{}
	L.SetMetatable(dummyErr, mt)

	// Also store in registry for explicit SetErrorMetatable calls
	L.SetField(L.Get(RegistryIndex), errorMetatableName, mt)
}

// errorToString implements __tostring for errors
func errorToString(L *LState) int {
	v := L.Get(1)
	if e, ok := v.(*Error); ok {
		L.Push(LString(e.String()))
		return 1
	}
	L.Push(LString(""))
	return 1
}

// errorConcat implements __concat for errors
func errorConcat(L *LState) int {
	lhs := L.Get(1)
	rhs := L.Get(2)

	var left, right string

	switch v := lhs.(type) {
	case LString:
		left = string(v)
	case *Error:
		left = v.String()
	default:
		left = lhs.String()
	}

	switch v := rhs.(type) {
	case LString:
		right = string(v)
	case *Error:
		right = v.String()
	default:
		right = rhs.String()
	}

	L.Push(LString(left + right))
	return 1
}

// getErrorMetatable retrieves the error metatable from registry.
func getErrorMetatable(L *LState) LValue {
	return L.GetField(L.Get(RegistryIndex), errorMetatableName)
}

// SetErrorMetatable sets the error metatable on an Error value.
func SetErrorMetatable(L *LState, e *Error) {
	L.SetMetatable(e, getErrorMetatable(L))
}

// NewLuaError creates an Error and sets up its metatable.
func NewLuaError(L *LState, message string) *Error {
	e := NewError(message)
	e.LuaStack = captureStackTrace(L)
	SetErrorMetatable(L, e)
	return e
}

// errorKind implements err:kind() method
func errorKind(L *LState) int {
	e, ok := L.Get(1).(*Error)
	if !ok {
		L.Push(LString(Unknown))
		return 1
	}
	L.Push(LString(e.Kind()))
	return 1
}

// errorRetryable implements err:retryable() method
func errorRetryable(L *LState) int {
	e, ok := L.Get(1).(*Error)
	if !ok {
		L.Push(LNil)
		return 1
	}
	switch e.Retryable() {
	case TernaryUnknown:
		L.Push(LNil)
	case TernaryTrue:
		L.Push(LTrue)
	case TernaryFalse:
		L.Push(LFalse)
	default:
		L.Push(LNil)
	}
	return 1
}

// errorDetails implements err:details() method
func errorDetails(L *LState) int {
	e, ok := L.Get(1).(*Error)
	if !ok {
		L.Push(LNil)
		return 1
	}
	details := e.Details()
	if len(details) == 0 {
		L.Push(LNil)
		return 1
	}

	tbl := L.CreateTable(0, len(details))
	for k, v := range details {
		tbl.RawSetString(k, goToLuaValue(L, v))
	}
	L.Push(tbl)
	return 1
}

// errorMessage implements err:message() method
func errorMessage(L *LState) int {
	e, ok := L.Get(1).(*Error)
	if !ok {
		L.Push(LString(""))
		return 1
	}
	L.Push(LString(e.Message))
	return 1
}

// errorStack implements err:stack() method
func errorStack(L *LState) int {
	e, ok := L.Get(1).(*Error)
	if !ok {
		L.Push(LString(""))
		return 1
	}
	L.Push(LString(e.Stack()))
	return 1
}

// errorsNew implements errors.new(msg) or errors.new({...})
func errorsNew(L *LState) int {
	arg := L.CheckAny(1)

	var e *Error

	switch v := arg.(type) {
	case LString:
		// Simple string error
		e = NewError(string(v))

	case *LTable:
		// Table-based error with metadata
		msgVal := v.RawGetString("message")
		if msgVal == LNil {
			L.ArgError(1, "message field is required")
			return 0
		}
		msgStr, ok := msgVal.(LString)
		if !ok {
			L.ArgError(1, "message must be a string")
			return 0
		}
		e = NewError(string(msgStr))

		// Extract kind
		kindVal := v.RawGetString("kind")
		if kindVal != LNil {
			if kindStr, ok := kindVal.(LString); ok {
				e.kind = Kind(kindStr)
			}
		}

		// Extract retryable
		retryableVal := v.RawGetString("retryable")
		if retryableVal != LNil {
			if retryableBool, ok := retryableVal.(LBool); ok {
				b := bool(retryableBool)
				e.retryable = &b
			}
		}

		// Extract details
		detailsVal := v.RawGetString("details")
		if detailsVal != LNil {
			if detailsTable, ok := detailsVal.(*LTable); ok {
				e.details = make(map[string]any)
				detailsTable.ForEach(func(k, val LValue) {
					if keyStr, ok := k.(LString); ok {
						e.details[string(keyStr)] = luaToGoValue(val)
					}
				})
			}
		}

	default:
		L.ArgError(1, "expected string or table")
		return 0
	}

	// Capture Lua stack
	e.LuaStack = captureStackTrace(L)

	// Set metatable
	SetErrorMetatable(L, e)

	L.Push(e)
	return 1
}

// errorsWrap implements errors.wrap(parent, context)
func errorsWrap(L *LState) int {
	parent := L.CheckAny(1)
	contextArg := L.CheckAny(2)

	var parentErr error
	var parentError *Error

	switch v := parent.(type) {
	case *Error:
		parentErr = v
		parentError = v
	case LString:
		parentErr = fmt.Errorf("%s", string(v))
	default:
		L.ArgError(1, "expected string or error object")
		return 0
	}

	var context string
	switch v := contextArg.(type) {
	case *Error:
		context = v.Message
	case LString:
		context = string(v)
	default:
		L.ArgError(2, "expected string or error object")
		return 0
	}

	e := WrapErrorWithLua(L, parentErr, context)

	// Preserve metadata from parent if available
	if parentError != nil {
		if parentError.kind != "" {
			e.kind = parentError.kind
		}
		if parentError.retryable != nil {
			e.retryable = parentError.retryable
		}
		if len(parentError.details) > 0 {
			e.details = parentError.details
		}
	}

	// Set metatable
	SetErrorMetatable(L, e)

	L.Push(e)
	return 1
}

// errorsCallStack implements errors.call_stack(err)
func errorsCallStack(L *LState) int {
	v := L.Get(1)
	e, ok := v.(*Error)
	if !ok {
		L.Push(LNil)
		return 1
	}

	if e.LuaStack == nil || len(e.LuaStack.Frames) == 0 {
		L.Push(LNil)
		return 1
	}

	// Create table representation of stack trace
	result := L.NewTable()
	result.RawSetString("thread", LString(e.LuaStack.ThreadID))

	framesTable := L.NewTable()
	for i, frame := range e.LuaStack.Frames {
		frameTable := L.NewTable()
		frameTable.RawSetString("level", LNumber(frame.Level))
		frameTable.RawSetString("source", LString(frame.Source))
		frameTable.RawSetString("line", LNumber(frame.CurrentLine))
		frameTable.RawSetString("name", LString(frame.Name))
		frameTable.RawSetString("type", LString(frame.FuncType))
		framesTable.RawSetInt(i+1, frameTable)
	}
	result.RawSetString("frames", framesTable)

	L.Push(result)
	return 1
}

// errorsIs implements errors.is(err, kind)
func errorsIs(L *LState) int {
	v := L.CheckAny(1)
	kindStr := L.CheckString(2)

	e, ok := v.(*Error)
	if !ok {
		L.Push(LFalse)
		return 1
	}

	if string(e.Kind()) == kindStr {
		L.Push(LTrue)
	} else {
		L.Push(LFalse)
	}
	return 1
}

// goToLuaValue converts a Go value to a Lua value.
func goToLuaValue(L *LState, val any) LValue {
	switch v := val.(type) {
	case string:
		return LString(v)
	case int:
		return LInteger(v)
	case int8:
		return LInteger(v)
	case int16:
		return LInteger(v)
	case int32:
		return LInteger(v)
	case int64:
		return LInteger(v)
	case uint:
		return LInteger(v)
	case uint8:
		return LInteger(v)
	case uint16:
		return LInteger(v)
	case uint32:
		return LInteger(v)
	case uint64:
		return LInteger(int64(v))
	case float32:
		return LNumber(v)
	case float64:
		return LNumber(v)
	case bool:
		return LBool(v)
	case map[string]any:
		table := L.CreateTable(0, len(v))
		for k, val := range v {
			table.RawSetString(k, goToLuaValue(L, val))
		}
		return table
	case nil:
		return LNil
	default:
		return LString(fmt.Sprintf("%v", v))
	}
}

// luaToGoValue converts a Lua value to a Go value.
func luaToGoValue(val LValue) any {
	switch v := val.(type) {
	case LString:
		return string(v)
	case LNumber:
		return float64(v)
	case LInteger:
		return int64(v)
	case LBool:
		return bool(v)
	case *LTable:
		m := make(map[string]any)
		v.ForEach(func(k, val LValue) {
			if keyStr, ok := k.(LString); ok {
				m[string(keyStr)] = luaToGoValue(val)
			}
		})
		return m
	case *LNilType:
		return nil
	default:
		return val.String()
	}
}

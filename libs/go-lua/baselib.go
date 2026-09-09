package lua

import (
	"fmt"
	"strconv"
	"strings"
)

// baseFuncs contains stateless Go functions for base library.
// Minimal sandbox set for AI agent environments.
var baseFuncs = map[string]LGoFunc{
	"assert":       baseAssert,
	"error":        baseError,
	"getmetatable": baseGetMetatable,
	"next":         baseNext,
	"pcall":        basePCall,
	"print":        basePrint,
	"rawequal":     baseRawEqual,
	"rawget":       baseRawGet,
	"rawset":       baseRawSet,
	"select":       baseSelect,
	"setmetatable": baseSetMetatable,
	"tonumber":     baseToNumber,
	"tostring":     baseToString,
	"type":         baseType,
	"unpack":       baseUnpack,
	"xpcall":       baseXPCall,
}

// Stateless auxiliary functions for ipairs/pairs
var (
	gofuncIpairsAux = LGoFunc(ipairsaux)
	gofuncPairsAux  = LGoFunc(pairsaux)
)

/* basic functions {{{ */

func OpenBase(L *LState) int {
	global := L.Get(GlobalsIndex).(*LTable)
	global.RawSetString("_G", global)
	global.RawSetString("_VERSION", LString(Version))
	global.RawSetString("_GOPHER_LUA_VERSION", LString(PackageName+" "+PackageVersion))

	// Register all base functions as LGoFunc (zero allocation)
	for name, fn := range baseFuncs {
		global.RawSetString(name, fn)
	}

	// ipairs and pairs push aux functions directly
	global.RawSetString("ipairs", LGoFunc(baseIpairs))
	global.RawSetString("pairs", LGoFunc(basePairs))

	return 1
}

func baseAssert(L *LState) int {
	if !L.ToBool(1) {
		L.RaiseError(L.OptString(2, "assertion failed!"))
		return 0
	}
	return L.GetTop()
}

func baseError(L *LState) int {
	obj := L.CheckAny(1)
	level := L.OptInt(2, 1)
	L.Error(obj, level)
	return 0
}

func baseGetMetatable(L *LState) int {
	L.Push(L.GetMetatable(L.CheckAny(1)))
	return 1
}

func ipairsaux(L *LState) int {
	tb := L.CheckTable(1)
	i := L.CheckInt(2)
	i++
	v := tb.RawGetInt(i)
	if v == LNil {
		return 0
	}
	L.Push(LInteger(i))
	L.Push(v)
	return 2
}

func baseIpairs(L *LState) int {
	tb := L.CheckTable(1)
	L.Push(gofuncIpairsAux)
	L.Push(tb)
	L.Push(LInteger(0))
	return 3
}

func baseNext(L *LState) int {
	tb := L.CheckTable(1)
	index := LNil
	if L.GetTop() >= 2 {
		index = L.Get(2)
	}
	key, value := tb.Next(index)
	if key == LNil {
		L.Push(LNil)
		return 1
	}
	L.Push(key)
	L.Push(value)
	return 2
}

func pairsaux(L *LState) int {
	tb := L.CheckTable(1)
	key, value := tb.Next(L.Get(2))
	if key == LNil {
		return 0
	}
	L.Pop(1)
	L.Push(key)
	L.Push(key)
	L.Push(value)
	return 2
}

func basePairs(L *LState) int {
	tb := L.CheckTable(1)
	L.Push(gofuncPairsAux)
	L.Push(tb)
	L.Push(LNil)
	return 3
}

// pcallContinuation is called after yield resumes inside pcall.
// At this point, the called function has returned its results on the stack.
func pcallContinuation(L *LState, _ interface{}, _ ResumeState) int {
	// Results are on stack. Just prepend true.
	L.Insert(LTrue, 1)
	return L.GetTop()
}

func basePCall(L *LState) int {
	L.CheckAny(1)
	v := L.Get(1)
	if v.Type() != LTFunction && L.GetMetaField(v, "__call").Type() != LTFunction {
		L.Push(LFalse)
		L.Push(LString("attempt to call a " + v.Type().String() + " value"))
		return 2
	}

	L.currentFrame.Protected = true
	nargs := L.GetTop() - 1

	if L.Parent == nil {
		// Direct call (not in coroutine) - use defer/recover
		sp := L.stack.Sp()
		base := L.reg.Top() - nargs - 1

		var err error
		func() {
			defer func() {
				if rcv := recover(); rcv != nil {
					if aerr, ok := rcv.(*ApiError); ok {
						err = aerr
					} else {
						err = &ApiError{Object: LString(fmt.Sprint(rcv))}
					}
				}
			}()
			L.CallK(nargs, MultRet, pcallContinuation, nil)
		}()

		if err != nil {
			L.stack.SetSp(sp)
			L.currentFrame = L.stack.Last()
			L.reg.SetTop(base)
			L.currentFrame.Protected = false
			// Clear continuation info after error
			if ext := L.getFrameExt(L.currentFrame); ext != nil {
				ext.Continuation = nil
				ext.ContinuationCtx = nil
			}
			L.Push(LFalse)
			L.Push(err.(*ApiError).Object)
			return 2
		}
		L.currentFrame.Protected = false
	} else {
		// In coroutine - use continuation for yield-transparency
		// Still need defer/recover for error handling
		sp := L.stack.Sp()
		base := L.reg.Top() - nargs - 1

		var err error
		func() {
			defer func() {
				if rcv := recover(); rcv != nil {
					if aerr, ok := rcv.(*ApiError); ok {
						err = aerr
					} else {
						err = &ApiError{Object: LString(fmt.Sprint(rcv))}
					}
				}
			}()
			L.CallK(nargs, MultRet, pcallContinuation, nil)
		}()

		// Check for yield before any cleanup
		if L.yieldState != yieldNone {
			return -1
		}

		if err != nil {
			L.stack.SetSp(sp)
			L.currentFrame = L.stack.Last()
			L.reg.SetTop(base)
			L.currentFrame.Protected = false
			// Clear continuation info after error
			if ext := L.getFrameExt(L.currentFrame); ext != nil {
				ext.Continuation = nil
				ext.ContinuationCtx = nil
			}
			L.Push(LFalse)
			L.Push(err.(*ApiError).Object)
			return 2
		}
		L.currentFrame.Protected = false
	}

	L.Insert(LTrue, 1)
	return L.GetTop()
}

func basePrint(L *LState) int {
	top := L.GetTop()
	for i := 1; i <= top; i++ {
		fmt.Print(L.ToStringMeta(L.Get(i)).String())
		if i != top {
			fmt.Print("\t")
		}
	}
	fmt.Println("")
	return 0
}

func baseRawEqual(L *LState) int {
	lhs := L.CheckAny(1)
	rhs := L.CheckAny(2)

	// Handle LGoFunc specially since function types are not comparable with ==
	if lhs.Type() == LTFunction && rhs.Type() == LTFunction {
		l, lok := lhs.(LGoFunc)
		r, rok := rhs.(LGoFunc)
		if lok && rok {
			// Compare function pointers via their string representation
			if fmt.Sprintf("%p", l) == fmt.Sprintf("%p", r) {
				L.Push(LTrue)
			} else {
				L.Push(LFalse)
			}
			return 1
		}
	}

	if lhs == rhs {
		L.Push(LTrue)
	} else {
		L.Push(LFalse)
	}
	return 1
}

func baseRawGet(L *LState) int {
	L.Push(L.RawGet(L.CheckTable(1), L.CheckAny(2)))
	return 1
}

func baseRawSet(L *LState) int {
	L.RawSet(L.CheckTable(1), L.CheckAny(2), L.CheckAny(3))
	return 0
}

func baseSelect(L *LState) int {
	L.CheckTypes(1, LTNumber, LTInteger, LTString)
	switch lv := L.Get(1).(type) {
	case LNumber:
		idx := int(lv)
		num := L.GetTop()
		if idx < 0 {
			idx = num + idx
		} else if idx > num {
			idx = num
		}
		if 1 > idx {
			L.ArgError(1, "index out of range")
		}
		return num - idx
	case LInteger:
		idx := int(lv)
		num := L.GetTop()
		if idx < 0 {
			idx = num + idx
		} else if idx > num {
			idx = num
		}
		if 1 > idx {
			L.ArgError(1, "index out of range")
		}
		return num - idx
	case LString:
		if string(lv) != "#" {
			L.ArgError(1, "invalid string '"+string(lv)+"'")
		}
		L.Push(LNumber(L.GetTop() - 1))
		return 1
	}
	return 0
}

func baseSetMetatable(L *LState) int {
	L.CheckTypes(2, LTNil, LTTable)
	obj := L.Get(1)
	if obj == LNil {
		L.RaiseError("cannot set metatable to a nil object.")
	}
	mt := L.Get(2)
	if m := L.metatable(obj, true); m != LNil {
		if tb, ok := m.(*LTable); ok && tb.RawGetString("__metatable") != LNil {
			L.RaiseError("cannot change a protected metatable")
		}
	}
	L.SetMetatable(obj, mt)
	L.SetTop(1)
	return 1
}

func baseToNumber(L *LState) int {
	base := L.OptInt(2, 10)
	noBase := L.Get(2) == LNil

	switch lv := L.CheckAny(1).(type) {
	case LNumber:
		L.Push(lv)
	case LInteger:
		L.Push(LNumber(lv))
	case LString:
		str := strings.Trim(string(lv), " \n\t")
		if strings.Contains(str, ".") {
			if v, err := strconv.ParseFloat(str, LNumberBit); err != nil {
				L.Push(LNil)
			} else {
				L.Push(LNumber(v))
			}
		} else {
			if noBase && strings.HasPrefix(strings.ToLower(str), "0x") {
				base, str = 16, str[2:] // Hex number
			}
			if v, err := strconv.ParseInt(str, base, LNumberBit); err != nil {
				L.Push(LNil)
			} else {
				L.Push(LNumber(v))
			}
		}
	default:
		L.Push(LNil)
	}
	return 1
}

func baseToString(L *LState) int {
	v1 := L.CheckAny(1)
	L.Push(L.ToStringMeta(v1))
	return 1
}

func baseType(L *LState) int {
	L.Push(LString(L.CheckAny(1).Type().String()))
	return 1
}

func baseUnpack(L *LState) int {
	tb := L.CheckTable(1)
	start := L.OptInt(2, 1)
	end := L.OptInt(3, tb.Len())
	for i := start; i <= end; i++ {
		L.Push(tb.RawGetInt(i))
	}
	ret := end - start + 1
	if ret < 0 {
		return 0
	}
	return ret
}

// xpcallContinuation is called after yield resumes inside xpcall.
func xpcallContinuation(L *LState, ctx interface{}, _ ResumeState) int {
	// ctx contains the top value before the call
	top := ctx.(int)
	// Results are on stack. Just prepend true.
	L.Insert(LTrue, top+1)
	return L.GetTop() - top
}

func baseXPCall(L *LState) int {
	fn := L.CheckFunction(1)
	errfunc := L.CheckFunction(2)

	// Mark the xpcall frame as protected for error handling
	L.currentFrame.Protected = true
	L.setFrameExt(L.currentFrame).ErrFunc = errfunc

	top := L.GetTop()
	L.Push(fn)

	// Use CallK with continuation for yield-transparent xpcall
	L.CallK(0, MultRet, xpcallContinuation, top)

	// Check for yield before any cleanup
	if L.yieldState != yieldNone {
		return -1
	}

	L.currentFrame.Protected = false
	if ext := L.getFrameExt(L.currentFrame); ext != nil {
		ext.ErrFunc = nil
	}
	L.Insert(LTrue, top+1)
	return L.GetTop() - top
}

/* }}} */

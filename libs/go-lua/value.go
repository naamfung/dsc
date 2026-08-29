package lua

import (
	"context"
	"fmt"
	"strconv"
)

type LValueType int

const (
	LTNil LValueType = iota
	LTBool
	LTNumber
	LTInteger
	LTString
	LTFunction
	LTUserData
	LTThread
	LTTable
	LTChannel
	LTType
)

var lValueNames = [11]string{"nil", "boolean", "number", "number", "string", "function", "userdata", "thread", "table", "channel", "type"}

func (vt LValueType) String() string {
	return lValueNames[vt]
}

type LValue interface {
	String() string
	Type() LValueType
}

// LVIsFalse returns true if a given LValue is a nil or false otherwise false.
func LVIsFalse(v LValue) bool { return v == LNil || v == LFalse }

// LVAsBool returns false if a given LValue is a nil or false otherwise true.
func LVAsBool(v LValue) bool { return v != LNil && v != LFalse }

// LVAsString returns string representation of a given LValue
// if the LValue is a string, number, or Error, otherwise an empty string.
func LVAsString(v LValue) string {
	switch sn := v.(type) {
	case LString:
		return string(sn)
	case LNumber:
		return sn.String()
	case LInteger:
		return sn.String()
	case *Error:
		return sn.String()
	default:
		return ""
	}
}

// LVCanConvToString returns true if a given LValue is a string, number, or Error
// otherwise false.
func LVCanConvToString(v LValue) bool {
	switch v.(type) {
	case LString, LNumber, LInteger, *Error:
		return true
	default:
		return false
	}
}

// LVAsNumber tries to convert a given LValue to a number.
func LVAsNumber(v LValue) LNumber {
	switch lv := v.(type) {
	case LNumber:
		return lv
	case LInteger:
		return LNumber(lv)
	case LString:
		if num, err := parseNumber(string(lv)); err == nil {
			return num
		}
	}
	return LNumber(0)
}

type LNilType struct{}

func (nl *LNilType) String() string   { return "nil" }
func (nl *LNilType) Type() LValueType { return LTNil }

var LNil = LValue(&LNilType{})

type LBool bool

func (bl LBool) String() string {
	if bl {
		return "true"
	}
	return "false"
}
func (bl LBool) Type() LValueType { return LTBool }

var LTrue = LBool(true)
var LFalse = LBool(false)

type LString string

func (st LString) String() string   { return string(st) }
func (st LString) Type() LValueType { return LTString }

// Format implements the fmt.Formatter interface.
func (st LString) Format(f fmt.State, c rune) {
	switch c {
	case 'd', 'i':
		if nm, err := parseNumber(string(st)); err == nil {
			defaultFormat(nm, f, 'd')
		} else {
			defaultFormat(string(st), f, 's')
		}
	default:
		defaultFormat(string(st), f, c)
	}
}

func (nm LNumber) String() string {
	if IsIntegerValue(nm) {
		return strconv.FormatInt(int64(nm), 10)
	}
	return strconv.FormatFloat(float64(nm), 'g', -1, 64)
}

func (nm LNumber) Type() LValueType { return LTNumber }

func (i LInteger) String() string   { return strconv.FormatInt(int64(i), 10) }
func (i LInteger) Type() LValueType { return LTInteger }

// Format implements the fmt.Formatter interface.
func (nm LNumber) Format(f fmt.State, c rune) {
	switch c {
	case 'q', 's':
		defaultFormat(nm.String(), f, c)
	case 'b', 'c', 'd', 'o', 'x', 'X', 'U':
		defaultFormat(int64(nm), f, c)
	case 'e', 'E', 'f', 'F', 'g', 'G':
		defaultFormat(float64(nm), f, c)
	case 'i':
		defaultFormat(int64(nm), f, 'd')
	default:
		if IsIntegerValue(nm) {
			defaultFormat(int64(nm), f, c)
		} else {
			defaultFormat(float64(nm), f, c)
		}
	}
}

type LTable struct {
	Metatable LValue
	Immutable bool

	Array   []LValue
	Dict    map[LValue]LValue
	Strdict map[string]LValue
	Keys    []LValue
	K2i     map[LValue]int
}

func (tb *LTable) String() string   { return fmt.Sprintf("table: %p", tb) }
func (tb *LTable) Type() LValueType { return LTTable }

type LFunction struct {
	IsG       bool
	Env       *LTable
	Proto     *FunctionProto
	GFunction LGFunction
	Upvalues  []*Upvalue
}

// LGFunction is the Go function signature for Lua-callable functions.
type LGFunction func(*LState) int

func (fn *LFunction) String() string   { return fmt.Sprintf("function: %p", fn) }
func (fn *LFunction) Type() LValueType { return LTFunction }

// LGoFunc is a stateless Go function that can be shared across all LStates.
// Unlike LFunction, it requires no per-state allocation and can be stored
// directly in tables or globals without wrapping.
//
// Performance: LGoFunc avoids the LFunction allocation overhead and pointer
// indirection through Fn.GFunction. For modules that don't need upvalues or
// environments, using LGoFunc provides better performance.
//
// TODO: Refactor all go-lua internal code to use LGoFunc natively:
// - Migrate internal libs (baselib, stringlib, mathlib, tablelib, etc.) to use LGoFunc
// - Simplify callFrame to primarily use GoFunc, remove Fn.IsG complexity
// - Update pushCallFrame, initCallFrame to assume GoFunc is the default
// - Remove legacy LFunction wrapping code and pinning state hacks
// - Update public API (SetGlobal, PreloadModule, etc.) to prefer LGoFunc
// This would eliminate the dual code paths in VM and simplify maintenance.
type LGoFunc LGFunction

func (gf LGoFunc) String() string   { return fmt.Sprintf("gofunc: %p", gf) }
func (gf LGoFunc) Type() LValueType { return LTFunction }

type Global struct {
	MainThread    *LState
	CurrentThread *LState
	Registry      *LTable
	Global        *LTable

	builtinMts map[int]LValue

	// Owner is the host process/context that owns this Lua VM.
	// Set by the host runtime for fast access from modules.
	Owner any
}

type LState struct {
	G       *Global
	Parent  *LState
	Env     *LTable
	Panic   func(*LState)
	Dead    bool
	Options Options

	stop         int32
	reg          *registry
	stack        callFrameStack
	currentFrame *callFrame
	wrapped      bool
	uvcache      *Upvalue
	hasErrorFunc bool
	mainLoop     func(*LState, *callFrame)
	ctx          context.Context
	ctxCancelFn  context.CancelFunc
	ctxDone      <-chan struct{}
	frameExt     map[int16]*callFrameExt // lazy-allocated frame extensions keyed by Idx
	yieldState   uint8                   // 0=not yielded, 1=system yield, 2=user yield
	// tailYieldPending 记录「上一次 resume 是 tail 位置 Go 函数 yield、且协程栈
	// 已被清空」；下次 resume 的传入值即整个协程的最终返回。
	tailYieldPending bool
	yieldCont        uint8 // pending yield continuation type for Lua frames
	yieldContRA      int32 // target register for continuation result
	yieldContRB      int32 // call's ReturnBase (where the result lands)
	yieldContIdx     int16 // frame Idx that owns this continuation
}

func (ls *LState) String() string   { return fmt.Sprintf("thread: %p", ls) }
func (ls *LState) Type() LValueType { return LTThread }

type LUserData struct {
	Value     interface{}
	Metatable LValue
}

func (ud *LUserData) String() string {
	if ud.Value == nil {
		return fmt.Sprintf("userdata: %p", ud)
	}
	return fmt.Sprintf("userdata(%T): %p", ud.Value, ud)
}
func (ud *LUserData) Type() LValueType { return LTUserData }

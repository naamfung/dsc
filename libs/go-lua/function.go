package lua

import (
	"fmt"
	"strings"
	"sync"
)

const VarArgIsVarArg uint8 = 2

type DbgLocalInfo struct {
	Name    string
	StartPc int
	EndPc   int
}

type DbgCall struct {
	Name string
	Pc   int
}

type FunctionProto struct {
	SourceName         string
	LineDefined        int
	LastLineDefined    int
	NumUpvalues        uint8
	NumParameters      uint8
	IsVarArg           uint8
	NumUsedRegisters   uint8
	Code               []uint32
	Constants          []LValue
	FunctionPrototypes []*FunctionProto

	DbgSourcePositions []int
	DbgLocals          []*DbgLocalInfo
	DbgCalls           []DbgCall
	DbgUpvalues        []string

	// TypeInfo holds encoded type manifest for this module.
	// Use types/io.EncodeManifest/DecodeManifest to work with this data.
	// Stored on the root FunctionProto and propagated to nested functions.
	TypeInfo []byte

	stringConstants []string

	typeInfoOnce       sync.Once
	typeBindings       []typeBinding
	typeBindingsByName map[string]*LType
}

/* Upvalue {{{ */

type Upvalue struct {
	next   *Upvalue
	reg    *registry
	index  int
	value  LValue
	closed bool
}

func (uv *Upvalue) Value() LValue {
	if uv.closed || uv.reg == nil {
		return uv.value
	}
	return uv.reg.array[uv.index]
}

func (uv *Upvalue) SetValue(value LValue) {
	if uv.IsClosed() {
		uv.value = value
	} else {
		uv.reg.Set(uv.index, value)
	}
}

func (uv *Upvalue) Close() {
	value := uv.Value()
	uv.closed = true
	uv.value = value
}

func (uv *Upvalue) IsClosed() bool {
	return uv.closed || uv.reg == nil
}

func UpvalueIndex(i int) int {
	return GlobalsIndex - i
}

/* }}} */

/* FunctionProto {{{ */

func newFunctionProto(name string) *FunctionProto {
	return &FunctionProto{
		SourceName:         name,
		LineDefined:        0,
		LastLineDefined:    0,
		NumUpvalues:        0,
		NumParameters:      0,
		IsVarArg:           0,
		NumUsedRegisters:   2,
		Code:               make([]uint32, 0, 128),
		Constants:          make([]LValue, 0, 32),
		FunctionPrototypes: make([]*FunctionProto, 0, 16),

		DbgSourcePositions: make([]int, 0, 128),
		DbgLocals:          make([]*DbgLocalInfo, 0, 16),
		DbgCalls:           make([]DbgCall, 0, 128),
		DbgUpvalues:        make([]string, 0, 16),

		stringConstants: make([]string, 0, 32),
	}
}

// RebuildStringConstants rebuilds the stringConstants cache from Constants.
// This is needed after deserializing bytecode since stringConstants is not serialized.
// The stringConstants slice has the same length as Constants - each entry is either
// the string value (if Constants[i] is LString) or empty string (otherwise).
func (fp *FunctionProto) RebuildStringConstants() {
	fp.stringConstants = make([]string, len(fp.Constants))
	for i, c := range fp.Constants {
		if s, ok := c.(LString); ok {
			fp.stringConstants[i] = string(s)
		}
	}
}

// GetTypeInfo returns the encoded type manifest bytes.
func (fp *FunctionProto) GetTypeInfo() []byte {
	return fp.TypeInfo
}

// SetTypeInfo sets the encoded type manifest bytes.
func (fp *FunctionProto) SetTypeInfo(data []byte) {
	fp.TypeInfo = data
	// Propagate to nested prototypes so type values are available
	// inside nested functions at runtime.
	for _, child := range fp.FunctionPrototypes {
		if child != nil {
			child.SetTypeInfo(data)
		}
	}
}

func (fp *FunctionProto) String() string {
	return fp.str(1, 0)
}

func (fp *FunctionProto) str(level int, count int) string {
	indent := strings.Repeat("  ", level-1)
	var buf []string
	buf = append(buf, fmt.Sprintf("%v; function [%v] definition (level %v)\n",
		indent, count, level))
	buf = append(buf, fmt.Sprintf("%v; %v upvalues, %v params, %v stacks\n",
		indent, fp.NumUpvalues, fp.NumParameters, fp.NumUsedRegisters))
	for reg, linfo := range fp.DbgLocals {
		buf = append(buf, fmt.Sprintf("%v.local %v ; %v\n", indent, linfo.Name, reg))
	}
	for reg, upvalue := range fp.DbgUpvalues {
		buf = append(buf, fmt.Sprintf("%v.upvalue %v ; %v\n", indent, upvalue, reg))
	}
	for reg, conzt := range fp.Constants {
		buf = append(buf, fmt.Sprintf("%v.const %v ; %v\n", indent, conzt.String(), reg))
	}
	buf = append(buf, "\n")

	protono := 0
	for no, code := range fp.Code {
		inst := opGetOpCode(code)
		if inst == OP_CLOSURE {
			buf = append(buf, "\n")
			buf = append(buf, fp.FunctionPrototypes[protono].str(level+1, protono))
			buf = append(buf, "\n")
			protono++
		}
		buf = append(buf, fmt.Sprintf("%v[%03d] %v (line:%v)\n",
			indent, no+1, opToString(code), fp.DbgSourcePositions[no]))

	}
	buf = append(buf, fmt.Sprintf("%v; end of function\n", indent))
	return strings.Join(buf, "")
}

/* }}} */

/* LFunction {{{ */

func newLFunctionL(proto *FunctionProto, env *LTable, nupvalue int) *LFunction {
	return &LFunction{
		IsG: false,
		Env: env,

		Proto:     proto,
		GFunction: nil,
		Upvalues:  make([]*Upvalue, nupvalue),
	}
}

func newLFunctionG(gfunc LGFunction, env *LTable, nupvalue int) *LFunction {
	return &LFunction{
		IsG: true,
		Env: env,

		Proto:     nil,
		GFunction: gfunc,
		Upvalues:  make([]*Upvalue, nupvalue),
	}
}

func (fn *LFunction) LocalName(regno, pc int) (string, bool) {
	if fn.IsG {
		return "", false
	}
	p := fn.Proto
	for i := 0; i < len(p.DbgLocals) && p.DbgLocals[i].StartPc < pc; i++ {
		if pc < p.DbgLocals[i].EndPc {
			regno--
			if regno == 0 {
				return p.DbgLocals[i].Name, true
			}
		}
	}
	return "", false
}

/* }}} */

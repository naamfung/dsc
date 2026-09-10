// Package bytecode provides serialization and deserialization of compiled Lua bytecode.
package bytecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"

	lua "github.com/wippyai/go-lua"
)

const (
	version     = 2          // v2: added TypeInfo
	headerMagic = 0x4C554143 // "LUAC"
)

var (
	ErrInvalidHeader     = errors.New("invalid bytecode header")
	ErrVersionMismatch   = errors.New("bytecode version mismatch")
	ErrCorruptedBytecode = errors.New("corrupted bytecode")
)

// Dump serializes a FunctionProto to bytes.
func Dump(proto *lua.FunctionProto) ([]byte, error) {
	var buf bytes.Buffer
	w := &writer{w: &buf}

	// Header
	w.writeUint32(headerMagic)
	w.writeByte(version)

	// Write the proto
	if err := w.writeProto(proto); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Undump deserializes bytes to a FunctionProto.
func Undump(data []byte) (*lua.FunctionProto, error) {
	r := &reader{r: bytes.NewReader(data)}

	// Header
	magic := r.readUint32()
	if magic != headerMagic {
		return nil, ErrInvalidHeader
	}

	ver := r.readByte()
	if ver != version {
		return nil, ErrVersionMismatch
	}

	proto, err := r.readProto()
	if err != nil {
		return nil, err
	}

	// Rebuild stringConstants cache from Constants for all protos
	rebuildStringConstants(proto)
	// Propagate root type info to nested protos for runtime type usage.
	if len(proto.TypeInfo) > 0 {
		proto.SetTypeInfo(proto.TypeInfo)
	}

	return proto, nil
}

// rebuildStringConstants rebuilds the stringConstants cache from Constants.
// This is needed because stringConstants is not serialized but is used by the VM.
func rebuildStringConstants(p *lua.FunctionProto) {
	p.RebuildStringConstants()

	// Recursively rebuild for nested protos
	for _, np := range p.FunctionPrototypes {
		rebuildStringConstants(np)
	}
}

type writer struct {
	w   io.Writer
	err error
}

func (w *writer) writeByte(b byte) {
	if w.err != nil {
		return
	}
	_, w.err = w.w.Write([]byte{b})
}

func (w *writer) writeUint32(v uint32) {
	if w.err != nil {
		return
	}
	w.err = binary.Write(w.w, binary.LittleEndian, v)
}

func (w *writer) writeUint64(v uint64) {
	if w.err != nil {
		return
	}
	w.err = binary.Write(w.w, binary.LittleEndian, v)
}

func (w *writer) writeInt(v int) {
	w.writeUint64(uint64(v))
}

func (w *writer) writeString(s string) {
	if w.err != nil {
		return
	}
	data := []byte(s)
	w.writeUint32(uint32(len(data)))
	if len(data) > 0 {
		_, w.err = w.w.Write(data)
	}
}

func (w *writer) writeProto(p *lua.FunctionProto) error {
	if w.err != nil {
		return w.err
	}

	// Basic info
	w.writeString(p.SourceName)
	w.writeInt(p.LineDefined)
	w.writeInt(p.LastLineDefined)
	w.writeByte(p.NumUpvalues)
	w.writeByte(p.NumParameters)
	w.writeByte(p.IsVarArg)
	w.writeByte(p.NumUsedRegisters)

	// Code
	w.writeUint32(uint32(len(p.Code)))
	for _, c := range p.Code {
		w.writeUint32(c)
	}

	// Constants
	w.writeUint32(uint32(len(p.Constants)))
	for _, c := range p.Constants {
		w.writeConstant(c)
	}

	// Nested protos
	w.writeUint32(uint32(len(p.FunctionPrototypes)))
	for _, np := range p.FunctionPrototypes {
		if err := w.writeProto(np); err != nil {
			return err
		}
	}

	// Debug info
	w.writeUint32(uint32(len(p.DbgSourcePositions)))
	for _, pos := range p.DbgSourcePositions {
		w.writeInt(pos)
	}

	w.writeUint32(uint32(len(p.DbgLocals)))
	for _, local := range p.DbgLocals {
		w.writeString(local.Name)
		w.writeInt(local.StartPc)
		w.writeInt(local.EndPc)
	}

	w.writeUint32(uint32(len(p.DbgCalls)))
	for _, call := range p.DbgCalls {
		w.writeString(call.Name)
		w.writeInt(call.Pc)
	}

	w.writeUint32(uint32(len(p.DbgUpvalues)))
	for _, uv := range p.DbgUpvalues {
		w.writeString(uv)
	}

	// TypeInfo (only for root proto)
	w.writeUint32(uint32(len(p.TypeInfo)))
	if len(p.TypeInfo) > 0 {
		_, w.err = w.w.Write(p.TypeInfo)
	}

	return w.err
}

func (w *writer) writeConstant(v lua.LValue) {
	if w.err != nil {
		return
	}

	switch val := v.(type) {
	case *lua.LNilType:
		w.writeByte(byte(lua.LTNil))
	case lua.LBool:
		w.writeByte(byte(lua.LTBool))
		if val {
			w.writeByte(1)
		} else {
			w.writeByte(0)
		}
	case lua.LNumber:
		w.writeByte(byte(lua.LTNumber))
		w.writeUint64(math.Float64bits(float64(val)))
	case lua.LInteger:
		w.writeByte(byte(lua.LTInteger))
		w.writeUint64(uint64(val))
	case lua.LString:
		w.writeByte(byte(lua.LTString))
		w.writeString(string(val))
	default:
		w.writeByte(byte(lua.LTNil))
	}
}

type reader struct {
	r   *bytes.Reader
	err error
}

func (r *reader) readByte() byte {
	if r.err != nil {
		return 0
	}
	b, err := r.r.ReadByte()
	if err != nil {
		r.err = err
		return 0
	}
	return b
}

func (r *reader) readUint32() uint32 {
	if r.err != nil {
		return 0
	}
	var v uint32
	r.err = binary.Read(r.r, binary.LittleEndian, &v)
	return v
}

func (r *reader) readUint64() uint64 {
	if r.err != nil {
		return 0
	}
	var v uint64
	r.err = binary.Read(r.r, binary.LittleEndian, &v)
	return v
}

func (r *reader) readInt() int {
	return int(r.readUint64())
}

func (r *reader) readString() string {
	if r.err != nil {
		return ""
	}
	length := r.readUint32()
	if length == 0 {
		return ""
	}
	data := make([]byte, length)
	_, r.err = io.ReadFull(r.r, data)
	return string(data)
}

func (r *reader) readProto() (*lua.FunctionProto, error) {
	if r.err != nil {
		return nil, r.err
	}

	p := &lua.FunctionProto{
		SourceName:       r.readString(),
		LineDefined:      r.readInt(),
		LastLineDefined:  r.readInt(),
		NumUpvalues:      r.readByte(),
		NumParameters:    r.readByte(),
		IsVarArg:         r.readByte(),
		NumUsedRegisters: r.readByte(),
	}

	// Code
	codeLen := r.readUint32()
	p.Code = make([]uint32, codeLen)
	for i := uint32(0); i < codeLen; i++ {
		p.Code[i] = r.readUint32()
	}

	// Constants
	constLen := r.readUint32()
	p.Constants = make([]lua.LValue, constLen)
	for i := uint32(0); i < constLen; i++ {
		p.Constants[i] = r.readConstant()
	}

	// Nested protos
	protoLen := r.readUint32()
	p.FunctionPrototypes = make([]*lua.FunctionProto, protoLen)
	for i := uint32(0); i < protoLen; i++ {
		np, err := r.readProto()
		if err != nil {
			return nil, err
		}
		p.FunctionPrototypes[i] = np
	}

	// Debug info
	posLen := r.readUint32()
	p.DbgSourcePositions = make([]int, posLen)
	for i := uint32(0); i < posLen; i++ {
		p.DbgSourcePositions[i] = r.readInt()
	}

	localLen := r.readUint32()
	p.DbgLocals = make([]*lua.DbgLocalInfo, localLen)
	for i := uint32(0); i < localLen; i++ {
		p.DbgLocals[i] = &lua.DbgLocalInfo{
			Name:    r.readString(),
			StartPc: r.readInt(),
			EndPc:   r.readInt(),
		}
	}

	callLen := r.readUint32()
	p.DbgCalls = make([]lua.DbgCall, callLen)
	for i := uint32(0); i < callLen; i++ {
		p.DbgCalls[i] = lua.DbgCall{
			Name: r.readString(),
			Pc:   r.readInt(),
		}
	}

	uvLen := r.readUint32()
	p.DbgUpvalues = make([]string, uvLen)
	for i := uint32(0); i < uvLen; i++ {
		p.DbgUpvalues[i] = r.readString()
	}

	// TypeInfo
	typeInfoLen := r.readUint32()
	if typeInfoLen > 0 {
		p.TypeInfo = make([]byte, typeInfoLen)
		_, r.err = io.ReadFull(r.r, p.TypeInfo)
	}

	if r.err != nil {
		return nil, ErrCorruptedBytecode
	}

	return p, nil
}

func (r *reader) readConstant() lua.LValue {
	if r.err != nil {
		return lua.LNil
	}

	typ := lua.LValueType(r.readByte())
	switch typ {
	case lua.LTNil:
		return lua.LNil
	case lua.LTBool:
		if r.readByte() == 1 {
			return lua.LTrue
		}
		return lua.LFalse
	case lua.LTNumber:
		return lua.LNumber(math.Float64frombits(r.readUint64()))
	case lua.LTInteger:
		return lua.LInteger(int64(r.readUint64()))
	case lua.LTString:
		return lua.LString(r.readString())
	default:
		return lua.LNil
	}
}

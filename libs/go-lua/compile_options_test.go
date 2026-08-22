package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/parse"
	typeio "github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCompileWithOptions_StringCastAndLib(t *testing.T) {
	source := `
		local s1 = string("x")
		local s2 = string.rep("y", 2)
		return s1, s2
	`
	chunk, err := parse.ParseString(source, "cast.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "cast.lua", CompileOptions{})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if !protoHasOp(proto, OP_LOADTYPE) {
		t.Fatalf("expected OP_LOADTYPE in bytecode")
	}

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 2, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	s1 := L.Get(-2)
	s2 := L.Get(-1)
	L.Pop(2)
	if s1 != LString("x") {
		t.Errorf("expected s1 to be %q, got %v", "x", s1)
	}
	if s2 != LString("yy") {
		t.Errorf("expected s2 to be %q, got %v", "yy", s2)
	}
}

func TestCompileWithOptions_TypeIsDotAndColon(t *testing.T) {
	source := `
		local ok1 = (Point:is({x = 1, y = 2})) ~= nil
		local ok2 = (Point.is({x = 1, y = 2})) ~= nil
		return ok1 and ok2
	`
	chunk, err := parse.ParseString(source, "type_is.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	manifest := typeio.NewManifest("type_is")
	pointType := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()
	manifest.DefineType("Point", pointType)
	data, err := typeio.EncodeManifest(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "type_is.lua", CompileOptions{TypeInfo: data})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if !protoHasOp(proto, OP_LOADTYPE) {
		t.Fatalf("expected OP_LOADTYPE in bytecode")
	}

	L := NewState()
	defer L.Close()
	OpenBase(L)
	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 1, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	result := L.Get(-1)
	L.Pop(1)
	if result != LTrue {
		t.Errorf("expected true result, got %v", result)
	}
}

func TestCompileWithOptions_LocalShadowDoesNotLoadType(t *testing.T) {
	source := `
		local string = function(x)
			return "ok:" .. x
		end
		return string("x")
	`
	chunk, err := parse.ParseString(source, "shadow.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "shadow.lua", CompileOptions{})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if protoHasOp(proto, OP_LOADTYPE) {
		t.Fatalf("did not expect OP_LOADTYPE in bytecode")
	}

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 1, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	result := L.Get(-1)
	L.Pop(1)
	if result != LString("ok:x") {
		t.Errorf("expected %q, got %v", "ok:x", result)
	}
}

func TestCompileWithOptions_TailcallTypeCast(t *testing.T) {
	source := `
		return string("ok")
	`
	chunk, err := parse.ParseString(source, "tailcall.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "tailcall.lua", CompileOptions{})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if !protoHasOp(proto, OP_LOADTYPE) {
		t.Fatalf("expected OP_LOADTYPE in bytecode")
	}

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 1, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	result := L.Get(-1)
	L.Pop(1)
	if result != LString("ok") {
		t.Errorf("expected %q, got %v", "ok", result)
	}
}

func TestCompileWithOptions_TopLevelTypeDefAddsTypeName(t *testing.T) {
	source := `
		type User = {name: string}
		local u = User({name = "x"})
		return u.name
	`
	chunk, err := parse.ParseString(source, "typedef.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "typedef.lua", CompileOptions{})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if !protoHasOp(proto, OP_LOADTYPE) {
		t.Fatalf("expected OP_LOADTYPE in bytecode")
	}
	if !protoHasTypeName(proto, "User") {
		t.Fatalf("expected type name %q in proto constants", "User")
	}
}

func protoHasOp(proto *FunctionProto, opcode int) bool {
	if proto == nil {
		return false
	}
	for _, inst := range proto.Code {
		if opGetOpCode(inst) == opcode {
			return true
		}
	}
	for _, child := range proto.FunctionPrototypes {
		if protoHasOp(child, opcode) {
			return true
		}
	}
	return false
}

func protoHasTypeName(proto *FunctionProto, name string) bool {
	if proto == nil || name == "" {
		return false
	}
	for _, value := range proto.stringConstants {
		if value == name {
			return true
		}
	}
	for _, child := range proto.FunctionPrototypes {
		if protoHasTypeName(child, name) {
			return true
		}
	}
	return false
}

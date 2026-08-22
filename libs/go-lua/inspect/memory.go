package inspect

import (
	"unsafe"

	lua "github.com/wippyai/go-lua"
)

const (
	ptrSize    = unsafe.Sizeof(uintptr(0))
	lvalueSize = 16 // any (interface) on 64-bit
)

// MemoryStats holds detailed memory usage statistics.
type MemoryStats struct {
	Tables       int64
	TableEntries int64
	Strings      int64
	StringBytes  int64
	Functions    int64
	UserData     int64
	Threads      int64
	Total        int64
}

// GetMemoryStats walks the Lua state and calculates memory usage.
func GetMemoryStats(L *lua.LState) MemoryStats {
	visited := make(map[uintptr]bool)
	stats := MemoryStats{}

	// Walk globals
	walkTable(L.G.Global, visited, &stats)

	// Walk registry
	walkTable(L.G.Registry, visited, &stats)

	// Walk main thread environment
	if L.Env != nil {
		walkTable(L.Env, visited, &stats)
	}

	stats.Total = stats.Tables + stats.Strings + stats.Functions + stats.UserData + stats.Threads

	return stats
}

// GetMemorySize returns the total estimated memory usage in bytes.
func GetMemorySize(L *lua.LState) int64 {
	return GetMemoryStats(L).Total
}

func walkTable(t *lua.LTable, visited map[uintptr]bool, stats *MemoryStats) {
	if t == nil {
		return
	}

	ptr := uintptr(unsafe.Pointer(t))
	if visited[ptr] {
		return
	}
	visited[ptr] = true

	// Base table struct size (approximate)
	stats.Tables += 80 // LTable struct overhead

	// Array part
	if len(t.Array) > 0 {
		stats.Tables += int64(cap(t.Array)) * lvalueSize
		stats.TableEntries += int64(len(t.Array))
		for _, v := range t.Array {
			walkValue(v, visited, stats)
		}
	}

	// Dict part (map[LValue]LValue)
	if len(t.Dict) > 0 {
		// Map overhead: ~8 bytes per bucket + header
		stats.Tables += int64(len(t.Dict)) * (lvalueSize*2 + 8)
		stats.TableEntries += int64(len(t.Dict))
		for k, v := range t.Dict {
			walkValue(k, visited, stats)
			walkValue(v, visited, stats)
		}
	}

	// Strdict part (map[string]LValue)
	if len(t.Strdict) > 0 {
		for k, v := range t.Strdict {
			stats.Tables += int64(len(k)) + 16 + lvalueSize + 8
			stats.TableEntries++
			walkValue(v, visited, stats)
		}
	}

	// Keys slice
	if len(t.Keys) > 0 {
		stats.Tables += int64(cap(t.Keys)) * lvalueSize
	}

	// K2i map
	if len(t.K2i) > 0 {
		stats.Tables += int64(len(t.K2i)) * (lvalueSize + 8 + 8)
	}

	// Metatable
	if mt, ok := t.Metatable.(*lua.LTable); ok {
		walkTable(mt, visited, stats)
	}
}

func walkValue(v lua.LValue, visited map[uintptr]bool, stats *MemoryStats) {
	if v == nil || v == lua.LNil {
		return
	}

	switch val := v.(type) {
	case lua.LString:
		stats.Strings += int64(len(val)) + 16 // string header
		stats.StringBytes += int64(len(val))

	case *lua.LTable:
		walkTable(val, visited, stats)

	case *lua.LFunction:
		ptr := uintptr(unsafe.Pointer(val))
		if visited[ptr] {
			return
		}
		visited[ptr] = true

		stats.Functions += 64 // LFunction struct
		if val.Proto != nil {
			stats.Functions += walkProto(val.Proto, visited)
		}
		if len(val.Upvalues) > 0 {
			stats.Functions += int64(len(val.Upvalues)) * 48
		}
		if val.Env != nil {
			walkTable(val.Env, visited, stats)
		}

	case *lua.LUserData:
		ptr := uintptr(unsafe.Pointer(val))
		if visited[ptr] {
			return
		}
		visited[ptr] = true
		stats.UserData += 32 // LUserData struct

	case *lua.LState:
		ptr := uintptr(unsafe.Pointer(val))
		if visited[ptr] {
			return
		}
		visited[ptr] = true
		stats.Threads += 256 // LState struct (approximate)
		if val.Env != nil {
			walkTable(val.Env, visited, stats)
		}
	}
}

func walkProto(p *lua.FunctionProto, visited map[uintptr]bool) int64 {
	if p == nil {
		return 0
	}

	ptr := uintptr(unsafe.Pointer(p))
	if visited[ptr] {
		return 0
	}
	visited[ptr] = true

	var size int64 = 64 // FunctionProto struct base

	// Code
	size += int64(cap(p.Code)) * 4

	// Constants
	size += int64(cap(p.Constants)) * lvalueSize
	for _, c := range p.Constants {
		if s, ok := c.(lua.LString); ok {
			size += int64(len(s)) + 16
		}
	}

	// Nested protos
	size += int64(cap(p.FunctionPrototypes)) * int64(ptrSize)
	for _, np := range p.FunctionPrototypes {
		size += walkProto(np, visited)
	}

	// Debug info
	size += int64(cap(p.DbgSourcePositions)) * 8
	size += int64(cap(p.DbgLocals)) * 32
	size += int64(cap(p.DbgCalls)) * 24
	size += int64(cap(p.DbgUpvalues)) * 16

	// Source name
	size += int64(len(p.SourceName)) + 16

	return size
}

// Package stdlib provides type definitions for Lua's standard library.
//
// This package defines typed signatures for all Lua built-in functions and
// library tables. These definitions enable the type checker to validate calls
// to standard library functions and infer result types.
//
// The library includes:
//   - Global functions: print, type, pairs, ipairs, assert, error, etc.
//   - Math library: math.floor, math.sin, math.random, etc.
//   - String library: string.sub, string.find, string.format, etc.
//   - Table library: table.insert, table.remove, table.sort, etc.
//   - Coroutine library: coroutine.create, coroutine.resume, etc.
//   - Debug library: debug.traceback, debug.getinfo, etc.
//   - UTF8 library: utf8.codes, utf8.len, etc.
//   - Package library: package.path, package.loaded, etc.
//   - Errors library: errors.new, errors.wrap, errors.is, etc.
//
// Usage:
//
//	lib := stdlib.Library()
//	mathType := lib["math"]
//	printType := lib["print"]
package stdlib

import "github.com/wippyai/go-lua/types/typ"

var library = initLibrary()

// Library returns the complete standard library type map.
// The returned map is shared and should not be modified.
func Library() map[string]typ.Type {
	return library
}

// initLibrary builds the standard library type map.
func initLibrary() map[string]typ.Type {
	return map[string]typ.Type{
		"assert":         Assert,
		"collectgarbage": CollectGarbage,
		"dofile":         DoFile,
		"error":          Error,
		"getmetatable":   GetMetatable,
		"ipairs":         Ipairs,
		"next":           Next,
		"pairs":          Pairs,
		"pcall":          Pcall,
		"cpcall":         Cpcall,
		"print":          Print,
		"rawequal":       RawEqual,
		"rawget":         RawGet,
		"rawlen":         RawLen,
		"rawset":         RawSet,
		"number":         Number,
		"integer":        Integer,
		"require":        Require,
		"select":         Select,
		"setmetatable":   SetMetatable,
		"tonumber":       ToNumber,
		"tostring":       ToString,
		"type":           Type,
		"unpack":         Unpack,
		"xpcall":         Xpcall,

		"math":      MathLib,
		"string":    StringLib,
		"table":     TableLib,
		"coroutine": CoroutineLib,
		"debug":     DebugLib,
		"utf8":      UTF8Lib,
		"package":   PackageLib,
		"errors":    ErrorsLib,

		"_G":       typ.Any,
		"_VERSION": typ.String,
	}
}

// Lookup resolves a standard library path to its type.
// Accepts both simple names ("print") and dotted paths ("string.upper").
// Returns nil if the path does not match any standard library entry.
func Lookup(path string) typ.Type {
	if t, ok := library[path]; ok {
		return t
	}

	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			base := path[:i]
			field := path[i+1:]
			if baseType, ok := library[base]; ok {
				if rec, ok := baseType.(*typ.Record); ok {
					if f := rec.GetField(field); f != nil {
						return f.Type
					}
				}
			}
			break
		}
	}

	return nil
}

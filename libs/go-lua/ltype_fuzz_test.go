package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/parse"
	typeio "github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// ---------------------------------------------------------------------------
// Fuzz 1: Corrupted manifest bytes → decode → validate
//
// This is the real attack surface: a corrupted or truncated manifest file
// is decoded, producing types that are then used for runtime validation.
// Must never panic — corrupted data must produce errors, not segfaults.
// ---------------------------------------------------------------------------

func FuzzManifestToValidation(f *testing.F) {
	// Seed with small valid manifests (one type each to keep size down)
	for _, entry := range []struct {
		name string
		typ  typ.Type
	}{
		{"Num", typ.Number},
		{"Pt", typ.NewRecord().Field("x", typ.Number).Build()},
		{"Opt", typ.NewOptional(typ.String)},
		{"Arr", typ.NewArray(typ.Number)},
	} {
		m := typeio.NewManifest("m")
		m.DefineType(entry.name, entry.typ)
		encoded, err := typeio.EncodeManifest(m)
		if err == nil && len(encoded) <= 512 {
			f.Add(encoded)
		}
	}

	testValues := []LValue{
		LNil, LTrue, LNumber(42), LInteger(7), LString("hello"),
		&LTable{},
		&LTable{Strdict: map[string]LValue{"x": LNumber(1), "y": LNumber(2)}},
		&LTable{Array: []LValue{LString("a"), LString("b")}},
		&LUserData{Value: "ud"},
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			return
		}

		manifest, err := typeio.DecodeManifest(data)
		if err != nil || manifest == nil {
			return
		}

		resolver := &typeResolver{path: manifest.Path, types: manifest.Types}

		for name, tp := range manifest.Types {
			if tp == nil {
				continue
			}
			lt := &LType{inner: tp, name: name, resolver: resolver}

			for _, val := range testValues {
				// Validate() must not panic
				lt.Validate(nil, val)
			}

			// :is() must not panic
			L := NewState()
			OpenErrors(L)
			isMethod := L.typeGetField(lt, "is")
			for _, val := range testValues {
				L.Push(isMethod)
				L.Push(val)
				L.Call(1, 2)
				L.Pop(2)
			}
			L.Close()
		}
	})
}

// ---------------------------------------------------------------------------
// Fuzz 2: Corrupted single-type bytes → decode → validate
//
// Feeds corrupted bytes to the single type decoder, then validates.
// Smaller surface than manifest but faster iteration.
// ---------------------------------------------------------------------------

func FuzzTypeDecodeToValidation(f *testing.F) {
	seeds := []typ.Type{
		typ.Number, typ.String, typ.Boolean,
		typ.NewOptional(typ.Number),
		typ.NewArray(typ.String),
		typ.NewMap(typ.String, typ.Number),
		typ.NewRecord().Field("x", typ.Number).OptField("y", typ.String).Build(),
		typ.NewUnion(typ.Number, typ.String),
		typ.LiteralString("active"),
		typ.NewInterface("table", nil),
		typ.NewTuple(typ.Number, typ.String),
		typ.NewAnnotated(typ.Number, []typ.Annotation{{Name: "min", Arg: float64(0)}}),
	}

	for _, seed := range seeds {
		data, err := typeio.Encode(seed)
		if err == nil {
			f.Add(data)
		}
	}
	f.Add([]byte{})
	f.Add([]byte{0xFF})

	testValues := []LValue{
		LNil, LTrue, LNumber(42), LString("x"),
		&LTable{},
		&LTable{Strdict: map[string]LValue{"x": LNumber(1)}},
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			return
		}

		decoded, err := typeio.Decode(data)
		if err != nil || decoded == nil {
			return
		}

		lt := &LType{inner: decoded}

		for _, val := range testValues {
			// Must not panic
			lt.Validate(nil, val)
		}

		// :is() path
		L := NewState()
		OpenErrors(L)
		isMethod := L.typeGetField(lt, "is")
		for _, val := range testValues {
			L.Push(isMethod)
			L.Push(val)
			L.Call(1, 2)
			L.Pop(2)
		}
		L.Close()
	})
}

// ---------------------------------------------------------------------------
// Fuzz 3: Lua source code with type declarations → compile → execute
//
// This tests the REAL pipeline: Lua source code goes through the parser
// and compiler, producing types that are used at runtime for validation.
// Must never panic — parse/compile errors are expected, panics are not.
// ---------------------------------------------------------------------------

func FuzzLuaTypeValidation(f *testing.F) {
	// Seed with real Lua code patterns that exercise runtime validation
	f.Add(`
		type Point = {x: number, y: number}
		local p, err = Point:is({x = 1, y = 2})
		assert(p ~= nil)
	`)
	f.Add(`
		type Status = "active" | "draft" | "archived"
		local s, err = Status:is("active")
	`)
	f.Add(`
		type Input = {
			id: string,
			name: string?,
			tags: {string}?,
			meta: table?,
		}
		local data = {id = "abc", name = "test"}
		local v, err = Input:is(data)
	`)
	f.Add(`
		type Pair = {first: number, second: string}
		local v, err = Pair:is({first = "bad"})
	`)
	f.Add(`
		type Config = {[string]: number}
		local c, err = Config:is({a = 1, b = 2})
	`)
	f.Add(`
		local val = string("hello")
	`)
	f.Add(`
		local ok = number(42)
	`)
	f.Add(`
		type Node = {value: number, next: Node?}
		local n = {value = 1, next = {value = 2}}
		local v, err = Node:is(n)
	`)

	f.Fuzz(func(t *testing.T, source string) {
		// Parse
		chunk, err := parse.ParseString(source, "fuzz.lua")
		if err != nil {
			return
		}

		// Compile with type resolution
		proto, err := CompileWithOptions(chunk, "fuzz.lua", CompileOptions{})
		if err != nil {
			return
		}

		// Execute — must never panic
		L := NewState()
		defer L.Close()
		OpenBase(L)
		OpenErrors(L)
		OpenString(L)

		fn := L.LoadProto(proto)
		L.Push(fn)
		_ = L.PCall(0, MultRet, nil) // runtime errors are OK, panics are not
	})
}

// ---------------------------------------------------------------------------
// Fuzz 4: Lua source with manifest-provided types → compile → execute
//
// Types come from a manifest (as in real module loading), and the Lua code
// uses :is() and Type(value) calls. Tests the manifest → runtime path.
// ---------------------------------------------------------------------------

func FuzzLuaWithManifestTypes(f *testing.F) {
	f.Add(`
		local v, err = Point:is({x = 1, y = 2})
		if v then return v.x + v.y end
		return nil, err
	`)
	f.Add(`local p = Point({x = 10, y = 20}); return p.x`)
	f.Add(`local v, err = Point:is("not a table"); return err`)
	f.Add(`local v, err = Point:is(nil); return v, err`)
	f.Add(`local v, err = Point:is({}); return v, err`)
	f.Add(`local v, err = Point:is({x = "bad", y = 2}); return v, err`)
	f.Add(`
		local v, err = Status:is("active")
		if not v then return nil, err end
		return v
	`)
	f.Add(`local v = Status:is("unknown"); return v`)
	f.Add(`
		local v, err = Input:is({id = "abc", name = "test", meta = {key = "val"}})
		return v, err
	`)
	f.Add(`local v, err = Input:is({id = 123}); return v, err`)
	f.Add(`local v, err = Input:is(nil); return v, err`)

	// Build a manifest with several types
	manifest := typeio.NewManifest("fuzz")
	manifest.DefineType("Point", typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build())
	manifest.DefineType("Status", typ.NewUnion(
		typ.LiteralString("active"),
		typ.LiteralString("draft"),
		typ.LiteralString("archived"),
	))
	manifest.DefineType("Input", typ.NewRecord().
		Field("id", typ.String).
		OptField("name", typ.String).
		OptField("tags", typ.NewArray(typ.String)).
		OptField("meta", typ.NewInterface("table", nil)).
		Build())
	manifestData, err := typeio.EncodeManifest(manifest)
	if err != nil {
		return
	}

	f.Fuzz(func(t *testing.T, source string) {
		chunk, err := parse.ParseString(source, "fuzz.lua")
		if err != nil {
			return
		}

		proto, err := CompileWithOptions(chunk, "fuzz.lua", CompileOptions{TypeInfo: manifestData})
		if err != nil {
			return
		}

		L := NewState()
		defer L.Close()
		OpenBase(L)
		OpenErrors(L)

		fn := L.LoadProto(proto)
		L.Push(fn)
		_ = L.PCall(0, MultRet, nil)
	})
}

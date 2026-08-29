package lualib

import (
	"reflect"
	"testing"

	lua "github.com/wippyai/go-lua"
)

func mkState(t *testing.T) *lua.LState {
	t.Helper()
	return lua.NewState()
}

func TestFromLuaScalar(t *testing.T) {
	if got := FromLua(lua.LString("hi")); got != "hi" {
		t.Fatalf("string = %#v", got)
	}
	if got := FromLua(lua.LInteger(42)); got != int64(42) {
		t.Fatalf("integer = %#v", got)
	}
	if got := FromLua(lua.LNumber(1.5)); got != 1.5 {
		t.Fatalf("number = %#v", got)
	}
	if got := FromLua(lua.LTrue); got != true {
		t.Fatalf("bool = %#v", got)
	}
	if got := FromLua(lua.LNil); got != nil {
		t.Fatalf("nil = %#v", got)
	}
}

func TestFromLuaArray(t *testing.T) {
	L := mkState(t)
	tbl := L.NewTable()
	tbl.RawSetInt(1, lua.LInteger(1))
	tbl.RawSetInt(2, lua.LInteger(2))
	tbl.RawSetInt(3, lua.LInteger(3))
	got := FromLua(tbl)
	want := []any{int64(1), int64(2), int64(3)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("array = %#v, want %#v", got, want)
	}
}

func TestFromLuaMap(t *testing.T) {
	L := mkState(t)
	tbl := L.NewTable()
	tbl.RawSetString("a", lua.LInteger(1))
	tbl.RawSetString("b", lua.LString("x"))
	got := FromLua(tbl).(map[string]any)
	if got["a"] != int64(1) || got["b"] != "x" {
		t.Fatalf("map = %#v", got)
	}
}

func TestFromLuaNested(t *testing.T) {
	L := mkState(t)
	inner := L.NewTable()
	inner.RawSetInt(1, lua.LString("p"))
	outer := L.NewTable()
	outer.RawSetString("items", inner)
	outer.RawSetString("n", lua.LInteger(2))
	got := FromLua(outer).(map[string]any)
	if !reflect.DeepEqual(got["items"], []any{"p"}) {
		t.Fatalf("nested items = %#v", got["items"])
	}
	if got["n"] != int64(2) {
		t.Fatalf("nested n = %#v", got["n"])
	}
}

func TestToLuaRoundTrip(t *testing.T) {
	L := mkState(t)
	in := map[string]any{
		"query": "dsc",
		"n":     int64(3),
		"tags":  []any{"a", "b"},
	}
	back := FromLua(ToLua(L, in)).(map[string]any)
	if back["query"] != "dsc" || back["n"] != int64(3) ||
		!reflect.DeepEqual(back["tags"], []any{"a", "b"}) {
		t.Fatalf("round-trip mismatch: %#v", back)
	}
}

func TestToLuaIntegralFloatIsInteger(t *testing.T) {
	L := mkState(t)
	if v := ToLua(L, 3.0); v != lua.LInteger(3) {
		t.Fatalf("3.0 should become integer, got %#v", v)
	}
	if v := ToLua(L, 3.5); v != lua.LNumber(3.5) {
		t.Fatalf("3.5 should stay float, got %#v", v)
	}
}

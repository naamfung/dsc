// Package lualib 提供 go-lua 值与 Go/JSON 值之间的转换，供 coderuntime 与
// workflow 共用（避免在多处复制同一套 Lua↔JSON 转换逻辑）。
package lualib

import (
	"encoding/json"
	"fmt"
	"math"

	lua "github.com/wippyai/go-lua"
)

// FromLua 把 LValue 转成 JSON 兼容的 Go 值。
// 表全部键为正整数时按最大下标生成 []any，否则生成 map[string]any。
func FromLua(v lua.LValue) any {
	switch t := v.(type) {
	case *lua.LTable:
		return tableToGo(t)
	case lua.LString:
		return string(t)
	case lua.LNumber:
		return float64(t)
	case lua.LInteger:
		return int64(t)
	case lua.LBool:
		return bool(t)
	default:
		if v == lua.LNil {
			return nil
		}
		return v.String()
	}
}

// ToLua 把 Go 值转成 LValue（整数/浮点/字符串/布尔/表/其余按 JSON 物化）。
func ToLua(L *lua.LState, v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		if t {
			return lua.LTrue
		}
		return lua.LFalse
	case string:
		return lua.LString(t)
	case int:
		return lua.LInteger(t)
	case int64:
		return lua.LInteger(t)
	case float64:
		if i, ok := integral(t); ok {
			return lua.LInteger(i)
		}
		return lua.LNumber(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return lua.LInteger(i)
		}
		if f, err := t.Float64(); err == nil {
			return lua.LNumber(f)
		}
		return lua.LString(t.String())
	case []any:
		tbl := L.NewTable()
		for i, e := range t {
			tbl.RawSetInt(i+1, ToLua(L, e))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k, e := range t {
			tbl.RawSetString(k, ToLua(L, e))
		}
		return tbl
	default:
		// 其余类型：按 JSON 走（struct -> 表），失败退回字符串。
		if b, err := json.Marshal(t); err == nil {
			var j any
			if json.Unmarshal(b, &j) == nil {
				return ToLua(L, j)
			}
			return lua.LString(string(b))
		}
		return lua.LString(fmt.Sprintf("%v", t))
	}
}

// tableToGo 把 Lua 表转成 Go 值：全部键为正整数（LInteger 或整数值 LNumber）
// → 按最大下标生成 []any；否则 → map[string]any。
func tableToGo(t *lua.LTable) any {
	isArray := true
	arrLen := 0
	t.ForEach(func(k, _ lua.LValue) {
		n, ok := intKey(k)
		if !ok || n < 1 {
			isArray = false
			return
		}
		if n > arrLen {
			arrLen = n
		}
	})
	if isArray && arrLen > 0 {
		out := make([]any, arrLen)
		for i := 1; i <= arrLen; i++ {
			if rv := t.RawGetInt(i); rv != lua.LNil {
				out[i-1] = FromLua(rv)
			}
		}
		return out
	}
	out := make(map[string]any, 4)
	t.ForEach(func(k, v lua.LValue) { out[k.String()] = FromLua(v) })
	return out
}

// intKey 把整数下标 key（LInteger 或整数值 LNumber）归一成 int。
func intKey(k lua.LValue) (int, bool) {
	switch t := k.(type) {
	case lua.LInteger:
		return int(int64(t)), true
	case lua.LNumber:
		f := float64(t)
		if f != float64(int(f)) {
			return 0, false
		}
		return int(f), true
	}
	return 0, false
}

// integral 若浮点可精确表示整数则返回其整数值。
func integral(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if math.Trunc(f) != f {
		return 0, false
	}
	return int64(f), true
}

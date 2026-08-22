package generics

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestGenericFunctions tests generic function definitions and inference.
func TestGenericFunctions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "identity instantiation with literal",
			Code: `
				local function identity<T>(x: T): T
					return x
				end
				local s = identity("hello")
				local n = identity(42)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "identity preserves record fields",
			Code: `
				local function identity<T>(x: T): T
					return x
				end
				local p = identity({x = 1, y = 2})
				local sum = p.x + p.y
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "identity result assignable to typed variable",
			Code: `
				local function identity<T>(x: T): T
					return x
				end
				local s: string = identity("test")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "pair instantiation with two types",
			Code: `
				local function pair<K, V>(k: K, v: V): {key: K, value: V}
					return {key = k, value = v}
				end
				local p = pair("name", 42)
				local k = p.key
				local v = p.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "pair result assignable to compatible record",
			Code: `
				local function pair<K, V>(k: K, v: V): {key: K, value: V}
					return {key = k, value = v}
				end
				local p: {key: string, value: integer} = pair("count", 100)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typed first returns element type",
			Code: `
				local function first<T>(...: T): T
					return select(1, ...)
				end
				local n = first(1, 2, 3)
				local sum = n + 1
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGenericInference tests generic type inference scenarios.
func TestGenericInference(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "map function infers T from callback return",
			Code: `
				local function map<T, U>(arr: {T}, fn: (T) -> U): {U}
					local result: {U} = {}
					for i, v in ipairs(arr) do
						result[i] = fn(v)
					end
					return result
				end
				local nums = {1, 2, 3}
				local strs = map(nums, tostring)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic with inline function",
			Code: `
				local function transform<T, U>(x: T, fn: (T) -> U): U
					return fn(x)
				end
				local n = transform("hello", function(s) return #s end)
				local len: integer = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic inline function uses inferred param for arithmetic",
			Code: `
				local function apply<T, U>(x: T, fn: (T) -> U): U
					return fn(x)
				end
				local n = apply(10, function(x) return x + 1 end)
				local out: integer = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic inline function uses inferred param for field access",
			Code: `
				local function apply<T, U>(x: T, fn: (T) -> U): U
					return fn(x)
				end
				local n = apply({count = 1}, function(r) return r.count end)
				local out: integer = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "get function infers K and V from table",
			Code: `
				local function get<K, V>(t: {[K]: V}, k: K): V?
					return t[k]
				end
				local ages: {[string]: integer} = {["alice"] = 30, ["bob"] = 25}
				local age = get(ages, "alice")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "set function infers K and V from arguments",
			Code: `
				local function set<K, V>(t: {[K]: V}, k: K, v: V)
					t[k] = v
				end
				local scores: {[string]: number} = {}
				set(scores, "test", 95.5)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "keys function returns array of K",
			Code: `
				local function keys<K, V>(t: {[K]: V}): {K}
					local result: {K} = {}
					for k in pairs(t) do
						result[#result + 1] = k
					end
					return result
				end
				local data: {[string]: number} = {["a"] = 1, ["b"] = 2}
				local ks = keys(data)
				local first: string = ks[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "wrap function creates container with T",
			Code: `
				type Box<T> = {value: T}
				local function wrap<T>(x: T): Box<T>
					return {value = x}
				end
				local box = wrap(42)
				local n: integer = box.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "unwrap function extracts T from container",
			Code: `
				type Box<T> = {value: T}
				local function unwrap<T>(box: Box<T>): T
					return box.value
				end
				local box: Box<string> = {value = "hello"}
				local s: string = unwrap(box)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic container with single type param",
			Code: `
				type Container<T> = {value: T}
				local function wrap<T>(value: T): Container<T>
					return {value = value}
				end
				local c = wrap("hello")
				local s: string = c.value
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestAnyPropagation tests that any type propagates correctly.
func TestAnyPropagation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "integer cast assigned to typed variable",
			Code: `
				local x: any = 100
				local n: integer = integer(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast passed to typed function",
			Code: `
				local function double(n: integer): integer
					return n * 2
				end
				local x: any = 5
				local result = double(integer(x))
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast in arithmetic",
			Code: `
				local x: any = 100
				local n = integer(x) + 50
				local m: integer = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast in comparison",
			Code: `
				local x: any = 100
				local cmp = integer(x) > 50
				local b: boolean = cmp
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast return from function",
			Code: `
				local function parse(s: any): integer
					return integer(s)
				end
				local n: integer = parse("42")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast in table field",
			Code: `
				local x: any = 100
				local t: {count: integer} = {count = integer(x)}
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast chained with tostring",
			Code: `
				local x: any = 100
				local s: string = tostring(integer(x))
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number cast assigned to typed variable",
			Code: `
				local x: any = "3.14"
				local n: number = number(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number cast in arithmetic",
			Code: `
				local x: any = "100"
				local n = number(x) * 2.5
				local m: number = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tostring result assigned to string variable",
			Code: `
				local x: any = 42
				local s: string = tostring(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tostring result in concatenation",
			Code: `
				local x: any = 42
				local s: string = "value: " .. tostring(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

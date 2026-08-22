package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAnyPropagation_IntegerCast(t *testing.T) {
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
	}
	testutil.RunCases(t, tests)
}

func TestAnyPropagation_NumberCast(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "number cast from any",
			Code: `
				local x: any = 3.14
				local n = number(x)
				local sum = n + 1.0
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number cast assigned to typed variable",
			Code: `
				local x: any = 3.14
				local n: number = number(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestAnyPropagation_TostringReturn(t *testing.T) {
	tests := []testutil.Case{
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

func TestGenericInference_Identity(t *testing.T) {
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
	}
	testutil.RunCases(t, tests)
}

func TestGenericInference_Pair(t *testing.T) {
	tests := []testutil.Case{
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
	}
	testutil.RunCases(t, tests)
}

func TestGenericInference_MapFunction(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "generic map function preserves element type",
			Code: `
				local function map<T, U>(arr: {T}, fn: (T) -> U): {U}
					local result: {U} = {}
					for i, v in ipairs(arr) do
						result[i] = fn(v)
					end
					return result
				end
				local doubled = map({1, 2, 3}, function(x: number): number return x * 2 end)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic filter function",
			Code: `
				local function filter<T>(arr: {T}, pred: (T) -> boolean): {T}
					local result: {T} = {}
					for _, v in ipairs(arr) do
						if pred(v) then
							table.insert(result, v)
						end
					end
					return result
				end
				local evens = filter({1, 2, 3, 4}, function(x: integer): boolean return x % 2 == 0 end)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestGenericInference_TableOperations(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "generic find function returns optional",
			Code: `
				local function find<T>(arr: {T}, pred: (T) -> boolean): T?
					for _, v in ipairs(arr) do
						if pred(v) then
							return v
						end
					end
					return nil
				end
				local found = find({1, 2, 3}, function(x: integer): boolean return x > 2 end)
				if found then
					local n: integer = found
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic reduce function",
			Code: `
				local function reduce<T, U>(arr: {T}, init: U, fn: (U, T) -> U): U
					local acc = init
					for _, v in ipairs(arr) do
						acc = fn(acc, v)
					end
					return acc
				end
				local sum = reduce({1, 2, 3}, 0, function(acc: integer, x: integer): integer return acc + x end)
				local total: integer = sum
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestGenericInference_NestedGenerics(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "nested generic type instantiation",
			Code: `
				type Result<T> = {ok: boolean, value: T}
				local function wrap<T>(v: T): Result<T>
					return {ok = true, value = v}
				end
				local r = wrap("hello")
				local v: string = r.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic function with generic type parameter",
			Code: `
				type Option<T> = T | nil
				local function unwrap<T>(opt: Option<T>, default: T): T
					if opt then
						return opt
					end
					return default
				end
				local value = unwrap("hello", "default")
				local s: string = value
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestAnyPropagation_UntypedParameterToTypedFunction tests passing an untyped
// function parameter to a function expecting a concrete type.
// This reproduces the wippy false positive where process.send(parent_pid, ...)
// fails because parent_pid is untyped (any) but process.send expects string.
func TestAnyPropagation_UntypedParameterToTypedFunction(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "untyped parameter passed to string expecting function",
			Code: `
				local function send_message(target: string, topic: string, payload: any)
				end

				local function worker(parent_pid)
					-- parent_pid is any (untyped parameter)
					-- send_message expects string
					send_message(parent_pid, "response", "data")
				end
			`,
			WantError: false, // Untyped params are inferred from usage in typed calls
			Stdlib:    true,
		},
		{
			Name: "typed parameter passed to string expecting function",
			Code: `
				local function send_message(target: string, topic: string, payload: any)
				end

				local function worker(parent_pid: string)
					send_message(parent_pid, "response", "data")
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "any variable with type annotation passed",
			Code: `
				local function send_message(target: string, topic: string, payload: any)
				end

				local function worker(parent_pid: any)
					local pid: string = tostring(parent_pid)
					send_message(pid, "response", "data")
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

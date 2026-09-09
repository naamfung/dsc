package types

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestTypeCasts tests type cast expressions.
func TestTypeCasts(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string cast",
			Code: `
				local x: any = "hello"
				local s = string(x)
				local len = #s
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast",
			Code: `
				local x: any = 42
				local n = integer(x)
				local sum = n + 1
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number cast",
			Code: `
				local x: any = 3.14
				local n = number(x)
				local sum = n + 1.0
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "boolean cast",
			Code: `
				local x: any = true
				local b = boolean(x)
				if b then
					return 1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string library still works",
			Code: `
				local s = "hello"
				local upper = string.upper(s)
				local lower = string.lower(s)
				local len = string.len(s)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "both cast and library",
			Code: `
				local x: any = "hello"
				local s = string(x)
				local upper = string.upper(s)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "chained cast",
			Code: `
				local data: any = { name = "test", count = 42 }
				local name = string(data.name)
				local count = integer(data.count)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "pass to typed function",
			Code: `
				local function greet(name: string): string
					return "Hello, " .. name
				end
				local x: any = "World"
				local result = greet(string(x))
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "custom type cast",
			Code: `
				type Point = {x: number, y: number}
				local v: any = {x = 1, y = 2}
				local p = Point(v)
				local sum = p.x + p.y
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "alias types int and bool",
			Code: `
				local n = int(42)
				local b = bool(true)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested record type cast",
			Code: `
				type Address = {street: string, city: string}
				type Person = {name: string, address: Address}
				local data: any = {name = "Alice", address = {street = "123 Main", city = "NYC"}}
				local p = Person(data)
				local name = p.name
				local city = p.address.city
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "cast in return",
			Code: `
				local function getName(data: any): string
					return string(data)
				end
				local result = getName("test")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "cast in arithmetic",
			Code: `
				local a: any = 10
				local b: any = 20
				local sum = integer(a) + integer(b)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "cast in concat",
			Code: `
				local prefix: any = "Hello, "
				local name: any = "World"
				local greeting = string(prefix) .. string(name)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple casts in statement",
			Code: `
				local data: any = {a = "1", b = 2, c = true}
				local s, n, b = string(data.a), integer(data.b), boolean(data.c)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "cast in table constructor",
			Code: `
				type Config = {name: string, count: integer}
				local raw: any = {name = "test", count = 42}
				local config = {
					name = string(raw.name),
					count = integer(raw.count)
				}
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic record cast",
			Code: `
				type Result<T> = {ok: boolean, value: T}
				local data: any = {ok = true, value = "success"}
				type StringResult = {ok: boolean, value: string}
				local r = StringResult(data)
				local v = r.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "array type cast",
			Code: `
				type Numbers = {integer}
				local data: any = {1, 2, 3}
				local nums = Numbers(data)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "cast from method return",
			Code: `
				local obj = {
					getData = function(self): any
						return "value"
					end
				}
				local value = string(obj:getData())
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTypeIsMethod tests Type:is narrowing method.
func TestTypeIsMethod(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "Type:is narrows",
			Code: `
				type Point = {x: number, y: number}
				local v: any = {x = 1, y = 2}
				local p, err = Point:is(v)
				if err == nil then
					local q: {x: number, y: number} = v
					local sum = q.x + q.y
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is with error check",
			Code: `
				type Point = {x: number, y: number}
				local v: any = "not a point"
				local p, err = Point:is(v)
				if err == nil then
					local q: {x: number, y: number} = p
					local sum = q.x + q.y
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestTypeCallNarrowing tests that Type(x) narrows the type of x.
func TestTypeCallNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "Type call wrapper narrows argument",
			Code: `
				type Point = {x: number, y: number}
				local function expectPoint(x)
					return Point(x)
				end
				function validate(data: any)
					expectPoint(data)
					local p: {x: number, y: number} = data
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is assert guard narrows argument",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					local _, err = Point:is(data)
					if err ~= nil then
						error("no")
					end
					local p: {x: number, y: number} = data
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows argument",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					Point(data)
					local p: {x: number, y: number} = data
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows argument in assignment",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					local v = Point(data)
					local p: {x: number, y: number} = data
					local q: {x: number, y: number} = v
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call assignment narrows both",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					local p = Point(data)
					local q: {x: number, y: number} = p
					local r: {x: number, y: number} = data
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows field path",
			Code: `
				type Point = {x: number, y: number}
				function validate(obj: {p: any})
					Point(obj.p)
					local p: {x: number, y: number} = obj.p
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows bracket field path",
			Code: `
				type Point = {x: number, y: number}
				function validate(obj: {["p-q"]: any})
					Point(obj["p-q"])
					local p: {x: number, y: number} = obj["p-q"]
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows const key",
			Code: `
				type Point = {x: number, y: number}
				function validate(obj: {["p-q"]: any})
					local key = "p-q"
					Point(obj[key])
					local p: {x: number, y: number} = obj[key]
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows const index",
			Code: `
				type Point = {x: number, y: number}
				function validate(obj: {[integer]: any})
					local key = 1
					Point(obj[key])
					local p: {x: number, y: number} = obj[key]
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows in numeric for",
			Code: `
				type Point = {x: number, y: number}
				function validate(xs: {[integer]: any})
					for i = 1, 10 do
						local v = xs[i]
						Point(v)
						local p: {x: number, y: number} = v
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows in while with break",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					while true do
						local v = Point(data)
						local p: {x: number, y: number} = v
						break
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows in generic for ipairs",
			Code: `
				type Point = {x: number, y: number}
				function validate(xs: {[integer]: any})
					for i, v in ipairs(xs) do
						Point(v)
						local p: {x: number, y: number} = v
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows in generic for pairs",
			Code: `
				type Point = {x: number, y: number}
				function validate(xs: {[string]: any})
					for k, v in pairs(xs) do
						Point(v)
						local p: {x: number, y: number} = v
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call narrows in while with early skip",
			Code: `
				type Point = {x: number, y: number}
				function validate(xs: {[integer]: any})
					local i = 1
					while i <= 3 do
						local v = xs[i]
						if not v then
							i = i + 1
						else
							Point(v)
							local p: {x: number, y: number} = v
							i = i + 1
						end
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type call without narrowing fails",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					local p: {x: number, y: number} = data
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTypeofNarrowing_Extended tests additional type() narrowing patterns.
func TestTypeofNarrowing_Extended(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "type() ~= string narrows to other",
			Code: `
				function f(x: string | number)
					if type(x) ~= "string" then
						local n: number = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() == table narrows",
			Code: `
				function f(x: {x: number} | string)
					if type(x) == "table" then
						local t: {x: number} = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() == function narrows",
			Code: `
				function f(x: (() -> number) | string)
					if type(x) == "function" then
						local fn: () -> number = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() == boolean narrows",
			Code: `
				function f(x: boolean | string)
					if type(x) == "boolean" then
						local b: boolean = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() == nil narrows",
			Code: `
				function f(x: string?)
					if type(x) == "nil" then
						local n: nil = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() narrows else branch",
			Code: `
				function f(x: string | number)
					if type(x) == "string" then
						local s: string = x
					else
						local n: number = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() no narrowing without check",
			Code: `
				function f(x: string | number)
					local s: string = x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "type() early return pattern",
			Code: `
				function f(x: string | number): string
					if type(x) ~= "string" then
						return ""
					end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() with and condition",
			Code: `
				function f(x: string | number, flag: boolean)
					if type(x) == "string" and flag then
						local s: string = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type() nested checks",
			Code: `
				function f(x: string | number | boolean)
					if type(x) == "string" then
						local s: string = x
					elseif type(x) == "number" then
						local n: number = x
					else
						local b: boolean = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestOrFallbackPattern tests the x or default pattern.
func TestOrFallbackPattern(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "or fallback with nil",
			Code: `
				local x: number? = nil
				local y: number = x or 0
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "or fallback chain",
			Code: `
				local a: number? = nil
				local b: string? = nil
				local result: number | string = a or b or "default"
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "or fallback narrows optional",
			Code: `
				function f(x: number?)
					local y = x or 0
					local n: number = y
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "or fallback in arithmetic",
			Code: `
				function f(x: number?): number
					return (x or 0) + 1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "or fallback without default fails",
			Code: `
				function f(x: number?): number
					return x + 1
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "and-or ternary pattern",
			Code: `
				function f(cond: boolean, a: number, b: number): number
					return cond and a or b
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string.match or default",
			Code: `
				local s: string = string.match("hello", "h") or "default"
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestEarlyReturnNarrowing tests guard clause patterns.
func TestEarlyReturnNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "return if nil narrows",
			Code: `
				function f(x: string?): string
					if x == nil then
						return ""
					end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "return if not narrows else",
			Code: `
				function f(x: string?): string
					if x ~= nil then
						return x
					end
					return ""
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "guard clause pattern",
			Code: `
				function process(data: {value: number}?)
					if not data then return end
					local v: number = data.value
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple guard clauses",
			Code: `
				function process(a: string?, b: number?)
					if not a then return end
					if not b then return end
					local s: string = a
					local n: number = b
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "error return pattern",
			Code: `
				function process(x: number?): (number, string?)
					if x == nil then
						return 0, "x is nil"
					end
					return x * 2, nil
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

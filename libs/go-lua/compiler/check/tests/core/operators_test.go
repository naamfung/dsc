package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestOperators tests all Lua operators.
func TestOperators(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "arithmetic operators",
			Code: `
				local a: number = 1 + 2 - 3 * 4 / 5 % 6 ^ 2
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "comparison operators",
			Code: `
				local a: boolean = 1 < 2
				local b: boolean = 2 <= 2
				local c: boolean = 3 > 2
				local d: boolean = 3 >= 3
				local e: boolean = 1 == 1
				local f: boolean = 1 ~= 2
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string concatenation",
			Code: `
				local s: string = "hello" .. " " .. "world"
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "length operator",
			Code: `
				local arr = {1, 2, 3}
				local n: integer = #arr
				local s = "hello"
				local m: integer = #s
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "unary operators",
			Code: `
				local x: number = -5
				local b: boolean = not true
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "bitwise operators",
			Code: `
				local a: integer = 5 & 3
				local b: integer = 5 | 3
				local c: integer = 5 ~ 3
				local d: integer = ~5
				local e: integer = 5 << 2
				local f: integer = 20 >> 2
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "floor division",
			Code: `
				local x: integer = 7 // 3
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestErrorDetection tests that type errors are caught.
func TestErrorDetection(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "type mismatch in assignment",
			Code:      `local x: number = "string"`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name:      "undefined variable",
			Code:      `local x: number = undefinedVar`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "wrong argument type",
			Code: `
				local function foo(x: number): number return x end
				local result = foo("string")
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "accessing undefined field",
			Code: `
				local obj = {x = 1}
				local y: number = obj.y
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "wrong return type",
			Code: `
				local function foo(): number
					return "not a number"
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "arithmetic on string",
			Code: `
				local s: string = "hello"
				local x = s + 1
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "index non-indexable",
			Code: `
				local x: number = 42
				local y = x[1]
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "call non-callable",
			Code: `
				local x: number = 42
				local y = x()
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "too few arguments",
			Code: `
				local function foo(a: number, b: number): number
					return a + b
				end
				local x = foo(1)
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name:      "duplicate local names",
			Code:      `local x, x = 1, 2`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "arithmetic on optional number",
			Code: `
				local function f(x: number?): number
					return x + 1
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "unary minus on string",
			Code: `
				local function f(x: string): number
					return -x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "bitwise not on string",
			Code: `
				local function f(x: string): integer
					return ~x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "length on number",
			Code: `
				local function f(x: number): integer
					return #x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "string concat on boolean",
			Code: `
				local function f(x: boolean): string
					return x .. "ok"
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "less than on boolean",
			Code: `
				local function f(a: boolean, b: boolean): boolean
					return a < b
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "less than on table",
			Code: `
				local function f(a: {}, b: {}): boolean
					return a < b
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

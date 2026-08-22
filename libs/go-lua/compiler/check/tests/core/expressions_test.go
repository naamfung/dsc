package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestExpressions_OrFallback(t *testing.T) {
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

func TestExpressions_Operators(t *testing.T) {
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

func TestExpressions_EarlyReturn(t *testing.T) {
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

func TestExpressions_MultipleReturns(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "multi return assignment",
			Code: `
				local function divmod(a: number, b: number): (number, number)
					return math.floor(a / b), a % b
				end
				local q, r = divmod(10, 3)
				local qn: number = q
				local rn: number = r
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "single var from multi return",
			Code: `
				local function getValues(): (number, string)
					return 42, "hello"
				end
				local x = getValues()
				local n: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "discard with underscore",
			Code: `
				local function getValues(): (number, string, boolean)
					return 1, "a", true
				end
				local a, _, c = getValues()
				local n: number = a
				local b: boolean = c
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multi return in expression",
			Code: `
				local function getNum(): (number, string)
					return 5, "five"
				end
				local x: number = getNum() + 1
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestExpressions_HigherOrder(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "map function",
			Code: `
				local function map(arr: {number}, fn: (number) -> number): {number}
					local result: {number} = {}
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
			Name: "callback with closure",
			Code: `
				local function createCounter(): () -> number
					local count = 0
					return function(): number
						count = count + 1
						return count
					end
				end
				local counter = createCounter()
				local first: number = counter()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function composition",
			Code: `
				local function compose(f: (number) -> number, g: (number) -> number): (number) -> number
					return function(x: number): number
						return f(g(x))
					end
				end
				local function double(x: number): number return x * 2 end
				local function addOne(x: number): number return x + 1 end
				local composed = compose(double, addOne)
				local result: number = composed(5)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "curried function",
			Code: `
				local function createAdder(x: number): (number) -> number
					return function(y: number): number
						return x + y
					end
				end
				local add5 = createAdder(5)
				local result: number = add5(3)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestExpressions_Pcall(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "pcall returns boolean and results",
			Code: `
				local ok, result = pcall(function() return 42 end)
				local b: boolean = ok
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "pcall with args",
			Code: `
				local ok, result = pcall(tostring, 42)
				local b: boolean = ok
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "xpcall with handler",
			Code: `
				local ok, result = xpcall(function() return "test" end, function(err) return err end)
				local b: boolean = ok
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestExpressions_Variadic(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "simple variadic",
			Code: `
				local function sum(...: number): number
					local result = 0
					for _, v in ipairs({...}) do
						result = result + v
					end
					return result
				end
				local total: number = sum(1, 2, 3)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "variadic with fixed params",
			Code: `
				local function printf(fmt: string, ...: any)
					print(string.format(fmt, ...))
				end
				printf("Hello %s", "world")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "select with variadic",
			Code: `
				local function test(...: number)
					local count = select("#", ...)
					local first = select(1, ...)
				end
				test(1, 2, 3)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestExpressions_LiteralTypes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string literal type",
			Code: `
				local status: "ok" | "error" = "ok"
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "literal in condition",
			Code: `
				local status: "ok" | "error" = "ok"
				if status == "ok" then
					local msg: string = "success"
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "literal type mismatch",
			Code: `
				local status: "ok" | "error" = "unknown"
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "boolean literal",
			Code: `
				local enabled: true = true
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number literal type",
			Code: `
				local code: 200 | 404 | 500 = 200
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestExpressions_Recursion(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "simple recursion",
			Code: `
				local function factorial(n: number): number
					if n <= 1 then return 1 end
					return n * factorial(n - 1)
				end
				local result: number = factorial(5)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "mutual recursion",
			Code: `
				local function isEven(n: number): boolean
					if n == 0 then return true end
					return isOdd(n - 1)
				end
				local function isOdd(n: number): boolean
					if n == 0 then return false end
					return isEven(n - 1)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestExpressions_DeepNesting(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "deep table nesting",
			Code: `
				local deep = {a={b={c={d={e={value=42}}}}}}
				local x: number = deep.a.b.c.d.e.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "deep conditional",
			Code: `
				local function classify(a: boolean, b: boolean, c: boolean): number
					if a then
						if b then
							if c then return 1 else return 2 end
						else return 3 end
					else return 4 end
				end
				local r: number = classify(true, true, false)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "variable shadowing in nested scope",
			Code: `
				local x = 10
				do
					local x = "hello"
					local s: string = x
				end
				local n: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestExpressions_GotoLabel(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "simple goto forward",
			Code: `
				goto target
				::target::
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "goto backward",
			Code: `
				::start::
				goto start
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "goto undefined label",
			Code: `
				goto undefined
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "duplicate label",
			Code: `
				::dup::
				::dup::
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "break outside loop",
			Code: `
				break
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "break in function inside loop",
			Code: `
				while true do
					local f = function() break end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

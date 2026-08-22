package types

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestLiteralTypes tests literal type annotations and narrowing.
func TestLiteralTypes(t *testing.T) {
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

// TestLiteralWidening tests that boolean literals widen correctly.
func TestLiteralWidening(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "record with boolean false can be set to true",
			Code: `
				local received = { inbox = false, custom = false, cancel = false }
				received.inbox = true
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "record with boolean true can be set to false",
			Code: `
				local flags = { enabled = true, visible = true }
				flags.enabled = false
				flags.visible = false
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "record with mixed boolean literals",
			Code: `
				local state = { active = false, pending = true }
				state.active = true
				state.pending = false
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested record with boolean fields",
			Code: `
				local config = {
					flags = { enabled = false, debug = false }
				}
				config.flags.enabled = true
				config.flags.debug = true
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "boolean field in loop mutation",
			Code: `
				local received = { inbox = false, custom = false, cancel = false }
				for i = 1, 3 do
					received.inbox = true
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "boolean field conditional assignment",
			Code: `
				local state = { done = false }
				if true then
					state.done = true
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple boolean fields progressive mutation",
			Code: `
				local received = { inbox = false, custom = false, cancel = false }
				received.inbox = true
				received.custom = true
				received.cancel = true
				local a: boolean = received.inbox
				local b: boolean = received.custom
				local c: boolean = received.cancel
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "boolean field used in condition after mutation",
			Code: `
				local state = { ready = false }
				state.ready = true
				if state.ready then
					local x = 1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestDeepNesting tests deeply nested structures and control flow.
func TestDeepNesting(t *testing.T) {
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
			Name: "variable shadowing",
			Code: `
				local x: number = 1
				do
					local x: string = "a"
					do
						local x: boolean = true
					end
				end
				local final: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestMultipleReturns tests functions that return multiple values.
func TestMultipleReturns(t *testing.T) {
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

// TestRecursion tests recursive function definitions.
func TestRecursion(t *testing.T) {
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
				local isEven: (number) -> boolean
				local isOdd: (number) -> boolean
				isEven = function(n: number): boolean
					if n == 0 then return true end
					return isOdd(n - 1)
				end
				isOdd = function(n: number): boolean
					if n == 0 then return false end
					return isEven(n - 1)
				end
				local result: boolean = isEven(4)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tail recursion",
			Code: `
				local function sum(n: number, acc: number): number
					if n == 0 then return acc end
					return sum(n - 1, acc + n)
				end
				local result: number = sum(10, 0)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

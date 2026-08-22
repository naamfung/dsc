package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestEffects_AssertNotNil tests that assert functions narrow optional types.
func TestEffects_AssertNotNil(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "assert function narrows optional",
			Code: `
				function assert_not_nil(x: any)
					if x == nil then error("nil") end
				end

				function f(x: number?)
					assert_not_nil(x)
					local n: number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "stdlib assert narrows",
			Code: `
				function f(x: number?)
					assert(x ~= nil)
					local n: number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestEffects_ErrorTerminates tests that error() terminates control flow.
func TestEffects_ErrorTerminates(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "error terminates branch",
			Code: `
				function f(x: number?)
					if x == nil then
						error("x is nil")
					end
					local n: number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "early error allows narrowing",
			Code: `
				function validate(data: {value: number}?)
					if not data then error("no data") end
					local v: number = data.value
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestEffects_AssertTypeNarrowing tests type assertion narrowing effects.
func TestEffects_AssertTypeNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "typeof assert narrows",
			Code: `
				function assert_string(x: any)
					if type(x) ~= "string" then error("not string") end
				end

				function f(x: any)
					assert_string(x)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "equality assert narrows",
			Code: `
				function assert_eq(a: any, b: any)
					if a ~= b then error("not equal") end
				end

				function f(x: any, expected: string)
					assert_eq(x, expected)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestEffects_CallEffectPropagation tests that call effects propagate.
func TestEffects_CallEffectPropagation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "wrapped assert propagates effect",
			Code: `
				function my_assert(cond: any)
					if not cond then error("failed") end
				end

				function check_not_nil(x: any)
					my_assert(x ~= nil)
				end

				function f(x: number?)
					check_not_nil(x)
					local n: number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestEffects_ReturnEffects tests that return statements affect narrowing.
func TestEffects_ReturnEffects(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "return in branch allows narrowing after",
			Code: `
				function f(x: number?): number
					if x == nil then
						return 0
					end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple returns with narrowing",
			Code: `
				function f(x: number?): number
					if x == nil then return -1 end
					if x < 0 then return 0 end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

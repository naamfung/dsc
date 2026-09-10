package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestGuards_TypeIsNarrowing tests Type:is() pattern for narrowing.
func TestGuards_TypeIsNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "type is guard narrows",
			Code: `
				type Point = {x: number, y: number}
				function validate(obj: any)
					local _, err = Point:is(obj)
					if err == nil then
						local p: {x: number, y: number} = obj
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type is guard on nested field",
			Code: `
				type Point = {x: number, y: number}
				function validate(obj: {p: any})
					local _, err = Point:is(obj.p)
					if err == nil then
						local p: {x: number, y: number} = obj.p
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGuards_TypeCallNarrowing tests Type(x) pattern for narrowing.
func TestGuards_TypeCallNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "type call narrows without loop",
			Code: `
				type Point = {x: number, y: number}
				function validate(x: any)
					Point(x)
					local p: {x: number, y: number} = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type call narrows in while loop",
			Code: `
				type Point = {x: number, y: number}
				function validate(xs: {[integer]: any})
					local i = 1
					while i <= 10 do
						local v = xs[i]
						Point(v)
						local p: {x: number, y: number} = v
						i = i + 1
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "type call narrows in numeric for loop",
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
	}
	testutil.RunCases(t, tests)
}

// TestGuards_TypeIsInGenericFor tests Type:is() in generic for loops.
func TestGuards_TypeIsInGenericFor(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "type is guard in ipairs loop",
			Code: `
				type Point = {x: number, y: number}
				function validate(xs: {[integer]: any})
					for i, v in ipairs(xs) do
						local _, err = Point:is(v)
						if err == nil then
							local p: {x: number, y: number} = v
						end
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGuards_NotGuardClause tests early return guard clause pattern.
func TestGuards_NotGuardClause(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "not guard clause narrows",
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
			Name: "nil check guard clause",
			Code: `
				function process(data: {value: number}?)
					if data == nil then return end
					local v: number = data.value
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGuards_RelationalBranchNarrowing tests narrowing from field comparisons.
func TestGuards_RelationalBranchNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "field equals variable narrows union",
			Code: `
				type ChanInt = {__tag: "int"}
				type ChanStr = {__tag: "str"}
				type SelResult =
					{channel: ChanInt, value: {error: string}, ok: boolean} |
					{channel: ChanStr, value: {data: number}, ok: boolean}

				function get_result(a: ChanInt, b: ChanStr): SelResult
					return {channel = a, value = {error = "oops"}, ok = true}
				end

				function f(ch1: ChanInt, ch2: ChanStr)
					local result = get_result(ch1, ch2)
					if result.channel == ch1 then
						local e: string = result.value.error
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestGuards_FieldTruthyNarrowsUnion(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "field truthy narrows union with missing field",
			Code: `
				type Result = {error: string} | {name: string}
				function f(flag: boolean): string
					local res: Result
					if flag then
						res = { error = "bad" }
					else
						res = { name = "ok" }
					end
					if res.error then
						return "err"
					end
					local name: string = res.name
					return name
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

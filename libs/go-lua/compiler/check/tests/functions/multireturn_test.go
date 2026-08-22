package functions

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestMultiReturnExpansion tests that multi-return expansion is correctly applied.
func TestMultiReturnExpansion(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "multi-return to multiple targets",
			Code: `
				local function pair(): ({x: number}, {y: string})
					return {x = 1}, {y = "hello"}
				end
				local a, b = pair()
				local x: number = a.x
				local y: string = b.y
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multi-return first value only for single target",
			Code: `
				local function pair(): ({x: number}, {y: string})
					return {x = 1}, {y = "hello"}
				end
				local a = pair()
				local x: number = a.x
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multi-return to field targets",
			Code: `
				local function pair(): (number, string)
					return 1, "hello"
				end
				local a: number, b: string = pair()
				local x: number = a
				local y: string = b
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "extra targets get nil",
			Code: `
				local function single(): number
					return 42
				end
				local a: number, b: nil = single()
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestDeclaredVsInferredTypes tests type inference priority.
func TestDeclaredVsInferredTypes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "explicit annotation wins over inference",
			Code: `
				local n: number = 42
				local x: number = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function declared return type used at call site",
			Code: `
				local function getTable(): {x: any}
					return {x = 42}
				end
				local t = getTable()
				local a: any = t.x
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function return type prevents narrow assignment",
			Code: `
				local function getTable(): {x: any}
					return {x = 42}
				end
				local t = getTable()
				local n: number = t.x
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "table literal preserves precise type",
			Code: `
				local t = {x = 42}
				local n: number = t.x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

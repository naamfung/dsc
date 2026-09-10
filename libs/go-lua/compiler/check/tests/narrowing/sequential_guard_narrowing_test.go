package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

// TestSequentialGuardNarrowing covers stacked guard patterns that previously
// produced "never" after sequential checks.
func TestSequentialGuardNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "type() guard after nil check",
			Code: `
				type Err = {kind: string}
				function error_kind(err: Err?, expected_kind: string)
					if err == nil then error("nil") end
					if type(err) ~= "table" then error("not table") end
					if err.kind ~= expected_kind then
						error("wrong kind")
					end
					local k: string = err.kind
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple field inequality guards on narrowed value",
			Code: `
				type Meta = {role: string, department: string}
				function check(meta: Meta?)
					if not meta then error("no meta") end
					if meta.role ~= "admin" then error("not admin") end
					if meta.department ~= "engineering" then
						error("not engineering")
					end
					local dept: string = meta.department
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert + helper type guard chain",
			Code: `
				type Err = {kind: string}

				local function assert_table(x: any)
					if type(x) ~= "table" then error("not table") end
				end

				function error_kind(err: Err?, expected_kind: string)
					assert(err ~= nil)
					assert_table(err)
					if err.kind ~= expected_kind then
						error("wrong kind")
					end
					local k: string = err.kind
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "inner helper terminators with sequential guards",
			Code: `
				type Meta = {role: string, department: string}

				function check(meta: Meta?)
					local function require_meta(x: any)
						if x == nil then error("no meta") end
					end

					local function fail(msg: string)
						error(msg)
					end

					require_meta(meta)
					if meta.role ~= "admin" then fail("not admin") end
					if meta.department ~= "engineering" then fail("not engineering") end
					local dept: string = meta.department
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "err optional narrowed through sequential guards with or type check",
			Code: `
				local funcs = require("funcs")

				function main()
					local result, err = funcs.call("app.test.temporal:error_activity", {})
					if err then
						if type(err) == "userdata" or type(err) == "table" then
							if err.retryable then
								local r: boolean = err:retryable()
							end
							if err.message then
								local m: string = err:message()
							end
						end
					end
					return result
				end
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{
				"funcs": testutil.FuncsManifest(),
			},
		},
	}
	testutil.RunCases(t, tests)
}

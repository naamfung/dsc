package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestPhiUnknownJoin tests that phi joins don't include unknown in the result type
// when one branch has a concrete type.
//
// Bug: When a variable has type unknown on one CFG path and a concrete type
// on another path, the phi join created unknown | concrete instead of just concrete.
func TestPhiUnknownJoin(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "conditional assignment - one branch assigns string",
			Code: `
				local function test(cond: boolean)
					local plaintext
					if cond then
						plaintext = "Hello, Temporal!"
					end
					-- plaintext should be string?, not unknown | "Hello, Temporal!"
					local s: string? = plaintext
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "conditional assignment to typed parameter function",
			Code: `
				local function hmac(secret: string, data: string): string
					return secret .. data
				end

				local function test(cond: boolean)
					local plaintext
					if cond then
						plaintext = "Hello, Temporal!"
					else
						plaintext = "default"
					end
					-- plaintext should be string (both branches assign string)
					local result = hmac("secret", plaintext)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multi-return assignment propagates types",
			Code: `
				local function getData(): (string, string?)
					return "data", nil
				end

				local function process(data: string): string
					return data
				end

				local function test()
					local plaintext, err = getData()
					-- plaintext should be string, not unknown
					local result = process(plaintext)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "early return doesn't contaminate types",
			Code: `
				local function getData(): (string, string?)
					return "data", nil
				end

				local function process(data: string): string
					return data
				end

				local function test()
					local plaintext, err = getData()
					if err then
						return
					end
					-- plaintext should still be string after the if block
					local result = process(plaintext)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

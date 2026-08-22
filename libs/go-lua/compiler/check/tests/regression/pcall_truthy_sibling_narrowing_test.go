package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard for pcall-style handlers used across llm providers:
// `if not ok then return error_record end; return mapped` must keep the mapped
// success shape in the truthy-ok fallthrough path.
func TestRegression_PcallTruthyGuardNarrowsMappedReturn(t *testing.T) {
	source := `
local function map_error_response(message: string)
	return {
		success = false,
		error = "server_error",
		error_message = message,
		metadata = {},
	}
end

local function map_success_response()
	return {
		success = true,
		result = { content = "ok" },
		tokens = { prompt_tokens = 1, completion_tokens = 1, total_tokens = 2 },
		finish_reason = "stop",
		metadata = {},
	}
end

local function handler()
	local ok, mapped = pcall(function()
		return map_success_response()
	end)
	if not ok then
		return map_error_response("failed")
	end
	return mapped
end

local response = handler()
assert(response.success)
local lowered = response.result.content:lower()
assert(lowered)
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for pcall truthy-guard narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

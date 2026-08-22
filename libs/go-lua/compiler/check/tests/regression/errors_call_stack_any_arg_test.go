package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Runtime errors.call_stack accepts non-error values and returns nil.
// Lint should not require the argument to be statically typed as Error.
func TestErrorsCallStack_AcceptsAny(t *testing.T) {
	source := `
local function format_error_message(err)
	local message = tostring(err)
	if errors and errors.call_stack then
		local cs = errors.call_stack(err)
		if cs and cs.frames and #cs.frames > 0 then
			return message .. ":stack"
		end
	end
	return message
end

local ok, err = pcall(function()
	error("boom")
end)
if not ok then
	local out = format_error_message(err)
end
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

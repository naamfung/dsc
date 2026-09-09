package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Reassignment with explicit cast should override prior local narrow type.
func TestReassignmentCastOverridesPriorType(t *testing.T) {
	source := `
		local function f(x)
			local resp = false
			resp = x :: any
			local _ = resp.usage
		end

		f({ usage = 1 })
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

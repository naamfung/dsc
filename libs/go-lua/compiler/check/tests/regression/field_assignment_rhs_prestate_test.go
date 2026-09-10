package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: field-assignment RHS must be checked against pre-assignment
// state at the same CFG point (Lua evaluates RHS before LHS write).
func TestFieldAssignmentRHSUsesPreAssignmentState(t *testing.T) {
	source := `
		local state: { restart_count: number }? = { restart_count = 0 }
		if not state then
			return
		end

		state.restart_count = state.restart_count + 1
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

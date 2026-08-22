package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: unreachable truthy branches (callee narrowed to never)
// must not emit secondary "expected function, got never" diagnostics.
func TestDeadBranchCallNever_NoFalsePositive(t *testing.T) {
	source := `
		local f = nil
		if f then
			f()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy framework test runner:
// existing_after_each() call appears in a guarded wrapper branch.
func TestDeadBranchCallNever_AfterEachWrapperPattern(t *testing.T) {
	source := `
		local ctx = { after_each = nil }
		local fn = function() end
		local existing_after_each = ctx.after_each

		if existing_after_each then
			ctx.after_each = function()
				existing_after_each()
				fn()
			end
		else
			ctx.after_each = function()
				fn()
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

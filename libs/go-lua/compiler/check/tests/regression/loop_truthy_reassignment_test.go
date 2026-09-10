package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: truthy loop conditions must narrow inside loop bodies even
// when the same variable is reassigned within the body.
func TestLoopTruthyCondition_PreservedInsideBodyBeforeReassign(t *testing.T) {
	source := `
		local function ancestry(suite)
			local ancestry = {}
			local current = suite
			while current do
				table.insert(ancestry, 1, current)
				current = current.parent
			end
			return ancestry
		end

		local root = {
			after_each = function() end,
			parent = nil,
		}

		local chain = ancestry(root)
		local ancestor = chain[1]
		if ancestor.after_each then
			ancestor.after_each()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

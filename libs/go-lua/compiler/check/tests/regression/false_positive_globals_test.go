package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestFalsePositive_UnknownGlobalCalledAsFunction reproduces the wippy test
// framework pattern where describe/it/before_each are globals set dynamically
// by _G.it = test.it at runtime. The checker cannot see these assignments.
func TestFalsePositive_UnknownGlobalCalledAsFunction(t *testing.T) {
	source := `
		local function define_tests()
			describe("test suite", function()
				it("should work", function()
					local x = 1
				end)
			end)
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("line %d: %s", d.Position.Line, d.Message)
	}
}

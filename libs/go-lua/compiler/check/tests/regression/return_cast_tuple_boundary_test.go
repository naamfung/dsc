package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// A cast in return position must be honored even for multi-value return
// signatures like (T?, string?).
func TestReturnCast_TupleBoundary(t *testing.T) {
	source := `
		type SelectionResult = {
			success: boolean,
			agent: string,
			reason: string,
		}

		local function pick(): (SelectionResult?, string?)
			local result: any = {
				success = true,
				agent = "a1",
				reason = "ok",
			}
			return result :: SelectionResult, nil
		end

		local _, _ = pick()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard (docker containers_list pattern):
// a progressively-built record with optional fields must be accepted when the
// callee expects required fields whose types already admit nil.
func TestOptionalLikeRecordParamFlow(t *testing.T) {
	source := `
		local function list(filter: {limit: number?, status: string?, status_not: string?}?)
			return nil
		end

		local filter = {
			limit = 100,
		}

		local status_param: string? = nil
		if status_param and status_param ~= "" then
			filter.status = status_param
		else
			filter.status_not = "removed"
		end

		list(filter)
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for optional-like record param flow, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

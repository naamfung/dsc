package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// Regression: false branch of one predicate must not over-narrow unknown
// values to nil before a second predicate check.
func TestPredicateFalseBranch_DoesNotCollapseUnknownToNil(t *testing.T) {
	source := `
		local function is_exit(result: any): boolean
			return type(result) == "table" and result._actor_exit == true
		end

		local function is_next(result: any): boolean
			return type(result) == "table" and result._actor_next == true
		end

		local function provider(): any
			return unknown_value()
		end

		local init_result = provider()
		if is_exit(init_result) then
			return init_result.result
		end

		if is_next(init_result) then
			local topic = init_result.topic
			local payload = init_result.payload
			return topic, payload
		end

		return nil
	`

	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Fatalf("unexpected error at %d:%d: %s", d.Position.Line, d.Position.Column, d.Message)
		}
	}
}

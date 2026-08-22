package inference

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Multi-return stability: second return should remain a table, not nil.
func TestReturnTuple_SecondValueStable(t *testing.T) {
	source := `
		local function group_by_suite(entries)
			local suites = {}
			local no_suite = {}
			return suites, no_suite
		end

		local suites, no_suite = group_by_suite({})
		local n: number = #no_suite
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for multi-return second value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// First return precision: first element should remain integer when all returns are integer.
func TestReturnTuple_FirstValuePrecision(t *testing.T) {
	source := `
		local function run_suite()
			if true then
				return 10, {}
			else
				return 20, {}
			end
		end

		local count, failures = run_suite()
		local n: number = count
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for first return precision, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

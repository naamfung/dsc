package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// Regression: call-expression predicate constraints are one-sided.
// False branch of is_table(v) must not collapse v to nil.
func TestCallPredicate_FalseBranchDoesNotNegateToNil(t *testing.T) {
	source := `
		local function is_table(v: any): boolean
			return type(v) == "table"
		end

		local value = unknown_value()
		if is_table(value) then
			local _a = value.foo
		else
			local _b = value.foo
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Fatalf("unexpected error at %d:%d: %s", d.Position.Line, d.Position.Column, d.Message)
		}
	}
}

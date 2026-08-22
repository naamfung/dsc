package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: `table` annotations should accept map-like row values,
// matching Lua's generic table semantics.
func TestRegression_TableAnnotationAcceptsMapRow(t *testing.T) {
	source := `
		local function query(): ({[string]: any}[]?, string?)
			return {{ id = "x", value = 1 }}, nil
		end

		local function get(): (table?, string?)
			local rows, err = query()
			if err then
				return nil, err
			end
			if not rows or #rows == 0 then
				return nil, nil
			end
			return rows[1]
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for table? return from map row, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

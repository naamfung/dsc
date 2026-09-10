package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: nested calls used as arguments must still contribute
// parameter hints to local helper functions.
func TestNestedCall_ParamHintsFlowIntoLocalHelper(t *testing.T) {
	source := `
		type Entry = { id: string, kind: string }

		local function extract(entry)
			return {
				id = entry.id,
				kind = entry.kind,
			}
		end

		local entries: {Entry} = {
			{ id = "a", kind = "k" },
		}

		local out = {}
		for _, entry in ipairs(entries) do
			table.insert(out, extract(entry))
		end

		for _, item in ipairs(out) do
			local id: string = item.id
			local kind: string = item.kind
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

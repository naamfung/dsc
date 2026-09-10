package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: after type(x) == "table", a literal field comparison
// should refine the table-top marker to a structural record carrying the
// discriminant field.
func TestTableTopFieldLiteralNarrowing(t *testing.T) {
	source := `
		type ContentPart = { type: string }

		local function refine(x: any): ContentPart?
			if type(x) == "table" and x.type == "image" then
				local cp: ContentPart = x
				return x
			end
			return nil
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

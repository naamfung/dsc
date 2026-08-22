package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_AssertNestedContentNarrowing(t *testing.T) {
	source := `
type Response =
	{ success: true, result: { content: string } } |
	{ success: false, error_message: string }

local function handler(): Response
	return {
		success = true,
		result = { content = "Ahoy!" }
	}
end

local response = handler()
assert(response.success)
assert(response.result.content)
local lowered = response.result.content:lower()
assert(lowered)
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for nested assert-based narrowing")
	}
}

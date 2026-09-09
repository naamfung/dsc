package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Reproduces llm test pattern:
// - function returns success/error discriminated union
// - helper assertion enforces success == false
// - error_message should be string in following call
func TestRegression_AssertFalseDiscriminantNarrowing(t *testing.T) {
	source := `
type Response =
	{ success: true, result: {content: string} } |
	{ success: false, error: string, error_message: string }

local function is_false(v: any)
	if v ~= false then
		error("expected false")
	end
end

local function contains(str: string, substr: string)
	if type(str) ~= "string" or not string.find(str, substr, 1, true) then
		error("expected string to contain substring")
	end
end

local function handler(): Response
	return {
		success = false,
		error = "invalid_request",
		error_message = "Model is required"
	}
end

local response = handler()
is_false(response.success)
contains(response.error_message, "Model is required")
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for assert-based discriminant narrowing")
	}
}

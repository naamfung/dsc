package inference

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestOptionalParam_InferredByOrDefault(t *testing.T) {
	source := `
local function eq(actual, expected, msg)
    if actual ~= expected then
        error((msg or "assertion failed") .. ": expected " .. tostring(expected) .. ", got " .. tostring(actual), 2)
    end
end

eq("a", "b")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOptionalParam_AnnotatedStillRequiresArgs(t *testing.T) {
	source := `
local function eq(actual: string, expected: string, msg: string)
    if actual ~= expected then
        error((msg or "assertion failed") .. ": expected " .. tostring(expected) .. ", got " .. tostring(actual), 2)
    end
end

eq("a", "b")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected wrong-arity error for annotated params, got none")
	}
}

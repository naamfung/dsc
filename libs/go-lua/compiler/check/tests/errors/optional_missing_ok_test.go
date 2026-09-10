package errors

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestOptionalReturnAllowsMissing(t *testing.T) {
	result := testutil.Check(`
local function f(): number?
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOptionalLocalWithoutInitializerAllowsNil(t *testing.T) {
	result := testutil.Check(`
local x: number?
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

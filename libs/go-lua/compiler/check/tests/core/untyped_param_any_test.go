package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestUntypedParamDefaultsToAny(t *testing.T) {
	source := `
local function g(x)
    return math.floor(x)
end

local n: number = g(1.2)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

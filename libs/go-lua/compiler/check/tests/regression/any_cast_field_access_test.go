package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAnyCastAllowsDynamicFieldAccess(t *testing.T) {
	source := `
		local maybe_false: false? = false
		local dyn = maybe_false :: any
		local v = dyn.usage
		local w = dyn["metadata"]
		local x = dyn:method_call()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no checker errors, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

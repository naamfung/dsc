package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Explicit dynamic receivers should allow method calls even when reached via
// nested field access (ctx.reader:method()).
func TestAnyNestedMethodCallAllowed(t *testing.T) {
	source := `
		function run(ctx)
			local session_context, err = ctx.reader:get_full_context()
			if err then
				session_context = {}
			end
			return session_context
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

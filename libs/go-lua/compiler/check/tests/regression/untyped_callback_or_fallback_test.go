package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestFP_UntypedCallbackFallbackDoesNotCollapseToZeroArity(t *testing.T) {
	source := `
		local function run(callbacks)
			callbacks = callbacks or {}
			local on_content = callbacks.on_content or function() end
			local on_error = callbacks.on_error or function() end
			local on_done = callbacks.on_done or function() end

			on_content("x")
			on_error({ message = "boom" })
			on_done({ content = "ok" })
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for untyped callback or-fallback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

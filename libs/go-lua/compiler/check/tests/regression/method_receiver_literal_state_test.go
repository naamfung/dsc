package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard for method receiver typing:
// literal-only receiver fields (intercepted = false) must not make truthy
// method guards unreachable when sibling methods mutate that field.
func TestMethodReceiverLiteralStateDoesNotPoisonCallableField(t *testing.T) {
	source := `
		local Bus = {
			context = nil :: any,
			intercepted = false,
			intercept_handler = nil :: ((any, any) -> (any, string?))?,
		}
		Bus.__index = Bus

		function Bus:intercept(fn)
			self.intercepted = true
			self.intercept_handler = fn
		end

		function Bus:process_operation(op)
			if self.intercepted then
				if self.intercept_handler and type(self.intercept_handler) == "function" then
					local _, _ = self.intercept_handler(self.context, op)
				end
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

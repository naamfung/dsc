package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Guards against order-sensitive narrowing when a value is reassigned from `any`
// and then refined with type(x) == "table".
func TestTypeofTableAfterAnyReassign_IsStable(t *testing.T) {
	source := `
		local function f(msg: { metadata: string }, v: any)
			local meta = msg.metadata
			if type(meta) == "string" then
				local decoded = v
				if decoded then
					meta = decoded
				end
			end
			if type(meta) == "table" then
				if meta.model then
					return tostring(meta.model)
				end
			end
			return ""
		end
	`

	for i := 0; i < 80; i++ {
		result := testutil.Check(source, testutil.WithStdlib())
		if result.HasError() {
			t.Fatalf("iteration %d: expected no errors, got %v", i+1, testutil.ErrorMessages(result.Diagnostics))
		}
	}
}

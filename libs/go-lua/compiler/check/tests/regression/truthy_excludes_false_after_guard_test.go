package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestTruthyGuardEliminatesFalseAndNil(t *testing.T) {
	source := `
		local function needs_payload(v: {usage: number} | false?)
			if not v then
				return 0
			end
			local n: number = v.usage
			return n
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no checker errors, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_BuiltinTableTopAllowsDynamicFieldAccess(t *testing.T) {
	source := `
		local user_config: table = {}
		local _ctx = user_config.context_merger
		if user_config.delegates then
			local _tool = user_config.delegates.tool_schema
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for dynamic field access on table top, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

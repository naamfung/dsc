package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Guard against cast loss after call-site parameter inference.
// Even if a param is inferred from mixed callers, local cast to any must win.
func TestAnyCastAfterParamInference_RemainsDynamic(t *testing.T) {
	source := `
		local function format_success_response(claude_response)
			local resp = claude_response :: any
			local tokens = resp.usage
			local finish_reason = resp.stop_reason
			local metadata = resp.metadata or {}
			return tokens, finish_reason, metadata
		end

		local _ = format_success_response(false)
		local _ = format_success_response({
			usage = { prompt_tokens = 1 },
			stop_reason = "end_turn",
			metadata = {},
		})
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

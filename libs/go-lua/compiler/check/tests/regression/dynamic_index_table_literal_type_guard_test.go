package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard for service/event patterns:
// a table literal assigned through dynamic index must preserve type()
// narrowing on sourced fields through later map read.
func TestDynamicIndexTableLiteralPreservesTypeGuardedField(t *testing.T) {
	source := `
		local function send(pid: string, topic: string, payload: any)
		end

		local function service(payload: any, worker_pid: string, event_from: string)
			local pending = {}
			if type(payload.respond_to) ~= "string" then
				return
			end

			pending[worker_pid] = {
				from = "caller",
				respond_to = payload.respond_to,
			}

			local operation = pending[event_from]
			if operation then
				send(operation.from, operation.respond_to, {})
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for type-guarded field in dynamic-index table literal, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

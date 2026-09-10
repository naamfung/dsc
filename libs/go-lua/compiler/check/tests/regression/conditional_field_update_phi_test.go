package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: when different branches assign different fields on the
// same table, phi merging must preserve those fields as nil-capable instead of
// dropping them from the merged shape.
func TestConditionalFieldUpdatesSurvivePhiJoin(t *testing.T) {
	source := `
		type Updates = {
			title: string?,
			status: string?,
			kind: string?,
		}

		local function update(session_id: string, updates: Updates)
			local result = { session_id = session_id, updated = true }

			if updates.title ~= nil then
				result.title = updates.title
			end
			if updates.status ~= nil then
				result.status = updates.status
			end
			if updates.kind ~= nil then
				result.kind = updates.kind
			end

			result.last_message_date = "now"
			return result
		end

		local result = update("s", {
			title = "title",
			status = "active",
			kind = "default",
		})

		local title: string? = result.title
		local status: string? = result.status
		local kind: string? = result.kind
		return title, status, kind
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors after conditional field updates, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
